package sim

import (
	"fmt"

	"github.com/hunterinvariants/hyperion/raft"
	"github.com/hunterinvariants/hyperion/storage/raftwal"
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
	hard, base, entries := store.StateWithBase()
	peers := append([]uint32(nil), node.Peers...)
	stable := &stableStore{wal: store, snapshot: s.stores[id].snapshot}
	s.disks[id], s.stores[id] = rebootedDisk, stable
	if base != 0 {
		if stable.snapshot.LastIndex != base {
			return fmt.Errorf("sim: snapshot/WAL base mismatch %d != %d", stable.snapshot.LastIndex, base)
		}
		s.Nodes[id] = raft.NewRecoveredNodeWithSnapshot(
			id, peers, 8+uint64(id*2), stable, hard, stable.snapshot, entries[1:])
	} else {
		s.Nodes[id] = raft.NewRecoveredNode(id, peers, 8+uint64(id*2), stable, hard, entries)
	}

	kept := s.queue[:0]
	for _, event := range s.queue {
		if event.msg.From != id && event.msg.To != id {
			kept = append(kept, event)
		}
	}
	s.queue = kept
	return nil
}
