// Package raftstore composes the WAL and atomic snapshot generations into the
// complete persistence boundary required by compacting Raft nodes.
package raftstore

import (
	"fmt"

	"github.com/hunterinvariants/promtact/raft"
	"github.com/hunterinvariants/promtact/storage/raftwal"
	"github.com/hunterinvariants/promtact/storage/snapshot"
	"github.com/hunterinvariants/promtact/storage/wal"
)

type Recovery struct {
	Hard     raft.HardState
	Snapshot raft.Snapshot
	Suffix   []raft.Entry
}

type Store struct {
	wal      *raftwal.Store
	snapshot *snapshot.Store
	pending  raft.Snapshot
}

func Open(device wal.Device, snapshotPath string) (*Store, Recovery, error) {
	snapStore, image, err := snapshot.OpenStore(snapshotPath)
	if err != nil {
		return nil, Recovery{}, err
	}
	walStore, err := raftwal.Open(device)
	if err != nil {
		return nil, Recovery{}, err
	}
	hard, base, entries := walStore.StateWithBase()
	if base > image.LastIndex {
		return nil, Recovery{}, fmt.Errorf(
			"raftstore: WAL base %d exceeds snapshot %d", base, image.LastIndex)
	}
	// A crash after the snapshot rename but before the WAL compaction fence is
	// completed here. The durable snapshot makes discarding its prefix safe.
	if image.LastIndex > base {
		if image.LastIndex > base+uint64(len(entries))-1 {
			return nil, Recovery{}, fmt.Errorf(
				"raftstore: snapshot %d exceeds WAL last index", image.LastIndex)
		}
		if term, present := walStore.Term(image.LastIndex); present && term == image.LastTerm {
			if err := walStore.CompactLog(image.LastIndex); err != nil {
				return nil, Recovery{}, err
			}
		} else if err := walStore.ResetBase(image.LastIndex, image.LastTerm); err != nil {
			return nil, Recovery{}, err
		}
		hard, base, entries = walStore.StateWithBase()
	}
	if base != image.LastIndex {
		return nil, Recovery{}, fmt.Errorf(
			"raftstore: snapshot/WAL base mismatch %d != %d", image.LastIndex, base)
	}
	recovery := Recovery{
		Hard: hard,
		Snapshot: raft.Snapshot{
			LastIndex: image.LastIndex,
			LastTerm:  image.LastTerm,
			State:     append([]byte(nil), image.State...),
			OldVoters: image.OldVoters,
			NewVoters: image.NewVoters,
		},
	}
	if len(entries) > 1 {
		recovery.Suffix = append([]raft.Entry(nil), entries[1:]...)
	}
	return &Store{wal: walStore, snapshot: snapStore}, recovery, nil
}

func (s *Store) SaveHardState(h raft.HardState) error {
	return s.wal.SaveHardState(h)
}

func (s *Store) Append(index uint64, entry raft.Entry) error {
	return s.wal.Append(index, entry)
}

func (s *Store) SaveSnapshot(snap raft.Snapshot) error {
	if err := s.snapshot.Save(snapshot.Image{
		LastIndex: snap.LastIndex,
		LastTerm:  snap.LastTerm,
		State:     snap.State,
		OldVoters: snap.OldVoters,
		NewVoters: snap.NewVoters,
	}); err != nil {
		return err
	}
	s.pending = snap
	return nil
}

func (s *Store) CompactLog(index uint64) error {
	if s.pending.LastIndex != index {
		return fmt.Errorf("raftstore: no durable snapshot for compaction index %d", index)
	}
	var err error
	if term, present := s.wal.Term(index); present && term == s.pending.LastTerm {
		err = s.wal.CompactLog(index)
	} else {
		err = s.wal.ResetBase(index, s.pending.LastTerm)
	}
	if err == nil {
		s.pending = raft.Snapshot{}
	}
	return err
}
