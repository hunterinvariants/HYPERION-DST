package cli

import (
	"runtime"
	"sort"
	"strings"
	"testing"
)

// expectedNames pins the command set for this platform. A command that appears
// or disappears without this list changing is a mistake.
//
// Most of these names are also standalone binaries under cmd/, invoked by name
// from scripts/*.sh, the CI workflows, and the recorded evidence; those cannot
// be renamed. "simulate" and "verify" are newer and exist only as subcommands,
// because nothing historical refers to them.
func expectedNames() []string {
	names := []string{"backup", "ctl", "new", "probe", "seeds", "serve", "sim", "simulate", "uring-bench", "verify"}
	if runtime.GOOS == "linux" {
		names = append(names, "chaos")
		if runtime.GOARCH == "amd64" {
			names = append(names, "raw-bench")
		}
	}
	sort.Strings(names)
	return names
}

func TestCommandSetMatchesPlatform(t *testing.T) {
	var got []string
	for _, command := range Commands() {
		got = append(got, command.Name)
	}
	want := expectedNames()
	if len(got) != len(want) {
		t.Fatalf("commands = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("commands = %v, want %v", got, want)
		}
	}
}

func TestCommandsAreSortedAndComplete(t *testing.T) {
	seen := make(map[string]bool)
	previous := ""
	for _, command := range Commands() {
		if command.Name <= previous {
			t.Fatalf("command %q is out of order after %q", command.Name, previous)
		}
		previous = command.Name
		if seen[command.Name] {
			t.Fatalf("duplicate command %q", command.Name)
		}
		seen[command.Name] = true
		if command.Summary == "" {
			t.Errorf("command %q has no summary", command.Name)
		}
		if command.Run == nil {
			t.Errorf("command %q has no implementation", command.Name)
		}
	}
}

func TestLookupRejectsUnknownCommands(t *testing.T) {
	if _, ok := Lookup("no-such-command"); ok {
		t.Fatal("Lookup accepted an unknown command")
	}
	if code := Execute("no-such-command", nil); code != 2 {
		t.Fatalf("Execute returned %d for an unknown command, want 2", code)
	}
}

// TestPlatformCommandsAreAbsentNotFailing pins that a command needing kernel
// facilities this build cannot reach is simply not offered, rather than listed
// and then failing when invoked.
func TestPlatformCommandsAreAbsentNotFailing(t *testing.T) {
	_, hasChaos := Lookup("chaos")
	if want := runtime.GOOS == "linux"; hasChaos != want {
		t.Fatalf("chaos present = %v on %s, want %v", hasChaos, runtime.GOOS, want)
	}
	_, hasRawBench := Lookup("raw-bench")
	if want := runtime.GOOS == "linux" && runtime.GOARCH == "amd64"; hasRawBench != want {
		t.Fatalf("raw-bench present = %v on %s/%s, want %v",
			hasRawBench, runtime.GOOS, runtime.GOARCH, want)
	}
}

func TestUsageListsEveryCommand(t *testing.T) {
	var out strings.Builder
	Usage(&out, "hyperion")
	text := out.String()
	if !strings.Contains(text, "usage: hyperion <command> [flags]") {
		t.Fatalf("usage line missing from:\n%s", text)
	}
	for _, command := range Commands() {
		if !strings.Contains(text, command.Name) {
			t.Errorf("usage omits command %q", command.Name)
		}
		if !strings.Contains(text, command.Summary) {
			t.Errorf("usage omits the summary of %q", command.Name)
		}
	}
}

// TestArgumentValidationExitCodes pins the exit statuses the standalone
// binaries used before the implementations moved here. Scripts and CI branch on
// these, so they are part of the interface.
func TestArgumentValidationExitCodes(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want int
	}{
		{"probe", []string{"-entries", "0"}, 2},
		{"probe", []string{"-entries", "99999"}, 2},
		{"seeds", []string{"-from", "10", "-to", "5"}, 2},
		{"seeds", []string{"-workers", "0"}, 2},
		{"backup", []string{"-mode", "bogus"}, 1},
		{"ctl", []string{"-operation", "bogus"}, 2},
		{"uring-bench", []string{"-operations", "0"}, 1},
	} {
		command, ok := Lookup(test.name)
		if !ok {
			t.Fatalf("command %q is not registered", test.name)
		}
		if got := command.Run(test.args); got != test.want {
			t.Errorf("%s %v exited %d, want %d", test.name, test.args, got, test.want)
		}
	}
}
