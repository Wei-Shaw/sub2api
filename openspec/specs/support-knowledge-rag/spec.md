# Support Knowledge RAG

## Purpose

Provide a Retrieval-Augmented Generation (RAG) substrate for the support-chat capability so the floating customer-service widget can answer in-product questions grounded in (a) admin-curated FAQ items and (b) auto-crawled documentation pages, instead of relying solely on the LLM's prior. The capability owns the persistent knowledge store (two `pgvector`-backed tables), the document fetch+chunk+embed pipeline, the admin CRUD/rebuild/purge endpoints, the cron-based auto-rebuild scheduler, and the top-K vector retrieval helper consumed by the chat handler. Embeddings are computed on demand using the same admin-configured customer-service API key as the chat (no separate billing path). The capability is gated by `support_chat_rag_enabled`; when disabled, no embedding cost is incurred and the chat handler falls back to its non-RAG behavior. On first install the capability migrates any pre-existing `support_chat_faqs` setting (owned by the `support-chat` capability) into the new `support_faq_items` table so that operators upgrading from the chat-widget release lose no data.
## Requirements
### Requirement: pgvector Schema and Indexing

The system SHALL enable PostgreSQL's `vector` extension at startup (`CREATE EXTENSION IF NOT EXISTS vector`) and SHALL create two tables for the knowledge base:

- `support_faq_items(id, question VARCHAR(200), answer TEXT, tags TEXT[], enabled BOOLEAN, sort_order INTEGER, embedding VECTOR(1536) NULL, created_at, updated_at)`.
- `support_doc_chunks(id, source_url VARCHAR(500), chunk_text TEXT, content_hash CHAR(64), embedding VECTOR(1536) NULL, fetched_at, created_at, UNIQUE(source_url, content_hash))`.

The system SHALL create an `IVFFlat` index using `vector_cosine_ops` on the `embedding` column of each table. The `embedding` column SHALL be nullable; rows with `NULL` embeddings SHALL NOT participate in retrieval queries (filtered out at the SQL level).

If the `vector` extension cannot be created (e.g. the PG instance lacks the extension files), the system SHALL log a fatal error but SHALL NOT panic; downstream RAG features SHALL respond with a clear "RAG unavailable" error when invoked, while non-RAG features (ticket system, chat widget without RAG) remain fully operational.

#### Scenario: Fresh install creates schema and indexes

- **GIVEN** a clean database with `pgvector` available
- **WHEN** the application starts
- **THEN** `vector` extension is enabled, both tables exist, both `IVFFlat` indexes exist on the `embedding` columns

#### Scenario: pgvector unavailable degrades gracefully

- **GIVEN** a PG instance without `pgvector`
- **WHEN** the application starts
- **THEN** a fatal log entry is recorded, the application continues to start, the ticket system and chat widget (with `support_chat_rag_enabled = false`) function normally, and any RAG admin endpoint returns `503 Service Unavailable` with body indicating pgvector is missing

### Requirement: FAQ Item CRUD

The system SHALL expose admin-only CRUD endpoints for FAQ items:

- `GET /api/admin/support/faqs` — list, ordered by `sort_order ASC, id ASC`.
- `POST /api/admin/support/faqs` — create.
- `PUT /api/admin/support/faqs/:id` — update (full replacement of provided fields).
- `DELETE /api/admin/support/faqs/:id` — delete.

Each request body SHALL validate `question` 1..200 chars, `answer` 1..5000 chars, `tags` array max 20 entries each 1..30 chars, `sort_order` integer, `enabled` boolean. On every successful create or update where `question` or `answer` changed, the system SHALL **synchronously** trigger an embedding computation for the new content and persist the resulting vector to the row's `embedding` column. If the embedding call fails, the row SHALL still be persisted with `embedding = NULL` and the response SHALL include a non-blocking warning (`"warning": "embedding_failed"`); the row will be skipped in RAG retrieval until re-indexed.

The list response SHALL include a per-row `indexed: boolean` field (true when `embedding IS NOT NULL`) so the admin UI can flag un-indexed rows.

The system SHALL additionally expose `POST /api/admin/support/faqs/:id/reindex` to recompute the embedding for a single row.

