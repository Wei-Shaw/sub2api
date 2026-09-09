# OpenAI OAuth Auto-Provisioning

Sub2API can replenish primary OpenAI OAuth capacity on demand by asking `turb-gpt-free-register` to register more accounts. The configured target is a hard ceiling, not a prewarming floor: an idle pool below the target does not purchase email/SMS verification resources. Replenishment is driven by recent OpenAI OAuth usage and short-lived real capacity-denial signals. It can also send errored OAuth accounts to Turb for Codex reauthorization and persist the returned credentials back into the original Sub2API account.

## Configuration

Configure these values in the admin system settings:

- Enable auto provisioning.
- Set the healthy-account target, polling interval, worker count, and email source.
- Set the five-hour request and token capacity per OAuth account. Calibrate both values from the actual account plan and observed provider quota; they are not inferred from percentage-only quota snapshots.
- Set the Turb base URL and its WebUI `X-Auth-Code`.
- Set the Sub2API callback URL, normally ending in `/api/v1/integrations/openai/auto-provision/callback`.
- Set a callback secret. The same value must be set as `SUB2API_AUTOMATION_CALLBACK_SECRET` in Turb.
- Enable reauthorization separately when errored OAuth accounts should be repaired. Reauthorization is dispatched by Sub2API's existing backend OAuth token-refresh cycle; the account-list auto-refresh button only refreshes the web list and is not the worker.

Secrets are write-only in the settings API. Sending an empty secret leaves the stored value unchanged.

## Machine endpoints

Turb exposes these authenticated endpoints using the existing `X-Auth-Code`:

- `POST /api/automation/provision`
- `POST /api/automation/reauthorize`

Turb calls these Sub2API callback endpoints with `X-Sub2API-Automation-Secret`:

- `POST /api/v1/integrations/openai/auto-provision/callback`
- `POST /api/v1/integrations/openai/auto-provision/reauthorization/callback`
- `POST /api/v1/integrations/openai/auto-provision/reauthorization/completion`

Registration callbacks contain only request status and counts. Reauthorization callbacks contain the complete OAuth `callback_url` plus `session_id`; Sub2API parses the authorization `code` and `state` from that URL, then exchanges and validates the credentials. They do not contain access or refresh tokens. Sub2API checks the account email and ChatGPT account identity before updating credentials and clearing the account error.

## Delivery behavior

Sub2API stores pending request IDs and processed event IDs in the settings repository. A request is not dispatched again while it is pending. Terminal callbacks are idempotent, and a failed registration batch is followed by a fresh deficit calculation on the next polling cycle.

The coordinator reads successful OAuth usage from the most recent five hours. It calculates required capacity as `max(ceil(requests / configured_request_capacity), ceil(tokens / configured_token_capacity))`, then subtracts effective usable capacity. A capacity-denied user signal does not masquerade as a request count; it only reserves at least one additional slot when the measured usage has not yet exposed the shortage. The configured target is a hard ceiling, and each dispatch is still capped at 100 accounts; the next polling cycle continues after the terminal callback. Quota-exhausted accounts do not consume the target's provisionable-capacity ceiling. The signal is best-effort and is not allowed to delay or fail the user's gateway request.

The worker callback secret must be configured before enabling the feature. Do not put OAuth codes, tokens, or callback secrets in logs, job metadata, or support messages.
