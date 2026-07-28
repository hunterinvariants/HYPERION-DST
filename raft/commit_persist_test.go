package raft

import "testing"

func TestFollowerPersistsCommitBeforeAppendAcknowledgement(t *testing.T) {
	store := &recordingStore{}
	n := NewNodeWithStore(2, []uint32{1, 3}, 10, store)
	n.Term = 3
	n.Log = append(n.Log, Entry{Term: 3, Command: 8})
	n.Step(Message{Type: MsgAppend, From: 1, To: 2, Term: 3,
		LogIndex: 1, LogTerm: 3, Commit: 1})
	if store.hard.Commit != 1 || n.Commit != 1 {
		t.Fatalf("durable=%d volatile=%d", store.hard.Commit, n.Commit)
	}
	if out := n.Drain(nil); len(out) != 1 || out[0].Reject {
		t.Fatalf("response = %+v", out)
	}
}

func TestCommitPersistenceFailureSendsNoAcknowledgement(t *testing.T) {
	store := &recordingStore{fail: true}
	n := NewNodeWithStore(2, []uint32{1, 3}, 10, store)
	n.Term = 3
	n.Log = append(n.Log, Entry{Term: 3, Command: 8})
	n.Step(Message{Type: MsgAppend, From: 1, To: 2, Term: 3,
		LogIndex: 1, LogTerm: 3, Commit: 1})
	if !n.Faulted || n.Commit != 0 || len(n.Drain(nil)) != 0 {
		t.Fatal("commit persistence failure was exposed or acknowledged")
	}
}
