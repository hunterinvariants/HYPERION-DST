# Sentinel raw block-device qualification

Date: 2026-07-28

## Test target

- Linux host: Sentinel
- target: dedicated disposable 20 GiB SCSI block device
- path: `/dev/sdb`
- mounted: no
- partitions/filesystem/signatures: none
- system disk excluded by the guarded benchmark runner
- durability path: `O_DIRECT`, registered io_uring file and buffer,
  `WRITE_FIXED`, CQE validation, then `FSYNC`

This qualifies the raw block-device implementation. It is not evidence for
physical NVMe latency.

## Results

| Operations | Total | Operations/s | p50 | p99 | max |
|---:|---:|---:|---:|---:|---:|
| 1,000 | 186.701137 ms | 5,356 | 159.770 us | 383.010 us | 14.788684 ms |
| 10,000 | 1.748091074 s | 5,721 | 162.675 us | 287.590 us | 11.133332 ms |

The 10,000-operation run is the acceptance sample. No CQE loss, short write,
or durability error occurred.
