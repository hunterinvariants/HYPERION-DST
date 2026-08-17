# Contributing

This project has one rule that everything else follows from: **no claim without
evidence bounding it.** A change that adds a capability must also add the thing
that would catch it being wrong, and must say what it does not establish.

That bar is higher than most repositories set. It is the reason the numbers here
can be trusted, so it is not negotiable, and this document exists so that it is
at least predictable.

## Before you open a pull request

Run these. They are the same gates CI runs.

```bash
go vet ./...
go test ./... -race -count=1
```

If you changed anything under `dst/`, also run the wide sweep. It is the gate
that catches a scheduling or protocol change that the default seed count misses:

```bash
PROMTACT_EQUIV_SEEDS=1000 go test ./dst/... -count=1
```

The race detector needs cgo. On Windows without a C toolchain it cannot run at
all; use Linux, or rely on CI.

## Tests must be able to fail

A test that cannot fail is worse than no test, because it reports confidence it
did not earn. Every kind of assertion here carries a guard against that:

- **An equality assertion needs a negative control.** If two things are
  compared and required to match, prove they can differ. See
  `TestEquivalenceComparisonHasTeeth`.
- **An invariant needs a mutation test.** Corrupt exactly the state the
  property protects and require it to report the violation. See the three in
  `dst/raftcluster/invariants_test.go`.
- **An injected fault needs a drop-count assertion.** A partition that dropped
  nothing proves nothing. Assert on `InjectedDrops()`.
- **A campaign needs proof it reached the interesting state.** The Paxos
  example fails if no proposer ever had to adopt an already-accepted value,
  because without that branch the campaign tested a single proposer succeeding
  rather than Paxos.

If you cannot write the guard, say so in the pull request rather than leaving
the reader to assume it exists.

## Determinism

Anything reachable from the engine must be deterministic. The usual way to
break it is iterating a Go map where the order affects behavior. `TraceHash()`
is the detector: run the same seed twice and compare.

## Evidence

A change that produces measurements records them in `benchmarks/`, naming:

- the commit under test and the host;
- the numbers, from a run that actually happened;
- an explicit list of what the run does **not** establish.

Never write a number into an evidence document before the run that produces it
has finished. If a measurement contradicts an earlier one, say so in the
document and explain which is right, several reports here carry withdrawn
claims for exactly that reason.

## What not to touch

`EVIDENCE.md`, `STATUS.md`, and `ROADMAP.md` describe the six completed roadmap
phases and are frozen. Framework work claims no phase acceptance and does not
amend them. If you believe one of them is wrong, open an issue with the
measurement that shows it rather than editing it.

`sim` is retained deliberately. It is the reference the equivalence campaigns
compare `dst` against; removing it removes the gate.

The safety guards in `chaos` (the mandatory `promtact-*` namespace, the CIDR
validation, the bounded delay and loss) are not extension points. They stand
between a test and a broken production network.

## API changes

`docs/API.md` says which identifiers are contractual. If you change one,
`internal/apisurface` will fail; regenerate the record in the same commit so a
reviewer sees the change:

```bash
go test ./internal/apisurface -update
```

## Licensing of contributions

The project is under the Apache License 2.0. Section 5 of that license already
governs what you send: unless you say otherwise in writing, a contribution you
submit for inclusion is licensed under the same terms. There is no separate
agreement to sign.

Do not paste in code you did not write, or code under an incompatible license.
If a change is derived from another project, say so in the pull request with a
link, so the provenance is recorded rather than reconstructed later.

## Conventions

- Commit subjects are one line, imperative, no body, no trailers.
- Pull request descriptions are one short sentence. Detail belongs in the
  commits and in `benchmarks/`.
- Match the surrounding code. Comments here explain why, not what.
