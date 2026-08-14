# Overview

This service is an AI API gateway. You get one API key, one base URL, and the
request formats you already use — Anthropic Messages, OpenAI Chat Completions,
OpenAI Responses, and Google Gemini — all served from the same host.

If your code already talks to Anthropic, OpenAI, or Gemini, you change two
things: the base URL and the key. Nothing else.

## Base URL

```
{{SITE_ORIGIN}}
```

Three prefixes live under that host:

| Prefix | Protocol | Example |
| --- | --- | --- |
| `/v1` | Anthropic and OpenAI-compatible | `POST /v1/messages`, `POST /v1/chat/completions` |
| `/v1beta` | Google Gemini | `POST /v1beta/models/{model}:generateContent` |
| `/backend-api/codex` | Codex direct access | `POST /backend-api/codex/responses` |

A few OpenAI-style paths are also served at the root, without the `/v1`
prefix — `POST /responses`, `GET /models`, `POST /messages/count_tokens` — so
clients that hardcode an unversioned path still work.

## Keys and groups

Two things decide what a request can do:

- **Your API key** identifies you and carries your quota. It starts with `sk-`
  and is created in the dashboard under **API Keys**.
- **The group your key is assigned to** decides which upstream platform serves
  it — Anthropic, OpenAI, Grok, Google, or a composite group that routes per
  model — and which billing rate applies.

A key that is not assigned to any group is rejected with `403` unless the
administrator has explicitly allowed ungrouped keys. If you see *"API Key is
not assigned to any group"*, ask the operator to assign it.

The group matters when you pick an endpoint: an Anthropic-platform group serves
`/v1/messages`; an OpenAI-platform group serves `/v1/chat/completions`,
`/v1/responses`, `/v1/embeddings` and the image and video endpoints; a Google
group serves `/v1beta`. Composite groups dispatch by the `model` field in the
body, so one key can reach several platforms.

## What is supported

- **Text** — Messages, Chat Completions, Responses, streaming on all three.
- **Token counting** — `POST /v1/messages/count_tokens`, billed as nothing.
- **Embeddings** — `POST /v1/embeddings`.
- **Images** — synchronous, asynchronous, and batch. See
  [API reference](/docs/api-reference).
- **Video** — generation, edits, extensions, plus status and content polling.
- **Audio** — TTS, STT, and custom voices.
- **Search** — web search and X search on Grok groups.

## Where to go next

- [Quickstart](/docs/quickstart) — first working request in about a minute.
- [Authentication](/docs/authentication) — which header to send, and why a
  query parameter will not work.
- [API reference](/docs/api-reference) — the endpoint list.
- [Billing and usage](/docs/billing-and-usage) — read your rate multiplier and
  your consumption from the API.
- [Errors](/docs/errors) — response shapes per protocol, and what each status
  means.
