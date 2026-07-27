# Project status

Updated: 2026-07-28

## Completed and verified

| Area | Capability | Evidence |
|---|---|---|
| DST | deterministic clock, network, seeds, crash/restart | reproducibility and committed-prefix tests |
| Raft | elections, replication, commit | randomized safety tests |
| Persistence | term/vote before vote response | fail-stop ordering tests |
| Persistence | entry durable before AppendEntries ACK | fail-stop ordering tests |
| WAL | CRC32C, monotonic sequence, torn-tail recovery | exhaustive byte-cut tests |
| Recovery | durable WAL reconstructs node state | restart matrix and replay tests |
| io_uring | setup and ring mapping | Sentinel integration pass |
| io_uring | registered file and buffer, `WRITE_FIXED`, CQE, `FSYNC` | Sentinel integration pass |
| Direct I/O | aligned 4096-byte `O_DIRECT` blocks | Sentinel integration pass |
| WAL + io_uring | write, close, reopen, validate, replay | `storage/uringwal` integration pass |
| Kernel chaos | XDP and TC verifier/JIT | Sentinel verifier pass |
| Kernel chaos | isolated delay/drop and cleanup | Sentinel live test |
| Snapshot format | checksummed encode/decode and torn-image rejection | unit tests |
| Membership | old/new joint-majority calculation | unit tests |
| Tooling | seed sweeper, race suite, fuzz target, CI | executable commands/workflows |
| Formal | durable-election/crash TLA+ model | checked-in model |

Additional measured evidence:

- 1,000 seeds x 1,000 virtual ticks with periodic restart: pass;
- WAL encode benchmark: 0 B/op, 0 allocs/op;
- 1,000 durable `WRITE_FIXED + FSYNC` operations: 1,844 ops/s;
- latency: p50 533.815 us, p99 705.035 us, max 1.382461 ms.

## Remaining production gates

### P0: protocol safety and completeness

- add absolute log indexes and compacted-base metadata to Raft;
- implement and persist `InstallSnapshot`, including interrupted-install recovery;
- compact WAL/log only after a durable snapshot replacement;
- encode configuration entries in the replicated log;
- drive joint configuration and final configuration as two committed transitions;
- add ReadIndex or a rigorously bounded leader-lease protocol;
- add pre-vote, leadership transfer, and snapshot catch-up for lagging followers.

### P0: usable distributed service

- expose a versioned client protocol and state-machine API;
- run multiple OS processes with real peer transport;
- implement request IDs, deduplication, retries, backpressure, and graceful shutdown;
- export health, leadership, commit, storage, and fault-injection metrics.

### P0: independent verification

- build a live Jepsen workload and Knossos linearizability checker;
- expand TLA+ to AppendEntries, commit, snapshot installation, and membership;
- run the model checker in CI with a documented state bound;
- add process-kill, filesystem-full, I/O-error, and restart integration tests.

### P1: storage performance and operability

- batch multiple SQEs and harvest multiple CQEs per enter call;
- add buffer/file lifecycle state machines and queue saturation backpressure;
- benchmark group commit and durability-policy tradeoffs;
- test disk-full, short-I/O, checksum corruption, and failed `FSYNC` paths;
- add manifest/version migration, observability, backup, and restore procedures.

### External hardware gate

- attach a dedicated, disposable physical NVMe namespace;
- record model, firmware, kernel, filesystem, mount options, CPU policy, and queue settings;
- run power-loss or equivalent fault testing without risking a system disk;
- publish latency distributions and raw benchmark data.

## Claim policy

HYPERION-DST is currently a runnable, Linux-validated engineering prototype.
It is not yet a production database. Full zero-allocation, sub-microsecond
physical durability, strict serializability, and machine-checked safety must not
be claimed until their corresponding gates above have passed.
