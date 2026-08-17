package raftwal

import (
	"testing"

	"github.com/hunterinvariants/promtact/raft"
	"github.com/hunterinvariants/promtact/storage/wal"
)

func TestJointConfigurationSurvivesReplay(t *testing.T) {
	device := wal.NewMemoryDevice(nil)
	store, err := Open(device)
	if err != nil {
		t.Fatal(err)
	}
	entry := raft.Entry{Term: 9, Kind: raft.EntryKind(1),
		OldVoters: 0b111, NewVoters: 0b1101}
	if err := store.Append(1, entry); err != nil {
		t.Fatal(err)
	}
	recovered, err := Open(wal.NewMemoryDevice(device.DurableBytes()))
	if err != nil {
		t.Fatal(err)
	}
	_, entries := recovered.State()
	if len(entries) != 2 || entries[1] != entry {
		t.Fatalf("configuration entry after replay = %+v", entries)
	}
}
