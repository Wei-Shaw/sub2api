# Company IAM billing path matrix

This matrix is the rollout inventory for company-account billing. Company
applications and IAM creation must remain disabled if a billable path is added
without updating this file and its tests.

## Required billing contract

Every consumption path resolves a `BillingContext` containing the consumer,
organization, payer, balance source, and authorization generation. A personal
or root user pays itself. An IAM user with `CompanySharedBalanceUse` pays from
the organization root; otherwise it pays from its allocated balance. There is
no fallback when the selected payer has insufficient funds.

Once a deduction or hold starts, capture, release, refund, reconciliation,
balance-cache invalidation, and low-balance notification use the persisted
payer snapshot. They do not resolve current policy or lifecycle state again.

## Inventory

| Path | Resolve and persist | Settlement and recovery | Cache/notification target | Coverage |
| --- | --- | --- | --- | --- |
| Gateway and OpenAI synchronous usage | `gateway_usage_billing.go`, `openai_gateway_usage.go`, `usage_billing_repo.go` | One idempotent `UsageBillingCommand`; usage log copies the same context | `UsageBillingApplyResult.PayerUserID` | `gateway_usage_billing_fallback_test.go`, `usage_billing_repo_unit_test.go` |
| Gateway balance preflight | `billing_cache_service.go` resolves the payer before reading balance | No mutation | Current selected payer | `billing_cache_service_balance_test.go` |
| Balance RPC deduct | `balance_ledger_service.go`, `balance_ledger_repo.go` | Ledger stores the payer snapshot; replay reads the committed result before current authorization resolution | Result payer | `balance_ledger_service_test.go`, `balance_ledger_repo_integration_test.go` |
| Balance RPC partial refund | Original deduct ledger row | Always credits the original payer; row lock prevents aggregate over-refund | Original payer | `balance_ledger_repo_integration_test.go` |
| Async media submit | `async_media_executor.go` stores the context on `async_media_tasks` before charging | Success delta refund, failure refund, timeout reconciliation, and duplicate terminal handling use task payer | Task payer | `async_media_executor_test.go` |
| Batch image submit | `batch_image_public.go` stores the context on `batch_image_jobs` before the hold | Capture, cancellation, failed submit, stale-unsubmitted recovery, and terminal worker release use job payer | Job payer | `batch_image_settlement_test.go`, `batch_image_billing_recovery_test.go`, `batch_image_mvp_smoke_test.go` |
| Company allocation and reclaim | Owner-scoped command in `organization_repo.go` | Stable two-user locking and an immutable transfer ledger; this is a transfer, not consumption | Both changed user balances | `organization_repo_integration_test.go` |
| Company-upgrade fee | Applicant root is the fixed payer | Pending reserve is captured on approval or released on reject/withdraw | Applicant root | `organization_repo_integration_test.go` |

## Explicitly non-billable or unavailable to IAM

| Path | Rationale or guard |
| --- | --- |
| Payment order and subscription purchase | `GuardIAMFinancialOperation` runs before order creation in `payment_order.go`. |
| Redeem code or stored value | `GuardIAMFinancialOperation` runs before redemption mutation in `redeem_service.go`. |
| Affiliate value transfer | `GuardIAMFinancialOperation` runs before transfer in `affiliate_service.go`. |
| Admin balance adjustment | A system-admin operation, not IAM consumption. It targets the explicitly selected user and invalidates that user's caches. |
| Payment-provider refund | Reverses a root-owned payment order. IAM users cannot create such orders. |
| Subscription quota usage | New IAM users receive no subscription grant. Existing root subscription behavior remains unchanged. |

## Review commands

Run these searches whenever a balance mutation or new billable producer is
introduced, then classify every new result above:

```bash
rg -n "Deduct|Refund|UpdateBalance|DeductBalance|AddBalance|Capture|Release" \
  backend/internal/service backend/internal/repository --glob '*.go'

rg -n "UsageLog\\{|PayerUserID|OrganizationID|BalanceSource|AuthzGeneration" \
  backend/internal/service backend/internal/repository backend/internal/rpc --glob '*.go'

rg -n "InvalidateUserBalance|CheckBalanceAfterDeduction" \
  backend/internal/service --glob '*.go'
```

The rollout reconciliation command must report zero for all payer-snapshot and
financial-conservation violations before either company feature flag is enabled.
