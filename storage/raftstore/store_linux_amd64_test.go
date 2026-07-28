//go:build linux && amd64

package raftstore

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hunterinvariants/HYPERION-DST/raft"
	"github.com/hunterinvariants/HYPERION-DST/storage/uringwal"
)

func TestIOUringSnapshotCompactionRecovery(t *testing.T) {
	if os.Getenv("HYPERION_URING_INTEGRATION") != "1" {
		t.Skip("set HYPERION_URING_INTEGRATION=1 on a supported Linux host")
	}
	dir := t.TempDir()
	walPath := filepath.Join(dir, "node.wal")
	snapshotPath := filepath.Join(dir, "node.snap")
	device, err := uringwal.Open(walPath, 8)
	if err != nil {
		t.Fatal(err)
	}
	store, _, err := Open(device, snapshotPath)
	if err != nil {
		_ = device.Close()
		t.Fatal(err)
	}
	node := raft.NewNodeWithStore(1, nil, 10, store)
	node.State, node.Term, node.Leader = raft.Leader, 3, 1
	for command := uint64(1); command <= 3; command++ {
		if !node.Propose(command) {
			t.Fatalf("proposal %d failed: %v", command, node.StorageError())
		}
	}
	node.Commit, node.Applied = 3, 3
	if !node.Compact(2, []byte("state-at-two")) {
		t.Fatalf("compaction failed: %v", node.StorageError())
	}
	if err := store.SaveHardState(raft.HardState{Term: 3, Commit: 3}); err != nil {
		t.Fatal(err)
	}
	if err := device.Close(); err != nil {
		t.Fatal(err)
	}

	reopenedDevice, err := uringwal.Open(walPath, 8)
	if err != nil {
		t.Fatal(err)
	}
	defer reopenedDevice.Close()
	reopened, recovery, err := Open(reopenedDevice, snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	recovered := raft.NewRecoveredNodeWithSnapshot(
		1, nil, 10, reopened, recovery.Hard, recovery.Snapshot, recovery.Suffix)
	if recovered.Faulted || recovered.BaseIndex != 2 || recovered.Commit != 3 ||
		len(recovered.Log) != 2 || recovered.Log[1].Command != 3 {
		t.Fatalf("recovered node = %+v", recovered)
	}
}
