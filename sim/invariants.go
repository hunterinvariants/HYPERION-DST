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
		if node.Commit >= uint64(len(node.Log)) || node.Applied > node.Commit {
			return fmt.Errorf("node %d invalid indexes applied=%d commit=%d log=%d",
				id, node.Applied, node.Commit, len(node.Log))
		}
	}
	for a, na := range s.Nodes {
		for b, nb := range s.Nodes {
			if b <= a {
				continue
			}
			limit := min(na.Commit, nb.Commit)
			for index := uint64(1); index <= limit; index++ {
				if na.Log[index] != nb.Log[index] {
					return fmt.Errorf("nodes %d/%d disagree at committed index %d", a, b, index)
				}
			}
		}
	}
	return nil
}
