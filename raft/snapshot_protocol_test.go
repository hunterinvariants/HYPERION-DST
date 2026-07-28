package raft

import "testing"

type snapshotRecordingStore struct {
	recordingStore
	snapshot     Snapshot
	compacted    uint64
	failSnapshot bool
	failCompact  bool
}

func (s *snapshotRecordingStore) SaveSnapshot(snapshot Snapshot) error {
	s.events = append(s.events, "snapshot")
	if s.failSnapshot {
		return ErrStorage
	}
	s.snapshot = snapshot
	return nil
}

func (s *snapshotRecordingStore) CompactLog(index uint64) error {
	s.events = append(s.events, "compact")
	if s.failCompact {
		return ErrStorage
	}
	s.compacted = index
	return nil
}

func TestCompactedBaseUsesAbsoluteIndexes(t *testing.T) {
	store := &snapshotRecordingStore{}
	n := NewNodeWithStore(1, []uint32{2, 3}, 10, store)
	n.State, n.Term, n.Leader = Leader, 3, 1
	n.Log = []Entry{{}, {Term: 2, Command: 10}, {Term: 3, Command: 20}, {Term: 3, Command: 30}}
	n.Commit, n.Applied = 3, 3
	if !n.Compact(2, []byte("state-20")) {
		t.Fatal("compaction rejected")
	}
	if n.BaseIndex != 2 || n.BaseTerm != 3 || n.lastIndex() != 3 {
		t.Fatalf("base=(%d,%d) last=%d", n.BaseIndex, n.BaseTerm, n.lastIndex())
	}
	if len(n.Log) != 2 || n.entryAt(3).Command != 30 {
		t.Fatalf("retained log = %+v", n.Log)
	}
	if len(store.events) != 2 || store.events[0] != "snapshot" || store.events[1] != "compact" {
		t.Fatalf("durability order = %v", store.events)
	}
	entry := Entry{Term: 3, Command: 40}
	if err := store.Append(4, entry); err != nil {
		t.Fatal(err)
	}
	n.Log = append(n.Log, entry)
	if n.lastIndex() != 4 || n.entryAt(4) != entry {
		t.Fatal("absolute append failed after compaction")
	}
}

func TestLaggingFollowerInstallsSnapshotThenCatchesUp(t *testing.T) {
	leader := NewNode(1, []uint32{2, 3}, 10)
	leader.State, leader.Term, leader.Leader = Leader, 4, 1
	leader.Log = []Entry{{}, {Term: 2, Command: 10}, {Term: 3, Command: 20}, {Term: 4, Command: 30}}
	leader.Commit, leader.Applied = 3, 3
	if !leader.Compact(2, []byte("snapshot")) {
		t.Fatal("leader compaction rejected")
	}
	leader.next[2] = 1
	leader.appendTo(2)
	out := leader.Drain(nil)
	if len(out) != 1 || out[0].Type != MsgInstallSnapshot {
		t.Fatalf("snapshot send = %+v", out)
	}

	follower := NewNode(2, []uint32{1, 3}, 10)
	follower.Step(out[0])
	response := follower.Drain(nil)
	if len(response) != 1 || response[0].Type != MsgInstallSnapshotResponse || response[0].Reject {
		t.Fatalf("snapshot response = %+v", response)
	}
	if follower.BaseIndex != 2 || follower.Applied != 2 || string(follower.snapshot.State) != "snapshot" {
		t.Fatalf("follower snapshot state = base %d applied %d image %+v",
			follower.BaseIndex, follower.Applied, follower.snapshot)
	}

	leader.Step(response[0])
	appendMessage := leader.Drain(nil)
	if len(appendMessage) != 1 || appendMessage[0].Type != MsgAppend ||
		appendMessage[0].LogIndex != 2 || !appendMessage[0].HasEntry {
		t.Fatalf("post-snapshot catch-up = %+v", appendMessage)
	}
	follower.Step(appendMessage[0])
	if follower.lastIndex() != 3 || follower.entryAt(3).Command != 30 {
		t.Fatalf("follower log after catch-up = %+v", follower.Log)
	}
}

func TestSnapshotFailureSendsNoAcknowledgement(t *testing.T) {
	store := &snapshotRecordingStore{failSnapshot: true}
	n := NewNodeWithStore(2, []uint32{1, 3}, 10, store)
	n.Step(Message{Type: MsgInstallSnapshot, From: 1, To: 2, Term: 2,
		SnapshotIndex: 10, SnapshotTerm: 2, Snapshot: []byte("state"), SnapshotOld: voterBits(1, 2, 3)})
	if !n.Faulted || len(n.Drain(nil)) != 0 || n.BaseIndex != 0 {
		t.Fatal("failed snapshot was acknowledged or exposed")
	}
}
