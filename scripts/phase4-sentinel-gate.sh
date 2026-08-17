#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

if [[ "$(uname -s)" != Linux || "$(uname -m)" != x86_64 ]]; then
  echo "phase4-gate: Linux x86_64 is required" >&2
  exit 1
fi

echo "phase4-gate commit=$(git rev-parse HEAD)"
echo "phase4-gate kernel=$(uname -r)"
echo "phase4-gate go=$(go version)"

go test ./... -race -count=1

PROMTACT_URING_INTEGRATION=1 \
  go test ./storage/raftstore \
    -run '^TestIOUringSnapshotCompactionRecovery$' \
    -race -count=100

go test ./raft ./sim ./storage/raftstore ./storage/raftwal \
  -race -count=100

echo "phase4-gate PASS"
