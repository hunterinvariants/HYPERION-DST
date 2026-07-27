package raft

const outboundCapacity = 4096

type Node struct {
	ID       uint32
	Peers    []uint32
	State    State
	Term     uint64
	VotedFor uint32
	Leader   uint32
	Log      []Entry
	Commit   uint64
	Applied  uint64
	timeout  uint64
	elapsed  uint64
	votes    int
	next     map[uint32]uint64
	match    map[uint32]uint64
	outbound []Message
}

func NewNode(id uint32, peers []uint32, electionTimeout uint64) *Node {
	return &Node{
		ID: id, Peers: append([]uint32(nil), peers...), State: Follower,
		Log: []Entry{{}}, timeout: electionTimeout,
		next:     make(map[uint32]uint64, len(peers)),
		match:    make(map[uint32]uint64, len(peers)),
		outbound: make([]Message, 0, outboundCapacity),
	}
}

func (n *Node) Tick() {
	n.elapsed++
	if n.State == Leader {
		if n.elapsed >= 3 {
			n.elapsed = 0
			n.broadcastAppend()
		}
		return
	}
	if n.elapsed >= n.timeout {
		n.startElection()
	}
}

func (n *Node) startElection() {
	n.State, n.Term, n.VotedFor, n.votes, n.elapsed = Candidate, n.Term+1, n.ID, 1, 0
	last := uint64(len(n.Log) - 1)
	for _, p := range n.Peers {
		n.send(Message{Type: MsgRequestVote, From: n.ID, To: p, Term: n.Term,
			LogIndex: last, LogTerm: n.Log[last].Term})
	}
	if n.quorum(n.votes) {
		n.becomeLeader()
	}
}

func (n *Node) Step(m Message) {
	if m.Term > n.Term {
		n.State, n.Term, n.VotedFor, n.Leader = Follower, m.Term, 0, 0
	}
	switch m.Type {
	case MsgRequestVote:
		n.onVote(m)
	case MsgRequestVoteResponse:
		if n.State == Candidate && m.Term == n.Term && !m.Reject {
			n.votes++
			if n.quorum(n.votes) {
				n.becomeLeader()
			}
		}
	case MsgAppend:
		n.onAppend(m)
	case MsgAppendResponse:
		n.onAppendResponse(m)
	}
}

func (n *Node) onVote(m Message) {
	last := uint64(len(n.Log) - 1)
	upToDate := m.LogTerm > n.Log[last].Term ||
		(m.LogTerm == n.Log[last].Term && m.LogIndex >= last)
	grant := m.Term == n.Term && (n.VotedFor == 0 || n.VotedFor == m.From) && upToDate
	if grant {
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
				n.Log = append(n.Log, m.Entry)
			}
		}
		if m.Commit > n.Commit {
			n.Commit = min(m.Commit, uint64(len(n.Log)-1))
		}
	}
	n.send(Message{Type: MsgAppendResponse, From: n.ID, To: m.From,
		Term: n.Term, Reject: reject, Match: uint64(len(n.Log) - 1)})
}

func (n *Node) onAppendResponse(m Message) {
	if n.State != Leader || m.Term != n.Term {
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
	if n.State != Leader {
		return false
	}
	n.Log = append(n.Log, Entry{Term: n.Term, Command: command})
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
	next := n.next[p]
	if next == 0 {
		next = 1
	}
	prev := next - 1
	m := Message{Type: MsgAppend, From: n.ID, To: p, Term: n.Term,
		LogIndex: prev, LogTerm: n.Log[prev].Term, Commit: n.Commit}
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
