// Package paxos is a worked example of driving a protocol that is not Raft
// through the Promtact deterministic engine.
//
// It implements single-decree Paxos: proposers try to get one value chosen,
// and the safety property is that no two different values are ever chosen, no
// matter how messages are lost, delayed, reordered, or partitioned.
//
// The protocol here is deliberately plain. What the example is about is the
// wiring in cluster.go, the properties in invariants.go, and the campaigns in
// paxos_test.go.
package paxos

// Kind distinguishes the four Paxos message types.
type Kind uint8

const (
	Prepare Kind = iota
	Promise
	Accept
	Accepted
)

// Message is one protocol message. The engine treats it as opaque and only
// reads what Wire exposes.
type Message struct {
	Kind     Kind
	From, To uint32
	// Num is the proposal number the message concerns.
	Num uint64
	// Val carries the proposed value on Accept.
	Val uint64
	// OK reports whether an acceptor honoured a Prepare or an Accept.
	OK bool
	// AcceptedNum and AcceptedVal report what the acceptor had already
	// accepted when it answered a Prepare. This is the field that makes Paxos
	// safe: a later proposer must adopt the value of the highest-numbered
	// acceptance it learns about.
	AcceptedNum uint64
	AcceptedVal uint64
	HasAccepted bool
}

type phase uint8

const (
	idle phase = iota
	preparing
	accepting
	decided
)

// Node is both a proposer and an acceptor, which is the usual arrangement.
type Node struct {
	id    uint32
	size  int
	peers []uint32

	// Acceptor state. All three are the state a crash must preserve for the
	// protocol to stay safe.
	promised    uint64
	acceptedNum uint64
	acceptedVal uint64
	hasAccepted bool

	// accepts records every acceptance this node ever made, so that the
	// invariants can reconstruct which values reached a quorum. A real
	// implementation would not keep this.
	accepts []Acceptance

	// Proposer state.
	phase       phase
	round       uint64
	myNum       uint64
	myVal       uint64
	promiseMask uint64
	acceptMask  uint64
	bestNum     uint64
	bestVal     uint64
	hasBest     bool
	elapsed     uint64
	timeout     uint64

	// adopted counts how often this proposer had to abandon its own value and
	// take over one that was already accepted. That branch is the whole reason
	// Paxos is safe, and a campaign in which it never runs has not tested it.
	adopted int

	outbound []Message
}

// Acceptance is one recorded (proposal number, value) pair.
type Acceptance struct {
	Num uint64
	Val uint64
}

// NewNode builds a node. The timeout is per-node so that proposers do not
// retry in lockstep forever, the same trick the Raft core uses for elections.
func NewNode(id uint32, size int, timeout uint64) *Node {
	peers := make([]uint32, 0, size-1)
	for i := 1; i <= size; i++ {
		if uint32(i) != id {
			peers = append(peers, uint32(i))
		}
	}
	return &Node{id: id, size: size, peers: peers, timeout: timeout,
		outbound: make([]Message, 0, 16)}
}

func (n *Node) quorum() int { return n.size/2 + 1 }

// Decided reports whether this node has seen its own proposal chosen.
func (n *Node) Decided() bool { return n.phase == decided }

// Accepted reports the node's acceptor state.
func (n *Node) Accepted() (num, val uint64, ok bool) {
	return n.acceptedNum, n.acceptedVal, n.hasAccepted
}

// Acceptances returns every acceptance this node made, oldest first.
func (n *Node) Acceptances() []Acceptance { return n.accepts }

// Promised returns the highest proposal number this node has promised on.
func (n *Node) Promised() uint64 { return n.promised }

// Propose starts a new round for val. Calling it again supersedes the previous
// attempt with a higher number.
func (n *Node) Propose(val uint64) {
	n.round = max(n.round+1, n.promised/uint64(n.size)+1)
	// Proposal numbers must be unique per proposer. Distinct ids in 1..size
	// occupy distinct residues modulo size, so no two proposers can ever pick
	// the same number.
	n.myNum = n.round*uint64(n.size) + uint64(n.id)
	n.myVal = val
	n.phase = preparing
	n.elapsed = 0
	n.promiseMask, n.acceptMask = 0, 0
	n.hasBest = false

	// Answer our own Prepare directly rather than sending a message to
	// ourselves; the engine only needs to carry traffic between nodes.
	if ok, accNum, accVal, hasAcc := n.onPrepare(n.myNum); ok {
		n.recordPromise(n.id, accNum, accVal, hasAcc)
	}
	for _, peer := range n.peers {
		n.outbound = append(n.outbound, Message{Kind: Prepare, From: n.id, To: peer, Num: n.myNum})
	}
}