#### Scenario: Admin creates FAQ, embedding succeeds

- **GIVEN** a working embedding service and an admin
- **WHEN** the admin POSTs `{question:"How to recharge?", answer:"Go to /payment...", enabled:true}`
- **THEN** the response is `201 Created`, the row exists, and `embedding IS NOT NULL`; the list endpoint reports `indexed: true` for this row

#### Scenario: Admin creates FAQ, embedding fails

- **GIVEN** the embedding service is failing (5xx)
- **WHEN** the admin POSTs a valid FAQ
- **THEN** the response is `201 Created` with body containing `"warning":"embedding_failed"`, the row is persisted with `embedding = NULL`, and the list endpoint reports `indexed: false` for this row

#### Scenario: Admin re-indexes a row

- **GIVEN** an FAQ row with `embedding = NULL`
- **WHEN** the admin POSTs `/api/admin/support/faqs/:id/reindex`
- **THEN** the row's `embedding` is computed and stored, response is `200 OK`

#### Scenario: Non-admin denied

- **GIVEN** a regular authenticated user
- **WHEN** they call any FAQ admin endpoint
- **THEN** the response is `403 Forbidden`

### Requirement: Document Pipeline

The system SHALL provide a document pipeline that fetches the configured `support_chat_rag_doc_url`, parses HTML, extracts the main content, splits the text into chunks, computes embeddings, and upserts them into `support_doc_chunks`.

The pipeline SHALL:

1. **Refuse to start** when `support_chat_rag_doc_url` is empty (record status `empty_doc_url`).
2. **Fetch** the seed URL with a 30s timeout and `User-Agent: Sub2APIDocBot/1.0`. Non-2xx responses SHALL be skipped with the per-URL error recorded in the pipeline status.
3. **Discover** additional URLs to depth `support_chat_rag_doc_depth` (default 1, max 2): only same-domain `<a href>` links from the seed page (and recursively at depth 2).
4. **Cap** the total number of fetched URLs at **50** (a hardcoded safety limit independent of admin configuration).
5. **Chunk** each page's text by Markdown headers (`#`, `##`, `###`) preferentially, falling back to character-count splits at `support_chat_rag_chunk_size` boundaries (200..2000 chars, default 800) with `support_chat_rag_chunk_overlap` (0..500, default 80) chars overlapping. Chunks shorter than 50 characters SHALL be discarded.
6. **De-duplicate** chunks via `(source_url, content_hash = sha256(chunk_text))`. Existing chunks with matching hash SHALL be left untouched (NOT re-embedded).
7. **Embed** new chunks in batches of 100, persisting `embedding` on success. Failed batches SHALL be logged but SHALL NOT abort the pipeline; affected rows SHALL be persisted with `embedding = NULL`.
8. **Clean up** orphaned chunks: any pre-existing chunks with `source_url ∈ fetched_urls` whose `content_hash` is NOT in the current run's set SHALL be deleted.
9. **Record status** at the end (regardless of partial failures): `last_run_at`, `chunks_total`, `chunks_added`, `chunks_removed`, `chunks_failed_embed`, `errors[]` (per-URL).

The pipeline SHALL hold a PostgreSQL advisory lock for its duration; concurrent invocations SHALL refuse to start and SHALL return `"already_running"`.

#### Scenario: Empty doc_url short-circuits

- **GIVEN** `support_chat_rag_doc_url = ""`
- **WHEN** the pipeline is invoked
- **THEN** the status is recorded with `status = "empty_doc_url"`, no HTTP fetches occur, no rows change

#### Scenario: 50-page cap is enforced

- **GIVEN** a documentation site with 200 pages, `support_chat_rag_doc_depth = 1`
- **WHEN** the pipeline runs
- **THEN** at most 50 URLs are fetched and the status `errors` array contains an explanatory entry indicating the cap was hit

#### Scenario: Unchanged content is skipped

- **GIVEN** an initial pipeline run that produced 100 chunks; the source HTML has not changed
- **WHEN** the pipeline runs again
- **THEN** `chunks_added = 0`, `chunks_removed = 0`, no embedding calls are made

#### Scenario: Page removed → chunks cleaned up

