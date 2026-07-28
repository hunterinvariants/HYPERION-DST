#!/usr/bin/env bash
# Reduced Jepsen/Knossos linearizability gate against a five-process loopback
# cluster. This is the CI-sized version of the Sentinel gate: same client
# protocol, same Knossos register model, same checker -- smaller history, and
# process faults only, because a loopback run has no network namespaces to
# partition. The full networked gate remains scripts/phase6-sentinel-gate.sh.
#
# Usage: scripts/jepsen-local-gate.sh [time-limit-seconds]
#
# Exits non-zero unless the checker reports :valid? true.
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
LIMIT=${1:-20}
RUN=$(mktemp -d "${TMPDIR:-/tmp}/hyperion-jepsen.XXXXXX")

cleanup() {
  set +e
  for id in 1 2 3 4 5; do
    [[ -f "$RUN/node$id.pid" ]] && kill "$(cat "$RUN/node$id.pid")" 2>/dev/null
  done
  wait 2>/dev/null
}
trap cleanup EXIT INT TERM

go build -o "$RUN/hyperiond" "$ROOT/cmd/hyperiond"

PEERS=""
CLIENTS=""
NODES=""
for id in 1 2 3 4 5; do
  PEERS+="${PEERS:+,}$id=127.0.0.1:$((9100 + id))"
  CLIENTS+="${CLIENTS:+,}127.0.0.1:$((9200 + id))"
  NODES+="${NODES:+,}n$id"
done

for id in 1 2 3 4 5; do
  mkdir -p "$RUN/data$id"
  "$RUN/hyperiond" \
    -id "$id" \
    -peer-address "127.0.0.1:$((9100 + id))" \
    -client-address "127.0.0.1:$((9200 + id))" \
    -http-address "127.0.0.1:$((9300 + id))" \
    -data-dir "$RUN/data$id" \
    -peers "$PEERS" \
    >"$RUN/node$id.log" 2>&1 &
  echo $! >"$RUN/node$id.pid"
done

# Wait for a leader rather than sleeping a fixed amount: a fixed sleep is how
# this kind of gate becomes flaky on a loaded runner.
for _ in $(seq 1 60); do
  if grep -qs '"leader":[1-9]' "$RUN"/node*.log 2>/dev/null ||
     curl -sf "http://127.0.0.1:9301/health" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

export HYPERION_CLIENTS="$CLIENTS"
export HYPERION_NODES="$NODES"
export HYPERION_JEPSEN_DUMMY_SSH=1

cd "$ROOT/jepsen"
lein run test --time-limit "$LIMIT" --concurrency 5

LATEST=$(ls -1dt store/hyperion-live-linearizability/*/ 2>/dev/null | head -1)
if [[ -z "$LATEST" ]]; then
  echo "jepsen produced no store directory" >&2
  exit 1
fi
echo "results: $LATEST/results.edn"
cat "$LATEST/results.edn"
grep -q ':valid? true' "$LATEST/results.edn"
