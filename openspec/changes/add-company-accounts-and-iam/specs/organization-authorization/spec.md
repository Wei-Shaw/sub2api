## ADDED Requirements

### Requirement: Versioned system-managed policies
The authorization model SHALL represent permissions as versioned actions grouped into policies with immutable policy keys, display names, policy type, and descriptions. Phase one policies SHALL be system-managed and SHALL NOT be editable or deletable by organization owners. The system SHALL seed `CompanyFinanceReadOnly` and `CompanySharedBalanceUse` idempotently.

#### Scenario: List policies
- **WHEN** an organization owner lists attachable policies
- **THEN** each result SHALL include its key, localized display name, `system` type, description, version, and actions

#### Scenario: Attempt to edit a system policy
- **WHEN** an organization owner attempts to change or delete a system-managed policy
- **THEN** the system SHALL deny the request

### Requirement: Phase-one policy actions
`CompanyFinanceReadOnly` SHALL grant only read access to the root account's available, frozen, and total balance. `CompanySharedBalanceUse` SHALL grant eligibility to use the root balance as payer for billable API consumption but SHALL NOT grant visibility of the root balance amount. Neither policy SHALL permit recharge, subscription purchase, member administration, policy administration, balance allocation, or system-admin functions.

#### Scenario: Finance reader views root balance
- **WHEN** an IAM member with `CompanyFinanceReadOnly` requests the organization finance summary
- **THEN** the response SHALL include root available, frozen, and total balance
- **AND** SHALL not expose payment credentials or unrelated financial administration

#### Scenario: Shared-balance user lacks finance read
- **WHEN** an IAM member has `CompanySharedBalanceUse` but not `CompanyFinanceReadOnly`
- **THEN** the member SHALL be eligible for shared-balance consumption
- **AND** SHALL not receive the root balance amount from account, finance, or preflight APIs

### Requirement: Direct policy attachment
Only the active organization owner SHALL attach or detach system-managed policies directly to IAM users in the same organization. IAM users SHALL begin with no policy attachments. An operation targeting another organization SHALL be denied even if the target internal ID exists.

#### Scenario: Attach a policy
- **WHEN** an owner attaches `CompanyFinanceReadOnly` to an active member in their organization
- **THEN** the attachment SHALL become effective and SHALL record actor, subject, policy version, and time

#### Scenario: Cross-organization attachment
- **WHEN** an owner supplies an IAM user ID belonging to another organization
- **THEN** the system SHALL deny the operation without revealing that user's details

### Requirement: Owner capabilities are implicit
The sole organization owner SHALL receive organization member, authorization, allocation, finance, and usage management capabilities from the owner role rather than attachable policies. IAM users SHALL NOT receive or be promoted to the owner role in phase one.

#### Scenario: Owner manages members without attachments
- **WHEN** an approved company root has no policy attachments
- **THEN** it SHALL still be authorized to perform owner management actions

#### Scenario: Promote IAM user
- **WHEN** an owner attempts to grant the owner role to an IAM user
- **THEN** the system SHALL reject the operation

### Requirement: Deny-by-default evaluation and immediate invalidation
Authorization SHALL evaluate the authenticated user, organization status, membership status, owner role, direct policy attachments, policy version, and requested action. Actions not granted SHALL be denied. Policy attachment changes, member status changes, organization suspension, and policy-version changes SHALL immediately invalidate affected session and API-key authorization caches.

#### Scenario: Unattached action
- **WHEN** an IAM member requests an organization action absent from all attached policies
- **THEN** authorization SHALL deny it

#### Scenario: Revoke shared-balance policy
- **WHEN** an owner detaches `CompanySharedBalanceUse`
- **THEN** subsequent requests from existing sessions and API keys SHALL stop selecting the root payer immediately

### Requirement: Authorization audit trail
Every policy attachment, detachment, failed cross-organization attempt, lifecycle-driven revocation, and system policy version change SHALL append an immutable audit event with actor, subject, organization, action, result, timestamp, and request correlation ID.

#### Scenario: Policy detachment is audited
- **WHEN** an owner detaches a member policy
- **THEN** one append-only audit event SHALL identify the owner, member, policy, organization, and successful result
