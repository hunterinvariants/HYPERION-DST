# Roadmap

## Phase 1 - deterministic core: complete

- virtual time and seeded network scheduler;
- Raft pre-vote, duplicate-safe election, replication, commit, and invariant checking;
- reproducible crash/restart seed sweeps.

## Phase 2 - durability foundation: complete

- CRC32C WAL and sequence validation;
- torn-write and crash/replay testing;
- durable term/vote and entry acknowledgements;
- checksummed snapshot image format.

## Phase 3 - Linux kernel paths: complete for functional baseline

- real `io_uring_setup` and mmap ring setup;
- registered file and registered aligned buffer;
- `O_DIRECT` `WRITE_FIXED`, CQE validation, and `FSYNC`;
- WAL adapter over aligned io_uring blocks;
- XDP/TC programs and namespace-safe netem controller;
- live Sentinel integration and cleanup tests.

Performance optimization remains open: batching, group commit, queue-depth
sweeps, backpressure, and physical NVMe measurements.

## Phase 4 - complete Raft protocol: complete

- compacted-base absolute log indexing;
- durable `InstallSnapshot` and lagging-follower catch-up;
- snapshot-before-fence log/WAL compaction and interrupted-install recovery;
- replicated joint and final configuration entries with dual-majority commit;
- quorum-confirmed ReadIndex;
- leadership transfer and removal-triggered leader step-down;
- deterministic compaction, membership, crash, and restart campaigns;
- Sentinel race and io_uring acceptance gates.

Evidence: `benchmarks/sentinel-phase4-2026-07-28.md`.

## Phase 5 - distributed product surface: complete

1. CRC32C-framed, versioned peer and client protocols;
2. TCP multi-process cluster service and isolated five-node namespace runner;
3. replicated request IDs, deterministic deduplication, retry safety, and bounded backpressure;
4. Prometheus metrics, health endpoint, graceful shutdown, checksummed offline backup and restore;
5. Jepsen register workload with Knossos linearizability checking and live process/network faults.

Sentinel evidence: `benchmarks/sentinel-phase5-2026-07-28.md`.

## Phase 6 - formal and production qualification: in progress

1. complete TLA+ protocol model and bounded CI model checking;
2. disk-full, I/O-error, process-kill, and corruption campaigns;
3. dedicated GCP Local SSD NVMe qualification: complete for the recorded
   configuration; bare-metal power-loss qualification remains external;
4. documented operating envelope and release checklist.

No phase is considered complete from code presence alone. It requires an
executable test or recorded external evidence.
