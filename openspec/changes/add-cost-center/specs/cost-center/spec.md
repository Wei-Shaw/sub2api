## ADDED Requirements

### Requirement: Cost-center events use auditable USD snapshots

The system SHALL record every in-scope post-activation cost-center event with an event type, occurred-at timestamp, USD amount, source classification, and operator or system origin. Events derived from non-USD money SHALL also retain original amount, original currency, and the settlement-time FX rate. Settled events MUST be immutable; corrections SHALL use an auditable reversal or adjustment event.

#### Scenario: Settled payment creates cash-income event
- **WHEN** a payment order is fulfilled successfully after the cost center is enabled
- **THEN** the system creates one idempotent cash-income event with the settled USD snapshot, payment order reference, original currency metadata, and source order identifier

#### Scenario: Repeated webhook does not duplicate income
- **WHEN** the same payment fulfillment notification is processed more than once
- **THEN** the cost-center event remains a single event linked to that payment order

#### Scenario: Historical records are excluded
- **WHEN** an administrator opens a report covering dates before activation
- **THEN** the report does not invent cost-center events for historical orders, usage, balances, or expenses

### Requirement: Account and global expenses support one-off and recurring workflows

The system SHALL allow an administrator to create a one-off expense for an account or a global operating expense, and SHALL support recurring expense plans that materialize due occurrences as pending records. A bulk account expense amount SHALL create an independent expense event for every selected account. Pending recurring records SHALL be excluded from cash profit until confirmed settled.

#### Scenario: Create-account initial expense
- **WHEN** an administrator creates an account with an initial expense amount and category
- **THEN** the system creates the account and one settled expense event linked to that account with the supplied USD amount and note

#### Scenario: Bulk expense applies per account
- **WHEN** an administrator bulk-edits five accounts with an expense amount of USD 10
- **THEN** the system creates five separate USD 10 expense events, one for each account, for a total of USD 50

#### Scenario: Append ongoing account expense
- **WHEN** an administrator uses the account action menu to add a later renewal or recharge expense
- **THEN** the system appends a new event without overwriting or mutating prior account expenses

#### Scenario: Recurring occurrence awaits confirmation
- **WHEN** a recurring expense plan reaches a due period
- **THEN** the system materializes one pending occurrence keyed by the plan and period, and excludes it from cash profit until an administrator confirms payment

#### Scenario: Global operating expense
- **WHEN** an administrator records a server, proxy, payment-fee, rebate, refund-loss, or other operating expense without an account target
- **THEN** the system records it as a global expense with a category, USD amount, occurred-at date, and audit metadata

### Requirement: New usage is classified by funding source and upstream cost

The system SHALL classify post-activation token consumption as paid balance, subscription, recharge bonus, administrator/affiliate grant, or unknown when the source can be determined from existing billing associations. Each usage-derived cost entry SHALL be idempotently linked to the usage/request identifier and SHALL preserve the account upstream-cost snapshot used for reporting.

#### Scenario: Paid balance consumption
- **WHEN** a usage record deducts a user's paid balance
- **THEN** the cost center reports its realized consumption income separately from its account upstream cost

#### Scenario: Gift consumption
- **WHEN** a usage record is funded by recharge bonus or an administrator grant
- **THEN** the cost center excludes its consumed face value from cash and realized income, reports it as promotional/free consumption, and includes its associated upstream cost in operating costs

#### Scenario: Unknown source is visible
- **WHEN** a new usage record has no resolvable funding source
- **THEN** the cost center places it in an explicit unknown bucket and exposes it to reconciliation instead of guessing a paid or gifted source

### Requirement: Subscription revenue is recognized over token-quota consumption

The system SHALL snapshot a paid subscription plan's price, standard token-quota value, validity window, and realization factor at fulfillment. For each linked usage record, it SHALL recognize at most one amount equal to standard consumption multiplied by that factor, capped by the remaining paid entitlement. Unused entitlement SHALL remain deferred, and expired unused entitlement SHALL be shown separately without automatic income recognition.

#### Scenario: Subscription purchase creates deferred entitlement
- **WHEN** a subscription order is fulfilled
- **THEN** the system records the USD cash income and a subscription entitlement snapshot, but does not recognize the full plan price as consumption income immediately

#### Scenario: Subscription usage recognition
- **WHEN** a linked subscription usage consumes standard token-quota value during its validity window
- **THEN** the system creates one idempotent recognition event using the frozen realization factor and reduces the remaining deferred entitlement

#### Scenario: Expired unused entitlement
- **WHEN** a subscription reaches its expiry with unused token quota
- **THEN** the cost center reports the unused amount as expired entitlement and does not recognize it automatically as income

### Requirement: Administrator reports provide cash and operating profit by time range

The system SHALL provide an administrator-only report query with an inclusive start and exclusive end timestamp in the configured reporting timezone. The response SHALL include cash income, realized consumption income, promotional/free consumption, upstream token cost, settled operating expenses, pending expense forecast, refunds, fees/rebates when available, cash profit, operating profit, and profit margin. It SHALL support filtering and grouping by account, platform, user, group, model, subscription plan, source classification, and expense category.

#### Scenario: Time-range summary
- **WHEN** an administrator requests a report for a custom date range
- **THEN** the response aggregates only events and usage finalized within that range and returns both cash profit and operating profit in USD

#### Scenario: Pending expenses are separated
- **WHEN** the range contains generated but unconfirmed recurring expenses
- **THEN** the response excludes them from cash profit and returns their amount in pending forecast totals

#### Scenario: Account profitability drill-down
- **WHEN** an administrator filters by an account
- **THEN** the response separates that account's usage-derived upstream cost, manual/recurring account expenses, paid consumption income, gifted consumption cost, and resulting operating profit

#### Scenario: Unauthorized access is denied
- **WHEN** a non-administrator requests a cost-center report or expense mutation
- **THEN** the API rejects the request and the administrator navigation entry and mutation controls are not shown

### Requirement: Cost-center console supports review and audit actions

The administrator console SHALL provide a cost-center route with time-range controls, summary metrics, income/consumption and expense detail views, source/category filters, recurring-plan pending confirmations, and an audit-friendly event detail. Account creation, bulk edit, and account action workflows SHALL expose the applicable expense entry controls.

#### Scenario: Review a recurring expense
- **WHEN** an administrator opens a pending recurring expense from the cost center
- **THEN** the UI shows its plan, period, target, category, USD amount, and notes, and allows confirm, cancel, or adjustment with an audit reason

#### Scenario: Expense entry from account action menu
- **WHEN** an administrator opens an account's action menu
- **THEN** an add-expense action opens a form that appends a new event and leaves prior events unchanged
