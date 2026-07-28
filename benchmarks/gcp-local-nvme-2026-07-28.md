# GCP Local SSD NVMe qualification

Date: 2026-07-28

## Configuration

- instance: `hyperion-phase6-nvme`, `europe-west6-a`;
- machine: `n2-custom-4-12288`, 4 vCPU, 12 GiB RAM;
- CPU: Intel Xeon at 2.80 GHz, KVM;
- OS: Ubuntu 24.04.4 LTS;
- kernel: `6.17.0-1021-gcp`;
- Go: `go1.25.0 linux/amd64`;
- device: `/dev/nvme0n1`, stable ID
  `/dev/disk/by-id/google-local-nvme-ssd-0`;
- model/serial: `nvme_card` / `local-nvme-ssd-0`;
- capacity/sector format: 402,653,184,000 bytes, 4 KiB;
- device was dedicated, unmounted, unpartitioned, and contained no filesystem;
- boot device was a separate Persistent Disk at `/dev/sda`.

This is a directly attached GCP Local SSD exposed through NVMe.

## Gate

Commit: `8de2fd18c7134f246724748b61d65eb75ef9abbd`

The fail-closed gate verified device identity and isolation before running:

- complete Go race suite;
- bounded TLC model check: 1,765 generated / 343 distinct states, depth 13;
- 100 repetitions of the io_uring, io_uring WAL, and snapshot-compaction tests;
- 10,000 raw durable writes using 4 KiB `O_DIRECT`, registered file/buffer,
  `WRITE_FIXED`, CQE validation, and a completed `FSYNC` per operation.

All gates passed.

## Raw durability result

| Operations | Total | Operations/s | p50 | p99 | max |
|---:|---:|---:|---:|---:|---:|
| 10,000 | 987.004969 ms | 10,132 | 95.074 us | 132.080 us | 551.414 us |

SMART before and after the run reported zero critical warnings, media errors,
and error-log entries. Temperature remained 30 C and reported wear remained
0 percent.

Evidence directory reported by the runner:
`benchmarks/artifacts/gcp-phase6-20260728T023003Z`.
