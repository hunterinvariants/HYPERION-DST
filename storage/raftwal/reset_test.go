package raftwal

import (
	"testing"

	"github.com/hunterinvariants/HYPERION-DST/raft"
	"github.com/hunterinvariants/HYPERION-DST/storage/wal"
)

func TestResetBaseAcrossLogGapSurvivesReplay(t *testing.T) {
	device := wal.NewMemoryDevice(nil)
	store, err := Open(device)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Append(1, raft.Entry{Term: 1, Command: 1}); err != nil {
		t.Fatal(err)
	}
	if err := store.ResetBase(100, 7); err != nil {
		t.Fatal(err)
	}
	recovered, err := Open(wal.NewMemoryDevice(device.DurableBytes()))
	if err != nil {
		t.Fatal(err)
	}
	_, base, entries := recovered.StateWithBase()
	if base != 100 || len(entries) != 1 || entries[0].Term != 7 {
		t.Fatalf("base=%d entries=%+v", base, entries)
	}
	if err := recovered.Append(101, raft.Entry{Term: 8, Command: 2}); err != nil {
		t.Fatal(err)
	}
}
