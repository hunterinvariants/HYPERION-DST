package cli

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hunterinvariants/HYPERION-DST/dst/scenario"
)

// TestProjectTemplateIsValidGo catches the failure that would matter most: a
// template that no longer parses. A generated project that does not compile is
// worse than no template, and nothing else here would notice, because the
// bodies are plain strings to the compiler.
func TestProjectTemplateIsValidGo(t *testing.T) {
	fset := token.NewFileSet()
	found := 0
	for _, file := range projectTemplate {
		if !strings.HasSuffix(file.name, ".go") {
			continue
		}
		found++
		body := strings.ReplaceAll(file.body, modulePlaceholder, "example.com/generated")
		if _, err := parser.ParseFile(fset, file.name, body, parser.SkipObjectResolution); err != nil {
			t.Errorf("%s does not parse: %v", file.name, err)
		}
	}
	if found == 0 {
		t.Fatal("the template contains no Go files")
	}
}

// TestProjectTemplateScenarioIsAccepted requires the shipped scenario to
// survive the same strict parser a user's own file meets. Shipping one that
// the parser rejects would make the first thing a newcomer runs fail.
func TestProjectTemplateScenarioIsAccepted(t *testing.T) {
	var body string
	for _, file := range projectTemplate {
		if file.name == "scenario.json" {
			body = file.body
		}
	}
	if body == "" {
		t.Fatal("the template has no scenario.json")
	}
	spec, err := scenario.Read(strings.NewReader(body))
	if err != nil {
		t.Fatalf("the shipped scenario is rejected: %v", err)
	}
	injectors, err := spec.Injectors()
	if err != nil {
		t.Fatalf("injectors: %v", err)
	}
	if len(injectors) == 0 {
		t.Error("the shipped scenario declares no fault, so it teaches nothing about faults")
	}
	// A fault window that opens after the placeholder protocol has finished
	// sending would drop nothing, and the generated project's own vacuity
	// guard would fail on a fresh checkout.
	for _, fault := range spec.Faults {
		if fault.Start > 10 {
			t.Errorf("fault %d opens at step %d, likely after the placeholder protocol has gone quiet",
				fault.Start, fault.Start)
		}
	}
}

func TestNewRefusesANonEmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := runNew([]string{dir}); code != 2 {
		t.Fatalf("runNew returned %d for a non-empty directory, want 2", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "protocol.go")); !os.IsNotExist(err) {
		t.Error("the command wrote into a directory it should have refused")
	}
}

func TestNewWritesEveryTemplateFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "project")
	if code := runNew([]string{"-module", "example.com/generated", dir}); code != 0 {
		t.Fatalf("runNew returned %d", code)
	}
	for _, file := range projectTemplate {
		if _, err := os.Stat(filepath.Join(dir, file.name)); err != nil {
			t.Errorf("missing %s: %v", file.name, err)
		}
	}
	body, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "module example.com/generated") {
		t.Errorf("go.mod does not carry the requested module path:\n%s", body)
	}
	if strings.Contains(string(body), modulePlaceholder) {
		t.Error("the module placeholder survived into the generated file")
	}
}

func TestNewRequiresADirectory(t *testing.T) {
	if code := runNew(nil); code != 2 {
		t.Fatalf("runNew returned %d without a directory, want 2", code)
	}
}
