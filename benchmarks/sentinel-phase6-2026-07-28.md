# Sentinel Phase 6 qualification

Date: 2026-07-28

## Executable qualification

- complete Go race suite: pass;
- deterministic ENOSPC, append EIO, sync EIO, torn-write, corruption, and
  fail-stop acknowledgement campaigns: pass;
- extended crash/restart, compaction, and membership campaigns: pass;
- five-process leader `SIGKILL` and network-loss campaign: pass;
- Jepsen/Knossos register analysis: `valid? true`;
- operating envelope and release checklist: checked in.

Jepsen result:
`jepsen/store/hyperion-live-linearizability/20260728T045906.512+0200/results.edn`.

## Bounded formal verification

TLC 2.19 checked the three-node model at commit `7b131ec` with:

- maximum term: 2;
- maximum log length: 2;
- modeled transitions: election, durable vote, AppendEntries replication,
  current-term commit, compaction, InstallSnapshot, joint/final membership,
  and crash recovery;
- invariants: type safety, election safety, committed-prefix safety,
  snapshot safety, and durable-vote safety.

Final result:

- 46,667,923 states generated;
- 6,121,927 distinct states;
- complete state graph depth: 25;
- zero states left on the queue;
- no invariant violation.

TLC fingerprint-collision estimates were `1.3E-5` optimistic and `3.3E-6`
from the actual fingerprints. This is bounded model checking, not an
unbounded mathematical proof.
