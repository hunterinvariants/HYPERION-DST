package raftcluster_test

import (
	"os"
	"strconv"
	"testing"

	"github.com/hunterinvariants/HYPERION-DST/dst"
	"github.com/hunterinvariants/HYPERION-DST/dst/raftcluster"
	"github.com/hunterinvariants/HYPERION-DST/raft"
	"github.com/hunterinvariants/HYPERION-DST/sim"
)

// equivalenceSeeds keeps the default CI cost low while allowing a qualification
// host to widen the sweep via HYPERION_EQUIV_SEEDS.
func equivalenceSeeds(t *testing.T) int64 {
	raw, set := os.LookupEnv("HYPERION_EQUIV_SEEDS")
	if !set {
		return 24
	}
	seeds, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || seeds < 1 {
		t.Fatalf("HYPERION_EQUIV_SEEDS must be a positive integer, got %q", raw)
	}
	return seeds
}

// scenario is one deterministic workload, expressed once and replayed against
// both the frozen sim.Simulator and the generic engine.
type scenario struct {
	name         string
	nodes        int
	steps        uint64
	dropPermille int
	maxDelay     uint64
	proposeEvery uint64
	crashEvery   uint64
}

var scenarios = []scenario{
	{name: "quiet", nodes: 3, steps: 400, dropPermille: 0, maxDelay: 0, proposeEvery: 13},
	{name: "lossy", nodes: 5, steps: 500, dropPermille: 75, maxDelay: 5, proposeEvery: 17},
	{name: "seed-sweep-profile", nodes: 5, steps: 600, dropPermille: 50, maxDelay: 5, proposeEvery: 17, crashEvery: 101},
	{name: "restart-storm", nodes: 5, steps: 500, dropPermille: 60, maxDelay: 5, proposeEvery: 19, crashEvery: 37},
	{name: "delay-only", nodes: 7, steps: 300, dropPermille: 0, maxDelay: 9, proposeEvery: 11, crashEvery: 71},
}

// TestEngineMatchesFrozenSimulator is the safety net for the modularization:
// the generic engine driving raftcluster must produce a bit-identical
// execution to the qualified sim.Simulator. The trace hash covers every
// delivery, so any divergence in scheduling, loss, delay, or protocol handling
// changes it. The per-node state comparison catches divergence that the
// protocol absorbs without changing message order.
func TestEngineMatchesFrozenSimulator(t *testing.T) {
	seeds := equivalenceSeeds(t)
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			for seed := int64(1); seed <= seeds; seed++ {
				legacy := sim.New(sim.Config{
					Nodes:        sc.nodes,
					Seed:         seed,
					DropPermille: sc.dropPermille,
					MaxDelay:     sc.maxDelay,
				})
				cluster := raftcluster.New(sc.nodes)
				engine := dst.New[raft.Message](dst.Config{
					Seed:         seed,
					DropPermille: sc.dropPermille,
					MaxDelay:     sc.maxDelay,
				}, cluster, cluster)

				for tick := uint64(1); tick <= sc.steps; tick++ {
					legacy.Step()
					engine.Step()

					if sc.proposeEvery != 0 && tick%sc.proposeEvery == 0 {
						command := uint64(seed)<<32 | tick
						legacyOK := legacy.Propose(command)
						engineOK := cluster.Propose(command)
						if engineOK {
							engine.Collect()
						}
						if legacyOK != engineOK {
							t.Fatalf("seed %d tick %d: proposal accepted by legacy=%v engine=%v",
								seed, tick, legacyOK, engineOK)
						}
					}

					if sc.crashEvery != 0 && tick%sc.crashEvery == 0 {
						id := uint32((tick/sc.crashEvery)%uint64(sc.nodes) + 1)
						if err := legacy.CrashRestart(id); err != nil {
							t.Fatalf("seed %d tick %d: legacy restart: %v", seed, tick, err)
						}
						if err := cluster.Restart(id); err != nil {
							t.Fatalf("seed %d tick %d: engine restart: %v", seed, tick, err)
						}
						engine.Isolate(id)
					}

					if want, got := legacy.TraceHash(), engine.TraceHash(); want != got {
						t.Fatalf("seed %d tick %d: trace diverged\n legacy: %s\n engine: %s",
							seed, tick, want, got)
					}
				}

				if err := legacy.CheckSafety(); err != nil {
					t.Fatalf("seed %d: legacy safety: %v", seed, err)
				}
				for _, id := range cluster.Nodes() {
					want, got := legacy.Nodes[id], cluster.Node(id)
					if want.Term != got.Term || want.State != got.State ||
						want.Commit != got.Commit || want.Applied != got.Applied ||
						want.BaseIndex != got.BaseIndex || want.LastIndex() != got.LastIndex() {
						t.Fatalf("seed %d node %d state diverged: legacy(term=%d state=%d commit=%d applied=%d base=%d last=%d) engine(term=%d state=%d commit=%d applied=%d base=%d last=%d)",
							seed, id,
							want.Term, want.State, want.Commit, want.Applied, want.BaseIndex, want.LastIndex(),
							got.Term, got.State, got.Commit, got.Applied, got.BaseIndex, got.LastIndex())
					}
					for index := got.BaseIndex + 1; index <= got.Commit; index++ {
						a, oka := want.EntryAt(index)
						b, okb := got.EntryAt(index)
						if oka != okb || a != b {
							t.Fatalf("seed %d node %d: committed entry %d diverged", seed, id, index)
						}
					}
				}
			}
		})
	}
}

