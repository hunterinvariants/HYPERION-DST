package raft

import "errors"

var ErrStorage = errors.New("raft: stable storage failure")

type HardState struct {
	Term     uint64
	VotedFor uint32
	Commit   uint64
}

// StableStore is Raft's safety-critical persistence boundary.
type StableStore interface {
	SaveHardState(HardState) error
	Append(uint64, Entry) error
}

type Snapshot struct {
	LastIndex uint64
	LastTerm  uint64
	State     []byte
	OldVoters uint64
	NewVoters uint64
}

// SnapshotStore is the ordered durability extension used for compaction.
// SaveSnapshot must make the replacement durable before CompactLog returns.
type SnapshotStore interface {
	SaveSnapshot(Snapshot) error
	CompactLog(uint64) error
}
type memoryStore struct {
	hard     HardState
	log      []Entry
	snapshot Snapshot
	base     uint64
}

func (s *memoryStore) SaveHardState(h HardState) error {
	s.hard = h
	return nil
}

func (s *memoryStore) Append(_ uint64, e Entry) error {
	s.log = append(s.log, e)
	return nil
}

func (s *memoryStore) SaveSnapshot(snapshot Snapshot) error {
	snapshot.State = append([]byte(nil), snapshot.State...)
	s.snapshot = snapshot
	return nil
}

func (s *memoryStore) CompactLog(index uint64) error {
	if index < s.base {
		return ErrStorage
	}
	s.base = index
	return nil
}
