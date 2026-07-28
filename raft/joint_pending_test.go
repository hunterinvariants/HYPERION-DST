package raft

import "testing"

func TestNormalEntryCannotBypassPendingJointQuorum(t *testing.T) {
	n := NewNode(1, []uint32{2, 3}, 10)
	n.State, n.Term, n.Leader = Leader, 7, 1
	n.next[2], n.next[3] = 1, 1
	if !n.ProposeJoint([]uint32{1, 3, 4}) || !n.Propose(99) {
		t.Fatal("proposal rejected")
	}
	last := n.lastIndex()
	_ = n.Drain(nil)
	n.Step(Message{Type: MsgAppendResponse, From: 2, To: 1, Term: 7, Match: last})
	if n.Commit != 0 {
		t.Fatalf("normal suffix bypassed pending joint quorum: commit=%d", n.Commit)
	}
	n.Step(Message{Type: MsgAppendResponse, From: 3, To: 1, Term: 7, Match: last})
	if n.Commit != last {
		t.Fatalf("joint quorum did not commit suffix: commit=%d last=%d", n.Commit, last)
	}
}
