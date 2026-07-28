# The 2026-07-28 Jepsen session, in full

Seven runs were recorded on `sentinel` between 04:10 and 04:59. Two reported
`:valid? false`. Publishing only the passing one would have been the easy thing
to do; this file exists because the failures turned out to be worth more than
the passes.

| Run | Invoked | `:ok` | Verdict | Configuration |
|---|---:|---:|---|---|
| `20260728T041055.927+0200` | 6,663 | 436 | unresolved | 60 s, stagger 0.01, 3 fault rounds |
| `20260728T041413.407+0200` | 5,763 | 957 | **true** | 60 s, stagger 0.01, 3 fault rounds |
| `20260728T044336.661+0200` | 5,820 | 984 | **false** | 60 s, stagger 0.01, 3 fault rounds |
| `20260728T044932.072+0200` | 5,827 | 965 | no `results.edn` | 60 s, stagger 0.01, 3 fault rounds |
| `20260728T045241.881+0200` | 5,894 | 1,002 | **false** | 60 s, stagger 0.01, 3 fault rounds |
| `20260728T045550.420+0200` | 5,625 | 920 | unresolved | 60 s, stagger 0.01, 3 fault rounds |
| `20260728T045906.512+0200` | 150 | 25 | **true** | 15 s, stagger 0.1, 1 fault round |

## Why two runs reported `:valid? false`

Not a consensus defect. Both counterexamples have the same shape: the history
begins with the register already holding a value, so Knossos correctly reports a
read of something that nothing *in this history* wrote.

`20260728T044336.661+0200`:

```clojure
{:op {:process 3, :type :ok, :f :read, :value 571763, :index 6},
 :model #knossos.model.Inconsistent{:msg "nil≠571763"}}
```

571763 is not an arbitrary number. It is the exact final register value of the
preceding run, `20260728T041413.407+0200`:

```clojure
:model #knossos.model.Register{:value 571763}
```

`20260728T045241.881+0200` fails the same way, with `nil≠1` and `567487≠1`:
reads returning values before any write in that history.

The workload always used register key `1`. When a history ran against a cluster
whose state machine had survived an earlier history, key `1` still held the
previous run's last value, and the checker saw a read out of nowhere. That is a
test-isolation defect, and Knossos was right both times.

## What was done about it

The immediate response at the time (commit `6397765`, "Bound Jepsen history for
deterministic Knossos analysis") reduced the workload: `stagger` 0.01 to 0.1,
time limit 60 s to 15 s, fault rounds 3 to 1. That produced the small passing run
at 04:59. It treated the symptom -- the failures stopped -- without the cause
being understood, and the resulting 25-operation history became the published
evidence while the two failures went unmentioned.

The actual fix is in `jepsen/src/hyperion/core.clj`: each run now operates on a
register key unique to that run (`HYPERION_JEPSEN_KEY` pins it when reproducing a
recorded history). A leftover value can no longer be mistaken for a violation,
whatever state the cluster is in when the history starts.

## What the session actually supports

`20260728T041413.407+0200` is the strongest recorded result and its history is
checked in, so these counts come from the file rather than from a report:

| | |
|---|---:|
| Invoked | 5,763 |
| Completed `:ok` | 957 |
| Rejected `:fail` (`:not-leader`) | 3,421 |
| Indeterminate `:info` | 1,385 (1,373 `Connection refused`, 12 `Connect timed out`) |
| Reads / writes invoked | 2,831 / 2,932 |

60 seconds at concurrency 5, under three rounds of `netem loss 100%` on one
node's veth -- `:valid? true`.

The last `:ok` write in that history is `571763`, which is also the register
value in its `results.edn` and the value the next run read out of nowhere. The
carried-over-state explanation is therefore verifiable from the checked-in files
alone.

The two failures do not weaken it. They were the harness reading a register it
had not cleaned up, and the counterexamples say so explicitly.

## Still open

- `results.edn` and `history.txt` are checked in for `041413`; for the other
  runs only `results.edn` where one exists. The remaining histories live on
  `sentinel`.
- Two runs (`041055`, `045550`) have no resolved verdict and one (`044932`) has
  no `results.edn` at all. Those were not investigated.
- The session predates the key-isolation fix, so no recorded run yet exercises
  it. The next `sentinel` campaign should, and can then restore the 60-second
  configuration that commit `6397765` reduced.
