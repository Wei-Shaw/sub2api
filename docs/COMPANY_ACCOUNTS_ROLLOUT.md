# Company Accounts Rollout And Rollback

Run every command from a release build that contains the company-account code. Keep `company.applications_enabled` and `company.iam_enabled` false until the final gate. Take a database backup and record the release SHA before starting.

## 1. Prepare And Inspect

1. Stop all application instances except the one maintenance/canary instance that will run the migration. Keep `company.applications_enabled`, `company.iam_enabled`, `company.public_ids_finalized`, and `company.billing_integration_enabled` false.
2. Take a PostgreSQL backup and record the release SHA:

   ```bash
   pg_dump --format=custom --file=/secure/backup/sub2api-before-account-id-migration.dump "$DATABASE_URL"
   ```

3. Check for ambiguous duplicate root IDs. This must return zero rows; do not continue if it returns any row:

   ```bash
   psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -c "
   SELECT account_id, COUNT(*) AS root_count
   FROM users
   WHERE identity_type = 'root' AND account_id IS NOT NULL
   GROUP BY account_id HAVING COUNT(*) > 1;"
   ```

4. If migration `187_public_account_identifiers.sql` is already recorded in `schema_migrations`, run the legacy backfill before starting the new release. It is optional for correctness because migration 231 also handles remaining shared/missing rows, but it reduces the final migration transaction for large tables:

   ```bash
   cd backend
   python3 tools/backfill_public_account_ids.py --dsn "$DATABASE_URL" --dry-run --batch-size 500
   ```

5. Run bounded batches. Re-running from `--start-after 0` is safe because already unique IDs are skipped:

   ```bash
   cd backend
   python3 tools/backfill_public_account_ids.py --dsn "$DATABASE_URL" --batch-size 500 --start-after 0
   ```

   The script prints `last_cursor` when it completes. If a batch run is interrupted, resume from a recorded cursor when available; restarting at `0` is also safe.

6. Verify the result. Missing, malformed, or duplicate account IDs must be zero:

   ```bash
   psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f tools/verify_public_account_identifiers.sql
   ```

   Never log or export generated IDs together with email or password data.

## 2. Apply The Release Migration

1. Deploy the release containing `231_user_account_ids.sql` and start exactly one instance with a sufficiently long migration timeout (for example `SETUP_MIGRATION_TIMEOUT_SECONDS=1800`). `InitEnt` runs the normal migration runner; it applies pending migrations in filename order and records them in `schema_migrations`.
2. Migration 231 preserves unique IDs, gives shared IAM/missing rows fresh 16-digit IDs, aborts on duplicate root ownership, removes `external_user_id`, makes `account_id` non-null, and installs the global unique index. It is transactional; if it fails, restore/fix the reported data and rerun rather than editing rows manually.
3. For a DBA-controlled staged rollout, the standalone finalizer remains available, but only run it after verification and only when no pending migration runner will concurrently apply 231:

   ```bash
   psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f tools/finalize_public_account_identifiers.sql
   ```

4. Confirm the idempotent policy seed is exact:

   ```sql
   SELECT p.policy_key, p.policy_type, p.version, array_agg(a.action ORDER BY a.action)
   FROM managed_policies p
   JOIN managed_policy_actions a ON a.policy_id = p.id
   WHERE p.policy_key IN ('CompanyFinanceReadOnly', 'CompanySharedBalanceUse')
   GROUP BY p.policy_key, p.policy_type, p.version;
   ```

   The result must contain one version-1 `system` row per policy. The only actions are `organization.finance.balance.read` and `organization.balance.shared.use`, respectively.
5. Set only `company.public_ids_finalized=true`; keep both product switches false until the billing and reconciliation gates pass.

## 3. Billing And Reconciliation Gate

1. Review [COMPANY_BILLING_PATH_MATRIX.md](COMPANY_BILLING_PATH_MATRIX.md) against a fresh `rg` inventory of balance deductions, holds, captures, releases, refunds, and cache invalidations.
2. Run backend unit and integration suites, frontend tests/lint, vulnerability checks, and the pnpm audit exception check.
3. As a system administrator, call `GET /api/v1/admin/organizations/operations`. The scheduled monitor runs the same checks every `company.reconcile_interval_seconds`. These reconciliation values must be zero:
   - pending reservation mismatch and frozen shortfall;
   - upgrade settlement mismatch;
   - owner cardinality and member limit violations;
   - transfer conservation violations;
   - missing usage, async-media, or batch-image payer snapshots.
   Also confirm that ID collision retries, payer-resolution failures, outbox failures/lag, and authorization database fallbacks are at their expected zero baseline. Review any IAM financial-operation denial rather than treating it as a billing failure.
4. Complete acceptance for reserve/approve/reject/withdraw mail, concurrent twentieth-member creation, IAM login/recovery, immediate policy revocation, allocated/shared billing without fallback, async refund to original payer, finance redaction, usage scoping, and suspension. Archive the test evidence with the release.

## 4. Staged Enablement

1. Keep the checked-in defaults false. Set `company.public_ids_finalized=true` and `company.billing_integration_enabled=true` only in the canary deployment configuration, then restart one canary instance.
2. Enable `company.applications_enabled=true` only on the canary. Keep IAM creation off until at least one upgrade approval and notification cycle succeeds.
3. Enable `company.iam_enabled=true` only on the canary, then expand traffic in explicit stages (for example 5%, 25%, 50%, 100%) after a clean monitoring interval at each stage.
4. Alert on non-zero ID collision retry growth, payer-resolution failures, IAM financial-operation denials above the expected baseline, authorization database fallbacks, notification outbox failures/lag, review queue age, and any reconciliation violation.

## Rollback

- Before any application exists, turn `company.applications_enabled` and `company.iam_enabled` off, leave the readiness attestations recorded, and roll back the binary. Additive tables and nullable snapshots remain intact.
- With pending applications, disable new submissions first and release reservations only through the normal idempotent withdraw/reject command. Do not edit balances or delete ledger/outbox rows manually.
- After approval or IAM creation, old binaries are not compatible with nullable IAM email. Disable IAM login and organization mutations, retain all identities/history, and deploy a forward fix.
- Never reverse an approved upgrade fee automatically. Organization suspension is the reversible access-control action.
- During rollback, continue the notification outbox worker and reconciliation monitor until pending decisions and refunds settle. Never delete audit, ledger, usage, membership, or outbox history.
