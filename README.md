# HYPERION-DST

HYPERION-DST is a laboratory for building and verifying a deterministic,
crash-safe replicated state machine. The repository deliberately separates
portable consensus logic from Linux-only storage and fault-injection backends.

The first executable vertical slice contains:

- a single-threaded virtual clock and seeded network scheduler;
- a compact Raft core with elections, heartbeats, replication, and commit;
- reproducible drop, delay, and node-crash faults;
- trace hashing so equal seeds can be checked bit-for-bit;
- a fixed-record CRC32C WAL with deterministic torn-write crash recovery;
- executable safety tests and a mathematical specification.

```text
go test ./...
go run ./cmd/hyperion-sim -seed 0x4A2C -steps 10000 -nodes 5
```

The current hot-path contract is allocation-bounded, not yet proven
zero-allocation. Run benchmarks with `-benchmem`; a later Linux milestone will
add fixed pools, `io_uring` + `O_DIRECT`, and XDP/TC programs behind the
interfaces in `storage`.

See [SPEC.md](SPEC.md) for the invariants and [ROADMAP.md](ROADMAP.md) for the
kernel and verification milestones.

