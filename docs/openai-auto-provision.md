# OpenAI OAuth Auto-Provisioning

Sub2API can replenish primary OpenAI OAuth capacity on demand by asking `turb-gpt-free-register` to register more accounts. The configured target is a hard ceiling, not a prewarming floor: an idle pool below the target does not purchase email/SMS verification resources. Replenishment is driven by recent OpenAI OAuth usage and short-lived real capacity-denial signals. It can also send errored OAuth accounts to Turb for Codex reauthorization and persist the returned credentials back into the original Sub2API account.

## Configuration

Configure these values in the admin system settings:

- Enable auto provisioning.
- Set the healthy-account target, polling interval, worker count, and email source.
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

The coordinator reads successful OAuth usage from the most recent five hours. A failed-capacity signal is retained for five minutes and counts distinct users, but each polling cycle is capped to a one-account step-up so a sequential burst cannot trigger a full target-sized purchase. Recent meaningful token traffic (at least 10,000 tokens in the window) can prewarm one replacement when the known quota headroom is already inside the reserve; quota-exhausted accounts do not consume the target's provisionable-capacity ceiling. Ordinary traffic with usable quota never creates accounts. The signal is best-effort and is not allowed to delay or fail the user's gateway request.

The worker callback secret must be configured before enabling the feature. Do not put OAuth codes, tokens, or callback secrets in logs, job metadata, or support messages.
