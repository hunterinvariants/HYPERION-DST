# Sentinel deterministic engine qualification

Date: 2026-08-17

Commit under test: `1d6ba08`.

## Purpose

The `dst` package is the deterministic execution engine: virtual time, seeded
scheduling, message loss and delay, the execution trace, invariant checking,
and programmable fault injection. It carries no protocol state, so protocols
other than Raft can be driven and checked under the same conditions.
`dst/raftcluster` wires the Raft core behind the engine's `Cluster` and `Wire`
interfaces and packages the Raft safety properties as reusable invariants.
`dst/scenario` makes a run declarable as a file.

This report records the gate run establishing that the engine drives the Raft
core identically to `sim.Simulator` on the qualification host.

## Host

- Linux `sentinel`, Ubuntu 24.04.4 LTS, kernel `6.8.0-137-generic`, x86_64;
- Go `go1.25.13 linux/amd64`.

Every gate in this report is pure Go and issues no `io_uring`, `O_DIRECT`, XDP,
or TC operation.

## Executable qualification

- `go vet ./...` on `windows/amd64`, `linux/amd64`, `linux/arm64`, and
  `darwin/arm64`: pass;
- complete Go race suite `go test ./... -race -count=1`: pass;
- engine/simulator equivalence, 7,000 paired runs under `-race`: pass;
- equivalence negative control: pass;
- packaged Raft invariants under loss, delay, proposal, and restart, 1,000
  seeds x 500 ticks: pass, 46.47 s;
- invariant mutation tests: pass;
- leader partition and heal, 1,000 seeds: pass, 15.92 s;
- asymmetric one-way link failure, 1,000 seeds: pass, 43.59 s;
- `wal.Device` conformance suite against `MemoryDevice` and `FileDevice`: pass;
- a declared scenario executed through `promtact simulate`: pass.

The deterministic seed sweep is not repeated here. It exercises `sim`, and
`sim` carries no logic change in this work; its last
recorded result is `PASS seeds=1000 range=1..1000 steps=1000`, re-run today
and recorded in `benchmarks/sentinel-2026-08-17.md`.

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
| quiet | 3 | 400 | 0 | 0 | - | 14.23 s |
| lossy | 5 | 500 | 75 permille | 5 | - | 33.73 s |
| seed-sweep-profile | 5 | 600 | 50 permille | 5 | every 101 ticks | 48.41 s |
| restart-storm | 5 | 500 | 60 permille | 5 | every 37 ticks | 46.56 s |
| delay-only | 7 | 300 | 0 | 9 | every 71 ticks | 126.58 s |
| compaction | 5 | 500 + 300 | 35 permille | 4 | all five, after compaction | 75.53 s |
| membership | 5 | 150 + 600 + 200 | 25 permille | 4 | nodes 1-3, after transition | 39.45 s |

The compaction and membership campaigns mirror the shapes of the qualified
campaigns in `sim/compaction_test.go` and `sim/membership_test.go`.

Tests pin that registering invariants or an always-allowing injector leaves the
trace hash identical, so neither facility can shift the schedule. The
`dst/raftcluster` package total for the run was 491.49 s under the race
detector.

## Fault injection

`dst.Injector` programs a deterministic network fault. The engine consults
injectors after it has drawn a message's random loss and delay, so the seeded
stream is identical with and without a fault: a run with a partition and a run
without one on the same seed differ only because of the partition, not because
the schedule drifted. Registering no injector leaves behavior bit-identical,
which the equivalence campaigns above re-confirm.

Two campaigns exercise it against Raft, 1,000 seeds each:

- **leader partition and heal.** The elected leader is cut off from the other
  four for a bounded window. The majority must elect a replacement in a
  strictly later term than the isolated leader held, the isolated node must not
  advance its commit index while it holds a minority, and after the window
  closes it must adopt the later term and stop claiming leadership. Safety
  invariants are checked at every step throughout. Pass, 15.92 s.
- **asymmetric link failure.** Node 1 can hear node 2, but node 2 never hears
  node 1, the failure a symmetric partition model cannot express. Safety
  invariants hold across 600 ticks with periodic proposals. Pass, 43.59 s.

Both campaigns fail if their fault dropped no message, so neither can pass
vacuously.

## Declared scenarios

`dst/scenario` parses a run description: seed, cluster size, steps, network
conditions, proposal and restart cadence, and faults with optional time
windows. Parsing is strict, an unknown field, an unknown fault type, a node
outside the cluster, a node on both sides of a split, or a backwards window is
an error, because a scenario that quietly did less than it claimed would
produce evidence for a campaign that never ran. Fourteen rejection cases are
covered by test.

`examples/leader-partition.json` executed on this host:

```
scenario="leader partition and heal" seed=0x4A2C nodes=5 steps=1200 leader=2
proposed=70 max_commit=42 trace=0ec85bc87c9895d2432ac5954c149f47d2a1f1fed20d44a42483f12fcac6f0a4
fault="split[1]|[2 3 4 5]@[200,700)" dropped=945
```

Node 1 was partitioned away and node 2 carried the cluster. The run reports the
drop count per fault so that a campaign whose fault never fired is visible
rather than assumed.

