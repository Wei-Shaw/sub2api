# Batch Image MVP

Sub2API Batch Image MVP provides asynchronous Gemini image batch generation through a unified API surface backed by Redis workers, PostgreSQL state, and provider-specific batch backends.

Supported providers:

- `gemini_api`
- `vertex`

API users do not see Gemini file names, Vertex job names, GCS paths, signed URLs, API keys, or service account material. Downloads are proxied through Sub2API in this MVP.

## API Routes

```text
POST   /v1/images/batches
GET    /v1/images/batches/{id}
GET    /v1/images/batches/{id}/items
GET    /v1/images/batches/{id}/items/{custom_id}/content
GET    /v1/images/batches/{id}/download
POST   /v1/images/batches/{id}/cancel
DELETE /v1/images/batches/{id}/outputs
```

Submit request:

```json
{
  "model": "gemini-2.5-flash-image",
  "provider": "gemini_api",
  "items": [
    {
      "custom_id": "cover_001",
      "prompt": "A clean product hero image...",
      "output_count": 1,
      "reference_images": [
        {
          "id": "product-front",
          "type": "subject",
          "mime_type": "image/png",
          "data": "<base64 image bytes without a data URL prefix>"
        },
        {
          "id": "style",
          "type": "style",
          "mime_type": "image/jpeg",
          "file_uri": "gs://internal-managed-bucket/batch-image/refs/style.jpg"
        }
      ]
    }
  ],
  "image_size": "1K",
  "response_mime_type": "image/png"
}
```

`reference_images` is optional per item. Inline `data` is a base64 string decoded by the backend; `file_uri` is reserved for internal Google Cloud Storage references and must be a `gs://` URI. Each reference image must use one of `image/png`, `image/jpeg`, or `image/webp`. Current model limits are:

- `gemini-2.5-flash-image` and other Flash Image aliases: up to 3 reference images per item.
- `gemini-3-pro-image` and other Pro Image aliases: up to 14 reference images per item.
- Per batch job: up to 1000 reference image attachments total after `output_count` expansion across all items. This is an internal Sub2API guardrail for request size and cost control, not the generated-image cap and not a Pro Image per-item capability. The generated-output cap is 200 images per job.
- Per batch job: up to 128 MB decoded inline reference image data total. For large batches or repeated reference images, prefer `gs://` `file_uri` references or split the request into multiple jobs.

`output_count` is optional per item and defaults to `1`. It means "repeat this prompt and reference image set N times" rather than relying on Gemini to return multiple images from one upstream request. The backend expands each repeat into a separate provider JSONL line with suffixed custom ids such as `cover_001_01`, `cover_001_02`. Current limits are:

- Per prompt item: up to 4 output images.
- Per batch job: up to 200 expected output images after expansion. This is the hard generated-output cap for a single job; clients and Codex skills must split larger workloads before submission.
- The output-image limit intentionally matches the default ZIP item limit so newly submitted jobs are always downloadable as one ZIP by item count. ZIP byte size is still capped separately by `max_download_bytes_per_request`.

Public batch response:

```json
{
  "id": "imgbatch_0123456789abcdef0123456789abcdef",
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
}
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
    }
  ],
  "has_more": false
}
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
- Reference images are sent to Gemini as input and can create small upstream input-token and temporary storage cost. They are counted once per expanded output request when `output_count > 1`, but the public MVP billing model does not add a separate reference-image surcharge. User-facing estimated, held, and settled amounts are still based on the output image count and configured batch image unit price.
- Settlement request id is `batch_image_settlement:{batch_id}`.
- Settlement is idempotent; re-running settlement must not double charge.
- Settlement billing failures are retried with a bounded retry limit. After the retry limit is reached, the job is failed and the remaining hold is released through the idempotent release path.

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
DELETE /v1/images/batches/{id}/outputs
```

After output cleanup, downloads return `410 Gone` with `BATCH_IMAGE_OUTPUT_DELETED`.

Cleanup never accepts user-supplied provider paths. Provider cleanup must use server-generated refs and prefix-safe deletion.

