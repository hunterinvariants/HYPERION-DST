// Package sim provides a deterministic, single-threaded execution environment.
package sim

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math/rand"

	"github.com/hunterinvariants/promtact/raft"
	"github.com/hunterinvariants/promtact/storage/raftwal"
	"github.com/hunterinvariants/promtact/storage/wal"
)

type scheduled struct {
	at  uint64
	seq uint64
	msg raft.Message
}

type Config struct {
	Nodes        int
	Seed         int64
	DropPermille int
	MaxDelay     uint64
}

type Simulator struct {
	Now     uint64
	Nodes   map[uint32]*raft.Node
	stores  map[uint32]*stableStore
	disks   map[uint32]*wal.MemoryDevice
	rng     *rand.Rand
	drop    int
	delay   uint64
	seq     uint64
	queue   []scheduled
	trace   [32]byte
	scratch []raft.Message
}

func New(c Config) *Simulator {
	if c.Nodes < 1 {
		panic("sim: Nodes must be positive")
	}
	s := &Simulator{Nodes: make(map[uint32]*raft.Node, c.Nodes),
		stores: make(map[uint32]*stableStore, c.Nodes),
		disks:  make(map[uint32]*wal.MemoryDevice, c.Nodes),
		rng:    rand.New(rand.NewSource(c.Seed)), drop: c.DropPermille,
		delay: c.MaxDelay, scratch: make([]raft.Message, 0, 4096)}
	for i := 1; i <= c.Nodes; i++ {
		id := uint32(i)
		peers := make([]uint32, 0, c.Nodes-1)
		for j := 1; j <= c.Nodes; j++ {
			if j != i {
				peers = append(peers, uint32(j))
			}
		}
		// Stable per-node jitter prevents perpetual split votes.
		disk := wal.NewMemoryDevice(nil)
		store, err := raftwal.Open(disk)
		if err != nil {
			panic(err)
		}
		stable := &stableStore{wal: store}
		s.disks[id], s.stores[id] = disk, stable
		s.Nodes[id] = raft.NewNodeWithStore(id, peers, 8+uint64(i*2), stable)
	}
	return s
}

func (s *Simulator) Step() {
	s.Now++
	for id := uint32(1); id <= uint32(len(s.Nodes)); id++ {
		s.Nodes[id].Tick()
	}
	s.collect()
	for {
		idx := s.nextReady()
		if idx < 0 {
			break
		}
		ev := s.queue[idx]
		s.queue = append(s.queue[:idx], s.queue[idx+1:]...)
		s.record(ev)
		s.Nodes[ev.msg.To].Step(ev.msg)
		s.collect()
	}
}

func (s *Simulator) Run(steps uint64) {
	for range steps {
		s.Step()
	}
}

func (s *Simulator) Propose(command uint64) bool {
	for id := uint32(1); id <= uint32(len(s.Nodes)); id++ {
		if s.Nodes[id].Propose(command) {
			s.collect()
			return true
		}
	}
	return false
}

// Compact snapshots a node at its applied index. The caller supplies the
// deterministic state-machine image represented by that index.
func (s *Simulator) Compact(id uint32, state []byte) bool {
	node, ok := s.Nodes[id]
	if !ok || node.Applied <= node.BaseIndex {
		return false
	}
	return node.Compact(node.Applied, state)
}
func (s *Simulator) ProposeJoint(voters []uint32) bool {
	for id := uint32(1); id <= uint32(len(s.Nodes)); id++ {
		if s.Nodes[id].ProposeJoint(voters) {
			s.collect()
			return true
		}
	}
	return false
}

func (s *Simulator) ProposeFinal() bool {
	for id := uint32(1); id <= uint32(len(s.Nodes)); id++ {
		if s.Nodes[id].ProposeFinal() {
			s.collect()
			return true
		}
	}
	return false
}
func (s *Simulator) Leader() uint32 {
	for id := uint32(1); id <= uint32(len(s.Nodes)); id++ {
		if s.Nodes[id].State == raft.Leader {
			return id
		}
	}
	return 0
}

func (s *Simulator) TraceHash() string { return fmt.Sprintf("%x", s.trace) }

func (s *Simulator) collect() {
	for id := uint32(1); id <= uint32(len(s.Nodes)); id++ {
		s.scratch = s.Nodes[id].Drain(s.scratch[:0])
		for _, m := range s.scratch {
			if s.drop > 0 && s.rng.Intn(1000) < s.drop {
				continue
			}
			d := uint64(0)
			if s.delay > 0 {
				d = uint64(s.rng.Int63n(int64(s.delay + 1)))
			}
			s.seq++
			s.queue = append(s.queue, scheduled{at: s.Now + d, seq: s.seq, msg: m})
		}
	}
}

func (s *Simulator) nextReady() int {
	best := -1
	for i := range s.queue {
		if s.queue[i].at > s.Now {
			continue
		}
		if best < 0 || s.queue[i].at < s.queue[best].at ||
			(s.queue[i].at == s.queue[best].at && s.queue[i].seq < s.queue[best].seq) {
			best = i
		}
	}
	return best
}

func (s *Simulator) record(e scheduled) {
	var b [65]byte
	binary.LittleEndian.PutUint64(b[0:], s.Now)
	binary.LittleEndian.PutUint64(b[8:], e.seq)
	binary.LittleEndian.PutUint32(b[16:], e.msg.From)
	binary.LittleEndian.PutUint32(b[20:], e.msg.To)
	b[24] = byte(e.msg.Type)
	binary.LittleEndian.PutUint64(b[25:], e.msg.Term)
	copy(b[33:], s.trace[:])
	s.trace = sha256.Sum256(b[:])
}
