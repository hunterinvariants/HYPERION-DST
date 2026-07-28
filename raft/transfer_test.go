package raft

import "testing"

func TestLeadershipTransferRequiresCaughtUpTarget(t *testing.T) {
	n := NewNode(1, []uint32{2, 3}, 10)
	n.State, n.Term, n.Leader = Leader, 4, 1
	n.next[2], n.match[2] = 1, 0
	if !n.Propose(99) {
		t.Fatal("proposal rejected")
	}
	_ = n.Drain(nil)

	if n.TransferLeadership(2) {
		t.Fatal("transfer accepted before target caught up")
	}
	out := n.Drain(nil)
	if len(out) != 1 || out[0].Type != MsgAppend || !out[0].HasEntry {
		t.Fatalf("catch-up messages = %+v", out)
	}

	n.Step(Message{Type: MsgAppendResponse, From: 2, To: 1, Term: 4, Match: 1})
	_ = n.Drain(nil)
	if !n.TransferLeadership(2) {
		t.Fatal("transfer rejected for caught-up voter")
	}
	out = n.Drain(nil)
	if len(out) != 1 || out[0].Type != MsgTimeoutNow || out[0].To != 2 {
		t.Fatalf("transfer message = %+v", out)
	}
}

func TestTimeoutNowStartsDurableElection(t *testing.T) {
	store := &recordingStore{}
	n := NewNodeWithStore(2, []uint32{1, 3}, 10, store)
	n.Term, n.Leader = 7, 1
	n.Step(Message{Type: MsgTimeoutNow, From: 1, To: 2, Term: 7})
	if n.State != Candidate || n.Term != 8 || n.VotedFor != 2 {
		t.Fatalf("node state = role %v term %d vote %d", n.State, n.Term, n.VotedFor)
	}
	if store.hard != (HardState{Term: 8, VotedFor: 2}) {
		t.Fatalf("hard state = %+v", store.hard)
	}
}

func TestTimeoutNowRejectsNonLeader(t *testing.T) {
	n := NewNode(2, []uint32{1, 3}, 10)
	n.Term, n.Leader = 7, 1
	n.Step(Message{Type: MsgTimeoutNow, From: 3, To: 2, Term: 7})
	if n.State != Follower || n.Term != 7 {
		t.Fatal("non-leader forced election")
	}
}
