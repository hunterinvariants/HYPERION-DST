---------------------------- MODULE HyperionRaft ----------------------------
EXTENDS Naturals, FiniteSets, Sequences

CONSTANTS Nodes, Nil, MaxTerm

VARIABLES term, votedFor, role, log, commitIndex, durableTerm, durableVote

vars == <<term, votedFor, role, log, commitIndex, durableTerm, durableVote>>

Quorum(Q) == Q \subseteq Nodes /\ Cardinality(Q) * 2 > Cardinality(Nodes)

Init ==
  /\ term = [n \in Nodes |-> 0]
  /\ votedFor = [n \in Nodes |-> Nil]
  /\ role = [n \in Nodes |-> "Follower"]
  /\ log = [n \in Nodes |-> <<>>]
  /\ commitIndex = [n \in Nodes |-> 0]
  /\ durableTerm = term
  /\ durableVote = votedFor

BecomeCandidate(n) ==
  /\ term[n] < MaxTerm
  /\ role' = [role EXCEPT ![n] = "Candidate"]
  /\ term' = [term EXCEPT ![n] = @ + 1]
  /\ votedFor' = [votedFor EXCEPT ![n] = n]
  /\ durableTerm' = [durableTerm EXCEPT ![n] = term'[n]]
  /\ durableVote' = [durableVote EXCEPT ![n] = n]
  /\ UNCHANGED <<log, commitIndex>>

Crash(n) ==
  /\ role' = [role EXCEPT ![n] = "Follower"]
  /\ term' = [term EXCEPT ![n] = durableTerm[n]]
  /\ votedFor' = [votedFor EXCEPT ![n] = durableVote[n]]
  /\ UNCHANGED <<log, commitIndex, durableTerm, durableVote>>

Next ==
  \/ \E n \in Nodes: BecomeCandidate(n)
  \/ \E n \in Nodes: Crash(n)

ElectionSafety ==
  \A t \in 0..MaxTerm:
    Cardinality({n \in Nodes: role[n] = "Leader" /\ term[n] = t}) <= 1

TypeOK ==
  /\ term \in [Nodes -> 0..MaxTerm]
  /\ durableTerm \in [Nodes -> 0..MaxTerm]
  /\ commitIndex \in [Nodes -> Nat]

Spec == Init /\ [][Next]_vars

=============================================================================

