package apisurface

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite docs/api-surface.txt from the source")

const goldenPath = "docs/api-surface.txt"

func moduleRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("locate module root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("module root %s has no go.mod: %v", root, err)
	}
	return root
}

// TestSurfaceMatchesGolden fails when the exported surface of a contractual
// package changes without docs/api-surface.txt changing in the same commit.
// The point is not to forbid API changes but to make them visible in review,
// where someone can decide whether the version policy in docs/API.md requires
// a minor bump.
func TestSurfaceMatchesGolden(t *testing.T) {
	root := moduleRoot(t)
	got, err := Collect(root)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	golden := filepath.Join(root, filepath.FromSlash(goldenPath))

	if *update {
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("rewrote %s", goldenPath)
		return
	}

	wantBytes, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read %s: %v\nrun: go test ./internal/apisurface -update", goldenPath, err)
	}
	want := strings.ReplaceAll(string(wantBytes), "\r\n", "\n")
	if got == want {
		return
	}

	t.Errorf("the exported API surface no longer matches %s.\n"+
		"If the change is intentional, run:\n\n    go test ./internal/apisurface -update\n\n"+
		"and review the resulting diff against the version policy in docs/API.md.\n\n%s",
		goldenPath, firstDifference(want, got))
}

// firstDifference reports the first differing line with a little context, so
// the failure names the identifier rather than dumping the whole surface.
func firstDifference(want, got string) string {
	wantLines := strings.Split(want, "\n")
	gotLines := strings.Split(got, "\n")
	for i := 0; i < len(wantLines) || i < len(gotLines); i++ {
		w, g := "", ""
		if i < len(wantLines) {
			w = wantLines[i]
		}
		if i < len(gotLines) {
			g = gotLines[i]
		}
		if w != g {
			return "first difference at line " + itoa(i+1) + ":\n  recorded: " + w + "\n  current:  " + g
		}
	}
	return "files differ only in trailing content"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// TestCollectIsDeterministic guards the generator itself. A surface rendered
// from map iteration would produce spurious diffs and train reviewers to run
// -update without looking.
func TestCollectIsDeterministic(t *testing.T) {
	root := moduleRoot(t)
	first, err := Collect(root)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	for i := 0; i < 5; i++ {
		again, err := Collect(root)
		if err != nil {
			t.Fatalf("collect: %v", err)
		}
		if again != first {
			t.Fatal("Collect produced different output for identical input")
		}
	}
}

// TestGoldenCoversTheFrameworkInterfaces is the negative control for the
// golden file: if the generator silently stopped finding declarations, the
// comparison above would still pass on an empty file.
func TestGoldenCoversTheFrameworkInterfaces(t *testing.T) {
	root := moduleRoot(t)
	got, err := Collect(root)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	for _, required := range []string{
		"type Cluster[M any] interface",
		"type Wire[M any] interface",
		"type Invariant interface",
		"type Injector interface",
		"type Device interface",
		"type StableStore interface",
		"type SnapshotStore interface",
		"func Split(a, b []uint32) Injector",
		"func RunDeviceSuite(t *testing.T, factory NewDevice)",
	} {
		if !strings.Contains(got, required) {
			t.Errorf("the collected surface is missing %q", required)
		}
	}
}
