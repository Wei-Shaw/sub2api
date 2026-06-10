## ADDED Requirements

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
