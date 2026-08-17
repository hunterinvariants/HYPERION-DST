//go:build linux && amd64

package uring

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestDurableWriterIntegration(t *testing.T) {
	if os.Getenv("PROMTACT_URING_INTEGRATION") != "1" {
		t.Skip("set PROMTACT_URING_INTEGRATION=1 on a Linux filesystem supporting O_DIRECT")
	}
	path := filepath.Join(t.TempDir(), "uring-direct.dat")
	writer, err := OpenDurableWriter(path, 8, DefaultAlignment)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("promtact-durable")
	if err := writer.AppendDurable(0, payload); err != nil {
		_ = writer.Close()
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	block, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(block) != DefaultAlignment || !bytes.Equal(block[:len(payload)], payload) {
		t.Fatalf("durable block mismatch: length=%d prefix=%q", len(block), block[:len(payload)])
	}
	for i, value := range block[len(payload):] {
		if value != 0 {
			t.Fatalf("non-zero padding at %d", i+len(payload))
		}
	}
}
