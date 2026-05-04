# image-generation-concurrency-isolation Specification

## ADDED Requirements

### Requirement: Image concurrency isolation is opt-in

The system SHALL keep image concurrency isolation disabled by default.

#### Scenario: default config keeps existing behavior
- **GIVEN** the deployment does not set `gateway.image_concurrency.enabled`
- **WHEN** image generation requests are received
- **THEN** no new image-specific concurrency limit is applied
- **AND** existing user/account concurrency and billing behavior remains unchanged

### Requirement: Dedicated image concurrency limit

The system SHALL provide an opt-in service-level image concurrency limit controlled by gateway configuration.

#### Scenario: explicit image endpoint is limited
- **GIVEN** `gateway.image_concurrency.enabled=true`
- **AND** `gateway.image_concurrency.max_concurrent_requests=1`
- **AND** one image generation request is already active
- **WHEN** another `/v1/images/generations` or `/v1/images/edits` request arrives
- **THEN** the second request is rejected with HTTP `429`
- **AND** the error type is `rate_limit_error`

#### Scenario: explicit Responses image generation request is limited
- **GIVEN** `gateway.image_concurrency.enabled=true`
- **AND** `gateway.image_concurrency.max_concurrent_requests=1`
- **AND** `gateway.image_concurrency.overflow_mode=reject`
- **AND** one image generation request is already active
- **WHEN** a `/v1/responses` request explicitly contains `tools[].type=image_generation`, an image model, or `tool_choice` selecting `image_generation`
- **THEN** the request is rejected with HTTP `429`
- **AND** it is not retried through account failover

#### Scenario: image request waits for a slot
- **GIVEN** `gateway.image_concurrency.enabled=true`
- **AND** `gateway.image_concurrency.max_concurrent_requests=1`
- **AND** `gateway.image_concurrency.overflow_mode=wait`
- **AND** `gateway.image_concurrency.wait_timeout_seconds` is greater than zero
- **AND** one image generation request is already active
- **WHEN** another explicit image generation request arrives
- **AND** the active image generation request releases its slot before the wait timeout
- **THEN** the waiting image generation request acquires the slot and continues

#### Scenario: image wait times out
- **GIVEN** `gateway.image_concurrency.enabled=true`
- **AND** `gateway.image_concurrency.max_concurrent_requests=1`
- **AND** `gateway.image_concurrency.overflow_mode=wait`
- **AND** one image generation request is already active
- **WHEN** another explicit image generation request waits longer than `gateway.image_concurrency.wait_timeout_seconds`
- **THEN** the waiting request is rejected with HTTP `429`
- **AND** the error type is `rate_limit_error`

#### Scenario: image waiting queue is full
- **GIVEN** `gateway.image_concurrency.enabled=true`
- **AND** `gateway.image_concurrency.overflow_mode=wait`
- **AND** `gateway.image_concurrency.max_waiting_requests` is already reached
- **WHEN** another explicit image generation request arrives
- **THEN** the request is rejected with HTTP `429`
- **AND** it does not wait for account scheduling

### Requirement: Text requests are not image-limited

The system SHALL NOT apply the image concurrency limit to requests without explicit image generation intent.

#### Scenario: normal coding request bypasses image limit
- **GIVEN** `gateway.image_concurrency.enabled=true`
- **AND** the image concurrency limit is full
- **WHEN** a `/v1/responses` request uses a text model and does not explicitly contain image generation intent
- **THEN** the image concurrency limiter does not reject it
- **AND** normal user/account concurrency handling continues

### Requirement: External image gateway remains a deployment pattern

The system SHALL document external image gateway routing as a deployment option without adding runtime forwarding code in this change.

#### Scenario: operator reads local design note
- **GIVEN** the repository documentation is available
- **WHEN** an operator evaluates isolating image traffic into a separate service
- **THEN** local `2ue` documentation describes which paths are safe to route by path
- **AND** explains why `/v1/responses` image tool requests require body-aware routing or main-gateway fallback
