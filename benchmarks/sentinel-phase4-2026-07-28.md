# Sentinel Phase 4 qualification

Date: 2026-07-28

## Environment

- host: Sentinel
- kernel: Linux 6.8.0-136-generic amd64
- Go: 1.25.0
- acceptance runner: `scripts/phase4-sentinel-gate.sh`

## Full protocol gate

Tested commit: `0f6440139f7170384d88097e229406d2084ff04a`

- complete repository race suite: pass;
- io_uring snapshot/compaction/recovery, 100 repetitions: pass;
- Raft protocol suite under the race detector, 100 repetitions: pass;
- deterministic five-node simulator suite under the race detector,
  100 repetitions: pass in 481.973 seconds;
- combined result: `phase4-gate PASS`.

The suite covers compacted absolute indexes, ordered snapshot/WAL durability,
snapshot installation and follower catch-up, two-phase joint consensus,
configuration recovery, quorum-confirmed ReadIndex, leadership transfer,
leader removal, and crash/restart simulation.

## Post-gate io_uring hardening

Tested commit: `5e948a3`

The submission path was hardened so an interrupted `io_uring_enter` cannot
submit an already-consumed SQE twice. The affected
`TestIOUringSnapshotCompactionRecovery` gate passed 100 repetitions under the
race detector in 1.729 seconds.

## Hardware boundary

Sentinel exposes a dedicated, empty, unmounted 20 GiB NVMe-interface block
device at `/dev/nvme0n1`. It is reserved for later raw-device qualification.
This Phase 4 result does not claim physical-NVMe qualification.
