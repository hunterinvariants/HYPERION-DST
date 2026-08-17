// Package raftcluster adapts the Promtact Raft core to the dst engine.
//
// It is the reference implementation of dst.Cluster and dst.Wire: a protocol
// that wants deterministic testing supplies its own equivalent of this file
// and reuses the engine unchanged.
package raftcluster

import (
	"fmt"

	"github.com/hunterinvariants/promtact/raft"
	"github.com/hunterinvariants/promtact/storage/raftwal"
	"github.com/hunterinvariants/promtact/storage/wal"
)

// Cluster holds one Raft node per identifier, each backed by its own
// checksummed WAL over a deterministic in-memory device.
type Cluster struct {
	ids    []uint32
	nodes  map[uint32]*raft.Node
	stores map[uint32]*stableStore
	disks  map[uint32]*wal.MemoryDevice
}

// New builds a cluster of count nodes with identifiers 1..count, each peered
// with all others. Election timeouts carry a stable per-node offset, which
// prevents perpetual split votes without introducing nondeterminism.
func New(count int) *Cluster {
	if count < 1 {
		panic("raftcluster: node count must be positive")
	}
	c := &Cluster{
		ids:    make([]uint32, 0, count),
		nodes:  make(map[uint32]*raft.Node, count),
		stores: make(map[uint32]*stableStore, count),
		disks:  make(map[uint32]*wal.MemoryDevice, count),
	}
	for i := 1; i <= count; i++ {
		id := uint32(i)
		peers := make([]uint32, 0, count-1)
		for j := 1; j <= count; j++ {
			if j != i {
				peers = append(peers, uint32(j))
			}
		}
		disk := wal.NewMemoryDevice(nil)
		store, err := raftwal.Open(disk)
		if err != nil {
			panic(err)
		}
		stable := &stableStore{wal: store}
		c.ids = append(c.ids, id)
		c.disks[id], c.stores[id] = disk, stable
		c.nodes[id] = raft.NewNodeWithStore(id, peers, electionTimeout(id), stable)
	}
	return c
}

func electionTimeout(id uint32) uint64 { return 8 + uint64(id*2) }

// Nodes returns the node identifiers in ascending order.
func (c *Cluster) Nodes() []uint32 { return c.ids }

// Node exposes one node for assertions. The engine never calls it.
func (c *Cluster) Node(id uint32) *raft.Node { return c.nodes[id] }

// Tick advances one node by one unit of virtual time.
func (c *Cluster) Tick(id uint32) { c.nodes[id].Tick() }

// Deliver hands one message to its destination node.
func (c *Cluster) Deliver(id uint32, msg raft.Message) { c.nodes[id].Step(msg) }

// Drain moves a node's outbound messages into dst.
func (c *Cluster) Drain(id uint32, dst []raft.Message) []raft.Message {
	return c.nodes[id].Drain(dst)
}

// Route reports the endpoints of a Raft message.
func (c *Cluster) Route(msg raft.Message) (uint32, uint32) { return msg.From, msg.To }

// Digest exposes the message type and term to the execution trace.
func (c *Cluster) Digest(msg raft.Message) (uint8, uint64) { return uint8(msg.Type), msg.Term }

// Propose submits a command to whichever node accepts it, and reports whether
// one did. Callers must run the engine's Collect afterwards so the resulting
// messages enter the schedule.
func (c *Cluster) Propose(command uint64) bool {
	for _, id := range c.ids {
		if c.nodes[id].Propose(command) {
			return true
		}
	}
	return false
}

// ProposeJoint submits a joint configuration to whichever node accepts it.
// Callers must run the engine's Collect afterwards.
func (c *Cluster) ProposeJoint(voters []uint32) bool {
	for _, id := range c.ids {
		if c.nodes[id].ProposeJoint(voters) {
			return true
		}
	}
	return false
}

// ProposeFinal submits the final configuration that leaves a joint transition.
// Callers must run the engine's Collect afterwards.
func (c *Cluster) ProposeFinal() bool {
	for _, id := range c.ids {
		if c.nodes[id].ProposeFinal() {
			return true
		}
	}
	return false
}

// Compact snapshots a node at its applied index and reports whether anything
// was compacted. The caller supplies the deterministic state-machine image that
// index represents. Compaction produces no messages, so no Collect is needed.
func (c *Cluster) Compact(id uint32, state []byte) bool {
	node, ok := c.nodes[id]
	if !ok || node.Applied <= node.BaseIndex {
		return false
	}
	return node.Compact(node.Applied, state)
}

// Leader returns the current leader, or zero if there is none.
func (c *Cluster) Leader() uint32 {
	for _, id := range c.ids {
		if c.nodes[id].State == raft.Leader {
			return id
		}
	}
	return 0
}

// Restart discards all volatile state for a node and reconstructs it
// exclusively from its durable WAL image. The caller must also isolate the
// node in the engine, which discards its in-flight messages.
func (c *Cluster) Restart(id uint32) error {
	node, ok := c.nodes[id]
	if !ok {
		return fmt.Errorf("raftcluster: unknown node %d", id)
	}
	rebootedDisk, err := c.disks[id].Crash(0)
	if err != nil {
		return err
	}
	store, err := raftwal.Open(rebootedDisk)
	if err != nil {
		return err
	}
	hard, base, entries := store.StateWithBase()
	peers := append([]uint32(nil), node.Peers...)
	stable := &stableStore{wal: store, snapshot: c.stores[id].snapshot}
	c.disks[id], c.stores[id] = rebootedDisk, stable
	if base != 0 {
		if stable.snapshot.LastIndex != base {
			return fmt.Errorf("raftcluster: snapshot/WAL base mismatch %d != %d",
				stable.snapshot.LastIndex, base)
		}
		c.nodes[id] = raft.NewRecoveredNodeWithSnapshot(
			id, peers, electionTimeout(id), stable, hard, stable.snapshot, entries[1:])
		return nil
	}
	c.nodes[id] = raft.NewRecoveredNode(id, peers, electionTimeout(id), stable, hard, entries)
	return nil
}
