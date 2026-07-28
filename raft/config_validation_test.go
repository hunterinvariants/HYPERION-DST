package raft

import "testing"

func TestFollowerRejectsMalformedConfigurationBeforePersistence(t *testing.T) {
	store := &recordingStore{}
	n := NewNodeWithStore(2, []uint32{1, 3}, 10, store)
	n.Term = 3
	n.Step(Message{
		Type: MsgAppend, From: 1, To: 2, Term: 3, LogIndex: 0, LogTerm: 0,
		HasEntry: true,
		Entry: Entry{Term: 3, Kind: EntryJointConfig,
			OldVoters: 0, NewVoters: voterBits(1, 2, 3)},
	})
	if len(store.entries) != 0 || len(n.Log) != 1 {
		t.Fatal("malformed configuration reached stable storage")
	}
	out := n.Drain(nil)
	if len(out) != 1 || !out[0].Reject {
		t.Fatalf("response = %+v", out)
	}
}

func TestRecoveryFailStopsOnMalformedConfiguration(t *testing.T) {
	n := NewRecoveredNode(
		1, []uint32{2, 3}, 10, &memoryStore{},
		HardState{Term: 4, Commit: 1},
		[]Entry{{}, {
			Term: 4, Kind: EntryJointConfig,
			OldVoters: voterBits(1, 2), NewVoters: voterBits(1, 3, 4),
		}},
	)
	if !n.Faulted {
		t.Fatal("malformed committed configuration survived recovery")
	}
}

func TestRecoveryRejectsUnknownEntryKind(t *testing.T) {
	n := NewRecoveredNode(
		1, []uint32{2, 3}, 10, &memoryStore{},
		HardState{Term: 1}, []Entry{{}, {Term: 1, Kind: EntryKind(99)}})
	if !n.Faulted {
		t.Fatal("unknown entry kind survived recovery")
	}
}
