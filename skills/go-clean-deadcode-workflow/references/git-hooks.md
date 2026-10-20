# Git hook integration

Inspect `core.hooksPath`, `.git/hooks/pre-commit`, and any hook manager before
installation. Preserve existing checks and file content. Ask before changing a
global setting. Prefer integrating an explicit command into the existing mechanism.

`templates/pre-commit.sh` is a portable POSIX example. Configure a pinned tool and
the reviewed JSON config, then incorporate it into the existing hook with explicit
authorization. Do not silently overwrite a hook or install a global hook path.

The hook analyzes **working-directory source**, not the Git index. It may therefore
observe unstaged edits. Document that fact. Exact-index verification is a separate
adapter concern; do not stash, reset, stage or rewrite work to simulate it here.

The hook performs no AI invocation or automatic fixes. Exit 1 asks for review; exit
2 means analysis failed/incomplete. It should fail plainly when the binary/config
is missing rather than silently disabling itself. A complete run can still contain
explicitly excluded modules/configurations, so the repository owns the scope policy.

The optional cache directory must be outside the source. Cache hits revalidate
inputs and can still return findings. Do not gate on the existence of an old report.
Test a clean case, findings, invalid config and deadline cancellation in a sandbox.
On non-Unix hosts use an appropriate supervisor if descendant cleanup is required.
POSIX shell templates need adaptation for repositories using another hook runtime.
