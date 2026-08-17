package cli

// projectTemplate is what "promtact new" writes. The protocol it contains is a
// placeholder, but a working one with a real safety property, so that a fresh
// project passes its tests immediately and the first edit has something to
// break. A skeleton full of TODOs teaches nothing about whether the wiring is
// right.
var projectTemplate = []templateFile{
	{name: "go.mod", body: goModTemplate},
	{name: "main.go", body: mainTemplate},
	{name: "protocol.go", body: protocolTemplate},
	{name: "cluster.go", body: clusterTemplate},
	{name: "protocol_test.go", body: testTemplate},
	{name: "scenario.json", body: scenarioTemplate},
	{name: "README.md", body: readmeTemplate},
}

// mainTemplate makes the generated project a program as well as a test suite.
// Without it the package declares main and has no main function, so go build
// and go run fail on a freshly generated project while go test succeeds, a
// trap that only shows up once someone tries the obvious command.
const mainTemplate = `package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/hunterinvariants/promtact/dst"
	"github.com/hunterinvariants/promtact/dst/scenario"
)

// main runs scenario.json once and reports what happened. The campaigns in
// protocol_test.go are where the real work is; this exists so that go run .
// gives you a single reproducible run to look at.
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	spec, err := scenario.Load("scenario.json")
	if err != nil {
		return err
	}
	config, err := spec.EngineConfig()
	if err != nil {
		return err
	}
	injectors, err := spec.Injectors()
	if err != nil {
		return err
	}

	cluster := New(spec.Nodes)
	engine := dst.New[Message](config, cluster, cluster)
	engine.Watch(cluster.SafetyInvariants()...)
	engine.Inject(injectors...)

	cluster.Propose(1, 0xFEED)
	engine.Collect()

	if err := engine.RunChecked(spec.Steps); err != nil {
		var violation *dst.Violation
		if errors.As(err, &violation) {
			// A violation is a coordinate, not just a message: the same seed
			// and the same actions reach the same step and the same trace.
			fmt.Printf("violated=%q step=%d trace=%s\n",
				violation.Invariant, violation.Step, violation.Trace)
		}
		return err
	}

	// The field counts nodes holding a value, not values held. Printing it as
	// "adopted" next to a property called "one adopted value" reads like a
	// violation the run failed to catch, which is the opposite of what a
	// starter project should teach.
	adopters := 0
	for _, id := range cluster.Nodes() {
		if _, ok := cluster.Node(id).Adopted(); ok {
			adopters++
		}
	}
	fmt.Printf("scenario=%q seed=%s nodes=%d steps=%d adopters=%d trace=%s\n",
		spec.Name, spec.Seed, spec.Nodes, spec.Steps, adopters, engine.TraceHash())
	for _, injector := range injectors {
		fmt.Printf("fault=%q dropped=%d\n", injector.Name(), engine.InjectedDrops()[injector.Name()])
	}
	return nil
}
`

// goModTemplate carries a toolchain directive for a reason that only shows up
// on a machine whose Go is older than this module's. A go line on its own asks
// for a toolchain named "go1.25", and no such toolchain is published: the
// releases are go1.25.0, go1.25.13 and so on. so every command in a generated
// project fails with "toolchain not available". Naming the patch release makes
// the switch resolvable, which is what lets somebody with the Go their
// distribution shipped run the project at all.
//
// TestProjectTemplatePinsTheRepositoryToolchain keeps this equal to the
// repository's own go.mod, so a bump there cannot leave this behind.
const goModTemplate = `module MODULEPATH

go 1.25

toolchain go1.25.13
`

const protocolTemplate = `package main

// Node is a placeholder protocol: the first value a node hears becomes the
// value it adopts, and it passes that value on. Replace it with yours.
//
// It has a genuine safety property (every node that has adopted must have
// adopted the same value) and a genuine flaw: the property only holds while a
// single node proposes. protocol_test.go shows the engine finding that flaw.
type Node struct {
	id       uint32
	peers    []uint32
	adopted  uint64
	hasValue bool
	outbound []Message
}

// Message is your protocol's message type. The engine is generic over it and
// never looks inside; it only reads what Route and Digest expose.
type Message struct {
	From, To uint32
	Value    uint64
}

func NewNode(id uint32, size int) *Node {
	peers := make([]uint32, 0, size-1)
	for i := 1; i <= size; i++ {
		if uint32(i) != id {
			peers = append(peers, uint32(i))
		}
	}
	return &Node{id: id, peers: peers}
}

// Adopted reports the node's decision.
func (n *Node) Adopted() (uint64, bool) { return n.adopted, n.hasValue }

// Propose starts the protocol at this node.
func (n *Node) Propose(value uint64) { n.adopt(value) }

// Tick advances one unit of virtual time. This protocol has no timers; yours
// probably will.
func (n *Node) Tick() {}

// Step handles one delivered message.
func (n *Node) Step(m Message) { n.adopt(m.Value) }

// Drain hands the engine everything this node wants to send.
func (n *Node) Drain(dst []Message) []Message {
	dst = append(dst, n.outbound...)
	n.outbound = n.outbound[:0]
	return dst
}

func (n *Node) adopt(value uint64) {
	if n.hasValue {
		return
	}
	n.adopted, n.hasValue = value, true
	for _, peer := range n.peers {
		n.outbound = append(n.outbound, Message{From: n.id, To: peer, Value: value})
	}
}
`

