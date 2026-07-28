package raft

import "testing"

func TestApplyDoesNotExposeConfigurationEntriesToStateMachine(t *testing.T) {
	n := NewNode(1, []uint32{2, 3}, 10)
	n.Term = 2
	n.Log = append(n.Log,
		Entry{Term: 2, Command: 10},
		Entry{Term: 2, Kind: EntryJointConfig,
			OldVoters: voterBits(1, 2, 3), NewVoters: voterBits(1, 3, 4)},
		Entry{Term: 2, Kind: EntryFinalConfig, OldVoters: voterBits(1, 3, 4)},
		Entry{Term: 2, Command: 20},
	)
	n.Commit = 4
	got := n.Apply()
	if len(got) != 2 || got[0] != 10 || got[1] != 20 || n.Applied != 4 {
		t.Fatalf("applied commands=%v applied-index=%d", got, n.Applied)
	}
}
