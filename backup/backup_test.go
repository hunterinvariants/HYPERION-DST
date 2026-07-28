package backup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateRestoreAndVerify(t *testing.T) {
	source := filepath.Join(t.TempDir(), "data")
	if err := os.Mkdir(source, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "raft.wal"), []byte("records"), 0o600); err != nil {
		t.Fatal(err)
	}
	image := filepath.Join(t.TempDir(), "backup")
	if err := Create(source, image); err != nil {
		t.Fatal(err)
	}
	restored := filepath.Join(t.TempDir(), "restored")
	if err := Restore(image, restored); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(restored, "raft.wal"))
	if err != nil || string(data) != "records" {
		t.Fatalf("restored = %q %v", data, err)
	}
}
