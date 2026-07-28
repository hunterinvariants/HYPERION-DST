package sim

import "testing"

func TestJointConsensusTransitionSurvivesCrashes(t *testing.T) {
	const finalMask = uint64(0b00111) // nodes 1,2,3
	for seed := int64(1); seed <= 24; seed++ {
		s := New(Config{Nodes: 5, Seed: seed, DropPermille: 25, MaxDelay: 4})
		s.Run(150)
		if !s.ProposeJoint([]uint32{1, 2, 3}) {
			t.Fatalf("seed %d: no leader accepted joint configuration", seed)
		}
		finalProposed := false
		for tick := 0; tick < 600; tick++ {
			s.Step()
			if !finalProposed && s.ProposeFinal() {
				finalProposed = true
			}
		}
		if !finalProposed {
			t.Fatalf("seed %d: joint configuration never committed", seed)
		}
		for id := uint32(1); id <= 3; id++ {
			old, joint := s.Nodes[id].VoterMasks()
			if old != finalMask || joint != 0 {
				t.Fatalf("seed %d node %d config old=%x joint=%x", seed, id, old, joint)
			}
			if err := s.CrashRestart(id); err != nil {
				t.Fatalf("seed %d restart node %d: %v", seed, id, err)
			}
		}
		s.Run(200)
		if err := s.CheckSafety(); err != nil {
			t.Fatalf("seed %d after membership restarts: %v", seed, err)
		}
		leader := s.Leader()
		if leader > 3 {
			t.Fatalf("seed %d removed node %d became leader", seed, leader)
		}
	}
}
