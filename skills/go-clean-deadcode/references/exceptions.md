# Retention decisions

An exception records an actual reason to retain a particular finding, not a way to
obtain a desired exit status. Match exact symbol and category (`unreachable`,
`test_only`, `unused_test_helper` or `never_assigned`). Required fields:

- `id`: unique human-readable record ID.
- `symbol`: exact report symbol ID.
- `category`: the reviewed finding category.
- `fingerprint`: the symbol's reviewed 64-character declaration fingerprint.
- `reason`: retained contract and justification.
- `approved_by`: reviewer authorized under the target repository's policy.
- `review_after`: inclusive UTC date, `YYYY-MM-DD`.

Approved external API compatibility, known native entry points, and legitimate
test-only infrastructure can warrant exceptions. Future hypothetical use normally
does not justify retention by itself; ask the owner when that decision is material.

Changed declarations, expiration, unmatched symbols and duplicate matching records
are stale findings. Do not refresh them mechanically. Fingerprints do not cover all
transitive behavior, so review again when the retained contract changes even if the
tool's fingerprint has not changed. `approved_by` is a record, not a verified
signature. The repository's human/agent authorization policy remains authoritative.

Exceptions retain the original evidence. They do not create roots or relabel code
as runtime-used. No broad package-wide suppression or automatic baseline acceptance
is part of this skill.
