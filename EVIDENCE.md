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

## Framework evidence

The engine, its invariants, its fault injection, the scenario format, and the
command extraction were built after the phases above and are **not** part of
any roadmap phase. They claim no phase acceptance, and nothing in this section
alters a result above it. They carry their own gate evidence:

| Area | Result | Evidence |
|---|---|---|
| Deterministic engine | 7,000 paired runs compare `dst.Engine` against the qualified `sim.Simulator` at every tick and require a bit-identical execution | [Engine qualification](benchmarks/sentinel-dst-engine-2026-08-16.md) |
| Invariants and faults | packaged Raft properties, leader partition, and one-way link failure hold across 1,000 seeds each | [Engine qualification](benchmarks/sentinel-dst-engine-2026-08-16.md) |
| Storage conformance | `MemoryDevice` and `FileDevice` pass the ten `wal.Device` properties | [Engine qualification](benchmarks/sentinel-dst-engine-2026-08-16.md) |
| Command extraction | seven binaries compared byte-for-byte against their pre-extraction builds, including the `chaos` and `raw-bench` refusal paths; Phase 5 cluster gate re-run for `hyperiond` | [Command extraction](benchmarks/sentinel-hyperion-cli-2026-08-16.md) |

Each report states explicitly what its run does not establish. The engine
equivalence is a relative comparison between two implementations on one host,
not a claim about cross-platform trace stability; the invariants are safety
properties only, with no liveness property checked.

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

The live five-node workload used Jepsen with the Knossos register checker.
During the measured history, the harness killed the leader process and applied
complete network loss to an isolated node. The final checker result was:

```clojure
{:linearizable {:valid? true}
 :timeline {:valid? true}
 :valid? true}
```

Recorded result:
`jepsen/store/hyperion-live-linearizability/20260728T045906.512+0200/results.edn`.

Knossos is the checker actually used by this repository; no Porcupine result is
claimed.

## TLA+ state-space result

TLC 2.19 exhaustively explored the configured finite model:

| Bound or result | Value |
|---|---:|
| Nodes | 3 |
| Maximum term | 2 |
| Maximum log length | 2 |
| States generated | 46,667,923 |
| Distinct states | 6,121,927 |
| State-graph depth | 25 |
| States remaining | 0 |
| Invariant violations | 0 |

Modeled transitions include durable election, AppendEntries replication,
current-term commit, compaction, InstallSnapshot, joint/final membership, and
crash recovery. Checked invariants cover type correctness, election safety,
committed-prefix safety, snapshot safety, and durable-vote safety.

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