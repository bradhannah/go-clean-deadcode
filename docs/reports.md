# Report contract v1

Text and JSON are renderings of the same report. JSON stdout contains one complete
document; progress goes to stderr with `--verbose`. Cancellation discards a worker's
partial stdout and emits an incomplete report. Output I/O failures use exit 2 and
stderr when the requested destination cannot be written.

Published schemas are in `schemas/`; `go-clean-deadcode schema report` and
`go-clean-deadcode schema config` emit them. Schema tests prevent wire-type drift.
Structural validation is not semantic acceptance. A consumer should:

1. Require a supported schema and tool version.
2. Require `mode == "analyze"` and `complete == true` for analysis acceptance.
3. Check all scope completion and declared exclusions against repository policy.
4. Require zero unresolved findings and stale exceptions for a clean result.
5. Inspect diagnostics. Report the analyzed scope alongside every conclusion.

## Records

- `scopes`: module/build selections, discovered/classified executable packages,
  test inclusion and nested modules.
- `symbols`: qualified identities, declaration locations, normalized declaration
  fingerprint, generated/test-file flags, test-harness registration, per-scope
  production/test states, explanatory paths and known compile-time obligations.
- `references`: type-resolved direct source references. Their enclosing caller
  may be dead. `from` is absent for package-level expressions/initialization.
- `edges`: inventoried caller/callee relationships. `context=source` includes
  direct calls in dead bodies. Production/test dynamic edges are possible calls.
- `groups`: recursive strongly connected components. Member identifiers refer
  to source symbols. These are dependency groups, not deletion transactions.
- `findings`: stable category/symbol identity, analyzed scopes, unresolved or
  accepted disposition, and any matching exception ID.
- `exceptions`: reasons, declared approval, review dates and match status.
- `diagnostics`: scope/configuration/load errors and explicit exclusions.
- `metrics`: this invocation's measured phase/total times and cache-hit status.
- `limitations`: precision boundaries that consumers must preserve.

Source locations are root-relative. Dependency locations, when present in evidence,
use `<dependency>/basename`; dependency qualified names remain in witness paths.
`production_roots` and `test_roots` describe selected witnesses, not every consumer.
`lexical_test_reference` witnesses identify enclosing registered tests for closures;
the first parent/closure step is source ownership rather than a direct call.
`registered_test` means referenced by a generated test harness, not observed running.
Benchmarks/fuzz targets need their respective execution modes. `TestMain` is suite
infrastructure. An example without expected output can be unreachable documentation.

## Useful queries

```sh
jq '{complete, summary, diagnostics}' report.json
jq '.findings[] | select(.category == "test_only")' report.json
jq '.symbols[] | select(.id == "example.com/project/pkg.Legacy")' report.json
jq '.references[] | select(.symbol == "example.com/project/pkg.Legacy")' report.json
jq '.groups[] | select(.members | index("example.com/project/pkg.Legacy"))' report.json
```

Use symbols' witnesses to locate candidate tests, then inspect their assertions and
fixtures. JSON can be large for large projects because the report retains source
inventory and relationships; default output does not silently truncate findings.

## Compatibility

Schema version 1 is the current development contract. Breaking field meaning,
removal, or type changes require a schema-version change once v1 is released.
Consumers should tolerate additional fields within a supported major version.
The checked-in schema describes the exact current producer and intentionally
disallows additional properties for producer regression checks. Tool version
`0.1.0-dev` does not imply a published release or stable cross-version cache format.
