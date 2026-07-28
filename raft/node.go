package raft

const outboundCapacity = 4096

type Node struct {
	ID            uint32
	Peers         []uint32
	State         State
	Term          uint64
	VotedFor      uint32
	Leader        uint32
	Log           []Entry
	BaseIndex     uint64
	BaseTerm      uint64
	Commit        uint64
	Applied       uint64
	timeout       uint64
	elapsed       uint64
	voteMask      uint64
	preVoteMask   uint64
	preVoteTerm   uint64
	next          map[uint32]uint64
	match         map[uint32]uint64
	outbound      []Message
	store         StableStore
	Faulted       bool
	storageErr    error
	readContext   uint64
	readAckMask   uint64
	readIndex     uint64
	readReady     bool
	snapshot      Snapshot
	votersOld     uint64
	votersNew     uint64
	pendingConfig uint64
}

func NewNode(id uint32, peers []uint32, electionTimeout uint64) *Node {
	return NewNodeWithStore(id, peers, electionTimeout, &memoryStore{})
}

func NewNodeWithStore(id uint32, peers []uint32, electionTimeout uint64, store StableStore) *Node {
	seen := nodeBit(id)
	voters := seen
	for _, peer := range peers {
		bit := nodeBit(peer)
		if seen&bit != 0 {
			panic("raft: duplicate node ID")
		}
		seen |= bit
		voters |= bit
	}
	return &Node{
		ID: id, Peers: append([]uint32(nil), peers...), State: Follower,
		Log: []Entry{{}}, timeout: electionTimeout,
		votersOld: voters,
		next:      make(map[uint32]uint64, len(peers)),
		match:     make(map[uint32]uint64, len(peers)),
		outbound:  make([]Message, 0, outboundCapacity),
		store:     store,
	}
}

// VoterMasks returns the committed C_old and optional C_new bitsets.
func (n *Node) VoterMasks() (uint64, uint64) { return n.votersOld, n.votersNew }

// LastIndex returns the absolute index of the final retained log entry.
func (n *Node) LastIndex() uint64 { return n.lastIndex() }

// EntryAt returns an entry by absolute index, including the compacted base.
func (n *Node) EntryAt(index uint64) (Entry, bool) {
	offset, ok := n.offset(index)
	if !ok {
		return Entry{}, false
	}
	return n.Log[offset], true
}
func (n *Node) lastIndex() uint64 {
	return n.BaseIndex + uint64(len(n.Log)) - 1
}

func (n *Node) offset(index uint64) (uint64, bool) {
	if index < n.BaseIndex || index > n.lastIndex() {
		return 0, false
	}
	return index - n.BaseIndex, true
}

func (n *Node) termAt(index uint64) (uint64, bool) {
	offset, ok := n.offset(index)
	if !ok {
		return 0, false
	}
	return n.Log[offset].Term, true
}

func (n *Node) termAtMust(index uint64) uint64 {
	term, ok := n.termAt(index)
	if !ok {
		panic("raft: log index out of range")
	}
	return term
}

func (n *Node) entryAt(index uint64) Entry {
	offset, ok := n.offset(index)
	if !ok {
		panic("raft: log index out of range")
	}
	return n.Log[offset]
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
	last := n.lastIndex()
	for _, p := range n.Peers {
		n.send(Message{Type: MsgPreVote, From: n.ID, To: p, Term: n.preVoteTerm,
			LogIndex: last, LogTerm: n.termAtMust(last)})
	}
	if n.quorumMask(n.preVoteMask) {
		n.startElection()
	}
}

