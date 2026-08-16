# Sentinel command extraction qualification

Date: 2026-08-16

Commit under test: `0ddd3b7` on branch `framework/hyperion-cli`.

## Purpose

Every command implementation moved from its `cmd/<name>/main.go` into
`internal/cli`, where it exists exactly once. Two entry points reach it: the
historical standalone binary, which keeps its name, flags, output, and exit
codes, and the new `hyperion` umbrella command, which offers each one as a
subcommand.

The recorded qualification gates invoke the standalone binaries by name, and
`scripts/*.sh` and `.github/workflows/ci.yml` do the same. Behavioral identity
of those binaries is therefore the acceptance criterion for this change, not a
nicety.

Commands requiring kernel facilities a build cannot reach are absent from the
umbrella rather than listed and then failing: `chaos` needs Linux, `raw-bench`
needs linux/amd64.

## Host

- Linux `sentinel`, kernel `6.8.0-137-generic`, x86_64;
- Go `go1.25.0 linux/amd64`.

Cross-compilation was additionally checked on the development host for
`windows/amd64`, `linux/amd64`, `linux/arm64`, and `darwin/arm64`, because the
command set is assembled by build tag and a mistake there is invisible on a
single platform.

## Executable qualification

- `go vet ./...` on all four platform targets: pass;
- complete Go race suite `go test ./... -race -count=1`: pass;
- differential comparison of seven binaries against `daf6eee`: identical;
- five-process cluster gate `scripts/phase5-sentinel-cluster.sh`: pass.

## Differential comparison

Both revisions were built into separate directories from a `git worktree` at
`daf6eee` and from the branch, then invoked with identical arguments. Standard
output, standard error, and exit status were compared as one string.

| Binary | Invocation | Result |
|---|---|---|
| hyperion-sim | `-seed 0x4A2C -steps 4000 -nodes 5` | identical |
| hyperion-seeds | `-from 1 -to 40 -steps 500` | identical |
| hyperion-probe | `-entries 32` | identical |
| hyperion-backup | `-mode bogus` | identical |
| hyperionctl | `-operation nope` | identical |
| hyperion-chaos | no arguments | identical |
| hyperion-raw-bench | no arguments | identical |

The last two rows are the ones that matter most. `hyperion-chaos` must refuse
without `-yes-really` before touching privileged network state, and
`hyperion-raw-bench` must refuse without a canonical device before it can write
to a block device. Both still refuse, with byte-identical output and exit
status.

### Known deviation

On a flag error the usage header changed from the temporary build path of the
executable to the command name:

```
-Usage of /tmp/go-build.../hyperion-sim:
+Usage of hyperion-sim:
```

This follows from parsing into a named `flag.FlagSet` instead of the global
`flag.CommandLine`. The error line, the flag list, and the exit status are
unchanged. No script or workflow parses this header.

## Coverage of the daemon

`hyperiond` cannot be verified by comparing one invocation's output: it is a
long-running process, and its peer-parsing is the kind of code whose regression
only appears once a cluster tries to form. It was instead covered by the
existing Phase 5 gate, which builds `hyperiond`, `hyperionctl`, and
`hyperion-backup` from this branch and runs the five-process campaign:

- five nodes in isolated namespaces reached health and metrics;
- a leader accepted a client write, was killed, and a replacement served the
  committed value;
- offline backup and restore produced a byte-identical WAL;
- Jepsen/Knossos under leader kill and 100 percent network loss on one node
  reported `{:linearizable {:valid? true} :timeline {:valid? true} :valid? true}`;
- `phase5-sentinel-cluster PASS`.

Recorded result:
`jepsen/store/hyperion-live-linearizability/20260816T210848.678+0200/results.edn`.

## Claim boundary

Behavioral identity is established for the invocations listed above. It is not
established for:

- the success paths of `hyperion-uring-bench` and `hyperion-raw-bench`, whose
  output is timing-dependent and, for the latter, destructive to a block
  device; only their argument validation and refusal paths were compared;
- the privileged path of `hyperion-chaos`, which was not exercised; only its
  refusal was compared;
- `hyperiond` output, which was not compared at all. Its coverage is
  behavioral, through the Phase 5 gate, not textual.

The extracted implementations are the previous ones moved without edits to
their logic. That is a claim about how the change was made, and it is
supported by the comparisons above rather than by inspection alone.

## Repository hygiene

`scripts/phase4-sentinel-gate.sh`, `scripts/phase5-sentinel-cluster.sh`, and
`scripts/sentinel-report.sh` were tracked without the executable bit while the
three Phase 6 scripts carried it. Running the Phase 5 gate required an explicit
interpreter. The mode is now `100755` for all six. This is unrelated to the
command extraction and changes no script content.

## Relationship to the qualified baseline

This report covers branch work and does not amend the frozen evidence index.
`EVIDENCE.md`, `STATUS.md`, and `ROADMAP.md` continue to describe the six
completed roadmap phases and are unchanged. The Phase 5 gate was re-run here as
a regression check on this branch; it does not restate or supersede the Phase 5
acceptance recorded in `benchmarks/sentinel-phase5-2026-07-28.md`.
