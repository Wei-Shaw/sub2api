# Remote Compaction: Native Capability And Server Summary

Support for ordinary Responses requests does not establish upstream compaction support. Sub2API selects native compaction from the actual account, mapped model, protocol, endpoint, and probe evidence. After an administrator explicitly enables summary compatibility, a target without native capability can use the existing forwarding path to generate a summary. The two capabilities remain independently identified.

This feature is still a review candidate. This document does not imply that it has been deployed or verified against every upstream.

## Configuration And Selection

| Input or field | Purpose |
| --- | --- |
| Group, composite route, account pool, and regular model mapping | Determine the authorization scope and concrete generation target; compaction grants no additional access |
| Protocol settings, per-protocol endpoints, and model catalog | Determine the actual upstream protocol and endpoint; a fixed Chat or Messages account cannot be forced to perform native Responses compaction |
| `openai_compact_mode` | Auto uses target evidence and default declarations, Force On permits native attempts, and Force Off excludes native execution; the policy does not prove upstream capability |
| `compact_summary_enabled` | Disabled by default and explicitly permits server-side summaries; independent from the native policy |
| `openai_compact_probe_targets` | Up to 32 target evidence records maintained by the server, separating v2 and legacy capabilities by model, protocol, endpoint, and configuration fingerprint; clients cannot write trusted results |
| Legacy account-level compact probe fields | Display-only history; they do not admit, reject, or rank a current target and do not disable summary fallback |
| `compact_model_mapping` | Retains its original OpenAI scope for initial legacy selection and some existing v2 error retries; summaries use the regular generation model |

Create, edit, and bulk edit reuse the shared settings. Providers such as DeepSeek can configure the native policy and summary compatibility independently. Bulk edit applies only fields selected individually. Native Force Off does not disable an explicitly enabled summary. OpenAI-specific compact model mapping keeps its existing scope.

Native v2 consists of `stream: true` plus `compaction_trigger` on the regular Responses path; legacy compaction uses `/responses/compact`. Evidence from one class never promotes eligibility for the other. Once an account is selected, the resolved protocol still constrains execution, and historical probe evidence cannot override the current fixed protocol or model catalog.

Scheduling reuses the existing default, advanced, load-batch, sticky, and failover mechanisms. Candidate filtering does not issue remote catalog requests per account. Verified native support takes precedence over unknown; unknown retains controlled attempt eligibility without being shown as supported. Capacity, rate limiting, and capability errors remain distinct.

## Probing And UI

In the existing connection test, select a concrete model and Native Compaction v2 Probe. Fixed Chat Completions and Anthropic Messages accounts show the incompatibility reason and disable that probe without disabling ordinary tests. Aggregated accounts are evaluated by concrete model.

Probing reuses actual target resolution, account authentication, proxy, and request headers. Support is recorded only after receiving a consumable compaction item and a normal terminal event; HTTP 200, ordinary text, an empty payload, multiple items, or truncation are not valid evidence. A 401, 429, timeout, or 5xx response is not permanently interpreted as unsupported.

History shows the actual model, protocol, redacted endpoint, result, time, and error category. Credential source, endpoint, protocol, mapping, or other configuration changes invalidate evidence. Target records currently have no independent expiry mechanism; an upstream that changes capability at an existing endpoint must be probed again, and historical results are not a permanent guarantee. Settings updates continue to use the existing snapshot/outbox flow and do not provide read-after-write consistency for every in-flight request.

Provider default declarations apply only to their corresponding default endpoints. DeepSeek default declarations and custom-relay probes are shown separately, and capability must not be inferred from the model name. Profile maintainers should review declarations when an upstream changes capability.

## Summary And Subsequent Restoration

1. The ingress validates and restores Sub2API-owned payloads before entering the existing protocol conversion path. Native ciphertext retains its original format and ownership constraints and is not removed to make a retry succeed.
2. Summary generation performs one tool-free inference with an account selected from the same authorization pool and the regular model mapping. It reuses authentication, concurrency, quota, conversion, and usage attribution without recursively calling this gateway.
3. Only a successful, non-empty, non-truncated completion is wrapped as one Sub2API-owned `compaction.encrypted_content`; v2 returns the corresponding SSE item and complete terminal state, and legacy returns the corresponding JSON.
4. When that item is returned later, it is restored as ordinary context and is not elevated into system or developer instructions. A later compaction includes the prior summary and subsequent history.

The format uses the `s2cmp1.v1.` prefix and standard-library AES-GCM. A separate random 32-byte key is stored as `compaction_summary_key_v1` in the existing `security_secrets` store. It does not reuse JWT or TOTP keys and does not depend on Redis TTL or process-local memory. A payload is bound to the user and authorized group; changing an API key within the same scope does not change ownership. Cross-user, cross-group, tampered, unknown-version, and missing-key payloads are rejected.

Online key rotation is not supported. The process caches the key after its first load, so replacing the database row and rolling the deployment would make old and new instances or historical payloads incompatible. Recovery must retain the original key.

Summaries lose information and add one regular inference of latency and usage. They do not alter native OpenAI ciphertext or authentication formats, and Sub2API-owned ciphertext is never sent to an upstream for decryption. Keys, summaries, and complete payloads must not enter diagnostic logs.

## Input And Recovery Limits

- Only one text summary is performed. The final model must have a known context window; the budget includes the input estimate, 8192 output tokens, and a 1024-token safety margin. Missing metadata or an insufficient budget fails explicitly instead of silently truncating input or splitting it into multiple billed requests.
- Image, file, audio, and video inputs are currently rejected for summaries even when the target model supports them. Unrestored hidden history referenced only by `previous_response_id`, or unreadable third-party native ciphertext, cannot be summarized without source material; failure preserves the original history.
- A summary is limited to 256 KiB and the encoded envelope to 2 MiB; larger results fail explicitly.
- Database backup and restore must cover the summary key. It cannot be deleted while old sessions still require it, and a payload cannot be moved to another upstream without this gateway.

## Rollout And Withdrawal

First deploy read support while summary issuance remains disabled for accounts, then enable it for selected accounts. Before deployment, inventory the actual `compact_summary_enabled` values; code defaults alone do not prove that no account already has it enabled.

Withdrawal after enablement should disable new issuance while retaining read support. An older version that cannot read Sub2API-owned payloads is not a safe rollback target; the rollback version must pass old-payload readback, ordinary continuation, and restart recovery.

Isolated tests cover the three protocols, first and repeated compaction, ownership and tamper rejection, account switching, key persistence, multiple instances, continuation after issuance is disabled, and usage attribution. A fake upstream does not prove summary quality with real models or compatibility with every client. Final acceptance still requires an isolated real-client test without a local compaction override; retain the existing recovery configuration for affected sessions until that passes.
