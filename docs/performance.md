# Performance and acceptance

## Caching

`--cache-dir` enables complete-report reuse. Every lookup reloads package metadata
and hashes selected dependency/standard-library source, source-file inventory,
module files, configuration, Go environment identity and the analyzer executable.
The UTC date participates so review dates cannot expire behind a cache hit.
Inputs are checked before and after analysis/reuse. Corrupt cache records cause
fresh analysis. Failed/incomplete runs are not cached as acceptance.

The cache stores results, including unresolved findings: cache hit does not mean
clean. It is a local performance facility, not a signed attestation. CGO reports
are always analyzed freshly because native compiler/environment dependencies are
not fully modeled by this v1 cache. Cache entries have content-derived names and
atomic writes; simultaneous processes can do redundant work but cannot expose a
partially written report. There is no daemon or incremental SSA persistence.

## Measurement

Use the same binary, root, configuration and resource settings for cold/warm/cache
comparisons. The first analysis is cold with respect to this tool, not necessarily
Go's global cache. Record whole-process elapsed time, maximum RSS, report size,
load/SSA/RTA phases and cache-hit status. `--cpuprofile` supports standard Go pprof.
Performance targets are evaluated on real projects, not inferred from fixture speed.

## Acceptance procedure

1. Controlled fixtures establish known answers for dead chains/cycles, generics,
   interfaces, callbacks, test-only code, multiple entry points, alternate platforms,
   configuration omissions, cache invalidation and incomplete loading.
2. Real repositories establish practical scope, runtime, memory and usefulness.
3. Use external configs/reports. Compare read-only source manifests before/after.
4. Spot-check findings against source references; also inspect live cases that
   could otherwise become false positives. Do not count all findings as deletions.
5. Keep private sample paths, source packets and machine-specific configs out of
   the distributable repository.

Initial local measurements and their exact limitations are recorded in
[acceptance results](acceptance.md). They are not guarantees for other machines.
