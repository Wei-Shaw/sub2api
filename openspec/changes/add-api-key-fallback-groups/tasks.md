## 1. Data Model and Persistence

- [x] 1.1 Add the ordered `fallback_group_ids` field to the API key Ent schema and service domain model.
- [x] 1.2 Generate Ent artifacts and add a production database migration with an empty-list default.
- [x] 1.3 Persist, query, update, and map fallback IDs in the API key repository with focused repository tests.

## 2. API and Cache Contract

- [x] 2.1 Add fallback IDs to create/update requests and user/admin API key responses.
- [x] 2.2 Validate list size, uniqueness, primary-group exclusion, same-platform membership, user group access, enabled status, and enterprise-key restrictions.
- [x] 2.3 Include fallback IDs in authentication cache snapshots and verify update invalidation behavior.

## 3. Ordered Gateway Routing

- [x] 3.1 Add shared helpers for primary-plus-fallback group candidate order and selection-error classification.
- [x] 3.2 Integrate ordered pre-send selection into supported Anthropic, Gemini, OpenAI, image, WebSocket, and async entry points.
- [x] 3.3 Propagate the selected effective group through mapping, policy, limits, billing, usage logs, observability, sticky sessions, and same-group retries.
- [x] 3.4 Add unit tests for primary success, ordered fallback, exhaustion, non-selection errors, and effective-group billing.

## 4. Personal Console

- [x] 4.1 Extend API key frontend types and request payloads with ordered fallback IDs.
- [x] 4.2 Add create/edit controls to add, remove, and reorder eligible fallback groups.
- [x] 4.3 Display the configured order in the key list/detail UI and add Chinese and English translations.
- [x] 4.4 Add frontend tests for validation, ordering, serialization, and reload display.

## 5. Verification

- [x] 5.1 Run focused backend and frontend tests for API key fallback behavior.
- [x] 5.2 Run backend unit tests, frontend tests, lint, and migration/schema consistency checks.
- [x] 5.3 Run `govulncheck ./...` and confirm no reachable vulnerability regression.
