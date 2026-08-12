## Context

The implemented Prompt Audit module already extracts protocol-specific user input, chunks Unicode text, runs synchronous or queued scans, fails over across prioritized endpoints, records normalized events, protects endpoint tokens, and blocks before account selection or billing. Its `OpenAICompatibleScanner`, however, sends raw chunks as a user message and parses only Qwen3Guard's line-oriented response.

The existing content moderation subsystem separately supports deterministic keyword rules. This change must compose those deterministic results with semantic LLM results without merging their storage models or changing existing moderation side effects. Generic providers are untrusted external processors: audited text may contain prompt injection, responses may violate schema, and model behavior may drift.

## Goals / Non-Goals

**Goals:**

- Support ordinary OpenAI-compatible chat models as Prompt Audit scanners.
- Preserve existing Qwen3Guard configurations and endpoint failover behavior.
- Produce deterministic server-side enforcement from strictly validated model output.
- Compose keyword and LLM decisions through explicit administrator policy.
- Support safe shadow rollout before warning or blocking enforcement.
- Expose enough metadata to evaluate accuracy, latency, cost, and drift.

**Non-Goals:**

- Calling sub2api's public gateway recursively with an internal API key in the first release.
- Auditing model output or interrupting streams after upstream generation starts.
- Training, fine-tuning, or hosting an audit model.
- Replacing the existing keyword moderation engine, its logs, or its account penalties.
- Letting a model execute tools, fetch URLs, or choose the final enforcement action.
- Persisting raw prompts, API tokens, or unrestricted model reasoning.

## Decisions

### 1. Dispatch scanner behavior by endpoint engine type

Each endpoint gains `engine_type`, defaulting to `qwen3_guard` when absent. The scanner dispatches to either the existing Qwen parser or a new generic LLM adapter while retaining the same `PromptScanner` boundary and endpoint failover loop.

`generic_llm` endpoints use OpenAI-compatible Chat Completions. The first release supports external `base_url + token + model` nodes only. Reusing a sub2api API key through the public gateway is deferred because it requires a trusted internal-call marker, billing ownership rules, and recursion prevention across every audit entry point.

Alternative: infer engine type from the model name. Rejected because aliases and provider model names are not stable contracts.

### 2. Use a strict versioned JSON result contract

The generic adapter sends a fixed security system prompt, enabled category definitions, administrator policy text, and the audited content in a clearly delimited data message. It requests JSON Schema output when configured and provider-supported, while still accepting a plain JSON object for compatible providers.

Version 1 response:

```json
{
  "schema_version": 1,
  "safety": "safe|review|unsafe",
  "categories": ["jailbreak"],
  "confidence": 0.0,
  "evidence": [{"category": "jailbreak", "excerpt": "redacted short excerpt"}],
  "reason": "short explanation"
}
```

Unknown fields are ignored, but missing required fields, invalid enums, non-finite/out-of-range confidence, unknown schema versions, oversized output, and malformed JSON are invalid responses. Markdown fence stripping is allowed only around one complete JSON object; the server does not recover JSON from arbitrary prose.

Alternative: parse free-form prose. Rejected because blocking behavior would depend on heuristic parsing.

### 3. Keep enforcement policy server-owned

The model reports observations, not `allow/warn/block`. The server normalizes categories and derives `NormalizedResult` using enabled categories, per-category severity, and a configurable confidence threshold. Unknown categories are represented by stable hashes and cannot silently produce an allow decision when the model says `unsafe`.

The built-in policy prompt is immutable. Administrators may append bounded policy guidance, but cannot replace the system safety contract. Audited content is always labeled untrusted data and tool use is disabled.

Alternative: accept a model-returned `action`. Rejected because prompt injection or model drift could bypass administrator policy.

### 4. Compose keyword and LLM decisions explicitly

The policy supports:

- `keyword_first`: a deterministic keyword block is final; otherwise run the LLM.
- `llm_only`: skip keyword evaluation for this audit decision.
- `combined`: evaluate both and take the highest severity, preserving each engine's own event and side effects.

`keyword_first` is the default for newly configured generic endpoints. Keyword evaluation occurs before disclosure to the external LLM, so an explicit block avoids unnecessary external transmission. Existing Qwen-only behavior is unchanged.

