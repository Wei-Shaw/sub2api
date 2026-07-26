## ADDED Requirements

### Requirement: Organization balance allocation
An active organization owner SHALL be able to allocate a positive amount from the root account's available balance to an active IAM user's available balance. The operation SHALL be atomic, non-overdrafting, idempotent, and recorded as paired immutable ledger movements identifying actor, source, destination, organization, amount, and post-operation balance snapshots.

#### Scenario: Allocate available funds
- **WHEN** the root has sufficient available balance and the owner allocates amount A to an active member
- **THEN** root available balance SHALL decrease by A
- **AND** member available balance SHALL increase by A
- **AND** the paired ledger movement SHALL be committed exactly once

#### Scenario: Allocation exceeds available balance
- **WHEN** amount A exceeds root available balance
- **THEN** the operation SHALL fail without changing either balance or creating a committed movement

### Requirement: Reclaim unspent allocations
An active organization owner SHALL be able to reclaim only an IAM user's available, unfrozen balance. Reclaim SHALL be atomic, non-overdrafting, idempotent, and recorded as paired immutable ledger movements. The owner SHALL NOT reclaim funds currently frozen or already consumed.

#### Scenario: Reclaim unspent funds
- **WHEN** a member has available balance A and frozen balance F and the owner reclaims amount A
- **THEN** A SHALL move to root available balance
- **AND** F SHALL remain unchanged

#### Scenario: Reclaim exceeds available funds
- **WHEN** the requested reclaim amount exceeds the member's available balance
- **THEN** the operation SHALL fail without changing root or member balances

### Requirement: Central effective-payer resolution
Every billable operation SHALL resolve an immutable billing context containing consuming user, organization, effective payer, balance source, and authorization version before reserving or deducting funds. A root or personal user SHALL pay from their own balance. An IAM user with an effective `CompanySharedBalanceUse` attachment SHALL use the organization root as payer; an IAM user without it SHALL use their own allocated balance. The system SHALL NOT fall back between those sources when the selected payer lacks funds.

#### Scenario: IAM user consumes shared balance
- **WHEN** an active IAM user has `CompanySharedBalanceUse` at authorization time
- **THEN** the billing context SHALL select the root user as effective payer
- **AND** the member's allocated balance SHALL remain unchanged

#### Scenario: IAM user consumes allocated balance
- **WHEN** an active IAM user lacks `CompanySharedBalanceUse`
- **THEN** the billing context SHALL select that IAM user as effective payer

#### Scenario: Selected root payer has insufficient balance
- **WHEN** a shared-balance member's root account cannot cover the charge
- **THEN** billing SHALL fail for insufficient balance
- **AND** SHALL NOT fall back to the member's allocated balance

#### Scenario: Shared policy is revoked
- **WHEN** `CompanySharedBalanceUse` is revoked before a new request resolves billing context
- **THEN** the new request SHALL select the member's allocated balance

### Requirement: Payer snapshot across all billing paths
Gateway billing, balance RPC deduction and refund, synchronous and asynchronous image billing, cache updates, low-balance notifications, reconciliation, and compensating refunds SHALL use the central billing context. Holds and deductions SHALL persist `consumer_user_id`, `organization_id`, `payer_user_id`, and balance source so later capture, release, refund, and reconciliation always target the original payer even if permissions change.

#### Scenario: Permission changes after asynchronous hold
- **WHEN** a media task reserves root funds and the member's shared-balance policy is then revoked
- **THEN** task capture or release SHALL still operate on the snapshotted root payer

#### Scenario: Refund after permission changes
- **WHEN** a charged request is refunded after its member's policy or lifecycle state changes
- **THEN** the refund SHALL credit the payer recorded by the original charge

#### Scenario: Low-balance notification
- **WHEN** consumption uses the root as payer and crosses a configured threshold
- **THEN** notification evaluation SHALL use the root payer's balance and notification settings
- **AND** SHALL not treat the member's allocated balance as the charged balance

### Requirement: Financial visibility boundaries
The organization owner and IAM users with `CompanyFinanceReadOnly` SHALL view the root available, frozen, and total balance. Other IAM users SHALL view only their own allocated available and frozen balances plus a non-numeric indication of whether their current consumption source is shared or allocated. Shared-balance permission alone SHALL never expose the root balance amount.

#### Scenario: Ordinary allocated-balance member
- **WHEN** an IAM member without finance permission requests account balance information
- **THEN** the response SHALL contain only that member's own balance amounts

#### Scenario: Shared-balance-only member
- **WHEN** an IAM member has shared-balance permission but lacks finance permission
- **THEN** the response MAY indicate `balance_source=shared`
- **AND** SHALL omit all root balance amounts

### Requirement: IAM financial product restrictions
IAM users SHALL NOT recharge, create payment orders, redeem value, purchase subscriptions, or transfer affiliate value. These guards SHALL execute in backend services and SHALL NOT rely solely on hidden frontend navigation. Allocated or shared balance SHALL only fund supported consumption.

#### Scenario: Direct API call to recharge
- **WHEN** an IAM user calls a recharge endpoint directly
- **THEN** the backend SHALL deny it before creating a payment order

#### Scenario: Direct API call to purchase subscription
- **WHEN** an IAM user calls a subscription purchase endpoint directly
- **THEN** the backend SHALL deny it without deducting any payer
