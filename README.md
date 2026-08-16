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
- bounded live Jepsen/Knossos workload under process and TC network faults: `valid? true`;
- bounded TLC model: 46,667,923 states generated, 6,121,927 distinct, no invariant violation.

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

Every command is also reachable through one umbrella binary. The subcommands run
the same implementations as the standalone binaries, with the same flags and
exit codes; commands needing kernel facilities the build cannot reach are not
listed.

```bash
go run ./cmd/hyperion help
go run ./cmd/hyperion seeds -from 1 -to 1000 -steps 1000
```

A cluster can be declared once and shared by every node, which selects its own
entry by identifier. The peer list is derived from the file, so the processes
of a cluster cannot disagree about who the members are. See
[examples/cluster.json](examples/cluster.json).

```bash
go run ./cmd/hyperiond -config examples/cluster.json -id 3
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

## Using the machinery on your own system

The deterministic simulator, its invariant checking, and its fault injection are
not specific to Raft. [docs/DEVELOPERS.md](docs/DEVELOPERS.md) covers driving
your own protocol through the engine, declaring campaigns as scenario files, and
verifying an alternative storage backend against the device conformance suite.

```bash
go run ./cmd/hyperion simulate -config examples/leader-partition.json
go test ./examples/paxos -v
```

[examples/paxos](examples/paxos/README.md) is a complete worked example of a
protocol that is not Raft — single-decree Paxos with its own invariants,
partition campaigns, and scenario file.

See [SPEC.md](SPEC.md), [STATUS.md](STATUS.md), and [ROADMAP.md](ROADMAP.md).
