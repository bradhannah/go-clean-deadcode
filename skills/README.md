# Portable agent skills

Copy either entire skill directory, including its references/scripts/templates, to
the skill location supported by your agent. Common examples:

- OpenCode: `.opencode/skills/<name>/SKILL.md`
- Claude Code: `.claude/skills/<name>/SKILL.md`
- Other agents: their supported skill directory or an explicitly linked workflow
  instruction. Do not assume every harness discovers the same location.

Ship both skills for analysis plus integration. Neither requires this repository's
directory layout after copying. A built `go-clean-deadcode` binary is required;
install/pin it separately. The templates are examples, not auto-installed hooks.
Preserve existing repository instructions and validation. Restart agents whose
skill discovery is startup-only after installation; shipping these files alone
does not alter a running session.

| Skill | Purpose |
| --- | --- |
| `go-clean-deadcode` | Scope, run, interpret and review Go reachability findings. |
| `go-clean-deadcode-workflow` | Integrate into an AI workflow or existing Git hook. |

`evals/evals.json` contains portable behavioral evaluation scenarios. Static checks
validate frontmatter, bundled links, JSON examples and script syntax. Agent scenario
evaluations are separate from those static checks and deterministic Go regressions.

See [initial evaluation results](../docs/skill-evaluation.md) for the observed scope
and limits of the first two with-skill/baseline scenarios.
