package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestReleaseWorkflowStampsThisVariable guards the one link that can break in
// silence. The stamp reaches this file through a string in a YAML file, so a
// renamed package, a renamed variable or a changed module path leaves the build
// working, the tests passing, and every published binary reporting "(devel)",
// which is exactly the question a downloaded file exists to answer.
func TestReleaseWorkflowStampsThisVariable(t *testing.T) {
	root := filepath.Join("..", "..")

	gomod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("reading go.mod: %v", err)
	}
	module := regexp.MustCompile(`(?m)^module (\S+)$`).FindStringSubmatch(string(gomod))
	if module == nil {
		t.Fatal("go.mod declares no module path")
	}

	workflow, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatalf("reading the release workflow: %v", err)
	}

	want := "-X " + module[1] + "/internal/cli.version="
	if !strings.Contains(string(workflow), want) {
		t.Errorf("the release workflow does not stamp %q;\n"+
			"published binaries would report their version as %q", want, "(devel)")
	}
}

// TestVersionReportsSomethingUsable pins the behaviour of an unstamped build,
// which is what a contributor gets. Reporting the toolchain's own "(devel)" is
// the honest answer; reporting an empty string or a made-up number is not.
func TestVersionReportsSomethingUsable(t *testing.T) {
	release, _, _, _ := buildIdentity()
	if strings.TrimSpace(release) == "" {
		t.Error("buildIdentity returned an empty version")
	}
}

// TestVersionRejectsUnknownFlags keeps the command from silently accepting
// arguments it does not implement.
func TestVersionRejectsUnknownFlags(t *testing.T) {
	if code := runVersion([]string{"-nonsense"}); code != 2 {
		t.Errorf("runVersion returned %d for an unknown flag, want 2", code)
	}
}
