package dst_test

import (
	"testing"

	"github.com/hunterinvariants/hyperion/dst"
)

// The ring protocol below is deliberately not Raft. It exists to prove that the
// engine drives an arbitrary message-passing protocol: if the engine still
// leaked consensus assumptions, this package would not compile or would not be
// reproducible.

type ping struct {
	from, to uint32
	seq      uint64
}

type ringNode struct {
	ticks    uint64
	sent     uint64
	received uint64
	outbound []ping
}

type ring struct {
	ids   []uint32
	nodes map[uint32]*ringNode
	// period is the tick interval at which each node emits to its successor.
	period uint64
}

func newRing(count int, period uint64) *ring {
	r := &ring{ids: make([]uint32, 0, count), nodes: make(map[uint32]*ringNode, count), period: period}
	for i := 1; i <= count; i++ {
		r.ids = append(r.ids, uint32(i))
		r.nodes[uint32(i)] = &ringNode{}
	}
	return r
}

func (r *ring) Nodes() []uint32 { return r.ids }

func (r *ring) Tick(id uint32) {
	n := r.nodes[id]
	n.ticks++
	if n.ticks%r.period != 0 {
		return
	}
	successor := uint32(int(id)%len(r.ids) + 1)
	n.sent++
	n.outbound = append(n.outbound, ping{from: id, to: successor, seq: n.sent})
}

func (r *ring) Deliver(id uint32, msg ping) { r.nodes[id].received++ }

func (r *ring) Drain(id uint32, dst []ping) []ping {
	n := r.nodes[id]
	dst = append(dst, n.outbound...)
	n.outbound = n.outbound[:0]
	return dst
}

func (r *ring) Route(msg ping) (uint32, uint32) { return msg.from, msg.to }

func (r *ring) Digest(msg ping) (uint8, uint64) { return 1, msg.seq }

func (r *ring) totals() (sent, received uint64) {
	for _, id := range r.ids {
		sent += r.nodes[id].sent
		received += r.nodes[id].received
	}
	return sent, received
}

func TestEngineDrivesNonConsensusProtocol(t *testing.T) {
	r := newRing(4, 3)
	engine := dst.New[ping](dst.Config{Seed: 11}, r, r)
	engine.Run(300)

	sent, received := r.totals()
	if sent == 0 {
		t.Fatal("ring produced no messages")
	}
	if received != sent {
		t.Fatalf("lossless run delivered %d of %d messages", received, sent)
	}
	if engine.Pending() != 0 {
		t.Fatalf("%d messages still in flight after a lossless run", engine.Pending())
	}
}

func TestEngineTraceIsSeedReproducible(t *testing.T) {
	run := func(seed int64) string {
		r := newRing(5, 2)
		engine := dst.New[ping](dst.Config{Seed: seed, DropPermille: 120, MaxDelay: 6}, r, r)
		engine.Run(250)
		return engine.TraceHash()
	}
	if a, b := run(3), run(3); a != b {
		t.Fatalf("same seed produced different traces: %s != %s", a, b)
	}
	if a, b := run(3), run(4); a == b {
		t.Fatalf("seeds 3 and 4 produced the same trace %s, so the seed is not driving the schedule", a)
	}
}

func TestEngineDropsEveryMessageAtFullLoss(t *testing.T) {
	r := newRing(3, 2)
	engine := dst.New[ping](dst.Config{Seed: 5, DropPermille: 1000}, r, r)
	engine.Run(120)

	sent, received := r.totals()
	if sent == 0 {
		t.Fatal("ring produced no messages")
	}
	if received != 0 {
		t.Fatalf("total partition delivered %d messages", received)
	}
	if engine.Pending() != 0 {
		t.Fatalf("%d messages queued under total loss", engine.Pending())
	}
}

func TestIsolateDiscardsOnlyTheNamedNodesTraffic(t *testing.T) {
	r := newRing(5, 2)
	engine := dst.New[ping](dst.Config{Seed: 9, MaxDelay: 40}, r, r)
	engine.Run(30)
	if engine.Pending() == 0 {
		t.Fatal("expected in-flight messages under a long delay bound")
	}

	before := engine.Pending()
	dropped := engine.Isolate(2)
	if dropped == 0 {
		t.Fatal("node 2 had no in-flight traffic to discard")
	}
	if got := engine.Pending(); got != before-dropped {
		t.Fatalf("queue length %d does not match %d - %d", got, before, dropped)
	}
	if again := engine.Isolate(2); again != 0 {
		t.Fatalf("second isolation discarded %d further messages", again)
	}
}

func TestEngineRequiresClusterAndWire(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected a panic for a nil cluster")
		}
	}()
	dst.New[ping](dst.Config{}, nil, nil)
}
