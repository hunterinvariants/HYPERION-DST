# Sentinel io_uring baseline â€” 2026-07-28

This is a functional durability baseline, not an NVMe performance result.

## Environment

- host: `sentinel` (virtual machine)
- OS: Ubuntu 24.04.4 LTS
- kernel: `6.8.0-136-generic`, `PREEMPT_DYNAMIC`
- CPU: Intel Core i7-1165G7, 8 vCPUs
- memory: 7.6 GiB
- filesystem: ext4 on block device `/dev/sda2`
- Go: 1.25.0 linux/amd64
- io_uring policy: enabled

## Verified gates

- `io_uring_setup`: pass
- registered buffer: pass
- registered file: pass
- `O_DIRECT` 4096-byte `WRITE_FIXED`: pass
- CQE result and user-data validation: pass
- separate `FSYNC` completion: pass
- XDP and TC eBPF verifier/JIT: pass
- isolated TC netem delay: 25 ms observed
- XDP configured drop rate: 10%; observed 7/100 packets
- namespace/veth/program cleanup: pass

## Measurement

Command:

```text
hyperion-uring-bench -path /var/tmp/hyperion-uring-bench.dat -operations 1000
```

Result after fixing a reproducible spurious-CQ-wakeup bug:

```text
operations=1000 block=4096 total=542.396749ms
ops_per_sec=1844 p50=533.815us p99=705.035us max=1.382461ms
```

The virtual disk and filesystem stack dominate these numbers. They must not be
presented as physical NVMe latency.

