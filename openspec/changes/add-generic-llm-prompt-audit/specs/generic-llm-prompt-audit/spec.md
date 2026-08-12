## ADDED Requirements

### Requirement: Generic LLM audit endpoints
The system SHALL support `generic_llm` audit endpoints using an administrator-configured external OpenAI-compatible Chat Completions base URL, token, and model. Existing endpoints without an engine type MUST retain Qwen3Guard behavior.

#### Scenario: Existing endpoint remains compatible
- **WHEN** a persisted endpoint has no `engine_type`
- **THEN** the system treats it as `qwen3_guard` and preserves the existing request and response parser behavior

#### Scenario: Generic endpoint is selected
- **WHEN** an enabled endpoint has `engine_type=generic_llm`
- **THEN** the system sends the audit request through the generic LLM adapter and does not use the Qwen3Guard response parser

### Requirement: Strict structured audit result
The generic LLM adapter MUST request and validate a versioned JSON result containing safety, categories, confidence, evidence, and a short reason. The server MUST reject malformed, oversized, unsupported-version, or semantically invalid results rather than infer a decision from prose.

#### Scenario: Valid generic result
- **WHEN** the provider returns a schema-version-1 JSON object with valid enums, categories, and confidence between 0 and 1
- **THEN** the system normalizes it into the existing Prompt Audit result model

#### Scenario: Provider returns prose
- **WHEN** the provider returns natural-language prose instead of one valid JSON object
- **THEN** the scan fails with the existing invalid-response classification

#### Scenario: Provider wraps one JSON object in a fence
- **WHEN** the provider returns exactly one complete JSON object inside a Markdown code fence
- **THEN** the system MAY strip the fence and MUST apply the same strict semantic validation

### Requirement: Server-owned enforcement
The system MUST derive allow, warn, or block behavior from administrator policy and validated model observations. It MUST NOT accept a provider-selected enforcement action as authoritative.

#### Scenario: Prompt attempts to force allow
- **WHEN** audited content instructs the audit model to return an allow action but the validated observations identify an enabled unsafe category above threshold
- **THEN** the server derives the configured warning or blocking decision independently of any model-supplied action

#### Scenario: Unknown unsafe category
- **WHEN** the model reports unsafe content with an unknown category
- **THEN** the system records a stable anonymized unknown-category identifier and does not silently downgrade the result to allow

### Requirement: Keyword and LLM composition policies
The system SHALL support `keyword_first`, `llm_only`, and `combined` composition modes between deterministic keyword auditing and generic LLM auditing. New generic configurations MUST default to `keyword_first`.

#### Scenario: Keyword-first explicit block
- **WHEN** composition mode is `keyword_first` and deterministic keyword policy produces a block
- **THEN** the final decision is block and the system does not disclose the prompt to the generic LLM endpoint

#### Scenario: Keyword-first no block
- **WHEN** composition mode is `keyword_first` and deterministic keyword policy does not block
- **THEN** the generic LLM result participates in the final decision

#### Scenario: Combined results disagree
- **WHEN** composition mode is `combined` and keyword and LLM decisions have different severities
- **THEN** the system returns the highest-severity normalized decision while retaining each engine's independent event semantics

#### Scenario: LLM-only mode
- **WHEN** composition mode is `llm_only`
- **THEN** the generic LLM decision is evaluated without using keyword results for that audit decision

### Requirement: Enforcement stages and failure policy
Generic LLM auditing SHALL support shadow, warn, and block stages plus explicit fail-open or fail-closed handling for synchronous transport and validation failures. Defaults MUST be shadow and fail-open.

#### Scenario: Shadow detects unsafe content
- **WHEN** stage is `shadow` and the generic LLM derives a block-level result
- **THEN** the request proceeds and the shadow decision is recorded for evaluation

#### Scenario: Warn detects unsafe content
- **WHEN** stage is `warn` and the generic LLM derives a block-level result
- **THEN** the request proceeds and a warning-level audit event is recorded

