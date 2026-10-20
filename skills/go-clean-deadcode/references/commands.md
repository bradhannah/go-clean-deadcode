# Commands and scope (schema 1)

```json
{
  "schema_version": 1,
  "modules": [{
    "path": ".",
    "patterns": ["./..."],
    "entry_points": [
      {"package": "./cmd/server", "role": "application"},
      {"package": "./cmd/migrate", "role": "utility"}
    ],
    "tests": true
  }],
  "configurations": [
    {"name": "linux", "goos": "linux", "goarch": "amd64", "tags": [], "cgo_enabled": false}
  ]
}
```

Modules resolve against `--root`; entry directories/patterns against their module.
Use exact import paths or relative executable directories. Entry points can have
`configurations: ["linux"]`. Libraries explicitly set `library: true` when there
is no executable. External API reachability is not inferred from tests. A library
whose importers live elsewhere sets `api_roots: "exported"`, so its exported
functions and methods outside `internal` packages count as reachable; unexported
and internal code still needs a path from that API. Shared fakes, builders and
fixture packages in normal files belong in `test_support`, so they count as test
helpers rather than code only tests reach.

Nested modules require another module record or `excluded_modules` entries with
`path` and `reason`. `ignored_mains` inside a module similarly needs exact path and
reason. Neither is a harmless way to hide findings. Narrow package patterns omit
source outside the selected universe; whole-module audits normally use `./...`.

Commands:

- `version`: tool identity.
- `discover --root ROOT [--config CONFIG] --format text|json`: executable inventory.
- `analyze --root ROOT --config CONFIG --format text|json`: full scoped analysis.
- `schema report` / `schema config`: machine-readable schemas.
- Analysis options: `--output FILE`, `--cache-dir DIR`, `--timeout 5m`, `--verbose`,
  `--cpuprofile FILE`, `--symbol EXACT_SYMBOL_OR_FINDING_ID` (text focus only).

Output/cache/profile locations must be outside the analyzed source tree. JSON is
one document on stdout unless `--output` is used. Progress goes to stderr.

Exit 0: complete/no unresolved findings or stale exceptions. Exit 1: complete with
findings/stale exceptions. Exit 2: incomplete/error/cancellation. Discovery exit 0
is not analysis acceptance. Invalid format/CLI/output failures may only produce a
stderr diagnostic; do not require an existing old report as evidence of success.

Analysis uses separate production/test SSA models with combined roots within each.
Test paths can be conservative across test packages. Maximum three test witnesses
and 2,000 visited nodes per search; witness-root lists are not exhaustive ownership.
Known obligations cover only some interface-assignment forms. Native calls,
assembly/linkname and external consumers need review. Full descendant cleanup on
non-Unix hosts currently needs an external process supervisor.
