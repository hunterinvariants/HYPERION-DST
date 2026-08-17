package raftwal

import (
	"testing"

	"github.com/hunterinvariants/promtact/raft"
	"github.com/hunterinvariants/promtact/storage/wal"
)

func TestClientRequestFieldsSurviveReplay(t *testing.T) {
	device := wal.NewMemoryDevice(nil)
	store, err := Open(device)
	if err != nil {
		t.Fatal(err)
	}
	want := raft.Entry{Term: 8, Operation: raft.CommandPut, ClientID: 41, RequestID: 12, Key: 99, Value: 1234}
	if err := store.Append(1, want); err != nil {
		t.Fatal(err)
	}
	recovered, err := Open(wal.NewMemoryDevice(device.DurableBytes()))
	if err != nil {
		t.Fatal(err)
	}
	_, entries := recovered.State()
	if len(entries) != 2 || entries[1] != want {
		t.Fatalf("replayed entry = %+v", entries)
	}
}
