# Sentinel deterministic engine qualification

Date: 2026-08-16

Commit under test: `dc8212b` on branch `framework/dst-invariants`.

## Purpose

The `dst` package extracts the deterministic execution engine — virtual time,
seeded scheduling, message loss and delay, the execution trace, and invariant
checking — from the Raft-specific simulator in `sim`, so that protocols other
than Raft can be driven and checked under the same conditions.
`dst/raftcluster` re-implements the Raft wiring behind the engine's `Cluster`
and `Wire` interfaces and packages the Raft safety properties as reusable
invariants.

The change is additive. Outside `dst/`, only `.gitignore` is touched, so every
previously qualified binary and code path is unchanged. This report records the
gate run that establishes the package is behaviorally identical to the frozen
simulator on the qualification host.

## Host

- Linux `sentinel`, kernel `6.8.0-137-generic`, x86_64;
- Go `go1.25.0 linux/amd64`.

The kernel differs from the `6.8.0-136-generic` recorded for the qualified
baseline. Every gate in this report is pure Go and issues no `io_uring`,
`O_DIRECT`, XDP, or TC operation, so the difference does not bear on these
results. It does bear on any future re-run of the hardware gates.

## Executable qualification

- `go vet ./...`: pass;
- complete Go race suite `go test ./... -race -count=1`: pass;
- engine/simulator equivalence, 7,000 paired runs under `-race`: pass;
- equivalence negative control: pass;
- packaged Raft invariants under loss, delay, proposal, and restart, 1,000
  seeds x 500 ticks: pass;
- invariant mutation tests: pass.

The deterministic seed sweep is not repeated here. It exercises `sim`, and
`git diff origin/main..dc8212b` shows `sim` unmodified on this branch; its last
recorded result is `PASS seeds=1000 range=1..1000 steps=1000` at `036a3ef`.

## Equivalence method

Each paired run drives `sim.Simulator` and `dst.Engine` over `raftcluster` from
the same seed, issuing identical proposals, configuration changes, compactions,
and restarts to both. The two are compared at **every tick** on the trace hash,
a running SHA-256 over the virtual time, sequence number, endpoints, message
type, and term of every delivery. At each checkpoint the following are compared
per node:

- `Term`, `State`, `Commit`, `Applied`, `BaseIndex`, and `LastIndex()`;
- `VoterMasks()`, covering both the old and the joint configuration;
- every committed log entry from `BaseIndex + 1` through `Commit`.

Seven campaigns ran 1,000 seeds each, for 7,000 paired runs.

| Campaign | Nodes | Steps | Drop | Max delay | Restart | Time |
|---|---:|---:|---:|---:|---|---:|
| quiet | 3 | 400 | 0 | 0 | - | 14.27 s |
| lossy | 5 | 500 | 75 permille | 5 | - | 31.07 s |
| seed-sweep-profile | 5 | 600 | 50 permille | 5 | every 101 ticks | 45.65 s |
| restart-storm | 5 | 500 | 60 permille | 5 | every 37 ticks | 44.63 s |
| delay-only | 7 | 300 | 0 | 9 | every 71 ticks | 156.90 s |
| compaction | 5 | 500 + 300 | 35 permille | 4 | all five, after compaction | 79.10 s |
| membership | 5 | 150 + 600 + 200 | 25 permille | 4 | nodes 1-3, after transition | 45.75 s |

The compaction and membership campaigns mirror the shapes of the qualified
campaigns in `sim/compaction_test.go` and `sim/membership_test.go`.

Adding the invariant facility required modifying `dst/engine.go`. `Step` and
`Run` are unchanged, and a test pins that registering invariants leaves the
trace hash identical, but the equivalence campaigns above were re-run in full
at this commit rather than carried over.

## Invariants

`dst.Invariant` is a named property the engine evaluates through `StepChecked`,
`RunChecked`, and `CheckInvariants`. `Step` and `Run` never evaluate
invariants, so an existing run loop keeps its exact behavior.

