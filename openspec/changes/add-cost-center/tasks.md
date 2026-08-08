## 1. Data Model and Authorization

- [x] 1.1 Add migrations and domain models for cost-center events, expense plans, source classifications, USD/original-money metadata, and reversal links.
- [x] 1.2 Add indexes and uniqueness constraints for event time, source references, usage/request identifiers, payment order identifiers, and recurring plan occurrence periods.
- [x] 1.3 Add administrator route authorization and audit operator fields for cost-center reads and expense writes.
- [x] 1.4 Add repository interfaces and idempotent append/list/summary helpers for cost-center events.

## 2. Billing and Usage Event Capture

- [x] 2.1 Create a settled payment income event from each newly fulfilled balance or subscription order, including USD/credited and original payment snapshots, with webhook idempotency.
- [x] 2.2 Capture new usage source classification from existing balance-source/subscription associations and persist paid, subscription, or unknown snapshots.
- [x] 2.3 Capture one idempotent upstream-cost event per finalized usage/request using the existing account pricing and account multiplier snapshot without changing user deductions.
- [x] 2.4 Implement recharge-bonus and administrator/affiliate-grant consumption reporting so face value is excluded from income while associated upstream cost remains included.
- [x] 2.5 Snapshot subscription plan price, standard quota valuation, validity window, and realization factor at fulfillment; create bounded, idempotent recognition events as linked usage is finalized.
- [x] 2.6 Add deferred and expired-entitlement calculations, keeping unused/expired subscription quota out of recognized income by default.
- [x] 2.7 Add reconciliation jobs or diagnostics for missing/duplicate cost-center events and unknown source classifications.

## 3. Expense Plans and Account Workflows

- [x] 3.1 Implement one-off expense creation for account-targeted and global operating expenses with categories, notes, occurred-at date, operator, and settled/pending status.
- [x] 3.2 Implement recurring expense plans and occurrence materialization for daily, weekly, monthly, quarterly, yearly, and custom intervals with transactional idempotency.
- [x] 3.3 Implement administrator confirmation, cancellation, adjustment, and reversal of pending/settled recurring expenses with audit reasons.
- [x] 3.4 Extend account creation to accept an optional initial expense and create the linked event atomically after account creation succeeds.
- [x] 3.5 Extend account bulk edit to apply an entered expense independently to every selected account and return per-account failures without partial duplicate writes.
- [x] 3.6 Add an account action-menu expense entry and account expense history/detail endpoint.
- [x] 3.7 Add global operating-expense management through the categorized cost-center expense endpoint.

## 4. Cost-Center Reporting API

- [x] 4.1 Define administrator report DTOs for cash income, realized income, promotional/free consumption, upstream cost, settled expenses, pending forecast, cash profit, operating profit, margin, and unknown-source warnings.
- [x] 4.2 Implement inclusive-start/exclusive-end time-range filtering and USD aggregation from cost-center events.
- [x] 4.3 Implement filters and groupings by account, platform, user, group, model, subscription plan, source classification, and expense category.
- [x] 4.4 Implement paginated event/expense detail endpoints and stable occurred-at/id ordering.
- [x] 4.5 Add authorization tests proving non-administrators cannot read or mutate cost-center data.

## 5. Administrator Frontend

- [x] 5.1 Add the administrator cost-center route, navigation entry, localization keys, permission visibility, and page shell.
- [x] 5.2 Build time-range controls, USD summary cards, cash-vs-operating profit sections, pending forecast, margin, and unknown-source reconciliation indicators.
- [x] 5.3 Build income/consumption and expense detail tables with filters, grouping/drill-down, pagination, source/category labels, and event audit details.
- [x] 5.4 Build recurring expense pending-review controls for confirm, cancel, adjust, and reverse actions with required audit reasons.
- [x] 5.5 Add initial expense controls to account creation, bulk-edit, and account action-menu flows; show per-account bulk results and validation errors.

## 6. Verification and Rollout

- [x] 6.1 Add service unit tests for event defaults, validation, and report inputs.
- [x] 6.2 Add repository/service integration tests for recurring occurrence uniqueness, pending confirmation, reversals, account/global expenses, and report time boundaries.
- [x] 6.3 Add frontend tests for route visibility, summary rendering, date filters, expense entry in all three account workflows, and pending confirmations.
- [x] 6.4 Add migration/checksum coverage and verify no historical backfill or mutation occurs.
- [x] 6.5 Run the repository-required frontend and backend checks before submitting the implementation.