const clusterTemplate = `package main

import (
	"fmt"

	"github.com/hunterinvariants/promtact/dst"
)

// Cluster is the entire integration with the engine: four methods for
// dst.Cluster and two for dst.Wire.
type Cluster struct {
	ids   []uint32
	nodes map[uint32]*Node
}

func New(count int) *Cluster {
	c := &Cluster{ids: make([]uint32, 0, count), nodes: make(map[uint32]*Node, count)}
	for i := 1; i <= count; i++ {
		id := uint32(i)
		c.ids = append(c.ids, id)
		c.nodes[id] = NewNode(id, count)
	}
	return c
}

// Nodes must return a stable order. Returning the range of a map here is the
// most common way to make a run irreproducible.
func (c *Cluster) Nodes() []uint32 { return c.ids }

func (c *Cluster) Tick(id uint32) { c.nodes[id].Tick() }

func (c *Cluster) Deliver(id uint32, m Message) { c.nodes[id].Step(m) }

func (c *Cluster) Drain(id uint32, dst []Message) []Message { return c.nodes[id].Drain(dst) }

func (c *Cluster) Route(m Message) (uint32, uint32) { return m.From, m.To }

// Digest feeds the execution trace. Include only values that are part of
// observable protocol behavior.
func (c *Cluster) Digest(m Message) (uint8, uint64) { return 0, m.Value }

// Node exposes one node for assertions. The engine never calls it.
func (c *Cluster) Node(id uint32) *Node { return c.nodes[id] }

// Propose starts the protocol at one node. Run the engine's Collect afterwards
// so the resulting messages enter the schedule.
func (c *Cluster) Propose(id uint32, value uint64) { c.nodes[id].Propose(value) }

// SafetyInvariants are the properties the engine checks after every step.
func (c *Cluster) SafetyInvariants() []dst.Invariant {
	return []dst.Invariant{
		dst.InvariantFunc{Label: "one adopted value", Fn: c.checkAgreement},
	}
}

func (c *Cluster) checkAgreement() error {
	var decided uint64
	var seen bool
	for _, id := range c.ids {
		value, ok := c.nodes[id].Adopted()
		if !ok {
			continue
		}
		if seen && value != decided {
			return fmt.Errorf("node %d adopted %d, another adopted %d", id, value, decided)
		}
		decided, seen = value, true
	}
	return nil
}

var (
	_ dst.Cluster[Message] = (*Cluster)(nil)
	_ dst.Wire[Message]    = (*Cluster)(nil)
)
`

