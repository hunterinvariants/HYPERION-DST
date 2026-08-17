package raftwal

import (
	"testing"

	"github.com/hunterinvariants/promtact/raft"
	"github.com/hunterinvariants/promtact/storage/wal"
)

func TestRebootRestoresHardStateAndLog(t *testing.T) {
	device := wal.NewMemoryDevice(nil)
	store, err := Open(device)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveHardState(raft.HardState{Term: 7, VotedFor: 3}); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(1, raft.Entry{Term: 7, Command: 10}); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(2, raft.Entry{Term: 7, Command: 20}); err != nil {
		t.Fatal(err)
	}
	rebooted, err := device.Crash(0)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := Open(rebooted)
	if err != nil {
		t.Fatal(err)
	}
	hard, entries := recovered.State()
	if hard != (raft.HardState{Term: 7, VotedFor: 3}) {
		t.Fatalf("hard state = %+v", hard)
	}
	if len(entries) != 3 || entries[2].Command != 20 {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestConflictReplacementSurvivesReplay(t *testing.T) {
	device := wal.NewMemoryDevice(nil)
	store, err := Open(device)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Append(1, raft.Entry{Term: 1, Command: 10}); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(2, raft.Entry{Term: 1, Command: 20}); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(2, raft.Entry{Term: 2, Command: 99}); err != nil {
		t.Fatal(err)
	}
	recovered, err := Open(wal.NewMemoryDevice(device.DurableBytes()))
	if err != nil {
		t.Fatal(err)
	}
	_, entries := recovered.State()
	if len(entries) != 3 || entries[2] != (raft.Entry{Term: 2, Command: 99}) {
		t.Fatalf("conflicting suffix was not replaced: %+v", entries)
	}
}