func (n *Node) startElection() {
	nextTerm := n.Term + 1
	if !n.persistHardState(HardState{Term: nextTerm, VotedFor: n.ID, Commit: n.Commit}) {
		return
	}
	n.State, n.Term, n.VotedFor, n.elapsed = Candidate, nextTerm, n.ID, 0
	n.voteMask = nodeBit(n.ID)
	n.preVoteMask = 0
	last := n.lastIndex()
	for _, p := range n.Peers {
		n.send(Message{Type: MsgRequestVote, From: n.ID, To: p, Term: n.Term,
			LogIndex: last, LogTerm: n.termAtMust(last)})
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
		if !n.persistHardState(HardState{Term: m.Term, Commit: n.Commit}) {
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
	case MsgInstallSnapshot:
		n.onInstallSnapshot(m)
	case MsgInstallSnapshotResponse:
		n.onInstallSnapshotResponse(m)
	case MsgTimeoutNow:
		if n.State == Follower && m.Term == n.Term && m.From == n.Leader {
			n.startElection()
		}
	}
}

// Compact publishes a durable state-machine snapshot before discarding its
// covered log prefix. The entry at index remains as the compacted-base term.
func (n *Node) Compact(index uint64, state []byte) bool {
	if index <= n.BaseIndex || index > n.Applied {
		return false
	}
	term, ok := n.termAt(index)
	if !ok {
		return false
	}
	store, ok := n.store.(SnapshotStore)
	if !ok {
		n.failStorage()
		return false
	}
	snapshot := Snapshot{LastIndex: index, LastTerm: term, State: append([]byte(nil), state...), OldVoters: n.votersOld, NewVoters: n.votersNew}
	if err := store.SaveSnapshot(snapshot); err != nil {
		n.failStorageWith(err)
		return false
	}
	if err := store.CompactLog(index); err != nil {
		n.failStorageWith(err)
		return false
	}
	offset, _ := n.offset(index)
	n.Log = append([]Entry(nil), n.Log[offset:]...)
	n.BaseIndex, n.BaseTerm, n.snapshot = index, term, snapshot
	return true
}

func (n *Node) onInstallSnapshot(m Message) {
	reject := m.Term < n.Term || m.SnapshotIndex < n.BaseIndex || m.SnapshotOld == 0 ||
		(m.SnapshotIndex == n.BaseIndex && m.SnapshotTerm != n.BaseTerm)
	if !reject && m.SnapshotIndex > n.BaseIndex {
		store, ok := n.store.(SnapshotStore)
		if !ok {
			n.failStorage()
			return
		}
		snapshot := Snapshot{LastIndex: m.SnapshotIndex, LastTerm: m.SnapshotTerm,
			State: append([]byte(nil), m.Snapshot...), OldVoters: m.SnapshotOld, NewVoters: m.SnapshotNew}
		if err := store.SaveSnapshot(snapshot); err != nil {
			n.failStorageWith(err)
			return
		}
		if err := store.CompactLog(snapshot.LastIndex); err != nil {
			n.failStorageWith(err)
			return
		}
		if term, present := n.termAt(snapshot.LastIndex); present && term == snapshot.LastTerm {
			offset, _ := n.offset(snapshot.LastIndex)
			n.Log = append([]Entry(nil), n.Log[offset:]...)
		} else {
			n.Log = []Entry{{Term: snapshot.LastTerm}}
		}
		n.BaseIndex, n.BaseTerm, n.snapshot = snapshot.LastIndex, snapshot.LastTerm, snapshot
		n.votersOld, n.votersNew = snapshot.OldVoters, snapshot.NewVoters
		n.Commit = max(n.Commit, snapshot.LastIndex)
		n.Applied = max(n.Applied, snapshot.LastIndex)
	}
	if !reject {
		n.State, n.Leader, n.elapsed = Follower, m.From, 0
	}
	n.send(Message{Type: MsgInstallSnapshotResponse, From: n.ID, To: m.From,
		Term: n.Term, Reject: reject, Match: n.BaseIndex})
}

func (n *Node) onInstallSnapshotResponse(m Message) {
	if n.State != Leader || m.Term != n.Term || m.Reject || !n.isVoter(m.From) {
		return
	}
	if m.Match > n.match[m.From] {
		n.match[m.From], n.next[m.From] = m.Match, m.Match+1
	}
	if n.next[m.From] <= n.lastIndex() {
		n.appendTo(m.From)
	}
}

// StartReadIndex begins a quorum-confirmed linearizable read barrier. Only one
// read barrier is active at a time; callers provide a non-zero unique context.
func (n *Node) StartReadIndex(context uint64) bool {
	if n.State != Leader || n.Faulted || context == 0 || n.Commit == 0 || n.termAtMust(n.Commit) != n.Term {
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

// TransferLeadership asks an up-to-date voter to start an immediate election.
// The transfer is rejected until the target durably acknowledges the full log.
func (n *Node) TransferLeadership(target uint32) bool {
	if n.State != Leader || target == n.ID || !n.isVoter(target) {
		return false
	}
	last := n.lastIndex()
	if n.match[target] < last {
		n.appendTo(target)
		return false
	}
	n.send(Message{Type: MsgTimeoutNow, From: n.ID, To: target, Term: n.Term})
	return true
}
func (n *Node) onPreVote(m Message) {
	last := n.lastIndex()
	upToDate := m.LogTerm > n.termAtMust(last) ||
		(m.LogTerm == n.termAtMust(last) && m.LogIndex >= last)
	leaderActive := n.Leader != 0 && n.elapsed < n.timeout
	grant := n.isVoter(m.From) && m.Term >= n.Term+1 && upToDate && !leaderActive
	n.send(Message{Type: MsgPreVoteResponse, From: n.ID, To: m.From,
		Term: m.Term, Reject: !grant})
}
func (n *Node) onVote(m Message) {
	last := n.lastIndex()
	upToDate := m.LogTerm > n.termAtMust(last) ||
		(m.LogTerm == n.termAtMust(last) && m.LogIndex >= last)
	grant := n.isVoter(m.From) && m.Term == n.Term && (n.VotedFor == 0 || n.VotedFor == m.From) && upToDate
	if grant {
		if !n.persistHardState(HardState{Term: n.Term, VotedFor: m.From, Commit: n.Commit}) {
			return
		}
		n.VotedFor, n.elapsed = m.From, 0
	}
	n.send(Message{Type: MsgRequestVoteResponse, From: n.ID, To: m.From,
		Term: n.Term, Reject: !grant})
}

func (n *Node) validNewEntry(entry Entry) bool {
	switch entry.Kind {
	case EntryNormal:
		return entry.OldVoters == 0 && entry.NewVoters == 0
	case EntryJointConfig:
		return n.pendingConfig == 0 && n.votersNew == 0 &&
			entry.OldVoters == n.votersOld && entry.OldVoters != 0 && entry.NewVoters != 0
	case EntryFinalConfig:
		return n.pendingConfig == 0 && n.votersNew != 0 &&
			entry.OldVoters == n.votersNew && entry.NewVoters == 0
	default:
		return false
	}
}
func (n *Node) onAppend(m Message) {
	prevTerm, present := n.termAt(m.LogIndex)
	reject := m.Term < n.Term || !present || prevTerm != m.LogTerm
	if !reject && m.HasEntry && m.LogIndex+1 == n.lastIndex()+1 && !n.validNewEntry(m.Entry) {
		reject = true
	}
	if !reject {
		n.State, n.Leader, n.elapsed = Follower, m.From, 0
		if m.HasEntry {
			at := m.LogIndex + 1
			if offset, ok := n.offset(at); ok && n.Log[offset].Term != m.Entry.Term {
				n.Log = n.Log[:offset]
				n.rebuildPendingConfig()
			}
			if at == n.lastIndex()+1 {
				if err := n.store.Append(at, m.Entry); err != nil {
					n.failStorageWith(err)
					return
				}
				n.Log = append(n.Log, m.Entry)
				if m.Entry.Kind == EntryJointConfig || m.Entry.Kind == EntryFinalConfig {
					n.pendingConfig = at
					n.ensurePeers(m.Entry.OldVoters | m.Entry.NewVoters)
				}
			}
		}
		if m.Commit > n.Commit {
			previous := n.Commit
			commit := min(m.Commit, n.lastIndex())
			if !n.persistHardState(HardState{Term: n.Term, VotedFor: n.VotedFor, Commit: commit}) {
				return
			}
			n.Commit = commit
			n.applyCommittedConfigurations(previous+1, n.Commit)
		}
	}
	n.send(Message{Type: MsgAppendResponse, From: n.ID, To: m.From,
		Term: n.Term, Reject: reject, Match: n.lastIndex(), Context: m.Context})
}

func (n *Node) onAppendResponse(m Message) {
	if n.State == Leader && m.Term == n.Term && !m.Reject && m.Context != 0 && m.Context == n.readContext && n.isVoter(m.From) {
		n.readAckMask |= nodeBit(m.From)
		if n.quorumMask(n.readAckMask) {
			n.readIndex, n.readReady = n.Commit, true
		}
	}
	if n.State != Leader || m.Term != n.Term || m.Match >= n.lastIndex()+1 {
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
	for idx := n.lastIndex(); idx > n.Commit; idx-- {
		if n.termAtMust(idx) != n.Term {
			continue
		}
		acked := nodeBit(n.ID)
		for _, p := range n.Peers {
			if n.match[p] >= idx {
				acked |= nodeBit(p)
			}
		}
		if n.canCommitIndex(idx, acked) {
			if !n.persistHardState(HardState{Term: n.Term, VotedFor: n.VotedFor, Commit: idx}) {
				return
			}
			previous := n.Commit
			n.Commit = idx
			n.applyCommittedConfigurations(previous+1, n.Commit)
			if n.State == Leader {
				n.broadcastAppend()
			}
			break
		}
	}
	if n.State != Leader {
		return
	}
	if n.next[m.From] < n.lastIndex()+1 {
		n.appendTo(m.From)
	}
}

func voterMask(ids []uint32) (uint64, bool) {
	if len(ids) == 0 {
		return 0, false
	}
	var mask uint64
	for _, id := range ids {
		bit := nodeBit(id)
		if mask&bit != 0 {
			return 0, false
		}
		mask |= bit
	}
	return mask, true
}

// ProposeJoint appends C_old,new. Only one membership transition may be in
// flight. A leader removed by C_new steps down when the final entry commits.
func (n *Node) ProposeJoint(newVoters []uint32) bool {
	newMask, ok := voterMask(newVoters)
	if !ok || n.State != Leader || n.Faulted || n.votersNew != 0 || n.pendingConfig != 0 {
		return false
	}
	n.ensurePeers(newMask)
	entry := Entry{Term: n.Term, Kind: EntryJointConfig,
		OldVoters: n.votersOld, NewVoters: newMask}
	if !n.appendLocal(entry) {
		return false
	}
	n.pendingConfig = n.lastIndex()
	n.broadcastAppend()
	return true
}

// ProposeFinal appends C_new after the joint entry has committed. The final
// entry itself is committed under both old and new majorities.
func (n *Node) ProposeFinal() bool {
	if n.State != Leader || n.Faulted || n.votersNew == 0 || n.pendingConfig != 0 {
		return false
	}
	entry := Entry{Term: n.Term, Kind: EntryFinalConfig, OldVoters: n.votersNew}
	if !n.appendLocal(entry) {
		return false
	}
	n.pendingConfig = n.lastIndex()
	n.broadcastAppend()
	return true
}

func (n *Node) appendLocal(entry Entry) bool {
	if err := n.store.Append(n.lastIndex()+1, entry); err != nil {
		n.failStorageWith(err)
		return false
	}
	n.Log = append(n.Log, entry)
	return true
}

func (n *Node) ensurePeers(mask uint64) {
	for id := uint32(1); id <= 64; id++ {
		if id == n.ID || mask&nodeBit(id) == 0 {
			continue
		}
		found := false
		for _, peer := range n.Peers {
			found = found || peer == id
		}
		if !found {
			n.Peers = append(n.Peers, id)
			n.next[id] = n.lastIndex() + 1
			n.match[id] = 0
		}
	}
}

func (n *Node) applyCommittedConfigurations(first, last uint64) {
	if first <= n.BaseIndex {
		first = n.BaseIndex + 1
	}
	for index := first; index <= last; index++ {
		entry := n.entryAt(index)
		switch entry.Kind {
		case EntryJointConfig:
			n.votersOld, n.votersNew = entry.OldVoters, entry.NewVoters
			n.ensurePeers(entry.OldVoters | entry.NewVoters)
		case EntryFinalConfig:
			n.votersOld, n.votersNew = entry.OldVoters, 0
			if !n.isVoter(n.ID) {
				n.State, n.Leader, n.readReady = Follower, 0, false
			}
		}
		if index == n.pendingConfig {
			n.pendingConfig = 0
		}
	}
}
func (n *Node) Propose(command uint64) bool {
	if n.State != Leader || n.Faulted {
		return false
	}
	entry := Entry{Term: n.Term, Command: command}
	if !n.appendLocal(entry) {
		return false
	}
	n.broadcastAppend()
	return true
}

func (n *Node) becomeLeader() {
	n.State, n.Leader, n.elapsed = Leader, n.ID, 0
	last := n.lastIndex() + 1
	for _, p := range n.Peers {
		n.next[p], n.match[p] = last, 0
	}
	n.broadcastAppend()
}

func (n *Node) broadcastAppend() {
	for _, p := range n.Peers {
		if n.isVoter(p) {
			n.appendTo(p)
		}
	}
}

func (n *Node) appendTo(p uint32) {
	n.appendToContext(p, 0)
}

func (n *Node) appendToContext(p uint32, context uint64) {
	if n.next[p] <= n.BaseIndex {
		n.send(Message{Type: MsgInstallSnapshot, From: n.ID, To: p, Term: n.Term,
			SnapshotIndex: n.snapshot.LastIndex, SnapshotTerm: n.snapshot.LastTerm, Snapshot: n.snapshot.State, SnapshotOld: n.snapshot.OldVoters, SnapshotNew: n.snapshot.NewVoters})
		return
	}
	next := n.next[p]
	if next > n.lastIndex()+1 {
		next = n.lastIndex() + 1
		n.next[p] = next
	}
	if next == 0 {
		next = 1
	}
	prev := next - 1
	m := Message{Type: MsgAppend, From: n.ID, To: p, Term: n.Term,
		LogIndex: prev, LogTerm: n.termAtMust(prev), Commit: n.Commit, Context: context}
	if next < n.lastIndex()+1 {
		m.Entry, m.HasEntry = n.entryAt(next), true
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
		entry := n.entryAt(n.Applied)
		if entry.Kind == EntryNormal {
			out = append(out, entry.Command)
		}
	}
	return out
}

func (n *Node) restoreConfigurationState() bool {
	oldVoters, newVoters := n.votersOld, n.votersNew
	pending := uint64(0)
	first := n.BaseIndex + 1
	for index := first; index <= n.lastIndex(); index++ {
		entry := n.entryAt(index)
		switch entry.Kind {
		case EntryNormal:
			if entry.OldVoters != 0 || entry.NewVoters != 0 {
				return false
			}
		case EntryJointConfig:
			if pending != 0 || newVoters != 0 || entry.OldVoters != oldVoters ||
				entry.OldVoters == 0 || entry.NewVoters == 0 {
				return false
			}
			if index <= n.Commit {
				oldVoters, newVoters = entry.OldVoters, entry.NewVoters
			} else {
				pending = index
			}
		case EntryFinalConfig:
			if pending != 0 || newVoters == 0 || entry.OldVoters != newVoters || entry.NewVoters != 0 {
				return false
			}
			if index <= n.Commit {
				oldVoters, newVoters = entry.OldVoters, 0
			} else {
				pending = index
			}
		default:
			return false
		}
		n.ensurePeers(entry.OldVoters | entry.NewVoters)
	}
	n.votersOld, n.votersNew, n.pendingConfig = oldVoters, newVoters, pending
	return true
}
func (n *Node) rebuildPendingConfig() {
	n.pendingConfig = 0
	first := max(n.Commit+1, n.BaseIndex+1)
	for index := first; index <= n.lastIndex(); index++ {
		kind := n.entryAt(index).Kind
		if kind == EntryJointConfig || kind == EntryFinalConfig {
			n.pendingConfig = index
			entry := n.entryAt(index)
			n.ensurePeers(entry.OldVoters | entry.NewVoters)
		}
	}
}

func (n *Node) effectiveVoters() (uint64, uint64) {
	oldVoters, newVoters := n.votersOld, n.votersNew
	first := max(n.Commit+1, n.BaseIndex+1)
	for index := first; index <= n.lastIndex(); index++ {
		entry := n.entryAt(index)
		if entry.Kind == EntryJointConfig {
			oldVoters, newVoters = entry.OldVoters, entry.NewVoters
		}
	}
	return oldVoters, newVoters
}
func (n *Node) canCommitIndex(index uint64, acked uint64) bool {
	oldVoters, newVoters := n.votersOld, n.votersNew
	for pending := n.Commit + 1; pending <= index; pending++ {
		entry := n.entryAt(pending)
		if entry.Kind == EntryJointConfig {
			oldVoters, newVoters = entry.OldVoters, entry.NewVoters
		}
		// A final entry is committed by the current joint configuration;
		// C_new becomes active only after that entry is committed.
	}
	return majorityMask(oldVoters, acked) &&
		(newVoters == 0 || majorityMask(newVoters, acked))
}
func majorityMask(voters, acked uint64) bool {
	return voters != 0 && bitsSet(voters&acked) >= bitsSet(voters)/2+1
}

func bitsSet(mask uint64) int {
	count := 0
	for mask != 0 {
		mask &= mask - 1
		count++
	}
	return count
}

func (n *Node) quorumMask(mask uint64) bool {
	oldVoters, newVoters := n.effectiveVoters()
	return majorityMask(oldVoters, mask) &&
		(newVoters == 0 || majorityMask(newVoters, mask))
}

func (n *Node) isVoter(id uint32) bool {
	oldVoters, newVoters := n.effectiveVoters()
	bit := nodeBit(id)
	return (oldVoters|newVoters)&bit != 0
}

func nodeBit(id uint32) uint64 {
	if id == 0 || id > 64 {
		panic("raft: node IDs must be in 1..64")
	}
	return uint64(1) << (id - 1)
}
func (n *Node) persistHardState(h HardState) bool {
	if err := n.store.SaveHardState(h); err != nil {
		n.failStorageWith(err)
		return false
	}
	return true
}

// StorageError reports the stable-storage error that caused fail-stop.
func (n *Node) StorageError() error { return n.storageErr }

func (n *Node) failStorageWith(err error) {
	n.storageErr = err
	n.failStorage()
}

func (n *Node) failStorage() {
	if n.storageErr == nil {
		n.storageErr = ErrStorage
	}
	n.Faulted = true
	n.State = Follower
	n.Leader = 0
	n.outbound = n.outbound[:0]
}
