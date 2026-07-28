// Package raftwal adapts the checksummed WAL to Raft's stable-store contract.
package raftwal

import (
	"context"
	"fmt"

	"github.com/hunterinvariants/HYPERION-DST/raft"
	"github.com/hunterinvariants/HYPERION-DST/storage"
	"github.com/hunterinvariants/HYPERION-DST/storage/wal"
)

// Index zero is reserved for hard-state records. Command stores VotedFor.
const (
	hardStateIndex uint64 = 0
	compactIndex          = ^uint64(0)
	resetIndex            = ^uint64(0) - 1
)

type Store struct {
	log  *wal.Log
	hard raft.HardState
	data []raft.Entry // includes the compacted-base sentinel
	base uint64
}

func Open(device wal.Device) (*Store, error) {
	log, records, err := wal.Open(device)
	if err != nil {
		return nil, err
	}
	s := &Store{log: log, data: []raft.Entry{{}}}
	for _, record := range records {
		e := record.Entry
		if e.Index == resetIndex {
			if e.Term < s.base {
				return nil, fmt.Errorf("raftwal: reset index regression %d", e.Term)
			}
			s.base = e.Term
			s.data = []raft.Entry{{Term: e.Command}}
			continue
		}
		if e.Index == compactIndex {
			if e.Term < s.base || e.Term > s.base+uint64(len(s.data))-1 {
				return nil, fmt.Errorf("raftwal: invalid compaction index %d", e.Term)
			}
			offset := e.Term - s.base
			s.data = append([]raft.Entry(nil), s.data[offset:]...)
			s.base = e.Term
			continue
		}
		if e.Index == hardStateIndex {
			if e.Term < s.hard.Term || e.OldVoters < s.hard.Commit ||
				(e.Term == s.hard.Term && s.hard.VotedFor != 0 && uint32(e.Command) != s.hard.VotedFor) {
				return nil, fmt.Errorf("raftwal: hard-state regression")
			}
			s.hard = raft.HardState{Term: e.Term, VotedFor: uint32(e.Command), Commit: e.OldVoters}
			continue
		}
		if e.Index <= s.base {
			continue
		}
		offset := e.Index - s.base
		if offset > uint64(len(s.data)) {
			return nil, fmt.Errorf("raftwal: log index gap at %d", e.Index)
		}
		entry := raft.Entry{Term: e.Term, Command: e.Command, Kind: raft.EntryKind(e.Kind), OldVoters: e.OldVoters, NewVoters: e.NewVoters, Operation: raft.CommandOp(e.Operation), ClientID: e.ClientID, RequestID: e.RequestID, Key: e.Key, Value: e.Value}
		if offset < uint64(len(s.data)) {
			s.data = s.data[:offset]
		}
		s.data = append(s.data, entry)
	}
	return s, nil
}

func (s *Store) SaveHardState(h raft.HardState) error {
	if h.Term < s.hard.Term || h.Commit < s.hard.Commit ||
		(h.Term == s.hard.Term && s.hard.VotedFor != 0 && h.VotedFor != s.hard.VotedFor) {
		return fmt.Errorf("raftwal: hard-state regression")
	}
	if err := s.append(storage.Entry{
		Index: hardStateIndex, Term: h.Term, Command: uint64(h.VotedFor), OldVoters: h.Commit,
	}); err != nil {
		return err
	}
	s.hard = h
	return nil
}

func (s *Store) Append(index uint64, entry raft.Entry) error {
	if index <= s.base || index > s.base+uint64(len(s.data)) {
		return fmt.Errorf("raftwal: invalid log index %d", index)
	}
	if err := s.append(storage.Entry{
		Index: index, Term: entry.Term, Command: entry.Command, Kind: uint8(entry.Kind),
		OldVoters: entry.OldVoters, NewVoters: entry.NewVoters,
		Operation: uint8(entry.Operation), ClientID: entry.ClientID, RequestID: entry.RequestID,
		Key: entry.Key, Value: entry.Value,
	}); err != nil {
		return err
	}
	offset := index - s.base
	if offset < uint64(len(s.data)) {
		s.data = s.data[:offset]
	}
	s.data = append(s.data, entry)
	return nil
}

func (s *Store) State() (raft.HardState, []raft.Entry) {
	return s.hard, append([]raft.Entry(nil), s.data...)
}

func (s *Store) StateWithBase() (raft.HardState, uint64, []raft.Entry) {
	return s.hard, s.base, append([]raft.Entry(nil), s.data...)
}

func (s *Store) Term(index uint64) (uint64, bool) {
	if index < s.base || index > s.base+uint64(len(s.data))-1 {
		return 0, false
	}
	return s.data[index-s.base].Term, true
}

// ResetBase installs a snapshot base even when the local log has no matching
// entry. Its durable fence discards the entire previous logical generation.
func (s *Store) ResetBase(index, term uint64) error {
	if index < s.base {
		return fmt.Errorf("raftwal: reset index regression %d", index)
	}
	if err := s.append(storage.Entry{Index: resetIndex, Term: index, Command: term}); err != nil {
		return err
	}
	s.base = index
	s.data = []raft.Entry{{Term: term}}
	return nil
}

// CompactLog durably appends a compaction fence before releasing the covered
// in-memory prefix. Replay applies the fence only after its CRC-checked record.
func (s *Store) CompactLog(index uint64) error {
	if index <= s.base || index > s.base+uint64(len(s.data))-1 {
		return fmt.Errorf("raftwal: invalid compaction index %d", index)
	}
	offset := index - s.base
	term := s.data[offset].Term
	if err := s.append(storage.Entry{Index: compactIndex, Term: index, Command: term}); err != nil {
		return err
	}
	s.data = append([]raft.Entry(nil), s.data[offset:]...)
	s.base = index
	return nil
}
func (s *Store) append(entry storage.Entry) error {
	ctx := context.Background()
	if err := s.log.Append(ctx, []storage.Entry{entry}); err != nil {
		return err
	}
	return s.log.Sync(ctx)
}
