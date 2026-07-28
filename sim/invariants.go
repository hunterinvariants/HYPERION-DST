package sim

import (
	"fmt"

	"github.com/hunterinvariants/HYPERION-DST/raft"
)

// CheckSafety validates invariants that must hold at every simulator step.
func (s *Simulator) CheckSafety() error {
	leaders := make(map[uint64]uint32)
	for id, node := range s.Nodes {
		if node.State == raft.Leader {
			if prior, exists := leaders[node.Term]; exists && prior != id {
				return fmt.Errorf("two leaders in term %d: %d and %d", node.Term, prior, id)
			}
			leaders[node.Term] = id
		}
		if node.Commit < node.BaseIndex || node.Commit > node.LastIndex() ||
			node.Applied < node.BaseIndex || node.Applied > node.Commit {
			return fmt.Errorf("node %d invalid indexes base=%d applied=%d commit=%d last=%d",
				id, node.BaseIndex, node.Applied, node.Commit, node.LastIndex())
		}
	}
	for a, na := range s.Nodes {
		for b, nb := range s.Nodes {
			if b <= a {
				continue
			}
			first := max(na.BaseIndex, nb.BaseIndex)
			limit := min(na.Commit, nb.Commit)
			for index := first; index <= limit; index++ {
				ea, oka := na.EntryAt(index)
				eb, okb := nb.EntryAt(index)
				if !oka || !okb || ea.Term != eb.Term || (index > first && ea != eb) {
					return fmt.Errorf("nodes %d/%d disagree at committed index %d", a, b, index)
				}
			}
		}
	}
	return nil
}
