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
		drop:    c.DropPermille,
		delay:   c.MaxDelay,
		scratch: make([]M, 0, 4096),
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
	for {
		idx := e.nextReady()
		if idx < 0 {
			break
		}
		ev := e.queue[idx]
		e.queue = append(e.queue[:idx], e.queue[idx+1:]...)
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
			e.seq++
			from, to := e.wire.Route(m)
			e.queue = append(e.queue, event[M]{at: e.Now + d, seq: e.seq, from: from, to: to, msg: m})
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
	return dropped
}

// Pending reports how many messages are currently in flight.
func (e *Engine[M]) Pending() int { return len(e.queue) }

// TraceHash is a running digest over every delivery the engine has performed.
// Two runs with the same seed, cluster, and caller actions must report the same
// hash at every step; a divergence localizes nondeterminism to the step where
// the hashes first differ.
func (e *Engine[M]) TraceHash() string { return fmt.Sprintf("%x", e.trace) }

func (e *Engine[M]) nextReady() int {
	best := -1
	for i := range e.queue {
		if e.queue[i].at > e.Now {
			continue
		}
		if best < 0 || e.queue[i].at < e.queue[best].at ||
			(e.queue[i].at == e.queue[best].at && e.queue[i].seq < e.queue[best].seq) {
			best = i
		}
	}
	return best
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
