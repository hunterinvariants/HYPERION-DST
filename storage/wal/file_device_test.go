package wal

import (
	"path/filepath"
	"testing"
)

func TestFileDeviceDurabilityAndReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "raft.wal")
	device, err := OpenFileDevice(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := device.Append([]byte("durable")); err != nil {
		t.Fatal(err)
	}
	if err := device.Sync(); err != nil {
		t.Fatal(err)
	}
	if err := device.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenFileDevice(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if got := string(reopened.DurableBytes()); got != "durable" {
		t.Fatalf("durable bytes = %q", got)
	}
}
