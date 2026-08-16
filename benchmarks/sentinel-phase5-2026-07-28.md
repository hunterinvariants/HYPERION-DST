# Sentinel Phase 5 acceptance evidence — 2026-07-28

## Scope

This record qualifies the Phase 5 distributed product surface on the isolated
`sentinel` Linux host. The acceptance gate was executed from commit `215b767`
with:

```bash
bash scripts/phase5-sentinel-cluster.sh
```

The gate is fail-closed: any Go race failure, cluster/bootstrap failure,
health or metrics failure, backup/restore mismatch, Jepsen non-zero exit, or
non-valid Knossos result prevents the final PASS marker.

## Executed gates

- complete `go test ./... -race -count=1`;
- five independent `hyperiond` processes in isolated Linux network namespaces;
- versioned CRC32C peer and client frames over TCP;
- leader discovery, committed client write, graceful leader termination, and
  replacement-leader linearizable read;
- health and Prometheus metrics endpoints on every node;
- offline checksummed backup, restore, and byte-for-byte WAL comparison;
- three live 100% TC loss windows while the Jepsen workload was running;
- 60-second concurrent Jepsen register workload;
- Knossos linearizability analysis and HTML timeline generation.

## Result

Jepsen stored the authoritative result at:

```text
/opt/hyperion/Hyperion/jepsen/store/hyperion-live-linearizability/20260728T041413.407+0200/results.edn
```

The reported checker result was:

```clojure
{:linearizable {:valid? true,
                :model #knossos.model.Register{:value 571763},
                :final-paths (),
                :configs ()},
 :timeline {:valid? true},
 :valid? true}
```

Terminal acceptance marker:

```text
phase5-sentinel-cluster PASS
```

## Claim boundary

This evidence completes Phase 5. It proves the tested protocol, process,
operability, failover, live network-fault, and linearizability gates on the
named Sentinel configuration. It does not complete the Phase 6 production
qualification, physical-NVMe, extended fault-campaign, or complete formal-model
gates.
