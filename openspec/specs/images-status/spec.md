# images-status Specification

## Purpose

Tracks short-lived image generation status by client request ID and exposes an API-key-authenticated status/result query endpoint for both Images API and Responses API image generation.

## Requirements

### Requirement: Track image status by client request ID
The system SHALL create a short-lived image generation status record when a supported image generation request includes the `x-client-request-id` header. Supported producers SHALL include `POST /v1/images/generations` and `POST /v1/responses` requests that perform image generation. The status record key MUST be `gen_img:<request_id>`, where `<request_id>` is the header value after normal request ID normalization.

#### Scenario: Images request with client request ID starts tracking
- **WHEN** a client sends an image generation request to `/v1/images/generations` with `x-client-request-id: img-123`
- **THEN** the system SHALL store image status under `gen_img:img-123`
- **AND** the status record SHALL expire after 7 days

#### Scenario: Responses request with client request ID starts tracking
- **WHEN** a client sends an image generation request to `/v1/responses` with `x-client-request-id: img-123`
- **THEN** the system SHALL store image status under `gen_img:img-123`
- **AND** the status record SHALL expire after 7 days

#### Scenario: Request without client request ID is not tracked
- **WHEN** a client sends a supported image generation request without `x-client-request-id`
- **THEN** the system SHALL NOT be required to create a `gen_img:` status record

#### Scenario: Duplicate request ID overwrites status
- **WHEN** a client sends another tracked image generation request with an existing `x-client-request-id`
- **THEN** the system SHALL allow the later status writes to overwrite the existing `gen_img:<request_id>` value

### Requirement: Use unscoped status keys
The system MUST NOT include API key, user, group, account, or subscription identifiers in the `gen_img:<request_id>` cache key. API key validation SHALL be used only to authorize access to the status query endpoint.

#### Scenario: Status key omits API key information
- **WHEN** the system stores status for `x-client-request-id: img-123`
- **THEN** the cache key SHALL be exactly `gen_img:img-123`
- **AND** the key SHALL NOT contain any API key identifier or API key value

#### Scenario: Valid API key can query by request ID only
- **WHEN** a client calls `GET /v1/images/status/?request_id=img-123` with any valid API key
- **THEN** the system SHALL look up `gen_img:img-123` without enforcing API-key ownership of that status record

### Requirement: Expose batch image status query endpoint
The system SHALL expose `GET /v1/images/status/` for querying short-lived image generation status. The endpoint MUST require a valid API key and MUST support batch lookups.

#### Scenario: Query existing status
- **WHEN** a client calls `GET /v1/images/status/?request_id=img-123` with a valid API key
- **AND** `gen_img:img-123` exists
- **THEN** the system SHALL return HTTP 200 with the stored status payload in `data`

#### Scenario: Query multiple statuses
- **WHEN** a client calls `GET /v1/images/status/?request_ids=img-123,img-456` with a valid API key
- **THEN** the system SHALL return HTTP 200 with found status payloads in `data`
- **AND** missing or expired IDs, if any, SHALL be returned in `not_found`

#### Scenario: Query missing or expired single status
- **WHEN** a client calls `GET /v1/images/status/?request_id=img-123` with a valid API key
- **AND** `gen_img:img-123` does not exist or has expired
- **THEN** the system SHALL return HTTP 404

#### Scenario: Query without valid API key
- **WHEN** a client calls `GET /v1/images/status/?request_id=img-123` without a valid API key
- **THEN** the system SHALL reject the request with the existing API-key authentication error behavior

### Requirement: Publish lifecycle status and progress
The system SHALL publish coarse lifecycle state and progress for tracked image generation requests. The payload MUST include `request_id`, `status`, `progress`, `created_at`, and `updated_at`.

#### Scenario: Generation is in progress
- **WHEN** a tracked image generation request has been accepted or forwarded upstream
- **THEN** the status payload SHALL report a non-terminal status such as `accepted` or `running`
- **AND** `progress` SHALL be less than 100

#### Scenario: Upstream generation completes before COS upload
- **WHEN** upstream image generation has completed and COS upload has not completed
- **THEN** the status payload SHALL report a non-terminal status such as `upstream_done` or `cos_uploading`
- **AND** `progress` SHALL be less than 100

#### Scenario: Generation fails
- **WHEN** a tracked image generation request fails before producing a successful image result
- **THEN** the status payload SHALL report `status: failed`
- **AND** `progress` SHALL be 100
- **AND** the payload SHALL include error information

### Requirement: Publish final result URLs
The system SHALL include final result URL information in the status payload when tracked image generation reaches a terminal success state. If COS upload succeeds, the status payload MUST include the COS URL or URLs.

#### Scenario: Success with COS URLs
- **WHEN** a tracked image generation request succeeds
- **AND** generated image output is uploaded to COS successfully
- **THEN** the status payload SHALL report `status: succeeded`
- **AND** `progress` SHALL be 100
- **AND** `cos_urls` SHALL contain the uploaded COS URL or URLs

#### Scenario: Success without COS URLs
- **WHEN** a tracked image generation request succeeds
- **AND** COS is disabled, unavailable, or upload fails
- **THEN** the status payload SHALL report a terminal success state
- **AND** `progress` SHALL be 100
- **AND** `cos_urls` SHALL be empty or omitted
- **AND** available fallback result URLs SHALL be exposed when the system has them

### Requirement: Document the Images status API
The system SHALL provide API documentation under `api_docs/` for image status tracking and querying.

#### Scenario: Documentation describes tracking and query behavior
- **WHEN** the change is implemented
- **THEN** `api_docs/` SHALL contain documentation for the `x-client-request-id` tracking header, `GET /v1/images/status/`, authentication requirements, batch query parameters, response fields, status values, 7-day TTL, duplicate overwrite behavior, and missing-status errors
