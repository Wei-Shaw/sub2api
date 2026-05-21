# Script Map

Read this file when you need exact script behavior, non-default entry points, or command variants.

## Primary Entry Point

### `scripts/sync.sh`

Default orchestrator. Runs, in order:

1. `upstream-preflight.sh`
2. `overlay-apply.sh`
3. `generated-repair.sh`
4. `contract-gate.sh`
5. `deploy-sanity.sh` unless skipped
6. `backfill-support.sh` unless disabled
7. `publish-release.sh` by default unless `--no-publish` is supplied

## Direct-Use Scripts

### `scripts/upstream-preflight.sh`

Use when you only need to verify that upstream drift is still small enough for the LDAP overlay flow.

- auto-detects patch branch: `feature/ldap-patch` then `feature/ldap-support`
- fails fast if upstream changed too much around the LDAP touch points
- accepts `--change-threshold <percent>` when you have reviewed upstream churn and want a higher guardrail than the default 40%

### `scripts/overlay-apply.sh`

Use when you need just the merge/overlay step.

- creates local `main` from upstream
- overlays the LDAP patch branch
- exits early if origin already matches the current upstream + patch state

### `scripts/resume-after-conflict.sh`

Use after you have resolved merge conflicts on the release branch and created the merge commit.

- runs generated repair, contract gate, and deploy sanity
- commits regenerated artifacts when needed
- backfills `feature/ldap-support`
- publishes by default unless `--no-publish` is supplied

### `scripts/generated-repair.sh`

Use after successful overlay or after manual conflict resolution.

- syncs embedded version
- runs Ent generation
- runs Wire generation
- repairs missing `go.sum` entries when possible

### `scripts/contract-gate.sh`

Standard LDAP release gate:

- backend LDAP contract tests
- backend compile check
- frontend typecheck
- frontend Vitest

Set `LDAP_SYNC_FULL_TESTS=1` or use `sync.sh --full-test` to run broader backend tests.

### `scripts/deploy-sanity.sh`

Checks the LDAP deploy surface:

- compose healthcheck and data-dir behavior
- Dockerfile healthcheck expectations
- setup fallback behavior
- single-script upgrade flow
- deploy docs consistency

### `scripts/backfill-support.sh`

Moves `feature/ldap-support` up to the release branch.

- fast-forwards when possible
- merges release into support only if the branches diverged

### `scripts/publish-release.sh`

Push helper for `main` plus one additional branch.

- pushes only when the remote differs
- uses fast-forward push when possible
- falls back to `--force-with-lease` only when branch history diverged

## Common Variants

```bash
# Default full flow: sync, validate, backfill, publish
bash skills/sub2api-sync-ldap/scripts/sync.sh

# Keep the sync local only
bash skills/sub2api-sync-ldap/scripts/sync.sh --no-publish

# Resume after a manual conflict resolution commit
bash skills/sub2api-sync-ldap/scripts/resume-after-conflict.sh

# Specify the patch branch explicitly
bash skills/sub2api-sync-ldap/scripts/sync.sh --patch-branch feature/ldap-support

# Allow a higher reviewed preflight drift threshold
bash skills/sub2api-sync-ldap/scripts/sync.sh --change-threshold 40

# Specify a different backfill target
bash skills/sub2api-sync-ldap/scripts/sync.sh --backfill-branch feature/ldap-support

# Run slower, broader backend tests in the gate
bash skills/sub2api-sync-ldap/scripts/sync.sh --full-test

# Skip deploy sanity only when the user explicitly accepts the risk
bash skills/sub2api-sync-ldap/scripts/sync.sh --skip-deploy-sanity

# Disable backfill only when the user explicitly asks
bash skills/sub2api-sync-ldap/scripts/sync.sh --no-backfill
```
