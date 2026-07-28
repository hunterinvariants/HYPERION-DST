---------------------------- MODULE HyperionRaft ----------------------------
EXTENDS Naturals, FiniteSets, Sequences

CONSTANTS Nodes, Nil, MaxTerm, MaxLog

VARIABLES term, votedFor, role, votes, log, commitIndex, snapshotIndex,
          configOld, configNew, durableTerm, durableVote, durableIndex

vars == <<term, votedFor, role, votes, log, commitIndex, snapshotIndex,
          configOld, configNew, durableTerm, durableVote, durableIndex>>

\* durableIndex[n] is the log prefix of n that survives a crash: everything the
\* WAL has fsynced. Entries past it exist only in memory and are lost on Crash.
\* Without this variable the model silently assumed every append was durable --
\* the one assumption the WAL implementation specifically does not make.

Min2(a, b) == IF a <= b THEN a ELSE b
Max2(a, b) == IF a >= b THEN a ELSE b
Quorum(config, acks) == Cardinality(config \cap acks) * 2 > Cardinality(config)
JointQuorum(acks) == Quorum(configOld, acks) /\
                       (configNew = {} \/ Quorum(configNew, acks))
Prefix(a, b, i) == i = 0 \/
  (Len(a) >= i /\ Len(b) >= i /\ SubSeq(a, 1, i) = SubSeq(b, 1, i))
LastTerm(entries) == IF Len(entries) = 0 THEN 0 ELSE entries[Len(entries)]
UpToDate(candidate, voter) ==
  LastTerm(candidate) > LastTerm(voter) \/
  (LastTerm(candidate) = LastTerm(voter) /\ Len(candidate) >= Len(voter))

Init ==
  /\ term = [n \in Nodes |-> 0]
  /\ votedFor = [n \in Nodes |-> Nil]
  /\ role = [n \in Nodes |-> "Follower"]
  /\ votes = [n \in Nodes |-> {}]
  /\ log = [n \in Nodes |-> <<>>]
  /\ commitIndex = [n \in Nodes |-> 0]
  /\ snapshotIndex = [n \in Nodes |-> 0]
  /\ configOld = Nodes
  /\ configNew = {}
  /\ durableTerm = term
  /\ durableVote = votedFor
  /\ durableIndex = [n \in Nodes |-> 0]

BecomeCandidate(n) ==
  /\ term[n] < MaxTerm
  /\ term' = [term EXCEPT ![n] = @ + 1]
  /\ votedFor' = [votedFor EXCEPT ![n] = n]
  /\ durableTerm' = [durableTerm EXCEPT ![n] = term'[n]]
  /\ durableVote' = [durableVote EXCEPT ![n] = n]
  /\ role' = [role EXCEPT ![n] = "Candidate"]
  /\ votes' = [votes EXCEPT ![n] = {n}]
  /\ UNCHANGED <<log, commitIndex, snapshotIndex, configOld, configNew,
                 durableIndex>>

GrantVote(voter, candidate) ==
  /\ role[candidate] = "Candidate"
  /\ term[voter] <= term[candidate]
  /\ votedFor[voter] = Nil \/ votedFor[voter] = candidate
  /\ UpToDate(log[candidate], log[voter])
  /\ Prefix(log[candidate], log[voter], commitIndex[voter])
  /\ term' = [term EXCEPT ![voter] = term[candidate]]
  /\ durableTerm' = [durableTerm EXCEPT ![voter] = term[candidate]]
  /\ votedFor' = [votedFor EXCEPT ![voter] = candidate]
  /\ durableVote' = [durableVote EXCEPT ![voter] = candidate]
  /\ role' = [role EXCEPT ![voter] = "Follower"]
  /\ votes' = [votes EXCEPT ![candidate] = @ \cup {voter}]
  /\ UNCHANGED <<log, commitIndex, snapshotIndex, configOld, configNew,
                 durableIndex>>

BecomeLeader(n) ==
  /\ role[n] = "Candidate"
  /\ JointQuorum(votes[n])
  /\ role' = [role EXCEPT ![n] = "Leader"]
  /\ UNCHANGED <<term, votedFor, votes, log, commitIndex, snapshotIndex,
                 configOld, configNew, durableTerm, durableVote, durableIndex>>

\* A leader appends to its in-memory log. durableIndex deliberately does NOT
\* move here: the entry is not on stable storage until Persist runs.
AppendEntry(n) ==
  /\ role[n] = "Leader"
  /\ Len(log[n]) < MaxLog
  /\ log' = [log EXCEPT ![n] = Append(@, term[n])]
  /\ UNCHANGED <<term, votedFor, role, votes, commitIndex, snapshotIndex,
                 configOld, configNew, durableTerm, durableVote, durableIndex>>

