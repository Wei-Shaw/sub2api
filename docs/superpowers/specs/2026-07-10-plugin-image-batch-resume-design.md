# Plugin Image Batch Resume Design

## Goal

Change the image-generation plugin's single-image, text-to-image flow to reuse the main service batch image API. A submitted image remains recoverable across page refreshes and plugin restarts. Pausing stops local status tracking; resuming continues tracking the same main-service batch and never submits a duplicate generation request.

This design does not claim that upstream image generation itself can be paused. The main-service batch continues running while plugin tracking is paused.

## Scope

The first implementation covers one text-to-image request with one output image. Requests containing reference images continue to use the existing synchronous image-edit path. Existing successful and failed history records remain readable.

The implementation adds plugin-level pause and resume behavior, main-service batch submission and reconciliation, persisted batch identity, result normalization, and focused backend/frontend tests. It does not change the main-service batch API, its billing rules, or its terminal cancellation semantics.

## Architecture

The plugin backend remains the owner of plugin history and acts as a small adapter around the main-service batch API. The selected `provider_api_key` is already a main-service API key and is used as the bearer token for batch submission and queries. `PLUGIN_MAIN_SERVICE_BASE_URL` remains the preferred main-service origin.

For a request without reference images, the backend creates a pending history record and submits one item to `POST /v1/images/batches`. The item uses a stable custom ID derived from the plugin history ID, `output_count: 1`, and an idempotency key derived from the same history ID. The returned batch ID is persisted before the response is returned to the browser.

The initial plugin response is asynchronous and contains the plugin history ID, pending status, and main-service batch status. The browser polls a plugin-owned status endpoint rather than calling the main service directly. This keeps history authorization and result mapping in one place.

## Persisted Data

No schema migration is required. The existing history JSON fields store batch tracking data:

- `request.batch_id`: main-service batch identifier.
- `request.batch_custom_id`: the single item custom ID.
- `request.tracking_paused`: whether the plugin should actively poll this task.
- `request.provider_api_key`: retained server-side for retry/query operations and removed by existing response sanitization.
- `result.batch_status`: latest observed main-service status.
- `result.images`: normalized image output after completion, preserving the existing creation/history rendering contract.

Plugin history gains a `paused` status. Existing statuses retain their current meanings. The persisted `batch_id`, rather than an in-memory worker, is the recovery point after restart.

## API Behavior

`POST /plugins/image-generation/api/generate` keeps the existing request shape. For text-to-image it returns `201` after the main service accepts the batch, without waiting for image completion.

`GET /plugins/image-generation/api/history/{id}/status` reconciles one pending or paused history record with the main service. For a paused record it returns the saved state without contacting the main service unless the request is an explicit resume operation.

`POST /plugins/image-generation/api/history/{id}/pause` changes a pending record to paused and sets `tracking_paused`. It does not call the main-service cancel endpoint.

`POST /plugins/image-generation/api/history/{id}/resume` clears `tracking_paused`, changes the record to pending, and immediately performs one reconciliation using the existing batch ID.

`POST /plugins/image-generation/api/history/{id}/cancel` calls the main-service batch cancel endpoint when a batch ID exists. Cancellation remains terminal and cannot be resumed. Legacy pending records without a batch ID retain the current local cancellation behavior.

`POST /plugins/image-generation/api/history/{id}/retry` always creates a new history record and a new batch. It is distinct from resume and cannot reuse a failed or cancelled batch.

## State Mapping

Main-service nonterminal states such as validating, queued, running, and settling map to plugin `pending`. A locally paused record remains `paused` even if the main-service work advances; resume or explicit reconciliation discovers its latest state.

Main-service `completed` maps to plugin `succeeded` after the plugin retrieves the single item output and normalizes it into the existing `image_generation` result. Main-service `failed` maps to plugin `failed`, preserving a public error message. Main-service `cancelled` maps to plugin `canceled`.

Reconciliation updates are idempotent. A terminal plugin history record is never moved back to pending or paused. Concurrent status requests may repeat reads but must produce the same terminal result.

## Result Retrieval

When the batch completes, the adapter lists the batch items and verifies the expected custom ID succeeded. It then downloads or resolves the batch output using the main-service download contract and converts the one image into the existing result shape:

```json
{
  "type": "image_generation",
  "provider": "batch",
  "model": "requested-model",
  "size": "requested-size",
  "images": [
    {
      "url": "resolved-output-url-or-data-url",
      "b64_json": "optional-base64-content",
      "revised_prompt": ""
    }
  ]
}
```

The adapter must use the actual main-service download response format discovered during implementation and must not invent a public URL when only binary output is available.

## Frontend Flow

After submission, the waiting assistant message stores the returned history ID and polls the plugin status endpoint while the task is pending. Polling uses a bounded interval and stops on terminal status, component teardown, network loss, or user pause.

The pending message exposes pause and cancel actions. A paused message exposes resume and cancel actions. Resume reuses the same history ID and batch ID. When history is loaded after refresh, pending tasks restart polling and paused tasks remain paused until the user resumes them.

Transient polling errors keep the task visible and allow later polling; they do not mark generation failed. Only a terminal main-service failure changes the history to failed.

## Error Handling and Security

Batch submission failures mark the newly created history record failed unless the outcome is ambiguous. The stable idempotency key permits a safe retry of an ambiguous submission without creating a second batch.

Main-service authorization failures are returned as actionable plugin errors without exposing the API key. Existing history sanitization continues to remove `provider_api_key`. Logs include history and batch identifiers but never bearer tokens or image payloads.

Missing batches, malformed item results, and unexpected terminal states become explicit failed history records with bounded public error messages. Network timeouts during reconciliation leave nonterminal records unchanged.

## Testing

Backend tests cover the request body and idempotency key for a one-item batch, persistence of `batch_id`, nonblocking generate responses, pause without upstream cancellation, resume using the same batch, terminal status mapping, result normalization, cancellation, retry creating a new batch, authorization isolation, and transient reconciliation failures.

Handler/router tests cover the new status, pause, and resume routes plus response sanitization. Existing synchronous reference-image tests remain unchanged.

Frontend tests cover pending polling, pause stopping polls, resume continuing the same history task, refresh recovery, terminal rendering, transient query errors, and cancel being non-resumable.

## Acceptance Criteria

A single text-to-image request is submitted through `POST /v1/images/batches` with one item and one output. The generate call returns before image completion. The plugin persists the batch ID, survives restart, can pause tracking, and resumes against the same batch without duplicate generation or duplicate charging. Completion renders and stores the image using the current history/creation contract. Reference-image generation continues to work through the existing synchronous path.
