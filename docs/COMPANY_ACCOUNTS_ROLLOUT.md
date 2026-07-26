# Company Accounts Rollout And Rollback

Run every command from a release build that contains the company-account code. Keep `company.applications_enabled` and `company.iam_enabled` false until the final gate. Take a database backup and record the release SHA before starting.

## 1. Add Nullable Public IDs

1. Apply `backend/migrations/187_public_account_identifiers.sql` through the normal migration runner. This phase deliberately permits null public IDs.
2. Deploy dual-write code while these fields remain nullable.
3. Dry-run the legacy backfill:

   ```bash
   cd backend
   python3 tools/backfill_public_account_ids.py --dsn "$DATABASE_URL" --dry-run --batch-size 500
   ```

4. Run bounded batches. Record the printed cursor and resume with `--start-after` after interruption:

   ```bash
   python3 tools/backfill_public_account_ids.py --dsn "$DATABASE_URL" --batch-size 500 --start-after 0
   ```

   The script prints `last_cursor`. If interrupted, pass that exact value to `--start-after`; reruns also skip already populated rows.

5. Verify the result. `missing_ids`, `invalid_account_ids`, `invalid_root_ids`, `invalid_iam_ids`, and `duplicate_external_ids` must all be zero:

   ```bash
   psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f tools/verify_public_account_identifiers.sql
   ```

   Never log or export generated IDs together with email or password data.

## 2. Finalize IDs And Company Schema

1. Apply `backend/tools/finalize_public_account_identifiers.sql` only after verification. It intentionally fails on missing or malformed IDs:

   ```bash
   psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f tools/finalize_public_account_identifiers.sql
   ```

2. Apply `backend/migrations/188_company_accounts.sql`, `189_company_billing_snapshots.sql`, and `190_company_application_idempotency_scope.sql` through the migration runner.
3. Confirm the idempotent policy seed is exact:

   ```sql
   SELECT p.policy_key, p.policy_type, p.version, array_agg(a.action ORDER BY a.action)
   FROM managed_policies p
   JOIN managed_policy_actions a ON a.policy_id = p.id
   WHERE p.policy_key IN ('CompanyFinanceReadOnly', 'CompanySharedBalanceUse')
   GROUP BY p.policy_key, p.policy_type, p.version;
   ```

   The result must contain one version-1 `system` row per policy. The only actions are `organization.finance.balance.read` and `organization.balance.shared.use`, respectively.
4. Set only `company.public_ids_finalized=true`; keep both product switches false.

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
