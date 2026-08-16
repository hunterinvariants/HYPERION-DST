package protocol

import (
	"encoding/binary"

	"github.com/hunterinvariants/hyperion/raft"
)

const peerFixedSize = 176

func EncodePeer(message raft.Message) []byte {
	out := make([]byte, peerFixedSize+len(message.Snapshot))
	out[0] = byte(message.Type)
	if message.Reject {
		out[1] = 1
	}
	if message.HasEntry {
		out[2] = 1
	}
	binary.LittleEndian.PutUint32(out[4:8], message.From)
	binary.LittleEndian.PutUint32(out[8:12], message.To)
	values := []uint64{
		message.Term, message.LogIndex, message.LogTerm, message.Commit,
		message.Match, message.Context, message.SnapshotIndex, message.SnapshotTerm,
	}
	for index, value := range values {
		binary.LittleEndian.PutUint64(out[12+index*8:20+index*8], value)
	}
	offset := 76
	binary.LittleEndian.PutUint64(out[offset:offset+8], message.Entry.Term)
	binary.LittleEndian.PutUint64(out[offset+8:offset+16], message.Entry.Command)
	out[offset+16] = byte(message.Entry.Kind)
	out[offset+17] = byte(message.Entry.Operation)
	binary.LittleEndian.PutUint64(out[offset+24:offset+32], message.Entry.OldVoters)
	binary.LittleEndian.PutUint64(out[offset+32:offset+40], message.Entry.NewVoters)
	binary.LittleEndian.PutUint64(out[offset+40:offset+48], message.Entry.ClientID)
	binary.LittleEndian.PutUint64(out[offset+48:offset+56], message.Entry.RequestID)
	binary.LittleEndian.PutUint64(out[offset+56:offset+64], message.Entry.Key)
	binary.LittleEndian.PutUint64(out[offset+64:offset+72], message.Entry.Value)
	binary.LittleEndian.PutUint64(out[148:156], message.SnapshotOld)
	binary.LittleEndian.PutUint64(out[156:164], message.SnapshotNew)
	binary.LittleEndian.PutUint32(out[164:168], uint32(len(message.Snapshot)))
	copy(out[peerFixedSize:], message.Snapshot)
	return out
}

func DecodePeer(data []byte) (raft.Message, error) {
	if len(data) < peerFixedSize {
		return raft.Message{}, ErrFrame
	}
	length := binary.LittleEndian.Uint32(data[164:168])
	if int(length) != len(data)-peerFixedSize {
		return raft.Message{}, ErrFrame
	}
	message := raft.Message{
		Type:     raft.MessageType(data[0]),
		Reject:   data[1] != 0,
		HasEntry: data[2] != 0,
		From:     binary.LittleEndian.Uint32(data[4:8]),
		To:       binary.LittleEndian.Uint32(data[8:12]),
	}
	fields := []*uint64{
		&message.Term, &message.LogIndex, &message.LogTerm, &message.Commit,
		&message.Match, &message.Context, &message.SnapshotIndex, &message.SnapshotTerm,
	}
	for index, field := range fields {
		*field = binary.LittleEndian.Uint64(data[12+index*8 : 20+index*8])
	}
	offset := 76
	message.Entry = raft.Entry{
		Term:      binary.LittleEndian.Uint64(data[offset : offset+8]),
		Command:   binary.LittleEndian.Uint64(data[offset+8 : offset+16]),
		Kind:      raft.EntryKind(data[offset+16]),
		Operation: raft.CommandOp(data[offset+17]),
		OldVoters: binary.LittleEndian.Uint64(data[offset+24 : offset+32]),
		NewVoters: binary.LittleEndian.Uint64(data[offset+32 : offset+40]),
		ClientID:  binary.LittleEndian.Uint64(data[offset+40 : offset+48]),
		RequestID: binary.LittleEndian.Uint64(data[offset+48 : offset+56]),
		Key:       binary.LittleEndian.Uint64(data[offset+56 : offset+64]),
		Value:     binary.LittleEndian.Uint64(data[offset+64 : offset+72]),
	}
	message.SnapshotOld = binary.LittleEndian.Uint64(data[148:156])
	message.SnapshotNew = binary.LittleEndian.Uint64(data[156:164])
	message.Snapshot = append([]byte(nil), data[peerFixedSize:]...)
	return message, nil
}
