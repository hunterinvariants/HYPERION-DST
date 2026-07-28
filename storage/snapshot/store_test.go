package snapshot

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestStoreDurableReplacementAndReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.snap")
	store, empty, err := OpenStore(path)
	if err != nil || empty.LastIndex != 0 {
		t.Fatalf("open empty: image=%+v err=%v", empty, err)
	}
	want := Image{LastIndex: 9, LastTerm: 3, State: []byte("state-nine")}
	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}
	_, got, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.LastIndex != want.LastIndex || got.LastTerm != want.LastTerm ||
		string(got.State) != string(want.State) {
		t.Fatalf("reopened image = %+v", got)
	}
}

func TestStoreRejectsRegressionAndCorruption(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.snap")
	store, _, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(Image{LastIndex: 10, LastTerm: 2}); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(Image{LastIndex: 9, LastTerm: 2}); !errors.Is(err, ErrRegression) {
		t.Fatalf("regression error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data[len(data)-1] ^= 1
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := OpenStore(path); !errors.Is(err, ErrChecksum) {
		t.Fatalf("corrupt reopen error = %v", err)
	}
}
