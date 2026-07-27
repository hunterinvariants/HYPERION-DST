package raft

import "errors"

var ErrStorage = errors.New("raft: stable storage failure")

type HardState struct {
	Term     uint64
	VotedFor uint32
}

// StableStore is Raft's safety-critical persistence boundary.
type StableStore interface {
	SaveHardState(HardState) error
	Append(uint64, Entry) error
}

type memoryStore struct {
	hard HardState
	log  []Entry
}

func (s *memoryStore) SaveHardState(h HardState) error {
	s.hard = h
	return nil
}

func (s *memoryStore) Append(_ uint64, e Entry) error {
	s.log = append(s.log, e)
	return nil
}
