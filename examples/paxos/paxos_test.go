package paxos

import (
	"os"
	"testing"

	"github.com/hunterinvariants/HYPERION-DST/dst"
	"github.com/hunterinvariants/HYPERION-DST/dst/scenario"
)

func newEngine(t *testing.T, nodes int, config dst.Config) (*Cluster, *dst.Engine[Message]) {
	t.Helper()
	cluster := New(nodes)
	engine := dst.New[Message](config, cluster, cluster)
	engine.Watch(cluster.SafetyInvariants()...)
	return cluster, engine
}

// TestQuietClusterChooses is the liveness sanity check. The invariants are
// safety properties and would be perfectly happy with a protocol that does
// nothing, so progress has to be asserted separately.
func TestQuietClusterChooses(t *testing.T) {
	cluster, engine := newEngine(t, 5, dst.Config{Seed: 1})
	cluster.Propose(1, 0xC0FFEE)
	engine.Collect()

	if err := engine.RunChecked(200); err != nil {
		t.Fatal(err)
	}
	if !cluster.Decided() {
		t.Fatal("no node saw its proposal chosen in a quiet cluster")
	}
	value, ok := cluster.Chosen()
	if !ok || value != 0xC0FFEE {
		t.Fatalf("chosen value = %#x, ok = %v", value, ok)
	}
}

// TestCompetingProposersStaySafe is the case Paxos is built for: several
// proposers push different values at once, so rounds interleave and later
// proposers must adopt what earlier ones already got accepted.
func TestCompetingProposersStaySafe(t *testing.T) {
	adoptions := 0
	for seed := int64(1); seed <= 200; seed++ {
		cluster, engine := newEngine(t, 5, dst.Config{Seed: seed, DropPermille: 40, MaxDelay: 4})
		for tick := uint64(1); tick <= 600; tick++ {
			if err := engine.StepChecked(); err != nil {
				t.Fatalf("seed %d: %v", seed, err)
			}
			if tick%37 == 0 {
				proposer := uint32(tick/37%5 + 1)
				cluster.Propose(proposer, uint64(proposer)*1000+tick)
				engine.Collect()
			}
			if err := engine.CheckInvariants(); err != nil {
				t.Fatalf("seed %d: after proposing: %v", seed, err)
			}
		}
		if _, ok := cluster.Chosen(); !ok {
			t.Fatalf("seed %d: nothing was ever chosen, so this run proves nothing", seed)
		}
		adoptions += cluster.Adoptions()
	}
	// Without this the campaign could pass while never once forcing a proposer
	// to take over an already-accepted value, which is the only part of Paxos
	// that makes competing proposers safe.
	if adoptions == 0 {
		t.Fatal("no proposer ever adopted an accepted value; the campaign never created a real race")
	}
	t.Logf("proposers adopted an already-accepted value %d times across 200 seeds", adoptions)
}

// TestMinorityCannotChoose partitions two nodes away from three. The minority
// side must not be able to get anything chosen, and once the partition heals
// the whole cluster must still agree on a single value.
func TestMinorityCannotChoose(t *testing.T) {
	const heal = 400
	for seed := int64(1); seed <= 100; seed++ {
		cluster, engine := newEngine(t, 5, dst.Config{Seed: seed, MaxDelay: 3})
		minority, majority := []uint32{1, 2}, []uint32{3, 4, 5}
		partition := dst.During(0, heal, dst.Split(minority, majority))
		engine.Inject(partition)

		// Both sides try, so the run is a genuine race rather than a quiet
		// majority with an idle minority.
		cluster.Propose(1, 0xAAAA)
		cluster.Propose(4, 0xBBBB)
		engine.Collect()

		if err := engine.RunChecked(heal); err != nil {
			t.Fatalf("seed %d: during the partition: %v", seed, err)
		}
		if engine.InjectedDrops()[partition.Name()] == 0 {
			t.Fatalf("seed %d: the partition never dropped a message", seed)
		}
		if value, ok := cluster.Chosen(); ok && value == 0xAAAA {
			t.Fatalf("seed %d: the minority got %#x chosen", seed, value)
		}

		if err := engine.RunChecked(600); err != nil {
			t.Fatalf("seed %d: after healing: %v", seed, err)
		}
		if _, ok := cluster.Chosen(); !ok {
			t.Fatalf("seed %d: nothing chosen even after healing", seed)
		}
	}
}