- **GIVEN** chunk rows exist for `source_url = X`; on the next run, page `X` is no longer linked from the seed page (or returns 404)
- **WHEN** the pipeline runs
- **THEN** the rows for `X` are deleted (`chunks_removed` reflects the count) and the status `errors` array contains the 404 if applicable

#### Scenario: Concurrent invocation refused

- **GIVEN** pipeline run A is in progress (advisory lock held)
- **WHEN** pipeline run B is invoked
- **THEN** B refuses to start and returns `{"status":"already_running"}` immediately

### Requirement: Document Index Management Endpoints

The system SHALL expose admin-only endpoints to manage the document index:

- `POST /api/admin/support/doc-index/rebuild` — asynchronously triggers a pipeline run and returns `{"accepted": true}` immediately (or `{"accepted": false, "reason": "already_running"}` if locked).
- `GET /api/admin/support/doc-index/status` — returns the most recent pipeline run summary (or an empty object on first install).
- `POST /api/admin/support/doc-index/purge` — deletes all rows from `support_doc_chunks` (admin opt-in, used when migrating doc_url to a new domain).

#### Scenario: Admin triggers rebuild

- **GIVEN** the pipeline is idle
- **WHEN** an admin POSTs `/api/admin/support/doc-index/rebuild`
- **THEN** the response is `{"accepted":true}` and a goroutine begins running the pipeline; the status endpoint reflects an in-progress state until completion

#### Scenario: Status returns most recent run

- **GIVEN** a pipeline ran 10 minutes ago with 152 chunks added
- **WHEN** an admin GETs `/api/admin/support/doc-index/status`
- **THEN** the response includes `last_run_at`, `chunks_total = 152`, and any captured errors

#### Scenario: Purge clears chunks

- **GIVEN** the table has 1000 rows
- **WHEN** an admin POSTs `/api/admin/support/doc-index/purge`
- **THEN** the response is `200 OK`, `support_doc_chunks` has 0 rows; `support_faq_items` is untouched

### Requirement: Vector Retrieval

The system SHALL provide a retrieval helper that, given a query text, returns top-K similar entries from FAQ items and document chunks combined. The helper SHALL:

1. Embed the query text using the configured embedding model.
2. Execute a single SQL query that `UNION ALL`s `support_faq_items` (where `enabled AND embedding IS NOT NULL`) and `support_doc_chunks` (where `embedding IS NOT NULL`), computes `1 - (embedding <=> query_vec)` as similarity, orders by similarity DESC, and limits to `K`.
3. Filter results below a similarity threshold of `0.3` (results below threshold SHALL NOT be returned).
4. On embedding failure, return an empty result set without raising (callers MUST treat empty as "no relevant knowledge").

The helper's K SHALL come from `support_chat_rag_top_k` (default 5, range 1..20).

#### Scenario: Top-K mixed sources

- **GIVEN** 10 FAQ items and 100 doc chunks; query embedding has high similarity (≥ 0.6) to 2 FAQs and 4 doc chunks; K = 5
- **WHEN** retrieval is invoked
- **THEN** the result contains 5 entries ordered by similarity DESC, drawn from both `faq` and `doc` source types

#### Scenario: Below-threshold filtered

- **GIVEN** all rows have similarity < 0.3 to the query
- **WHEN** retrieval is invoked
- **THEN** the result is empty (length 0)

#### Scenario: Embedding service down

- **GIVEN** the embedding service returns 5xx for the query
- **WHEN** retrieval is invoked
- **THEN** the result is empty (length 0); the error is logged but not raised to the caller

### Requirement: Cron-Based Auto-Rebuild

The system SHALL run the document pipeline automatically on a schedule determined by `support_chat_rag_doc_cron`:

- `daily-03` (default): every day at 03:00 server local time.
- `weekly`: every Monday at 03:00 server local time.
- `manual`: never auto-runs; only triggered by admin button.

When `support_chat_rag_enabled = false`, the cron job SHALL still run (so re-enabling RAG immediately has fresh data), unless `support_chat_rag_doc_cron = manual`.

#### Scenario: Daily schedule fires at 03:00

