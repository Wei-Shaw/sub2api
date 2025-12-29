## ADDED Requirements

### Requirement: Versioned SQL Migrations
The system MUST manage database schema changes via versioned SQL migration files under `backend/migrations/*.sql` and MUST record applied migrations in the database for auditability and idempotency.

#### Scenario: Migrations are applied idempotently
- **GIVEN** an empty PostgreSQL database
- **WHEN** the backend initializes its database connection
- **THEN** it MUST apply all SQL migrations in lexicographic filename order
- **AND** it MUST record each applied migration in `schema_migrations` with a checksum
- **AND** a subsequent initialization MUST NOT re-apply already-recorded migrations

### Requirement: Soft Delete Semantics
For entities that support soft delete, the system MUST preserve the existing semantics: soft-deleted rows are excluded from queries by default, and delete operations are idempotent.

#### Scenario: Soft-deleted rows are hidden by default
- **GIVEN** a row has `deleted_at` set
- **WHEN** the backend performs a standard "list" or "get" query
- **THEN** the row MUST NOT be returned by default

### Requirement: Allowed Groups Data Model
The system MUST migrate `users.allowed_groups` from a PostgreSQL array column to a normalized join table for type safety and maintainability.

#### Scenario: Allowed groups are represented as relationships
- **GIVEN** a user is allowed to bind a group
- **WHEN** the user/group association is stored
- **THEN** it MUST be stored as a `(user_id, group_id)` relationship row
- **AND** removing an association MUST hard-delete that relationship row
