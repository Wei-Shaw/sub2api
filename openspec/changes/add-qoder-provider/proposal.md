# Add Qoder Provider

## Summary

Add Qoder Cloud Agents as a new platform provider in Sub2API, bridging stateless OpenAI Chat Completions requests onto stateful Qoder Agent sessions using a single PAT token.

## Motivation

Users with Qoder subscriptions want to access their Qoder agents through the standard OpenAI-compatible API that Sub2API already provides for other platforms. Qoder's Cloud Agents API is stateful (Agent → Environment → Session → Events), requiring a translation layer.

## Design

### Account Model

- Platform: `qoder`
- Type: `apikey` (PAT token stored in `credentials.api_key`)
- Base URL: `credentials.base_url` (default: `https://api.qoder.com/api/v1/cloud`)
- Extra: `qoder_agent_id`, `qoder_env_id` (auto-provisioned)

### Request Flow

1. Client sends `POST /v1/chat/completions` with a Sub2API key bound to a qoder group
2. Scheduler selects an eligible qoder account
3. `QoderGatewayService.ForwardChatCompletions`:
   - Parses messages into system + turns
   - Ensures Agent/Environment exist (one-time provisioning with Redis lock)
   - Resolves session: Redis lookup by conversation hash, or create + seed history
   - Sends user message, streams SSE events back as OpenAI chat completion chunks
   - Stores updated session binding for next turn

### Session Stitching

Conversation hash: `sha256(accountID | model | system | all-messages-except-last)`
- Hit → reuse session with `Last-Event-ID` for incremental streaming
- Miss → create new session, seed with flattened history as a single user message

### Endpoints

| Public Endpoint | Behavior |
|----------------|----------|
| `/v1/chat/completions` | Full chat completions (stream + non-stream) |
| `/v1/models` | Static list of 15 Qoder model aliases |

### Billing

No upstream usage data available. Tokens estimated at `runes / 4`.

## Scope

- Exposes OpenAI Chat Completions only (no Responses, no Images, no WebSocket)
- No OAuth flow (PAT only)
- No quota display (Qoder API doesn't expose rate-limit headers)
- Reuses existing scheduling, failover, billing, and group infrastructure
