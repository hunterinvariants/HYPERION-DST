# Release checklist

A release candidate is rejected unless every applicable item has attached,
immutable evidence.

- [ ] clean commit and reproducible version identifier;
- [ ] `go test ./... -race -count=1`;
- [ ] bounded TLC model check with invariant success;
- [ ] deterministic crash/restart, compaction, and membership campaigns;
- [ ] ENOSPC, append EIO, sync EIO, torn-write, and corruption campaigns;
- [ ] Linux io_uring registered-buffer/file integration repetitions;
- [ ] five-process process-kill and network-fault campaign;
- [ ] Jepsen/Knossos result reports `valid? true`;
- [ ] backup/restore and graceful-shutdown gates;
- [ ] named storage/kernel benchmark with SMART/error-log evidence;
- [ ] operating-envelope and known-limitations review;
- [ ] artifact hashes stored with the release.

