# AI Relay OpenAI Platform Design

## Goal

Add a generic `OpenAI` platform to the standalone AI Relay plugin. OpenAI-compatible upstreams will use this platform, and upstream path differences will be handled exclusively through route-level URL path mappings.

## Scope

- Add an `openai` platform descriptor to the plugin platform registry and admin selector.
- Support transparent forwarding for `OpenAI` routes under `/v1/*`, including endpoints not currently listed by the plugin.
- Preserve HTTP method, request body, query string, end-to-end headers, upstream status, and response body.
- Apply an optional route mapping to the requested endpoint path. A mapped target replaces the path while retaining the configured Base URL scheme and host.
- Leave routes without a mapping on the existing path-resolution behavior.
- Keep Agnes image request/response conversion unchanged.
- Do not modify the main-site frontend.

## Architecture

The adapter registry will expose a protocol capability on each platform descriptor. Agnes remains an image adapter with its current conversion path. OpenAI is registered as a transparent adapter and does not build or parse image payloads.

The plugin service will register a wildcard `/v1/{path...}` relay route in addition to the existing explicit routes. The wildcard handler will load the configured route, validate that the route uses the transparent OpenAI adapter, resolve the endpoint with `ResolveRouteEndpointURL`, copy the incoming request to the upstream, and stream the upstream response back unchanged. Explicit existing routes continue to use their current handlers so Agnes behavior and backward compatibility are preserved.

The mapping resolver will receive the endpoint path without the leading `v1/` and will continue accepting stored keys with or without `v1/`. No mapping means the endpoint is appended to the Base URL as before; a mapping means the mapped path is used directly under the Base URL host.

## Error Handling

- Unknown platforms return the existing unsupported-platform error.
- Invalid Base URLs or invalid mapping targets return the existing upstream/configuration errors.
- Transparent proxy connection failures return an OpenAI-compatible `502 upstream_error` response.
- The wildcard route must not bypass authorization or route lookup.

## Testing

- Registry tests verify both Agnes and OpenAI descriptors.
- Handler tests verify OpenAI wildcard forwarding for an unmapped endpoint, mapped endpoint, query string, request body, and response status/body.
- Handler tests verify Agnes image conversion remains active.
- Mapping tests verify the existing no-mapping and `v1/`-prefixed compatibility behavior.
- Frontend tests verify the OpenAI platform appears through the platform API and can be selected in the route form.
- Run the plugin backend test suite, frontend unit tests, typecheck, and build.

## Non-Goals

- No OpenAI-to-Agnes request conversion.
- No Responses-to-Chat conversion.
- No changes to the main-site page or menu configuration.
- No per-upstream platform adapters.
