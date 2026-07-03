## ADDED Requirements

### Requirement: WS Preflight Failures Lower Account Priority Temporarily

The OpenAI account scheduler SHALL temporarily lower the priority of accounts with recent WS preflight failures so that normal non-sticky selection avoids accounts that are actively reconnecting.

#### Scenario: Account with recent preflight failures is deprioritized

- **GIVEN** two eligible OpenAI accounts have the same configured priority
- **AND** one account has recent WS preflight ping failures
- **WHEN** the scheduler compares the two accounts for a non-sticky request
- **THEN** the account with recent WS preflight failures is treated as lower priority
- **AND** the penalty expires after the recent-failure window passes

### Requirement: Account Performance Includes WS Connection Metrics

The system SHALL persist and aggregate WS connection metrics for account performance diagnostics, including connection reuse, preflight failures, connection acquisition time, upstream payload size, event count, and queue wait time.

#### Scenario: Admin views account performance diagnostics

- **GIVEN** a WSv2 request records connection metrics
- **WHEN** an admin views account performance in account management
- **THEN** the performance summary includes connection timing and preflight failure hints
- **AND** the detailed tooltip can distinguish connection/queue/payload factors from total generation time
