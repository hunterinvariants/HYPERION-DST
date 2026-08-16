//go:build linux && amd64

package uringwal

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/hunterinvariants/hyperion/storage"
	"github.com/hunterinvariants/hyperion/storage/wal"
)

func TestWALReopenThroughIOUring(t *testing.T) {
	if os.Getenv("HYPERION_URING_INTEGRATION") != "1" {
		t.Skip("set HYPERION_URING_INTEGRATION=1 on Linux")
	}
	path := filepath.Join(t.TempDir(), "uring.wal")
	device, err := Open(path, 8)
	if err != nil {
		t.Fatal(err)
	}
	log, _, err := wal.Open(device)
	if err != nil {
		t.Fatal(err)
	}
	want := []storage.Entry{
		{Index: 1, Term: 4, Command: 10},
		{Index: 2, Term: 4, Command: 20},
		{Index: 3, Term: 5, Command: 30},
	}
	if err := log.Append(context.Background(), want); err != nil {
		t.Fatal(err)
	}
	if err := log.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := device.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path, 8)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	_, records, err := wal.Open(reopened)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != len(want) {
		t.Fatalf("recovered %d records, want %d", len(records), len(want))
	}
	for index := range records {
		if records[index].Entry != want[index] {
			t.Fatalf("record %d = %+v, want %+v", index, records[index].Entry, want[index])
		}
	}
}
