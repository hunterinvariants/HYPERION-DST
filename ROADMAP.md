# Roadmap

## Phase 1 - deterministic core: complete

- virtual time and seeded network scheduler;
- Raft election, replication, commit, and invariant checking;
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

## Phase 4 - complete Raft protocol: in progress

1. compacted-base absolute log indexing;
2. durable `InstallSnapshot` and follower catch-up;
3. safe log/WAL compaction;
4. replicated joint and final configuration entries;
5. ReadIndex or proven leases;
6. pre-vote and leadership transfer.

## Phase 5 - distributed product surface: planned

1. versioned peer and client protocols;
2. multi-process cluster runner;
3. request deduplication and backpressure;
4. metrics, health, shutdown, backup, and restore;
5. Jepsen/Knossos live verification.

## Phase 6 - formal and production qualification: planned

1. complete TLA+ protocol model and bounded CI model checking;
2. disk-full, I/O-error, process-kill, and corruption campaigns;
3. dedicated physical NVMe test matrix;
4. documented operating envelope and release checklist.

No phase is considered complete from code presence alone. It requires an
executable test or recorded external evidence.
