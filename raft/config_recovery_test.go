package raft

import "testing"

func TestRestartRestoresPendingJointElectionQuorum(t *testing.T) {
	joint := Entry{
		Term: 4, Kind: EntryJointConfig,
		OldVoters: voterBits(1, 2, 3),
		NewVoters: voterBits(1, 4, 5),
	}
	n := NewRecoveredNode(
		1, []uint32{2, 3}, 10, &memoryStore{},
		HardState{Term: 4}, []Entry{{}, joint})
	if n.Faulted || n.pendingConfig != 1 || !n.isVoter(4) || !n.isVoter(5) {
		t.Fatalf("pending configuration not restored: %+v", n)
	}
	if len(n.Peers) != 4 {
		t.Fatalf("transport peers not restored: %v", n.Peers)
	}
	n.startPreVote()
	out := n.Drain(nil)
	if len(out) != 4 {
		t.Fatalf("pre-vote did not target joint union: %+v", out)
	}
	n.Step(Message{Type: MsgPreVoteResponse, From: 2, To: 1, Term: 5})
	n.Step(Message{Type: MsgPreVoteResponse, From: 3, To: 1, Term: 5})
	if n.State != Follower {
		t.Fatal("old-only quorum started an election")
	}
	n.Step(Message{Type: MsgPreVoteResponse, From: 4, To: 1, Term: 5})
	n.Step(Message{Type: MsgPreVoteResponse, From: 5, To: 1, Term: 5})
	if n.State != Candidate {
		t.Fatal("joint quorum did not start an election")
	}
}
