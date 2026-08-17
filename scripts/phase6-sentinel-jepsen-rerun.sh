#!/usr/bin/env bash
set -Eeuo pipefail

ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
STAMP=$(date -u +%Y%m%dT%H%M%SZ)
OUT="$ROOT/benchmarks/artifacts/sentinel-phase6-jepsen-$STAMP"
mkdir -p "$OUT"
exec > >(tee "$OUT/gate.log") 2>&1

if [[ $EUID -ne 0 ]]; then
  echo "phase6-sentinel-jepsen-rerun requires root" >&2
  exit 2
fi

cd "$ROOT"
echo "phase6-jepsen-rerun commit=$(git rev-parse HEAD)"
PROMTACT_KILL_SIGNAL=KILL bash scripts/phase5-sentinel-cluster.sh
sha256sum "$OUT/gate.log" > "$OUT/SHA256SUMS"
echo "phase6-sentinel-jepsen-rerun PASS evidence=$OUT"