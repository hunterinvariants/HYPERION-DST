# Security policy

## Reporting

Use GitHub's **private vulnerability reporting** on this repository. Do not open
a public issue for a suspected vulnerability.

This is a one-person project. There is no response-time commitment, and
pretending otherwise would be the kind of unbacked claim the rest of this
repository avoids. Reports are read and answered as soon as they are seen.

## Supported versions

The latest tag only. There are no backports.

## What is worth reporting

The parts where someone else's bytes or privileges are involved:

- **The wire format.** `protocol` decodes CRC32C-framed messages from a socket.
  A crash, hang, or unbounded allocation reachable from a malformed frame is a
  denial of service against a consensus node. This surface is fuzzed, which
  means the obvious cases are covered and the interesting ones are not.
- **The durable format.** `storage/wal` parses records and recovers torn tails
  from disk. Relevant if a node can be made to accept a crafted log.
- **The chaos controller.** `chaos` runs privileged network commands. The
  guards — the mandatory `hyperion-*` namespace, the CIDR validation, the
  bounded delay and loss — exist so that a mistake cannot reach a real
  interface. A way around any of them is a real finding.
- **The raw block-device benchmark.** `hyperion raw-bench` refuses to write
  without a canonical `/dev/` path, an exact expected size, and an explicit
  `ERASE:` confirmation. A way to make it write to the wrong device is a real
  finding.
- **Release artifacts.** Anything that would let a published binary,
  `SHA256SUMS`, or the build attestation misrepresent what it is.

## What is not a vulnerability

- **The placeholder protocol in `hyperion new`.** It is deliberately unsafe
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