const testTemplate = `package main

import (
	"errors"
	"testing"

	"github.com/hunterinvariants/promtact/dst"
	"github.com/hunterinvariants/promtact/dst/scenario"
)

func newEngine(t *testing.T, nodes int, config dst.Config) (*Cluster, *dst.Engine[Message]) {
	t.Helper()
	cluster := New(nodes)
	engine := dst.New[Message](config, cluster, cluster)
	engine.Watch(cluster.SafetyInvariants()...)
	return cluster, engine
}

// TestSingleProposerAgrees is the case the placeholder protocol handles.
func TestSingleProposerAgrees(t *testing.T) {
	for seed := int64(1); seed <= 200; seed++ {
		cluster, engine := newEngine(t, 5, dst.Config{Seed: seed, DropPermille: 30, MaxDelay: 4})
		cluster.Propose(1, 0xABCD)
		engine.Collect()

		if err := engine.RunChecked(300); err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		adopted := 0
		for _, id := range cluster.Nodes() {
			if _, ok := cluster.Node(id).Adopted(); ok {
				adopted++
			}
		}
		// Without this the run could pass with nobody having adopted anything,
		// which the safety property alone is perfectly happy with.
		if adopted == 0 {
			t.Fatalf("seed %d: no node adopted a value", seed)
		}
	}
}

// TestTwoProposersAreCaught is the interesting one. The placeholder protocol is
// not safe when two nodes propose different values, and this test requires the
// engine to say so. It is how you will find out that your own protocol is
// wrong, so it is worth reading before you delete it.
func TestTwoProposersAreCaught(t *testing.T) {
	for seed := int64(1); seed <= 200; seed++ {
		cluster, engine := newEngine(t, 5, dst.Config{Seed: seed, MaxDelay: 3})
		cluster.Propose(1, 0x1111)
		cluster.Propose(5, 0x2222)
		engine.Collect()

		err := engine.RunChecked(300)
		if err == nil {
			continue // this seed happened not to expose it
		}
		var violation *dst.Violation
		if !errors.As(err, &violation) {
			t.Fatalf("seed %d: unexpected error %v", seed, err)
		}
		// The violation carries a reproducible coordinate: same seed, same
		// step, same trace.
		t.Logf("seed %d: %q broke at step %d, trace %s",
			seed, violation.Invariant, violation.Step, violation.Trace)
		return
	}
	t.Fatal("two competing proposers never broke agreement; the engine is not being driven")
}

// TestScenarioFile shows that a run can be declared in a file rather than
// written into the test.
func TestScenarioFile(t *testing.T) {
	spec, err := scenario.Load("scenario.json")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	config, err := spec.EngineConfig()
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	injectors, err := spec.Injectors()
	if err != nil {
		t.Fatalf("injectors: %v", err)
	}

	cluster, engine := newEngine(t, spec.Nodes, config)
	engine.Inject(injectors...)
	cluster.Propose(1, 0xFEED)
	engine.Collect()

	if err := engine.RunChecked(spec.Steps); err != nil {
		t.Fatalf("%s: %v", spec.Name, err)
	}
	for _, injector := range injectors {
		// A fault that never fired means the scenario tested less than it says.
		// Note that a fault window has to overlap with when your protocol
		// actually sends: this placeholder broadcasts once at the start, so a
		// window opening at step 50 would find nothing to drop.
		if engine.InjectedDrops()[injector.Name()] == 0 {
			t.Fatalf("fault %q never fired", injector.Name())
		}
	}
}

// TestRunIsReproducible is the determinism check. A protocol that iterates a
// map or reads the clock fails here.
func TestRunIsReproducible(t *testing.T) {
	run := func() string {
		cluster, engine := newEngine(t, 5, dst.Config{Seed: 9, DropPermille: 60, MaxDelay: 5})
		cluster.Propose(1, 42)
		engine.Collect()
		engine.Run(200)
		return engine.TraceHash()
	}
	if a, b := run(), run(); a != b {
		t.Fatalf("trace differs across identical runs:\n %s\n %s", a, b)
	}
}
`

const scenarioTemplate = `{
  "name": "starter scenario",
  "seed": "0x1234",
  "nodes": 5,
  "steps": 400,
  "dropPermille": 20,
  "maxDelay": 4,
  "faults": [
    {
      "type": "split",
      "a": [1, 2],
      "b": [3, 4, 5],
      "start": 0,
      "end": 200
    }
  ]
}
`

const readmeTemplate = `# MODULEPATH

A starter project for testing a distributed protocol with the Promtact
deterministic engine.

    go mod tidy
    go test ./... -v
    go run .

The tests are where the work happens. ` + "`go run .`" + ` executes scenario.json once and
prints the trace hash and what each fault dropped, which is useful while you are
changing the protocol and want a single run to look at.

## What is here

| File | Role |
|---|---|
| main.go | Runs scenario.json once and reports the result. |
| protocol.go | The protocol. Replace this with yours. |
| cluster.go | The adapter: four methods for dst.Cluster, two for dst.Wire, plus your invariants. |
| protocol_test.go | Campaigns, including one that requires the engine to catch a real flaw. |
| scenario.json | A declared run: seed, topology, faults. |

## The placeholder protocol is deliberately flawed

The first value a node hears becomes the value it adopts. That is safe while
one node proposes and unsafe the moment two do. TestTwoProposersAreCaught
requires the engine to detect it, and prints the step and trace hash where
agreement broke.

Read that test before replacing the protocol. It is the shape of how you will
find out that your own protocol is wrong.

## Replacing the protocol

1. Rewrite protocol.go with your state, message type, and transitions.
2. Adjust cluster.go so Tick, Deliver, and Drain reach them. The six methods
   are the whole integration.
3. Replace checkAgreement with the property your protocol actually promises.
4. Write a mutation test: corrupt the state that property protects and require
   the invariant to fire. An invariant that never fires is indistinguishable
   from one that is never evaluated.

## Rules that will bite you

Every method the engine calls must be deterministic. Iterating a Go map where
the order affects behavior is the usual way to break it; TestRunIsReproducible
is the detector.

The invariants are safety properties. A protocol that does nothing at all
violates none of them, so assert progress separately.

See the Promtact documentation for the engine, the scenario format, and the
storage backend conformance suite.
`