// TestEquivalenceComparisonHasTeeth is the negative control for
// TestEngineMatchesFrozenSimulator. Running the two implementations under
// different seeds must make the trace hashes diverge, otherwise the equality
// assertion above proves nothing.
func TestEquivalenceComparisonHasTeeth(t *testing.T) {
	legacy := sim.New(sim.Config{Nodes: 5, Seed: 1, DropPermille: 75, MaxDelay: 5})
	cluster := raftcluster.New(5)
	engine := dst.New[raft.Message](dst.Config{Seed: 2, DropPermille: 75, MaxDelay: 5}, cluster, cluster)

	for tick := uint64(1); tick <= 300; tick++ {
		legacy.Step()
		engine.Step()
		if legacy.TraceHash() != engine.TraceHash() {
			return
		}
	}
	t.Fatalf("seeds 1 and 2 produced identical traces (%s), so the trace hash does not distinguish runs",
		engine.TraceHash())
}

// TestEngineRunIsReproducible pins the engine's own determinism, independent of
// the legacy simulator.
func TestEngineRunIsReproducible(t *testing.T) {
	run := func() string {
		cluster := raftcluster.New(5)
		engine := dst.New[raft.Message](dst.Config{Seed:0x4A2C, DropPermille: 75, MaxDelay: 5}, cluster, cluster)
		for tick := uint64(1); tick <= 400; tick++ {
			engine.Step()
			if tick%17 == 0 && cluster.Propose(tick) {
				engine.Collect()
			}
			if tick%53 == 0 {
				id := uint32(tick%5 + 1)
				if err := cluster.Restart(id); err != nil {
					t.Fatal(err)
				}
				engine.Isolate(id)
			}
		}
		return engine.TraceHash()
	}
	if a, b := run(), run(); a != b {
		t.Fatalf("trace differs across identical runs: %s != %s", a, b)
	}
}

// TestEngineElectsLeader guards against an engine that is deterministic but
// inert: a quiet five-node cluster must reach a leader.
func TestEngineElectsLeader(t *testing.T) {
	cluster := raftcluster.New(5)
	engine := dst.New[raft.Message](dst.Config{Seed: 7}, cluster, cluster)
	engine.Run(200)
	if cluster.Leader() == 0 {
		t.Fatal("no leader after 200 quiet steps")
	}
}
