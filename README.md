# HYPERION-DST

[![CI](https://github.com/hunterinvariants/HYPERION-DST/actions/workflows/ci.yml/badge.svg)](https://github.com/hunterinvariants/HYPERION-DST/actions/workflows/ci.yml)
[![Kernel build](https://github.com/hunterinvariants/HYPERION-DST/actions/workflows/kernel.yml/badge.svg)](https://github.com/hunterinvariants/HYPERION-DST/actions/workflows/kernel.yml)
[![Formal](https://github.com/hunterinvariants/HYPERION-DST/actions/workflows/formal.yml/badge.svg)](https://github.com/hunterinvariants/HYPERION-DST/actions/workflows/formal.yml)
![Specification & Qualification](https://img.shields.io/badge/specification%20%26%20qualification-complete-2ea44f)

HYPERION-DST is a verification-focused distributed consensus engine combining
a deterministic simulator, durable Raft state, a checksummed WAL, registered
Linux `io_uring` I/O, and isolated XDP/TC kernel fault injection.

The project reports only capabilities backed by executable tests, bounded model
checking, or checked-in measurements. All six scoped roadmap phases are
complete; evidence bounds and remaining optimizations are tracked in
[STATUS.md](STATUS.md).

The implementation and its six scoped qualification phases are frozen at the
documented reference baseline. See [EVIDENCE.md](EVIDENCE.md) for the final
evidence index and the precise bounds of every claim.

## What works today

- deterministic virtual time, seeded scheduling, message delay/drop, and restart;
- Raft pre-vote, elections, duplicate-safe voting, replication, commit, durable term/vote, and durable entry ACKs;
- fail-stop behavior when stable storage rejects a write;
- fixed 112-byte CRC32C WAL records, sequence validation, torn-tail recovery;
- crash/restart reconstruction exclusively from durable WAL state;
- deterministic bit-rot, misdirected-write, and phantom-prefix storage faults;
- registered-file and registered-buffer `io_uring` data path using `WRITE_FIXED`;
- CQE identity, error, and short-write validation followed by a separate `FSYNC`;
- checksummed WAL records stored in aligned 4096-byte `O_DIRECT` blocks;
- XDP ingress drop/partition and TC egress drop/corruption programs;
- namespace-safe eBPF/netem controller with mandatory cleanup;
- checksummed snapshot image format and joint-quorum calculation primitive;
- parallel seed sweeper, race tests, fuzz target, benchmarks, and CI;
- bounded TLA+ model covering election, replication, commit, snapshots, membership, and crash recovery;
- versioned CRC32C peer/client protocol, TCP multi-process service, replicated deduplication, ReadIndex reads, bounded backpressure, health/metrics, and backup/restore.

## What CI verifies on every push

- `go test ./... -race -count=1`, `go vet`, the WAL encode benchmark, and a 1,000-seed sweep;
- the registered io_uring / `O_DIRECT` / `WRITE_FIXED` / CQE / `FSYNC` integration tests, after probing that the runner's kernel allows io_uring;
- the bounded TLC model check;
- a reduced Jepsen/Knossos linearizability run against a five-process loopback cluster, with the history uploaded as a build artifact;
- the BPF object build and the chaos controller build.

Live XDP/TC injection, the networked five-process gate, the full Jepsen
workload under network partition, and the NVMe measurements are host-specific
and are not run by CI. Those are the `sentinel` and GCP results below.

## Verified Linux baseline

On the `sentinel` Linux host, the following gates passed:

- Ubuntu 24.04.4 LTS, kernel `6.8.0-136-generic`, Go 1.25.0;
- `io_uring_setup`, registered buffer/file, `O_DIRECT`, `WRITE_FIXED`, CQE, `FSYNC`;
- WAL write, close, reopen, checksum validation, and bit-exact replay;
- XDP and TC verifier/JIT loading;
- isolated 25 ms TC delay and configured 10% XDP drop injection;
- namespace, veth, map, and program cleanup;
- complete `go test ./... -race -count=1` suite;
- five-process Phase 5 failover, health, metrics, backup/restore gate;
- live Jepsen/Knossos workload under repeated total packet loss on one node: `valid? true` over a 60-second, 5,763-operation history (957 completed); raw history checked in under `jepsen/store/`;
- bounded TLC model: 74,698,942 states generated, 9,560,875 distinct, no invariant violation.

Measured durable block writes on ext4 over `/dev/sda2`:

| Metric | Result |
|---|---:|
| Operations | 1,000 |
| Throughput | 1,844 ops/s |
| p50 | 533.815 us |
| p99 | 705.035 us |
| Max | 1.382461 ms |

This is a block-device baseline, not a physical NVMe measurement. Full evidence
and commands are in [benchmarks/sentinel-block-device-2026-07-28.md](benchmarks/sentinel-block-device-2026-07-28.md).

## Quick start

```bash
go test ./... -race -count=1
go run ./cmd/hyperion-sim -seed 0x4A2C -steps 10000 -nodes 5
go run ./cmd/hyperion-seeds -from 1 -to 1000 -steps 1000
go test ./storage/wal -run '^$' -bench BenchmarkEncode -benchmem
```

Linux capability and integration gates:

```bash
go run ./cmd/hyperion-probe -entries 32
HYPERION_URING_INTEGRATION=1 go test ./storage/uring ./storage/uringwal -count=1 -v
```

The chaos controller must be used only with its dedicated `hyperion-*`
namespace and veth pair. Never attach development fault policies to a management
interface. See [bpf/README.md](bpf/README.md).

## Architecture

```text
seed runner / deterministic simulator
                |
                v
        durable Raft core
          |           |
          v           v
     CRC32C WAL   snapshots
          |
          v
 registered io_uring + O_DIRECT

isolated netns -> XDP / TC / netem -> controlled kernel faults
```

## Repository map

- `raft/`: consensus state machine, persistence boundary, quorum logic;
- `sim/`: deterministic scheduler, crash/restart, safety invariants;
- `storage/wal/`: portable WAL format and recovery;
- `storage/uring/`: Linux ring mappings and registered I/O;
- `storage/uringwal/`: WAL-to-aligned-io_uring adapter;
- `storage/snapshot/`: checksummed snapshot images;
- `chaos/`, `bpf/`: safe controller and kernel programs;
- `verification/tla/`: current formal model;
- `benchmarks/`: checked-in measurement evidence.

See [SPEC.md](SPEC.md), [STATUS.md](STATUS.md), and [ROADMAP.md](ROADMAP.md).
