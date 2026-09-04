## ADDED Requirements

### Requirement: Atomic account provisioning

The system SHALL keep every new or imported account unschedulable until credentials, groups, proxies, system-managed seed, identity policy, profiles and slots have been validated and committed atomically.

#### Scenario: Concurrent request during import

- **WHEN** an account import transaction has not committed
- **THEN** database, Redis scheduler cache and sticky lookup SHALL NOT return the account as schedulable

#### Scenario: One account fails in a bulk import

- **WHEN** one row has an invalid Profile or proxy
- **THEN** that account SHALL leave no active account, group, policy, slot, binding or outbox record while other valid rows MAY commit independently

### Requirement: OS Profile policy

The system SHALL support Windows, macOS, Linux and Generic profiles with validated canonical surfaces, architectures, slot counts, session policy and proxy inheritance. Linux Desktop SHALL be supported.

#### Scenario: Cross-OS mismatch

- **WHEN** a Profile-managed Linux request has no compatible Linux account in a fully Profile-managed candidate pool
- **THEN** the system SHALL return `DEVICE_PROFILE_UNSUPPORTED` and SHALL NOT route it through a Windows or macOS Adapter

#### Scenario: Mixed legacy and Profile pool

- **WHEN** an account remains in the default-off identity mode and the conversation has not established Profile affinity
- **THEN** that account SHALL retain legacy scheduling and identity behavior
- **AND WHEN** a conversation has established Profile affinity
- **THEN** failover SHALL NOT downgrade it to a default-off account

#### Scenario: Explicit nested direct connection

- **WHEN** a Profile or device slot selects `proxy_mode=direct` while the account has a default proxy
- **THEN** requests resolved through that Profile or slot SHALL connect directly and SHALL NOT inherit the account proxy
- **AND** `proxy_mode=inherit` SHALL remain distinguishable from explicit direct routing after save, import and proxy-expiry fallback

### Requirement: Stable device slots

The system SHALL bind an authenticated API Key and OS Profile to a stable device slot within the selected OAuth account and derive identity from the account seed, Profile, slot and epoch.

#### Scenario: Shared device without shared state

- **WHEN** two API Keys bind to the same device slot
- **THEN** their installation identity MAY match but their HTTP/TLS pool, WS pool, response state and turn state SHALL remain isolated

### Requirement: Configurable session identity

The system SHALL implement conversation-isolated, API-Key-shared, session-pool and device-shared session policies with validated concurrency restrictions.

#### Scenario: Default policy

- **WHEN** the new identity mode is enabled without an explicit session policy
- **THEN** the system SHALL use conversation-isolated semantics

#### Scenario: Device-shared active capacity

- **WHEN** `device_shared` is selected
- **THEN** each physical device slot SHALL admit at most one in-progress HTTP/SSE request stream or one WebSocket session at a time
- **AND** the slot SHALL become available immediately when that request or session ends
- **AND** a downstream disconnect SHALL NOT release the slot while the host is still draining the upstream response for billing
- **AND** this restriction SHALL NOT reduce the whole account to one concurrent request
- **AND** the other session policy modes SHALL NOT acquire this extra slot mutex

#### Scenario: Session-pool size

- **WHEN** `session_pool.sessions_per_device` is configured
- **THEN** the value SHALL determine the number of reusable upstream session identities per device
- **AND** it SHALL NOT be interpreted as a request-concurrency limit

### Requirement: Per-slot Codex client version

The system SHALL let every device slot either inherit the deployment Codex client version or pin one validated Codex client version without changing its model, client family or Desktop application build.

#### Scenario: Inherited version

- **WHEN** a slot uses `client_version_mode=inherit`
- **THEN** the effective version SHALL follow the global administrator override, then the synced stable version, then the built-in fallback

#### Scenario: Pinned version

- **WHEN** a slot uses `client_version_mode=pinned`
- **THEN** `client_version` SHALL be required, syntactically valid and no lower than `0.144.0`
- **AND** the same effective value SHALL drive the User-Agent version segment, `version` header and structured client/turn metadata

#### Scenario: Version change

- **WHEN** a slot's version mode or pinned version changes
- **THEN** the affected Profile epoch SHALL advance and its previous slots SHALL drain
- **AND** `catalog_version` SHALL remain the identity-fixture format version rather than the Codex client version

#### Scenario: Global inherited-version refresh

- **WHEN** the deployment-wide Codex version changes while a slot remains in `client_version_mode=inherit`
- **THEN** the next attempt SHALL use the new validated version
- **AND** an already running attempt SHALL keep the version captured when it was prepared
- **AND** the slot's device identity and Profile epoch SHALL remain stable
- **AND** no account-wide epoch fan-out SHALL be required

### Requirement: Structured identity round trip

The system SHALL transform and restore identity only through known JSON fields and HTTP/SSE/WS protocol events.

#### Scenario: Model text contains an alias

- **WHEN** ordinary model output text happens to contain a generated alias
- **THEN** the text SHALL remain unchanged

### Requirement: Profile-aware affinity and failover

The system SHALL use an independent sticky namespace containing API Key scope, OS Profile, conversation and policy version.

#### Scenario: Later-turn 429

- **WHEN** a later turn receives a retryable 429 before client output
- **THEN** the system SHALL replay the current turn on a compatible account, update the Profile binding, clear old account state and avoid automatic rebound for the affinity TTL

### Requirement: Backward compatibility

The new mode SHALL be default-off and SHALL NOT change existing `off/device/session/full` behavior or PR #2 gateway runtime settings.

#### Scenario: Existing account after upgrade

- **WHEN** an existing account has no OS Profile identity policy
- **THEN** the account SHALL retain its existing fingerprint, scheduling, HTTP/WS isolation and gateway runtime behavior