// TestAgreementInvariantDetectsTwoChosenValues is the mutation test. An
// invariant that never fires is indistinguishable from one that is never
// evaluated, so the state it protects is corrupted on purpose here.
func TestAgreementInvariantDetectsTwoChosenValues(t *testing.T) {
	cluster, engine := newEngine(t, 5, dst.Config{Seed: 2})
	cluster.Propose(1, 0x1111)
	engine.Collect()
	if err := engine.RunChecked(200); err != nil {
		t.Fatal(err)
	}
	if err := engine.CheckInvariants(); err != nil {
		t.Fatalf("the unmutated cluster already violates an invariant: %v", err)
	}

	// Forge a second value accepted by a quorum at a different number.
	for _, id := range []uint32{3, 4, 5} {
		node := cluster.Node(id)
		node.accepts = append(node.accepts, Acceptance{Num: 999999, Val: 0x2222})
	}

	err := engine.CheckInvariants()
	var violation *dst.Violation
	if err == nil {
		t.Fatal("two chosen values went unnoticed")
	}
	if !asViolation(err, &violation) || violation.Invariant != "at most one value chosen" {
		t.Fatalf("reported %v, want the agreement invariant", err)
	}
}

func asViolation(err error, target **dst.Violation) bool {
	v, ok := err.(*dst.Violation)
	if ok {
		*target = v
	}
	return ok
}

// TestScenarioFileDrivesPaxos shows that the scenario format is not
// Raft-specific. The file describes the engine and its faults; this function
// is the twenty-line runner a different protocol supplies for itself.
func TestScenarioFileDrivesPaxos(t *testing.T) {
	spec, err := scenario.Load("scenario.json")
	if err != nil {
		t.Fatalf("load scenario: %v", err)
	}
	config, err := spec.EngineConfig()
	if err != nil {
		t.Fatalf("engine config: %v", err)
	}
	injectors, err := spec.Injectors()
	if err != nil {
		t.Fatalf("injectors: %v", err)
	}

	cluster, engine := newEngine(t, spec.Nodes, config)
	engine.Inject(injectors...)

	for tick := uint64(1); tick <= spec.Steps; tick++ {
		if err := engine.StepChecked(); err != nil {
			t.Fatalf("%s: %v", spec.Name, err)
		}
		if spec.ProposeEvery != 0 && tick%spec.ProposeEvery == 0 {
			proposer := uint32(tick/spec.ProposeEvery%uint64(spec.Nodes) + 1)
			cluster.Propose(proposer, uint64(proposer)<<32|tick)
			engine.Collect()
		}
	}

	for _, injector := range injectors {
		if engine.InjectedDrops()[injector.Name()] == 0 {
			t.Fatalf("fault %q never fired, so the scenario tested less than it declares",
				injector.Name())
		}
	}
	if _, ok := cluster.Chosen(); !ok {
		t.Fatalf("%s: nothing was chosen", spec.Name)
	}
}

func TestScenarioFileExists(t *testing.T) {
	if _, err := os.Stat("scenario.json"); err != nil {
		t.Fatalf("the example's scenario file is missing: %v", err)
	}
}

// TestRunIsReproducible pins that this protocol is deterministic under the
// engine. A protocol that iterates a map or reads the clock would fail here.
func TestRunIsReproducible(t *testing.T) {
	run := func() string {
		cluster, engine := newEngine(t, 5, dst.Config{Seed: 77, DropPermille: 80, MaxDelay: 6})
		for tick := uint64(1); tick <= 400; tick++ {
			engine.Step()
			if tick%29 == 0 {
				cluster.Propose(uint32(tick/29%5+1), tick)
				engine.Collect()
			}
		}
		return engine.TraceHash()
	}
	if a, b := run(), run(); a != b {
		t.Fatalf("trace differs across identical runs:\n %s\n %s", a, b)
	}
}
