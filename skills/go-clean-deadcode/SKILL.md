---
name: go-clean-deadcode
description: Use when auditing Go dead functions, code used only by tests, obsolete test candidates, recursive call groups, or interpreting go-clean-deadcode JSON reports. Establish complete scope and review evidence before proposing code or test removal.
compatibility: Requires go-clean-deadcode and a compatible Go toolchain; bundled report reader uses Python 3.9+.
---

# Go reachability review

Use the deterministic analyzer to establish facts, then inspect contracts and tests
to decide what those facts mean. The analyzer never proves a feature obsolete or a
test redundant. Respect the user's requested review/edit scope and local rules.

## 1. Establish the scope

1. Read the repository's instructions and test/build documentation. Identify modules,
   executable tools, platforms/tags, external APIs, native/plugin boundaries, and
   generated code. Preserve unrelated changes.
2. Check `go-clean-deadcode version`. This skill targets report/config schema 1 and
   the v1 development CLI. There is no assumed public release tag. Use an owner-
   selected built binary or pinned revision; do not silently install `@latest`.
3. Run `go-clean-deadcode discover --root ROOT --format json`. This is inventory,
   not dead-code acceptance. With existing config, pass `--config CONFIG` to inspect
   its build matrix. Inventory nested modules explicitly.
4. Review/configure every executable and applicable platform/tag combination.
   `./...` does not include nested modules. Missing scopes, excluded APIs, and
   native callbacks cannot be waved away as dead code.

See [scope and commands](references/commands.md) for the actual config shape and
exit codes. Changes to roots/exclusions require scope review because they change
what the analyzer can conclude.

## 2. Run a fresh analysis

```sh
go-clean-deadcode analyze --root ROOT --config CONFIG --format json \
  --output OUTSIDE_ROOT/report.json --cache-dir OUTSIDE_ROOT/cache
```

Keep reports/caches outside the target; no source modifications are required.
The analyzer loads/type-checks tests but does not run them. It uses readonly module
mode, disables ambient workspaces/GOFLAGS and requires compatible provisioned
dependencies. If loading fails, resolve the dependency/scope issue with permission;
do not change source merely to manufacture an analyzable result.

Exit 0 means complete with no unresolved findings/stale exceptions; 1 means review
findings remain; 2 means incomplete/error. Check JSON `mode`, `complete`, `scopes`
and `diagnostics`, not only the process status. Cache reuse does not imply clean.

## 3. Inspect facts, then test intent

Use the bundled reader, or equivalent JSON queries:

```sh
python3 SKILL_DIR/scripts/read_report.py OUTSIDE_ROOT/report.json
python3 SKILL_DIR/scripts/read_report.py OUTSIDE_ROOT/report.json --symbol EXACT_ID
python3 SKILL_DIR/scripts/read_report.py OUTSIDE_ROOT/report.json --test EXACT_TEST_ID
```

For each coherent candidate group:

- Read declarations, source references, path witnesses, recursive members, and
  compile-time obligations. A dead caller's reference is not application use.
- Distinguish direct static paths, possible dynamic paths, and unexplained retention.
  Root lists are bounded witness examples. An absent path is not proof of deadness.
- Read associated tests' assertions, inputs, setup and historical regression purpose.
  Determine whether they protect a supported contract, an abandoned implementation,
  or shared infrastructure. If mixed, preserve/migrate useful assertions.
- Check external API obligations and build configurations. Exported-but-unused in
  this repository does not establish no downstream consumers.
- Treat marker methods as compile-time structure. Other uncalled methods may also
  be required by interfaces, even without an explicit obligation record.

[Reviewing tests](references/reviewing-tests.md) covers the subtle cases.

## 4. Resolve only authorized work

For review-only requests, report evidence and decisions needed; edit nothing.
For authorized cleanup, make small coherent removals or migrations, then run the
repository's required validation and fresh analysis. Deleting tests only to reduce
the finding count is not a valid resolution. Call-graph overlap does not establish
duplicate assertions. Inspect code before changing a whole recursive group.

Propose a narrow exception only for an actual retained contract. Record the exact
symbol, category, reviewed fingerprint, reason, reviewer and review date after
approval under local policy. Never invent approval or auto-refresh a changed or
expired exception. See [exceptions](references/exceptions.md).

Repeat analysis after changes, including after removing a test/root. Stop and
report blocked decisions when there is no progress or scope is incomplete. Do not
weaken analysis or broaden exceptions to force a pass. This skill grants no Git
commit/push authorization; workflow integration is a separate skill.

## Handoff

State analyzed configurations/roots, excluded scopes, completion, counts by
category, accepted/stale exceptions, representative evidence, edits made, and
validation actually run. Say "zero unresolved findings in this scope," not
"proved no dead code anywhere." Report timeout or unsupported scope as incomplete.
