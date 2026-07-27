package sim

import "testing"

func TestSeedIsReproducible(t *testing.T) {
	run := func() string {
		s := New(Config{Nodes: 5, Seed: 0x4A2C, DropPermille: 75, MaxDelay: 5})
		for i := uint64(1); i <= 300; i++ {
			s.Step()
			if i%17 == 0 {
				s.Propose(i)
			}
		}
		return s.TraceHash()
	}
	if a, b := run(), run(); a != b {
		t.Fatalf("trace differs: %s != %s", a, b)
	}
}

func TestCommittedPrefixesAgree(t *testing.T) {
	for seed := int64(1); seed <= 100; seed++ {
		s := New(Config{Nodes: 5, Seed: seed, DropPermille: 40, MaxDelay: 4})
		for i := uint64(1); i <= 600; i++ {
			s.Step()
			if i%13 == 0 {
				s.Propose(uint64(seed)<<32 | i)
			}
		}
		for a := uint32(1); a <= 5; a++ {
			for b := a + 1; b <= 5; b++ {
				na, nb := s.Nodes[a], s.Nodes[b]
				limit := min(na.Commit, nb.Commit)
				for i := uint64(1); i <= limit; i++ {
					if na.Log[i] != nb.Log[i] {
						t.Fatalf("seed %d nodes %d/%d disagree at committed index %d", seed, a, b, i)
					}
				}
			}
		}
	}
}
