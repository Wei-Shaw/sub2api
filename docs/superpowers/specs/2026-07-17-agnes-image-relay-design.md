# Agnes Image Relay Plugin Design

## Goal

Add a plugin-only relay that lets a Sub2API account use an Agnes image API key
while presenting the OpenAI Images generation request and response contract to
the main service. Do not add chat, model-list, image-edit, variation, or
non-Agnes routes.

The main account uses a plugin URL containing only a stable identifier:

```text
http://plugin-server:8091/plugins/ai-relay/agnes/<slug>
```

The plugin stores target configuration but never stores the account API key.
The main service forwards its account Authorization header and the plugin uses
that credential for the outgoing Agnes request.

## Scope

The initial adapter supports a single operation:

```text
POST /plugins/ai-relay/agnes/{slug}
```

It accepts an OpenAI Images generation payload and calls Agnes Image 2.1 Flash
at `POST https://apihub.agnes-ai.com/v1/images/generations`. The adapter returns
an OpenAI Images response:

```json
{
  "created": 1780000000,
  "data": [{"url": "https://...", "b64_json": null, "revised_prompt": null}]
}
```

The route has no external `/v1` suffix. Endpoint selection is internal to the
adapter rather than part of the main-account URL.

## Architecture

Create an `ai-relay` hosted plugin with three separable concerns:

1. A configuration store keyed by `(platform, slug)`. An Agnes configuration
   contains the enabled flag, target base URL, default target model, and model
   mapping. It contains no API key.
2. A generic relay handler that authenticates through the existing plugin-host
   mechanisms, resolves the route configuration, reads the forwarded bearer
   token, invokes a platform adapter, and writes an OpenAI-shaped result.
3. A platform-adapter registry. The first adapter is `agnes`; future adapters
   implement the same request translation and response normalization contract
   without changing route resolution or credential handling.

The plugin UI is limited to administrator configuration of Agnes routes. Users
and accounts are not managed by the plugin.

## Request Mapping

The external request shape follows the GPT Image generation request contract.
The adapter accepts the common fields below so existing GPT Image clients can
reuse their payloads.

| OpenAI field | Agnes behavior |
| --- | --- |
| `model` | Resolve through the route model map; default to `agnes-image-2.1-flash`. |
| `prompt` | Pass through unchanged. |
| `size` | Map square, landscape, and portrait values to Agnes `size: "1K"` plus `ratio`: `1:1`, `3:2`, or `2:3`. |
| `response_format` | Map `url` to `extra_body.response_format: "url"`; map `b64_json` to `return_base64: true`. |
| `n` | Create `n` independent Agnes requests, bounded by a configured maximum, then merge their results into one `data` array. |
| `quality` | Map `low`, `medium`, `high`, and `auto` to configurable Agnes resolution tiers; default to `1K`. |
| `style`, `background`, `moderation`, `output_format`, `output_compression`, `user` | Accept for request compatibility. They have no documented Agnes equivalent and do not alter the outgoing request. |

Unknown request fields are rejected with an OpenAI-style invalid-request error.
Unsupported values for accepted fields are rejected rather than silently
coerced. The adapter must not claim semantic equivalence for fields that Agnes
does not provide.

## Reliability And Errors

- Preserve the incoming `Authorization: Bearer` value only for the outgoing
  Agnes request; never log it or persist it.
- Reject missing credentials, an unknown/disabled slug, invalid JSON, missing
  `prompt`, invalid `n`, and unsupported response formats before calling
  Agnes.
- Use an image-generation timeout suitable for Agnes's documented 60--360
  second execution window.
- Convert transport errors and non-2xx Agnes responses into OpenAI-compatible
  error envelopes while retaining a safe upstream status/message for diagnosis.
- If one request in an `n` fan-out fails, fail the whole OpenAI request rather
  than returning a partial `data` array.

## Testing

Tests cover route resolution, no-key persistence, Authorization forwarding,
default model selection, model mapping, `size`/`quality` mapping, URL and Base64
responses, `n` fan-out, validation failures, and Agnes error normalization.
Registry tests prove a future platform can be registered without editing the
generic relay handler.

## Non-Goals

- GPT image edit, variation, batch, chat, responses, model-list, and stream
  endpoints.
- Image binary upload conversion; the Agnes generation endpoint supports prompt
  generation, while image-to-image remains outside this first compatible route.
- Claiming that GPT-only controls such as `style` or output compression produce
  identical Agnes output.
