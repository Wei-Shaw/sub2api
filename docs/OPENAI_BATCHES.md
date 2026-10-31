# Local OpenAI Batch API

These optional APIs provide user-scoped PostgreSQL file storage and durable
batch execution through the existing Sub2API gateway. They require migration
`249_openai_files_batches.sql`, `250_openai_batch_request_json.sql`, and
`251_openai_batch_storage_reservations.sql` and are disabled by default.

## Enable

```yaml
openai_async:
  enabled: true
  max_file_bytes: 33554432          # 32 MiB per upload
  user_temporary_storage_bytes: 805306368 # 768 MiB, included in user_storage_bytes
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
The user storage quota includes uploaded/result files, temporary canonical request
bodies and custom IDs, and the full `max_output_bytes` reservation for each queued
or executing batch. Results written during execution consume that reservation,
not additional quota. Admission reserves capacity transactionally before any
upstream call; concurrent uploads and admissions on other replicas cannot spend
it. Quota failure rejects submission with HTTP 429 before inference is billed.

`user_temporary_storage_bytes` separately caps temporary inputs plus output
reservations per user, within `user_storage_bytes`. It must cover at least one
`max_output_bytes` reservation, and a real batch also needs room for its inputs.
The default is 768 MiB within a 1 GiB total quota. If you configure a smaller
total user quota, lower the temporary quota and output reservation accordingly. Five full 32 MiB inputs with
128 MiB output reservations may hit the storage cap before `max_active_batches`.
Canonical JSON escaping can make request copies larger than the uploaded JSONL;
admission measures those actual copies. Deleting an input file frees only that
file's space, not the batch's independent temporary copy or output reservation.
Expired files count until bounded cleanup removes them. Cleanup runs while workers are enabled. Keep database backups and retention
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
deleted. An oversized individual response produces an explicit item error. If output
export cannot fit after an administrator lowers a quota or output limit, the job
remains `finalizing` with a `storage_limit_exceeded` error: all stored successful
and failed results remain intact. It still counts toward active-batch and storage
limits. Export retries after the database lease expires (normally about one
minute), including after a restart. It never executes completed requests again.
Free file storage, restore the quota, or raise the output limit as appropriate;
there is no need to resubmit the batch. The error clears on successful export.
Cancellation also preserves completed results and their original error records.
Retention of blocked results continues until export succeeds; they are not silently
expired or dropped by file cleanup. Operators should monitor and resolve blocked
jobs rather than allow them to accumulate.

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

## Capacity and recovery operations

Quotas count logical payload bytes, not PostgreSQL indexes, row headers, WAL,
TOAST overhead, backups, or process memory. Budget additional database disk and
memory: uploads and JSONL validation buffer input, and each finalizing worker can
buffer up to `max_output_bytes` while producing result files. Worker concurrency
is per replica, so multiply that memory allowance by concurrent workers and
replicas. Commit briefly holds both temporary results and their output-file copy;
logical reservations are released in the same transaction as file creation.

For each user, inspect the two sources of logical usage (including expired files
awaiting cleanup):

```sql
SELECT user_id, SUM(octet_length(content)) AS file_bytes
FROM openai_files GROUP BY user_id;
SELECT user_id, SUM(input_bytes) AS request_copy_bytes,
       SUM(GREATEST(reserved_output_bytes, result_bytes)) AS output_capacity_bytes
FROM openai_batches GROUP BY user_id;
SELECT id, user_id, payload->'errors' AS errors, lease_until
FROM openai_batches WHERE status = 'finalizing';
```

Use the authenticated Files API to remove unneeded files or change the configured
quota and restart the relevant replicas. Do not delete temporary item rows to
recover space: these can be the only copy of already billed inference results.
Migration 251 accounts for legacy temporary inputs and results; old queued jobs
reserve remaining output capacity before any further execution. If capacity is
unavailable, they wait without replaying previous running items.

The integration suite includes 32 concurrent admissions through independent store
instances, quota saturation by uploads, reservation release on completion, and
quota-reduction/restart recovery with an executor-call count of zero. Run with
`go test -race -tags=integration ./internal/openaiasync` for concurrency checking.
