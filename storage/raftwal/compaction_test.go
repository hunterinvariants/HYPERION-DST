package raftwal

import (
	"testing"

	"github.com/hunterinvariants/promtact/raft"
	"github.com/hunterinvariants/promtact/storage/wal"
)

func TestCompactionFenceSurvivesReplay(t *testing.T) {
	device := wal.NewMemoryDevice(nil)
	store, err := Open(device)
	if err != nil {
		t.Fatal(err)
	}
	for index := uint64(1); index <= 4; index++ {
		if err := store.Append(index, raft.Entry{Term: 2, Command: index * 10}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.CompactLog(3); err != nil {
		t.Fatal(err)
	}
	recovered, err := Open(wal.NewMemoryDevice(device.DurableBytes()))
	if err != nil {
		t.Fatal(err)
	}
	_, base, entries := recovered.StateWithBase()
	if base != 3 || len(entries) != 2 || entries[0].Term != 2 || entries[1].Command != 40 {
		t.Fatalf("recovered base=%d entries=%+v", base, entries)
	}
	if err := recovered.Append(5, raft.Entry{Term: 3, Command: 50}); err != nil {
		t.Fatalf("append after compaction: %v", err)
	}
}
