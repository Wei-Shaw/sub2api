## ADDED Requirements

### Requirement: Root and IAM public identifier semantics
Every user SHALL have an immutable `external_user_id`. A personal or company root user SHALL have a 16-digit `account_id` and SHALL use that same value as its `external_user_id`. An IAM user SHALL inherit its organization's 16-digit root `account_id` and SHALL have an independent 18-digit `external_user_id`. Public APIs SHALL serialize both fields as strings.

#### Scenario: Personal or company root identity
- **WHEN** the system returns a root user's identity
- **THEN** `account_id` SHALL be a 16-digit string
- **AND** `external_user_id` SHALL equal `account_id`

#### Scenario: IAM identity
- **WHEN** the system returns an IAM user's identity
- **THEN** `account_id` SHALL equal the organization's root account ID
- **AND** `external_user_id` SHALL be an independently generated 18-digit string

#### Scenario: JSON serialization preserves digits
- **WHEN** either identifier is returned by an API
- **THEN** the response SHALL encode it as a JSON string rather than a numeric value

### Requirement: Cryptographically secure decimal generation
The Go application SHALL generate identifiers with `crypto/rand`, and the legacy Python backfill SHALL use `secrets`. Generation SHALL sample decimal digits without modulo bias, SHALL choose the first digit from `1-9`, and SHALL choose all remaining digits from `0-9`. The system SHALL retry generation after a uniqueness conflict and SHALL NOT derive identifiers from timestamps, database primary keys, email addresses, or predictable sequences.

#### Scenario: Generate a root identifier
- **WHEN** a new root user is created
- **THEN** the system SHALL generate one non-zero-leading 16-digit value using a cryptographically secure random source
- **AND** SHALL assign it to both `account_id` and `external_user_id`

#### Scenario: Generate an IAM identifier
- **WHEN** an IAM user is created
- **THEN** the system SHALL generate one non-zero-leading 18-digit `external_user_id` using a cryptographically secure random source
- **AND** SHALL copy the organization's root ID into `account_id`

#### Scenario: Uniqueness conflict
- **WHEN** inserting a generated `external_user_id` conflicts with an existing or soft-deleted user
- **THEN** the system SHALL generate a new value and retry within a bounded attempt limit
- **AND** SHALL fail the operation without partially creating the identity if the limit is exhausted

### Requirement: Global uniqueness, immutability, and non-reuse
The database SHALL enforce global uniqueness of `external_user_id`, including soft-deleted users. `external_user_id` and `account_id` SHALL be immutable after assignment, and identifiers from deleted or archived identities SHALL never be reused.

#### Scenario: Attempt to mutate an identifier
- **WHEN** an update attempts to change an assigned `external_user_id` or to move a user to a different `account_id`
- **THEN** the system SHALL reject the update

#### Scenario: Soft-deleted identifier remains reserved
- **WHEN** a user is soft deleted
- **THEN** its identifier SHALL remain covered by the global uniqueness constraint
- **AND** future generation SHALL NOT reuse it

### Requirement: Phased legacy backfill
Identifier rollout SHALL use an additive migration that first permits null identifiers and adds a partial unique index over populated `external_user_id` values. A separately runnable Python script SHALL fill missing identifiers in bounded transactions, support dry-run and resumable execution, skip already populated rows, and report counts and failures. A follow-up migration SHALL require both identifiers, enforce their digit formats and root semantics, and replace the partial index with a full global unique constraint.

#### Scenario: Backfill resumes after interruption
- **WHEN** the backfill script is rerun after some legacy rows were committed
- **THEN** it SHALL skip rows whose identifiers are already populated
- **AND** SHALL continue assigning identifiers only to remaining rows

#### Scenario: Dry run
- **WHEN** an operator runs the script in dry-run mode
- **THEN** it SHALL report eligible and invalid rows without modifying the database

#### Scenario: Final constraint migration is premature
- **WHEN** any non-deleted or soft-deleted user still lacks a required identifier
- **THEN** the follow-up constraint migration SHALL fail rather than silently invent or discard an identity
