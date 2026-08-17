package raftcluster

import (
	"fmt"

	"github.com/hunterinvariants/promtact/dst"
	"github.com/hunterinvariants/promtact/raft"
)

// SafetyInvariants returns the Raft safety properties the qualified simulator
// checks in sim.CheckSafety, split into separately named properties so a
// violation report identifies which one broke.
//
// They are the reference example of the dst.Invariant contract: each closes
// over the cluster, inspects only its state, and iterates node identifiers in
// ascending order so that a violation message is itself deterministic.
func (c *Cluster) SafetyInvariants() []dst.Invariant {
	return []dst.Invariant{
		dst.InvariantFunc{Label: "election safety", Fn: c.checkElectionSafety},
		dst.InvariantFunc{Label: "index sanity", Fn: c.checkIndexSanity},
		dst.InvariantFunc{Label: "committed prefix", Fn: c.checkCommittedPrefix},
	}
}

// checkElectionSafety requires at most one leader per term.
func (c *Cluster) checkElectionSafety() error {
	leaders := make(map[uint64]uint32, len(c.ids))
	for _, id := range c.ids {
		node := c.nodes[id]
		if node.State != raft.Leader {
			continue
		}
		if prior, exists := leaders[node.Term]; exists && prior != id {
			return fmt.Errorf("two leaders in term %d: %d and %d", node.Term, prior, id)
		}
		leaders[node.Term] = id
	}
	return nil
}

// checkIndexSanity requires each node's base, applied, commit, and last indexes
// to stay in their permitted order.
func (c *Cluster) checkIndexSanity() error {
	for _, id := range c.ids {
		node := c.nodes[id]
		if node.Commit < node.BaseIndex || node.Commit > node.LastIndex() ||
			node.Applied < node.BaseIndex || node.Applied > node.Commit {
			return fmt.Errorf("node %d invalid indexes base=%d applied=%d commit=%d last=%d",
				id, node.BaseIndex, node.Applied, node.Commit, node.LastIndex())
		}
	}
	return nil
}

// checkCommittedPrefix requires every pair of nodes to agree on every entry
// both have committed and both still retain after compaction.
func (c *Cluster) checkCommittedPrefix() error {
	for i, a := range c.ids {
		for _, b := range c.ids[i+1:] {
			na, nb := c.nodes[a], c.nodes[b]
			first := max(na.BaseIndex, nb.BaseIndex)
			limit := min(na.Commit, nb.Commit)
			for index := first; index <= limit; index++ {
				ea, oka := na.EntryAt(index)
				eb, okb := nb.EntryAt(index)
				// The entry at the shared base is only comparable by term:
				// a compacted base carries no command payload.
				if !oka || !okb || ea.Term != eb.Term || (index > first && ea != eb) {
					return fmt.Errorf("nodes %d/%d disagree at committed index %d: a(term=%d command=%d commit=%d last=%d state=%d) b(term=%d command=%d commit=%d last=%d state=%d)",
						a, b, index, ea.Term, ea.Command, na.Commit, na.LastIndex(), na.State,
						eb.Term, eb.Command, nb.Commit, nb.LastIndex(), nb.State)
				}
			}
		}
	}
	return nil
}
