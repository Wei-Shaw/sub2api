## ADDED Requirements

### Requirement: Codex Official Clients Use Header-Based Thread Identity

The system SHALL treat `session_id` or `conversation_id` as the only thread-identity signals for Codex official client relay. It SHALL NOT promote `prompt_cache_key` into upstream `session_id` or Codex sticky-session identity.

#### Scenario: Prompt cache key does not become Codex session identity

- **GIVEN** a Codex official client request without `session_id` and without `conversation_id`
- **AND** the request contains `prompt_cache_key`
- **WHEN** the gateway builds the upstream Codex WS request
- **THEN** it does not synthesize `session_id` from `prompt_cache_key`
- **AND** it does not use `prompt_cache_key` as the Codex sticky-session key

### Requirement: Codex Official Clients Use Turn-Scoped WS State

The system SHALL scope `x-codex-turn-state` and upstream websocket connection affinity to a single Codex turn.

#### Scenario: Next turn does not reuse previous turn state

- **GIVEN** a Codex official client completes one turn and the upstream handshake returns `x-codex-turn-state`
- **WHEN** the client starts the next turn
- **THEN** the gateway opens a fresh upstream websocket connection
- **AND** it does not send the previous turn's `x-codex-turn-state`
- **AND** it does not preflight-ping the previous turn's upstream connection as a prerequisite

### Requirement: Codex Official Clients Preserve Previous Response Semantics

The system SHALL preserve the client's `previous_response_id` semantics for Codex official requests and SHALL NOT silently rewrite the request into a full replay when upstream reports `previous_response_not_found`.

#### Scenario: Previous response missing is returned instead of auto-rewritten

- **GIVEN** a Codex official client request includes `previous_response_id`
- **AND** upstream returns `previous_response_not_found`
- **WHEN** the gateway handles the error
- **THEN** it returns the upstream error to the client
- **AND** it does not retry after removing `previous_response_id`

### Requirement: Non-Codex Compatibility Path Remains Unchanged

The system SHALL keep the existing compatibility behaviors for non-Codex clients unless those behaviors are explicitly modified by another change.

#### Scenario: Generic client still uses existing compatibility logic

- **GIVEN** a non-Codex client request reaches the OpenAI Responses gateway
- **WHEN** the request is forwarded
- **THEN** the gateway continues using the existing compatibility path
- **AND** the Codex high-fidelity constraints above do not apply
