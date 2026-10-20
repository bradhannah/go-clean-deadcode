---
name: go-clean-deadcode-workflow
description: Use when integrating go-clean-deadcode into an AI completion or commit workflow, a Git hook, or an existing hook manager. Preserve current validation, configure complete source scope, and require evidence-based resolution instead of automated test deletion.
compatibility: Requires go-clean-deadcode; Git only for the optional hook adapter. No particular agent harness is required.
---

# Integrating Go reachability review

The analyzer operates on source, not commits. This skill adapts it to repository
policy. Install/configure integrations only when authorized; an integration request
does not authorize unrelated cleanup, exceptions, commits or publication.

## Inspect before installing

1. Read project/agent instructions and existing build/test validation. Locate current
   AI completion hooks, Git hooks/hook managers and any configured hooks path.
2. Ask which route is wanted if ambiguous: AI workflow, Git hook, or both. Do not
   assume a Workshop, OpenCode, Claude, CI service or global configuration exists.
3. Pin an owner-selected analyzer build/revision. Check `version` and schema support.
   Version `0.1.0-dev` has no assumed published tag; do not invent a release URL.
4. Inventory executable packages, modules, platform/tag variants and API boundaries
   with `discover`. Review a schema-1 config; excluded scope is visible and justified.
5. Run initial analysis in report-only mode. Resolve findings or obtain narrow
   retention decisions before claiming strict adoption. Do not silently baseline all
   existing findings. Keep current quality checks intact.

Copy the entire skill directory, including references/templates. The companion
`go-clean-deadcode` skill provides deeper review guidance; this skill still contains
the integration acceptance rules needed when copied alone.

## AI workflow route

Read [AI workflow](references/ai-workflow.md). Add a pre-completion/pre-commit
instruction to the existing repository-owned workflow, with its authorization rules:

1. Run `analyze` against the final source using reviewed configuration.
2. Require `mode=analyze`, `complete=true`, zero unresolved findings and zero stale
   exceptions. All requested scopes must have completed. Exit 1 requires review;
   exit 2 is incomplete/error, never clean.
3. Review test-only implementations together with actual assertions, contracts,
   setup, callbacks and interface requirements. Changes need task authorization.
4. Make coherent authorized changes, run relevant existing validation and analyze
   again. At least one fresh/revalidated result must follow the final edit.
5. Stop on lack of progress, unsupported scope, or an unresolved product decision.
   Report the blocker instead of weakening policy. Commit only under existing
   authorization; this skill does not itself grant it.

The loop belongs to the agent workflow, not inside the analyzer or synchronous Git
hook. Analysis/report caching is source-based and does not prove a Git index match.

## Git-hook route

Read [Git hooks](references/git-hooks.md) before applying the bundled template.
Integrate into the existing hook chain rather than replacing it. The hook should
be deterministic and noninteractive: run analysis, emit evidence, return its exit
status. Never launch an AI conversation, edit source, stage files or add exceptions
from the hook. Keep reports/caches outside the source root.

The template checks the **working directory's source**, not staged changes. State
that scope plainly. If the repository requires exact-index acceptance, its adapter
must separately provide that capability or reject mismatches; the analyzer does
not implement it. Local hooks are bypassable and are not server enforcement.

## Retention policy and failure behavior

Exceptions use exact symbol/category, reviewed declaration fingerprint, reason,
approved reviewer and review date. They remain visible. Obtain approval under local
policy; an agent cannot invent it. Changed/expired/unmatched exceptions require
review. Root changes and exclusions are scope changes, not suppressions.

Missing binary, invalid config, failed loading, cancellation, unsupported schema,
stale results or incomplete analysis must not become success. A new failed run does
not make an older report current. Caches can avoid repeat work but cannot waive
scope or approval. Full descendant cleanup on non-Unix hosts currently needs an
external supervisor.

## Verify installation

Test the adapter in a sandbox with a clean fixture, a known finding, malformed
config and a timed-out run. Confirm all existing checks still execute, failures
propagate, and no source changes occur. Document actual command, scope, exceptions,
pinning and how to rerun it. Restart agents with startup-only skill/instruction
discovery after authorized installation. Shipping the skill itself does not install
or activate a hook anywhere.
