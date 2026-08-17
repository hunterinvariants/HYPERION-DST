package statemachine

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"sort"

	"github.com/hunterinvariants/promtact/raft"
)

var (
	ErrStaleRequest = errors.New("statemachine: stale request")
	ErrBadSnapshot  = errors.New("statemachine: invalid snapshot")
)

type Result struct {
	ClientID  uint64
	RequestID uint64
	Value     uint64
	Found     bool
}

type clientRecord struct {
	RequestID uint64
	Value     uint64
	Found     bool
}

type Machine struct {
	values  map[uint64]uint64
	clients map[uint64]clientRecord
}

func New() *Machine {
	return &Machine{values: make(map[uint64]uint64), clients: make(map[uint64]clientRecord)}
}

func (m *Machine) Apply(entry raft.Entry) (Result, error) {
	if entry.Operation == raft.CommandLegacy {
		return Result{}, nil
	}
	if previous, ok := m.clients[entry.ClientID]; ok {
		if previous.RequestID == entry.RequestID {
			return Result{entry.ClientID, previous.RequestID, previous.Value, previous.Found}, nil
		}
		if entry.RequestID < previous.RequestID {
			return Result{}, ErrStaleRequest
		}
	}
	result := Result{ClientID: entry.ClientID, RequestID: entry.RequestID}
	switch entry.Operation {
	case raft.CommandPut:
		m.values[entry.Key] = entry.Value
		result.Value, result.Found = entry.Value, true
	case raft.CommandDelete:
		result.Value, result.Found = m.values[entry.Key]
		delete(m.values, entry.Key)
	default:
		return Result{}, errors.New("statemachine: unsupported operation")
	}
	m.clients[entry.ClientID] = clientRecord{result.RequestID, result.Value, result.Found}
	return result, nil
}

func (m *Machine) Get(key uint64) (uint64, bool) {
	value, ok := m.values[key]
	return value, ok
}

func (m *Machine) Cached(clientID, requestID uint64) (Result, bool, error) {
	record, ok := m.clients[clientID]
	if !ok || requestID > record.RequestID {
		return Result{}, false, nil
	}
	if requestID < record.RequestID {
		return Result{}, false, ErrStaleRequest
	}
	return Result{clientID, requestID, record.Value, record.Found}, true, nil
}

func (m *Machine) Snapshot() []byte {
	keys := make([]uint64, 0, len(m.values))
	for key := range m.values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	clients := make([]uint64, 0, len(m.clients))
	for client := range m.clients {
		clients = append(clients, client)
	}
	sort.Slice(clients, func(i, j int) bool { return clients[i] < clients[j] })

	size := 20 + len(keys)*16 + len(clients)*32
	out := make([]byte, size)
	copy(out[:4], "HYSM")
	binary.LittleEndian.PutUint16(out[4:6], 1)
	binary.LittleEndian.PutUint32(out[8:12], uint32(len(keys)))
	binary.LittleEndian.PutUint32(out[12:16], uint32(len(clients)))
	offset := 16
	for _, key := range keys {
		binary.LittleEndian.PutUint64(out[offset:offset+8], key)
		binary.LittleEndian.PutUint64(out[offset+8:offset+16], m.values[key])
		offset += 16
	}
	for _, client := range clients {
		record := m.clients[client]
		binary.LittleEndian.PutUint64(out[offset:offset+8], client)
		binary.LittleEndian.PutUint64(out[offset+8:offset+16], record.RequestID)
		binary.LittleEndian.PutUint64(out[offset+16:offset+24], record.Value)
		if record.Found {
			out[offset+24] = 1
		}
		offset += 32
	}
	binary.LittleEndian.PutUint32(out[len(out)-4:], crc32.Checksum(out[:len(out)-4], crc32.MakeTable(crc32.Castagnoli)))
	return out
}

func Restore(snapshot []byte) (*Machine, error) {
	if len(snapshot) < 20 || !bytes.Equal(snapshot[:4], []byte("HYSM")) ||
		binary.LittleEndian.Uint16(snapshot[4:6]) != 1 {
		return nil, ErrBadSnapshot
	}
	expected := binary.LittleEndian.Uint32(snapshot[len(snapshot)-4:])
	if crc32.Checksum(snapshot[:len(snapshot)-4], crc32.MakeTable(crc32.Castagnoli)) != expected {
		return nil, ErrBadSnapshot
	}
	keyCount := int(binary.LittleEndian.Uint32(snapshot[8:12]))
	clientCount := int(binary.LittleEndian.Uint32(snapshot[12:16]))
	if 20+keyCount*16+clientCount*32 != len(snapshot) {
		return nil, ErrBadSnapshot
	}
	m := New()
	offset := 16
	for range keyCount {
		key := binary.LittleEndian.Uint64(snapshot[offset : offset+8])
		m.values[key] = binary.LittleEndian.Uint64(snapshot[offset+8 : offset+16])
		offset += 16
	}
	for range clientCount {
		client := binary.LittleEndian.Uint64(snapshot[offset : offset+8])
		m.clients[client] = clientRecord{
			RequestID: binary.LittleEndian.Uint64(snapshot[offset+8 : offset+16]),
			Value:     binary.LittleEndian.Uint64(snapshot[offset+16 : offset+24]),
			Found:     snapshot[offset+24] == 1,
		}
		offset += 32
	}
	return m, nil
}
