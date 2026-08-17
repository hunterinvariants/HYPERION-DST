# Security policy

## Reporting

Use GitHub's **private vulnerability reporting** on this repository. Do not open
a public issue for a suspected vulnerability.

This is a one-person project. There is no response-time commitment, and
pretending otherwise would be the kind of unbacked claim the rest of this
repository avoids. Reports are read and answered as soon as they are seen.

## Supported versions

The latest tag only. There are no backports.

## Verifying a release

Every release ships `SHA256SUMS`, an SPDX software bill of materials, and a
Sigstore build attestation. The checksum says the file did not change in
transit. The attestation says more: which workflow, in which repository, at
which tag produced it.

**The checksum.**

```bash
curl -fsSLO https://github.com/hunterinvariants/Promtact/releases/download/vX.Y.Z/promtact-vX.Y.Z-linux-amd64
curl -fsSLO https://github.com/hunterinvariants/Promtact/releases/download/vX.Y.Z/SHA256SUMS
sha256sum -c SHA256SUMS --ignore-missing
```

**The attestation.** This needs `gh` **2.49 or newer** — the `attestation`
subcommand does not exist before that, and the `gh` in Ubuntu 24.04 is 2.45.
Install a current one from [cli.github.com](https://cli.github.com) rather than
from your distribution.

The attestation bundle is public, so no GitHub account is needed to check it:

```bash
digest=$(sha256sum promtact-vX.Y.Z-linux-amd64 | awk '{print $1}')
curl -fsSL "https://api.github.com/repos/hunterinvariants/Promtact/attestations/sha256:$digest" \
  | jq '.attestations[0].bundle' > bundle.json
gh attestation verify promtact-vX.Y.Z-linux-amd64 --bundle bundle.json \
  --repo hunterinvariants/Promtact
```

A signed-in `gh` can fetch the bundle itself, which is why the same command
without `--bundle` asks you to authenticate first.

What a passing check looks like:

```
✓ Verification succeeded!

- Attestation #1
  - Build repo:..... hunterinvariants/Promtact
  - Build workflow:. .github/workflows/release.yml@refs/tags/vX.Y.Z
```

**Which release you are holding.** The file name is not evidence; anyone can
rename a file. The binary answers for itself:

```bash
./promtact-vX.Y.Z-linux-amd64 version
```

A release binary reports its tag, the commit it was built from, and the Go
toolchain that built it. A binary built from a checkout reports `(devel)` and
its revision, and says so if the tree was modified.

## What is worth reporting

The parts where someone else's bytes or privileges are involved:

- **The wire format.** `protocol` decodes CRC32C-framed messages from a socket.
  A crash, hang, or unbounded allocation reachable from a malformed frame is a
  denial of service against a consensus node. This surface is fuzzed, which
  means the obvious cases are covered and the interesting ones are not.
- **The durable format.** `storage/wal` parses records and recovers torn tails
  from disk. Relevant if a node can be made to accept a crafted log.
- **The chaos controller.** `chaos` runs privileged network commands. The
  guards — the mandatory `promtact-*` namespace, the CIDR validation, the
  bounded delay and loss — exist so that a mistake cannot reach a real
  interface. A way around any of them is a real finding.
- **The raw block-device benchmark.** `promtact raw-bench` refuses to write
  without a canonical `/dev/` path, an exact expected size, and an explicit
  `ERASE:` confirmation. A way to make it write to the wrong device is a real
  finding.
- **Release artifacts.** Anything that would let a published binary,
  `SHA256SUMS`, or the build attestation misrepresent what it is.

## What is not a vulnerability

- **The placeholder protocol in `promtact new`.** It is deliberately unsafe
  when two nodes propose at once, and the generated test suite requires the
  engine to catch it. That is the lesson, not a defect.
- **Running the chaos controller against a real interface.** It refuses by
  design; deliberately disabling the guards and then reporting the result is
  not a finding.
- **The scope of the correctness claims.** `EVIDENCE.md` states what each
  result establishes and what it does not. A gap that is already documented
  there is a known bound, not a vulnerability. If a documented bound is wrong,
  that is worth reporting — with the measurement that shows it.
- **This being a reference implementation.** It is not a production database
  and does not claim to be.

## Disclosure

Once a fix is released, the report may be published as a GitHub security
advisory, crediting the reporter unless they prefer otherwise.
