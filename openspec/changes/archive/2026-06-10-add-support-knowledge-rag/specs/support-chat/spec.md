## ADDED Requirements

### Requirement: RAG Injection in Chat Prompt Assembly

The chat handler defined in `support-chat` SHALL extend its system-prompt assembly with a RAG section when `support_chat_rag_enabled = true`. Specifically, before invoking the upstream LLM the system SHALL:

1. Take the **most recent** user message from the request body.
2. Compute its embedding via the embedding service (using the configured customer-service API key).
3. Retrieve top-K knowledge entries via the vector retrieval helper defined by the `support-knowledge-rag` capability (K = `support_chat_rag_top_k`).
4. If retrieval returns one or more entries, format them as a `## 相关知识` (or `## Relevant Knowledge`) Markdown section, with each entry prefixed by `[FAQ]` (for FAQ items) or `[DOC]` (for doc chunks, including the `source_url` for attribution), and inject the section between the admin-configured prompt and the hardcoded safety footer.
5. If retrieval returns zero entries, the prompt SHALL be assembled exactly as defined in `support-chat` (no `## 相关知识` section emitted, avoiding noise to the LLM).

When `support_chat_rag_enabled = false`, the chat handler SHALL behave exactly as defined by `support-chat` with no embedding call performed. This preserves the ability to fully disable RAG (and skip its embedding cost) via a single setting toggle.

The system SHALL impose a **token budget** on the RAG section: the total characters of `[FAQ]/[DOC]` content SHALL NOT exceed `support_chat_max_request_tokens × 2 × 0.5` characters (using the project's char≈0.5 tokens estimate). When the budget is exceeded, lower-similarity entries SHALL be dropped first until the budget is met. The truncation SHALL prefer to keep at least one entry rather than emitting an empty section.

The system's pre-flight rules from `support-chat` (auth check, rate limiting, turn truncation, token cap) SHALL execute **before** the RAG injection. Token-cap-driven truncation of historical messages SHALL apply to the assembled prompt **including** the RAG section.

#### Scenario: RAG enabled and retrieval succeeds

- **GIVEN** `support_chat_rag_enabled = true`, retrieval returns 3 entries (1 FAQ + 2 docs) above threshold
- **WHEN** an authenticated user sends a chat message
- **THEN** the assembled system prompt contains a `## 相关知识` section with the 3 entries (FAQ first or interleaved by score), each entry prefixed with `[FAQ]` or `[DOC]`, the doc entries include their `source_url` (e.g. `[DOC] (来源: https://docs.example/keys)`); the section appears between the admin prompt and the safety footer

#### Scenario: RAG enabled but no relevant knowledge

- **GIVEN** `support_chat_rag_enabled = true`, retrieval returns zero entries (all below 0.3 threshold)
- **WHEN** an authenticated user sends a chat message
- **THEN** the assembled system prompt does NOT contain a `## 相关知识` section; the prompt structure is `<admin_prompt>\n\n<safety_footer>`

#### Scenario: RAG disabled bypasses embedding entirely

- **GIVEN** `support_chat_rag_enabled = false`
- **WHEN** an authenticated user sends a chat message
- **THEN** no embedding call is made AND the assembled prompt matches the `support-chat` capability's structure exactly (no `## 相关知识` section)

#### Scenario: Embedding service failure does not break chat

- **GIVEN** `support_chat_rag_enabled = true` and the embedding service returns 5xx for the user message
- **WHEN** the chat request is processed
- **THEN** the chat request still succeeds (SSE response streams normally); the assembled prompt does NOT contain a `## 相关知识` section; the embedding error is logged but not surfaced to the user

#### Scenario: RAG section honors token budget

- **GIVEN** `support_chat_rag_enabled = true`, `support_chat_max_request_tokens = 4000`, retrieval returned 5 entries totalling ~6000 chars (estimated ~3000 tokens, well over the 0.5 × budget allocation)
- **WHEN** the prompt is assembled
- **THEN** the lowest-similarity entries are dropped from the section until the section's token estimate ≤ `support_chat_max_request_tokens × 0.5`, and at least the highest-similarity entry remains

### Requirement: FAQ Source of Truth Migration

The chat handler's FAQ click behavior (defined by `support-chat`'s "Public FAQ Endpoint" requirement) SHALL continue to surface FAQ items to the widget, but the data source SHALL be `support_faq_items` (the new table) rather than the legacy `support_chat_faqs` setting. The public FAQ endpoint `GET /api/v1/support/chat/faqs` SHALL return entries from `support_faq_items` where `enabled = true`, ordered by `sort_order ASC, id ASC`.

When `support_faq_items` is empty AND the legacy setting is non-empty (e.g. immediately before the migration runs, or in an edge-case where the migration is delayed), the system MAY fall back to reading the legacy setting, but the **persistent** source of truth SHALL be the table.

#### Scenario: Public FAQ endpoint reads from new table

- **GIVEN** `support_faq_items` contains 3 rows with `enabled = true` and 1 with `enabled = false`
- **WHEN** an anonymous client GETs `/api/v1/support/chat/faqs`
- **THEN** the response contains exactly the 3 enabled rows in `sort_order` order; the legacy `support_chat_faqs` setting (regardless of its content) is not consulted