// Tick advances the node by one unit of virtual time. A proposer that has not
// completed within its timeout starts a fresh, higher-numbered round.
func (n *Node) Tick() {
	if n.phase != preparing && n.phase != accepting {
		return
	}
	n.elapsed++
	if n.elapsed >= n.timeout {
		n.Propose(n.myVal)
	}
}

// Step handles one delivered message.
func (n *Node) Step(m Message) {
	switch m.Kind {
	case Prepare:
		ok, accNum, accVal, hasAcc := n.onPrepare(m.Num)
		n.outbound = append(n.outbound, Message{
			Kind: Promise, From: n.id, To: m.From, Num: m.Num, OK: ok,
			AcceptedNum: accNum, AcceptedVal: accVal, HasAccepted: hasAcc,
		})
	case Promise:
		if m.Num != n.myNum || n.phase != preparing || !m.OK {
			return
		}
		n.recordPromise(m.From, m.AcceptedNum, m.AcceptedVal, m.HasAccepted)
	case Accept:
		ok := n.onAccept(m.Num, m.Val)
		n.outbound = append(n.outbound, Message{
			Kind: Accepted, From: n.id, To: m.From, Num: m.Num, OK: ok,
		})
	case Accepted:
		if m.Num != n.myNum || n.phase != accepting || !m.OK {
			return
		}
		n.acceptMask |= bit(m.From)
		if popcount(n.acceptMask) >= n.quorum() {
			n.phase = decided
		}
	}
}

// Drain moves outbound messages to dst.
func (n *Node) Drain(dst []Message) []Message {
	dst = append(dst, n.outbound...)
	n.outbound = n.outbound[:0]
	return dst
}

// onPrepare is the acceptor half of phase one. It promises only on a strictly
// higher number, and reports what it has already accepted.
func (n *Node) onPrepare(num uint64) (ok bool, accNum, accVal uint64, hasAcc bool) {
	if num <= n.promised {
		return false, 0, 0, false
	}
	n.promised = num
	return true, n.acceptedNum, n.acceptedVal, n.hasAccepted
}

// onAccept is the acceptor half of phase two. Accepting at a number it has
// already promised past would break the protocol, so it refuses.
func (n *Node) onAccept(num, val uint64) bool {
	if num < n.promised {
		return false
	}
	n.promised = num
	n.acceptedNum, n.acceptedVal, n.hasAccepted = num, val, true
	n.accepts = append(n.accepts, Acceptance{Num: num, Val: val})
	return true
}

// recordPromise collects one promise and moves to phase two once a quorum has
// answered. Adopting the highest-numbered value already accepted is the step
// that makes a second round unable to choose a different value.
func (n *Node) recordPromise(from uint32, accNum, accVal uint64, hasAcc bool) {
	n.promiseMask |= bit(from)
	if hasAcc && (!n.hasBest || accNum > n.bestNum) {
		n.bestNum, n.bestVal, n.hasBest = accNum, accVal, true
	}
	if popcount(n.promiseMask) < n.quorum() {
		return
	}
	if n.hasBest {
		if n.bestVal != n.myVal {
			n.adopted++
		}
		n.myVal = n.bestVal
	}
	n.phase = accepting
	n.acceptMask = 0
	if n.onAccept(n.myNum, n.myVal) {
		n.acceptMask |= bit(n.id)
	}
	if popcount(n.acceptMask) >= n.quorum() {
		n.phase = decided
		return
	}
	for _, peer := range n.peers {
		n.outbound = append(n.outbound, Message{
			Kind: Accept, From: n.id, To: peer, Num: n.myNum, Val: n.myVal,
		})
	}
}

func bit(id uint32) uint64 { return 1 << (id - 1) }

func popcount(mask uint64) int {
	count := 0
	for mask != 0 {
		mask &= mask - 1
		count++
	}
	return count
}
