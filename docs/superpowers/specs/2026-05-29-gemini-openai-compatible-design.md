# Gemini OpenAI-Compatible `/v1beta/openai` Design

## Context

Google Gemini exposes an OpenAI-compatible API under `/v1beta/openai`. In sub2api, Gemini currently has two separate surfaces:

- Gemini native REST endpoints under `/v1beta/models...`.
- OpenAI Chat Completions compatibility through `/v1/chat/completions`, which routes Gemini groups through `GeminiMessagesCompatService`.

There is no sub2api route for `/v1beta/openai/*` today. Clients that follow Google's documented OpenAI SDK shape and set `base_url` to a `/v1beta/openai` prefix will post to `/v1beta/openai/chat/completions` and receive 404. The existing Gemini Chat Completions conversion path also does not preserve OpenAI `input_audio` content parts; unknown parts can become text instead of Gemini `inlineData`.

## Goals

- Add a Gemini-only OpenAI-compatible entrypoint at `/v1beta/openai`.
- Preserve the existing sub2api routing, account selection, billing, model mapping, failover, moderation, and ops logging behavior.
- Support OpenAI SDK clients that use `base_url=<sub2api>/v1beta/openai`.
- Cover the documented Gemini OpenAI-compatible prefix with either a working endpoint or an explicit unsupported-endpoint error.
- Support audio input in Chat Completions by converting OpenAI `input_audio` parts to Gemini `inlineData`.
- Update frontend key-usage guidance so users can discover the new OpenAI-compatible Gemini base URL.

## Non-Goals

- Do not make `/v1beta/openai` a generic alias for all platforms.
- Do not bypass sub2api's Gemini scheduling by blindly proxying to Google's `/v1beta/openai` upstream.
- Do not add OpenAI Responses, Realtime, or arbitrary future OpenAI endpoints under this prefix.
- Do not implement Gemini video generation until sub2api has a video-operation backend and billing path.
- Do not replace the existing Gemini CLI/native `/v1beta` guidance.

## Route Design

Register a new route group:

```text
/v1beta/openai
```

Supported endpoints:

```text
GET  /v1beta/openai/models
POST /v1beta/openai/chat/completions
POST /v1beta/openai/embeddings
POST /v1beta/openai/images/generations
```

The route group uses the same Google/Gemini API key auth style as native `/v1beta` routes:

- `Authorization: Bearer <api-key>`
- `x-goog-api-key: <api-key>`
- `key=<api-key>`
- `api_key=<api-key>`

The route group must require the API key's group platform to be `gemini`. If the group is missing or not Gemini, return an explicit JSON error rather than falling through to OpenAI or Anthropic routing.

Unsupported subpaths under `/v1beta/openai/*` return a clear OpenAI-style JSON error, for example:

```json
{
  "error": {
    "type": "invalid_request_error",
    "message": "Unsupported endpoint for Gemini OpenAI compatibility"
  }
}
```

Known official Gemini OpenAI-compatible paths that sub2api does not yet have backend support for, such as video operations, should be registered only if needed to return explicit unsupported errors. They must not fall through to unrelated OpenAI or Gemini native handlers.

## Chat Completions Flow

`POST /v1beta/openai/chat/completions` should reuse the existing Gemini Chat Completions path:

```text
Gateway.ChatCompletions
  -> GeminiMessagesCompatService.ForwardAsChatCompletions
  -> Gemini native generateContent / streamGenerateContent
```

This keeps existing behavior for:

- user/account concurrency limits
- Gemini account selection and failover
- model mapping
- channel restrictions
- content moderation
- usage and billing records
- ops error logging
- streaming and non-streaming response conversion

The new route should differ only in external path compatibility and platform guard semantics.

## Audio Input Conversion

OpenAI-compatible clients can send audio in Chat Completions content parts:

```json
{
  "type": "input_audio",
  "input_audio": {
    "data": "<base64>",
    "format": "wav"
  }
}
```

The converter must preserve this as a real Gemini media part:

```json
{
  "inlineData": {
    "mimeType": "audio/wav",
    "data": "<base64>"
  }
}
```

Supported format mapping:

```text
wav  -> audio/wav
mp3  -> audio/mpeg
m4a  -> audio/mp4
aac  -> audio/aac
flac -> audio/flac
ogg  -> audio/ogg
```

If `data` is missing, empty, or only whitespace, the converter should drop the audio part when other usable parts exist, matching the existing empty image handling pattern. If the message contains only an empty audio part, return a validation error rather than sending an empty Gemini `parts` list.

If `format` is missing or unsupported, return HTTP 400 with a specific error. Do not stringify unknown `input_audio` blocks into prompt text.

The likely implementation seam is:

- extend `apicompat.ChatContentPart` with an `InputAudio` field
- preserve audio parts through Chat Completions to Responses conversion
- preserve audio parts through Responses to Anthropic conversion
- teach `convertClaudeMessagesToGeminiContents` to convert audio blocks to `inlineData`

If preserving audio through the generic Anthropic bridge would create broad risk, a narrower Gemini-specific conversion helper may be used, but it must remain covered by tests and must not duplicate the whole gateway flow.

## Models Endpoint

