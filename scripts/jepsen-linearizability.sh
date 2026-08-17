#!/usr/bin/env bash
#
# Run the Jepsen/Knossos linearizability workload against a five-node cluster,
# under a leader kill and a window of total network loss on another node.
#
# This is the check behind the "jepsen" badge. It exists because the claim was
# on the front page and in EVIDENCE.md while nothing ran it except a person on
# one machine, and a result nobody re-runs is a historical note rather than a
# property of the code.
#
# It is not the Phase 6 qualification gate and does not amend it. That gate does
# more (race suite, backup and restore, metrics, the recorded artifacts) and
# stays where it is.
#
#   sudo ./scripts/jepsen-linearizability.sh
#   JEPSEN_TIME_LIMIT=60 sudo ./scripts/jepsen-linearizability.sh
#
set -euo pipefail

ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
RUN=$(mktemp -d /var/tmp/promtact-jepsen.XXXXXX)
BRIDGE=promtact-br0
PREFIX=10.77.0
LIMIT=${JEPSEN_TIME_LIMIT:-30}

cleanup() {
  set +e
  for id in 1 2 3 4 5; do
    [[ -f "$RUN/node$id.pid" ]] && kill "$(cat "$RUN/node$id.pid")" 2>/dev/null
    ip netns del "promtact-n$id" 2>/dev/null
  done
  ip link del "$BRIDGE" 2>/dev/null
}
trap cleanup EXIT INT TERM

[[ ${EUID} -ne 0 ]] && { echo "this needs root: it creates namespaces and applies tc" >&2; exit 2; }
command -v lein >/dev/null || { echo "leiningen is not installed" >&2; exit 2; }

cleanup
mkdir -p "$RUN"

echo "== building the node and the client"
go build -o "$RUN/promtactd" "$ROOT/cmd/promtactd"
go build -o "$RUN/promtactctl" "$ROOT/cmd/promtactctl"

echo "== bringing up five nodes"
# The promtact-* prefix is the same convention the chaos controller enforces,
# so that nothing here can be confused for a real interface.
ip link add "$BRIDGE" type bridge
ip addr add "$PREFIX.1/24" dev "$BRIDGE"
ip link set "$BRIDGE" up

PEERS=""
for id in 1 2 3 4 5; do
  PEERS+="${PEERS:+,}$id=$PREFIX.$((10 + id)):9100"
done

for id in 1 2 3 4 5; do
  ns="promtact-n$id"; host="promtact-h$id"; guest="promtact-g$id"
  ip netns add "$ns"
  ip link add "$host" type veth peer name "$guest"
  ip link set "$host" master "$BRIDGE"
  ip link set "$host" up
  ip link set "$guest" netns "$ns"
  ip -n "$ns" link set lo up
  ip -n "$ns" addr add "$PREFIX.$((10 + id))/24" dev "$guest"
  ip -n "$ns" link set "$guest" up
  ip -n "$ns" route add default via "$PREFIX.1"
  mkdir -p "$RUN/node$id"
  ip netns exec "$ns" "$RUN/promtactd" \
    -id "$id" \
    -peer-address "$PREFIX.$((10 + id)):9100" \
    -client-address "$PREFIX.$((10 + id)):9200" \
    -http-address "$PREFIX.$((10 + id)):9300" \
    -peers "$PEERS" \
    -data-dir "$RUN/node$id" \
    -snapshot-entries 1000 \
    >"$RUN/node$id.log" 2>&1 &
  echo $! >"$RUN/node$id.pid"
done

for id in 1 2 3 4 5; do
  for _ in $(seq 1 200); do
    if curl -fsS "http://$PREFIX.$((10 + id)):9300/healthz" >/dev/null &&
       curl -fsS "http://$PREFIX.$((10 + id)):9300/metrics" |
         grep -q '^promtact_commit_index '; then
      break
    fi
    sleep 0.05
  done
done

echo "== finding the leader"
leader=""
for attempt in $(seq 1 200); do
  for id in 1 2 3 4 5; do
    if "$RUN/promtactctl" -address "$PREFIX.$((10 + id)):9200" \
      -operation put -client 1 -request "$attempt" -key 999 -value 1 >/dev/null 2>&1; then
      leader=$id; break 2
    fi
  done
  sleep 0.1
done
[[ -n "$leader" ]] || { echo "no leader accepted a write" >&2; exit 1; }
echo "   node $leader"

# The faults have to overlap the recorded history, not bracket it, or the run
# proves only that a healthy cluster is linearizable.
fault_node=1
[[ $fault_node == "$leader" ]] && fault_node=2
(
  sleep 4
  echo "== killing the leader, node $leader"
  kill -KILL "$(cat "$RUN/node$leader.pid")" 2>/dev/null
  sleep 3
  echo "== total loss on node $fault_node"
  tc qdisc replace dev "promtact-h$fault_node" root netem loss 100%
  sleep 5
  tc qdisc del dev "promtact-h$fault_node" root 2>/dev/null || true
  echo "== healed"
) &
chaos=$!

echo "== running the workload for ${LIMIT}s"
export PROMTACT_CLIENTS="$PREFIX.11:9200,$PREFIX.12:9200,$PREFIX.13:9200,$PREFIX.14:9200,$PREFIX.15:9200"
status=0
(cd "$ROOT/jepsen" && lein run -- test --no-ssh --time-limit "$LIMIT") 2>&1 |
  tee "$RUN/jepsen.log" || status=$?
wait "$chaos" 2>/dev/null || true

[[ ${PIPESTATUS[0]:-0} -eq 0 && $status -eq 0 ]] ||
  { echo "jepsen exited non-zero" >&2; exit 1; }

# Grepping for the verdict rather than trusting the exit code: a checker that
# reports an invalid history and still exits 0 would otherwise pass silently.
if ! grep -Eq 'Analysis valid|:valid\? true' "$RUN/jepsen.log"; then
  echo "the checker did not report a valid history" >&2
  tail -40 "$RUN/jepsen.log" >&2
  exit 1
fi

# A history with no operations is trivially linearizable, so require that the
# workload actually did something.
if ! grep -Eq ':ok|:invoke' "$RUN/jepsen.log"; then
  echo "the history contains no operations; the workload tested nothing" >&2
  exit 1
fi

echo "jepsen-linearizability PASS  leader-kill=node$leader loss=node$fault_node limit=${LIMIT}s"
