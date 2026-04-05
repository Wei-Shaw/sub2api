# sub2api Batch C Hotspots Design

## Purpose

This document turns the high-risk overlap files from the upstream absorption inventory into an explicit transplant boundary. It does **not** merge upstream logic yet. Its purpose is to make the next phase safe by stating, per hotspot, what local semantics are frozen, what upstream added, and what transplant strategy is acceptable.

## Frozen local semantics

These three semantic baselines must survive any Batch C work:

1. OpenAI `active/exhausted` routing and `-Sys` continuation behavior
2. Current billing/usage semantics:
   - priority-account multiplier
   - priority service-tier pricing source
   - user-visible billing-factor / unit-price explanation
3. `sub2api-openai` independent OpenCode provider semantics and recommendation flow

## Hotspot groups

### 1. Billing/pricing execution chain

Files:

- `backend/internal/service/billing_service.go`
- `backend/internal/service/pricing_service.go`
- `backend/internal/service/model_pricing_resolver.go`
- `backend/internal/service/gateway_service.go`
- `backend/internal/service/openai_gateway_record_usage_test.go`
- `backend/internal/service/gateway_record_usage_test.go`

Local semantics to preserve:

- `priority` is a pricing source, not an extra Fast multiplier
- subscription accumulation follows `actual_cost`
- user-visible explanations reflect actual unit price and multiplier causes

Upstream additions worth considering:

- channel pricing resolution
- billing-model source tracking
- image output token billing
- unified pricing/model restriction decisions

Conflict points:

- same execution chain already changed heavily both locally and upstream
- upstream `channel_mapped` billing source can overwrite current local `pricing_source` semantics
- careless merge can reintroduce the previously fixed subscription/usage mismatches

Recommended strategy:

- do **not** merge these files directly
- split upstream ideas into smaller importable concepts:
  - billing-model source tagging
  - image output token accounting
  - channel pricing resolver
- reapply them on top of current local billing semantics one concept at a time

### 2. OpenAI request/selection/transport chain

Files:

- `backend/internal/service/openai_gateway_service.go`
- `backend/internal/service/openai_model_mapping.go`
- `backend/internal/service/openai_ws_forwarder.go`
- `backend/internal/handler/gateway_handler.go`
- `backend/internal/handler/openai_gateway_handler.go`
- `backend/internal/handler/openai_chat_completions.go`

Local semantics to preserve:

- `-Sys` detection for role-based user items
- active/exhausted target-group behavior
- upstream structured error envelope
- instructions promotion from system messages

Upstream additions worth considering:

- channel restrictions during scheduling
- prompt/cache helpers
- model normalization simplifications

Conflict points:

- same code path already holds local routing, passthrough, prompt, and error-semantic fixes
- upstream channel restriction logic could change account-selection order and billing source timing

Recommended strategy:

- do not import wholesale
- compare per stage:
  1. request normalization
  2. account selection
  3. upstream forwarding
  4. usage recording
- only transplant upstream ideas that do not alter stage ordering or local target-group semantics

### 3. usage_log schema/repository band

Files:

- `backend/ent/schema/usage_log.go`
- `backend/ent/migrate/schema.go`
- `backend/ent/usagelog*.go`
- `backend/internal/repository/usage_log_repo.go`
- `backend/internal/service/usage_log.go`
- `backend/internal/service/usage_log_helpers.go`
- `backend/migrations/084_channel_billing_model_source.sql`
- `backend/migrations/085_channel_restrict_and_per_request_price.sql`
- `backend/migrations/086_channel_platform_pricing.sql`
- `backend/migrations/087_usage_log_billing_mode.sql`
- `backend/migrations/088_channel_billing_model_source_channel_mapped.sql`
- `backend/migrations/089_usage_log_image_output_tokens.sql`

Local semantics to preserve:

- routing observability fields
- billing breakdown fields
- effective unit-price persistence

Upstream additions worth considering:

- usage billing mode
- image output token columns
- channel billing source columns

Conflict points:

- this is the highest-risk persistence layer
- column order, migration order, scan order, and ent regeneration are all intertwined

Recommended strategy:

- treat this as a fresh schema reconciliation exercise, not a merge
- first write a target `usage_log` column matrix with three classes:
  - local-only fields to keep
  - upstream-only fields worth adding
  - duplicate/overlapping fields to unify
- only after that generate ent and migrations once

### 4. DTO and user/admin display semantics

Files:

- `backend/internal/handler/dto/types.go`
- `backend/internal/handler/dto/mappers.go`
- `frontend/src/types/index.ts`
- `frontend/src/views/admin/UsageView.vue`
- `frontend/src/views/user/UsageView.vue`
- `frontend/src/i18n/locales/en.ts`
- `frontend/src/i18n/locales/zh.ts`

Local semantics to preserve:

- user-visible pricing reason explanations
- admin-visible routing and pricing source detail

Upstream additions worth considering:

- usage billing mode display
- three-level model mapping display
- image token breakdown display

Conflict points:

- the same UI regions are already occupied by local price-factor explanations
- naive copy would likely displace or dilute current explanations

Recommended strategy:

- decide information architecture first:
  - user pages prioritize cost explanation
  - admin pages prioritize routing/mapping/audit detail
- then transplant only the upstream UI pieces that add information without replacing those priorities

## Immediate next step

The next implementation phase should **not** touch these files yet. It should first absorb Batch A and Batch B, then reopen this document to design a dedicated Batch C reconciliation plan file-by-file.
