package raftcluster_test

import (
	"testing"

	"github.com/hunterinvariants/promtact/dst"
	"github.com/hunterinvariants/promtact/dst/raftcluster"
	"github.com/hunterinvariants/promtact/raft"
)

// TestMajorityElectsANewLeaderWhileTheLeaderIsPartitioned drives the scenario
// a consensus protocol exists for: the leader is cut off from the majority,
// the majority must elect a replacement, and no safety property may break
// while two nodes both believe they lead.
//
// It is also the worked example of the fault facility: the partition is
// declared once, scoped to a virtual time window, and the run stays fully
// deterministic.
func TestMajorityElectsANewLeaderWhileTheLeaderIsPartitioned(t *testing.T) {
	const (
		settle = 200
		heal   = 700
	)
	for seed := int64(1); seed <= equivalenceSeeds(t); seed++ {
		cluster := raftcluster.New(5)
		engine := dst.New[raft.Message](dst.Config{Seed: seed}, cluster, cluster)
		engine.Watch(cluster.SafetyInvariants()...)

		if err := engine.RunChecked(settle); err != nil {
			t.Fatalf("seed %d: before the partition: %v", seed, err)
		}
		original := cluster.Leader()
		if original == 0 {
			t.Fatalf("seed %d: no leader after %d quiet steps", seed, settle)
		}
		originalTerm := cluster.Node(original).Term
		originalCommit := cluster.Node(original).Commit

		// Cut the leader off from everyone else, then heal. The window closes
		// on its own, so the run needs no mid-flight mutation.
		minority := []uint32{original}
		majority := make([]uint32, 0, 4)
		for _, id := range cluster.Nodes() {
			if id != original {
				majority = append(majority, id)
			}
		}
		partition := dst.During(settle, heal, dst.Split(minority, majority))
		engine.Inject(partition)

		// Stop one step short of the window closing, so the assertions below
		// observe the cluster while it is still cut in two.
		if err := engine.RunChecked(heal - settle - 1); err != nil {
			t.Fatalf("seed %d: during the partition: %v", seed, err)
		}
		if engine.InjectedDrops()[partition.Name()] == 0 {
			t.Fatalf("seed %d: the partition never dropped a message", seed)
		}

		replacement := uint32(0)
		for _, id := range majority {
			if cluster.Node(id).State == raft.Leader {
				replacement = id
			}
		}
		if replacement == 0 {
			t.Fatalf("seed %d: the majority elected no leader while node %d was cut off", seed, original)
		}
		// A replacement must lead in a strictly later term than the leader it
		// displaced held when it was cut off; anything else would mean two
		// leaders shared a term.
		if got := cluster.Node(replacement).Term; got <= originalTerm {
			t.Fatalf("seed %d: replacement %d leads in term %d, not later than the isolated leader's %d",
				seed, replacement, got, originalTerm)
		}
		// The isolated node holds a minority and therefore must not have
		// committed anything further. This is the property the partition exists
		// to test, and the one whose failure would be a genuine safety bug.
		if got := cluster.Node(original).Commit; got != originalCommit {
			t.Fatalf("seed %d: isolated node %d advanced its commit index from %d to %d",
				seed, original, originalCommit, got)
		}

		// After the window closes the isolated node must rejoin, adopt the
		// later term, and stop claiming leadership.
		if err := engine.RunChecked(400); err != nil {
			t.Fatalf("seed %d: after healing: %v", seed, err)
		}
		rejoined := cluster.Node(original)
		if rejoined.Term < cluster.Node(replacement).Term {
			t.Fatalf("seed %d: node %d rejoined at term %d, behind the cluster's %d",
				seed, original, rejoined.Term, cluster.Node(replacement).Term)
		}
		if rejoined.State == raft.Leader && cluster.Leader() != original {
			t.Fatalf("seed %d: node %d still reports leadership after rejoining", seed, original)
		}
		if err := engine.CheckInvariants(); err != nil {
			t.Fatalf("seed %d: final: %v", seed, err)
		}
	}
}

// TestAsymmetricLinkFailureKeepsSafety exercises the failure a symmetric
// partition model cannot express: node A hears B, but B never hears A.
func TestAsymmetricLinkFailureKeepsSafety(t *testing.T) {
	for seed := int64(1); seed <= equivalenceSeeds(t); seed++ {
		cluster := raftcluster.New(5)
		engine := dst.New[raft.Message](dst.Config{Seed: seed, MaxDelay: 3}, cluster, cluster)
		engine.Watch(cluster.SafetyInvariants()...)
		broken := dst.Link(1, 2)
		engine.Inject(broken)

		for tick := uint64(1); tick <= 600; tick++ {
			if err := engine.StepChecked(); err != nil {
				t.Fatalf("seed %d: %v", seed, err)
			}
			if tick%23 == 0 && cluster.Propose(uint64(seed)<<32|tick) {
				engine.Collect()
			}
			if err := engine.CheckInvariants(); err != nil {
				t.Fatalf("seed %d: after proposing: %v", seed, err)
			}
		}
		if engine.InjectedDrops()[broken.Name()] == 0 {
			t.Fatalf("seed %d: the one-way link failure never fired", seed)
		}
	}
}
