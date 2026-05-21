---
name: sub2api-sync-ldap
description: Refresh the LDAP fork of Wei-Shaw/sub2api onto the latest official upstream with a small, recoverable workflow, then validate deploy/upgrade_main.sh and publish the refreshed fork to GitHub. Use when the user asks to pull the latest official code, rebuild the LDAP edition, resolve LDAP overlay conflicts, or push the refreshed fork to GitHub.
metadata:
  short-description: Sync and publish the LDAP fork safely.
---

# Sub2API LDAP Sync

Use this skill only in the `sub2api` repository root.

## Trigger

Use this skill when the user wants any of the following:

- pull or sync the latest official `Wei-Shaw/sub2api`
- rebuild or refresh the LDAP edition
- re-apply the LDAP patch branch onto upstream
- run the LDAP fork validation gate before release
- backfill `feature/ldap-support` from `main`
- publish the refreshed LDAP fork to GitHub

## Default Path

Default completion path is manual and small-step. Do not start with the full orchestrator unless the user explicitly asks for the legacy all-in-one flow.

```bash
git fetch --all --prune
git merge upstream/main
go generate ./cmd/server
bash skills/sub2api-sync-ldap/scripts/contract-gate.sh
bash skills/sub2api-sync-ldap/scripts/deploy-sanity.sh
```

The legacy orchestrator is available only as a toolbox fallback:

```bash
bash skills/sub2api-sync-ldap/scripts/sync.sh --no-publish
```

## Workflow

1. Inspect the current worktree first. If there are uncommitted LDAP conflict repairs, understand and commit them before merging a new upstream.
2. Fetch `upstream/main`, merge it into `main`, and resolve conflicts as "official upstream plus LDAP-only additions".
3. Regenerate only what the code change requires. Prefer `go generate ./cmd/server` for Wire; run Ent generation only after schema changes.
4. Run the targeted release gate first: LDAP contract tests, backend server compile, frontend typecheck/tests when frontend changed, and deploy sanity.
5. Verify `deploy/upgrade_main.sh` still builds from `https://github.com/big-dimple/sub2api.git` branch `main` and does not depend on local git refs.
6. Publish `main` to `origin/main`. Backfill `feature/ldap-support` only if the user still needs that branch to mirror the release state.
7. If the merge conflicts are large, use the scripts in `scripts/` as helpers, not as the default driver.
8. If the overlay merge conflicts, resolve them on `main`, commit the merge, then continue with `scripts/resume-after-conflict.sh`.
5. If you need manual conflict guidance, read [references/conflict-recovery.md](references/conflict-recovery.md).
6. If you need policy or merge-priority guidance, read [references/policies.md](references/policies.md).
7. If you need script-by-script behavior, options, or when to call a script directly, read [references/script-map.md](references/script-map.md).
8. If preflight stops on reviewed upstream churn, rerun with `scripts/sync.sh --change-threshold <percent>` instead of patching environment variables.

## Operating Rules

- Default to short, inspectable commands; use the orchestrator only when the repository is clean and the user wants the legacy full automation.
- Treat LDAP behavior as the required customization; prefer upstream plus LDAP-only changes, not unrelated fork drift.
- Keep generated artifacts and branch history releasable at every stop point.
- Treat "完成最新版本 LDAP 化" as including `origin/main` being updated unless the user explicitly says local-only.
- Publish only after validation passes or the user explicitly accepts an incomplete release.

## Validation

`scripts/contract-gate.sh` is the standard release gate. It covers:

- backend LDAP contract tests
- backend server compile check
- frontend typecheck
- frontend Vitest suite

`scripts/deploy-sanity.sh` checks the LDAP deploy surface before release. Read [references/script-map.md](references/script-map.md) only if you need the exact coverage.
