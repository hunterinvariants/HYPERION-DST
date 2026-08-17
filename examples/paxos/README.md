# Single-decree Paxos on the Promtact engine

A complete, self-contained example of driving a protocol that is **not Raft**
through `dst`. Roughly 250 lines of protocol and 60 lines of wiring.

```bash
go test ./examples/paxos -v
```

## What is here

| File | Role |
|---|---|
| `paxos.go` | The protocol. Proposers, acceptors, four message types. Knows nothing about the engine. |
| `cluster.go` | The adapter. Six methods: four for `dst.Cluster`, two for `dst.Wire`. |
| `invariants.go` | Three properties the engine checks after every step. |
| `scenario.json` | A declared run: seed, topology, faults, cadence. |
| `paxos_test.go` | The campaigns, including a mutation test and the scenario runner. |

## The adapter is the whole integration

```go
func (c *Cluster) Nodes() []uint32                          { return c.ids }
func (c *Cluster) Tick(id uint32)                           { c.nodes[id].Tick() }
func (c *Cluster) Deliver(id uint32, m Message)             { c.nodes[id].Step(m) }
func (c *Cluster) Drain(id uint32, dst []Message) []Message { return c.nodes[id].Drain(dst) }
func (c *Cluster) Route(m Message) (uint32, uint32)         { return m.From, m.To }
func (c *Cluster) Digest(m Message) (uint8, uint64)         { return uint8(m.Kind), m.Num }
```

That is all the engine needs. It supplies virtual time, a seeded schedule,
message loss and delay, fault injection, invariant checking, and a
reproducible trace.

## The property that matters

Paxos exists so that **no two different values are ever chosen**, however
badly the network behaves. That is `checkSingleChoice`, and the engine
evaluates it after every single step:

```go
engine.Watch(cluster.SafetyInvariants()...)
if err := engine.RunChecked(600); err != nil {
    var v *dst.Violation
    errors.As(err, &v)
    // v.Invariant, v.Step, v.Trace — a reproducible coordinate, not just a message
}
```

## Faults are declared, not scripted

```go
partition := dst.During(0, 400, dst.Split([]uint32{1, 2}, []uint32{3, 4, 5}))
engine.Inject(partition)
```

Because injectors are consulted *after* the engine draws each message's random
loss and delay, the same seed produces the same schedule with and without the
partition. A run with the fault and a run without it are directly comparable.

## Every campaign refuses to pass vacuously

This is the part worth copying, more than the protocol:

- `TestCompetingProposersStaySafe` fails if no proposer ever had to **adopt**
  an already-accepted value. That branch is the only reason competing
  proposers are safe; a campaign that never triggers it has not tested Paxos,
  it has tested a single proposer succeeding. Across 200 seeds it currently
  fires about 3,000 times.
- `TestMinorityCannotChoose` fails if the partition dropped no message, and
  fails if nothing is chosen after healing.
- `TestAgreementInvariantDetectsTwoChosenValues` forges a second chosen value
  and requires the invariant to catch it. An invariant that never fires is
  indistinguishable from one that is never evaluated.
- `TestRunIsReproducible` runs the same seed twice and compares trace hashes.
  A protocol that iterated a map or read the clock would fail here.

## The scenario format is not Raft-specific

`scenario.json` is parsed by `dst/scenario`, the same package the Raft
`promtact simulate` command uses. Only the runner differs, and for this
protocol it is about twenty lines — see `TestScenarioFileDrivesPaxos`.

## What this example is not

The protocol is written for clarity. It keeps every acceptance in memory so the
invariants can reconstruct which values reached a quorum, it has no
persistence, and it makes no attempt at the optimizations a real deployment
needs. The qualified, durable consensus implementation in this repository is
`raft/`, not this.
