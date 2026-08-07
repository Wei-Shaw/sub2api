# Qoder Integration Requirements

## Overview

Integrate Qoder Cloud Agents as a new platform in Sub2API, allowing users to paste a single Qoder PAT token (`pt-...`) into an account and immediately use it through the standard OpenAI-compatible `/v1/chat/completions` endpoint.

## User Requirements

1. **Single-token onboarding**: User pastes one PAT token; Sub2API handles all provisioning (Agent, Environment, Session) automatically.
2. **OpenAI-compatible interface**: Expose only `/v1/chat/completions` (+ `/v1/models`). No proprietary endpoints.
3. **Session reuse**: Consecutive multi-turn conversations with the same context should reuse the same Qoder session to preserve agent state.
4. **Streaming + non-streaming**: Both `stream: true` and `stream: false` must work.
5. **Graceful degradation**: If Redis is unavailable, fall back to creating a new session per request with flattened history.

## Process Requirement Changes

- **Commit language**: All git commits for this feature use **English** messages (changed from the earlier Chinese-commit convention). This applies to all future commits in this project.

## Technical Design

### Architecture

```
Client (OpenAI SDK)
  │
  ▼
Sub2API Gateway (/v1/chat/completions)
  │
  ├── Scheduling / Billing / Failover (reused from OpenAI gateway)
  │
  ▼
QoderGatewayService
  │
  ├── EnsureAgentAndEnvironment (one-time, cached in account extra)
  ├── resolveQoderSession (Redis lookup or create + seed history)
  ├── SendUserMessage
  ├── StreamEvents (SSE with Last-Event-ID)
  │
  ▼
Qoder Cloud Agents API (https://api.qoder.com/api/v1/cloud)
```

### Session Stitching

- Key: `sha256(accountID | model | system | messages[:-1])` → `{sessionId, lastEventId}`
- After each turn: store `sha256(... | messages + assistantReply)` for next lookup
- TTL: 1 hour
- Storage: Redis (`qoder:conv:{accountID}:{hex}`)

### Provisioning

- Agent + Environment created once per account on first request
- Redis SetNX lock (`qoder:provision:{accountID}`, TTL 30s) prevents concurrent creation
- IDs persisted in account `extra` JSON: `qoder_agent_id`, `qoder_env_id`

### Billing

- Qoder SSE stream does not include usage/token data
- Tokens estimated locally: `runes / 4` (min 1) for both prompt and completion
- Documented as approximate

### Models

Static list of 15 Qoder model aliases served from `/v1/models`:
`auto`, `ultimate`, `performance`, `efficient`, `lite`, `cmodel`, `qmodel_preview`, `qmodel_latest`, `qmodel`, `kmodel_latest`, `kmodel`, `gm51model`, `dmodel`, `dfmodel`, `mmodel`

### Frontend

- Platform registered with rose color theme
- Account creation: PAT-only account type (no OAuth flow)
- All platform filter arrays, badge components, and i18n strings updated

## Files Changed

### Backend (Go)
- `internal/domain/constants.go` — PlatformQoder constant
- `internal/service/domain_constants.go` — service-level alias
- `internal/model/error_passthrough_rule.go` — AllPlatforms
- `internal/handler/admin/group_handler.go` — platform oneof validation
- `internal/pkg/qoder/` — client, events, models (new package)
- `internal/service/account.go` — IsQoder, GetQoderApiKey, GetQoderBaseURL, IsOpenAICompatible
- `internal/service/qoder_session_map.go` — Redis session stitching
- `internal/service/qoder_provision.go` — Agent/Environment provisioning
- `internal/service/qoder_gateway_service.go` — chat completions bridge
- `internal/service/openai_gateway_service.go` — qoderService field + redis param
- `internal/service/openai_gateway_chat_completions.go` — platform dispatch
- `internal/server/routes/gateway.go` — platform compatibility
- `internal/handler/openai_gateway_handler.go` — platform resolution
- `internal/service/openai_gateway_scheduling.go` — normalizePlatform
- `internal/service/scheduler_snapshot_service.go` — snapshot platforms
- `internal/service/account_service.go` — TestCredentials no-op
- `internal/handler/gateway_handler.go` — Models fallback
- `internal/service/admin_group.go` — default models list
- `cmd/server/wire_gen.go` — redisClient wiring

### Frontend (Vue 3 + TypeScript)
- `src/utils/platformColors.ts` — full rose theme
- `src/types/index.ts` — GroupPlatform, AccountPlatform unions
- `src/components/common/PlatformIcon.vue` — SVG icon
- `src/components/common/PlatformTypeBadge.vue` — badge classes
- `src/components/common/GroupBadge.vue` — badge classes
- `src/components/account/CreateAccountModal.vue` — platform button, PAT type, placeholders
- `src/components/account/EditAccountModal.vue` — default base URL
- `src/views/admin/GroupsView.vue` — platform options + colors
- `src/components/admin/account/AccountTableFilters.vue` — filter
- `src/components/admin/ErrorPassthroughRulesModal.vue` — platform list
- `src/views/admin/ops/components/OpsDashboardHeader.vue` — platform list
- `src/views/admin/ChannelsView.vue` — platformOrder
- `src/i18n/locales/{en,zh}/admin/accounts.ts` — platforms + types
- `src/i18n/locales/{en,zh}/admin/overview.ts` — groups.platforms
