package sim

import (
	"fmt"

	"github.com/hunterinvariants/HYPERION-DST/raft"
	"github.com/hunterinvariants/HYPERION-DST/storage/raftwal"
)

// CrashRestart discards all volatile node and in-flight network state, then
// reconstructs the node exclusively from its durable WAL image.
func (s *Simulator) CrashRestart(id uint32) error {
	node, ok := s.Nodes[id]
	if !ok {
		return fmt.Errorf("sim: unknown node %d", id)
	}
	rebootedDisk, err := s.disks[id].Crash(0)
	if err != nil {
		return err
	}
	store, err := raftwal.Open(rebootedDisk)
	if err != nil {
		return err
	}
	hard, entries := store.State()
	peers := append([]uint32(nil), node.Peers...)
	s.disks[id], s.stores[id] = rebootedDisk, store
	s.Nodes[id] = raft.NewRecoveredNode(id, peers, 8+uint64(id*2), store, hard, entries)

	kept := s.queue[:0]
	for _, event := range s.queue {
		if event.msg.From != id && event.msg.To != id {
			kept = append(kept, event)
		}
	}
	s.queue = kept
	return nil
}
