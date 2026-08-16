package dst_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/hunterinvariants/hyperion/dst"
)

var errTooManyTicks = errors.New("too many ticks")

// ticksBelow fails once node 1 has ticked at least limit times. Because the
// engine ticks every node exactly once per step, the violation is detected at
// step == limit, which makes the reported step directly assertable.
func ticksBelow(r *ring, limit uint64) dst.Invariant {
	return dst.InvariantFunc{
		Label: "ticks below limit",
		Fn: func() error {
			if got := r.nodes[1].ticks; got >= limit {
				return fmt.Errorf("%w: node 1 reached %d", errTooManyTicks, got)
			}
			return nil
		},
	}
}

func TestRunCheckedReportsReproductionData(t *testing.T) {
	r := newRing(3, 2)
	engine := dst.New[ping](dst.Config{Seed: 4, MaxDelay: 3}, r, r)
	engine.Watch(ticksBelow(r, 10))

	err := engine.RunChecked(100)
	if err == nil {
		t.Fatal("RunChecked returned nil despite a violated invariant")
	}

	var violation *dst.Violation
	if !errors.As(err, &violation) {
		t.Fatalf("error is %T, want *dst.Violation", err)
	}
	if violation.Invariant != "ticks below limit" {
		t.Fatalf("violation names %q", violation.Invariant)
	}
	if violation.Step != 10 {
		t.Fatalf("violation reports step %d, want 10", violation.Step)
	}
	if violation.Trace != engine.TraceHash() {
		t.Fatalf("violation trace %s does not match engine trace %s",
			violation.Trace, engine.TraceHash())
	}
	if !errors.Is(err, errTooManyTicks) {
		t.Fatal("violation does not unwrap to the invariant's own error")
	}
	if engine.Now != 10 {
		t.Fatalf("engine advanced to step %d after the violation, want 10", engine.Now)
	}
}

// TestRunIgnoresInvariants pins that adding invariants cannot change the
// behavior of an existing run loop.
func TestRunIgnoresInvariants(t *testing.T) {
	plain := newRing(3, 2)
	plainEngine := dst.New[ping](dst.Config{Seed: 4, MaxDelay: 3}, plain, plain)
	plainEngine.Run(100)

	watched := newRing(3, 2)
	watchedEngine := dst.New[ping](dst.Config{Seed: 4, MaxDelay: 3}, watched, watched)
	watchedEngine.Watch(ticksBelow(watched, 10))
	watchedEngine.Run(100)

	if watchedEngine.Now != 100 {
		t.Fatalf("Run stopped at step %d despite a violated invariant", watchedEngine.Now)
	}
	if a, b := plainEngine.TraceHash(), watchedEngine.TraceHash(); a != b {
		t.Fatalf("watching invariants changed the trace: %s != %s", a, b)
	}
}

func TestRunCheckedWithoutInvariantsMatchesRun(t *testing.T) {
	plain := newRing(4, 3)
	plainEngine := dst.New[ping](dst.Config{Seed: 8, DropPermille: 50, MaxDelay: 4}, plain, plain)
	plainEngine.Run(200)

	checked := newRing(4, 3)
	checkedEngine := dst.New[ping](dst.Config{Seed: 8, DropPermille: 50, MaxDelay: 4}, checked, checked)
	if err := checkedEngine.RunChecked(200); err != nil {
		t.Fatalf("RunChecked failed without registered invariants: %v", err)
	}
	if a, b := plainEngine.TraceHash(), checkedEngine.TraceHash(); a != b {
		t.Fatalf("RunChecked diverged from Run: %s != %s", a, b)
	}
}

// TestInvariantsAreCheckedInRegistrationOrder matters because the first
// failure wins: a report should name the property the caller considers most
// fundamental, not whichever happened to be evaluated first.
func TestInvariantsAreCheckedInRegistrationOrder(t *testing.T) {
	r := newRing(3, 2)
	engine := dst.New[ping](dst.Config{Seed: 1}, r, r)
	alwaysFails := func(label string) dst.Invariant {
		return dst.InvariantFunc{Label: label, Fn: func() error { return errors.New("failed") }}
	}
	engine.Watch(alwaysFails("first"), alwaysFails("second"))

	var violation *dst.Violation
	if err := engine.StepChecked(); !errors.As(err, &violation) {
		t.Fatalf("error is %T, want *dst.Violation", err)
	}
	if violation.Invariant != "first" {
		t.Fatalf("reported %q, want the first registered invariant", violation.Invariant)
	}
}
