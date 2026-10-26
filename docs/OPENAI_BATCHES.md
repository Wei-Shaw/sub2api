# Local OpenAI Batch API

These optional APIs provide user-scoped PostgreSQL file storage and durable
batch execution through the existing Sub2API gateway. They require migration
`249_openai_files_batches.sql` and `250_openai_batch_request_json.sql` and are disabled by default.

## Enable

```yaml
openai_async:
  enabled: true
  max_file_bytes: 33554432          # 32 MiB per upload
  user_storage_bytes: 1073741824   # 1 GiB, shared by this user's keys
  file_ttl_seconds: 2592000        # 30 days
  max_batch_requests: 1000
  max_active_batches: 5            # per user, including queued batches
  max_line_bytes: 1048576
  max_output_bytes: 134217728      # output + error files per batch
  max_response_bytes: 8388608      # per inference response
  worker_concurrency: 2            # per application replica
  request_timeout_seconds: 300
```

Every scalar is also configurable by environment variable, for example
`OPENAI_ASYNC_ENABLED=true`. Existing global HTTP and gateway body limits also
apply; allow space for multipart framing above the largest intended upload.
Increasing the per-file limit increases buffering and database storage costs.
`max_output_bytes` must reserve at least 4096 bytes per allowed batch request
(`max_batch_requests * 4096`), including worst-case escaped failure identifiers.
The configured storage quota counts expired files until bounded cleanup removes
them. Cleanup runs while workers are enabled. Keep database backups and retention
policies appropriate for user-supplied content.

No separate filesystem volume, Redis queue, or new upstream credential is
required. PostgreSQL is the shared queue and file store across replicas. Worker
concurrency is per replica; normal gateway user/account concurrency still applies.

## Files

```text
POST   /v1/files
GET    /v1/files
GET    /v1/files/{file_id}
GET    /v1/files/{file_id}/content
DELETE /v1/files/{file_id}
```

Upload multipart `file` and `purpose=batch`; generated result files have purpose
`batch_output`. The Files endpoints are the minimal input/output storage needed
by Batch. General-purpose document and image uploads are separate follow-up work. Batch input
filenames must end in `.jsonl`. File ids use the opaque `file-local-` namespace.
An optional `expires_after[anchor]=created_at` and `expires_after[seconds]` can set
a retention period from one hour through thirty days.

File listing supports `purpose`, `after`, `order=asc|desc`, and `limit=1..10000`.
Batch listing supports `after` and `limit=1..100`. Pagination is stable within a
timestamp and cursors are resolved only within the authenticated user's records.
Every operation checks user ownership, including content retrieval and deletion.
Different keys owned by the same user can manage that user's resources. Basic
credential, user, group, and IP checks still apply. File management, batch reads,
and cancellation remain available after balance or key quota exhaustion.

## Batch lifecycle

```text
POST /v1/batches
GET  /v1/batches
GET  /v1/batches/{batch_id}
POST /v1/batches/{batch_id}/cancel
```

Upload the input JSONL with purpose `batch`. Each line must be one request:

```jsonl
{"custom_id":"question-1","method":"POST","url":"/v1/responses","body":{"model":"your-model","input":"What is 2+2?"}}
{"custom_id":"question-2","method":"POST","url":"/v1/responses","body":{"model":"your-model","input":"What is 3+3?"}}
```

Create the job:

```json
{
  "input_file_id": "file-local-...",
  "endpoint": "/v1/responses",
  "completion_window": "24h",
  "metadata": {"workload": "example"}
}
```

Supported endpoints are `/v1/responses`, `/v1/chat/completions`, and
`/v1/embeddings`. Every line must use the batch's endpoint, `POST`, a unique
non-empty `custom_id`, and the same model. Streaming is rejected. Invalid input
returns a synchronous 400 before queueing. Limits intentionally default below
the official service's 50,000 requests and 200 MB JSONL limits. Metadata supports
16 entries, with keys up to 64 and values up to 512 characters.

`output_expires_after` optionally accepts `{"anchor":"created_at","seconds":3600}`
through thirty days. Otherwise result files use `file_ttl_seconds`.

Jobs transition from `validating` to `in_progress` and then `completed`, `failed`,
`expired`, or `cancelled`; cancellation first enters `cancelling`. Successful and
failed requests are written to separate JSONL files and their ids are available
as `output_file_id` and `error_file_id`. Each record contains `id`, `custom_id`,
`response` (`status_code`, `request_id`, and `body`) and `error`. Counts track
successful and failed executed requests. On expiry, pending requests receive
`batch_expired` error records. Cancellation lets the currently running request
finish and skips the remaining pending requests.

Each item re-enters the actual authenticated router with the submitting API key
and the security IP recorded at creation. The key's owner and group must still
match. Revocation, model allowlists, subscriptions, moderation, concurrency,
rate limits, accounting, and audits use the same gateway path as live requests.
Only the key id is persisted by this feature; the existing key repository supplies
the credential at execution time. The worker never makes requests to URLs from
the input file.

This is local asynchronous execution of **ordinary billed requests**. It does
not provide OpenAI's discounted Batch pricing, separate Batch quota, or guaranteed
completion within 24 hours. The existing gateway's own bounded failover/retry
policies still apply within an execution attempt.

Workers claim jobs with database leases and fence all writes with a lease token.
An item is committed as running before upstream execution. After a crash or lost
lease, a running item gets `batch_execution_interrupted` and is **not replayed**:
its upstream result and billing may already have committed. Pending items resume.
This favors avoiding duplicate submissions over silently claiming exactly-once
delivery. A client should inspect interruption/timeout errors before resubmitting.
Output files and final state commit together; temporary item records are then
deleted. Output exceeding local limits produces an explicit error or failed job.

## Validation

```sh
cd backend
make test-unit
make test-integration
```

Database integration tests use isolated PostgreSQL testcontainers by default.
Alternatively, set `OPENAI_ASYNC_TEST_DSN` to a dedicated test database. Each
test creates and removes its own schema and checks that both forward migrations
can be replayed. Never point this variable at a production database.

Migration 250 keeps queued bodies as JSON rather than JSONB, preserving valid
JSON containing escaped NUL characters and numbers outside PostgreSQL numeric
limits. No JSONB indexing is required for these opaque request bodies.

Official contracts: [Files](https://developers.openai.com/api/reference/resources/files),
[Batches](https://developers.openai.com/api/reference/resources/batches).
