#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TOOLS="${TLA_TOOLS_JAR:-$ROOT/.cache/tla2tools.jar}"
URL="https://github.com/tlaplus/tlaplus/releases/download/v1.7.4/tla2tools.jar"
SHA1="bee4a54f3ee3d4afc347c3240ec2d9e93b075104"

if [[ ! -f "$TOOLS" ]]; then
  mkdir -p "$(dirname "$TOOLS")"
  curl --fail --location --retry 3 --output "$TOOLS" "$URL"
fi

printf '%s  %s\n' "$SHA1" "$TOOLS" | sha1sum --check -
exec java -XX:+UseParallelGC -cp "$TOOLS" tlc2.TLC \
  -cleanup -workers auto \
  -config "$ROOT/verification/tla/HyperionRaft.cfg" \
  "$ROOT/verification/tla/HyperionRaft.tla"
