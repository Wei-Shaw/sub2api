## 1. Configuration and Compatibility

- [x] 1.1 Add versioned endpoint fields for engine type, generic system guidance, confidence threshold, JSON output mode, sample rate, output-token limit, stage, failure policy, and composition mode across storage, active, public, and update DTOs
- [x] 1.2 Normalize missing endpoint fields to byte-compatible Qwen3Guard defaults and add backward-compatibility tests for existing persisted JSON
- [x] 1.3 Validate generic endpoint enums, numeric bounds, block-stage full sampling, bounded policy guidance, and unsupported combinations with stable error codes
- [x] 1.4 Preserve encrypted token update/clear/masked-sentinel behavior for both engine types and test that public/admin responses never expose plaintext

## 2. Generic LLM Protocol

- [x] 2.1 Define the immutable generic audit system contract and version-1 JSON response schema with enabled category definitions and an untrusted-content boundary
- [x] 2.2 Build OpenAI-compatible Chat Completions requests with tools disabled, bounded output, deterministic temperature settings, and optional JSON Schema response format
- [x] 2.3 Implement strict response-envelope and JSON-object parsing, including the single-code-fence allowance and output size limit
- [x] 2.4 Validate schema version, safety enum, normalized categories, confidence, evidence, and reason; reject prose, invalid values, and unsupported versions
- [x] 2.5 Normalize standard provider token usage and latency without retaining raw provider responses or unrestricted reasoning
- [x] 2.6 Add table-driven scanner tests for valid, malformed, fenced, oversized, injected, unknown-category, and usage-bearing responses

## 3. Scanner Dispatch and Policy

- [x] 3.1 Dispatch `PromptScanner.Scan` by endpoint engine type while preserving the existing Qwen3Guard request and parser path
- [x] 3.2 Implement server-owned generic observation-to-decision mapping using enabled categories, confidence threshold, severity policy, and stable unknown-category hashes
- [x] 3.3 Add keyword-first, LLM-only, and combined highest-severity composition at the security-audit coordination boundary without merging engine-specific side effects
- [x] 3.4 Ensure keyword-first block short-circuits external LLM disclosure and add no-outbound-call tests
- [x] 3.5 Implement shadow, warn, and block stage mapping while preserving pre-account-selection/pre-billing blocking invariants
- [x] 3.6 Implement fail-open and fail-closed synchronous error handling using existing protocol-specific unavailable/invalid response envelopes

## 4. Sampling, Failover, and Outbound Safety

- [x] 4.1 Implement stable request-ID-based deterministic sampling and ensure retries produce the same sample decision
- [x] 4.2 Apply input, output-token, and timeout limits to generic calls and retain existing concurrency and endpoint failover bounds
- [x] 4.3 Update failover compatibility so retryable generic endpoint failures advance correctly and invalid results never become implicit allows
- [x] 4.4 Reject endpoint destinations that resolve to the current sub2api public gateway or otherwise create recursive audited calls
- [x] 4.5 Extend outbound security tests for DNS rebinding, redirect rejection, local-address rejection, recursion rejection, timeout, rate limit, and oversized response behavior

## 5. Endpoint Probe and Administration API

- [x] 5.1 Extend endpoint probe requests and responses with engine-specific configuration and sanitized diagnostic metadata
- [x] 5.2 Probe generic endpoints with fixed benign and unsafe inputs, requiring valid structured semantic results for success
- [x] 5.3 Return sanitized engine, model, latency, schema-version, and decision summaries without token or raw-response disclosure
- [x] 5.4 Add handler and config-manager tests for save conflicts, invalid generic configurations, probe failures, and backward-compatible Qwen probes

## 6. Events, Runtime, and Metrics

- [x] 6.1 Extend Prompt Audit event/runtime metadata with engine type, model, schema version, confidence, stage, policy version, latency, and optional token usage
- [x] 6.2 Apply existing redaction and length limits to generic reason/evidence before persistence and API exposure
- [x] 6.3 Add metrics for generic requests, sampled-out requests, schema failures, fail-open decisions, fail-closed decisions, token usage, and per-engine latency
- [x] 6.4 Expose keyword-versus-LLM comparison data needed for shadow evaluation without storing raw prompts
- [x] 6.5 Add repository/API compatibility tests and a migration only if structured event columns are chosen over existing JSON metadata

## 7. Administration Console

- [x] 7.1 Add engine type selection to each Prompt Audit endpoint editor and retain Qwen3Guard as the default for existing rows
- [x] 7.2 Add generic model controls for system guidance, confidence, JSON mode, sample rate, output limit, stage, failure policy, and composition mode with inline validation
- [x] 7.3 Display external-disclosure, cost, prompt-injection, fail-open/fail-closed, and block-stage sampling warnings at the relevant controls
- [x] 7.4 Extend probe UI to display structured semantic validation and sanitized generic endpoint diagnostics
- [x] 7.5 Extend runtime and event views with engine/model/stage/confidence/usage metadata and keyword-versus-LLM shadow comparisons
- [x] 7.6 Add frontend unit tests for backward-compatible form hydration, validation, masked tokens, save payloads, probe states, and event rendering

## 8. End-to-End Verification and Rollout

- [ ] 8.1 Add HTTP gateway tests proving shadow and warn allow requests while block rejects before scheduling, concurrency acquisition, billing, and upstream writes
- [ ] 8.2 Add protocol-envelope tests for OpenAI, Anthropic, Gemini, SSE preflight, and Responses WebSocket fail-open/fail-closed behavior
- [ ] 8.3 Add asynchronous worker tests for generic retry, failover, terminal failure, lease recovery, and redacted event persistence
- [ ] 8.4 Add mixed keyword/LLM decision-matrix tests covering all composition modes, confidence boundaries, unknown categories, and deterministic short-circuiting
- [ ] 8.5 Document the recommended shadow-to-warn-to-block rollout, privacy implications, provider requirements, rollback procedure, and model-drift review process
- [ ] 8.6 Run focused security-audit backend tests, frontend Prompt Audit tests, lint/type checks, and the repository submission checks when implementation is ready to commit
