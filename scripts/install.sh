#!/usr/bin/env bash
#
# Install a released promtact binary, after checking that it is the one this
# repository published.
#
# This script lives in the repository so that you can read it before you run
# it. Piping it straight from the network into a shell would undo the point of
# everything it does: it exists to prove the file it installs came from a named
# build, and a script you have not read cannot prove anything to you.
#
# It installs one file, needs no root, and touches no system packages.
#
#   ./scripts/install.sh                    latest release, into ~/.local/bin
#   ./scripts/install.sh -v v0.3.5          a named release
#   PREFIX=/opt/bin ./scripts/install.sh    somewhere else
#
set -u

REPO="hunterinvariants/Promtact"
PREFIX="${PREFIX:-$HOME/.local/bin}"
VERSION="${VERSION:-}"

die() { printf '\nerror: %s\n' "$*" >&2; exit 1; }
step() { printf '\n== %s\n' "$*"; }

while [ $# -gt 0 ]; do
  case "$1" in
    -v|--version) VERSION="${2:-}"; shift 2 || die "-v needs a version" ;;
    -p|--prefix)  PREFIX="${2:-}";  shift 2 || die "-p needs a directory" ;;
    # Print the header comment and stop at the first line of code, so that
    # editing the comment cannot leave the help text quoting the script.
    -h|--help)    awk 'NR>=3 && /^#/ {sub(/^# ?/,""); print; next} NR>=3 {exit}' "$0"; exit 0 ;;
    *)            die "unknown argument: $1" ;;
  esac
done

# ---------------------------------------------------------------- platform

case "$(uname -s)" in
  Linux)  os=linux ;;
  Darwin) os=darwin ;;
  *) die "no released binary for $(uname -s). Build from source instead:
  go install github.com/hunterinvariants/promtact/cmd/promtact@latest" ;;
esac

case "$(uname -m)" in
  x86_64|amd64)  arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) die "no released binary for $(uname -m)" ;;
esac

# macOS ships shasum rather than sha256sum, and a missing checksum tool must
# stop the install rather than downgrade it to a plain download.
if command -v sha256sum >/dev/null 2>&1; then
  sha256() { sha256sum "$1" | awk '{print $1}'; }
elif command -v shasum >/dev/null 2>&1; then
  sha256() { shasum -a 256 "$1" | awk '{print $1}'; }
else
  die "no sha256sum or shasum on this machine; refusing to install unverified"
fi

command -v curl >/dev/null 2>&1 || die "curl is required"

# ---------------------------------------------------------------- version

if [ -z "$VERSION" ]; then
  step "asking GitHub for the latest release"
  VERSION=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" |
            sed -n 's/.*"tag_name" *: *"\([^"]*\)".*/\1/p' | head -1)
  [ -n "$VERSION" ] || die "could not determine the latest release"
fi
printf '   %s %s/%s\n' "$VERSION" "$os" "$arch"

binary="promtact-$VERSION-$os-$arch"
base="https://github.com/$REPO/releases/download/$VERSION"

work=$(mktemp -d) || die "could not create a temporary directory"
trap 'rm -rf "$work"' EXIT
cd "$work" || die "could not enter $work"

# ---------------------------------------------------------------- download

step "downloading $binary"
curl -fsSLO "$base/$binary"  || die "no such release asset: $base/$binary"
curl -fsSLO "$base/SHA256SUMS" || die "the release has no SHA256SUMS"

# ---------------------------------------------------------------- checksum

step "checking the checksum"
want=$(awk -v f="$binary" '$2 == f || $2 == "*" f {print $1}' SHA256SUMS)
[ -n "$want" ] || die "SHA256SUMS does not list $binary"
got=$(sha256 "$binary")
if [ "$want" != "$got" ]; then
  die "checksum mismatch for $binary
  published $want
  received  $got
Nothing was installed. Do not run this file."
fi
printf '   ok  %s\n' "$got"

# ---------------------------------------------------------------- provenance

step "checking the build attestation"
gh_ok=no
if command -v gh >/dev/null 2>&1 && command -v jq >/dev/null 2>&1; then
  # gh grew "attestation" in 2.49. Older builds fail with an unknown-command
  # error that reads like a broken install, so say which it is.
  ghver=$(gh --version 2>/dev/null | sed -n 's/gh version \([0-9][0-9]*\.[0-9][0-9]*\).*/\1/p' | head -1)
  major=${ghver%%.*}; minor=${ghver#*.}
  if [ "${major:-0}" -gt 2 ] || { [ "${major:-0}" -eq 2 ] && [ "${minor:-0}" -ge 49 ]; }; then
    # The bundle is public, so this needs no GitHub account. Fetching it
    # explicitly is also what makes the check work without one.
    # --format json rather than the human output: gh prints its success banner
    # only to a terminal, so a script reading the plain output sees nothing and
    # cannot tell a reader what was verified.
    if curl -fsSL "https://api.github.com/repos/$REPO/attestations/sha256:$got" |
         jq -e '.attestations[0].bundle' > bundle.json 2>/dev/null &&
       gh attestation verify "$binary" --bundle bundle.json --repo "$REPO" \
         --format json > verify.json 2>verify.err; then
      gh_ok=yes
      built=$(jq -r '.[0].verificationResult.signature.certificate.buildSignerURI // empty' \
                verify.json 2>/dev/null)
      # Field names have moved between gh versions, so fall back to finding the
      # workflow reference anywhere in the document.
      [ -n "$built" ] || built=$(grep -oE '[^"]*\.github/workflows/[^"]*@refs/[^"]*' verify.json | head -1)
      built=${built#https://github.com/}
      if [ -n "$built" ]; then
        printf '   ok  built by %s\n' "$built"
      else
        # Saying this is better than printing a generic phrase that reads like
        # the workflow was identified when it was not.
        printf '   ok  signature verified; this script could not read the workflow name from gh\n'
      fi
    else
      die "the attestation did not verify for $binary
Nothing was installed. Report this: https://github.com/$REPO/security/advisories/new"
    fi
  else
    printf '   skipped: gh %s is older than 2.49 and has no attestation command\n' "${ghver:-?}"
  fi
else
  # Naming the tool that is actually missing matters: on a machine that has jq
  # and not gh, "needs gh and jq" sends the reader to check something that is
  # already there.
  missing=""
  command -v gh >/dev/null 2>&1 || missing="gh 2.49+"
  command -v jq >/dev/null 2>&1 || missing="${missing:+$missing and }jq"
  printf '   skipped: %s not installed\n' "$missing"
fi

if [ "$gh_ok" = no ]; then
  cat <<EOF
   The checksum above proves the file did not change on the way here. The
   attestation proves which workflow built it, which is the stronger claim.
   To check it later, see SECURITY.md, section "Verifying a release".
EOF
fi

# ---------------------------------------------------------------- install

step "installing"
mkdir -p "$PREFIX" || die "could not create $PREFIX"
chmod +x "$binary"
mv -f "$binary" "$PREFIX/promtact" || die "could not write to $PREFIX"
printf '   %s\n' "$PREFIX/promtact"

step "what was installed"
"$PREFIX/promtact" version

case ":$PATH:" in
  *":$PREFIX:"*) ;;
  *) cat <<EOF

$PREFIX is not on your PATH. Add it, or call the binary by its full path:

  export PATH="\$PATH:$PREFIX"
EOF
  ;;
esac

cat <<EOF

The scenario files the commands read live in the repository, not in the binary:

  git clone https://github.com/$REPO.git
  cd Promtact
  promtact simulate -config examples/leader-partition.json
EOF
