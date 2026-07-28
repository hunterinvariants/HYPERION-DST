---------------------------- MODULE HyperionRaft ----------------------------
EXTENDS Naturals, FiniteSets, Sequences

CONSTANTS Nodes, Nil, MaxTerm, MaxLog

VARIABLES term, votedFor, role, votes, log, commitIndex, snapshotIndex,
          configOld, configNew, durableTerm, durableVote

vars == <<term, votedFor, role, votes, log, commitIndex, snapshotIndex,
          configOld, configNew, durableTerm, durableVote>>

Min2(a, b) == IF a <= b THEN a ELSE b
Max2(a, b) == IF a >= b THEN a ELSE b
Quorum(config, acks) == Cardinality(config \cap acks) * 2 > Cardinality(config)
JointQuorum(acks) == Quorum(configOld, acks) /\
                       (configNew = {} \/ Quorum(configNew, acks))
Prefix(a, b, i) == i = 0 \/
  (Len(a) >= i /\ Len(b) >= i /\ SubSeq(a, 1, i) = SubSeq(b, 1, i))

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

BecomeCandidate(n) ==
  /\ term[n] < MaxTerm
  /\ term' = [term EXCEPT ![n] = @ + 1]
  /\ votedFor' = [votedFor EXCEPT ![n] = n]
  /\ durableTerm' = [durableTerm EXCEPT ![n] = term'[n]]
  /\ durableVote' = [durableVote EXCEPT ![n] = n]
  /\ role' = [role EXCEPT ![n] = "Candidate"]
  /\ votes' = [votes EXCEPT ![n] = {n}]
  /\ UNCHANGED <<log, commitIndex, snapshotIndex, configOld, configNew>>

GrantVote(voter, candidate) ==
  /\ role[candidate] = "Candidate"
  /\ term[voter] <= term[candidate]
  /\ votedFor[voter] = Nil \/ votedFor[voter] = candidate
  /\ Prefix(log[candidate], log[voter], commitIndex[voter])
  /\ term' = [term EXCEPT ![voter] = term[candidate]]
  /\ durableTerm' = [durableTerm EXCEPT ![voter] = term[candidate]]
  /\ votedFor' = [votedFor EXCEPT ![voter] = candidate]
  /\ durableVote' = [durableVote EXCEPT ![voter] = candidate]
  /\ role' = [role EXCEPT ![voter] = "Follower"]
  /\ votes' = [votes EXCEPT ![candidate] = @ \cup {voter}]
  /\ UNCHANGED <<log, commitIndex, snapshotIndex, configOld, configNew>>

BecomeLeader(n) ==
  /\ role[n] = "Candidate"
  /\ JointQuorum(votes[n])
  /\ role' = [role EXCEPT ![n] = "Leader"]
  /\ UNCHANGED <<term, votedFor, votes, log, commitIndex, snapshotIndex,
                 configOld, configNew, durableTerm, durableVote>>

AppendEntry(n) ==
  /\ role[n] = "Leader"
  /\ Len(log[n]) < MaxLog
  /\ log' = [log EXCEPT ![n] = Append(@, term[n])]
  /\ UNCHANGED <<term, votedFor, role, votes, commitIndex, snapshotIndex,
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
       ![follower] = Min2(commitIndex[leader], Len(log[leader]))]
  /\ UNCHANGED <<votedFor, durableVote, votes, snapshotIndex,
                 configOld, configNew>>

Commit(leader, i) ==
  LET acks == {n \in Nodes : Prefix(log[n], log[leader], i)} IN
  /\ role[leader] = "Leader"
  /\ commitIndex[leader] < i
  /\ i <= Len(log[leader])
  /\ log[leader][i] = term[leader]
  /\ JointQuorum(acks)
  /\ commitIndex' = [commitIndex EXCEPT ![leader] = i]
  /\ UNCHANGED <<term, votedFor, role, votes, log, snapshotIndex,
                 configOld, configNew, durableTerm, durableVote>>

Compact(n, i) ==
  /\ snapshotIndex[n] < i
  /\ i <= commitIndex[n]
  /\ snapshotIndex' = [snapshotIndex EXCEPT ![n] = i]
  /\ UNCHANGED <<term, votedFor, role, votes, log, commitIndex,
                 configOld, configNew, durableTerm, durableVote>>

InstallSnapshot(leader, follower) ==
  /\ role[leader] = "Leader"
  /\ follower # leader
  /\ snapshotIndex[leader] > snapshotIndex[follower]
  /\ snapshotIndex' = [snapshotIndex EXCEPT ![follower] = snapshotIndex[leader]]
  /\ commitIndex' = [commitIndex EXCEPT
       ![follower] = Max2(@, snapshotIndex[leader])]
  /\ log' = [log EXCEPT ![follower] = log[leader]]
  /\ UNCHANGED <<term, votedFor, role, votes, configOld, configNew,
                 durableTerm, durableVote>>

BeginJoint(leader, config) ==
  /\ role[leader] = "Leader"
  /\ configNew = {}
  /\ config \subseteq Nodes
  /\ config # {}
  /\ config # configOld
  /\ configNew' = config
  /\ UNCHANGED <<term, votedFor, role, votes, log, commitIndex,
                 snapshotIndex, configOld, durableTerm, durableVote>>

FinalizeJoint(leader) ==
  /\ role[leader] = "Leader"
  /\ configNew # {}
  /\ JointQuorum({n \in Nodes : Prefix(log[n], log[leader], commitIndex[leader])})
  /\ configOld' = configNew
  /\ configNew' = {}
  /\ role' = [n \in Nodes |-> "Follower"]
  /\ votes' = [n \in Nodes |-> {}]
  /\ UNCHANGED <<term, votedFor, log, commitIndex, snapshotIndex,
                 durableTerm, durableVote>>

Crash(n) ==
  /\ role' = [role EXCEPT ![n] = "Follower"]
  /\ votes' = [votes EXCEPT ![n] = {}]
  /\ term' = [term EXCEPT ![n] = durableTerm[n]]
  /\ votedFor' = [votedFor EXCEPT ![n] = durableVote[n]]
  /\ UNCHANGED <<log, commitIndex, snapshotIndex, configOld, configNew,
                 durableTerm, durableVote>>

Next ==
  \/ \E n \in Nodes : BecomeCandidate(n)
  \/ \E voter, candidate \in Nodes : GrantVote(voter, candidate)
  \/ \E n \in Nodes : BecomeLeader(n)
  \/ \E n \in Nodes : AppendEntry(n)
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

Spec == Init /\ [][Next]_vars

=============================================================================