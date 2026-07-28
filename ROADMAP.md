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

## Phase 3 - Linux kernel paths: complete

- real `io_uring_setup` and mmap ring setup;
- registered file and registered aligned buffer;
- `O_DIRECT` `WRITE_FIXED`, CQE validation, and `FSYNC`;
- WAL adapter over aligned io_uring blocks;
- XDP/TC programs and namespace-safe netem controller;
- live Sentinel verifier, fault-injection, and cleanup tests;
- dedicated GCP Local SSD NVMe qualification with recorded kernel, device,
  SMART state, and latency distribution.

Evidence: benchmarks/sentinel-block-device-2026-07-28.md,
benchmarks/sentinel-raw-block-2026-07-28.md, and
benchmarks/gcp-local-nvme-2026-07-28.md.

Batching, group commit, and queue-depth tuning remain performance improvements,
not missing Phase 3 correctness gates.

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

## Phase 6 - formal and production qualification: complete

1. complete TLA+ protocol model and bounded CI model checking;
2. disk-full, I/O-error, process-kill, and corruption campaigns;
3. dedicated GCP Local SSD NVMe qualification: complete for the recorded configuration;
4. documented operating envelope and release checklist.

Evidence: `benchmarks/sentinel-phase6-2026-07-28.md` and
`benchmarks/gcp-local-nvme-2026-07-28.md`.

No phase is considered complete from code presence alone. It requires an
executable test or recorded external evidence.

## Known evidence bounds, not yet closed

These are limits of the recorded evidence rather than missing implementation.
They are listed here so they are not mistaken for claims.

- **Linearizability history is short.** The recorded Sentinel run is 15 seconds
  and 150 operations, of which 25 completed. Faults are applied by the cluster
  script rather than a Jepsen nemesis, so the fault windows are not recorded in
  the history. A longer nemesis-driven campaign, with the faults in the history,
  would make the result considerably stronger.
- **The formal model is not tied to the implementation.** There is no refinement
  mapping and no trace validation between the TLA+ model and the Go code; the
  model is an independent abstraction that is checked on its own terms.
- **Model bounds are small and safety-only.** Three nodes, two terms, two log
  entries, no fairness conditions, so no liveness property is checked.
- **Live XDP/TC injection, the networked five-process gate and the NVMe
  measurements are host-specific** and are not reproduced by CI. See the
  coverage table in `STATUS.md`.
