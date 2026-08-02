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
Every billable operation SHALL resolve an immutable billing context containing consuming user, organization, effective wallet attribution, balance source, and authorization version before reserving or deducting funds. A root or personal user SHALL pay from their own balance. An IAM user's allocated balance SHALL be selected when it can cover the full charge. When the allocated balance cannot cover the charge, an effective `CompanySharedBalanceUse` attachment SHALL select the organization's independent balance; it SHALL NOT debit the organization owner's personal balance. Without that attachment, the member SHALL remain the selected payer and the charge SHALL fail for insufficient balance. One charge SHALL NOT be split across both balances.

#### Scenario: Allocated balance takes priority over shared permission
- **WHEN** an active IAM user has `CompanySharedBalanceUse` and allocated balance sufficient for charge A
- **THEN** the billing context SHALL select the IAM user as effective payer
- **AND** charge A SHALL be deducted from the member's allocated balance
- **AND** the root balance SHALL remain unchanged

#### Scenario: IAM user consumes allocated balance
- **WHEN** an active IAM user lacks `CompanySharedBalanceUse`
- **THEN** the billing context SHALL select that IAM user as effective payer

#### Scenario: Shared balance covers an insufficient allocation
- **WHEN** an active IAM user's allocated balance cannot cover charge A and `CompanySharedBalanceUse` is effective
- **THEN** the billing context SHALL select the organization balance as the effective wallet for the full charge
- **AND** the member's remaining allocated balance SHALL remain unchanged
- **AND** the organization owner's personal balance SHALL remain unchanged

#### Scenario: Allocation is insufficient without shared permission
- **WHEN** an active IAM user's allocated balance cannot cover charge A and `CompanySharedBalanceUse` is not effective
- **THEN** billing SHALL fail for insufficient balance without changing either balance

#### Scenario: Shared policy is revoked
- **WHEN** `CompanySharedBalanceUse` is revoked before a new request resolves billing context
- **THEN** the new request SHALL select the member's allocated balance
- **AND** SHALL fail if that balance cannot cover the charge

### Requirement: Payer snapshot across all billing paths
Gateway billing, balance RPC deduction and refund, synchronous and asynchronous image billing, cache updates, low-balance notifications, reconciliation, and compensating refunds SHALL use the central billing context. Holds and deductions SHALL persist `consumer_user_id`, `organization_id`, `payer_user_id`, and balance source so later capture, release, refund, and reconciliation always target the original wallet even if permissions change. New organization-wallet operations SHALL use `balance_source=company`; historical `balance_source=shared` snapshots SHALL retain their original owner-wallet settlement semantics during rollout.

#### Scenario: Permission changes after asynchronous hold
- **WHEN** a media task reserves organization funds and the member's shared-balance policy is then revoked
- **THEN** task capture or release SHALL still operate on the snapshotted organization wallet

#### Scenario: Refund after permission changes
- **WHEN** a charged request is refunded after its member's policy or lifecycle state changes
- **THEN** the refund SHALL credit the payer recorded by the original charge

#### Scenario: Low-balance notification
- **WHEN** consumption uses the organization wallet and crosses a configured threshold
- **THEN** personal owner low-balance notification and personal balance-cache mutation SHALL NOT run
- **AND** organization member spend-limit alert evaluation SHALL use the company-sponsored usage

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
