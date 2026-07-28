package raft

import "testing"

func TestLeaderStepsDownWhenFinalConfigurationRemovesIt(t *testing.T) {
	n := NewNode(1, []uint32{2, 3}, 10)
	n.State, n.Term, n.Leader = Leader, 9, 1
	n.next[2], n.next[3] = 1, 1
	if !n.ProposeJoint([]uint32{2, 3, 4}) {
		t.Fatal("joint proposal removing leader rejected")
	}
	joint := n.lastIndex()
	_ = n.Drain(nil)
	n.Step(Message{Type: MsgAppendResponse, From: 2, To: 1, Term: 9, Match: joint})
	n.Step(Message{Type: MsgAppendResponse, From: 3, To: 1, Term: 9, Match: joint})
	if n.Commit != joint || n.State != Leader {
		t.Fatalf("joint phase commit=%d state=%v", n.Commit, n.State)
	}
	_ = n.Drain(nil)
	if !n.ProposeFinal() {
		t.Fatal("final proposal rejected")
	}
	final := n.lastIndex()
	_ = n.Drain(nil)
	n.Step(Message{Type: MsgAppendResponse, From: 2, To: 1, Term: 9, Match: final})
	n.Step(Message{Type: MsgAppendResponse, From: 3, To: 1, Term: 9, Match: final})
	if n.Commit != final || n.State != Follower || n.Leader != 0 || n.isVoter(1) {
		t.Fatalf("removed leader commit=%d state=%v leader=%d voters=%x",
			n.Commit, n.State, n.Leader, n.votersOld)
	}
	if out := n.Drain(nil); len(out) != 0 {
		t.Fatalf("removed leader sent messages: %+v", out)
	}
}
