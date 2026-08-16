package raftcluster_test

import (
	"errors"
	"testing"

	"github.com/hunterinvariants/HYPERION-DST/dst"
	"github.com/hunterinvariants/HYPERION-DST/dst/raftcluster"
	"github.com/hunterinvariants/HYPERION-DST/raft"
)

// TestSafetyInvariantsHoldUnderFaults runs the packaged Raft properties against
// the engine under loss, delay, proposals, and restarts.
func TestSafetyInvariantsHoldUnderFaults(t *testing.T) {
	for seed := int64(1); seed <= equivalenceSeeds(t); seed++ {
		cluster := raftcluster.New(5)
		engine := dst.New[raft.Message](dst.Config{Seed: seed, DropPermille: 60, MaxDelay: 5}, cluster, cluster)
		engine.Watch(cluster.SafetyInvariants()...)

		for tick := uint64(1); tick <= 500; tick++ {
			if err := engine.StepChecked(); err != nil {
				t.Fatalf("seed %d: %v", seed, err)
			}
			if tick%19 == 0 && cluster.Propose(uint64(seed)<<32|tick) {
				engine.Collect()
			}
			if tick%37 == 0 {
				id := uint32((tick/37)%5 + 1)
				if err := cluster.Restart(id); err != nil {
					t.Fatalf("seed %d: restart node %d: %v", seed, id, err)
				}
				engine.Isolate(id)
			}
			if err := engine.CheckInvariants(); err != nil {
				t.Fatalf("seed %d: after out-of-band actions: %v", seed, err)
			}
		}
	}
}

// settled runs a cluster until it has elected a leader and committed entries,
// so that a mutation test starts from meaningful state rather than an empty log.
func settled(t *testing.T, nodes int) (*raftcluster.Cluster, *dst.Engine[raft.Message]) {
	t.Helper()
	cluster := raftcluster.New(nodes)
	engine := dst.New[raft.Message](dst.Config{Seed: 3}, cluster, cluster)
	engine.Watch(cluster.SafetyInvariants()...)

	for tick := uint64(1); tick <= 400; tick++ {
		if err := engine.StepChecked(); err != nil {
			t.Fatalf("unmutated cluster violated an invariant: %v", err)
		}
		if tick%9 == 0 && cluster.Propose(tick) {
			engine.Collect()
		}
	}
	if cluster.Leader() == 0 {
		t.Fatal("no leader after 400 steps")
	}
	if got := cluster.Node(1).Commit; got < 2 {
		t.Fatalf("node 1 committed only %d entries, too few to mutate", got)
	}
	return cluster, engine
}

func requireViolation(t *testing.T, err error, want string) {
	t.Helper()
	var violation *dst.Violation
	if !errors.As(err, &violation) {
		t.Fatalf("error is %T (%v), want *dst.Violation", err, err)
	}
	if violation.Invariant != want {
		t.Fatalf("violation names %q, want %q", violation.Invariant, want)
	}
}

// The three tests below are mutation tests. An invariant that never fires is
// indistinguishable from one that is not evaluated at all, so each deliberately
// corrupts the state the property is supposed to protect.

func TestElectionSafetyDetectsTwoLeadersInOneTerm(t *testing.T) {
	cluster, engine := settled(t, 3)
	for _, id := range []uint32{1, 2} {
		cluster.Node(id).State = raft.Leader
		cluster.Node(id).Term = 99
	}
	requireViolation(t, engine.CheckInvariants(), "election safety")
}

func TestIndexSanityDetectsCommitBeyondLog(t *testing.T) {
	cluster, engine := settled(t, 3)
	node := cluster.Node(1)
	node.Commit = node.LastIndex() + 5
	requireViolation(t, engine.CheckInvariants(), "index sanity")
}

func TestCommittedPrefixDetectsDivergentEntry(t *testing.T) {
	cluster, engine := settled(t, 3)
	node := cluster.Node(1)
	if node.BaseIndex != 0 {
		t.Skipf("node 1 compacted to base %d, so Log is not absolutely indexed", node.BaseIndex)
	}
	node.Log[1].Command = ^uint64(0)
	requireViolation(t, engine.CheckInvariants(), "committed prefix")
}
