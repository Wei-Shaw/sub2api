## Context

Responses image generation is handled through the OpenAI-compatible gateway. The request is synchronous from the client's point of view, but generated image base64 payloads can be uploaded to COS after the response has already been written. Usage recording waits briefly for that COS upload so usage logs can contain `cos_urls`, but clients do not currently have a short-lived way to query those COS URLs by their own request identifier.

The fal native async status endpoints are close in shape, but they are tied to persisted `async_media_tasks`, upstream fal request IDs, and API-key ownership checks. This change needs a lighter status projection for Responses image generation keyed only by `x-client-request-id`, with API key validation used only as an access gate for the query endpoint.

## Goals / Non-Goals

**Goals:**
- Track Responses image-generation state when the inbound request includes `x-client-request-id`.
- Store status under `gen_img:<request_id>` with a 7-day TTL and no API-key component.
- Allow later status/result lookup through `GET /v1/responses/image-status/:request_id` after validating that the caller has a valid API key.
- Include status, progress, original/fallback URLs when available, and COS URLs after successful upload.
- Add API documentation under `api_docs/`.

**Non-Goals:**
- Do not make Responses image generation fully asynchronous.
- Do not reuse fal persisted async task ownership semantics.
- Do not guarantee long-term result storage beyond the 7-day status TTL.
- Do not prevent overwrites for duplicate client request IDs.
- Do not expose base64 image payloads through the status query.

## Decisions

1. Use a dedicated short-lived status projection.

   Store a compact JSON status object in the existing Redis/cache layer rather than adding a database table or reusing `async_media_tasks`. The status record is operational delivery state, not billing source of truth. It can expire after 7 days without affecting usage logs or accounting.

   Alternative considered: reuse fal native task APIs. Rejected because fal tasks are model/upstream-request oriented, persisted, and owner-scoped, while this feature intentionally uses client-provided request IDs and loose query semantics.

2. Key status records as `gen_img:<request_id>`.

   The key MUST NOT include API key ID, user ID, or API key value. If clients reuse a request ID, later writes overwrite earlier status. This matches the desired loose query behavior and keeps the client responsible for uniqueness.

   Alternative considered: namespace by API key. Rejected because it would make query stricter than intended and force clients to remember the same credential used for generation.

3. Gate the query endpoint with API-key auth only.

   The endpoint should live at `GET /v1/responses/image-status/:request_id` and reuse the existing API-key middleware so invalid keys are rejected. After auth succeeds, lookup is by `gen_img:<request_id>` only; there is no owner check.

   Alternative considered: public unauthenticated lookup. Rejected because even loose status lookup should require permission to use the service.

4. Update status around the Responses/COS lifecycle.

   Suggested states are `accepted`, `running`, `upstream_done`, `cos_uploading`, `succeeded`, and `failed`. Progress can be coarse and monotonic, for example 0, 25, 70, 85, and 100. The implementation should write `accepted`/`running` before forwarding, transition to `upstream_done` after image output is detected, transition to `cos_uploading` when COS upload starts, and finish with `succeeded` or `failed`.

   The COS upload completion hook should update the status with `cos_urls` when upload succeeds. If COS is disabled or upload fails, the record can still reach a terminal state with available fallback URLs and empty `cos_urls`.

5. Keep the response schema explicit and stable.

   Return a JSON object with at least `request_id`, `status`, `progress`, `urls`, `cos_urls`, `error`, `created_at`, and `updated_at`. `urls` are non-COS result URLs when the system has them; `cos_urls` are populated only for successful COS uploads. `error` is present for failed states and omitted or null otherwise.

## Risks / Trade-offs

- [Risk] Looser lookup means any valid API key holder who knows a request ID can query its short-lived status. -> Mitigation: require high-entropy client request IDs in docs and keep the TTL limited to 7 days.
- [Risk] Duplicate request IDs overwrite earlier records. -> Mitigation: document overwrite behavior and client responsibility for uniqueness.
- [Risk] COS upload currently runs after the client response is written, so a first status query may see `upstream_done` or `cos_uploading` before `cos_urls` are available. -> Mitigation: expose progress/status clearly and make polling expected.
- [Risk] Cache write failures should not break image generation. -> Mitigation: status writes should be best-effort; generation and billing remain authoritative elsewhere.
- [Risk] Streaming Responses image output may have partial lifecycle visibility. -> Mitigation: capture terminal image output from the existing image-output counters and mark status based on observed final result.

## Migration Plan

- Add the cache-backed status store and wire it into the service/handler graph.
- Add lifecycle updates in the Responses image generation path and COS upload completion path.
- Add the `GET /v1/responses/image-status/:request_id` route with existing API-key auth and no owner filtering.
- Add focused unit/integration tests for key construction, TTL, auth behavior, missing status, success with COS URLs, fallback without COS URLs, failure status, and duplicate overwrite behavior.
- Add `api_docs/` documentation for the new header and query endpoint.
- Rollback is safe by removing the new route and lifecycle writes; expired cache records disappear naturally.

## Open Questions

- None. The current proposal intentionally chooses loose API-key-gated lookup, unscoped `gen_img:<request_id>` keys, overwrite-on-duplicate behavior, and 7-day retention.
