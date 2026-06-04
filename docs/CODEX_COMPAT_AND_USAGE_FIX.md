# Codex Responses Compatibility and OpenAI Usage Display Fix

This branch contains two related fixes for OpenAI-compatible Responses API usage.

## 1. Optional Codex-compatible Responses stream normalization

### Problem

Some upstream providers expose OpenAI-compatible `/v1/responses` streaming, but
their Server-Sent Events can differ from the event shape expected by Codex
clients. In practice, this can make Codex receive a stream that is accepted by
generic OpenAI-compatible clients but not consumed correctly by Codex CLI or
Codex editor integrations.

The compatibility issue is most visible when a public model name is mapped to a
different upstream model. The upstream may emit a valid Responses stream, but
the client expects a stricter OpenAI Responses event sequence and output item
shape.

### Fix

This branch adds an API-key-level compatibility option:

- `codex_responses_stream_compat`

When enabled, Sub2API normalizes OpenAI Responses SSE output for Codex clients.
The normalization is intentionally opt-in so existing users and existing
OpenAI-compatible clients keep the current behavior unless an API key explicitly
enables the compatibility mode.

The option is exposed in the API key editor and persisted with the API key. It
is also carried through the request context so the gateway can decide whether to
normalize the outgoing stream for that request.

### Why API-key-level

The incompatibility is client-specific rather than globally protocol-specific.
Keeping the switch on the API key allows one Sub2API deployment to serve normal
OpenAI-compatible clients and Codex clients at the same time.

## 2. OpenAI OAuth usage display as remaining quota

### Problem

The OpenAI OAuth account usage display mixed the meanings of the 5-hour and
7-day windows:

- The 5-hour value is already a remaining percentage.
- The 7-day value is a used percentage and must be converted to remaining quota.

Displaying both values with the same assumption made the UI confusing. For
example, a 5-hour value of `83` could be shown as `17%` remaining even though it
actually means `83%` remaining.

### Fix

The usage progress component now supports explicit display modes:

- `used`: display the value as used percentage.
- `remaining`: display the value directly as remaining percentage.
- `remaining-from-used`: convert used percentage to remaining percentage.

OpenAI OAuth usage now uses:

- 5-hour window: `remaining`
- 7-day window: `remaining-from-used`

The display text is consistently shown as remaining quota, and the color now
follows remaining-quota semantics: green when there is plenty remaining, amber
when it is low, and red when it is nearly exhausted.

## Tests

The branch adds frontend component coverage for both remaining display modes:

- `utilization=83` with `remaining` displays the original remaining value.
- utilization 87 with remaining-from-used displays the converted remaining value.

A production Docker build was also verified during deployment testing.
