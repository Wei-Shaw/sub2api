# Batch Image MVP

Sub2API Batch Image MVP provides asynchronous Gemini image batch generation through a unified API surface backed by Redis workers, PostgreSQL state, and provider-specific batch backends.

Supported providers:

- `gemini_api`
- `vertex`

API users do not see Gemini file names, Vertex job names, GCS paths, signed URLs, API keys, or service account material. Downloads are proxied through Sub2API.

## API Routes

```text
POST   /v1/images/batches
GET    /v1/images/batches/{idREDACTED
GET    /v1/images/batches/{idREDACTED/items
GET    /v1/images/batches/{idREDACTED/items/{custom_idREDACTED/content
GET    /v1/images/batches/{idREDACTED/download
POST   /v1/images/batches/{idREDACTED/cancel
DELETE /v1/images/batches/{idREDACTED/outputs
```

Submit request:

```json
{
  "model": "gemini-2.5-flash-image",
  "provider": "gemini_api",
  "items": [
    {
      "custom_id": "cover_001",
      "prompt": "A clean product hero image..."
    REDACTED
  ],
  "image_size": "1K",
  "response_mime_type": "image/png"
REDACTED
```

Public batch response:

```json
{
  "id": "imgbatch_REDACTED",
  "object": "image.batch",
  "status": "queued",
  "model": "gemini-2.5-flash-image",
  "provider": "gemini_api",
  "item_count": 1,
  "success_count": 0,
  "fail_count": 0,
  "estimated_cost": 0.25,
  "actual_cost": null,
  "created_at": 1783123200,
  "submitted_at": 1783123201,
  "settled_at": null
REDACTED
```

Public items response:

```json
{
  "object": "list",
  "data": [
    {
      "custom_id": "cover_001",
      "status": "succeeded",
      "mime_type": "image/png",
      "file_extension": "png",
      "image_count": 1,
      "error": null
    REDACTED
  ],
  "has_more": false
REDACTED
```

## Lifecycle

Internal lifecycle:

```text
created -> uploading -> submitted -> running -> indexing -> settling -> completed
```

Terminal and cleanup statuses:

```text
failed
cancelled
completed -> output_deleted
```

Public status mapping:

```text
created/uploading/submitted -> queued
running                    -> running
indexing                   -> processing_results
settling                   -> settling
completed                  -> completed
failed                     -> failed
cancelled                  -> cancelled
output_deleted             -> output_deleted
```

`completed -> output_deleted` happens after manual output deletion or TTL cleanup.

## Redis

Redis is used for wakeups, retries, worker coordination, per-job locks, and download limiting. PostgreSQL remains the source of truth.

`batch_image.queue_enabled` defaults to `false`. When it is set to `true`, app startup starts `BatchImageWorker` runtime loops for the Redis ready queue, delayed queue mover, and stale active recovery. The worker reserves jobs from the Redis ready queue and blocks there when no job is available.

Redis structures:

- Ready queue: `batch_image.queue_ready_key`
- Delayed queue: `batch_image.queue_delayed_key`
- Active set: `batch_image.queue_active_key`
- Inflight keys: `batch_image.inflight_key_prefix`
- Per-job lock keys: `batch_image.lock_key_prefix`
- Queue idempotency keys: `batch_image.idempotency_key_prefix`
- Download limiter keys managed by the download limiter

Workers should reserve from Redis. They are not expected to run as a database scan loop.

The worker does not perform DB scan polling. Database reads happen only after a Redis queue reservation yields a specific batch id.

## Billing

MVP billing rules:

- Submit may estimate cost.
- Settlement runs after result indexing.
- Only successful images are charged.
- Failed items are not charged.
- Settlement request id is `batch_image_settlement:{batch_idREDACTED`.
- Settlement is idempotent; re-running settlement must not double charge.

Exact production pricing is resolved through model pricing configuration and is not defined here.

## Cleanup

Defaults:

- Input retention after terminal status: 24 hours.
- Output retention after terminal status: 72 hours.
- Maximum output retention: 7 days.
- Cleanup interval: 30 minutes.
- Cleanup batch size: 100.

Manual output deletion:

```text
DELETE /v1/images/batches/{idREDACTED/outputs
```

After output cleanup, downloads return `410 Gone` with `BATCH_IMAGE_OUTPUT_DELETED`.

Cleanup never accepts user-supplied provider paths. Provider cleanup must use server-generated refs and prefix-safe deletion.

