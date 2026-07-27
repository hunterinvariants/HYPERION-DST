package chaos

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeRunner struct {
	calls  []string
	failAt int
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) error {
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	if f.failAt > 0 && len(f.calls) == f.failAt {
		return errors.New("injected command failure")
	}
	return nil
}

func TestRejectsHostInterfaceNames(t *testing.T) {
	_, err := New(Plan{
		Namespace: "hyperion-test", HostVeth: "eth0",
		PeerVeth: "hyperion-peer", BPFObject: "chaos.o",
	}, &fakeRunner{})
	if err == nil {
		t.Fatal("accepted non-HYPERION host interface")
	}
}

func TestFailureAlwaysRunsNamespaceCleanup(t *testing.T) {
	runner := &fakeRunner{failAt: 4}
	controller, err := New(Plan{
		Namespace: "hyperion-test", HostVeth: "hyperion-host",
		PeerVeth: "hyperion-peer", BPFObject: "chaos.o",
	}, runner)
	if err != nil {
		t.Fatal(err)
	}
	if err := controller.Apply(context.Background()); err == nil {
		t.Fatal("expected injected failure")
	}
	joined := strings.Join(runner.calls, "\n")
	if !strings.Contains(joined, "ip netns del hyperion-test") ||
		!strings.Contains(joined, "ip link del hyperion-host") {
		t.Fatalf("cleanup missing:\n%s", joined)
	}
}
