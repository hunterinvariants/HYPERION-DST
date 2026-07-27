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
	if len(log) > 0 {
		n.Log = append(n.Log[:0], log...)
	}
	if len(n.Log) == 0 {
		n.Log = append(n.Log, Entry{})
	}
	return n
}