For the managed Vertex/GCS batch bucket, disable Cloud Storage soft delete or configure lifecycle carefully to avoid hidden retained storage cost.

## Provider Notes

`gemini_api`:

- Uses Gemini Batch API with JSONL file mode.
- Supports Gemini `apikey` upstream accounts with a configured API key.
- Result file refs are internal.
- API keys are never returned.
- The provider can be selected and submitted through Sub2API when an administrator configures a Gemini API-key upstream account. In the 2026-07-07 PR validation, this path was verified as selectable/callable, but successful image generation was not continued because the test API key had no prepayment.

`vertex`:

- Uses Vertex `BatchPredictionJob` with managed GCS JSONL.
- Supports Gemini `service_account` upstream accounts with valid service account JSON.
- GCS bucket and prefix are server-managed.
- Vertex job name and GCS paths are internal.
- Batch image output should be treated as `1K`/default only in MVP.
- Do not promise `2K` or `4K`.

Other Gemini account/login types are not selected by the current batch image providers unless they expose equivalent API-key or service-account credentials through the same provider flow. They were not covered by the 2026-07-07 PR validation.

## Official Google Enablement

Operators must enable Gemini/Vertex capability in Google's official console before turning on Sub2API batch image for any group. Sub2API feature flags and group switches do not create Google-side access by themselves.

Recommended production path:

- Use a Google Cloud project with billing enabled.
- Enable the relevant Gemini API / Vertex AI APIs for the project.
- Use a service account or Application Default Credentials for the Sub2API runtime.
- Create one fixed Cloud Storage bucket for batch image input and output, then grant the runtime and Vertex service agent the minimum required bucket permissions.
- Configure Sub2API with the project id, location, managed bucket, provider account, model whitelist, and pricing.
- Enable `BATCH_IMAGE_ENABLED` globally, enable image generation on the intended Gemini group, then enable `allow_batch_image_generation` for that group. Non-Gemini groups are not eligible for batch image generation, and the admin UI only shows the batch image group switch after image generation is enabled on a Gemini group.

API-key path:

- Google API keys are suitable for Gemini API development and supported Gemini methods.
- The Sub2API `x-goog-api-key` compatibility header still expects a Sub2API key, not a plain Google key.
- Plain Google API keys should not be documented as the default production credential for Vertex service-account batch jobs.
- If an administrator configures a Gemini API-key upstream account, validate it with one low-cost batch image after the Google account has the required billing/prepayment state. If it has no prepayment, record only that the provider is selectable/callable and that failed submit releases hold.

Official references:

- Gemini API key guide: https://ai.google.dev/gemini-api/docs/api-key
- Gemini API Batch API: https://ai.google.dev/gemini-api/docs/batch-api
- Gemini API image generation and batch image notes: https://ai.google.dev/gemini-api/docs/image-generation
- Vertex/Gemini batch inference: https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/capabilities/batch-inference
- Vertex batch predictions API: https://docs.cloud.google.com/gemini-enterprise-agent-platform/reference/models/batch-prediction-api

## Config

These keys exist in `backend/internal/config/config.go`:

```yaml
batch_image:
  enabled: false
  max_items_per_job_default: 200
  max_items_per_job_trial: 50
  max_output_images_per_job: 200
  max_output_images_per_item: 4
  max_prompt_chars_per_item: 8000
  max_reference_images_per_job: 1000
  max_reference_inline_bytes_per_job: 134217728
  default_response_mime_type: "image/png"
  default_image_size: "1K"

  max_download_items_zip: 200
  max_download_bytes_per_request: 536870912
  max_download_duration_seconds: 600
  max_download_concurrency_per_user: 1

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
  vertex_managed_gcs_prefix: "batch-image/{env}/{batch_id}"
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

## Future Optimization

- Optional object-storage download offload: persist completed image outputs to an operator-configured object store such as GCS, S3, or R2, then issue short-lived signed download links to users. This would avoid routing large image/ZIP downloads through the Sub2API server, which is useful for small-bandwidth deployments. Keep it opt-in because it needs extra storage credentials, lifecycle cleanup, signed-URL expiry policy, access auditing, and compatibility with output deletion.

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
