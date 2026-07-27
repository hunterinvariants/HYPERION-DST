// Package wal implements HYPERION's checksummed write-ahead log.
package wal

import (
	"encoding/binary"
	"errors"
	"hash/crc32"

	"github.com/hunterinvariants/HYPERION-DST/storage"
)

const (
	RecordSize = 48
	version    = uint16(1)
)

var (
	magic       = [8]byte{'H', 'Y', 'P', 'W', 'A', 'L', '0', '1'}
	crcTable    = crc32.MakeTable(crc32.Castagnoli)
	ErrChecksum = errors.New("wal: checksum mismatch")
	ErrFormat   = errors.New("wal: invalid record format")
	ErrSequence = errors.New("wal: non-contiguous sequence")
)

type Record struct {
	Sequence uint64
	Entry    storage.Entry
}

func Encode(dst *[RecordSize]byte, r Record) {
	copy(dst[0:8], magic[:])
	binary.LittleEndian.PutUint16(dst[8:10], version)
	binary.LittleEndian.PutUint16(dst[10:12], 0)
	binary.LittleEndian.PutUint64(dst[12:20], r.Sequence)
	binary.LittleEndian.PutUint64(dst[20:28], r.Entry.Index)
	binary.LittleEndian.PutUint64(dst[28:36], r.Entry.Term)
	binary.LittleEndian.PutUint64(dst[36:44], r.Entry.Command)
	binary.LittleEndian.PutUint32(dst[44:48], crc32.Checksum(dst[:44], crcTable))
}

func Decode(src []byte) (Record, error) {
	if len(src) != RecordSize || string(src[:8]) != string(magic[:]) ||
		binary.LittleEndian.Uint16(src[8:10]) != version {
		return Record{}, ErrFormat
	}
	want := binary.LittleEndian.Uint32(src[44:48])
	if got := crc32.Checksum(src[:44], crcTable); got != want {
		return Record{}, ErrChecksum
	}
	return Record{
		Sequence: binary.LittleEndian.Uint64(src[12:20]),
		Entry: storage.Entry{
			Index:   binary.LittleEndian.Uint64(src[20:28]),
			Term:    binary.LittleEndian.Uint64(src[28:36]),
			Command: binary.LittleEndian.Uint64(src[36:44]),
		},
	}, nil
}
