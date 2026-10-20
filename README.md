# go-clean-deadcode

Evidence-based Go function and test reachability analysis. Find implementations
reachable only from tests, inspect their callers, and review disconnected call
groups without modifying the analyzed project.

**v1 development preview.** The executable does not delete code, execute target
tests, invoke an AI, install hooks, or inspect Git commits. It loads Go source using
Go-project `x/tools` APIs, not runtime reflection or textual reference counts.

## Build

Requires Go 1.27 or newer compatible with the pinned `x/tools` version.

```sh
go build -trimpath -o bin/go-clean-deadcode ./cmd/go-clean-deadcode
go test ./...
go vet ./...
```

There is no published release/tag yet. Build this checkout; do not assume an
installation command referring to a nonexistent v1 tag works.

## Start with discovery

```sh
bin/go-clean-deadcode discover --root /path/to/project --format text
```

Discovery lists executable packages and nested modules. Classify every executable
as an application, utility, or explicitly excluded entry with a reason. Analyze
each module deliberately. See [configuration](docs/configuration.md) and
[the sample config](examples/sample.json).

```sh
bin/go-clean-deadcode analyze \
  --root /path/to/project \
  --config /path/to/project-config.json \
  --format json \
  --output /path/outside/project/report.json \
  --cache-dir /path/outside/project/analysis-cache
```

All configuration paths describing source resolve against `--root`, including
module paths. Entry-point directories and package patterns are module-relative.
The configuration file itself can be anywhere. The analyzer writes no files into
the target. Report, profile and cache destinations must be outside it.

Use `--format text` for factual findings with locations, direct references,
bounded path witnesses, known interface obligations, exceptions and recursive
groups. `--symbol exact.qualified.Name` focuses text findings without changing
analysis or acceptance; JSON always contains the complete result.

## What findings mean

| Category | Fact established within configured scope |
| --- | --- |
| `unreachable` | Non-test declaration not reachable from selected executables or included test executables. |
| `test_only` | Non-test declaration unreachable in the production analysis but reachable in the separate test analysis. |
| `unused_test_helper` | Declaration in a test file unreachable from the test harness. This also includes documentation-only examples, not just helpers. |
| `never_assigned` | Unexported struct field that reachable code reads but nothing ever writes, so every read sees the zero value. The finding lists the reads; the fix is a review of the code that depends on them, not a removal. |

Findings cover functions, methods, package-level types, constants and variables,
and struct fields. Some unused declarations are never findings because removing
them would change a meaning: exported fields, tagged or embedded fields, every
field of a struct whose layout matters (used with `encoding/binary`, `reflect` or
`unsafe`, converted to another struct type, or built with an unkeyed literal),
fields that are all that keep a struct from being zero-sized (pointer identity
depends on it),
and variables whose initializer calls something. An unused value of a constant
block or typed enum that is otherwise in use is reported as a note for review,
not a finding.

Reachability keeps exported methods of runtime types because reflection could
call them. The report lists which code performs reflective method lookup; once
an owner reviews each entry as never seeing the module's values (see
[configuration](docs/configuration.md#reflection-reviews)), code kept only by
that rule is classified like any other, and until then it is reported as
`reflection_retained` notes.

These are review categories, not deletion instructions. Test-only code can be
legitimate infrastructure or an external API. Two tests reaching the same code do
not necessarily test the same behavior. Empty private methods actually belonging
to implemented local interfaces are recorded as structural marker methods rather
than runtime cleanup findings.

**Exit codes:** `0` complete with no unresolved findings/stale exceptions;
`1` complete with unresolved findings or stale exceptions; `2` incomplete,
cancelled, configuration/load error, or output failure. Discovery's exit `0`
means inventory succeeded, **not** that the source is clean. Always check `mode`
and `complete` when consuming JSON.

## Documentation

- [Architecture and precision boundaries](docs/architecture.md)
- [Configuration, exceptions, and read-only behavior](docs/configuration.md)
- [JSON contract and report interpretation](docs/reports.md)
- [Performance and acceptance methodology](docs/performance.md)
- [Portable skills and integration](skills/README.md)
- [JSON schemas](schemas/README.md)

The separate skills explain human/AI review and optional Git-hook or AI-workflow
integration. They are not installed automatically. They contain no host-project
paths or dependencies.
