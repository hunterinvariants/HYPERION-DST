# Project status

## Implemented and executable

- deterministic single-threaded network/time scheduler;
- Raft elections, replication, commit, durable term/vote, and durable entry ACKs;
- fail-stop behavior on persistence errors;
- CRC32C WAL, sequence validation, torn-tail truncation, and replay;
- crash/restart simulation reconstructed exclusively from durable WAL state;
- checksummed snapshot image format;
- joint-consensus quorum calculation;
- parallel seed sweeper with per-step safety checks;
- WAL encode benchmark and fuzz target;
- Linux/amd64 `io_uring_setup` capability probe and O_DIRECT alignment checks;
- XDP ingress drop/partition and TC egress drop/corruption eBPF programs;
- initial TLA+ durable-election/crash model;
- GitHub Actions test, race, vet, benchmark, and seed gates.

## Measured locally

- `go test ./...`: pass
- 1,000 seeds x 1,000 virtual ticks with periodic crash/restart: pass
- WAL encode: 0 B/op, 0 allocs/op
- Linux/amd64 io_uring package cross-compilation: pass

## Required before production claims

- integrate snapshot installation and compaction into Raft messages;
- encode configuration entries and drive two-phase joint consensus;
- implement registered buffers/files and the SQE/CQE storage data path;
- add a privileged eBPF/netem controller with namespace-safe cleanup;
- expose a client protocol and build the live Jepsen/Knossos workload;
- expand and model-check TLA+ AppendEntries, commit, snapshot, and membership;
- run Linux hardware benchmarks on named kernel/NVMe configurations.

HYPERION-DST is currently a runnable verification-focused reference system, not
a production database. Sub-microsecond durability, full zero-allocation, and
formal-proof claims remain prohibited until the corresponding gates pass.

