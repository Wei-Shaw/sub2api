# Anthropic image dimension guard

Clients can resend a large image in conversation history or in a `tool_result`
long after it was first attached. Claude rejects a request with more than 20
images when any image exceeds the many-image dimension limit (3000px in the
reported error). Retrying the same conversation does not repair it. A client
option such as `full_resolution` can bypass its own image compression.

Enable an outbound guard in `config.yaml`:

```yaml
gateway:
  anthropic_image_guard_enabled: true
```

The default is **false**, including for existing installations. Disabling it
restores the previous forwarding behavior. No client prompt changes are needed.

## Behavior

The guard runs before constructing the outgoing HTTP request. It inspects all
Messages image blocks, including historical `tool_result.content`, and bounds
inline images to a 3000px longest edge while preserving aspect ratio. The same
conservative bound applies below the 20-image threshold so adding an image later
does not invalidate the earlier images. It does not split, drop, reorder or
summarize images, and it does not add retries.

Small images and text-only requests remain unchanged. Only the `source.data`
and `source.media_type` fields of oversized images are replaced. Text, tool-use
IDs, tool inputs, cache controls and signed thinking blocks are not rewritten.
PNG transparency is preserved; JPEG is re-encoded at quality 90. Oversized GIF
and WebP are sent as PNG using the decoded first frame (Claude also uses only
the first frame of animations). Original files on the client are never modified.

Enabled routes:

- Anthropic Messages through the standard Gateway request builder (including
  OAuth/setup-token and API-key accounts).
- Chat Completions/Responses converted to Messages by that same builder.
- Anthropic API-key passthrough.
- OpenAI-platform native Anthropic-protocol forwarding.
- Vertex requests routed through the standard Gateway request builder.

Separate Bedrock and Antigravity forwarding, token-counting endpoints and
non-Anthropic output protocols are not covered by this change.

## Bounds and failures

To bound raster allocations, encoded input must fit a 5 MiB decoded-byte budget;
a raster to resize must have at most 16 million pixels. A request can resize at
most 32 million source pixels in total. Encoded output is bounded to 5 MiB. These
are local processing budgets, not claims about the upstream API's size limits.

When the guard cannot safely inspect or resize an image, it returns a local 400
`invalid_request_error` with its Messages path and does not construct an upstream
request. This includes invalid base64/headers, MIME mismatches, pixel/byte budget
violations and URL or file-ID sources. It deliberately does not fetch image URLs
(avoiding SSRF and remote image changes) or dereference provider file IDs.
Oversized JPEGs with EXIF metadata are rejected rather than risk stripping their
orientation; normalize the orientation or send PNG in that case.

This is dimension protection, not a complete image validator: in-limit payloads
are header-inspected rather than fully decoded. Total image-count limits, whole
request-body size, context length and corrupt compressed pixel data in an
in-limit image remain subject to existing/upstream validation.

Resizing changes image coordinates and may remove fine detail. Workflows that
require original-pixel coordinates should keep the guard disabled or supply
client-side crops with explicit coordinate mapping. Oversized images are
re-encoded on each outgoing request; this initial implementation does not add a
persistent image cache or retain user image data.

## Verification

```sh
cd backend
go test -tags=unit ./internal/service -run TestAnthropicImageGuard
```

Tests cover the exact edge boundary, landscape/portrait resizing, historical
images beyond the 20-image threshold, preservation of signed/tool content,
resource limits, disabled behavior, and the actual outgoing HTTP body from the
three request builders. No live provider request is needed.

Reference: https://platform.claude.com/docs/en/build-with-claude/vision
