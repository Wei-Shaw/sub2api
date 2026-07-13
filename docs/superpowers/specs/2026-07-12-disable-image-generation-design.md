# Global Image Generation Disable Design

## Context

Sub2API v0.1.151 rejects an OpenAI `/responses` request as soon as it detects an
`image_generation` declaration while the API key group has
`allow_image_generation=false`. This group-level check runs before account
selection, so the existing account policy
`codex_image_generation_explicit_tool_policy=strip` cannot remove the tool in
time. Codex clients attach the tool to otherwise normal text requests, causing
those requests to fail with HTTP 403.

## Goal

Add a global, opt-in `DISABLE_IMAGE_GENERATION=true` setting that prevents
image generation without rejecting ordinary text-capable `/responses`
requests merely because the client attached an image tool.

## Configuration

- Add `DisableImageGeneration bool` to the top-level configuration with the
  mapstructure key `disable_image_generation`.
- Set its default to `false` so existing installations retain current behavior.
- Viper's existing environment mapping exposes the setting as
  `DISABLE_IMAGE_GENERATION`.
- The global setting takes precedence over group, channel, and account image
  settings when it is enabled.

## Request Behavior

### HTTP `/responses`

When global image generation is disabled, normalize the validated request body
before content moderation, image-intent classification, concurrency accounting,
session hashing, model mapping, or account selection:

1. Remove every `image_generation` entry from `tools`.
2. Remove image-generation declarations embedded in `input`.
3. Remove `tool_choice` when it explicitly selects image generation.
4. Preserve all unrelated request fields and tools.
5. Continue through the normal text request path using the normalized body.

Reuse the existing shared image-tool stripping implementation. The operation
must remain idempotent and must not require the request to be recognized as a
Codex client.

If the requested or mapped model is an image-only model such as `gpt-image-*`,
the request remains an image-generation intent and returns HTTP 403. Removing a
tool cannot turn an image-only model into a text model.

### WebSocket `/responses`

Apply the same normalization to the first `response.create` message before its
image-intent gate and to subsequent `response.create` messages at WebSocket
ingress. Forward the normalized payload. Text-capable turns continue; image-only
models are closed with the existing policy-violation response.

### Dedicated Image Endpoints

When global image generation is disabled, dedicated image endpoints remain
blocked regardless of group settings:

- `/v1/images/generations`
- `/v1/images/edits`
- equivalent unversioned image routes
- Grok media generation routes

These endpoints return HTTP 403 with the existing stable permission message.

### Existing Account Policy

The account-level `strip` policy remains supported. With the global setting
enabled it is redundant for `/responses`, because stripping occurs before
account selection. With the global setting disabled, account policy behavior is
unchanged.

## Code Structure

- `backend/internal/config/config.go`: define and default the global setting.
- `backend/internal/service/openai_codex_transform.go`: export a small wrapper
  around the existing raw-payload stripping implementation so handlers can use
  one canonical parser and mutation path.
- `backend/internal/handler/openai_gateway_handler.go`: normalize HTTP and
  WebSocket payloads before the early image gate and propagate the normalized
  body/message through the rest of each request.
- Dedicated image handlers: add the global check without changing the existing
  group check or response contract.
- `deploy/.env.example` and Compose examples: document and pass through
  `DISABLE_IMAGE_GENERATION`, defaulting to `false`.

No database migration is required.

## Error Handling And Observability

- Invalid JSON remains a 400 and is never silently rewritten.
- A stripping failure returns the existing invalid-request error path rather
  than forwarding an unverified payload.
- Log one structured event when a payload is changed, without logging request
  contents.
- Do not log when no image declaration is present.
- Image-only and dedicated endpoint rejections retain the stable message
  `Image generation is not enabled for this group` for client compatibility.

## Tests

Use test-first development and cover:

1. Configuration defaults to disabled=false and reads
   `DISABLE_IMAGE_GENERATION=true`.
2. HTTP `/responses` with a text model and image declarations is normalized and
   passes the early group gate when the group disallows images.
3. HTTP normalization preserves non-image tools and unrelated fields.
4. HTTP `/responses` with an image-only model still returns 403.
5. WebSocket first and subsequent messages receive equivalent normalization.
6. Dedicated OpenAI and Grok image endpoints return 403 when the global switch
   is enabled, even if the group allows images.
7. With the switch disabled, existing behavior and account-level policy tests
   remain unchanged.

Run focused handler, service, and config tests first, followed by the complete
backend Go test suite.

## Deployment And Rollback

Build a versioned custom image from v0.1.151 plus this patch. Do not overwrite
the upstream `latest` tag. Update production Compose to the immutable custom
tag and add `DISABLE_IMAGE_GENERATION=true`, then recreate only the Sub2API
application service.

Before deployment, record the current upstream image digest and preserve the
existing Compose file. Verify container health, ordinary `/responses` behavior,
image-only rejection, dedicated endpoint rejection, and absence of new startup
errors.

Rollback consists of restoring the previous immutable image digest, removing
the environment setting, and recreating only the application service. Database,
Redis, and account policy data are unchanged by the deployment.
