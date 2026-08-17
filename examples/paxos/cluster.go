package paxos

import "github.com/hunterinvariants/promtact/dst"

// Cluster wires Paxos to the engine. It is the whole adapter: four methods for
// dst.Cluster and two for dst.Wire. Everything else in this package is the
// protocol itself, which the engine never looks inside.
type Cluster struct {
	ids   []uint32
	nodes map[uint32]*Node
}

// New builds a cluster of count nodes with identifiers 1..count. Timeouts
// carry a stable per-node offset so that proposers do not retry in lockstep,
// which is a fixed choice rather than a random one and therefore keeps runs
// reproducible.
func New(count int) *Cluster {
	if count < 1 {
		panic("paxos: node count must be positive")
	}
	c := &Cluster{ids: make([]uint32, 0, count), nodes: make(map[uint32]*Node, count)}
	for i := 1; i <= count; i++ {
		id := uint32(i)
		c.ids = append(c.ids, id)
		c.nodes[id] = NewNode(id, count, 12+uint64(i)*3)
	}
	return c
}

// Nodes returns the identifiers in ascending order. The engine requires a
// stable order; returning the range of a map here would make every run differ.
func (c *Cluster) Nodes() []uint32 { return c.ids }

// Node exposes one node for assertions. The engine never calls it.
func (c *Cluster) Node(id uint32) *Node { return c.nodes[id] }

func (c *Cluster) Tick(id uint32) { c.nodes[id].Tick() }

func (c *Cluster) Deliver(id uint32, m Message) { c.nodes[id].Step(m) }

func (c *Cluster) Drain(id uint32, dst []Message) []Message { return c.nodes[id].Drain(dst) }

// Route tells the engine which link a message travels on.
func (c *Cluster) Route(m Message) (uint32, uint32) { return m.From, m.To }

// Digest contributes to the execution trace. Include only fields that are part
// of observable protocol behavior: the kind and the proposal number identify a
// delivery, while a wall-clock timestamp or a pointer would destroy
// reproducibility.
func (c *Cluster) Digest(m Message) (uint8, uint64) { return uint8(m.Kind), m.Num }

// Propose starts a round at one node. Callers must run the engine's Collect
// afterwards so the resulting messages enter the schedule.
func (c *Cluster) Propose(id uint32, value uint64) { c.nodes[id].Propose(value) }

// Adoptions reports how often a proposer in this cluster had to give up its
// own value for one already accepted elsewhere. Campaigns assert on it: a run
// in which it stays zero never reached the situation Paxos exists to handle,
// and any confidence drawn from it would be misplaced.
func (c *Cluster) Adoptions() int {
	total := 0
	for _, id := range c.ids {
		total += c.nodes[id].adopted
	}
	return total
}

// Decided reports whether any node has seen its proposal chosen.
func (c *Cluster) Decided() bool {
	for _, id := range c.ids {
		if c.nodes[id].Decided() {
			return true
		}
	}
	return false
}

// Chosen returns the value a quorum has accepted at some proposal number, and
// whether one exists. If Paxos is correct there is at most one such value for
// the whole run, which is what SafetyInvariants checks.
func (c *Cluster) Chosen() (uint64, bool) {
	values := c.chosenValues()
	if len(values) == 0 {
		return 0, false
	}
	return values[0], true
}

// chosenValues returns every distinct value that reached a quorum of
// acceptances at some proposal number, in first-seen order.
func (c *Cluster) chosenValues() []uint64 {
	counts := make(map[Acceptance]int)
	for _, id := range c.ids {
		// An acceptor may accept the same (num, val) only once, so counting
		// acceptances counts distinct acceptors.
		for _, a := range c.nodes[id].Acceptances() {
			counts[a]++
		}
	}
	quorum := len(c.ids)/2 + 1
	var found []uint64
	seen := make(map[uint64]bool)
	// Iterate the acceptance histories rather than the map so the result is
	// deterministic; map order would make failure messages vary between runs.
	for _, id := range c.ids {
		for _, a := range c.nodes[id].Acceptances() {
			if counts[a] >= quorum && !seen[a.Val] {
				seen[a.Val] = true
				found = append(found, a.Val)
			}
		}
	}
	return found
}

// Compile-time proof that the adapter satisfies both engine interfaces.
var (
	_ dst.Cluster[Message] = (*Cluster)(nil)
	_ dst.Wire[Message]    = (*Cluster)(nil)
)
