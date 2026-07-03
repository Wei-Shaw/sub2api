# Add WS Account Health Metrics

## Why

OpenAI WS accounts can become slow when a reused upstream connection starts failing preflight checks. The current account list only shows total latency, P90, and first-token latency, so operators cannot tell whether slowness comes from connection acquisition, queueing, payload size, or model generation.

## What Changes

- Add a short-lived in-memory WS connection health penalty to account scheduling.
- Record WS connection metrics in `usage_logs`.
- Surface connection-side metrics in account performance summaries.

## Impact

- A recent WS preflight failure lowers the affected account's scheduling priority for a short window without changing its stored priority.
- Non-WS requests keep null metric fields.
- Existing usage log reads remain compatible after the migration adds nullable columns.
