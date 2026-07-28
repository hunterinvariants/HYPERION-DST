package raft

import "testing"

func TestAppendResponseReportsOnlyRequestMatchedPrefix(t *testing.T) {
	follower := NewRecoveredNode(2, []uint32{1, 3}, 10, &memoryStore{},
		HardState{Term: 3}, []Entry{{}, {Term: 1}, {Term: 2, Command: 20}, {Term: 2, Command: 30}})
	follower.Step(Message{Type: MsgAppend, From: 1, To: 2, Term: 3,
		LogIndex: 1, LogTerm: 1, Commit: 3})
	messages := follower.Drain(nil)
	if len(messages) != 1 {
		t.Fatalf("got %d responses, want 1", len(messages))
	}
	response := messages[0]
	if response.Reject {
		t.Fatal("matching prefix was rejected")
	}
	if follower.Commit != 1 {
		t.Fatalf("heartbeat advanced commit=%d beyond matched prefix 1", follower.Commit)
	}
	if response.Match != 1 {
		t.Fatalf("reported match=%d, want request prefix 1", response.Match)
	}
}
