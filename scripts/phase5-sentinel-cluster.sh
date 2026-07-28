#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
RUN=$(mktemp -d /var/tmp/hyperion-phase5.XXXXXX)
BRIDGE=hyperion-br0
PREFIX=10.77.0

cleanup() {
  set +e
  for id in 1 2 3 4 5; do
    if [[ -f "$RUN/node$id.pid" ]]; then
      kill "$(cat "$RUN/node$id.pid")" 2>/dev/null
    fi
    ip netns del "hyperion-n$id" 2>/dev/null
  done
  ip link del "$BRIDGE" 2>/dev/null
}
trap cleanup EXIT INT TERM

if [[ ${EUID} -ne 0 ]]; then
  echo "phase5 cluster gate requires root" >&2
  exit 1
fi

cleanup
mkdir -p "$RUN"
(cd "$ROOT" && go test ./... -race -count=1)
go build -o "$RUN/hyperiond" "$ROOT/cmd/hyperiond"
go build -o "$RUN/hyperionctl" "$ROOT/cmd/hyperionctl"
go build -o "$RUN/hyperion-backup" "$ROOT/cmd/hyperion-backup"

ip link add "$BRIDGE" type bridge
ip addr add "$PREFIX.1/24" dev "$BRIDGE"
ip link set "$BRIDGE" up

PEERS=""
for id in 1 2 3 4 5; do
  PEERS+="${PEERS:+,}$id=$PREFIX.$((10+id)):9100"
done

for id in 1 2 3 4 5; do
  ns="hyperion-n$id"
  host="hyperion-h$id"
  guest="hyperion-g$id"
  ip netns add "$ns"
  ip link add "$host" type veth peer name "$guest"
  ip link set "$host" master "$BRIDGE"
  ip link set "$host" up
  ip link set "$guest" netns "$ns"
  ip -n "$ns" link set lo up
  ip -n "$ns" addr add "$PREFIX.$((10+id))/24" dev "$guest"
  ip -n "$ns" link set "$guest" up
  ip -n "$ns" route add default via "$PREFIX.1"
  mkdir -p "$RUN/node$id"
  ip netns exec "$ns" "$RUN/hyperiond" \
    -id "$id" \
    -peer-address "$PREFIX.$((10+id)):9100" \
    -client-address "$PREFIX.$((10+id)):9200" \
    -http-address "$PREFIX.$((10+id)):9300" \
    -peers "$PEERS" \
    -data-dir "$RUN/node$id" \
    -snapshot-entries 1000 \
    >"$RUN/node$id.log" 2>&1 &
  echo $! >"$RUN/node$id.pid"
done

for id in 1 2 3 4 5; do
  for attempt in $(seq 1 100); do
    if curl -fsS "http://$PREFIX.$((10+id)):9300/healthz" >/dev/null &&
      curl -fsS "http://$PREFIX.$((10+id)):9300/metrics" |
        grep -q '^hyperion_commit_index '; then
      break
    fi
    sleep 0.05
  done
done

leader=""
for attempt in $(seq 1 100); do
  for id in 1 2 3 4 5; do
    if "$RUN/hyperionctl" -address "$PREFIX.$((10+id)):9200" \
      -operation put -client 1 -request 1 -key 999 -value 1 >/dev/null 2>&1; then
      leader=$id
      break 2
    fi
  done
  sleep 0.1
done
test -n "$leader"

kill "-${HYPERION_KILL_SIGNAL:-TERM}" "$(cat "$RUN/node$leader.pid")"
wait "$(cat "$RUN/node$leader.pid")" 2>/dev/null || true

backup_run=$(mktemp -d "$RUN/backup.XXXXXX")
"$RUN/hyperion-backup" -mode create \
  -data-dir "$RUN/node$leader" -backup-dir "$backup_run/image"
"$RUN/hyperion-backup" -mode restore \
  -backup-dir "$backup_run/image" -data-dir "$backup_run/restored"
cmp "$RUN/node$leader/raft.wal" "$backup_run/restored/raft.wal"

replacement=""
for attempt in $(seq 1 150); do
  for id in 1 2 3 4 5; do
    [[ $id == "$leader" ]] && continue
    if "$RUN/hyperionctl" -address "$PREFIX.$((10+id)):9200" \
      -operation get -client 2 -request "$attempt" -key 999 |
      grep -q 'status=0.*value=1'; then
      replacement=$id
      break 2
    fi
  done
  sleep 0.1
done
test -n "$replacement"

if command -v lein >/dev/null; then
  fault_node=1
  [[ $fault_node == "$leader" ]] && fault_node=2
  (
    for round in 1 2 3; do
      sleep 8
      tc qdisc replace dev "hyperion-h$fault_node" root netem loss 100%
      sleep 4
      tc qdisc del dev "hyperion-h$fault_node" root 2>/dev/null || true
    done
  ) &
  chaos_pid=$!
  export HYPERION_CLIENTS="$PREFIX.11:9200,$PREFIX.12:9200,$PREFIX.13:9200,$PREFIX.14:9200,$PREFIX.15:9200"
  set +e
  (cd "$ROOT/jepsen" && lein run -- test --no-ssh --time-limit 60) 2>&1 |
    tee "$RUN/jepsen.log"
  jepsen_status=${PIPESTATUS[0]}
  set -e
  wait "$chaos_pid"
  if [[ $jepsen_status -ne 0 ]]; then
    echo "Jepsen exited with status $jepsen_status" >&2
    exit "$jepsen_status"
  fi
  if ! grep -Eq 'Analysis valid|:valid\? true' "$RUN/jepsen.log"; then
    echo "Jepsen did not record a valid Knossos analysis" >&2
    exit 1
  fi
else
  echo "lein is required for the Jepsen/Knossos gate" >&2
  exit 1
fi

echo "phase5-sentinel-cluster PASS"
