package raft

import "testing"

func TestSingleVoterClientProposalCommitsDurably(t *testing.T) {
	store := &recordingStore{}
	node := NewNodeWithStore(1, nil, 1, store)
	node.Tick()
	if node.State != Leader {
		t.Fatalf("state = %v", node.State)
	}
	if !node.ProposeRequest(CommandPut, 10, 1, 20, 30) {
		t.Fatal("proposal rejected")
	}
	if node.Commit != 2 || store.hard.Commit != 2 {
		t.Fatalf("commit node=%d durable=%d", node.Commit, store.hard.Commit)
	}
	entries := node.ApplyEntries(nil)
	if len(entries) != 1 || entries[0].ClientID != 10 || entries[0].Value != 30 {
		t.Fatalf("applied = %+v", entries)
	}
}
