package raft

import "testing"

func TestIsolatedNodeDoesNotIncreaseTermWithoutPreVoteQuorum(t *testing.T) {
	node := NewNode(1, []uint32{2, 3, 4, 5}, 3)
	for range 100 {
		node.Tick()
		node.outbound = node.outbound[:0] // partition drops every request
	}
	if node.Term != 0 {
		t.Fatalf("isolated node increased term to %d", node.Term)
	}
	if node.State != Follower {
		t.Fatalf("isolated node state = %v", node.State)
	}
}

func TestPreVoteQuorumStartsOneElection(t *testing.T) {
	node := NewNode(1, []uint32{2, 3, 4, 5}, 1)
	node.Tick()
	node.outbound = node.outbound[:0]
	node.Step(Message{Type: MsgPreVoteResponse, From: 2, To: 1, Term: 1})
	node.Step(Message{Type: MsgPreVoteResponse, From: 3, To: 1, Term: 1})
	if node.Term != 1 || node.State != Candidate {
		t.Fatalf("term/state = %d/%v, want 1/Candidate", node.Term, node.State)
	}
	// A duplicate response must not form an election quorum.
	node.Step(Message{Type: MsgRequestVoteResponse, From: 2, To: 1, Term: 1})
	node.Step(Message{Type: MsgRequestVoteResponse, From: 2, To: 1, Term: 1})
	if node.State == Leader {
		t.Fatal("duplicate vote elected a leader")
	}
	node.Step(Message{Type: MsgRequestVoteResponse, From: 3, To: 1, Term: 1})
	if node.State != Leader {
		t.Fatal("distinct majority did not elect leader")
	}
}

func TestPreVoteRequestNeverAdvancesReceiverTerm(t *testing.T) {
	node := NewNode(2, []uint32{1, 3}, 10)
	node.Term = 4
	node.Step(Message{Type: MsgPreVote, From: 1, To: 2, Term: 99})
	if node.Term != 4 {
		t.Fatalf("pre-vote advanced receiver term to %d", node.Term)
	}
}

func TestHeartbeatStepHasZeroAllocations(t *testing.T) {
	node := NewNode(2, []uint32{1, 3}, 10)
	message := Message{Type: MsgAppend, From: 1, To: 2, Term: 0, LogIndex: 0}
	allocs := testing.AllocsPerRun(10_000, func() {
		node.Step(message)
		node.outbound = node.outbound[:0]
	})
	if allocs != 0 {
		t.Fatalf("heartbeat hot path allocated %f objects; want 0", allocs)
	}
}
