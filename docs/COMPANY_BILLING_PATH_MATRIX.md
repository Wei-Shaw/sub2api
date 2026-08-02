# Company Billing Path Matrix

This matrix is the enablement inventory for company shared and allocated balance. Every billable path must resolve the consumer once, persist the effective payer snapshot, and reuse that payer for settlement, release, refund, cache invalidation, and notifications.

| Path | Resolve point | Persistent snapshot | Later payer source | Cache target | Coverage |
| --- | --- | --- | --- | --- | --- |
| Gateway preflight | `BillingCacheService.CheckBillingEligibility` | N/A | Current resolver result | Effective payer | Shared, allocated, resolver failure, and no-fallback unit tests |
| Gateway synchronous usage | `applyUsageBilling` | `usage_logs` and atomic usage-billing command | Same command snapshot | Effective payer | Usage billing repository and gateway unit/integration tests |
| Gateway legacy fallback | `applyUsageBilling` | `usage_logs` | Resolved context passed into fallback | Effective payer | Explicit shared-payer fallback unit test; production normally has unified repository |
| Balance RPC deduct | `BalanceLedgerService.Deduct` | `balance_ledger` | Deduct ledger snapshot | Effective payer | Idempotency, insufficient balance, and payer snapshot repository tests |
| Balance RPC refund | Original deduct lookup | Refund ledger copies original deduct | Original ledger payer | Original payer | Partial/concurrent refund repository tests |
| Async media submit | `AsyncMediaService.Submit` | `async_media_tasks` | Task payer snapshot | Task payer | Failure, timeout, idempotency, and permission-change payer tests |
| Async media success/failure | No re-resolution | Terminal `usage_logs` copies task | Task payer | Task payer | Full/delta refund and terminal usage tests |
| Batch image reserve | `BatchImagePublicService.Submit` | `batch_image_jobs` and hold command | Job payer snapshot | Job payer | Reserve and insufficient selected-payer tests |
| Batch image capture/release | No re-resolution | Capture/release command copies job | Job payer | Job payer | Settlement, cancellation, recovery, duplicate settlement, and release tests |
| Organization allocation/reclaim | Owner organization context | `organization_financial_ledger` | Explicit locked source/destination | Both changed users through authorization invalidation | Conservation and idempotency integration tests |

Non-billable or out-of-scope balance mutations:

- Recharge, promotion, redemption, affiliate awards, and administrator balance adjustments credit a specific account. IAM callers are denied before recharge, redemption, subscription purchase, or affiliate transfer.
- Payment refund reverses the original personal payment order. IAM users cannot create payment orders, so it does not perform organization payer selection.
- `UsageService.Create` is an unused legacy write API with no registered HTTP producer. Gateway producers use the atomic usage-billing repository.
- Subscription quota consumption remains attached to the consumer's subscription. IAM users cannot purchase subscriptions; an existing imported subscription is not converted into shared-balance billing.

Enablement is blocked if a new deduction/refund call site is added without updating this matrix and adding either payer-snapshot tests or a documented non-billable rationale.
