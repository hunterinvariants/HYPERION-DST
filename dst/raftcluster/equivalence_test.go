package raftcluster_test

import (
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/hunterinvariants/promtact/dst"
	"github.com/hunterinvariants/promtact/dst/raftcluster"
	"github.com/hunterinvariants/promtact/raft"
	"github.com/hunterinvariants/promtact/sim"
)

// equivalenceSeeds keeps the default CI cost low while allowing a qualification
// host to widen the sweep via PROMTACT_EQUIV_SEEDS.
func equivalenceSeeds(t *testing.T) int64 {
	raw, set := os.LookupEnv("PROMTACT_EQUIV_SEEDS")
	if !set {
		return 24
	}
	seeds, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || seeds < 1 {
		t.Fatalf("PROMTACT_EQUIV_SEEDS must be a positive integer, got %q", raw)
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
				assertNodesAgree(t, seed, legacy, cluster)
			}
		})
	}
}

// assertNodesAgree compares every node's durable and volatile consensus state,
// including its committed log and its configuration masks. The trace hash only
// covers messages that were delivered, so this catches divergence the protocol
// absorbed without changing message order.
func assertNodesAgree(t *testing.T, seed int64, legacy *sim.Simulator, cluster *raftcluster.Cluster) {
	t.Helper()
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
		wantOld, wantNew := want.VoterMasks()
		gotOld, gotNew := got.VoterMasks()
		if wantOld != gotOld || wantNew != gotNew {
			t.Fatalf("seed %d node %d configuration diverged: legacy(old=%x new=%x) engine(old=%x new=%x)",
				seed, id, wantOld, wantNew, gotOld, gotNew)
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

// stepBoth advances both implementations one tick and requires their traces to
// still agree.
func stepBoth(t *testing.T, seed int64, tick uint64, legacy *sim.Simulator, engine *dst.Engine[raft.Message]) {
	t.Helper()
	legacy.Step()
	engine.Step()
	if want, got := legacy.TraceHash(), engine.TraceHash(); want != got {
		t.Fatalf("seed %d tick %d: trace diverged\n legacy: %s\n engine: %s", seed, tick, want, got)
	}
}

// proposeBoth submits the same command to both implementations and requires
// them to agree on whether it was accepted.
func proposeBoth(t *testing.T, seed int64, tick uint64, command uint64,
	legacy *sim.Simulator, cluster *raftcluster.Cluster, engine *dst.Engine[raft.Message]) {
	t.Helper()
	legacyOK := legacy.Propose(command)
	engineOK := cluster.Propose(command)
	if engineOK {
		engine.Collect()
	}
	if legacyOK != engineOK {
		t.Fatalf("seed %d tick %d: proposal accepted by legacy=%v engine=%v", seed, tick, legacyOK, engineOK)
	}
}

// TestEngineMatchesFrozenSimulatorUnderCompaction mirrors the deterministic
// compaction campaign in sim/compaction_test.go. It closes the snapshot half of
// the equivalence gap: without it, log compaction and InstallSnapshot catch-up
// would never execute through the engine.
func TestEngineMatchesFrozenSimulatorUnderCompaction(t *testing.T) {
	for seed := int64(1); seed <= equivalenceSeeds(t); seed++ {
		legacy := sim.New(sim.Config{Nodes: 5, Seed: seed, DropPermille: 35, MaxDelay: 4})
		cluster := raftcluster.New(5)
		engine := dst.New[raft.Message](dst.Config{Seed: seed, DropPermille: 35, MaxDelay: 4}, cluster, cluster)

		for tick := uint64(1); tick <= 500; tick++ {
			stepBoth(t, seed, tick, legacy, engine)
			if tick%11 == 0 {
				proposeBoth(t, seed, tick, uint64(seed)<<32|tick, legacy, cluster, engine)
			}
		}

		for _, id := range cluster.Nodes() {
			_ = legacy.Nodes[id].Apply()
			_ = cluster.Node(id).Apply()
		}
		assertNodesAgree(t, seed, legacy, cluster)

		compacted := 0
		for _, id := range cluster.Nodes() {
			// Both sides receive byte-identical state, so a divergence can only
			// come from the compaction path itself.
			state := []byte(fmt.Sprintf("seed=%d,index=%d", seed, legacy.Nodes[id].Applied))
			legacyOK := legacy.Compact(id, state)
			engineOK := cluster.Compact(id, state)
			if legacyOK != engineOK {
				t.Fatalf("seed %d node %d: compaction accepted by legacy=%v engine=%v",
					seed, id, legacyOK, engineOK)
			}
			if engineOK {
				compacted++
			}
		}
		if compacted == 0 {
			t.Fatalf("seed %d produced no compactable node, so the campaign proves nothing", seed)
		}

		for _, id := range cluster.Nodes() {
			if err := legacy.CrashRestart(id); err != nil {
				t.Fatalf("seed %d: legacy restart node %d: %v", seed, id, err)
			}
			if err := cluster.Restart(id); err != nil {
				t.Fatalf("seed %d: engine restart node %d: %v", seed, id, err)
			}
			engine.Isolate(id)
		}

		for tick := uint64(1); tick <= 300; tick++ {
			stepBoth(t, seed, tick, legacy, engine)
			if tick%17 == 0 {
				proposeBoth(t, seed, tick, uint64(seed)<<48|tick, legacy, cluster, engine)
			}
			if err := legacy.CheckSafety(); err != nil {
				t.Fatalf("seed %d tick %d: legacy safety: %v", seed, tick, err)
			}
		}
		assertNodesAgree(t, seed, legacy, cluster)

		// A compacted base that survived the restart is the evidence that the
		// snapshot path carried real state, rather than Compact merely
		// reporting success.
		compactedBase := false
		for _, id := range cluster.Nodes() {
			if cluster.Node(id).BaseIndex > 0 {
				compactedBase = true
				break
			}
		}
		if !compactedBase {
			t.Fatalf("seed %d: no node recovered a compacted base, so the snapshot path was not exercised", seed)
		}
	}
}

// TestEngineMatchesFrozenSimulatorUnderMembership mirrors the joint-consensus
// campaign in sim/membership_test.go. It closes the membership half of the
// equivalence gap: joint and final configuration entries, and the dual-majority
// commit rule, otherwise never execute through the engine.
func TestEngineMatchesFrozenSimulatorUnderMembership(t *testing.T) {
	const finalMask = uint64(0b00111) // nodes 1, 2, 3
	newVoters := []uint32{1, 2, 3}

	for seed := int64(1); seed <= equivalenceSeeds(t); seed++ {
		legacy := sim.New(sim.Config{Nodes: 5, Seed: seed, DropPermille: 25, MaxDelay: 4})
		cluster := raftcluster.New(5)
		engine := dst.New[raft.Message](dst.Config{Seed: seed, DropPermille: 25, MaxDelay: 4}, cluster, cluster)

		for tick := uint64(1); tick <= 150; tick++ {
			stepBoth(t, seed, tick, legacy, engine)
		}

		legacyJoint := legacy.ProposeJoint(newVoters)
		engineJoint := cluster.ProposeJoint(newVoters)
		if engineJoint {
			engine.Collect()
		}
		if legacyJoint != engineJoint {
			t.Fatalf("seed %d: joint configuration accepted by legacy=%v engine=%v",
				seed, legacyJoint, engineJoint)
		}
		if !legacyJoint {
			t.Fatalf("seed %d: no leader accepted the joint configuration", seed)
		}

		finalProposed := false
		for tick := uint64(1); tick <= 600; tick++ {
			stepBoth(t, seed, tick, legacy, engine)
			if finalProposed {
				continue
			}
			legacyFinal := legacy.ProposeFinal()
			engineFinal := cluster.ProposeFinal()
			if engineFinal {
				engine.Collect()
			}
			if legacyFinal != engineFinal {
				t.Fatalf("seed %d tick %d: final configuration accepted by legacy=%v engine=%v",
					seed, tick, legacyFinal, engineFinal)
			}
			finalProposed = legacyFinal
		}
		if !finalProposed {
			t.Fatalf("seed %d: joint configuration never committed, so the campaign proves nothing", seed)
		}
		assertNodesAgree(t, seed, legacy, cluster)

		for _, id := range newVoters {
			if old, joint := cluster.Node(id).VoterMasks(); old != finalMask || joint != 0 {
				t.Fatalf("seed %d node %d did not leave the joint configuration: old=%x joint=%x",
					seed, id, old, joint)
			}
			if err := legacy.CrashRestart(id); err != nil {
				t.Fatalf("seed %d: legacy restart node %d: %v", seed, id, err)
			}
			if err := cluster.Restart(id); err != nil {
				t.Fatalf("seed %d: engine restart node %d: %v", seed, id, err)
			}
			engine.Isolate(id)
		}

		for tick := uint64(1); tick <= 200; tick++ {
			stepBoth(t, seed, tick, legacy, engine)
		}
		if err := legacy.CheckSafety(); err != nil {
			t.Fatalf("seed %d after membership restarts: %v", seed, err)
		}
		assertNodesAgree(t, seed, legacy, cluster)

		if leader := cluster.Leader(); leader > 3 {
			t.Fatalf("seed %d: removed node %d became leader", seed, leader)
		}
	}
}
