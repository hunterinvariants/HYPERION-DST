package raft

const outboundCapacity = 4096

type Node struct {
	ID          uint32
	Peers       []uint32
	State       State
	Term        uint64
	VotedFor    uint32
	Leader      uint32
	Log         []Entry
	Commit      uint64
	Applied     uint64
	timeout     uint64
	elapsed     uint64
	voteMask    uint64
	preVoteMask uint64
	preVoteTerm uint64
	next        map[uint32]uint64
	match       map[uint32]uint64
	outbound    []Message
	store       StableStore
	Faulted     bool
	readContext uint64
	readAckMask uint64
	readIndex   uint64
	readReady   bool
}

func NewNode(id uint32, peers []uint32, electionTimeout uint64) *Node {
	return NewNodeWithStore(id, peers, electionTimeout, &memoryStore{})
}

func NewNodeWithStore(id uint32, peers []uint32, electionTimeout uint64, store StableStore) *Node {
	seen := nodeBit(id)
	for _, peer := range peers {
		bit := nodeBit(peer)
		if seen&bit != 0 {
			panic("raft: duplicate node ID")
		}
		seen |= bit
	}
	return &Node{
		ID: id, Peers: append([]uint32(nil), peers...), State: Follower,
		Log: []Entry{{}}, timeout: electionTimeout,
		next:     make(map[uint32]uint64, len(peers)),
		match:    make(map[uint32]uint64, len(peers)),
		outbound: make([]Message, 0, outboundCapacity),
		store:    store,
	}
}

func (n *Node) Tick() {
	if n.Faulted {
		return
	}
	n.elapsed++
	if n.State == Leader {
		if n.elapsed >= 3 {
			n.elapsed = 0
			n.broadcastAppend()
		}
		return
	}
	if n.elapsed >= n.timeout {
		n.startPreVote()
	}
}

func (n *Node) startPreVote() {
	n.elapsed = 0
	n.preVoteTerm = n.Term + 1
	n.Leader = 0
	n.preVoteMask = nodeBit(n.ID)
	last := uint64(len(n.Log) - 1)
	for _, p := range n.Peers {
		n.send(Message{Type: MsgPreVote, From: n.ID, To: p, Term: n.preVoteTerm,
			LogIndex: last, LogTerm: n.Log[last].Term})
	}
	if n.quorumMask(n.preVoteMask) {
		n.startElection()
	}
}

func (n *Node) startElection() {
	nextTerm := n.Term + 1
	if !n.persistHardState(HardState{Term: nextTerm, VotedFor: n.ID}) {
		return
	}
	n.State, n.Term, n.VotedFor, n.elapsed = Candidate, nextTerm, n.ID, 0
	n.voteMask = nodeBit(n.ID)
	n.preVoteMask = 0
	last := uint64(len(n.Log) - 1)
	for _, p := range n.Peers {
		n.send(Message{Type: MsgRequestVote, From: n.ID, To: p, Term: n.Term,
			LogIndex: last, LogTerm: n.Log[last].Term})
	}
	if n.quorumMask(n.voteMask) {
		n.becomeLeader()
	}
}

func (n *Node) Step(m Message) {
	if n.Faulted {
		return
	}
	isPreVote := m.Type == MsgPreVote || m.Type == MsgPreVoteResponse
	if !isPreVote && m.Term > n.Term {
		if !n.persistHardState(HardState{Term: m.Term}) {
			return
		}
		n.State, n.Term, n.VotedFor, n.Leader, n.readReady = Follower, m.Term, 0, 0, false
	}
	switch m.Type {
	case MsgPreVote:
		n.onPreVote(m)
	case MsgPreVoteResponse:
		if n.State != Leader && n.isVoter(m.From) && m.Term == n.Term+1 && m.Term == n.preVoteTerm && !m.Reject {
			n.preVoteMask |= nodeBit(m.From)
			if n.quorumMask(n.preVoteMask) {
				n.startElection()
			}
		}
	case MsgRequestVote:
		n.onVote(m)
	case MsgRequestVoteResponse:
		if n.State == Candidate && n.isVoter(m.From) && m.Term == n.Term && !m.Reject {
			n.voteMask |= nodeBit(m.From)
			if n.quorumMask(n.voteMask) {
				n.becomeLeader()
			}
		}
	case MsgAppend:
		n.onAppend(m)
	case MsgAppendResponse:
		n.onAppendResponse(m)
	case MsgTimeoutNow:
		if n.State == Follower && m.Term == n.Term && m.From == n.Leader {
			n.startElection()
		}
	}
}