- **GIVEN** `support_chat_rag_doc_cron = daily-03`, server clock just rolled over to 03:00:00
- **WHEN** the in-process scheduler ticks
- **THEN** the document pipeline starts (subject to the advisory lock)

#### Scenario: Manual schedule never auto-runs

- **GIVEN** `support_chat_rag_doc_cron = manual`
- **WHEN** server clock crosses 03:00 over multiple days
- **THEN** the pipeline is NOT triggered automatically; it only runs when the admin endpoint is invoked

### Requirement: FAQ Setting Migration

On application startup, when `support_faq_items` is empty AND the legacy `support_chat_faqs` setting (defined by `support-chat`) is non-empty, the system SHALL migrate each entry into `support_faq_items` (preserving `question`, `answer`, `sort_order`, `enabled`) and SHALL asynchronously trigger embedding computation for the migrated rows. The legacy setting key SHALL remain readable but SHALL no longer be the source of truth: the chat handler and admin UI SHALL read from `support_faq_items` going forward.

The migration SHALL be **idempotent** and **safe**: if `support_faq_items` already contains any rows, no migration occurs (preventing duplicate inserts on restart).

#### Scenario: First start migrates legacy setting

- **GIVEN** `support_chat_faqs = [{question:"q1", answer:"a1", enabled:true, sort_order:0}]` and `support_faq_items` is empty
- **WHEN** the application starts
- **THEN** `support_faq_items` contains 1 row matching the legacy entry; embedding is asynchronously computed for that row

#### Scenario: Subsequent restarts do not re-migrate

- **GIVEN** `support_faq_items` already contains 5 rows (from prior migration or manual edits)
- **WHEN** the application restarts
- **THEN** no rows are inserted; the legacy setting (if still present) is ignored

### Requirement: Embedding Service Uses Shared External Credentials

All embedding operations defined by this capability — synchronous embedding on FAQ create/update/reindex, the document pipeline's batch embedding, and the vector retrieval helper's query embedding — SHALL read their HTTP endpoint and bearer token from the `support_chat_llm_base_url` and `support_chat_llm_api_key` settings (defined by the `support-chat` capability) and SHALL issue requests to `<support_chat_llm_base_url>/embeddings`. The model parameter SHALL come from `support_chat_rag_embed_model` (default `text-embedding-3-small`).

The embedding service SHALL NOT consult the platform's internal `api_keys` table and SHALL NOT self-call the platform's own `/v1/embeddings` route. When either credential field is empty, the embedding service SHALL behave as if the upstream returned a non-2xx response: callers (FAQ CRUD, doc pipeline, retrieval helper) SHALL persist their rows with `embedding = NULL` (or return an empty result for retrieval) and log a warning. This ensures the rag pipeline degrades gracefully when an admin enables RAG but has not yet supplied credentials.

#### Scenario: Embedding call uses external endpoint

- **GIVEN** `support_chat_llm_base_url = "https://api.openai.com/v1"`, `support_chat_llm_api_key = "sk-real"`, `support_chat_rag_embed_model = "text-embedding-3-small"`
- **WHEN** an admin creates a new FAQ item triggering synchronous embedding
- **THEN** the platform issues an outbound HTTPS POST to `https://api.openai.com/v1/embeddings` with `Authorization: Bearer sk-real` and body `{"model":"text-embedding-3-small","input":"<question>\n\n<answer>"}`; the response vector is persisted to the row's `embedding` column

#### Scenario: Empty credentials degrade FAQ embed gracefully

- **GIVEN** `support_chat_llm_base_url = ""` (or `support_chat_llm_api_key = ""`) and an admin creates a new FAQ item
- **THEN** no outbound HTTP request is made; the row is persisted with `embedding = NULL`; the response includes `"warning":"embedding_failed"`; a WARN log entry records `embedding skipped: missing credentials`

#### Scenario: Empty credentials degrade retrieval gracefully

- **GIVEN** `support_chat_llm_base_url = ""` and `support_chat_rag_enabled = true`
- **WHEN** the chat handler invokes the retrieval helper for a user message
- **THEN** the helper returns an empty result; the chat request still succeeds (no `## 相关知识` section); a WARN log entry records the missing credentials

