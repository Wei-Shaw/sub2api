## 1. Data Model and Authorization

- [x] 1.1 Add migrations and domain models for cost-center events, expense plans, source classifications, USD/original-money metadata, and reversal links.
- [x] 1.2 Add indexes and uniqueness constraints for event time, source references, usage/request identifiers, payment order identifiers, and recurring plan occurrence periods.
- [x] 1.3 Add administrator route authorization and audit operator fields for cost-center reads and expense writes.
- [x] 1.4 Add repository interfaces and idempotent append/list/summary helpers for cost-center events.

## 2. Billing and Usage Event Capture

- [x] 2.1 Create a settled payment income event from each newly fulfilled balance or subscription order, including USD/credited and original payment snapshots, with webhook idempotency.
- [x] 2.2 Capture new usage source classification from existing balance-source/subscription associations and persist paid, subscription, or unknown snapshots.
- [x] 2.3 Initially captured one idempotent upstream-cost event per finalized usage/request; this behavior is superseded and removed by task 7.1.
- [x] 2.4 Implement recharge-bonus and administrator/affiliate-grant consumption reporting; automatic associated upstream cost is superseded and removed by task 7.1.
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

- [x] 4.1 Define administrator report DTOs for income, consumption, expenses, forecasts, profit, margin, and warnings; the upstream-cost field is superseded and removed by task 7.2.
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

## 7. Manual Account Cost Revision

- [x] 7.1 Stop creating automatic upstream-cost events for token, image, and video usage while preserving existing operational usage-cost data.
- [x] 7.2 Exclude historical automatic upstream-cost events from cost-center summaries, event listings, profit, and reconciliation.
- [x] 7.3 Return and display readable source-person and account names in cost-center event details.
- [x] 7.4 Show the account name with usage account cost and increase the category selector height in account expense workflows.
- [x] 7.5 Add focused backend coverage and run proportional frontend/backend verification for the revision.

## 8. Cost-Center Account Cost Entry

- [x] 8.1 Classify account-targeted expense API writes as audited account expenses on the server.
- [x] 8.2 Add a cost-center append-cost action with complete searchable account selection, category, USD amount, occurred-at time, and audit note.
- [x] 8.3 Refresh cost-center summaries and events after creation and show localized success/error feedback.
- [x] 8.4 Add focused coverage and run proportional frontend/backend verification.

## 9. Account-Optional Cost Entry

- [x] 9.1 Allow cost-center append-cost submissions without an account while preserving account-targeted classification when one is selected.
- [x] 9.2 Add the audited-account-expense category and rename the append-cost audit-note label to note.
- [x] 9.3 Add focused coverage and run proportional frontend/backend and OpenSpec verification.

## 10. Optional Append-Cost Note

- [x] 10.1 Allow the cost-center append-cost form to submit without a note and omit empty notes from the request.
- [x] 10.2 Add focused frontend coverage and run proportional frontend and OpenSpec verification.
