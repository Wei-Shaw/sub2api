## ADDED Requirements

### Requirement: Owner-created IAM members
Only the active organization's owner administrator SHALL create IAM members. Creation SHALL require a login name and SHALL assign the member to the owner's organization; an arbitrary organization or root account ID supplied by a client SHALL be ignored or rejected. A new IAM user SHALL receive no registration grant and SHALL start with zero available and frozen balance.

#### Scenario: Owner creates a member
- **WHEN** an active organization owner submits a valid unused login name
- **THEN** the system SHALL create an IAM user in that owner's organization with zero balances and no registration grant

#### Scenario: Non-owner attempts creation
- **WHEN** an IAM member or unrelated root user attempts to create a member
- **THEN** the system SHALL deny the request

### Requirement: Twenty-member default limit
Each organization SHALL have a configurable IAM-member limit that defaults to 20. The root account SHALL NOT count. Active and disabled IAM users SHALL count; archived IAM users SHALL not count. Creation SHALL lock or otherwise serialize the organization limit check so concurrent requests cannot exceed the configured limit.

#### Scenario: Create the twentieth member
- **WHEN** an organization has 19 non-archived IAM users
- **THEN** one additional valid member creation SHALL succeed

#### Scenario: Reject the twenty-first member
- **WHEN** an organization already has 20 non-archived IAM users under the default configuration
- **THEN** another member creation SHALL fail without creating credentials or partial records

#### Scenario: Concurrent creation at the limit
- **WHEN** an organization with 19 counted members processes two member creations concurrently
- **THEN** at most one SHALL commit under the default limit

#### Scenario: Archive releases a slot
- **WHEN** one of 20 members is archived
- **THEN** the owner SHALL be able to create one replacement while the archived member's history remains retained

### Requirement: IAM login principal
An IAM login name SHALL contain 1 to 64 ASCII letters, digits, periods, hyphens, or underscores and SHALL be unique case-insensitively among non-archived users in the organization. The canonical IAM principal SHALL be `<login_name>@<16-digit-root-account-id>`. IAM authentication SHALL accept the canonical principal and password, SHALL resolve exactly one organization member, and SHALL not treat the optional recovery email as a login identifier.

#### Scenario: Canonical principal is created
- **WHEN** the owner creates login name `finance.reader`
- **THEN** the member's login principal SHALL be `finance.reader@<owner-account-id>`

#### Scenario: Same name in different organizations
- **WHEN** two organizations each create login name `operator`
- **THEN** both SHALL succeed because their canonical principals contain different root account IDs

#### Scenario: Case-insensitive duplicate in one organization
- **WHEN** `Finance` exists and the same organization attempts to create `finance`
- **THEN** creation SHALL be rejected

#### Scenario: Recovery email used as IAM login
- **WHEN** an IAM user supplies their recovery email and password to IAM login
- **THEN** authentication SHALL fail without revealing whether that email exists

### Requirement: Initial and reset credentials
Member creation SHALL generate a cryptographically random initial password, store only its password hash, display the plaintext exactly once to the owner, and require the IAM user to change it at first successful authentication before accessing other protected APIs. An optional recovery email SHALL require verification before self-service password recovery; without a verified recovery email, only the organization owner SHALL reset the member password.

#### Scenario: Initial password shown once
- **WHEN** member creation succeeds
- **THEN** the creation response SHALL contain the initial password once
- **AND** subsequent reads SHALL never return that plaintext

#### Scenario: First login requires password change
- **WHEN** an IAM user authenticates with an initial or owner-reset password
- **THEN** the session SHALL be restricted to password-change and logout operations until the password is changed

#### Scenario: No verified recovery email
- **WHEN** an IAM user without a verified recovery email requests self-service reset
- **THEN** the system SHALL return a non-enumerating response without issuing a reset credential
- **AND** the owner SHALL remain able to reset that member's password

### Requirement: IAM lifecycle enforcement
An owner SHALL be able to disable, re-enable, and archive IAM users. Disabling or archiving SHALL immediately revoke active sessions and API-key authorization caches. Disabled and archived users SHALL be unable to log in or use API keys. Archival SHALL be a non-destructive terminal lifecycle action in phase one and SHALL retain usage, billing, policy, and audit history.

#### Scenario: Disable an active IAM user
- **WHEN** the owner disables a member
- **THEN** existing sessions and cached API-key authorizations SHALL stop working immediately
- **AND** the disabled member SHALL continue to count toward the member limit

#### Scenario: Archive an IAM user
- **WHEN** the owner archives a member
- **THEN** login and API access SHALL remain denied
- **AND** historical records SHALL continue to identify that member by immutable ID

### Requirement: IAM product restrictions
An active IAM user MAY manage their own profile, API keys, allocated balance view, and personal usage subject to policy checks. IAM users SHALL NOT recharge, create or pay payment orders, redeem codes or stored value, purchase subscriptions, or transfer affiliate value, regardless of attached policies.

#### Scenario: IAM user creates an API key
- **WHEN** an active IAM user with a changed initial password creates an API key
- **THEN** the key SHALL belong to that IAM user and inherit that user's organization and authorization context

#### Scenario: IAM user attempts a prohibited purchase
- **WHEN** an IAM user attempts to recharge, redeem value, or purchase a subscription
- **THEN** the system SHALL deny the request before creating an order or changing funds