`GET /v1beta/openai/models` returns OpenAI-compatible model-list JSON:

```json
{
  "object": "list",
  "data": [
    {
      "id": "gemini-2.5-flash",
      "object": "model",
      "created": 0,
      "owned_by": "google"
    }
  ]
}
```

The data source should reuse the existing Gemini model list path or curated fallback list. This endpoint should not require a separate upstream call shape unless existing model-list behavior already does so.

## Embeddings Endpoint

`POST /v1beta/openai/embeddings` should provide OpenAI-compatible embeddings for Gemini groups.

- accept OpenAI embeddings request shape
- convert to Gemini native `embedContent` or `batchEmbedContents`
- convert Gemini response to OpenAI embeddings response shape
- record input-token usage and zero output-token usage

This endpoint must not route to the OpenAI platform embeddings handler.

## Image Generation Endpoint

`POST /v1beta/openai/images/generations` should provide OpenAI-compatible image generation for Gemini image models.

Request handling:

- accept OpenAI `images.generate` request shape with `model`, `prompt`, optional `response_format`, and optional `n`
- require a Gemini image-capable model such as `gemini-2.5-flash-image` or `gemini-3.1-flash-image`
- convert the prompt into Gemini native `generateContent`
- request image output from Gemini using the same native image-generation path already used by Gemini image models
- preserve existing Gemini account selection, image billing, moderation, failover, and ops logging behavior

Response handling:

- return OpenAI image response shape with `data[].b64_json`
- support `response_format=b64_json`; if `response_format=url` is requested, return HTTP 400 unless a real URL-backed storage path exists
- support `n` absent or `n=1`; if `n>1`, return HTTP 400 unless the implementation deliberately fans out multiple Gemini calls with billing for each image

This endpoint must not route to the OpenAI platform images handler.

## Frontend Adaptation

Update the user key usage modal for Gemini groups.

Keep the existing Gemini CLI guidance unchanged:

```text
GOOGLE_GEMINI_BASE_URL=<baseRoot>/v1beta
GEMINI_API_KEY=<api-key>
```

Add an OpenAI-compatible Gemini tab with:

```text
OPENAI_BASE_URL=<baseRoot>/v1beta/openai
OPENAI_API_KEY=<api-key>
```

Also include a minimal Python OpenAI SDK example:

```python
from openai import OpenAI

client = OpenAI(
    api_key="<api-key>",
    base_url="<baseRoot>/v1beta/openai",
)
```

OpenCode guidance should not replace the existing Google provider example. If an OpenAI-provider example is added, label it as OpenAI-compatible Gemini and use `/v1beta/openai`.

Add or update Chinese and English i18n strings for:

- Gemini OpenAI-compatible tab label
- tab description
- usage note that this prefix is Gemini-only

## Error Handling

- Non-Gemini group: return an explicit platform error.
- Unsupported `/v1beta/openai/*` endpoint: return OpenAI-style unsupported-endpoint JSON.
- Unsupported audio format: return HTTP 400 and name the unsupported format.
- Empty audio-only message: return HTTP 400.
- Unsupported image `response_format` or `n>1`: return HTTP 400 with a specific message.
- Video endpoints and other known-but-unimplemented Gemini OpenAI-compatible endpoints: return an explicit unsupported-endpoint error.
- Upstream Gemini errors: preserve existing Gemini Chat Completions error mapping and failover behavior.

## Testing

Backend tests:

- route test: `/v1beta/openai/chat/completions` is registered and does not 404.
- platform guard test: non-Gemini groups are rejected for `/v1beta/openai/*`.
- chat routing test: Gemini OpenAI-compatible route reaches `GeminiMessagesCompatService.ForwardAsChatCompletions`.
- audio conversion test: `input_audio` with `format=wav` produces Gemini `inlineData.mimeType=audio/wav` and preserves base64 data.
- audio format validation test: unsupported `format` returns HTTP 400.
- empty audio validation test: audio-only empty payload returns HTTP 400.
- embeddings test: OpenAI embeddings request converts to Gemini embedding upstream and returns OpenAI embeddings shape.
- image generation test: OpenAI image generation request converts to Gemini image upstream and returns `b64_json`.
- image validation test: unsupported image `response_format=url` or `n>1` returns HTTP 400.
- unsupported endpoint test: `/v1beta/openai/videos` returns explicit unsupported error rather than 404 or wrong-platform routing.
- regression test: existing `/v1beta/models/{model}:generateContent` still works.
- regression test: existing Gemini `/v1/chat/completions` still works.

Frontend tests:

- Gemini key usage modal has an OpenAI-compatible tab.
- Generated Gemini OpenAI-compatible base URL is `<baseRoot>/v1beta/openai`.
- Existing Gemini CLI base URL remains `<baseRoot>/v1beta`.
- The OpenAI-compatible tab mentions Chat Completions, embeddings, and image generation, and notes that video is not available until backend support exists.
- i18n keys render for English and Chinese.

## Rollout Notes

This is backward compatible for existing clients because it only adds new routes and preserves existing Gemini CLI/native paths. The risky part is audio conversion through shared compatibility structures; tests should focus on ensuring existing text/image Chat Completions behavior is unchanged.
