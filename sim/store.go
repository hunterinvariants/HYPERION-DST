package sim

import (
	"github.com/hunterinvariants/promtact/raft"
	"github.com/hunterinvariants/promtact/storage/raftwal"
)

// stableStore keeps simulator snapshots in deterministic memory while the WAL
// retains its checksummed crash/replay behavior.
type stableStore struct {
	wal      *raftwal.Store
	snapshot raft.Snapshot
}

func (s *stableStore) SaveHardState(h raft.HardState) error {
	return s.wal.SaveHardState(h)
}

func (s *stableStore) Append(index uint64, entry raft.Entry) error {
	return s.wal.Append(index, entry)
}

func (s *stableStore) SaveSnapshot(snapshot raft.Snapshot) error {
	snapshot.State = append([]byte(nil), snapshot.State...)
	s.snapshot = snapshot
	return nil
}

func (s *stableStore) CompactLog(index uint64) error {
	return s.wal.CompactLog(index)
}
