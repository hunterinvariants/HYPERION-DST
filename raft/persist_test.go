package raft

import (
	"testing"
)

type recordingStore struct {
	hard    HardState
	entries []Entry
	fail    bool
	events  []string
}

func (s *recordingStore) SaveHardState(h HardState) error {
	s.events = append(s.events, "hard")
	if s.fail {
		return ErrStorage
	}
	s.hard = h
	return nil
}

func (s *recordingStore) Append(_ uint64, e Entry) error {
	s.events = append(s.events, "entry")
	if s.fail {
		return ErrStorage
	}
	s.entries = append(s.entries, e)
	return nil
}

func TestVoteIsDurableBeforeResponse(t *testing.T) {
	store := &recordingStore{}
	n := NewNodeWithStore(2, []uint32{1, 3}, 10, store)
	n.Step(Message{Type: MsgRequestVote, From: 1, To: 2, Term: 4})
	if store.hard != (HardState{Term: 4, VotedFor: 1}) {
		t.Fatalf("hard state = %+v", store.hard)
	}
	var out []Message
	out = n.Drain(out)
	if len(out) != 1 || out[0].Reject {
		t.Fatalf("vote response = %+v", out)
	}
}

func TestStorageFailureSendsNoVote(t *testing.T) {
	store := &recordingStore{fail: true}
	n := NewNodeWithStore(2, []uint32{1, 3}, 10, store)
	n.Step(Message{Type: MsgRequestVote, From: 1, To: 2, Term: 4})
	if !n.Faulted {
		t.Fatal("node did not fail-stop")
	}
	if out := n.Drain(nil); len(out) != 0 {
		t.Fatalf("unsafe response after persistence failure: %+v", out)
	}
}

func TestFollowerPersistsEntryBeforeAck(t *testing.T) {
	store := &recordingStore{}
	n := NewNodeWithStore(2, []uint32{1, 3}, 10, store)
	n.Step(Message{Type: MsgAppend, From: 1, To: 2, Term: 1, LogIndex: 0,
		LogTerm: 0, HasEntry: true, Entry: Entry{Term: 1, Command: 99}})
	if len(store.entries) != 1 || len(n.Log) != 2 {
		t.Fatalf("entry was not persisted and appended")
	}
	if out := n.Drain(nil); len(out) != 1 || out[0].Reject {
		t.Fatalf("append response = %+v", out)
	}
}

func TestEntryStorageFailureSendsNoAck(t *testing.T) {
	store := &recordingStore{fail: true}
	n := NewNodeWithStore(2, []uint32{1, 3}, 10, store)
	n.Step(Message{Type: MsgAppend, From: 1, To: 2, Term: 1, LogIndex: 0,
		LogTerm: 0, HasEntry: true, Entry: Entry{Term: 1, Command: 99}})
	if !n.Faulted || len(n.Log) != 1 || len(n.Drain(nil)) != 0 {
		t.Fatal("node acknowledged or exposed an entry after storage failure")
	}
}
