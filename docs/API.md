# Public API surface

Go exports everything with a capital letter, which makes every identifier in
this repository look equally like a promise. Most are not. This document says
which ones are, and `internal/apisurface` enforces it: the contractual surface
is recorded in `docs/api-surface.txt`, and a test fails if it drifts without
that file being updated in the same change.

## Status

No version is tagged yet. Until one is, consumers get a pseudo-version and
**nothing here is stable**. The tiers below describe what stability will mean
once tagging starts, and what breaking each tier would cost.

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

The `raft` entry is narrow on purpose. Only the persistence boundary is
contractual, because that is what an alternative storage backend implements.
The rest of `raft` is tier 2.

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

Removing a flag, changing its default, or changing an exit code is a break at
tier 1 severity.

## Versioning

Semantic versioning once tagging starts, at `v0.x.y` until the framework
surface has survived contact with an outside user:

- **patch** (`v0.1.0` to `v0.1.1`): tier 2 changes, fixes, new tests, evidence.
- **minor** (`v0.1.x` to `v0.2.0`): any tier 1 change, any CLI flag or exit
  code change, any new tier 1 identifier.
- **major**: reserved for `v1.0.0`, which should not happen before the
  framework surface has been used by someone who did not write it.

A release must carry the gate evidence for the commit it tags, following the
practice in `benchmarks/`.

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
