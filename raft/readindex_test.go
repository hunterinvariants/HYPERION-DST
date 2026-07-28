package raft

import "testing"

func TestReadIndexRequiresCurrentTermCommitAndQuorum(t *testing.T) {
	n := NewNode(1, []uint32{2, 3}, 10)
	n.State, n.Term, n.Leader = Leader, 5, 1
	n.next[2], n.next[3] = 1, 1
	if n.StartReadIndex(10) {
		t.Fatal("read barrier started without current-term commit")
	}
	if !n.Propose(7) {
		t.Fatal("proposal rejected")
	}
	_ = n.Drain(nil)
	n.Step(Message{Type: MsgAppendResponse, From: 2, To: 1, Term: 5, Match: 1})
	_ = n.Drain(nil)
	if n.Commit != 1 {
		t.Fatalf("commit = %d", n.Commit)
	}

	if !n.StartReadIndex(11) {
		t.Fatal("read barrier rejected")
	}
	out := n.Drain(nil)
	if len(out) != 2 || out[0].Context != 11 || out[1].Context != 11 {
		t.Fatalf("barrier messages = %+v", out)
	}
	if _, ok := n.ReadIndex(11); ok {
		t.Fatal("read became ready without quorum")
	}
	n.Step(Message{Type: MsgAppendResponse, From: 2, To: 1, Term: 5, Match: 1, Context: 11})
	if index, ok := n.ReadIndex(11); !ok || index != 1 {
		t.Fatalf("read index = %d, %v", index, ok)
	}
}

func TestFollowerEchoesReadContext(t *testing.T) {
	n := NewNode(2, []uint32{1, 3}, 10)
	n.Term = 2
	n.Step(Message{Type: MsgAppend, From: 1, To: 2, Term: 2, LogIndex: 0, Context: 44})
	out := n.Drain(nil)
	if len(out) != 1 || out[0].Reject || out[0].Context != 44 {
		t.Fatalf("response = %+v", out)
	}
}
