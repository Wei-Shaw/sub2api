## ADDED Requirements

### Requirement: Image stream resilience
The system SHALL keep image generation stream processing active after downstream client disconnects so long as upstream reading can continue, in order to preserve final image outputs and billing results.

#### Scenario: Images API stream survives downstream disconnect
- **WHEN** `/v1/images/generations` is streamed to a client
- **AND** the downstream writer returns an error before the upstream stream completes
- **THEN** the service continues draining the upstream stream
- **AND** it still counts final image outputs if the upstream later emits them
- **AND** the request can still complete with image billing metadata

#### Scenario: Responses image tool stream survives downstream disconnect
- **WHEN** a `/v1/responses` request uses `image_generation` and is streamed to a client
- **AND** the downstream writer returns an error before the upstream stream completes
- **THEN** the service continues draining the upstream stream
- **AND** it still counts final image outputs if the upstream later emits them
- **AND** the request can still complete with image billing metadata

#### Scenario: Client disconnect does not force image stream to downgrade to text billing
- **WHEN** a successful image stream request has already produced final image outputs
- **AND** the downstream client disconnects before the final flush
- **THEN** the request remains billed as an image request
- **AND** the image count is preserved in the forward result

### Requirement: Image stream timeout isolation
The system SHALL use image-specific streaming timeout settings for image generation stream paths, and these settings SHALL be independent from the ordinary text streaming timeout values.

#### Scenario: Image stream uses dedicated timeout defaults
- **WHEN** an image generation stream path is executed
- **THEN** it uses the image-specific data interval timeout and keepalive interval defaults
- **AND** it does not rely on the ordinary text stream timeout defaults

#### Scenario: Ordinary stream settings remain unchanged
- **WHEN** a normal non-image streaming request is executed
- **THEN** the existing ordinary stream timeout configuration and behavior remain unchanged

#### Scenario: Image stream timeout is longer than ordinary stream timeout
- **WHEN** the image streaming timeout defaults are compared with the ordinary streaming defaults
- **THEN** the image streaming timeout is configured to allow a longer wait window than ordinary text streaming

### Requirement: Image stream billing consistency
The system SHALL keep the image billing result consistent even when image stream handling uses retries, keepalive writes, or downstream disconnect recovery.

#### Scenario: Final image count is preserved after reconnect-unsafe downstream failure
- **WHEN** the downstream client disconnects after at least one final image output has been observed upstream
- **THEN** the forward result retains the final image count
- **AND** usage recording can still proceed with image billing metadata

#### Scenario: Image stream timeout does not silently switch billing mode
- **WHEN** an image stream times out before any final image output is observed
- **THEN** the request is handled as a failed image stream
- **AND** it does not fall back to ordinary text billing semantics
