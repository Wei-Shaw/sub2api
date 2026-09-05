# AI Relay Account Proxy Design

## Goal

Allow an account proxy selected in main-site account management to control the
AI Relay plugin's connection to its platform upstream. The first implementation
applies to Agnes and must be reusable by later relay adapters.

## Scope

- Apply the account's primary proxy to AI Relay upstream requests.
- Support every AI Relay upstream operation through the same proxy selection:
  image generation, image editing, model listing, and chat completions.
- Preserve direct upstream access when the account has no proxy.
- Reject missing, disabled, expired, or invalid referenced proxies.
- Keep proxy credentials out of browser traffic and main-to-plugin headers.

The first version does not implement backup proxies, automatic direct fallback,
or the main site's proxy fallback policy.

## Architecture

The main backend remains responsible for selecting the account. When the
selected account targets an AI Relay plugin URL, it sends the account's proxy
identifier in the reserved internal header `X-Sub2api-Proxy-Id`.

The plugin service resolves that identifier from the shared PostgreSQL
`proxies` table. It creates or reuses an HTTP client whose transport uses the
resolved HTTP, HTTPS, SOCKS5, or SOCKS5H proxy. The relay handler obtains this
client before contacting the adapter's upstream endpoint.

Proxy credentials never leave the two backend services. Only the numeric proxy
identifier crosses the internal request boundary.

## Request Flow

1. The main gateway selects an account and reads its `proxy_id`.
2. For an AI Relay target, the gateway connects directly to the plugin service
   instead of applying the account proxy to the internal plugin hop.
3. The gateway overwrites `X-Sub2api-Proxy-Id` with the selected account's proxy
   ID, or removes it when the account has no proxy.
4. The plugin validates and resolves the proxy ID from the shared database.
5. The plugin selects a proxy-aware or direct HTTP client.
6. The adapter builds the platform request and the selected client sends it to
   Agnes.
7. The plugin converts the platform response to the OpenAI-compatible response.

## Components

### Main Gateway

- Detect AI Relay targets by normalized plugin route URL, not model name.
- Attach the reserved proxy ID header from the selected account.
- Prevent account proxy routing from being applied to the internal plugin hop.
- Remove any caller-supplied reserved proxy header before setting the trusted
  value.

### Main-Site Plugin Reverse Proxy

- Strip `X-Sub2api-Proxy-Id` from browser-originated `/plugins/*` requests.
- Continue attaching existing trusted principal headers where required.

### Plugin Proxy Repository

- Resolve a proxy by numeric ID from the shared `proxies` table.
- Return only active, non-expired proxy configurations.
- Keep database access behind a small interface so tests can use a fake
  resolver and future storage changes do not affect relay handlers.

### Plugin HTTP Client Factory

- Build transports for supported proxy protocols using the repository result.
- Reuse clients by stable proxy configuration identity to avoid creating a new
  connection pool for every request.
- Keep direct and proxied clients separate.
- Never fall back to direct access after a configured proxy fails.

### Relay Handler

- Resolve one client per incoming request before platform dispatch.
- Use the same client for image generation, image editing, models, and chat.
- Keep adapters responsible only for platform URL and payload conversion.

## Security

- The reserved proxy header is backend-only and must be overwritten or removed
  at every public ingress.
- The plugin service port must remain internal and must not be exposed as a
  public browser endpoint.
- Proxy usernames and passwords are loaded only inside the plugin process and
  must not appear in errors or logs.
- Invalid proxy IDs must not select another proxy or silently use direct access.

## Error Handling

The plugin returns an OpenAI-compatible error response:

- `400 invalid_request_error` for a malformed proxy ID header.
- `502 upstream_error` when the configured proxy is missing, disabled, expired,
  unsupported, or cannot connect to the platform upstream.
- Existing upstream status and response handling remains unchanged after a
  connection succeeds.

Errors may identify the proxy ID but must not contain proxy credentials.

## Testing

- Main gateway attaches the selected account proxy ID only for AI Relay targets.
- Main gateway removes spoofed internal proxy headers.
- The internal plugin hop bypasses the account proxy.
- Plugin repository resolves active proxies and rejects missing, disabled, and
  expired proxies.
- HTTP, HTTPS, SOCKS5, and SOCKS5H transport construction is covered.
- Direct requests remain direct when no proxy ID is present.
- Image generation, image editing, models, and chat use the resolved client.
- Proxy failures never fall back to a direct connection.

## Extensibility

Proxy resolution and client selection sit outside Agnes-specific code. New AI
Relay adapters automatically inherit the same account proxy behavior when they
use the shared relay handler execution path. Backup proxy and fallback behavior
can later be added inside the proxy resolver/client factory without changing
adapter interfaces.
