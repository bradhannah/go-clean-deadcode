# go-clean-deadcode

Standalone Go source analyzer. Read README.md and docs/architecture.md before
changing analysis behavior. No Git, AI-provider, or host-project dependency belongs
in the analysis engine. Targets are read-only: never rewrite their source, module
files, configuration, or tests. Reports and caches belong outside the target.

Use dedicated branches. Run `go test ./...`, `go vet ./...`, and the fixture
acceptance tests after changes. Never claim static reachability proves execution,
test redundancy, or safety of deletion. Incomplete analysis cannot pass a gate.
Keep schema, examples, CLI help, and portable skills aligned with behavior.

Do not commit, push, publish releases, or change visibility without authorization.
