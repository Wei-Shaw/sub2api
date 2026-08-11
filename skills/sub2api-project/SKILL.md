---
name: sub2api-project
description: Work on the Sub2API repository safely and reliably. Use when changing Sub2API application code, docs, tests, build scripts, migrations, frontend Vue code, backend Go code, OpenSpec changes, or when preparing a stable git commit for this project.
---

# Sub2API Project

This skill is the default project workflow for repository changes. Keep edits small, verify the touched surface, and commit only the intended files.

## Project Map

- `backend/`: Go backend, Gin handlers, services, Ent schema and migrations.
- `frontend/`: Vue 3 frontend, Vite, Pinia, TailwindCSS, Vitest.
- `openspec/`: change proposals and specs for larger behavioral work.
- `deploy/`: deployment scripts and service assets.
- `skills/`: project-local OpenCode skills and supporting references.

## Change Workflow

1. Inspect the existing code path before editing. Prefer `rg`, targeted file reads, and nearby tests.
2. Check worktree state with `git status --short` and avoid unrelated user changes.
3. Make the smallest correct edit that follows local patterns.
4. Add or update focused tests when behavior changes.
5. Run the narrowest useful verification first, then broader checks when the touched surface justifies it.
6. Review `git diff` before committing.

## Common Verification

Run commands from the repository root unless noted.

```bash
make test-backend
make test-frontend
make test
pnpm --dir frontend run lint:check
pnpm --dir frontend run typecheck
pnpm --dir frontend exec vitest run <spec-file>
```

Backend-only quick checks:

```bash
cd backend && go test ./...
cd backend && go test ./internal/service -run TestName
```

Frontend-only quick checks:

```bash
pnpm --dir frontend exec vitest run src/path/to/file.spec.ts
pnpm --dir frontend run lint:check
pnpm --dir frontend run typecheck
```

## Git Commit Workflow

Use this workflow when the user asks for a stable, reliable commit.

1. Inspect `git status --short`, `git diff`, and `git log --oneline -10`.
2. Identify files changed by this task and files that were already dirty.
3. Run relevant verification and record any skipped checks with the reason.
4. Stage only intended files, never broad-stage with unrelated dirty files present.
5. Commit with a concise imperative message, for example `Add Sub2API project skill`.
6. Confirm the resulting commit with `git status --short` and `git log --oneline -1`.

## Safety Notes

- Do not revert or overwrite unrelated dirty files.
- Do not print secrets from `.env`, exports, account data, tokens, or admin payloads.
- Treat account exports and admin API responses as sensitive unless the user says otherwise.
- For destructive data operations, perform a read-only check first and clearly identify the target IDs.
- Prefer project Makefile and package scripts over one-off command variants.
