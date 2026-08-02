## ADDED Requirements

### Requirement: Personal API keys support an ordered fallback group list
The system SHALL allow the owner of a personal API key to store up to five fallback group IDs in an explicit order in addition to the existing primary group.

#### Scenario: Create a key with fallback groups
- **WHEN** a user creates a personal API key with primary group A and fallback groups B then C
- **THEN** the system persists and returns the ordered list `[B, C]`

#### Scenario: Update fallback order
- **WHEN** a user updates the fallback list from `[B, C]` to `[C, B]`
- **THEN** subsequent requests use C before B and API responses preserve `[C, B]`

#### Scenario: Existing key without fallbacks
- **WHEN** an existing API key has no fallback group configuration
- **THEN** its request routing behavior remains unchanged

### Requirement: Fallback group configuration is validated
The system MUST accept only distinct, enabled groups that the API key owner is allowed to bind and whose platform exactly matches the primary group, MUST reject the primary group in the fallback list, and MUST reject fallback groups for enterprise API keys.

#### Scenario: Unauthorized fallback group
- **WHEN** a user submits a fallback group they are not allowed to bind
- **THEN** the API rejects the request without changing the API key

#### Scenario: Duplicate or primary fallback group
- **WHEN** a submitted fallback list contains a duplicate ID or the primary group ID
- **THEN** the API rejects the request without changing the API key

#### Scenario: Cross-platform fallback group
- **WHEN** an OpenAI primary group is configured with an Anthropic, Gemini, Fal, or other non-OpenAI fallback group
- **THEN** the API rejects the request without changing the API key

#### Scenario: Too many fallback groups
- **WHEN** a submitted fallback list contains more than five group IDs
- **THEN** the API rejects the request without changing the API key

#### Scenario: Enterprise API key fallback configuration
- **WHEN** an API key is bound to an organization subscription and a fallback list is submitted
- **THEN** the API rejects the fallback configuration

### Requirement: Routing tries groups in configured order
For a personal API key, the system SHALL try the primary group first and then each configured same-platform fallback group in order when the current group is not billing-eligible or cannot select an available account that supports the requested model. Fallback order SHALL support transitions between metered and subscription groups within that platform and SHALL NOT select a group from another platform.

#### Scenario: Primary group has capacity
- **WHEN** the primary group can select an eligible account
- **THEN** the request uses the primary group and no fallback group is queried

#### Scenario: First fallback succeeds
- **WHEN** the primary group cannot select an eligible account and the first fallback can
- **THEN** the request uses the first fallback group

#### Scenario: Later fallback succeeds
- **WHEN** neither the primary group nor earlier fallback groups can select an eligible account
- **THEN** the system continues in order and uses the first later group that can select an eligible account

#### Scenario: Subscription primary falls back to metered group
- **WHEN** the primary subscription group has no active subscription or has exhausted its limits and a later metered group is eligible
- **THEN** the system skips the primary group and may serve the request from the metered group using personal balance billing

#### Scenario: Metered primary falls back to subscription group
- **WHEN** the metered primary group cannot select an eligible account and a later subscription group has an active subscription with remaining limits
- **THEN** the system may serve the request from the subscription group and charge its subscription usage

#### Scenario: Runtime data contains a cross-platform fallback
- **WHEN** stored or cached fallback data contains a group whose platform differs from the primary group
- **THEN** routing treats that candidate as unavailable and never queries or selects its accounts

#### Scenario: All groups exhausted
- **WHEN** no configured group can select an eligible account
- **THEN** the request returns the existing account-selection failure behavior after all candidates have been attempted

### Requirement: A selected fallback becomes the effective request group
Once routing selects a fallback group, the system MUST use that group consistently for the remainder of the request, including model mapping, account selection, group configuration, pricing, user-specific multiplier, rate limits, usage records, observability fields, and sticky-session metadata.

#### Scenario: Fallback request billing
- **WHEN** a request is served by fallback group B
- **THEN** its charge and usage log use group B rather than the API key primary group

#### Scenario: Fallback group rate limit
- **WHEN** fallback group B has a group-specific user rate limit
- **THEN** the request is evaluated against group B's limit before upstream execution

#### Scenario: Effective subscription context
- **WHEN** routing selects a subscription fallback group
- **THEN** downstream billing receives that group's active subscription rather than the primary group's subscription context

### Requirement: Fallback does not replay an upstream request
The system MUST NOT switch API-key fallback groups solely because an already-selected upstream account returns an ordinary HTTP or protocol error after request transmission.

#### Scenario: Upstream response error
- **WHEN** an account selected from the primary group returns an error after the request is sent
- **THEN** existing retry behavior may retry eligible accounts within the same effective group but does not advance to an API-key fallback group

### Requirement: Fallback changes invalidate authentication caches
The system SHALL include fallback group IDs in API key authentication snapshots and invalidate distributed and in-process authentication caches when the fallback list changes.

#### Scenario: Edited fallback configuration
- **WHEN** a user successfully edits an API key's fallback groups
- **THEN** later authenticated requests observe the new ordered list without waiting for the previous cache TTL

### Requirement: Personal console provides ordered fallback editing
The personal API key console SHALL let users add, remove, and reorder eligible fallback groups and SHALL expose the configured order when viewing a key.

#### Scenario: Reorder in the console
- **WHEN** a user moves group C ahead of group B and saves the API key
- **THEN** the console submits `[C, B]` and displays C before B after reload
