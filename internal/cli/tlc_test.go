package cli

import (
	"errors"
	"testing"
)

// successOutput reproduces the shape of a completed TLC 1.7 run, with the
// numbers this repository actually recorded in
// benchmarks/sentinel-phase6-2026-07-28.md.
const successOutput = `TLC2 Version 2.19 of Day Month 20?? (rev: 6e14a8d)
Running breadth-first search Model-Checking with fp 44 and seed ...
Parsing file /opt/promtact/Promtact/verification/tla/PromtactRaft.tla
Starting... (2026-07-28 04:53:54)
Computing initial states...
Finished computing initial states: 1 distinct state generated at 2026-07-28 04:53:56.
Progress(14) at 2026-07-28 04:54:56: 1,234,567 states generated, 234,567 distinct states found
Model checking completed. No error has been found.
  Estimates of the probability that TLC did not check all reachable states
  because two distinct states had the same fingerprint:
  calculated (optimistic):  val = 1.3E-5
  based on the actual fingerprints:  val = 3.3E-6
46667923 states generated, 6121927 distinct states found, 0 states left on queue.
The depth of the complete state graph search is 25.
Finished in 05min 12s at (2026-07-28 04:59:06)
`

const violationOutput = `TLC2 Version 2.19 of Day Month 20??
Starting... (2026-08-16 21:00:00)
Error: Invariant ElectionSafety is violated.
Error: The behavior up to this point is:
State 1: <Initial predicate>
State 2: <AppendEntries line 88, col 1 to line 99, col 20 of module PromtactRaft>
1234 states generated, 456 distinct states found, 78 states left on queue.
The depth of the complete state graph search is 7.
Finished in 03s at (2026-08-16 21:00:03)
`

func TestParseTLCReadsACompletedRun(t *testing.T) {
	result, err := ParseTLC(successOutput)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !result.Completed {
		t.Error("a run reporting no error was not marked complete")
	}
	if !result.Exhaustive() {
		t.Error("a completed run with an empty queue was not marked exhaustive")
	}
	if len(result.Violations) != 0 {
		t.Errorf("violations = %v, want none", result.Violations)
	}
	if result.StatesGenerated != 46667923 {
		t.Errorf("states generated = %d", result.StatesGenerated)
	}
	if result.DistinctStates != 6121927 {
		t.Errorf("distinct states = %d", result.DistinctStates)
	}
	if result.StatesLeftOnQueue != 0 {
		t.Errorf("states left on queue = %d", result.StatesLeftOnQueue)
	}
	if result.Depth != 25 {
		t.Errorf("depth = %d", result.Depth)
	}
	if result.CollisionOptimistic != "1.3E-5" || result.CollisionActual != "3.3E-6" {
		t.Errorf("collision estimates = %q and %q",
			result.CollisionOptimistic, result.CollisionActual)
	}
	if result.Duration != "05min 12s" {
		t.Errorf("duration = %q", result.Duration)
	}
}

// TestParseTLCReadsAViolation is the negative control. A parser that only ever
// saw passing output could report every run as a pass, which is the one
// failure mode that would matter here.
func TestParseTLCReadsAViolation(t *testing.T) {
	result, err := ParseTLC(violationOutput)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Completed {
		t.Error("a violated run was marked complete")
	}
	if result.Exhaustive() {
		t.Error("a violated run was marked exhaustive")
	}
	if len(result.Violations) != 1 || result.Violations[0] != "ElectionSafety" {
		t.Fatalf("violations = %v, want [ElectionSafety]", result.Violations)
	}
	if result.StatesLeftOnQueue != 78 {
		t.Errorf("states left on queue = %d, want 78", result.StatesLeftOnQueue)
	}
}

func TestParseTLCReadsDeadlock(t *testing.T) {
	const output = `Error: Deadlock reached.
12 states generated, 8 distinct states found, 3 states left on queue.
`
	result, err := ParseTLC(output)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(result.Violations) != 1 || result.Violations[0] != "deadlock" {
		t.Fatalf("violations = %v, want [deadlock]", result.Violations)
	}
}

func TestParseTLCReportsSeveralViolationsOnce(t *testing.T) {
	const output = `Error: Invariant ElectionSafety is violated.
Error: Invariant ElectionSafety is violated.
Error: Invariant SnapshotSafety is violated.
5 states generated, 5 distinct states found, 1 states left on queue.
`
	result, err := ParseTLC(output)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(result.Violations) != 2 {
		t.Fatalf("violations = %v, want two distinct names", result.Violations)
	}
}

// TestParseTLCRefusesUnreadableOutput is the property that keeps invented
// numbers out of an evidence document: output without a state count is an
// error, not a result full of zeros.
func TestParseTLCRefusesUnreadableOutput(t *testing.T) {
	for _, output := range []string{
		"",
		"Error: Could not read the module.\n",
		"java: command not found\n",
		"Model checking completed. No error has been found.\n",
	} {
		if _, err := ParseTLC(output); !errors.Is(err, ErrUnparsedTLC) {
			t.Errorf("ParseTLC(%q) returned %v, want ErrUnparsedTLC", output, err)
		}
	}
}

// TestCompletedRequiresNoViolation pins the belt-and-braces rule: if TLC ever
// printed both a completion banner and a violation, the violation wins.
func TestCompletedRequiresNoViolation(t *testing.T) {
	const output = `Model checking completed. No error has been found.
Error: Invariant TypeOK is violated.
9 states generated, 9 distinct states found, 0 states left on queue.
`
	result, err := ParseTLC(output)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Completed || result.Exhaustive() {
		t.Fatal("a run with a reported violation was treated as complete")
	}
}

// TestExhaustiveRequiresAnEmptyQueue distinguishes exhausting the bounded
// space from stopping early, which is the difference between the claim the
// evidence makes and a weaker one.
func TestExhaustiveRequiresAnEmptyQueue(t *testing.T) {
	const output = `Model checking completed. No error has been found.
100 states generated, 50 distinct states found, 7 states left on queue.
`
	result, err := ParseTLC(output)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !result.Completed {
		t.Error("run should be marked complete")
	}
	if result.Exhaustive() {
		t.Error("a run with states left on the queue was marked exhaustive")
	}
}
