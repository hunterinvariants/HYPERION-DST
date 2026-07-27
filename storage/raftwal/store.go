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
const hardStateIndex uint64 = 0

type Store struct {
	log  *wal.Log
	hard raft.HardState
	data []raft.Entry // includes the Raft sentinel at index zero
}

func Open(device wal.Device) (*Store, error) {
	log, records, err := wal.Open(device)
	if err != nil {
		return nil, err
	}
	s := &Store{log: log, data: []raft.Entry{{}}}
	for _, record := range records {
		e := record.Entry
		if e.Index == hardStateIndex {
			if e.Term < s.hard.Term {
				return nil, fmt.Errorf("raftwal: hard-state term regression")
			}
			s.hard = raft.HardState{Term: e.Term, VotedFor: uint32(e.Command)}
			continue
		}
		if e.Index > uint64(len(s.data)) {
			return nil, fmt.Errorf("raftwal: log index gap at %d", e.Index)
		}
		entry := raft.Entry{Term: e.Term, Command: e.Command}
		if e.Index < uint64(len(s.data)) {
			s.data = s.data[:e.Index]
		}
		s.data = append(s.data, entry)
	}
	return s, nil
}

func (s *Store) SaveHardState(h raft.HardState) error {
	if h.Term < s.hard.Term {
		return fmt.Errorf("raftwal: hard-state term regression")
	}
	if err := s.append(storage.Entry{
		Index: hardStateIndex, Term: h.Term, Command: uint64(h.VotedFor),
	}); err != nil {
		return err
	}
	s.hard = h
	return nil
}

func (s *Store) Append(index uint64, entry raft.Entry) error {
	if index == 0 || index > uint64(len(s.data)) {
		return fmt.Errorf("raftwal: invalid log index %d", index)
	}
	if err := s.append(storage.Entry{
		Index: index, Term: entry.Term, Command: entry.Command,
	}); err != nil {
		return err
	}
	if index < uint64(len(s.data)) {
		s.data = s.data[:index]
	}
	s.data = append(s.data, entry)
	return nil
}

func (s *Store) State() (raft.HardState, []raft.Entry) {
	return s.hard, append([]raft.Entry(nil), s.data...)
}

func (s *Store) append(entry storage.Entry) error {
	ctx := context.Background()
	if err := s.log.Append(ctx, []storage.Entry{entry}); err != nil {
		return err
	}
	return s.log.Sync(ctx)
}
