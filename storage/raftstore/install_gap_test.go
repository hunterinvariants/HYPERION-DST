package raftstore

import (
	"path/filepath"
	"testing"

	"github.com/hunterinvariants/promtact/raft"
	"github.com/hunterinvariants/promtact/storage/wal"
)

func TestInstallSnapshotResetsDivergentShortWAL(t *testing.T) {
	device := wal.NewMemoryDevice(nil)
	path := filepath.Join(t.TempDir(), "node.snap")
	store, _, err := Open(device, path)
	if err != nil {
		t.Fatal(err)
	}
	node := raft.NewNodeWithStore(2, []uint32{1, 3}, 10, store)
	node.Step(raft.Message{
		Type:          raft.MsgInstallSnapshot,
		From:          1,
		To:            2,
		Term:          5,
		SnapshotIndex: 100,
		SnapshotTerm:  4,
		Snapshot:      []byte("state-100"),
		SnapshotOld:   0b111,
	})
	if node.Faulted || node.BaseIndex != 100 {
		t.Fatalf("snapshot install failed: %+v", node)
	}
	if out := node.Drain(nil); len(out) != 1 || out[0].Reject {
		t.Fatalf("snapshot acknowledgement = %+v", out)
	}

	reopened, recovery, err := Open(
		wal.NewMemoryDevice(device.DurableBytes()), path)
	if err != nil {
		t.Fatal(err)
	}
	recovered := raft.NewRecoveredNodeWithSnapshot(
		2, []uint32{1, 3}, 10, reopened, recovery.Hard,
		recovery.Snapshot, recovery.Suffix)
	if recovered.Faulted || recovered.BaseIndex != 100 ||
		recovered.Commit != 100 || recovered.Applied != 100 {
		t.Fatalf("recovered snapshot = %+v", recovered)
	}
}