\* The fsync: models a successful storage/wal Sync() over the pending tail.
Persist(n) ==
  /\ durableIndex[n] < Len(log[n])
  /\ durableIndex' = [durableIndex EXCEPT ![n] = Len(log[n])]
  /\ UNCHANGED <<term, votedFor, role, votes, log, commitIndex, snapshotIndex,
                 configOld, configNew, durableTerm, durableVote>>

Replicate(leader, follower) ==
  /\ role[leader] = "Leader"
  /\ follower # leader
  /\ term[follower] <= term[leader]
  /\ Prefix(log[leader], log[follower], commitIndex[follower])
  /\ log' = [log EXCEPT ![follower] = log[leader]]
  /\ term' = [term EXCEPT ![follower] = term[leader]]
  /\ durableTerm' = [durableTerm EXCEPT ![follower] = term[leader]]
  /\ role' = [role EXCEPT ![follower] = "Follower"]
  /\ commitIndex' = [commitIndex EXCEPT
       ![follower] = Max2(@, Min2(commitIndex[leader], Len(log[leader])))]
  \* Durable before ACK: the follower has the entries in its WAL by the time the
  \* leader is allowed to count it, which is the rule the implementation states.
  /\ durableIndex' = [durableIndex EXCEPT ![follower] = Len(log[leader])]
  /\ UNCHANGED <<votedFor, durableVote, votes, snapshotIndex,
                 configOld, configNew>>

\* An ack only counts when the entry is durable on that node, so a quorum of
\* acks is a quorum of write-ahead logs, not a quorum of page caches.
Commit(leader, i) ==
  LET acks == {n \in Nodes : Prefix(log[n], log[leader], i) /\
                              durableIndex[n] >= i} IN
  /\ role[leader] = "Leader"
  /\ commitIndex[leader] < i
  /\ i <= Len(log[leader])
  /\ log[leader][i] = term[leader]
  \* The leader persists before it counts itself; a joint quorum could otherwise
  \* be reached without the leader's own WAL holding the entry.
  /\ durableIndex[leader] >= i
  /\ JointQuorum(acks)
  /\ commitIndex' = [commitIndex EXCEPT ![leader] = i]
  /\ UNCHANGED <<term, votedFor, role, votes, log, snapshotIndex,
                 configOld, configNew, durableTerm, durableVote, durableIndex>>

\* Compaction is fenced behind durability: a snapshot may only cover a prefix
\* that is already on stable storage, matching the snapshot-before-WAL-fence
\* ordering the implementation documents.
Compact(n, i) ==
  /\ snapshotIndex[n] < i
  /\ i <= commitIndex[n]
  /\ i <= durableIndex[n]
  /\ snapshotIndex' = [snapshotIndex EXCEPT ![n] = i]
  /\ UNCHANGED <<term, votedFor, role, votes, log, commitIndex,
                 configOld, configNew, durableTerm, durableVote, durableIndex>>

InstallSnapshot(leader, follower) ==
  /\ role[leader] = "Leader"
  /\ follower # leader
  /\ term[follower] = term[leader]
  /\ snapshotIndex[leader] > snapshotIndex[follower]
  /\ snapshotIndex' = [snapshotIndex EXCEPT ![follower] = snapshotIndex[leader]]
  /\ commitIndex' = [commitIndex EXCEPT
       ![follower] = Max2(@, snapshotIndex[leader])]
  /\ log' = [log EXCEPT ![follower] = log[leader]]
  \* A snapshot install is on stable storage before it is acknowledged.
  /\ durableIndex' = [durableIndex EXCEPT ![follower] = Len(log[leader])]
  /\ UNCHANGED <<term, votedFor, role, votes, configOld, configNew,
                 durableTerm, durableVote>>

BeginJoint(leader, config) ==
  LET acks == {n \in Nodes : term[n] = term[leader] /\
                              Prefix(log[n], log[leader], commitIndex[leader])} IN
  /\ role[leader] = "Leader"
  /\ configNew = {}
  /\ config \subseteq Nodes
  /\ config # {}
  /\ config # configOld
  /\ Quorum(configOld, acks)
  /\ Quorum(config, acks)
  /\ configNew' = config
  /\ UNCHANGED <<term, votedFor, role, votes, log, commitIndex,
                 snapshotIndex, configOld, durableTerm, durableVote,
                 durableIndex>>

