## Context

The current routing model reduces upstream support to a binary Responses decision. That is sufficient for single-protocol upstreams, but it cannot represent an upstream that should receive each client protocol on its matching native endpoint. The result is unnecessary Chat-to-Responses or Responses-to-Chat conversion.

## Goals

- Preserve the inbound OpenAI text protocol for a dual-endpoint API-key upstream.
- Reuse existing endpoint capability and routing metadata.
- Fail configuration validation before persisting a known capability conflict.
- Preserve all existing modes and defaults.

## Non-Goals

- Do not provide byte-for-byte body passthrough; `openai_passthrough` remains responsible for that behavior.
- Do not change OAuth or setup-token routing.
- Do not add automatic Chat Completions probing.
- Do not change scheduler fallback or account failover behavior.

## Decisions

### D1: Add an explicit routing mode

`preserve_inbound` is stored in the existing `openai_responses_mode` field. For a Chat Completions ingress it selects the existing raw Chat forwarding path. For a Responses ingress it resolves as Responses-supported, preventing the existing Chat fallback.

This mode is an operator assertion that both upstream endpoints work. The existing `openai_capabilities` value must include `chat_completions`; a missing capability field retains the project's backward-compatible default of enabled.

### D2: Keep protocol routing separate from body passthrough

The existing `openai_passthrough` option controls request and response mutation, not endpoint selection, and applies to additional account behavior. Reusing it would silently change established passthrough semantics. The new mode only prevents cross-protocol conversion; ordinary policy, mapping, billing, and audit hooks remain active.

### D3: Do not add a Chat Completions network probe

The existing Responses probe sends a tool call and persists its result. Adding a second active probe would create another billable provider call and can produce false negatives when a model or provider only partially implements Chat Completions. Operators already declare Chat support through `openai_capabilities`, so the new mode reuses that stable contract.

## Risks

- A wrongly configured upstream can still reject one endpoint. The UI states the dual-endpoint requirement, bulk validation rejects embeddings-only conflicts, and normal upstream failover remains available.
- A legacy account without explicit `openai_capabilities` is treated as Chat-capable. This preserves current behavior and avoids silently disabling existing accounts.
