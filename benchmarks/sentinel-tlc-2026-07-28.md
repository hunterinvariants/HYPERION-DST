# Sentinel TLC qualification

Date: 2026-07-28

## Environment

- Linux kernel: 6.8.0-136-generic amd64
- Java: OpenJDK 21.0.11
- TLC: 2.19 (TLA+ tools 1.7.4)
- model: `verification/tla/HyperionRaft.tla`
- configuration: three nodes, maximum term 3

## Result

- outcome: no error found
- generated states: 1,765
- distinct reachable states: 343
- states remaining: 0
- complete graph depth: 13
- fingerprint-collision probability estimate: 2.6E-14

This is executable evidence for the bounded durable-election/crash model and
its `TypeOK` and `ElectionSafety` invariants. It does not constitute a proof of
the production implementation or of protocol behaviors absent from the model.
