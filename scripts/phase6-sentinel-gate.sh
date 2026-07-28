#!/usr/bin/env bash
set -Eeuo pipefail

ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
STAMP=$(date -u +%Y%m%dT%H%M%SZ)
OUT="$ROOT/benchmarks/artifacts/sentinel-phase6-$STAMP"
mkdir -p "$OUT"
exec > >(tee "$OUT/gate.log") 2>&1

if [[ $EUID -ne 0 ]]; then
  echo "phase6-sentinel-gate requires root" >&2
  exit 2
fi

cleanup() {
  set +e
  pkill -f '/var/tmp/hyperion-phase5/hyperiond' 2>/dev/null
  for id in 1 2 3 4 5; do
    ip netns del "hyperion-n$id" 2>/dev/null
  done
  ip link del hyperion-br0 2>/dev/null
}
trap cleanup EXIT INT TERM

cd "$ROOT"
echo "phase6-sentinel commit=$(git rev-parse HEAD)"
echo "kernel=$(uname -r)"
echo "go=$(go version)"
java -version

go test ./... -race -count=1
bash verification/run-tlc.sh

# Deterministic storage campaigns: ENOSPC, append/sync EIO, bit rot,
# misdirected writes, phantom reads, torn writes, and fail-stop Raft ACK rules.
go test ./storage/wal \
  -run 'DiskFull|IOError|FailedSync|Torn|Corrupt' -race -count=1000
go test ./storage/faultdisk -race -count=1000
go test ./raft \
  -run 'StorageFailure|PersistenceFailure|SnapshotFailure' -race -count=1000

# Extended deterministic crash/restart, compaction, and membership campaign.
go test ./sim \
  -run 'Crash|Restart|Compaction|Membership' -race -count=100

# Live five-process campaign includes leader SIGTERM, restart-safe backup,
# network isolation, continued operations, and Jepsen/Knossos validation.
HYPERION_KILL_SIGNAL=KILL bash scripts/phase5-sentinel-cluster.sh

sha256sum "$OUT/gate.log" > "$OUT/SHA256SUMS"
echo "phase6-sentinel-gate PASS evidence=$OUT"