// TransferLeadership asks an up-to-date voter to start an immediate election.
// The transfer is rejected until the target has durably acknowledged the
// leader's complete log.
// StartReadIndex begins a quorum-confirmed linearizable read barrier. Only one
// read barrier is active at a time; callers provide a non-zero unique context.
func (n *Node) StartReadIndex(context uint64) bool {
	if n.State != Leader || n.Faulted || context == 0 || n.Commit == 0 || n.Log[n.Commit].Term != n.Term {
		return false
	}
	n.readContext, n.readAckMask = context, nodeBit(n.ID)
	n.readReady = n.quorumMask(n.readAckMask)
	if n.readReady {
		n.readIndex = n.Commit
	}
	for _, p := range n.Peers {
		n.appendToContext(p, context)
	}
	return true
}

// ReadIndex returns the commit index after the matching barrier reached quorum.
func (n *Node) ReadIndex(context uint64) (uint64, bool) {
	if n.State != Leader || context != n.readContext || !n.readReady {
		return 0, false
	}
	return n.readIndex, true
}
func (n *Node) TransferLeadership(target uint32) bool {
	if n.State != Leader || target == n.ID || !n.isVoter(target) {
		return false
	}
	last := uint64(len(n.Log) - 1)
	if n.match[target] < last {
		n.appendTo(target)
		return false
	}
	n.send(Message{Type: MsgTimeoutNow, From: n.ID, To: target, Term: n.Term})
	return true
}
func (n *Node) onPreVote(m Message) {
	last := uint64(len(n.Log) - 1)
	upToDate := m.LogTerm > n.Log[last].Term ||
		(m.LogTerm == n.Log[last].Term && m.LogIndex >= last)
	leaderActive := n.Leader != 0 && n.elapsed < n.timeout
	grant := n.isVoter(m.From) && m.Term >= n.Term+1 && upToDate && !leaderActive
	n.send(Message{Type: MsgPreVoteResponse, From: n.ID, To: m.From,
		Term: m.Term, Reject: !grant})
}
func (n *Node) onVote(m Message) {
	last := uint64(len(n.Log) - 1)
	upToDate := m.LogTerm > n.Log[last].Term ||
		(m.LogTerm == n.Log[last].Term && m.LogIndex >= last)
	grant := n.isVoter(m.From) && m.Term == n.Term && (n.VotedFor == 0 || n.VotedFor == m.From) && upToDate
	if grant {
		if !n.persistHardState(HardState{Term: n.Term, VotedFor: m.From}) {
			return
		}
		n.VotedFor, n.elapsed = m.From, 0
	}
	n.send(Message{Type: MsgRequestVoteResponse, From: n.ID, To: m.From,
		Term: n.Term, Reject: !grant})
}

func (n *Node) onAppend(m Message) {
	reject := m.Term < n.Term || m.LogIndex >= uint64(len(n.Log)) ||
		n.Log[m.LogIndex].Term != m.LogTerm
	if !reject {
		n.State, n.Leader, n.elapsed = Follower, m.From, 0
		if m.HasEntry {
			at := m.LogIndex + 1
			if at < uint64(len(n.Log)) && n.Log[at].Term != m.Entry.Term {
				n.Log = n.Log[:at]
			}
			if at == uint64(len(n.Log)) {
				if err := n.store.Append(at, m.Entry); err != nil {
					n.failStorage()
					return
				}
				n.Log = append(n.Log, m.Entry)
			}
		}
		if m.Commit > n.Commit {
			n.Commit = min(m.Commit, uint64(len(n.Log)-1))
		}
	}
	n.send(Message{Type: MsgAppendResponse, From: n.ID, To: m.From,
		Term: n.Term, Reject: reject, Match: uint64(len(n.Log) - 1), Context: m.Context})
}

