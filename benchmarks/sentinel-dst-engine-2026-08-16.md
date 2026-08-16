# Sentinel deterministic engine equivalence

Date: 2026-08-16

Commit under test: `036a3ef` on branch `framework/dst-engine`.

## Purpose

The `dst` package extracts the deterministic execution engine — virtual time,
seeded scheduling, message loss and delay, and the execution trace — from the
Raft-specific simulator in `sim`, so that protocols other than Raft can be
driven under the same conditions. `dst/raftcluster` re-implements the Raft
wiring behind the engine's `Cluster` and `Wire` interfaces.

The change is additive: no file outside `dst/` was modified, so every
previously qualified binary and code path is unchanged. This report records the
gate run that establishes the new package is behaviorally identical to the
frozen simulator on the qualification host.

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
- deterministic seed sweep, 1,000 seeds x 1,000 virtual ticks with periodic
  proposal and crash/restart: `PASS seeds=1000 range=1..1000 steps=1000`;
- engine/simulator equivalence, 7,000 paired runs under `-race`: pass;
- equivalence negative control: pass.

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
| quiet | 3 | 400 | 0 | 0 | - | 12.67 s |
| lossy | 5 | 500 | 75 permille | 5 | - | 29.61 s |
| seed-sweep-profile | 5 | 600 | 50 permille | 5 | every 101 ticks | 42.69 s |
| restart-storm | 5 | 500 | 60 permille | 5 | every 37 ticks | 39.35 s |
| delay-only | 7 | 300 | 0 | 9 | every 71 ticks | 144.24 s |
| compaction | 5 | 500 + 300 | 35 permille | 4 | all five, after compaction | 68.59 s |
| membership | 5 | 150 + 600 + 200 | 25 permille | 4 | nodes 1-3, after transition | 34.78 s |

Total: 372.96 s under the race detector, no divergence.

The compaction and membership campaigns mirror the shapes of the qualified
campaigns in `sim/compaction_test.go` and `sim/membership_test.go`.

## Guards against vacuous results

An equality assertion is worthless if the compared values cannot differ, and a
campaign is worthless if it never reaches the state it claims to exercise.
Three guards address this, and none of them fired at 1,000 seeds:

- `TestEquivalenceComparisonHasTeeth` runs the two implementations under
  different seeds and requires the trace hashes to diverge. It passes, so the
  agreements above are a result and not an artifact of the test setup.
- The compaction campaign fails if no node was compactable, and fails again if
  no node recovers a `BaseIndex` above zero after the restarts. The second
  guard is the one that matters: it distinguishes `Compact` merely reporting
  success from the snapshot path actually carrying state across a restart.
- The membership campaign fails if the joint configuration is never committed,
  and requires nodes 1 through 3 to end with the final voter mask and an empty
  joint mask.

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

- `benchmarks/artifacts/dst-engine-036a3ef-env.txt`;
- `benchmarks/artifacts/dst-engine-036a3ef-race.txt`;
- `benchmarks/artifacts/dst-engine-036a3ef-equivalence.txt`;
- `benchmarks/artifacts/dst-engine-036a3ef-seeds.txt`.

## Relationship to the qualified baseline

`main` carries the engine as of `fb82eed`; the qualified reference baseline it
builds on is `8148e35`. This report covers branch work and does not amend the
frozen evidence index: `EVIDENCE.md`, `STATUS.md`, and `ROADMAP.md` continue to
describe the six completed roadmap phases and are deliberately unchanged. The
`dst` package is not part of any roadmap phase and claims no phase acceptance.
