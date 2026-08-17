#!/usr/bin/env bash
set -euo pipefail

echo "== platform =="
uname -a
cat /etc/os-release

echo "== io_uring policy =="
sysctl kernel.io_uring_disabled 2>/dev/null || true
ulimit -l

echo "== storage (read-only inventory) =="
lsblk -e7 -o NAME,MODEL,SERIAL,SIZE,TYPE,FSTYPE,MOUNTPOINTS
findmnt /

echo "== network (no addresses) =="
ip -br link

echo "== toolchain =="
go version
clang --version | head -n 1
bpftool version
tc -V

echo "== Promtact =="
git rev-parse --short HEAD
go test ./... -count=1
go run ./cmd/promtact-probe
go run ./cmd/promtact-seeds -from 1 -to 1000 -steps 1000
go test ./storage/wal -run '^$' -bench BenchmarkEncode -benchmem

