# Roadmap

1. **Portable deterministic core:** virtual time, seeded scheduling,
   crash/drop/delay faults, trace hashes, Raft write replication.
2. **Durability (current):** fixed-record CRC32C WAL, crash/replay model, and
   exhaustive record-boundary torn-write tests. Next: snapshots, write-error
   injection, group commit, and allocation benchmarks.
3. **Linux storage:** `io_uring` backend, registered buffers/files, `O_DIRECT`
   alignment checks, batched SQE/CQE handling, and syscall fallback tests.
4. **Protocol completeness:** pre-vote, leadership transfer, ReadIndex/leases,
   joint-consensus membership, snapshot installation, and compaction.
5. **Kernel chaos:** CO-RE XDP drop/corrupt program and TC delay/reorder
   program, controlled by a userspace fault plan with mandatory cleanup.
6. **Independent verification:** TLA+ model, Jepsen workload, Knossos history
   checking, Linux integration CI, race/fuzz/sanitizer jobs.

Claims such as sub-microsecond persistence, billions of seeds, and a
zero-allocation hot path are acceptance targets. They are not project claims
until reproducible benchmark evidence is checked in.

