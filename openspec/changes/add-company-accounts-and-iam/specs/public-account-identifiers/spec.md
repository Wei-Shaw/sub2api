## ADDED Requirements

### Requirement: Unique user and enterprise identifiers
Every user SHALL have one immutable, 16-digit `account_id`. An IAM user SHALL have its own account ID and SHALL NOT reuse the account ID of a root user or organization owner. Every enterprise SHALL have an immutable `company_id`; user-to-enterprise association SHALL be resolved through `organization_memberships` and `organizations.company_id`.

#### Scenario: Root identity
- **WHEN** the system returns a root user's identity
- **THEN** `account_id` SHALL be a 16-digit string
- **AND** no second active or deleted user SHALL share that account ID

#### Scenario: IAM identity
- **WHEN** the system creates or returns an IAM user's identity
- **THEN** `account_id` SHALL be an independently generated 16-digit string
- **AND** the user's enterprise SHALL be resolved through its organization membership
- **AND** the response SHALL expose the user's account ID, not a second external user ID

#### Scenario: Enterprise association
- **WHEN** an IAM login or enterprise operation resolves an enterprise
- **THEN** the lookup SHALL use `company_id` and the membership relationship
- **AND** it SHALL NOT join an IAM user to an enterprise through `users.account_id`

#### Scenario: JSON serialization preserves digits
- **WHEN** a user or enterprise identifier is returned by an API
- **THEN** the response SHALL encode it as a JSON string rather than a numeric value

### Requirement: Cryptographically secure account ID generation
The Go application SHALL generate account IDs with `crypto/rand`, and the legacy Python backfill SHALL use `secrets`. Generation SHALL sample decimal digits without modulo bias, SHALL choose the first digit from `1-9`, and SHALL choose all remaining digits from `0-9`. The system SHALL retry generation after a uniqueness conflict and SHALL fail without partially creating the user if the retry limit is exhausted.

#### Scenario: Collision retry preserves the user creation transaction
- **WHEN** a generated account ID conflicts with an existing user
- **THEN** the system SHALL generate another unbiased 16-digit candidate and retry within the bounded limit
- **AND** it SHALL not leave a partially created user if all retries are exhausted

### Requirement: Global account ID uniqueness and non-reuse
The database SHALL enforce global uniqueness of non-null `account_id`, including soft-deleted users. `account_id` SHALL be immutable after assignment, and identifiers from deleted or archived users SHALL never be reused.

#### Scenario: Attempt to mutate an identifier
- **WHEN** an update attempts to change an assigned `account_id`
- **THEN** the system SHALL reject the update

#### Scenario: Soft-deleted identifier remains reserved
- **WHEN** a user is soft deleted
- **THEN** its account ID SHALL remain covered by the global uniqueness constraint
- **AND** future generation SHALL NOT reuse it

### Requirement: Legacy account migration
The rollout SHALL provide a separately runnable Python script that fills missing or shared account IDs in bounded transactions, supports dry-run and resumable execution, skips already unique rows, refuses ambiguous duplicate root accounts, and reports counts without printing secrets. A follow-up SQL migration SHALL require account IDs, enforce the digit format, install the global unique constraint, and remove the obsolete `external_user_id` column.

#### Scenario: Backfill resumes after interruption
- **WHEN** the backfill script is rerun after some rows have been committed
- **THEN** it SHALL skip rows whose account IDs are already unique
- **AND** it SHALL continue assigning identifiers only to remaining rows

#### Scenario: Ambiguous duplicate root IDs
- **WHEN** multiple root users share one account ID
- **THEN** the migration SHALL stop and require manual ownership reconciliation
- **AND** it SHALL not rewrite organization or billing ownership automatically