The security-audit coordinator remains responsible for preserving the established priority of existing content-moderation blocks over Prompt Audit responses.

### 5. Separate evaluation stage from transport failure policy

Generic LLM rollout stages are:

- `shadow`: persist/measure the decision but never alter the client response.
- `warn`: persist a warning decision but allow the request.
- `block`: enforce server-derived block decisions before upstream side effects.

Synchronous transport or validation failures use `failure_policy=fail_open|fail_closed`. Default is `fail_open` for generic LLM endpoints. `fail_closed` is allowed only in `block` stage and preserves the existing 503 unavailable/invalid error envelopes. Asynchronous jobs continue using existing retry and terminal-failure behavior.

Alternative: reuse only `blocking_enabled`. Rejected because operators need a non-enforcing comparison phase distinct from async transport mechanics.

### 6. Bound cost and disclosure

Each generic endpoint supports `sample_rate`, `input_limit`, `timeout_ms`, `max_output_tokens`, and optional JSON Schema mode. Sampling is deterministic by request identifier so retries make the same decision. Blocking stage requires `sample_rate=1`; partial sampling is valid only for shadow/warn observation.

The request never includes stored credentials, account details, or prior hidden system prompts. Existing prompt snapshot extraction decides which user-authored text is eligible. Event storage remains redacted and records provider-reported token usage only when available.

### 7. Extend existing configuration without a database migration

New fields live inside `prompt_audit_config` JSON. Missing fields normalize to Qwen-compatible defaults. Public/admin DTOs expose no token plaintext. Configuration version conflict handling and encrypted token persistence remain unchanged.

Endpoint probes execute the selected adapter against a fixed benign and fixed unsafe sample, validate the response contract, and return sanitized diagnostics. A successful HTTP response with invalid semantic output is a failed probe.

### 8. Make drift observable

Events and runtime metrics add engine type, model, result schema version, confidence, stage, policy version, provider latency, and token usage when supplied. Raw model reasoning is not stored; evidence and reason are length-limited and passed through existing redaction.

Administrators can compare keyword and LLM decisions in shadow mode. Promotion to blocking remains a manual configuration action.

## Risks / Trade-offs

- [Prompt injection causes a false allow] -> Use an immutable system contract, strict output validation, server-owned action mapping, no tools, and fail-closed as an explicit option.
- [External disclosure of sensitive prompts] -> Keep generic endpoints opt-in, show a disclosure warning, apply existing extraction/redaction boundaries, and avoid sending deterministic keyword blocks.
- [Latency degrades synchronous requests] -> Default to shadow/fail-open, enforce endpoint timeouts, preserve failover limits, and expose latency metrics.
- [Model cost becomes unbounded] -> Add deterministic sampling, input/output limits, usage metrics, and forbid partial sampling in blocking mode.
- [Providers lack JSON Schema support] -> Support strict plain-JSON fallback while applying the same parser; surface capability in endpoint probes.
- [Model behavior drifts] -> Store policy/model/schema versions and provide shadow comparison before manual promotion.
- [Keyword and LLM side effects conflict] -> Keep engine logs and side effects independent; compose only normalized severity at the coordinator boundary.
- [Existing Qwen endpoints change behavior] -> Default missing `engine_type` to `qwen3_guard` and retain the old request and parser byte-for-byte where practical.

## Migration Plan

1. Deploy backward-compatible config parsing and scanner dispatch with generic LLM disabled.
2. Add the administration controls and endpoint probes.
3. Configure generic endpoints in `shadow + fail_open`, initially with a low sample rate.
4. Compare decisions against keyword results and manually reviewed events.
5. Increase sampling, then promote selected policies to warn or block.
6. Roll back by disabling generic endpoints or restoring `engine_type=qwen3_guard`; no database rollback is required.

## Open Questions

- Should per-category confidence thresholds be included in the first release, or should one endpoint-wide threshold ship first?
- Should the first UI expose `llm_only`, given that it can bypass deterministic keyword checks, or reserve it for API configuration?
- Which provider usage envelope variants should be normalized initially beyond standard OpenAI `usage`?
