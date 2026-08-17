package raftstore

import (
	"path/filepath"
	"testing"

	"github.com/hunterinvariants/promtact/raft"
	"github.com/hunterinvariants/promtact/storage/wal"
)

func TestSnapshotBeforeFenceCrashCompletesCompactionOnOpen(t *testing.T) {
	device := wal.NewMemoryDevice(nil)
	path := filepath.Join(t.TempDir(), "node.snap")
	store, _, err := Open(device, path)
	if err != nil {
		t.Fatal(err)
	}
	for index := uint64(1); index <= 4; index++ {
		if err := store.Append(index, raft.Entry{Term: 2, Command: index}); err != nil {
			t.Fatal(err)
		}
	}
	snap := raft.Snapshot{
		LastIndex: 3, LastTerm: 2, State: []byte("state"),
		OldVoters: 0b111,
	}
	// Simulate power loss in the ordered gap: the replacement snapshot is
	// durable but the WAL compaction fence has not been appended.
	if err := store.SaveSnapshot(snap); err != nil {
		t.Fatal(err)
	}
	reopened, recovery, err := Open(
		wal.NewMemoryDevice(device.DurableBytes()), path)
	if err != nil {
		t.Fatal(err)
	}
	if recovery.Snapshot.LastIndex != 3 || len(recovery.Suffix) != 1 ||
		recovery.Suffix[0].Command != 4 {
		t.Fatalf("recovery = %+v", recovery)
	}
	if err := reopened.Append(5, raft.Entry{Term: 3, Command: 5}); err != nil {
		t.Fatalf("append after completed recovery: %v", err)
	}
}

func TestRecoveredNodeUsesSnapshotConfigurationAndAbsoluteSuffix(t *testing.T) {
	device := wal.NewMemoryDevice(nil)
	path := filepath.Join(t.TempDir(), "node.snap")
	store, _, err := Open(device, path)
	if err != nil {
		t.Fatal(err)
	}
	for index := uint64(1); index <= 3; index++ {
		if err := store.Append(index, raft.Entry{Term: 4, Command: index}); err != nil {
			t.Fatal(err)
		}
	}
	snap := raft.Snapshot{LastIndex: 2, LastTerm: 4, State: []byte("s"),
		OldVoters: 0b1101}
	if err := store.SaveSnapshot(snap); err != nil {
		t.Fatal(err)
	}
	if err := store.CompactLog(2); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveHardState(raft.HardState{Term: 4, Commit: 3}); err != nil {
		t.Fatal(err)
	}
	reopened, recovery, err := Open(
		wal.NewMemoryDevice(device.DurableBytes()), path)
	if err != nil {
		t.Fatal(err)
	}
	node := raft.NewRecoveredNodeWithSnapshot(
		1, []uint32{3, 4}, 10, reopened, recovery.Hard,
		recovery.Snapshot, recovery.Suffix)
	if node.Faulted || node.BaseIndex != 2 || node.Commit != 3 ||
		node.Log[len(node.Log)-1].Command != 3 {
		t.Fatalf("node recovery = %+v", node)
	}
}
