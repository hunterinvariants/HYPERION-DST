package raft

// NewRecoveredNode reconstructs volatile Raft state from a verified durable
// hard state and log. Leaders never survive a restart.
func NewRecoveredNode(
	id uint32,
	peers []uint32,
	electionTimeout uint64,
	store StableStore,
	hard HardState,
	log []Entry,
) *Node {
	n := NewNodeWithStore(id, peers, electionTimeout, store)
	n.Term = hard.Term
	n.VotedFor = hard.VotedFor
	n.Commit = hard.Commit
	if len(log) > 0 {
		n.Log = append(n.Log[:0], log...)
	}
	if len(n.Log) == 0 {
		n.Log = append(n.Log, Entry{})
	}
	if n.Commit > n.lastIndex() {
		n.failStorage()
		return n
	}
	if !n.restoreConfigurationState() {
		n.failStorage()
		return n
	}
	return n
}

// NewRecoveredNodeWithSnapshot restores the absolute compacted base before
// replaying the verified WAL suffix.
func NewRecoveredNodeWithSnapshot(
	id uint32,
	peers []uint32,
	electionTimeout uint64,
	store StableStore,
	hard HardState,
	snapshot Snapshot,
	suffix []Entry,
) *Node {
	n := NewNodeWithStore(id, peers, electionTimeout, store)
	n.Term, n.VotedFor = hard.Term, hard.VotedFor
	n.BaseIndex, n.BaseTerm = snapshot.LastIndex, snapshot.LastTerm
	n.snapshot = Snapshot{LastIndex: snapshot.LastIndex, LastTerm: snapshot.LastTerm,
		State: append([]byte(nil), snapshot.State...), OldVoters: snapshot.OldVoters, NewVoters: snapshot.NewVoters}
	n.votersOld, n.votersNew = snapshot.OldVoters, snapshot.NewVoters
	if n.votersOld == 0 {
		n.failStorage()
		return n
	}
	n.ensurePeers(n.votersOld | n.votersNew)
	n.Log = []Entry{{Term: snapshot.LastTerm}}
	n.Log = append(n.Log, suffix...)
	n.Commit = max(hard.Commit, snapshot.LastIndex)
	n.Applied = snapshot.LastIndex
	if n.Commit > n.lastIndex() {
		n.failStorage()
		return n
	}
	if !n.restoreConfigurationState() {
		n.failStorage()
		return n
	}
	return n
}
