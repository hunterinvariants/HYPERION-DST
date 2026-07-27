# HYPERION-DST Safety Specification

## Model

Let `N` be the finite node set, `T` the monotonically increasing term domain,
and `L_n[i] = (term, command)` the log entry at index `i` on node `n`.
A quorum is any `Q ⊆ N` such that `|Q| > |N|/2`. Nodes fail by crash-stop in
one execution and may recover from stable storage in a later execution.
Messages may be delayed, duplicated, reordered, or dropped, but not forged.

## Election safety

A node persists `(currentTerm, votedFor)` before granting a vote. It grants at
most one vote in a term:

`∀ n ∈ N, t ∈ T: |{c : VoteGranted(n, c, t)}| ≤ 1`.

Two leaders in the same term would each require a quorum. Any two majorities
intersect, so some node would have voted twice in that term, contradicting the
rule above. Therefore:

`∀ t ∈ T: |{n : Leader(n, t)}| ≤ 1`.

## Log matching

AppendEntries is accepted at index `i` only when the follower has the same term
as the leader at `i-1`. On conflict, the follower deletes the conflicting
suffix before appending:

`L_a[i].term = L_b[i].term ⇒ ∀ j ≤ i: L_a[j] = L_b[j]`.

This follows by induction on `i`: the base sentinel is common; the acceptance
check establishes the predecessor, and entries are copied identically.

## Leader completeness and state-machine safety

An entry is committed only after storage on a quorum and, for a leader's
commit-index advancement, only if it is from the leader's current term.
Every later election quorum intersects that storage quorum. The up-to-date-log
vote rule prevents a candidate missing the committed entry from collecting the
intersection vote. Thus every later leader contains every committed entry.

Since nodes apply committed entries strictly in increasing index order and log
matching makes committed prefixes identical:

`∀ a,b ∈ N, i: applied_a(i) ∧ applied_b(i) ⇒ L_a[i] = L_b[i]`.

## Linearizability

For completed operations `op ∈ H`, define the linearization point of a write as
the first instant its log entry becomes committed, and of a read as the instant
a quorum-confirmed leader observes its applied index. A valid sequential
history `S` must preserve real-time precedence:

`∀ op₁,op₂ ∈ H: op₁ ≺H op₂ ⇒ op₁ ≺S op₂`.

The current executable slice implements replicated writes. Linearizable reads
require the planned ReadIndex/lease proof and are not yet exposed.

## Crash/replay equivalence

For snapshot index `s` and durable WAL tail ending at `last_seq`:

`State(N,t_rec) = Fold(Snapshot(s), WAL[s+1..last_seq])`.

Recovery must reject torn or checksum-invalid records and must never expose an
acknowledged entry that was not durably persisted according to the configured
durability policy.

## Proof boundary

These are paper proofs over the stated model, not machine-checked proofs.
`go test ./...` performs bounded randomized invariant checking. TLA+/Apalache
model checking and Jepsen/Knossos live verification are roadmap gates.