A failure is reported as a `*dst.Violation` carrying the property name, the
step at which it was detected, and the trace hash at that point. Because the
same seed and the same caller actions reproduce the same trace hash, a
violation is a reproducible coordinate in a specific run rather than only a
message.

`raftcluster.SafetyInvariants()` packages the properties `sim.CheckSafety`
checks, split so that a report identifies which one broke: election safety,
index sanity, and committed-prefix agreement. Node iteration is in ascending
identifier order rather than map order, so violation messages are themselves
deterministic.

Run under loss, delay, proposals, and periodic restarts across 1,000 seeds and
500 ticks each: no violation, 52.94 s.

## Guards against vacuous results

An equality assertion is worthless if the compared values cannot differ, a
campaign is worthless if it never reaches the state it claims to exercise, and
an invariant that never fires is indistinguishable from one that is never
evaluated. Each of these has a guard, and none of them fired:

- `TestEquivalenceComparisonHasTeeth` runs the two implementations under
  different seeds and requires the trace hashes to diverge.
- The compaction campaign fails if no node was compactable, and fails again if
  no node recovers a `BaseIndex` above zero after the restarts. The second
  guard distinguishes `Compact` merely reporting success from the snapshot path
  actually carrying state across a restart.
- The membership campaign fails if the joint configuration is never committed,
  and requires nodes 1 through 3 to end with the final voter mask and an empty
  joint mask.
- Three mutation tests corrupt exactly the state each invariant protects — two
  leaders in one term, a commit index beyond the log, and a divergent committed
  entry — and require the matching property to report the violation.

## Claim boundary

The equivalence is a **relative** comparison between two implementations
executing on the same host in the same process. It is not a comparison against
a recorded reference hash, and it therefore establishes nothing about
cross-platform or cross-version trace stability.

Equivalence is established for the transitions the campaigns exercise:
election, pre-vote, replication, commit, message loss, message delay,
crash/restart recovery from the durable WAL, log compaction with
`InstallSnapshot` catch-up, and joint-to-final membership transitions under the
dual-majority commit rule, at three, five, and seven nodes.

Equivalence is **not** established for:

- node counts above seven;
- runs longer than 950 virtual steps;
- ReadIndex and leadership transfer.

The last item is not a gap relative to `sim`: neither simulator drives those
paths. They are exercised by the package-level tests in `raft`, which are
unchanged and unaffected by this work.

The invariants are safety properties only. No liveness property is checked, and
a run that makes no progress violates nothing.

The race suite result shows that the engine starts no concurrency of its own.
It does not show that the engine is safe under concurrent use; `dst.Engine` is
documented as single-threaded and is not safe for concurrent use.

## Known performance characteristic

`dst.Engine.nextReady` is a linear scan over the in-flight queue, carried over
verbatim from `sim` so that delivery order is preserved exactly. The
`delay-only` campaign is the visible cost: seven nodes at a delay bound of nine
hold enough messages in flight to dominate the run time.

A binary heap keyed on `(at, seq)` would remove the scan without changing
delivery order, because that key is a total order and the element the scan
selects is the global minimum whenever any message is due. The equivalence
campaigns in this report are the gate that would prove such a change safe.

## Artifacts

Raw gate output on the qualification host:

- `benchmarks/artifacts/dst-invariants-race.txt`;
- `benchmarks/artifacts/dst-invariants-equivalence.txt`;
- `benchmarks/artifacts/dst-engine-036a3ef-seeds.txt` (seed sweep, `sim`
  unmodified since).

These paths are ignored by the repository; the reports in `benchmarks/` cite
them, the raw files stay on the qualification host.

## Relationship to the qualified baseline

`main` carries the engine as of `3397835`, which builds on the qualified
reference baseline `8148e35`. This report covers branch work and does not amend
the frozen evidence index: `EVIDENCE.md`, `STATUS.md`, and `ROADMAP.md`
continue to describe the six completed roadmap phases and are deliberately
unchanged. The `dst` package is not part of any roadmap phase and claims no
phase acceptance.
