# Project status

Updated: 2026-07-28

## Completed and verified

| Area | Capability | Evidence |
|---|---|---|
| DST | deterministic clock, network, seeds, crash/restart | reproducibility and committed-prefix tests |
| Raft | pre-vote, duplicate-safe elections, replication, commit | randomized safety tests |
| Raft | compacted absolute indexes and durable snapshot catch-up | unit, crash/restart, DST, Sentinel io_uring gates |
| Raft | replicated joint/final configuration transitions | dual-majority, restart, removal, and five-node DST tests |
| Raft | quorum-confirmed ReadIndex and leadership transfer | protocol and race tests |
| Persistence | snapshot-before-WAL-fence compaction ordering | interrupted-install recovery and replay tests |
| DST storage | bit-rot, misdirected writes, phantom prefixes | seeded fault/recovery tests |
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
| Phase 3 | Linux io_uring, direct NVMe I/O, XDP/TC chaos, safe cleanup | complete; Sentinel and GCP Local SSD evidence |
| Snapshot format | checksummed encode/decode and torn-image rejection | unit tests |
| Membership | old/new joint-majority calculation | unit tests |
| Tooling | seed sweeper, race suite, fuzz target, CI | executable commands/workflows |
| Formal | durable-election/crash TLA+ model | checked-in model |
| Service | versioned peer/client protocol and multi-process TCP cluster | protocol and three-process restart tests |
| Client safety | replicated request IDs, deterministic deduplication, ReadIndex reads | restart and failover tests |
| Operations | bounded queues, health, metrics, shutdown, backup/restore | unit and integration tests |
| Jepsen | live register workload and Knossos checker | Sentinel `valid? true` under process and TC network faults |
| NVMe | GCP Local SSD raw `O_DIRECT` + registered io_uring durability | 10,000 writes, 10,132 ops/s, p99 132.080 us, all gates pass |

Additional measured evidence:

- 1,000 seeds x 1,000 virtual ticks with periodic restart: pass;
- WAL encode benchmark: 0 B/op, 0 allocs/op;
- Raft heartbeat Step: 0 allocations across 10,000 measured runs;
- 1,000 durable `WRITE_FIXED + FSYNC` operations: 1,844 ops/s;
- latency: p50 533.815 us, p99 705.035 us, max 1.382461 ms;
- Phase 4 Sentinel race gate: pass;
- 100x deterministic five-node Raft/DST gate: pass in 481.973 seconds;
- 100x io_uring snapshot/compaction/recovery gate: pass;
- Phase 5 five-process Sentinel gate: pass;
- Jepsen/Knossos register history under process and TC faults: `valid? true`.
- GCP Local SSD NVMe gate: 10,000 durable operations, p50 95.074 us,
  p99 132.080 us, max 551.414 us; race, TLC, and 100x integration gates pass.

## Remaining production gates

### P0: independent verification

- expand TLA+ to AppendEntries, commit, snapshot installation, and membership;
- run the model checker in CI with a documented state bound;
- add filesystem-full, injected I/O-error, and extended restart integration campaigns.

### P1: storage performance and operability

- batch multiple SQEs and harvest multiple CQEs per enter call;
- add buffer/file lifecycle state machines and queue saturation backpressure;
- benchmark group commit and durability-policy tradeoffs;
- test disk-full, short-I/O, checksum corruption, and failed `FSYNC` paths;
- add manifest/version migration and extended observability procedures.


## Claim policy

HYPERION-DST has a completed, Linux-qualified Raft protocol core. It remains
an engineering prototype rather than a production database until the Phase 6
production qualification gates are complete. Full
zero-allocation, sub-microsecond physical durability, strict serializability, and machine-checked safety must not
be claimed until their corresponding gates above have passed.
