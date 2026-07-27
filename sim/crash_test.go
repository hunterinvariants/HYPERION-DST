package sim

import "testing"

func TestCrashRestartDuringElectionsPreservesSafety(t *testing.T) {
	for seed := int64(1); seed <= 64; seed++ {
		s := New(Config{Nodes: 5, Seed: seed, DropPermille: 60, MaxDelay: 5})
		for tick := uint64(1); tick <= 800; tick++ {
			s.Step()
			if tick%19 == 0 {
				s.Propose(uint64(seed)<<32 | tick)
			}
			if tick%37 == 0 {
				id := uint32((tick/37)%5 + 1)
				if err := s.CrashRestart(id); err != nil {
					t.Fatalf("seed %d tick %d: %v", seed, tick, err)
				}
			}
		}
		assertCommittedPrefixes(t, seed, s)
	}
}

func TestCrashSeedIsReproducible(t *testing.T) {
	run := func() string {
		s := New(Config{Nodes: 3, Seed: 0xCAFE, DropPermille: 30, MaxDelay: 3})
		for tick := uint64(1); tick <= 300; tick++ {
			s.Step()
			if tick%29 == 0 {
				s.Propose(tick)
			}
			if tick%41 == 0 {
				if err := s.CrashRestart(uint32(tick%3 + 1)); err != nil {
					t.Fatal(err)
				}
			}
		}
		return s.TraceHash()
	}
	if a, b := run(), run(); a != b {
		t.Fatalf("crash trace differs: %s != %s", a, b)
	}
}

func assertCommittedPrefixes(t *testing.T, seed int64, s *Simulator) {
	t.Helper()
	for a := uint32(1); a <= uint32(len(s.Nodes)); a++ {
		for b := a + 1; b <= uint32(len(s.Nodes)); b++ {
			na, nb := s.Nodes[a], s.Nodes[b]
			limit := min(na.Commit, nb.Commit)
			for i := uint64(1); i <= limit; i++ {
				if na.Log[i] != nb.Log[i] {
					t.Fatalf("seed %d nodes %d/%d disagree at %d", seed, a, b, i)
				}
			}
		}
	}
}
