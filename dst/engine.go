// Package dst provides a protocol-agnostic deterministic simulation engine.
//
// The engine owns virtual time, seeded message scheduling, loss and delay
// injection, and a reproducible execution trace. It owns no protocol state:
// everything protocol-specific sits behind Cluster and Wire, so any
// message-passing protocol can be driven under the same conditions the
// HYPERION Raft core is driven under.
package dst

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math/rand"
)

// Cluster owns all protocol state for one simulated deployment.
//
// Every method must be deterministic: identical call sequences must produce
// identical observable behavior, otherwise the trace hash loses its meaning.
type Cluster[M any] interface {
	// Nodes returns the node identifiers to drive, in a stable order. The
	// engine neither retains nor mutates the returned slice, so an
	// implementation may return the same backing array on every call.
	Nodes() []uint32
	// Tick advances one node by one unit of virtual time.
	Tick(node uint32)
	// Deliver hands one scheduled message to its destination node.
	Deliver(node uint32, msg M)
	// Drain appends every message the node wants to send to dst and returns
	// the extended slice, leaving the node's outbound buffer empty.
	Drain(node uint32, dst []M) []M
}

// Wire is the engine's read-only view of a protocol message.
type Wire[M any] interface {
	// Route reports the sender and destination the engine uses for
	// scheduling and for per-node fault decisions.
	Route(msg M) (from, to uint32)
	// Digest contributes protocol-visible fields to the execution trace.
	// Include only values that are part of observable protocol behavior;
	// anything nondeterministic makes traces incomparable.
	Digest(msg M) (kind uint8, value uint64)
}

// Config parameterizes one deterministic run.
type Config struct {
	// Seed fixes the whole schedule. Equal seeds over equal Cluster behavior
	// produce equal trace hashes.
	Seed int64
	// DropPermille is the per-message loss probability in tenths of a percent.
	DropPermille int
	// MaxDelay is the inclusive upper bound, in virtual time units, on the
	// delay applied to a scheduled message.
	MaxDelay uint64
}

type event[M any] struct {
	at       uint64
	seq      uint64
	from, to uint32
	msg      M
}

// Engine is a single-threaded deterministic executor. It is not safe for
// concurrent use; parallelism belongs between runs, not inside one.
type Engine[M any] struct {
	// Now is the current virtual time, in steps since the run started.
	Now uint64

	cluster    Cluster[M]
	wire       Wire[M]
	rng        *rand.Rand
	drop       int
	delay      uint64
	seq        uint64
	queue      []event[M]
	trace      [32]byte
	scratch    []M
	invariants []Invariant
	injectors  []Injector
	injected   map[string]int
}

// New builds an engine over a cluster. The cluster must already be
// constructed; the engine never creates or destroys nodes.
func New[M any](c Config, cluster Cluster[M], wire Wire[M]) *Engine[M] {
	if cluster == nil || wire == nil {
		panic("dst: cluster and wire are required")
	}
	return &Engine[M]{
		cluster: cluster,
		wire:    wire,
		rng:     rand.New(rand.NewSource(c.Seed)),
		drop:     c.DropPermille,
		delay:    c.MaxDelay,
		scratch:  make([]M, 0, 4096),
		injected: make(map[string]int),
	}
}

// Step advances virtual time by one unit: every node ticks, then every message
// that has come due is delivered in schedule order. Messages produced by a
// delivery are collected immediately, so a single step can cascade.
func (e *Engine[M]) Step() {
	e.Now++
	for _, id := range e.cluster.Nodes() {
		e.cluster.Tick(id)
	}
	e.Collect()
	// The queue is a min-heap on (at, seq). Any message that is due is at the
	// root, because the root is the global minimum and a due message can only
	// be smaller than one that is not.
	for len(e.queue) > 0 && e.queue[0].at <= e.Now {
		ev := e.pop()
		e.record(ev)
		e.cluster.Deliver(ev.to, ev.msg)
		e.Collect()
	}
}

// Run advances the engine by the given number of steps.
func (e *Engine[M]) Run(steps uint64) {
	for range steps {
		e.Step()
	}
}

// Watch registers invariants for StepChecked and RunChecked to evaluate. It is
// additive: calling it twice keeps both sets. Step and Run ignore invariants
// entirely, so an existing run loop keeps its exact behavior.
func (e *Engine[M]) Watch(invariants ...Invariant) {
	e.invariants = append(e.invariants, invariants...)
}

// StepChecked advances one step and then evaluates every registered invariant
// in registration order, returning a *Violation for the first failure.
func (e *Engine[M]) StepChecked() error {
	e.Step()
	return e.CheckInvariants()
}

// RunChecked advances up to steps, stopping at the first violation. The engine
// is left at the step where the violation was detected so the caller can
// inspect the cluster.
func (e *Engine[M]) RunChecked(steps uint64) error {
	for range steps {
		if err := e.StepChecked(); err != nil {
			return err
		}
	}
	return nil
}

// CheckInvariants evaluates the registered invariants without advancing time.
// Use it after driving the cluster out of band, for example after a proposal or
// a restart.
func (e *Engine[M]) CheckInvariants() error {
	for _, inv := range e.invariants {
		if err := inv.Check(); err != nil {
			return &Violation{
				Invariant: inv.Name(),
				Step:      e.Now,
				Trace:     e.TraceHash(),
				Err:       err,
			}
		}
	}
	return nil
}