func (n *Node) onAppendResponse(m Message) {
	if n.State == Leader && m.Term == n.Term && !m.Reject && m.Context != 0 && m.Context == n.readContext && n.isVoter(m.From) {
		n.readAckMask |= nodeBit(m.From)
		if n.quorumMask(n.readAckMask) {
			n.readIndex, n.readReady = n.Commit, true
		}
	}
	if n.State != Leader || m.Term != n.Term || m.Match >= uint64(len(n.Log)) {
		return
	}
	if m.Reject {
		if n.next[m.From] > 1 {
			n.next[m.From]--
		}
		n.appendTo(m.From)
		return
	}
	n.match[m.From], n.next[m.From] = m.Match, m.Match+1
	for idx := uint64(len(n.Log) - 1); idx > n.Commit; idx-- {
		if n.Log[idx].Term != n.Term {
			continue
		}
		count := 1
		for _, p := range n.Peers {
			if n.match[p] >= idx {
				count++
			}
		}
		if n.quorum(count) {
			n.Commit = idx
			n.broadcastAppend()
			break
		}
	}
	if n.next[m.From] < uint64(len(n.Log)) {
		n.appendTo(m.From)
	}
}

func (n *Node) Propose(command uint64) bool {
	if n.State != Leader || n.Faulted {
		return false
	}
	entry := Entry{Term: n.Term, Command: command}
	if err := n.store.Append(uint64(len(n.Log)), entry); err != nil {
		n.failStorage()
		return false
	}
	n.Log = append(n.Log, entry)
	n.broadcastAppend()
	return true
}

func (n *Node) becomeLeader() {
	n.State, n.Leader, n.elapsed = Leader, n.ID, 0
	last := uint64(len(n.Log))
	for _, p := range n.Peers {
		n.next[p], n.match[p] = last, 0
	}
	n.broadcastAppend()
}

func (n *Node) broadcastAppend() {
	for _, p := range n.Peers {
		n.appendTo(p)
	}
}

func (n *Node) appendTo(p uint32) {
	n.appendToContext(p, 0)
}

func (n *Node) appendToContext(p uint32, context uint64) {
	next := n.next[p]
	if next > uint64(len(n.Log)) {
		next = uint64(len(n.Log))
		n.next[p] = next
	}
	if next == 0 {
		next = 1
	}
	prev := next - 1
	m := Message{Type: MsgAppend, From: n.ID, To: p, Term: n.Term,
		LogIndex: prev, LogTerm: n.Log[prev].Term, Commit: n.Commit, Context: context}
	if next < uint64(len(n.Log)) {
		m.Entry, m.HasEntry = n.Log[next], true
	}
	n.send(m)
}

func (n *Node) send(m Message) {
	if len(n.outbound) == cap(n.outbound) {
		panic("raft outbound capacity exceeded")
	}
	n.outbound = append(n.outbound, m)
}

func (n *Node) Drain(dst []Message) []Message {
	dst = append(dst, n.outbound...)
	n.outbound = n.outbound[:0]
	return dst
}

func (n *Node) Apply() []uint64 {
	out := make([]uint64, 0, n.Commit-n.Applied)
	for n.Applied < n.Commit {
		n.Applied++
		out = append(out, n.Log[n.Applied].Command)
	}
	return out
}

func (n *Node) quorum(v int) bool { return v >= (len(n.Peers)+1)/2+1 }

func (n *Node) quorumMask(mask uint64) bool {
	count := 0
	for id := range uint32(64) {
		if mask&(uint64(1)<<id) != 0 {
			count++
		}
	}
	return n.quorum(count)
}

func (n *Node) isVoter(id uint32) bool {
	if id == n.ID {
		return true
	}
	for _, peer := range n.Peers {
		if peer == id {
			return true
		}
	}
	return false
}

func nodeBit(id uint32) uint64 {
	if id == 0 || id > 64 {
		panic("raft: node IDs must be in 1..64")
	}
	return uint64(1) << (id - 1)
}
func (n *Node) persistHardState(h HardState) bool {
	if err := n.store.SaveHardState(h); err != nil {
		n.failStorage()
		return false
	}
	return true
}

func (n *Node) failStorage() {
	n.Faulted = true
	n.State = Follower
	n.Leader = 0
	n.outbound = n.outbound[:0]
}
