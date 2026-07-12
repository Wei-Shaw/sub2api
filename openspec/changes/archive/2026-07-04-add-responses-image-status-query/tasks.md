## 1. Status Store

- [x] 1.1 Define the Responses image status payload type with request ID, status, progress, URLs, COS URLs, error, created_at, and updated_at fields.
- [x] 1.2 Add a Redis/cache-backed status repository or service with `gen_img:<request_id>` key construction and 7-day TTL writes.
- [x] 1.3 Ensure status writes overwrite existing values for the same request ID and treat write failures as best-effort non-fatal errors.
- [x] 1.4 Add unit tests for key construction, TTL, overwrite behavior, serialization, missing values, and cache errors.

## 2. Responses Lifecycle Integration

- [x] 2.1 Extract the normalized `x-client-request-id` from tracked `/v1/responses` image generation requests.
- [x] 2.2 Write initial `accepted` or `running` status before forwarding tracked image generation requests upstream.
- [x] 2.3 Update tracked status to failed with error details when validation, moderation, billing, routing, forwarding, or failover exhaustion prevents a successful image result.
- [x] 2.4 Update tracked status after upstream image output is observed, including available fallback result URLs when present.
- [x] 2.5 Update the COS upload path so tracked image status enters `cos_uploading` and then `succeeded` with `cos_urls` when upload completes.
- [x] 2.6 Ensure success without COS upload still reaches a terminal success status with progress 100 and empty or omitted `cos_urls`.

## 3. Query API

- [x] 3.1 Add `GET /v1/responses/image-status/:request_id` under the existing `/v1` gateway route set.
- [x] 3.2 Reuse existing API-key authentication for the query endpoint.
- [x] 3.3 Query only `gen_img:<request_id>` after authentication and do not enforce API-key ownership matching.
- [x] 3.4 Return HTTP 200 with the status payload when present and HTTP 404 when missing or expired.
- [x] 3.5 Add handler/route tests for valid query, missing status, expired status, invalid API key, and cross-key lookup.

## 4. API Documentation

- [x] 4.1 Create the `api_docs/` directory if it does not already exist.
- [x] 4.2 Document `x-client-request-id` tracking behavior for Responses image generation.
- [x] 4.3 Document `GET /v1/responses/image-status/:request_id`, authentication, response schema, status values, progress semantics, TTL, overwrite behavior, and error responses.
- [x] 4.4 Include examples for a successful COS-backed result, an in-progress result, a failed result, and a missing or expired result.

## 5. Verification

- [x] 5.1 Run focused backend tests covering the new status store and query handler.
- [x] 5.2 Run relevant OpenAI Responses/COS upload tests to verify existing image delivery and usage logging behavior still works.
- [x] 5.3 Run `openspec status --change "add-responses-image-status-query"` and confirm the change is ready for implementation.
