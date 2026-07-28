package sim

import (
	"fmt"
	"testing"
)

func TestDeterministicCompactionCrashAndSnapshotCatchUp(t *testing.T) {
	for seed := int64(1); seed <= 32; seed++ {
		s := New(Config{Nodes: 5, Seed: seed, DropPermille: 35, MaxDelay: 4})
		for tick := uint64(1); tick <= 500; tick++ {
			s.Step()
			if tick%11 == 0 {
				s.Propose(uint64(seed)<<32 | tick)
			}
		}
		compacted := 0
		for id := uint32(1); id <= 5; id++ {
			node := s.Nodes[id]
			_ = node.Apply()
			if s.Compact(id, []byte(fmt.Sprintf("seed=%d,index=%d", seed, node.Applied))) {
				compacted++
			}
		}
		if compacted == 0 {
			t.Fatalf("seed %d produced no compactable node", seed)
		}
		for id := uint32(1); id <= 5; id++ {
			if err := s.CrashRestart(id); err != nil {
				t.Fatalf("seed %d restart node %d: %v", seed, id, err)
			}
		}
		for tick := uint64(1); tick <= 300; tick++ {
			s.Step()
			if tick%17 == 0 {
				s.Propose(uint64(seed)<<48 | tick)
			}
			if err := s.CheckSafety(); err != nil {
				t.Fatalf("seed %d tick %d: %v", seed, tick, err)
			}
		}
	}
}