FinalizeJoint(leader) ==
  /\ role[leader] = "Leader"
  /\ configNew # {}
  /\ JointQuorum({n \in Nodes : term[n] = term[leader] /\
                                  Prefix(log[n], log[leader], commitIndex[leader])})
  /\ configOld' = configNew
  /\ configNew' = {}
  /\ role' = [n \in Nodes |-> "Follower"]
  /\ votes' = [n \in Nodes |-> {}]
  /\ UNCHANGED <<term, votedFor, log, commitIndex, snapshotIndex,
                 durableTerm, durableVote, durableIndex>>

\* Crash and restart. The node comes back with exactly what stable storage held:
\* the durable term and vote, and the log truncated to the fsynced prefix.
\* Anything appended but not persisted is gone, and commitIndex cannot survive
\* past what remains.
Crash(n) ==
  /\ role' = [role EXCEPT ![n] = "Follower"]
  /\ votes' = [votes EXCEPT ![n] = {}]
  /\ term' = [term EXCEPT ![n] = durableTerm[n]]
  /\ votedFor' = [votedFor EXCEPT ![n] = durableVote[n]]
  /\ log' = [log EXCEPT ![n] = SubSeq(@, 1, durableIndex[n])]
  /\ commitIndex' = [commitIndex EXCEPT ![n] = Min2(@, durableIndex[n])]
  /\ UNCHANGED <<snapshotIndex, configOld, configNew,
                 durableTerm, durableVote, durableIndex>>

Next ==
  \/ \E n \in Nodes : BecomeCandidate(n)
  \/ \E voter, candidate \in Nodes : GrantVote(voter, candidate)
  \/ \E n \in Nodes : BecomeLeader(n)
  \/ \E n \in Nodes : AppendEntry(n)
  \/ \E n \in Nodes : Persist(n)
  \/ \E leader, follower \in Nodes : Replicate(leader, follower)
  \/ \E leader \in Nodes, i \in 1..MaxLog : Commit(leader, i)
  \/ \E n \in Nodes, i \in 1..MaxLog : Compact(n, i)
  \/ \E leader, follower \in Nodes : InstallSnapshot(leader, follower)
  \/ \E leader \in Nodes, config \in SUBSET Nodes : BeginJoint(leader, config)
  \/ \E leader \in Nodes : FinalizeJoint(leader)
  \/ \E n \in Nodes : Crash(n)

TypeOK ==
  /\ term \in [Nodes -> 0..MaxTerm]
  /\ durableTerm \in [Nodes -> 0..MaxTerm]
  /\ votedFor \in [Nodes -> Nodes \cup {Nil}]
  /\ durableVote \in [Nodes -> Nodes \cup {Nil}]
  /\ role \in [Nodes -> {"Follower", "Candidate", "Leader"}]
  /\ votes \in [Nodes -> SUBSET Nodes]
  /\ \A n \in Nodes : Len(log[n]) <= MaxLog
  /\ \A n \in Nodes : \A i \in 1..Len(log[n]) : log[n][i] \in 0..MaxTerm
  /\ commitIndex \in [Nodes -> 0..MaxLog]
  /\ snapshotIndex \in [Nodes -> 0..MaxLog]
  /\ durableIndex \in [Nodes -> 0..MaxLog]
  /\ \A n \in Nodes : durableIndex[n] <= Len(log[n])
  /\ configOld \subseteq Nodes /\ configOld # {}
  /\ configNew \subseteq Nodes

ElectionSafety ==
  \A t \in 0..MaxTerm :
    Cardinality({n \in Nodes : role[n] = "Leader" /\ term[n] = t}) <= 1

CommittedPrefixSafety ==
  \A a, b \in Nodes :
    \A i \in 1..Min2(commitIndex[a], commitIndex[b]) :
      Prefix(log[a], log[b], i)

SnapshotSafety == \A n \in Nodes : snapshotIndex[n] <= commitIndex[n]
DurableVoteSafety == \A n \in Nodes : term[n] = durableTerm[n] /\
                                         votedFor[n] = durableVote[n]

\* Nothing counts as committed on a node beyond what that node has actually
\* written to stable storage -- the property a crash is otherwise free to break.
DurableCommitSafety == \A n \in Nodes : commitIndex[n] <= durableIndex[n]

\* A snapshot never covers more than the durable log it was taken from.
DurableSnapshotSafety == \A n \in Nodes : snapshotIndex[n] <= durableIndex[n]

Spec == Init /\ [][Next]_vars

=============================================================================
