# HYPERION-DST safety specification

## Model

Let `N` be the finite node set, `T` the monotonically increasing term domain,
and `L_n[i] = (term, command)` the log entry at index `i` on node `n`. A quorum
is any `Q subseteq N` where `|Q| > |N|/2`. Nodes may crash and later recover
only from stable state. Messages may be delayed, duplicated, reordered, or
dropped, but not forged.

## Election safety

A node persists `(currentTerm, votedFor)` before granting a vote. It grants at
most one vote in a term:

```text
for all n in N, t in T:
  |{c : VoteGranted(n, c, t)}| <= 1
```

Two leaders in one term would each require a majority. Any two majorities
intersect, so an intersection node would have voted twice in the same term,
contradicting the persisted-vote rule. Therefore:

```text
for all t in T:
  |{n : Leader(n, t)}| <= 1
```

## Pre-vote and duplicate resistance

Election timeout starts a non-persistent pre-vote for `currentTerm + 1`. Receiving
or granting a pre-vote never advances durable term or vote state. Only a
majority of distinct configured voters starts a real election. Vote identities
are tracked in a fixed 64-bit voter mask, so duplicated or unknown responses
cannot form a quorum. A node with a recently active leader rejects disruptive
pre-votes.

Consequently, an isolated minority can retry pre-vote indefinitely without
increasing the term observed by the healthy majority.

## Log matching

AppendEntries is accepted at index `i` only when the follower has the same term
as the leader at `i-1`. On conflict, the follower deletes the conflicting
suffix before appending:

```text
L_a[i].term = L_b[i].term
  implies for all j <= i: L_a[j] = L_b[j]
```

The base sentinel is common. The predecessor check establishes the induction
step, and accepted entries are copied identically.

## Leader completeness and state-machine safety

An entry advances the leader commit index only after storage on a quorum and,
for commit advancement, only when the entry is from the leader's current term.
Every later election quorum intersects that storage quorum. The up-to-date-log
vote rule prevents a candidate missing the committed entry from receiving the
intersection vote.

Nodes apply committed entries strictly in increasing index order. Therefore:

```text
for all a, b in N and index i:
  applied_a(i) and applied_b(i) implies L_a[i] = L_b[i]
```

## Persistence ordering

The implementation enforces these happens-before relationships:

```text
Persist(term, vote) happens-before Send(VoteGranted)
Persist(log[index]) happens-before Send(AppendAccepted(index))
WriteFixed(block) happens-before CQE(write)
CQE(write) happens-before Submit(FSYNC)
CQE(FSYNC) happens-before DurableSuccess
```

A persistence error moves the node to fail-stop state and clears unsent
responses. It must never acknowledge state that was not reported durable.

## WAL integrity and recovery

Each fixed record contains magic, version, sequence, index, term, command, and a
CRC32C checksum. Recovery accepts only a contiguous, checksum-valid prefix. A
partial final record is truncated before a later append. Complete corrupt
records and sequence gaps are fatal.

For snapshot index `s` and durable WAL tail ending at `last_seq`:

```text
State(node, recovery_time) =
  Fold(Snapshot(s), WAL[s+1 .. last_seq])
```

The snapshot file format, ordered durable replacement, InstallSnapshot,
follower catch-up, absolute compacted indexes, and WAL compaction fences are
implemented and covered by crash/restart and Linux io_uring acceptance tests.

## Linearizability boundary

A completed write linearizes when its log entry first becomes committed. Raft
implements quorum-confirmed ReadIndex barriers for linearizable reads. A
production client protocol remains a Phase 5 gate, so end-to-end strict
serializability is not yet claimed.

For completed operations in history `H`, a valid sequential history `S` must
preserve real-time precedence:

```text
op1 precedes_H op2 implies op1 precedes_S op2
```

## Membership changes

During joint consensus, commit requires a majority of both the old and new
voter sets. Replicated joint and final entries execute both transition phases,
survive restart, restore pending election quorums, and step down a leader
removed by the final configuration.

## Proof boundary

These are engineering proof arguments over the stated model, supported by
bounded deterministic tests. They are not machine-checked proofs. The current
TLA+ model covers durable election state and crash recovery; AppendEntries,
commit, snapshot, and membership model checking remain required.
