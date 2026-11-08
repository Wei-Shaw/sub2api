# GPT-5.5 retirement on ChatGPT-backed Codex accounts

OpenAI retired GPT-5.5 from Codex with ChatGPT sign-in on October 14,
2026 at 19:00 UTC (October 15 at 03:00 in Asia/Shanghai). Official API access
is unaffected. See the [official migration notice](https://learn.chatgpt.com/docs/models#gpt-55-retirement).

Sub2API applies this distinction to the upstream account, not the key used by
clients to authenticate to Sub2API:

- OpenAI OAuth and SetupToken accounts, including credential shadows, cannot
  serve GPT-5.5. Normal, sticky, and compact scheduling exclude these targets.
- API-key accounts retain GPT-5.5 support. In mixed groups, only eligible API-key
  upstreams can serve it. Their actual model availability remains authoritative.
- Public model lists and Codex catalogs omit models whose targets are retired on
  the group's ChatGPT accounts and have no supporting API-key account. This also
  filters stale/pinned catalog entries before calculating the client ETag.
- Explicit administrator mappings to a supported replacement remain valid.
  Aliases mapped *to* GPT-5.5 are excluded from ChatGPT account routing.
  Remote catalogs are projected to public aliases before retirement filtering,
  using the same single mapping step as routing. Source metadata and shared
  upstream caches are preserved, including when source IDs are also alias keys.
- Unsupported requests return the existing `model_not_found` error with migration
  guidance when the availability diagnosis identifies a retired subscription
  target. Client chat requests are not silently rewritten to a replacement.

## Administrator migration

Review group/account mappings, client defaults, custom agents and scheduled tasks
that select GPT-5.5. OpenAI recommends GPT-6.1 Sol for eligible paid plans or
GPT-6 Luna for Free/Go in the desktop app, subject to model availability. Check
availability on the upstream account before selecting a replacement.

The global `gateway.openai_compact_model` setting remains compatible with API-key
upstreams. If that fallback resolves to retired GPT-5.5 on a ChatGPT account, it
is not applied; compact requests keep their selected model. A compact-only
account mapping targeting retired GPT-5.5 is ineligible for scheduling: update
that explicit mapping. No stored settings are automatically rewritten.

Changing the public registry, deleting GPT-5.5 pricing/history, or globally
mapping GPT-5.5 to a new model is unnecessary and would affect valid API usage.

Regression tests use local fixtures and stub upstream responses. They do not
establish real subscription access to a replacement model.
