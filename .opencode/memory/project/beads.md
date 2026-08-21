# Beads Work Ledger Memory

OpenCode must treat `.beads/` as the canonical task-level work ledger for this repository.

## Startup read pass
1. Read `.beads/AGENTS.md` when present.
2. Inspect current Beads state with `br ready`, `br list --status open`, and `br show <id>` when a bead id is relevant.
3. Read relevant artifacts under `.beads/artifacts/<bead-id>/` before changing code.

## Write-back rule
For non-trivial work, write task history under `.beads/artifacts/<bead-id>/`:
- `research.md` for findings and source-reading notes.
- `progress.md` or `progress.txt` for chronological implementation progress.
- `iterations.md` for failed attempts, pivots, and feedback loops.
- `verification.md` for commands, exit statuses, evidence, and unresolved failures.
- `handoff.md` for current state, blockers, risks, and next concrete action.

Do not rely on chat history as the durable record. Do not store secrets; redact sensitive values as `[REDACTED]`.
