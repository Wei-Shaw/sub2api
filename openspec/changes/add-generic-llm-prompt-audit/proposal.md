## Why

The current Prompt Audit integration speaks OpenAI-compatible Chat Completions but only understands Qwen3Guard's specialized `Safety/Categories` response. Administrators need to use generally available LLMs for semantic auditing without losing the deterministic keyword checks, failover, event history, or pre-upstream blocking guarantees already present in the audit pipeline.

## What Changes

- Add a `generic_llm` audit engine alongside the existing `qwen3_guard` engine for OpenAI-compatible endpoints.
- Require generic models to return a strict, versioned JSON decision schema and validate it server-side.
- Add configurable composition modes for deterministic keyword results and generic LLM results: keyword-first, LLM-only, and combined highest-risk.
- Add server-owned category/action thresholds so model output cannot directly override enforcement policy.
- Add shadow, warning, and blocking enforcement stages, with explicit fail-open/fail-closed behavior for synchronous failures.
- Add endpoint configuration and probe support for engine type, system prompt, confidence threshold, output mode, and sampling.
- Record engine/model/policy metadata, latency, token usage when available, and redacted evidence without persisting raw prompts or credentials.
- Keep existing Qwen3Guard configurations behaviorally compatible; new generic LLM behavior is opt-in.

## Capabilities

### New Capabilities

- `generic-llm-prompt-audit`: Generic OpenAI-compatible LLM auditing, structured result validation, keyword/LLM composition, enforcement stages, failure policy, and observability.

### Modified Capabilities

None. The existing Prompt Audit change has not been archived into the main spec registry; this capability explicitly extends its implemented runtime contract without redefining that historical change.

## Impact

- Backend: `internal/securityaudit` endpoint configuration, scanner dispatch, result parsing, policy aggregation, probes, metrics, and event metadata.
- Frontend: Prompt Audit administration page endpoint editor, policy controls, validation, probe output, and runtime/event displays.
- Settings: backward-compatible additions to `prompt_audit_config`; existing encrypted token handling remains in place.
- External systems: OpenAI-compatible Chat Completions providers used as audit nodes.
- Security and operations: additional outbound prompt disclosure, model cost, latency, prompt-injection resistance, and model-drift monitoring requirements.
