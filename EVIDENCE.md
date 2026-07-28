# HYPERION-DST evidence

Status: **Specification & Qualification Complete**

This file is the frozen evidence index for the qualified reference baseline.
Every result is tied to a named configuration or finite verification bound.

## Qualification summary

| Area | Result | Evidence |
|---|---|---|
| Raft and DST | phases 1-4 complete; crash, restart, snapshot, compaction, membership, ReadIndex, and leadership-transfer gates pass | [Phase 4 Sentinel report](benchmarks/sentinel-phase4-2026-07-28.md) |
| Distributed service | five-process failover, backup/restore, bounded backpressure, metrics, and shutdown gates pass | [Phase 5 Sentinel report](benchmarks/sentinel-phase5-2026-07-28.md) |
| Linearizability | Jepsen/Knossos register history reports `valid? true` during process and network faults | [Phase 6 Sentinel report](benchmarks/sentinel-phase6-2026-07-28.md) |
| Storage faults | ENOSPC, append EIO, sync EIO, torn writes, bit rot, misdirected writes, phantom reads, and fail-stop ACK rules pass | [Phase 6 Sentinel report](benchmarks/sentinel-phase6-2026-07-28.md) |
| Kernel paths | registered file/buffer, `O_DIRECT`, `WRITE_FIXED`, CQE validation, `FSYNC`, XDP/TC verification, injection, and cleanup pass | [Sentinel block-device report](benchmarks/sentinel-block-device-2026-07-28.md) |
| NVMe | 10,000 raw durable operations pass on the named GCP Local SSD NVMe configuration | [GCP Local SSD NVMe report](benchmarks/gcp-local-nvme-2026-07-28.md) |
| Formal model | bounded TLC exploration completes with no invariant violation | [Phase 6 Sentinel report](benchmarks/sentinel-phase6-2026-07-28.md) |

## NVMe result

Configuration: GCP `n2-custom-4-12288`, Ubuntu 24.04.4 LTS, kernel
`6.17.0-1021-gcp`, `/dev/nvme0n1`, 4 KiB blocks, registered io_uring
file/buffer, `O_DIRECT`, `WRITE_FIXED`, CQE validation, and one completed
`FSYNC` per operation.

| Operations | Throughput | p50 | p99 | Maximum |
|---:|---:|---:|---:|---:|
| 10,000 | 10,132 ops/s | 95.074 us | 132.080 us | 551.414 us |

SMART reported no critical warning, media error, or error-log entry before or
after the run. These numbers describe this GCP Local SSD configuration; they
are not generalized hardware claims.

## Linearizability result

The live five-node workload used Jepsen with the Knossos register checker. The
raw history is checked in, so the numbers below can be recounted rather than
taken on trust:

`jepsen/store/hyperion-live-linearizability/20260728T045906.512+0200/`
(`results.edn`, `history.txt`, `jepsen.log`).

```clojure
{:linearizable {:valid? true,
                :model #knossos.model.Register{:value 263637},
                :final-paths (),
                :configs ()},
 :timeline {:valid? true},
 :valid? true}
```

Exactly what that history contains, because the composition matters more than
the verdict:

| Property | Value |
|---|---:|
| Command | `lein run test --no-ssh --time-limit 15` |
| Wall-clock window | 15 s, concurrency 5 |
| Invoked operations | 150 |
| Completed `:ok` | 25 |
| Rejected `:fail` (`:not-leader`) | 75 |
| Indeterminate `:info` | 50 |

Two qualifications a reader needs. First, faults are applied **outside** Jepsen:
`scripts/phase5-sentinel-cluster.sh` runs a background subshell that puts
`netem loss 100%` on one node's host veth for about four seconds inside the
fifteen-second window and then removes it. The test itself uses `gen/clients`
with the noop nemesis from `tests/noop-test`, so no nemesis operations appear in
the history and the fault window cannot be read off it -- the `Connection
refused` `:info` entries are the only trace. Second, a leader process kill
happens earlier in that script, before the measured window, not during it.

So the supported claim is: **under a four-second total network isolation of one
node, a 15-second five-client register history containing 25 completed
operations was linearizable.** That is a real result on a real cluster and it is
now re-checkable. It is not a long-running, nemesis-driven Jepsen campaign, and
25 completed operations is a small history for a linearizability argument.
Widening it is tracked in `ROADMAP.md` rather than claimed here.

A reduced version of the same workload now also runs in CI on every push
(`.github/workflows/jepsen.yml`): same client protocol, same Knossos register
model, same checker, against a five-process loopback cluster. It fails the build
unless the checker reports `:valid? true`, and the full history is uploaded as a
build artifact. The CI run is smaller and exercises process faults only -- a
loopback cluster has no namespaces to partition -- so the networked Sentinel gate
above remains the authority for behaviour under network faults.

Knossos is the checker actually used by this repository; no Porcupine result is
claimed.

## TLA+ state-space result

TLC 2.19 exhaustively explored the configured finite model:

| Bound or result | Value |
|---|---:|
| Nodes | 3 |
| Maximum term | 2 |
| Maximum log length | 2 |
| States generated | 74,698,942 |
| Distinct states | 9,560,875 |
| State-graph depth | 30 |
| States remaining | 0 |
| Invariant violations | 0 |

Modeled transitions include durable election, AppendEntries replication,
current-term commit, compaction, InstallSnapshot, joint/final membership,
explicit persistence (fsync), and crash recovery. Checked invariants cover type
correctness, election safety, committed-prefix safety, snapshot safety,
durable-vote safety, durable-commit safety, and durable-snapshot safety.

The model tracks a durable log prefix separately from the in-memory log. An
append is not durable until the Persist action runs, replication and snapshot
install are durable before they are acknowledged, a commit counts only acks
whose entry is on stable storage including the leader's own, and Crash
truncates the log to the fsynced prefix and clamps commitIndex to what
survives. An earlier version of this model left the log untouched across a
crash, which assumed every append was already durable -- the one assumption the
WAL implementation deliberately does not make.

Fingerprint-collision estimates reported by TLC were `1.3E-5` optimistic and
`3.3E-6` from actual fingerprints. This is bounded model checking, not an
unbounded proof.

## Claim boundary

The repository is a specification-and-qualification-complete reference
implementation. Zero-allocation applies only to measured hot paths; latency
applies only to the named configuration; linearizability applies to the
recorded workload; and formal safety applies to the documented finite bounds.

See [STATUS.md](STATUS.md), [ROADMAP.md](ROADMAP.md),
[docs/OPERATING-ENVELOPE.md](docs/OPERATING-ENVELOPE.md), and
[docs/RELEASE-CHECKLIST.md](docs/RELEASE-CHECKLIST.md).