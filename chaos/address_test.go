package chaos

import (
	"context"
	"strings"
	"testing"
)

func TestApplyConfiguresOnlyReservedTestNetwork(t *testing.T) {
	runner := &fakeRunner{}
	controller, err := New(Plan{
		Namespace: "hyperion-test", HostVeth: "hyperion-host",
		PeerVeth: "hyperion-peer", BPFObject: "chaos.o",
	}, runner)
	if err != nil {
		t.Fatal(err)
	}
	if err := controller.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(runner.calls, "\n")
	if !strings.Contains(joined, "ip addr add 192.0.2.1/30 dev hyperion-host") ||
		!strings.Contains(joined, "ip addr add 192.0.2.2/30 dev hyperion-peer") {
		t.Fatalf("reserved test addresses missing:\n%s", joined)
	}
}
