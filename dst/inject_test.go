package dst_test

import (
	"testing"

	"github.com/hunterinvariants/promtact/dst"
)

// TestNoInjectorLeavesTheScheduleUnchanged is the compatibility guarantee: the
// fault facility must cost nothing when unused.
func TestNoInjectorLeavesTheScheduleUnchanged(t *testing.T) {
	trace := func(inject bool) string {
		r := newRing(5, 2)
		engine := dst.New[ping](dst.Config{Seed: 21, DropPermille: 60, MaxDelay: 5}, r, r)
		if inject {
			engine.Inject(dst.InjectorFunc{
				Label: "allow everything",
				Fn:    func(uint64, uint32, uint32, uint64) bool { return true },
			})
		}
		engine.Run(200)
		return engine.TraceHash()
	}
	if a, b := trace(false), trace(true); a != b {
		t.Fatalf("an always-allowing injector changed the trace: %s != %s", a, b)
	}
}

// TestSplitBlocksExactlyTheCrossingMessages checks both halves of the
// property: crossing traffic is dropped, and traffic inside a group is not.
func TestSplitBlocksExactlyTheCrossingMessages(t *testing.T) {
	r := newRing(6, 2)
	engine := dst.New[ping](dst.Config{Seed: 5}, r, r)
	left, right := []uint32{1, 2, 3}, []uint32{4, 5, 6}
	engine.Inject(dst.Split(left, right))

	var crossed int
	engine.Inject(dst.InjectorFunc{
		Label: "observer",
		Fn: func(_ uint64, from, to uint32, _ uint64) bool {
			crossed++
			return true
		},
	})
	engine.Run(120)

	drops := engine.InjectedDrops()
	blocked := drops[dst.Split(left, right).Name()]
	if blocked == 0 {
		t.Fatal("the partition never dropped a message, so this run proves nothing")
	}
	if crossed == 0 {
		t.Fatal("no message survived the partition, so the ring never ran")
	}
	// The ring sends 1->2, 2->3, ... 6->1. Exactly two of those six edges
	// cross the split, so a strict majority must survive.
	if crossed <= blocked {
		t.Fatalf("%d messages survived and %d were blocked; the split is dropping too much", crossed, blocked)
	}
}

func TestIsolateStopsAllTrafficForOneNode(t *testing.T) {
	r := newRing(4, 2)
	engine := dst.New[ping](dst.Config{Seed: 6}, r, r)
	engine.Inject(dst.Isolate(3))
	engine.Inject(dst.InjectorFunc{
		Label: "guard",
		Fn: func(_ uint64, from, to uint32, _ uint64) bool {
			if from == 3 || to == 3 {
				t.Errorf("message %d->%d reached the schedule despite isolation", from, to)
			}
			return true
		},
	})
	engine.Run(100)

	if engine.InjectedDrops()[dst.Isolate(3).Name()] == 0 {
		t.Fatal("isolation never fired")
	}
	if r.nodes[3].received != 0 {
		t.Fatalf("isolated node received %d messages", r.nodes[3].received)
	}
}

func TestLinkFailsOneDirectionOnly(t *testing.T) {
	r := newRing(3, 2)
	engine := dst.New[ping](dst.Config{Seed: 7}, r, r)
	engine.Inject(dst.Link(1, 2))
	engine.Run(100)

	if engine.InjectedDrops()[dst.Link(1, 2).Name()] == 0 {
		t.Fatal("the one-way link failure never fired")
	}
	if r.nodes[2].received != 0 {
		t.Fatalf("node 2 received %d messages from the failed link", r.nodes[2].received)
	}
	// The ring is 1->2->3->1, so the reverse direction of the broken link is
	// carried by 3->1 and must be unaffected.
	if r.nodes[1].received == 0 {
		t.Fatal("node 1 received nothing, so the failure was not one-way")
	}
}

func TestDuringLimitsAFaultToItsWindow(t *testing.T) {
	r := newRing(4, 1)
	engine := dst.New[ping](dst.Config{Seed: 8}, r, r)
	window := dst.During(20, 40, dst.Isolate(2))
	engine.Inject(window)

	engine.Run(19)
	if got := engine.InjectedDrops()[window.Name()]; got != 0 {
		t.Fatalf("fault dropped %d messages before its window opened", got)
	}
	engine.Run(21)
	during := engine.InjectedDrops()[window.Name()]
	if during == 0 {
		t.Fatal("fault dropped nothing inside its window")
	}
	engine.Run(40)
	if after := engine.InjectedDrops()[window.Name()]; after != during {
		t.Fatalf("fault dropped %d more messages after its window closed", after-during)
	}
}
