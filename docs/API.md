# Public API surface

Go exports everything with a capital letter, which makes every identifier in
this repository look equally like a promise. Most are not. This document says
which ones are, and `internal/apisurface` enforces it: the contractual surface
is recorded in `docs/api-surface.txt`, and a test fails if it drifts without
that file being updated in the same change.

## Status

The contractual surface is recorded in `docs/api-surface.txt` and gated by
`internal/apisurface`: it cannot drift without a reviewable diff. From `v0.1.0`
on, changing it requires a minor version bump.

## Tier 1: the framework surface

Contractual. Changing these breaks code written against the framework, and a
break requires a minor version bump with the reason recorded in the release
notes.

| Package | Surface |
|---|---|
| `dst` | `Engine[M]`, `New`, `Config`, `Cluster[M]`, `Wire[M]`, `Invariant`, `InvariantFunc`, `Violation`, `Injector`, `InjectorFunc`, `Split`, `Isolate`, `Link`, `During` |
| `dst/scenario` | `Scenario`, `Fault`, `Load`, `Read`, `MaxNodes`, and the JSON field names |
| `storage/wal` | `Device`, `Log`, `Open`, `Record`, `Encode`, `Decode`, `Recover`, `ErrChecksum`, `ErrInvalidTear`, `MemoryDevice`, `FileDevice` |
| `storage/storagetest` | `RunDeviceSuite`, `NewDevice` |
| `storage` | `Entry` |
| `raft` | `StableStore`, `SnapshotStore`, `HardState`, `Entry`, `Snapshot`, `ErrStorage` |
| `server` | `Spec`, `SpecNode`, `LoadSpec`, `ReadSpec`, `Spec.ConfigFor`, `Spec.Validate`, `MaxNodeID`, and the cluster JSON field names |

The `raft` and `server` entries are narrow on purpose. In `raft` only the
persistence boundary is contractual, because that is what an alternative
storage backend implements. In `server` only the cluster file format is, because
that is what an operator writes; `server.Config`, `server.Server`, and the rest
are tier 2 and may change in a patch release.

### What counts as a break

- Removing or renaming any identifier above.
- Adding a method to `Cluster`, `Wire`, `Invariant`, `Injector`, or `Device`.
  Every existing implementation stops compiling.
- Changing a method signature, or the meaning of a return value.
- Removing a JSON field from `Scenario` or `Fault`, or changing how one is
  interpreted. Adding a field is not a break, because parsing rejects unknown
  fields only in the other direction.

Adding a field to `Config` or `Scenario` is not a break for keyed struct
literals, which is the only form this project uses or documents.

## Tier 2: the reference implementation

Public because Go has no other way to compose packages, but not contractual.
These may change in a patch release. Do not build on them without pinning.

`raft` (beyond the persistence boundary), `sim`, `server`, `protocol`,
`statemachine`, `backup`, `chaos`, `storage/raftwal`, `storage/raftstore`,
`storage/snapshot`, `storage/uring`, `storage/uringwal`, `storage/faultdisk`,
`dst/raftcluster`.

Two of these deserve a note:

- **`sim`** is retained deliberately. It is the linear-scan reference the
  equivalence campaigns compare `dst` against, and removing it would remove the
  gate. It is not a recommended entry point for new work; use `dst`.
- **`dst/raftcluster`** is an example of implementing the tier 1 interfaces,
  the same role `examples/paxos` plays for a non-Raft protocol. It is not a
  supported Raft API.

## Tier 3: internal

`internal/cli` cannot be imported from outside the module, which the compiler
enforces. The command implementations live there precisely so that the CLI can
be restructured without it being an API change.

## The command line is also an interface

`cmd/hyperion` and the nine standalone binaries have contractual **flags and
exit codes**, because `scripts/*.sh`, `.github/workflows/ci.yml`, and the
recorded evidence in `benchmarks/` invoke them by name and branch on their
status. Output format is not contractual, with one exception: the final status
line of `hyperion-seeds` is quoted in evidence documents.

`simulate` and `verify` exist only as subcommands of `hyperion`. They are newer
than the extraction, nothing historical refers to them, and a new command does
not need a standalone binary to keep a promise nobody made.

Removing a flag, changing its default, or changing an exit code is a break at
tier 1 severity.

## Versioning

Semantic versioning from `v0.1.0`:

- **patch** (`v0.1.0` to `v0.1.1`): tier 2 changes, fixes, new tests, evidence.
- **minor** (`v0.1.x` to `v0.2.0`): any tier 1 change, any CLI flag or exit
  code change, any new tier 1 identifier.
- **major** (`v1.0.0`): a redesign of the framework surface.

A release must carry the gate evidence for the commit it tags, following the
practice in `benchmarks/`.

## Releasing

Pushing a `v*` tag runs `.github/workflows/release.yml`. It refuses to publish
anything until the tagged commit passes `go vet`, the full race suite, and the
1,000-seed sweep, because a release that skipped its gates would be the kind of
unbacked claim this repository exists to avoid.

It then builds the `hyperion` umbrella binary for linux, darwin, and windows on
amd64 and arm64, writes `SHA256SUMS`, generates an SPDX bill of materials, and
records a Sigstore build attestation with the workflow's own OIDC identity —
there is no signing key to store or leak.

```bash
gh attestation verify hyperion-v0.1.0-linux-amd64 --repo hunterinvariants/hyperion
sha256sum -c SHA256SUMS
```

Only the umbrella binary ships. It reaches every command, and the standalone
binaries exist for the recorded gate scripts, which build from source.

## Keeping this file honest

`docs/api-surface.txt` lists every exported identifier in the tier 1 packages.
`internal/apisurface` regenerates that list from the source and fails if it
differs. An intentional API change therefore shows up as a diff in a checked-in
file, in the same commit that makes it, where a reviewer can see it. An
unintentional one fails the build.

Regenerate after an intentional change:

```bash
go test ./internal/apisurface -update
```