// Collect drains every node's outbound buffer into the schedule, applying loss
// and delay. Call it after driving the cluster out of band, for example after
// submitting a client proposal, so those messages enter the same schedule.
func (e *Engine[M]) Collect() {
	for _, id := range e.cluster.Nodes() {
		e.scratch = e.cluster.Drain(id, e.scratch[:0])
		for _, m := range e.scratch {
			if e.drop > 0 && e.rng.Intn(1000) < e.drop {
				continue
			}
			d := uint64(0)
			if e.delay > 0 {
				d = uint64(e.rng.Int63n(int64(e.delay + 1)))
			}
			from, to := e.wire.Route(m)
			// Injectors are consulted only after both draws above, so a
			// registered fault never shifts the seeded schedule.
			if blocked := e.blockedBy(from, to, d); blocked != "" {
				e.injected[blocked]++
				continue
			}
			e.seq++
			e.push(event[M]{at: e.Now + d, seq: e.seq, from: from, to: to, msg: m})
		}
	}
}

// Isolate discards every in-flight message sent from or to the node and
// reports how many were dropped. It models the loss of volatile network state
// across a crash and must be paired with whatever durable-state reconstruction
// the cluster performs.
func (e *Engine[M]) Isolate(node uint32) int {
	kept, dropped := e.queue[:0], 0
	for _, ev := range e.queue {
		if ev.from == node || ev.to == node {
			dropped++
			continue
		}
		kept = append(kept, ev)
	}
	e.queue = kept
	// Removing arbitrary elements breaks the heap property, so rebuild.
	for i := len(e.queue)/2 - 1; i >= 0; i-- {
		e.siftDown(i)
	}
	return dropped
}

// Pending reports how many messages are currently in flight.
func (e *Engine[M]) Pending() int { return len(e.queue) }

// Inject registers network faults. It is additive, and registering none leaves
// the engine's behavior bit-identical to a build without this facility.
func (e *Engine[M]) Inject(injectors ...Injector) {
	e.injectors = append(e.injectors, injectors...)
}

// InjectedDrops reports how many messages each registered fault has discarded,
// keyed by injector name. A fault whose count is zero never fired, which makes
// any conclusion drawn from that run vacuous; assert on this rather than
// assuming the fault took effect.
func (e *Engine[M]) InjectedDrops() map[string]int {
	out := make(map[string]int, len(e.injected))
	for name, count := range e.injected {
		out[name] = count
	}
	return out
}

// blockedBy returns the name of the first injector that denies the message, or
// the empty string if every injector allows it.
func (e *Engine[M]) blockedBy(from, to uint32, delay uint64) string {
	for _, injector := range e.injectors {
		if !injector.Allow(e.Now, from, to, delay) {
			return injector.Name()
		}
	}
	return ""
}

// TraceHash is a running digest over every delivery the engine has performed.
// Two runs with the same seed, cluster, and caller actions must report the same
// hash at every step; a divergence localizes nondeterminism to the step where
// the hashes first differ.
func (e *Engine[M]) TraceHash() string { return fmt.Sprintf("%x", e.trace) }

// earlier is the schedule's total order: due time first, then the sequence
// number in which the message was collected. Because it is total, a heap
// ordered by it delivers messages in exactly the order a linear scan for the
// smallest due message would.
func earlier[M any](a, b event[M]) bool {
	return a.at < b.at || (a.at == b.at && a.seq < b.seq)
}

func (e *Engine[M]) push(ev event[M]) {
	e.queue = append(e.queue, ev)
	for i := len(e.queue) - 1; i > 0; {
		parent := (i - 1) / 2
		if !earlier(e.queue[i], e.queue[parent]) {
			break
		}
		e.queue[i], e.queue[parent] = e.queue[parent], e.queue[i]
		i = parent
	}
}

func (e *Engine[M]) pop() event[M] {
	top := e.queue[0]
	last := len(e.queue) - 1
	e.queue[0] = e.queue[last]
	var zero event[M]
	e.queue[last] = zero // release the message for collection
	e.queue = e.queue[:last]
	e.siftDown(0)
	return top
}

func (e *Engine[M]) siftDown(i int) {
	n := len(e.queue)
	for {
		left, smallest := 2*i+1, i
		if left < n && earlier(e.queue[left], e.queue[smallest]) {
			smallest = left
		}
		if right := left + 1; right < n && earlier(e.queue[right], e.queue[smallest]) {
			smallest = right
		}
		if smallest == i {
			return
		}
		e.queue[i], e.queue[smallest] = e.queue[smallest], e.queue[i]
		i = smallest
	}
}

func (e *Engine[M]) record(ev event[M]) {
	kind, value := e.wire.Digest(ev.msg)
	var b [65]byte
	binary.LittleEndian.PutUint64(b[0:], e.Now)
	binary.LittleEndian.PutUint64(b[8:], ev.seq)
	binary.LittleEndian.PutUint32(b[16:], ev.from)
	binary.LittleEndian.PutUint32(b[20:], ev.to)
	b[24] = kind
	binary.LittleEndian.PutUint64(b[25:], value)
	copy(b[33:], e.trace[:])
	e.trace = sha256.Sum256(b[:])
}
