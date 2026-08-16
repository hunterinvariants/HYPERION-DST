# Sentinel deterministic engine equivalence

Date: 2026-08-16

Commit: `a2a6b5e` on branch `framework/dst-engine`.

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

The kernel differs from the `6.8.0-136-generic` recorded for the frozen
baseline. Every gate in this report is pure Go and issues no `io_uring`,
`O_DIRECT`, XDP, or TC operation, so the difference does not bear on these
results. It does bear on any future re-run of the hardware gates.

## Executable qualification

- `go vet ./...`: pass;
- complete Go race suite `go test ./... -race -count=1`: pass;
- deterministic seed sweep, 1,000 seeds x 1,000 virtual ticks with periodic
  proposal and crash/restart: `PASS seeds=1000 range=1..1000 steps=1000`;
- engine/simulator equivalence, 5,000 paired runs under `-race`: pass;
- equivalence negative control: pass.

## Equivalence method

Each paired run drives `sim.Simulator` and `dst.Engine` over `raftcluster` from
the same seed, issuing identical proposals and restarts to both. The two are
compared at **every tick** on the trace hash, a running SHA-256 over the virtual
time, sequence number, endpoints, message type, and term of every delivery. At
the end of each run the following are compared per node:

- `Term`, `State`, `Commit`, `Applied`, `BaseIndex`, and `LastIndex()`;
- every committed log entry from `BaseIndex + 1` through `Commit`.

Five scenarios ran 1,000 seeds each:

| Scenario | Nodes | Steps | Drop | Max delay | Propose | Restart | Time |
|---|---:|---:|---:|---:|---:|---:|---:|
| quiet | 3 | 400 | 0 | 0 | every 13 | - | 12.82 s |
| lossy | 5 | 500 | 75 permille | 5 | every 17 | - | 29.31 s |
| seed-sweep-profile | 5 | 600 | 50 permille | 5 | every 17 | every 101 | 39.60 s |
| restart-storm | 5 | 500 | 60 permille | 5 | every 19 | every 37 | 37.58 s |
| delay-only | 7 | 300 | 0 | 9 | every 11 | every 71 | 131.14 s |

Total: 250.46 s under the race detector, no divergence.

## Negative control

An equality assertion is worthless if the compared values cannot differ.
`TestEquivalenceComparisonHasTeeth` runs the two implementations under
different seeds and requires the trace hashes to diverge. It passes, so the
5,000 agreements above are a result and not an artifact of the test setup.

## Claim boundary

The equivalence is a **relative** comparison between two implementations
executing on the same host in the same process. It is not a comparison against
a recorded reference hash, and it therefore establishes nothing about
cross-platform or cross-version trace stability.

Equivalence is established for the transitions the listed scenarios exercise:
election, pre-vote, replication, commit, message loss, message delay, and
crash/restart recovery from the durable WAL, at three, five, and seven nodes
over at most 600 virtual steps.

Equivalence is **not** established for:

- log compaction and `InstallSnapshot` catch-up;
- joint and final membership transitions;
- node counts above seven or runs longer than 600 steps.

`sim` supports `Compact`, `ProposeJoint`, and `ProposeFinal`; `dst/raftcluster`
does not yet expose them, so those paths have never executed through the
engine. Until they do, `sim` remains the only qualified driver for the
snapshot and membership campaigns.

The race suite result shows that the engine starts no concurrency of its own.
It does not show that the engine is safe under concurrent use; `dst.Engine` is
documented as single-threaded and is not safe for concurrent use.

## Artifacts

Raw gate output on the qualification host:

- `benchmarks/artifacts/dst-engine-env.txt`;
- `benchmarks/artifacts/dst-engine-race.txt`;
- `benchmarks/artifacts/dst-engine-equivalence.txt`;
- `benchmarks/artifacts/dst-engine-seeds.txt`.

## Relationship to the frozen baseline

This report covers branch work and does not amend the frozen evidence index.
`EVIDENCE.md`, `STATUS.md`, and `ROADMAP.md` continue to describe the qualified
reference baseline at `8148e35` and are deliberately unchanged.
