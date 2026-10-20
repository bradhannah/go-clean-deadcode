# Initial portable-skill evaluation

Two scenario prompts were run in independent agent sessions, each with and without
the relevant skill. The prompts and assertions are in `skills/evals/evals.json`.
These were review/plan-only tasks; evaluation runs did not change source or install
integrations. Local answer artifacts are retained separately from distribution.

| Scenario | With skill | Without skill |
| --- | --- | --- |
| Mixed-purpose tests and incomplete platform/utility scope | 5/5 assertions satisfied | 5/5 assertions satisfied |
| Existing hook, timeout, and changed exception | 6/6 assertions satisfied | 6/6 assertions satisfied |

The analysis-skill answer preserved supported format regressions, recommended
splitting/migrating mixed assertions, and rejected a repository-wide clean claim
with Windows and a utility entry point missing. The workflow-skill answer preserved
existing checks, kept the AI loop outside the synchronous hook, rejected stale
proof after timeout, and required renewed exception review.

Baseline answers also passed these basic scenarios. This is a ceiling effect, not
evidence of measured improvement over an unassisted agent. Skill-assisted answers
provided tool-specific commands, schema/exit rules and bounded-evidence details;
no latency/token comparison was available. Human review and harder executed
integration scenarios remain valuable before broad rollout.

Static metadata/link/script checks and Go analyzer regression tests are separate
from these behavioral evaluations. The skill creator's HTML evaluation viewer was
not available within this session's permitted directories; the Markdown answers
and this summary provide the review artifacts instead.
