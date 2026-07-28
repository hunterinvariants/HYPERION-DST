package statemachine

import (
	"bytes"
	"testing"

	"github.com/hunterinvariants/HYPERION-DST/raft"
)

func TestDeduplicationAndSnapshotRoundTrip(t *testing.T) {
	m := New()
	put := raft.Entry{Operation: raft.CommandPut, ClientID: 7, RequestID: 1, Key: 9, Value: 42}
	first, err := m.Apply(put)
	if err != nil || first.Value != 42 {
		t.Fatalf("first apply: %+v %v", first, err)
	}
	second, err := m.Apply(put)
	if err != nil || second != first {
		t.Fatalf("dedupe: %+v %v", second, err)
	}
	if _, err := m.Apply(raft.Entry{Operation: raft.CommandPut, ClientID: 7, RequestID: 0, Key: 9}); err != ErrStaleRequest {
		t.Fatalf("stale request error = %v", err)
	}
	image := m.Snapshot()
	if !bytes.Equal(image, m.Snapshot()) {
		t.Fatal("snapshot is not deterministic")
	}
	recovered, err := Restore(image)
	if err != nil {
		t.Fatal(err)
	}
	if value, ok := recovered.Get(9); !ok || value != 42 {
		t.Fatalf("restored value = %d, %t", value, ok)
	}
	image[len(image)-1] ^= 1
	if _, err := Restore(image); err != ErrBadSnapshot {
		t.Fatalf("corruption error = %v", err)
	}
}
