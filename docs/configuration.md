# Configuration and retention exceptions

The required configuration is strict JSON: unknown fields and unsupported schema
versions fail. See `examples/sample.json` and `schemas/config-v1.schema.json`.

## Modules

- `path`: root-relative module directory containing `go.mod`; cannot escape root.
- `patterns`: module-relative Go package patterns, normally `["./..."]`.
- `entry_points`: exact import paths or module-relative directories. Each has
  `role: "application" | "utility"` and optionally a list of configuration names.
- `tests`: defaults to true. False intentionally removes test evidence; the report
  retains `tests: "not_analyzed"` where applicable.
- `library`: explicitly permits no executable roots. It does **not** protect all
  exported symbols automatically; callers outside the project require contract
  review and narrow retention decisions.
- `test_support`: module-relative entries naming shared test support kept in
  normal files, because Go cannot import `_test.go` files across packages. An
  entry ending in `.go` names a file, one ending in `/...` a directory tree, and
  any other entry one package directory, for example `./test/...` or
  `./internal/state/fake_callbacks.go`. Declarations there are classified like
  test-file declarations: no finding while tests use them, `unused_test_helper`
  when they do not. Production code in such a file loses its `test_only`
  classification, so keep support in its own files.
- `api_roots`: `"exported"` adds the module's public API to the production roots,
  for libraries whose callers are other projects. In every non-`main` package
  without an `internal` element in its import path, the roots are package
  initialization, exported functions, exported methods declared on any type
  (importers reach unexported types through returned values and interfaces), and
  exported promoted methods of exported non-generic types. Unexported code and
  internal packages are still findings unless that API or a selected executable
  reaches them. Each scope records the mode. Test analysis is unchanged. A module
  with API roots needs no executable.
- `ignored_mains`: exact package/directory `path` and a nonempty `reason`.

Narrow patterns deliberately limit the analyzed package universe; use `./...` for
whole-module audits. A selected executable outside those patterns is an error.
Discovery inventories ordinary Go packages: the Go tool itself skips directories
such as `testdata`, dot/underscore-prefixed directories, and build-excluded files.
Explicit source patterns and build configurations are required where appropriate.

The analyzer also scans for nested `go.mod` files (excluding hidden, vendor and
node_modules directories). Each discovered nested module must appear in `modules`
or in `excluded_modules` with a reason. A root `./...` is not assumed to include it.
Exclusions appear in diagnostics. A complete scoped run does not claim acceptance
of excluded modules or configurations.

## Configurations

Each specifies unique `name`, `goos`, `goarch`, `tags` and `cgo_enabled`. Discovery
without a configuration uses the host architecture with CGO disabled. Configure
every platform and tag combination relevant to the project; single-host results
cannot establish cross-platform deadness. Cross-platform loading can fail when a
dependency needs native headers/tooling; that scope is incomplete.

Library contracts and plugin/native entry points need reviewed modeling; adding a
root or excluding a module changes scope and is not a harmless report suppression.

## Exceptions

Exceptions match an exact symbol and finding category. Example shape:

```json
{
  "id": "supported-legacy-api",
  "symbol": "example.com/project/api.LegacyDecode",
  "category": "test_only",
  "fingerprint": "<copy the 64-character fingerprint from the reviewed symbol>",
  "reason": "Supported compatibility API used by an external consumer.",
  "approved_by": "repository owner",
  "review_after": "2027-01-15"
}
```

The example placeholder is intentionally not a usable exception. Copy the actual
reviewed fingerprint. The UTC review date is inclusive. Missing targets, changed
declarations, expired dates and duplicate matching exceptions remain visible and
prevent exit 0. Exceptions do not modify graph roots, erase evidence or make code
appear runtime-reachable. `approved_by` records the declared reviewer; it is not a
cryptographic approval or authorization system.

Do not invent approvals, mass-accept a baseline, or refresh fingerprints merely
to pass. Review why the contract is retained and whether useful tests should be
migrated instead of discarded.

## Reflection reviews

Reachability keeps every exported method of a runtime type, because reflection
could call it. Only reflect's method lookups can: `reflect.Value.Method` and
`MethodByName`, and the same methods of `reflect.Type`. Each scope reports the
reachable lookups in `reflective_lookups`, per analysis, with `kind: method`,
`via` (the function calling the lookup) and `entry` (the first function outside
the standard library on a static call path to it, or module code that refers to
the lookup's package). An analysis with no lookup needs no review.

Rows with `kind: field` are production code outside the standard library that
reaches struct fields by reflection. Until each is reviewed, struct types
production can hand to reflection keep their fields, and the unused or
never-written ones are `reflection_retained` notes instead of findings.

A reflection review records an owner's finding that one entry never passes this
module's values to the lookup:

```json
{
  "id": "ebiten-builtin-shader-templates",
  "entry": "github.com/hajimehoshi/ebiten/v2/internal/builtinshader.ShaderSource",
  "reason": "Executes its shader templates with a struct of its own Filter and Address values.",
  "approved_by": "repository owner",
  "review_after": "2027-04-20"
}
```

When every method entry of an analysis has a matched, current review, code that
only reflection's exported-method rule keeps is unreachable in that analysis and
is classified normally. When every field entry of a scope is reviewed, its
fields are classified normally. A review names an entry, so it covers both kinds. Read the entry's source before approving: what it passes to
the lookup, and whether module values can reach it, for example through a
template's data or functions. An unmatched or expired review counts in
`stale_exceptions`. A dependency upgrade can add entries; an unreviewed entry
restores the conservative rule for its analysis. Do not invent approvals.

## File behavior

Target source is read-only. Configurations can be external. `--output`,
`--cpuprofile`, and `--cache-dir` must resolve outside the target; source-root
symlinks are resolved before enforcing this boundary. Atomic report replacement
avoids truncated files. No Git operations are performed by the analyzer.

Go dependency/build caches and the CLI's temporary stdout spool live outside the
source target. CGO can invoke native build tools when explicitly enabled. The
optional acceptance manifest helper uses read-only `git ls-files` to compare
sample source contents; it is not part of the analyzer or its cache identity.
