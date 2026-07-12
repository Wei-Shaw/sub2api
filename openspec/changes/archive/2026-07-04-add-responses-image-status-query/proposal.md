## Why

Responses image generation currently returns image data synchronously, while COS archival completes through asynchronous usage-recording work. Clients that lose the original response or want to poll for the archived image URL have no lightweight, API-key-authenticated way to query the short-lived delivery state for a known `x-client-request-id`.

## What Changes

- Track Responses image-generation status when the request includes `x-client-request-id`.
- Store the status under `gen_img:<x-client-request-id>` with a 7-day TTL; keys are intentionally not scoped by API key, and clients are responsible for uniqueness.
- Update status as the request moves through generation, upstream completion, COS upload, success, and failure states.
- Expose `GET /v1/responses/image-status/:request_id` for clients to query short-lived image status and result URLs.
- Require a valid API key on the query endpoint, but do not restrict status lookup by API key ownership.
- Add public API documentation under `api_docs/` for request headers, status query, response schema, status meanings, TTL, and error behavior.

## Capabilities

### New Capabilities
- `responses-image-status`: Tracks short-lived Responses image generation state keyed by client request ID and exposes an API-key-authenticated status/result query endpoint.

### Modified Capabilities
- None.

## Impact

- Backend gateway routing for `/v1/responses/image-status/:request_id`.
- OpenAI Responses image-generation lifecycle and COS upload completion hooks.
- Cache/Redis usage for 7-day `gen_img:<request_id>` status entries.
- API key authentication middleware on the status endpoint.
- API documentation in `api_docs/`.
