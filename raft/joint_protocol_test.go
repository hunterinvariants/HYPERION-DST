package raft

import "testing"

func TestJointAndFinalConfigurationExecuteInTwoCommittedPhases(t *testing.T) {
	n := NewNode(1, []uint32{2, 3}, 10)
	n.State, n.Term, n.Leader = Leader, 6, 1
	n.next[2], n.next[3] = 1, 1

	if !n.ProposeJoint([]uint32{1, 3, 4}) {
		t.Fatal("joint proposal rejected")
	}
	jointIndex := n.lastIndex()
	if jointIndex != 1 || n.entryAt(jointIndex).Kind != EntryJointConfig {
		t.Fatalf("joint entry = %+v", n.entryAt(jointIndex))
	}
	_ = n.Drain(nil)

	// Old quorum {1,2} is insufficient: the joint entry also requires a
	// majority of new {1,3,4}.
	n.Step(Message{Type: MsgAppendResponse, From: 2, To: 1, Term: 6, Match: jointIndex})
	if n.Commit != 0 {
		t.Fatalf("joint entry committed with old quorum only: %d", n.Commit)
	}
	n.Step(Message{Type: MsgAppendResponse, From: 3, To: 1, Term: 6, Match: jointIndex})
	if n.Commit != jointIndex || n.votersOld != voterBits(1, 2, 3) ||
		n.votersNew != voterBits(1, 3, 4) {
		t.Fatalf("joint state commit=%d old=%x new=%x", n.Commit, n.votersOld, n.votersNew)
	}
	_ = n.Drain(nil)

	if !n.ProposeFinal() {
		t.Fatal("final proposal rejected")
	}
	finalIndex := n.lastIndex()
	_ = n.Drain(nil)

	// The final entry is still committed under the joint configuration.
	n.Step(Message{Type: MsgAppendResponse, From: 2, To: 1, Term: 6, Match: finalIndex})
	if n.Commit != jointIndex {
		t.Fatalf("final entry committed without new quorum: %d", n.Commit)
	}
	n.Step(Message{Type: MsgAppendResponse, From: 3, To: 1, Term: 6, Match: finalIndex})
	if n.Commit != finalIndex || n.votersOld != voterBits(1, 3, 4) || n.votersNew != 0 {
		t.Fatalf("final state commit=%d old=%x new=%x", n.Commit, n.votersOld, n.votersNew)
	}
	if n.isVoter(2) {
		t.Fatal("removed voter remained active after final configuration")
	}
}

func TestConfigurationEntrySurvivesWALRepresentation(t *testing.T) {
	entry := Entry{Term: 8, Kind: EntryJointConfig,
		OldVoters: voterBits(1, 2, 3), NewVoters: voterBits(1, 3, 4)}
	store := &recordingStore{}
	n := NewNodeWithStore(1, []uint32{2, 3}, 10, store)
	n.State, n.Term, n.Leader = Leader, 8, 1
	n.ensurePeers(entry.NewVoters)
	if !n.appendLocal(entry) {
		t.Fatal("configuration append failed")
	}
	if len(store.entries) != 1 || store.entries[0] != entry {
		t.Fatalf("durable entry = %+v", store.entries)
	}
}

func voterBits(ids ...uint32) uint64 {
	var mask uint64
	for _, id := range ids {
		mask |= nodeBit(id)
	}
	return mask
}
