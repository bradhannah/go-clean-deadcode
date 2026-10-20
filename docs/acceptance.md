# Initial v1 acceptance

Recorded on 2026-10-20 using Go 1.27.1 on macOS arm64. These are source-analysis
results, not target-game test execution or approved deletion decisions.

## Deterministic validation

`make validate` runs Go regression tests, `go vet`, schema-drift checks and portable
skill metadata/link/script checks. Fixtures cover unreachable chains/cycles,
10,000-node recursive groups, interface dispatch/requirements, marker-method
classification, callbacks, generics, test-only implementations, subtest lexical
association, benchmarks/fuzz/examples, multiple roots, Windows/Linux file selection,
omitted roots/modules, invalid loading, exceptions, cache invalidation/corruption,
source preservation and machine-readable timeout failure.

The executable also cross-builds for Linux amd64 and Windows amd64. These are build
checks, not native execution acceptance. Actual CLI execution was on macOS arm64.

## Read-only real-project runs

Both targets were analyzed with external configs/reports, CGO disabled, no extra
build tags, and all discovered host executable entry points selected. The current
application's nested consumer-fixture module was explicitly excluded because it
requires an externally constructed temporary-module substitution. Host results do
not claim complete acceptance of other platform/tag combinations or that fixture.

| Observation | Current application | Historical application |
| --- | ---: | ---: |
| Root-module packages | 79 | 79 |
| Executable entry points | 15 | 14 |
| Source function/method symbols | 20,430 | 19,796 |
| Unreachable non-test declarations | 49 | 59 |
| Non-test declarations reachable only in tests | 258 | 270 |
| Unreachable test-file declarations | 9 | 8 |
| Structural marker methods, reported separately | 25 | 25 |
| Approved exceptions | 0 | 0 |

Representative source checks confirmed a UI adapter's callers occur in tests,
an unused character-selection fixture helper has no source callers, and the old
A* initializer's name remains in documentation while runtime callers use another
initializer. These support the reported classifications, not blanket removal.
Some findings are intentional helpers or interface-required methods and need
review. Not every finding has received semantic review.

Before/after manifests compared file contents/modes and symlink targets for **2,706
current** and **2,918 historical** tracked/nonignored source paths: unchanged.
No cleanup, hook installation or configuration edit was performed in either target.

## Performance observations

One instrumented run used `GOMAXPROCS=4`, `GOMEMLIMIT=2GiB`, provisioned Go caches,
the full JSON report, and the content-validated report cache:

| Operation | Whole-command elapsed |
| --- | ---: |
| Current application, fresh analysis | 20.6 seconds |
| Same current inputs, cached report | 2.8 seconds |
| Historical application, fresh analysis | 17.4 seconds |

These are observations, not p95 guarantees. Fresh means no matching analyzer
report, not an empty Go dependency/build cache. Full-run maximum RSS was roughly
3–3.6 GiB; cached full-report handling was about 2 GiB. Go's memory limit is soft
and does not cap aggregate process RSS. Large JSON graph inventories carry a real
memory/output cost. Cache-hit performance does not yet meet a strict sub-two-second
whole-command target on this sample.

Profiling the first implementation identified repeated call-path name construction
and sorting as allocation/GC hotspots. Cached graph identities and shared traversal
reduced an observed 64-second run to the approximately 20-second range without
dropping findings. Subtest witnesses were also improved to distinguish lexical
test ownership from conservative cross-test harness dispatch.

## Skills

Both portable skills passed their initial review/plan scenarios; baseline agents
also passed. See [evaluation details](skill-evaluation.md). No skills were installed
into the sample repositories or any host orchestration kit.
