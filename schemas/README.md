# JSON schemas

`config-v1.schema.json` and `report-v1.schema.json` describe the wire structure.
Configuration also undergoes semantic checks (entry-point roles, uniqueness,
paths, dates and scope coverage). Schema validation alone is not acceptance.

Regenerate with `go run ./scripts/schema-gen`; tests reject schema drift.
Consumers must check `schema_version`, `mode`, `complete`, findings, and exception
status. A successful discovery report is not a successful reachability analysis.
Unknown schema versions must be rejected, not silently interpreted as v1.
