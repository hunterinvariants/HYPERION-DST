// Package storage defines the durable record format shared by the write-ahead
// log and its backends.
//
// Entry here is deliberately not raft.Entry. The consensus core keeps entries
// in a slice where position carries the index, so raft.Entry has no Index
// field; a durable record has no position and must carry its index explicitly.
// The two types describe the same command at different boundaries and are
// converted at the raftwal seam.
//
// The pluggable surfaces live next to the code that uses them, not here:
// wal.Device is the durable byte backend, raft.StableStore and
// raft.SnapshotStore are the consensus persistence boundary.
package storage

// Entry is the stable representation of one replicated command.
type Entry struct {
	Index     uint64
	Term      uint64
	Command   uint64
	Kind      uint8
	OldVoters uint64
	NewVoters uint64
	Operation uint8
	ClientID  uint64
	RequestID uint64
	Key       uint64
	Value     uint64
}