#### Scenario: Block detects unsafe content
- **WHEN** stage is `block` and the generic LLM derives a block-level result
- **THEN** the request is rejected before account selection, billing, or upstream network side effects

#### Scenario: Fail-open timeout
- **WHEN** a synchronous generic LLM scan times out and `failure_policy=fail_open`
- **THEN** the request proceeds and the audit failure is observable

#### Scenario: Fail-closed invalid response
- **WHEN** a synchronous generic LLM scan returns invalid output and `failure_policy=fail_closed`
- **THEN** the request is rejected using the existing prompt-guard invalid-response envelope

### Requirement: Cost and sampling controls
The system SHALL enforce per-endpoint input limits, output-token limits, timeouts, and deterministic sample rates. Blocking stage MUST require a sample rate of one.

#### Scenario: Deterministic sampling
- **WHEN** a request is retried with the same stable request identifier and an endpoint sample rate below one
- **THEN** both attempts make the same sample-in or sample-out decision

#### Scenario: Partial blocking configuration rejected
- **WHEN** an administrator attempts to save stage `block` with a sample rate below one
- **THEN** the system rejects the configuration

### Requirement: Safe generic LLM prompt construction
The generic adapter MUST place policy instructions in a server-controlled system message, identify audited text as untrusted data, disable tool use, and prevent administrator extensions from replacing the mandatory output and safety contract.

#### Scenario: Audited content contains instructions
- **WHEN** audited content contains instructions to ignore the audit policy or invoke tools
- **THEN** those instructions remain inside the untrusted content boundary and no tool call is executed

#### Scenario: Administrator adds policy guidance
- **WHEN** an administrator configures additional audit guidance
- **THEN** the system appends bounded guidance without removing the built-in schema and enforcement instructions

### Requirement: Endpoint probing
The administration API SHALL probe a generic endpoint using fixed benign and unsafe inputs and MUST validate both transport and structured semantic output without exposing stored token plaintext.

#### Scenario: HTTP succeeds but schema is invalid
- **WHEN** a probe receives HTTP success with a response that violates the generic audit schema
- **THEN** the probe reports failure with sanitized invalid-response diagnostics

#### Scenario: Probe succeeds
- **WHEN** both fixed probe inputs produce valid structured results
- **THEN** the probe reports engine type, model, latency, schema version, and sanitized decision summaries

### Requirement: Generic audit observability
The system SHALL record engine type, endpoint ID, model, schema version, confidence, stage, policy version, latency, and provider token usage when available. It MUST NOT persist endpoint tokens, raw prompts, unrestricted reasoning, or unredacted evidence.

#### Scenario: Provider returns usage
- **WHEN** a generic provider includes standard token usage in its response
- **THEN** the system records normalized token counts with the audit event or metrics

#### Scenario: Evidence contains sensitive text
- **WHEN** model evidence contains sensitive or excessive text
- **THEN** the system applies existing redaction and length limits before persistence or API exposure

### Requirement: Generic endpoint failover
Generic LLM endpoints SHALL participate in the existing prioritized endpoint failover policy. Retryable transport failures MAY advance to the next compatible endpoint; invalid semantic results MUST follow the configured invalid-response behavior and MUST NOT be silently treated as safe.

#### Scenario: First endpoint is rate limited
- **WHEN** the first compatible endpoint returns a retryable rate-limit response and another enabled compatible endpoint exists
- **THEN** the scan attempts the next endpoint according to existing failover ordering

#### Scenario: All endpoints fail
- **WHEN** all eligible endpoints fail or return invalid results
- **THEN** synchronous enforcement follows the configured failure policy and asynchronous jobs follow existing retry and terminal-failure rules

### Requirement: No recursive public-gateway auditing
The first release MUST use external OpenAI-compatible audit endpoints and MUST NOT route generic audit calls through sub2api's public text gateway using a normal API key.

#### Scenario: Administrator configures local public gateway
- **WHEN** an endpoint configuration resolves to the current sub2api public gateway and would re-enter audited routes
- **THEN** endpoint validation or outbound destination checks reject the configuration as recursive or unsafe
