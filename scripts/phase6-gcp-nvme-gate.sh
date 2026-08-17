#!/usr/bin/env bash
set -Eeuo pipefail

# Destructive Phase 6 qualification for the disposable GCP Local SSD only.
# Evidence is written below the repository, which must live on the boot disk.
DEVICE_LINK=${PROMTACT_PHASE6_DEVICE:-/dev/disk/by-id/google-local-nvme-ssd-0}
OPERATIONS=${PROMTACT_PHASE6_OPERATIONS:-10000}
STAMP=$(date -u +%Y%m%dT%H%M%SZ)
ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
OUT="$ROOT/benchmarks/artifacts/gcp-phase6-$STAMP"
LOG="$OUT/gate.log"

if [[ $EUID -ne 0 ]]; then
  echo "phase6-gate REFUSED: run with sudo" >&2
  exit 2
fi
mkdir -p "$OUT"
exec > >(tee -a "$LOG") 2>&1

fail() { echo "phase6-gate REFUSED: $*" >&2; exit 2; }
command -v go >/dev/null || fail "Go is not installed"
command -v lsblk >/dev/null || fail "lsblk is not installed"
command -v findmnt >/dev/null || fail "findmnt is not installed"
[[ -L "$DEVICE_LINK" ]] || fail "missing stable Local SSD link $DEVICE_LINK"
DEVICE=$(readlink -f -- "$DEVICE_LINK")
[[ -b "$DEVICE" ]] || fail "$DEVICE is not a block device"
[[ "$DEVICE" == /dev/nvme*n* ]] || fail "resolved device is not NVMe: $DEVICE"

ROOT_SOURCE=$(findmnt -nro SOURCE /)
ROOT_PARENT=$(lsblk -nro PKNAME "$ROOT_SOURCE" | head -n1)
[[ "$DEVICE" != "$ROOT_SOURCE" ]] || fail "target is the root device"
[[ "/dev/$ROOT_PARENT" != "$DEVICE" ]] || fail "target contains the root filesystem"
[[ -z $(lsblk -nro MOUNTPOINTS "$DEVICE" | tr -d '[:space:]') ]] ||
  fail "target or a child is mounted"
[[ -z $(lsblk -nro FSTYPE "$DEVICE" | tr -d '[:space:]') ]] ||
  fail "target contains a filesystem signature"
! swapon --show=NAME --noheadings | grep -Fxq "$DEVICE" ||
  fail "target is active swap"
[[ ! -d "/sys/class/block/$(basename "$DEVICE")/holders" ]] ||
  [[ -z $(ls -A "/sys/class/block/$(basename "$DEVICE")/holders") ]] ||
  fail "target has device-mapper holders"

SERIAL=$(lsblk -dnro SERIAL "$DEVICE" | xargs)
MODEL=$(lsblk -dnro MODEL "$DEVICE" | xargs)
SIZE=$(lsblk -bdnro SIZE "$DEVICE" | xargs)
[[ "$SERIAL" == local-nvme-ssd-* ]] ||
  fail "serial is not a GCP Local SSD: <$SERIAL>"
[[ "$MODEL" == nvme_card ]] ||
  fail "unexpected model: <$MODEL>"
(( SIZE >= 400000000000 && SIZE <= 405000000000 )) ||
  fail "unexpected Local SSD size: $SIZE"
[[ "$OPERATIONS" =~ ^[0-9]+$ ]] && (( OPERATIONS >= 1000 && OPERATIONS <= 1000000 )) ||
  fail "operations must be in 1000..1000000"

echo "DESTRUCTIVE target=$DEVICE stable_link=$DEVICE_LINK serial=$SERIAL model=$MODEL size=$SIZE"
echo "commit=$(git -C "$ROOT" rev-parse HEAD)"
uname -a
cat /etc/os-release
lscpu
lsblk -p -o NAME,SIZE,MODEL,SERIAL,TYPE,FSTYPE,MOUNTPOINTS
command -v nvme >/dev/null && nvme list || true
command -v nvme >/dev/null && nvme smart-log "$DEVICE" || true

cd "$ROOT"
go test ./... -race -count=1
bash verification/run-tlc.sh
PROMTACT_URING_INTEGRATION=1 go test ./storage/uring ./storage/uringwal ./storage/raftstore \
  -race -count=100
go build -o "$OUT/promtact-raw-bench" ./cmd/promtact-raw-bench

# This is the only destructive operation. The binary independently repeats
# block-device, size, mount, swap, and partition checks before opening O_DIRECT.
"$OUT/promtact-raw-bench" \
  -device "$DEVICE_LINK" \
  -confirm-destroy "ERASE:$DEVICE" \
  -expected-size "$SIZE" \
  -operations "$OPERATIONS" |
  tee "$OUT/raw-nvme-result.txt"

sync
command -v nvme >/dev/null && nvme smart-log "$DEVICE" | tee "$OUT/nvme-smart-after.txt" || true
sha256sum "$LOG" "$OUT/raw-nvme-result.txt" > "$OUT/SHA256SUMS"
echo "phase6-gcp-nvme-gate PASS evidence=$OUT"
