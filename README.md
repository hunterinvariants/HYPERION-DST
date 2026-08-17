# Promtact

[![CI](https://github.com/hunterinvariants/promtact/actions/workflows/ci.yml/badge.svg)](https://github.com/hunterinvariants/promtact/actions/workflows/ci.yml)
[![Nightly](https://github.com/hunterinvariants/promtact/actions/workflows/nightly.yml/badge.svg)](https://github.com/hunterinvariants/promtact/actions/workflows/nightly.yml)
[![Kernel build](https://github.com/hunterinvariants/promtact/actions/workflows/kernel.yml/badge.svg)](https://github.com/hunterinvariants/promtact/actions/workflows/kernel.yml)
[![Formal](https://github.com/hunterinvariants/promtact/actions/workflows/formal.yml/badge.svg)](https://github.com/hunterinvariants/promtact/actions/workflows/formal.yml)
![Specification & Qualification](https://img.shields.io/badge/specification%20%26%20qualification-complete-2ea44f)

Test distributed protocols the way you test pure functions: run your own
consensus code under deterministic virtual time and injected network faults,
check invariants after every step, and reproduce any failure from its seed.

A qualified Raft implementation ships as the worked reference — durable
`io_uring` storage, a checksummed WAL, XDP/TC kernel fault injection,
Jepsen/Knossos linearizability, and a bounded TLA+ model. It is a reference
system, not a turnkey database.

The project reports only capabilities backed by executable tests, bounded model
checking, or checked-in measurements. Every claim carries its bounds in
[EVIDENCE.md](EVIDENCE.md) and [STATUS.md](STATUS.md).

## Requirements

- **Go 1.21 or newer** to build a checkout. The `toolchain` directive in
  `go.mod` fetches the release this project pins, so whatever Go your
  distribution ships is enough — the Go 1.22 on Ubuntu 24.04 switches to
  go1.25.13 by itself, with no action from you.
- **Go 1.25 or newer** to use it as a library in your own module. `go get`
  raises your `go` line to 1.25, and an older toolchain then has nothing to
  resolve unless you also add `toolchain go1.25.13` to your `go.mod`.
- **Git**, to clone.
- **A C compiler**, only for `go test ./... -race`, because `-race` requires
  cgo. Nothing else in this project does.
- **Linux**, for the `io_uring`, eBPF and raw-device commands. On other
  platforms those commands are absent rather than present and failing.

## Install

As a library, to drive your own protocol:

```bash
go get github.com/hunterinvariants/promtact@latest
```

As a command, to run scenarios and the gates:

```bash
go install github.com/hunterinvariants/promtact/cmd/promtact@latest
```

`go install` writes the binary to `$(go env GOPATH)/bin`, which is not on
`PATH` on a fresh machine:

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
```

The scenario and cluster files the commands read live in the repository, not
inside the installed binary. Clone it to run the shipped examples.

## Try it in a minute

```bash
git clone https://github.com/hunterinvariants/promtact.git
cd promtact
go test ./...
go test ./examples/paxos -v
go run ./cmd/promtact simulate -config examples/leader-partition.json
```

The full suite runs under the race detector too, which needs a C toolchain
because `-race` requires cgo. There is no cgo anywhere else in this project, so
the plain command above is enough to try it out.

The second command is the point of the project: a complete protocol that is
**not** Raft — single-decree Paxos — driven through the same engine, with its
own invariants and partition campaigns. See
[examples/paxos](examples/paxos/README.md).

Start your own from a working skeleton:

```bash
go run ./cmd/promtact new ../mysystem && cd ../mysystem && go mod tidy && go test ./... -v
```

The target is outside this checkout on purpose: a generated project is its own
Go module, and putting one inside the repository leaves an untracked directory
that looks like something you forgot to commit.

Every command lives behind one umbrella binary. Commands needing kernel
facilities a build cannot reach are absent rather than listed and failing.

```bash
go run ./cmd/promtact help
```

## What you can build with it

Four things are pluggable. [docs/DEVELOPERS.md](docs/DEVELOPERS.md) is the guide;
[docs/API.md](docs/API.md) says which identifiers are contractual.

**Your protocol.** Implement four methods for `dst.Cluster` and two for
`dst.Wire`, and the engine supplies virtual time, a seeded schedule, message
loss and delay, and a reproducible execution trace. It is generic over your
message type, so nothing is boxed and the hot path allocates nothing.

**Your properties.** A `dst.Invariant` is evaluated after every step. A failure
comes back as a `dst.Violation` carrying the property name, the step, and the
trace hash — a coordinate you can return to, not a message you have to
reproduce by guesswork.

**Your faults.** `Split`, `Isolate`, one-way `Link` failures, and `During` for
time windows. Injectors are consulted *after* the engine draws each message's
random loss and delay, so the same seed produces the same schedule with and
without a fault, and an A/B comparison means something.

**Your storage.** Implement `wal.Device` and you inherit the checksummed record
format, sequence validation, and torn-tail recovery of `wal.Log`. Verify it
against `storagetest.RunDeviceSuite` before trusting it with consensus state.

A run can be declared as a file rather than written into a test:

```json
{"seed": "0x4A2C", "nodes": 5, "steps": 1200, "proposeEvery": 17,
 "faults": [{"type": "split", "a": [1], "b": [2,3,4,5], "start": 200, "end": 700}]}
```

## The Raft reference

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

A cluster is declared once and shared by every node, which selects its own entry
by identifier. The peer list is derived from the file, so the processes cannot
disagree about who the members are.

```bash
go run ./cmd/promtactd -config examples/cluster.json -id 3
```

The extracted engine is verified against the simulator these gates qualified:
7,000 paired runs compare the two at every tick and require a bit-identical
execution. Evidence in
[benchmarks/sentinel-dst-engine-2026-08-17.md](benchmarks/sentinel-dst-engine-2026-08-17.md).

## Verified Linux baseline

On the `sentinel` Linux host, the following gates passed:

- Ubuntu 24.04.4 LTS, kernel `6.8.0-137-generic`, Go 1.25.13;
- `io_uring_setup`, registered buffer/file, `O_DIRECT`, `WRITE_FIXED`, CQE, `FSYNC`;
- WAL write, close, reopen, checksum validation, and bit-exact replay;
- XDP and TC verifier/JIT loading;
- isolated 25 ms TC delay and configured 10% XDP drop injection;
- namespace, veth, map, and program cleanup;
- complete `go test ./... -race -count=1` suite;
- five-process Phase 5 failover, health, metrics, backup/restore gate;
- bounded live Jepsen/Knossos workload under process and TC network faults: `valid? true`;
- bounded TLC model: 46,667,923 states generated, 6,121,927 distinct, no invariant violation.

Durable 4 KiB writes with one completed `FSYNC` per operation, through
registered `io_uring` with `O_DIRECT` and `WRITE_FIXED`:

| Target | Operations | Throughput | p50 | p99 | Max |
|---|---:|---:|---:|---:|---:|
| file on ext4 over `/dev/sda2` | 1,000 | 2,189 ops/s | 449.616 us | 543.602 us | 1.517617 ms |
| raw NVMe `/dev/nvme0n1` | 10,000 | 7,971 ops/s | 117.207 us | 196.343 us | 13.668893 ms |

These describe this machine's devices, not hardware in general. Full evidence
and commands are in [benchmarks/sentinel-2026-08-17.md](benchmarks/sentinel-2026-08-17.md).

Linux capability and integration gates:

```bash
go run ./cmd/promtact probe -entries 32
PROMTACT_URING_INTEGRATION=1 go test ./storage/uring ./storage/uringwal -count=1 -v
go run ./cmd/promtact verify -json
```

The chaos controller must be used only with its dedicated `promtact-*`
namespace and veth pair. Never attach development fault policies to a management
interface. See [bpf/README.md](bpf/README.md).

## Architecture

```text
your protocol            the Raft reference
       \                        /
        dst.Cluster / dst.Wire
                 |
   deterministic engine: virtual time, seeded
   schedule, faults, invariants, trace hash
                 |
        durable Raft core
          |           |
     CRC32C WAL   snapshots
          |
 registered io_uring + O_DIRECT

isolated netns -> XDP / TC / netem -> controlled kernel faults
```

## Repository map

- `dst/`: the protocol-agnostic engine, invariants, and fault injection;
- `dst/scenario/`: the declarative run format;
- `dst/raftcluster/`: the Raft adapter, and the equivalence campaigns;
- `examples/paxos/`: a complete protocol that is not Raft;
- `raft/`: consensus state machine, persistence boundary, quorum logic;
- `sim/`: the qualified simulator, retained as the equivalence reference;
- `storage/wal/`: portable WAL format and recovery;
- `storage/storagetest/`: conformance suite for an alternative backend;
- `storage/uring/`, `storage/uringwal/`: Linux registered I/O and its WAL adapter;
- `storage/snapshot/`: checksummed snapshot images;
- `server/`: the replicated service and its cluster file format;
- `chaos/`, `bpf/`: safe controller and kernel programs;
- `internal/cli/`: one implementation per command, shared by every binary;
- `verification/tla/`: current formal model;
- `benchmarks/`: checked-in measurement evidence.

## Documentation

- [docs/DEVELOPERS.md](docs/DEVELOPERS.md) — driving your own protocol, properties, faults, and storage;
- [docs/API.md](docs/API.md) — what is contractual, and what versioning will mean;
- [CONTRIBUTING.md](CONTRIBUTING.md) — the evidence bar a change has to clear;
- [SPEC.md](SPEC.md), [STATUS.md](STATUS.md), [ROADMAP.md](ROADMAP.md), [EVIDENCE.md](EVIDENCE.md).

The six scoped qualification phases are complete and frozen at the documented
reference baseline. Framework work claims no phase acceptance and does not amend
that record.

## License

Apache License 2.0 — see [LICENSE](LICENSE). It covers the project, including
earlier releases.

Apache rather than MIT for the express patent grant and its retaliation clause,
which matter more for a project touching `io_uring`, eBPF, and consensus than
copyleft would.

## Earlier names

This project was called `HYPERION-DST` through `v0.1.1`, then `Hyperion` for
`v0.2.x`. Both names described what it was at the time: first a deterministic
simulation testing harness, then a framework that had outgrown the suffix.

**Use `github.com/hunterinvariants/promtact` from `v0.3.0` on.** Nothing older
resolves to it. GitHub redirects the repository URL, but a Go module path is not
a redirect — imports have to be updated. Releases before `v0.3.0` stay published
under their original names and are not maintained.
