## ADDED Requirements

### Requirement: Company upgrade application
An active personal root user SHALL be able to submit one pending company-upgrade application containing a non-empty company name. Company names SHALL NOT be globally unique, but the review response SHALL surface normalized exact and similar-name matches. Existing company roots and IAM users SHALL NOT be eligible to apply.

#### Scenario: Eligible user submits an application
- **WHEN** an active personal root user supplies a valid company name and has no pending application
- **THEN** the system SHALL create a pending application linked to that user

#### Scenario: Duplicate company name
- **WHEN** the submitted name matches or resembles an existing or pending company name
- **THEN** the system SHALL allow submission
- **AND** the system-admin review view SHALL show the matching names as a warning

#### Scenario: Ineligible applicant
- **WHEN** an IAM user, existing company root, suspended user, or user with another pending application submits an application
- **THEN** the system SHALL reject the request without reserving a fee

### Requirement: Configurable upgrade fee reservation
The company-upgrade fee SHALL be configurable and SHALL default to USD 20. Submission SHALL snapshot the configured amount and currency on the application. In one database transaction, the system SHALL verify available balance, decrease available balance by the snapshot amount, increase frozen balance by the same amount, create an immutable idempotent ledger entry, and create the pending application. The application SHALL NOT be submitted when available balance is insufficient.

#### Scenario: USD 20 default fee is reserved
- **WHEN** the configured fee is unchanged and an eligible user with at least USD 20 submits an application
- **THEN** available balance SHALL decrease by USD 20
- **AND** frozen balance SHALL increase by USD 20
- **AND** the application fee snapshot SHALL be `20` and `USD`

#### Scenario: Fee configuration changes after submission
- **WHEN** the configured upgrade fee changes after an application was submitted
- **THEN** approval, rejection, or withdrawal SHALL settle or release the amount recorded in that application's fee snapshot

#### Scenario: Insufficient available balance
- **WHEN** an eligible user's available balance is less than the current fee
- **THEN** submission SHALL fail atomically
- **AND** neither balances, ledger entries, nor an application SHALL be created or changed

#### Scenario: Replayed submission
- **WHEN** the same submission idempotency key is retried
- **THEN** the system SHALL return the original application and reservation result without freezing the fee again

### Requirement: System-admin review and activation
An active system administrator SHALL be able to approve or reject a pending application, including an application they submitted themselves. Approval SHALL atomically capture the frozen fee, activate one organization, and assign the applicant as its sole owner administrator. Rejection SHALL require a non-empty reason and SHALL atomically release the frozen fee back to available balance. An approved fee SHALL be non-refundable.

#### Scenario: Approve an application
- **WHEN** an authorized system administrator approves a pending application
- **THEN** the frozen fee SHALL be captured through an immutable ledger entry
- **AND** one active organization SHALL be created using the applicant's root `account_id`
- **AND** the applicant SHALL become its sole owner administrator
- **AND** the organization effective time SHALL equal the approval time

#### Scenario: Reject an application
- **WHEN** an authorized system administrator rejects a pending application with a reason
- **THEN** the application SHALL record the reviewer, reason, and decision time
- **AND** the full fee snapshot SHALL move from frozen balance back to available balance

#### Scenario: System administrator reviews their own application
- **WHEN** an active system administrator decides their own pending application
- **THEN** the system SHALL apply the decision under the same atomic settlement and audit rules as any other review

#### Scenario: Duplicate or concurrent decisions
- **WHEN** approval or rejection is retried or two decisions race for the same application
- **THEN** exactly one terminal decision and one fee settlement SHALL be committed

### Requirement: Applicant withdrawal
The applicant SHALL be able to withdraw a pending application before review. Withdrawal SHALL atomically mark the application withdrawn and release its full fee snapshot. A terminal application SHALL NOT be withdrawable.

#### Scenario: Withdraw pending application
- **WHEN** the applicant withdraws a pending application
- **THEN** its reserved fee SHALL be released to available balance exactly once

#### Scenario: Withdraw decided application
- **WHEN** the applicant attempts to withdraw an approved or rejected application
- **THEN** the system SHALL reject the operation without changing balances

### Requirement: Upgrade and company-name notifications
After transaction commit, the system SHALL durably enqueue email notifications to all eligible system administrators for a newly submitted application and to the applicant for approval, rejection, or withdrawal. Notification delivery SHALL be retryable and idempotent and SHALL NOT roll back the business transaction when an email provider is unavailable.

#### Scenario: Submission email
- **WHEN** an application transaction commits
- **THEN** the outbox SHALL contain one deduplicated review notification for each eligible system administrator

#### Scenario: Decision email provider failure
- **WHEN** an approval or rejection commits but email delivery fails
- **THEN** the application decision SHALL remain committed
- **AND** delivery SHALL be retried without sending duplicate logical notifications

### Requirement: Company name change and suspension lifecycle
An organization owner SHALL be able to request a company-name change, which SHALL take effect only after system-admin approval under the same notification rules. An owner who is also an active system administrator MAY review their own name-change request. Phase one SHALL NOT support company downgrade, dissolution, ownership transfer, or additional organization administrators. A system administrator SHALL be able to suspend and reactivate an organization without deleting its audit or financial history.

#### Scenario: Name change awaits approval
- **WHEN** an owner submits a valid new company name
- **THEN** the active company name SHALL remain unchanged until an active system administrator approves the request

#### Scenario: Suspend an organization
- **WHEN** a system administrator suspends an active organization
- **THEN** IAM login and organization-funded consumption SHALL be denied
- **AND** organization, member, usage, authorization, and ledger history SHALL remain queryable by system administrators
