# AI workflow integration

Use an existing pre-completion/pre-commit instruction if available. Add a bounded
source-analysis step; do not create a competing workflow engine.

Suggested instruction, adapting paths to the repository:

> Before reporting the approved implementation ready for its normal commit step,
> run go-clean-deadcode with the reviewed config against the final working source.
> Read JSON completion, scope, diagnostics, findings and exceptions. Resolve
> authorized findings through code/test review and rerun required checks. Obtain
> an approved, narrow retention exception for supported code that is intentionally
> retained. Stop on incomplete analysis or unresolved decisions. Do not commit
> until there are zero unresolved blocking findings/stale exceptions within the
> required scope and the repository's other checks pass. Existing commit permission
> remains required. Do not alter analyzer scope or exception approval merely to pass.

Concrete invocation:

```sh
go-clean-deadcode analyze --root PROJECT_ROOT --config CONFIG_FILE --format json \
  --output OUTSIDE_PROJECT/report.json --cache-dir OUTSIDE_PROJECT/cache
```

Process status 0 is acceptance only for `analyze`, not `discover`. Status 1 is a
completed report needing review. Status 2 is incomplete/failure. A report reader
returning 0 merely means it read JSON; it does not replace analyzer acceptance.

Review units are coherent source/test groups. Test-only code may be infrastructure
or external API. Read assertions/inputs and preserve supported regressions. Same
call graph does not imply equivalent tests. Native/assembly calls and interface
requirements need review. Bound the loop by progress: if two consecutive rounds
do not reduce or explain unresolved findings, report the remaining decisions.

Avoid automatically running all expensive validation for every small edit. Follow
the project's focused checks during work, then its full required checks on the
final candidate. Analyzer checks supplement those checks; they do not replace them.

The tool knows no Git history/index/commit identity. A future adapter can impose
additional requirements, but the source audit must not claim them implicitly.