### An observation about cross-platform traces

This report has said in every revision that the equivalence establishes nothing
about cross-platform trace stability. That statement remains accurate about
what the equivalence proves. It is worth recording, though, that the scenario
above produced a bit-identical trace hash, leader, proposal count, commit index,
and drop count on `windows/amd64` and on `linux/amd64`.

This is one matching pair, not a claim of portability. It is consistent with
`math/rand` being specified as reproducible for a fixed seed and with the rest
of the engine being deterministic. If it holds more broadly, a scenario file
paired with an expected trace hash becomes a portable regression fixture; that
is not yet implemented and is not claimed here.

## Storage backend conformance

`storage/storagetest.RunDeviceSuite` checks the ten properties `wal.Log`
requires of a `wal.Device`, so an alternative backend can be verified before it
is trusted with consensus state. `MemoryDevice` and `FileDevice` both pass.

Writing the suite surfaced an underspecified contract. `FileDevice.DurableBytes`
calls `Sync` before reading, so for it every appended byte is reported;
`MemoryDevice.DurableBytes` reports only what `Sync` already made durable,
because modelling crash loss is its purpose. This is not a live defect: `wal.Log`
calls the method once, in `Open`, when nothing is pending, and there the two
readings agree. It was a defect in the interface, which said nothing about which
behavior an implementer owed. The contract is now stated on the interface.

The dead `storage.StableStore` interface, which nothing implemented and nothing
called, was removed. `storage.Entry` remains and is documented as deliberately
distinct from `raft.Entry`: a durable record has no position and must carry its
index, while the consensus core derives the index from the slice position.

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
- Three mutation tests corrupt exactly the state each invariant protects, two
  leaders in one term, a commit index beyond the log, and a divergent committed
  entry, and require the matching property to report the violation.
- Both fault campaigns fail if their injector dropped no message, and the
  partition campaign additionally fails if the majority elected no replacement.
  A fault that silently matched nothing would otherwise let a run report
  success for a scenario that never happened.

## Claim boundary

The equivalence is a **relative** comparison between two implementations
executing on the same host in the same process. It is not a comparison against
a recorded reference hash, and it therefore establishes nothing about
cross-platform or cross-version trace stability. The single cross-platform
match noted above is an observation, not a claim.

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
a run that makes no progress violates nothing. The partition campaign asserts
progress explicitly, by requiring a replacement leader, precisely because the
invariants would not have noticed its absence.

The fault facility is deterministic and in-process. It shares no code and no
guarantees with the kernel-level injection in `chaos` and `bpf/`, whose safety
guards (a dedicated `promtact-*` namespace, validated CIDRs, bounded delay and
loss) are unchanged and are not extension points.

The device conformance suite states the properties `wal.Log` relies on. Passing
it does not establish that a backend is durable on real hardware; that remains
the subject of the io_uring and NVMe gates.

The race suite result shows that the engine starts no concurrency of its own.
It does not show that the engine is safe under concurrent use; `dst.Engine` is
documented as single-threaded and is not safe for concurrent use.

## Scheduling cost

The engine's in-flight queue is a binary heap keyed on `(at, seq)`. `sim` keeps
a linear scan over the same queue, which is what the equivalence campaigns
compare against: if the heap ordered deliveries differently anywhere, the paired
trace hashes would diverge. The key is a total order and the element a scan
selects is the global minimum whenever any message is due, so the two orders
coincide by construction as well as by measurement.

Measured on this host, running the same benchmarks against each scheduler. The small topologies are reported as
medians of ten samples at automatically scaled iteration counts, roughly three
thousand runs per sample; the large ones as medians of five single-run samples,
which is adequate only because the effect there is large.

| Topology | Linear scan | Heap | |
|---|---:|---:|---|
| 5 nodes, delay 0 | 351 us | 340 us | -3% |
| 5 nodes, delay 5 | 375 us | 387 us | +3% |
| 16 nodes, delay 32 | 7.44 ms | 5.70 ms | 1.3x faster |
| 32 nodes, delay 128 | 55.1 ms | 11.8 ms | 4.7x faster |

At the topologies the campaigns use the two are indistinguishable; the value is
the removal of an `O(queue)` cost per delivery that reaches a factor of nearly
five at thirty-two nodes with a delay bound of 128. That is the regime a
framework user testing a wide topology reaches, not the regime these campaigns
use.


## Artifacts

Raw gate output on the qualification host:

- `benchmarks/artifacts/sentinel-phase6-20260817T152258Z/`.

These paths are ignored by the repository; the reports in `benchmarks/` cite
them, the raw files stay on the qualification host.

## Relationship to the rest of the evidence

The consensus core, its storage, and the service around it were qualified by
the gates recorded in `benchmarks/sentinel-2026-08-17.md`, run on the same host
on the same day at the same commit. This report covers the framework layer
built on top of them: the engine, its invariants, its fault injection, the
scenario format, and the storage conformance suite. That layer is not part of
any roadmap phase and claims no phase acceptance.

`sim` is retained deliberately. It is the reference the equivalence campaigns
compare against, and removing it would remove the gate.
