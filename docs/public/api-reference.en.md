# API reference

Base URL: `{{SITE_ORIGIN}}`. Every path below needs an API key — see
[Authentication](/docs/authentication).

Request and response bodies are the upstream vendor's own. This gateway does not
invent a schema: an Anthropic request body is an Anthropic request body. Read
each endpoint row as *"where do I send it"*, and the vendor's own reference as
*"what goes inside"*. The one gateway-specific endpoint is
`GET /v1/sub2api/billing`, documented in
[Billing and usage](/docs/billing-and-usage).

Availability depends on the platform of the group your key belongs to. A path
that your group's platform does not serve answers `403`, not `404`.

## Text

| Method | Path | Notes |
| --- | --- | --- |
| `POST` | `/v1/messages` | Anthropic Messages. Streaming with `"stream": true`. |
| `POST` | `/v1/messages/count_tokens` | Counts tokens. Checks quota, records no usage. Also at `/messages/count_tokens`. |
| `POST` | `/v1/chat/completions` | OpenAI Chat Completions. |
| `POST` | `/v1/responses` | OpenAI Responses. Also at `/responses`. |
| `POST` | `/v1/responses/{subpath}` | Responses subresources, e.g. cancel. |
| `GET` | `/v1/responses` | Retrieve a response. |
| `POST` | `/v1/embeddings` | OpenAI embeddings. |
| `GET` | `/v1/models` | Models your key can call. Also at `/models`. |

## Gemini

| Method | Path | Notes |
| --- | --- | --- |
| `GET` | `/v1beta/models` | List models. |
| `GET` | `/v1beta/models/{model}` | Retrieve one model. |
| `POST` | `/v1beta/models/{model}:generateContent` | Generate. |
| `POST` | `/v1beta/models/{model}:streamGenerateContent` | Streaming generate. |

Google-platform groups only. Errors here use Google's error shape.

## Images

| Method | Path | Notes |
| --- | --- | --- |
| `POST` | `/v1/images/generations` | Synchronous generation. |
| `POST` | `/v1/images/edits` | Synchronous edit. |
| `POST` | `/v1/images/generations/async` | Submit and get a task id. |
| `POST` | `/v1/images/edits/async` | Submit and get a task id. |
| `GET` | `/v1/images/tasks/{task_id}` | Poll one async task. |

Long generations are what async is for: submit, hold the `task_id`, poll. The
poll is a read and is not billed again.

### Batch images

| Method | Path | Notes |
| --- | --- | --- |
| `POST` | `/v1/images/batches` | Submit a batch. |
| `GET` | `/v1/images/batches` | List your batches. |
| `GET` | `/v1/images/batches/models` | Models available for batching. |
| `GET` | `/v1/images/batches/{id}` | Batch status. |
| `GET` | `/v1/images/batches/{id}/items` | Per-item status. |
| `GET` | `/v1/images/batches/{id}/items/{custom_id}/content` | One item's output. |
| `GET` | `/v1/images/batches/{id}/download` | Whole batch as one download. |
| `POST` | `/v1/images/batches/{id}/cancel` | Cancel pending items. |

A worked walkthrough with request bodies lives in the dashboard, at
**Batch Image Guide** — you need to be signed in to read it.

## Video

| Method | Path | Notes |
| --- | --- | --- |
| `POST` | `/v1/videos`, `/v1/videos/generations` | Start a generation. |
| `POST` | `/v1/videos/edits` | Start an edit. |
| `POST` | `/v1/videos/extensions` | Extend an existing video. |
| `GET` | `/v1/videos/{request_id}` | Status. Also `/v1/videos/generations/{request_id}` and the `edits` / `extensions` variants. |
| `GET` | `/v1/videos/{request_id}/content` | Download the result. |

Video is always asynchronous: start, poll status, then fetch content.

## Audio

| Method | Path | Notes |
| --- | --- | --- |
| `POST` | `/v1/tts` | Text to speech. |
| `POST` | `/v1/stt` | Speech to text. |
| `POST` | `/v1/custom-voices` | Create a custom voice. |
| `GET` | `/v1/custom-voices` | List custom voices. |
| `GET` | `/v1/custom-voices/{voice_id}` | Retrieve one. |
| `GET` | `/v1/custom-voices/{voice_id}/audio` | Fetch its sample audio. |

## Realtime and live

| Method | Path | Notes |
| --- | --- | --- |
| `GET` | `/v1/realtime` | Realtime session. |
| `POST` | `/v1/live` | Start a live call. |
| `GET` | `/v1/live/{call_id}` | Live sideband channel. |
| `POST` | `/backend-api/codex/realtime/calls` | Codex-direct live call. |

## Search

| Method | Path | Notes |
| --- | --- | --- |
| `POST` | `/v1/web_search` | Web search. Grok groups. |
| `POST` | `/v1/x_search` | X search. Grok groups. |
| `POST` | `/v1/alpha/search` | Also at `/alpha/search`. |

## Account

| Method | Path | Notes |
| --- | --- | --- |
| `GET` | `/v1/sub2api/billing` | Rate multipliers in effect for this key. |
| `GET` | `/v1/usage` | Your consumption. Optional `days`, 1–90. |

See [Billing and usage](/docs/billing-and-usage).