For the managed Vertex/GCS batch bucket, disable Cloud Storage soft delete or configure lifecycle carefully to avoid hidden retained storage cost.

## Provider Notes

`gemini_api`:

- Uses Gemini Batch API with JSONL file mode.
- Result file refs are internal.
- API keys are never returned.

`vertex`:

- Uses Vertex `BatchPredictionJob` with managed GCS JSONL.
- GCS bucket and prefix are server-managed.
- Vertex job name and GCS paths are internal.
- Batch image output should be treated as `1K`/default only in MVP.
- Do not promise `2K` or `4K`.

## Config

These keys exist in `backend/internal/config/config.go`:

```yaml
batch_image:
  enabled: false
  max_items_per_job_default: 500
  max_items_per_job_trial: 50
  max_prompt_chars_per_item: 8000
  default_response_mime_type: "image/png"
  default_image_size: "1K"

  max_download_items_zip: 1000
  max_download_bytes_per_request: 2147483648
  max_download_duration_seconds: 600
  max_download_concurrency_per_user: 2

  input_retention_after_terminal_hours: 24
  output_retention_after_terminal_hours: 72
  output_retention_max_days: 7
  cleanup_interval_minutes: 30
  cleanup_batch_size: 100

  queue_enabled: false
  queue_ready_key: "batch_image:queue:ready"
  queue_delayed_key: "batch_image:queue:delayed"
  queue_active_key: "batch_image:queue:active"
  inflight_key_prefix: "batch_image:queue:inflight:"
  lock_key_prefix: "batch_image:queue:lock:"
  idempotency_key_prefix: "batch_image:queue:idem:"
  inflight_ttl_seconds: 604800
  job_lock_ttl_seconds: 300
  default_requeue_delay_seconds: 30
  error_retry_delay_seconds: 60
  lock_conflict_delay_seconds: 5
  stale_active_after_seconds: 600
  delayed_mover_interval_seconds: 5
  recovery_interval_seconds: 300
  delayed_move_limit: 100
  recover_limit: 100

  vertex_enabled: false
  vertex_project_id: ""
  vertex_location: "global"
  vertex_managed_gcs_bucket: ""
  vertex_managed_gcs_prefix: "batch-image/{envREDACTED/{batch_idREDACTED"
  vertex_input_retention_hours: 24
  vertex_output_retention_hours: 72
  vertex_batch_prediction_base_url: ""
  vertex_gcs_base_url: ""
```

Feature flags default to disabled.

## Operations Checklist

- Enable `batch_image.enabled`.
- Configure Redis.
- Enable `batch_image.queue_enabled` when workers should consume queue jobs.
- Configure provider accounts.
- Configure the Vertex managed GCS bucket if using Vertex.
- Ensure bucket permissions are correct.
- Disable or manage GCS soft delete.
- Configure cleanup worker settings.
- Configure max items per job.
- Configure download concurrency.
- Confirm billing pricing.
- Run smoke tests before enabling.

## Security Checklist

- No provider refs in public responses.
- No GCS URI exposure.
- No signed URL exposure.
- No service account exposure.
- No API key exposure.
- No image bytes/base64 in PostgreSQL.
- No base64 in logs.
- Owner-scoped status, item, download, cancel, and delete routes.
- Output deletion is owner-scoped.
- Cleanup paths are server-generated only.

## Test Commands

Core smoke and compile commands:

```bash
go test -tags=unit ./internal/service -run 'BatchImage' -count=1
go test -tags=unit ./internal/config ./internal/service ./internal/repository -count=1
go test ./internal/config ./internal/service ./internal/repository ./internal/handler ./internal/server/routes -run '^$'
go test ./... -run '^$'
```

These commands should not require Docker, testcontainers, Redis, GCP, Gemini, Vertex, or GCS.

## PR Hygiene Checklist

- Do not accidentally commit `rfcs/batch-image-issue-draft.md` unless maintainers explicitly want it.
- Keep migrations ordered: `159_batch_image_foundation.sql`, then `160_batch_image_provider_refs.sql`, then later migrations.
- Include generated Ent code if generated code is committed in this repository.
- Keep generated server and wire files updated.
- Keep feature flags disabled by default unless maintainers ask otherwise.
- Do not commit real secrets, API keys, service account JSON, or local machine paths.
- Keep fixtures tiny and fake; no real cloud refs or credentials.
- Do not add new public routes, providers, dashboards, queues, or billing behavior in this stabilization PR.
