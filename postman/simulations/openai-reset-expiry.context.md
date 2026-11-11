# OpenAI reset-credit expiry simulation

## Purpose

Run the real quota HTTP client and automatic expiry evaluator against a local HTTP fixture. All accounts, tokens and cards are synthetic. The fixture accepts only account headers beginning with `fixture-`, and the Go test rejects non-loopback targets.

## Covered behavior

- Expiry evaluation works with usage-based auto-reset disabled and the account excluded from scheduling.
- A successful redemption refreshes the available credit count.
- An explicit `nothing_to_reset` response with zero reset windows retains the card and does not create a second redemption after recreating the evaluator.
- A retryable HTTP 503 preserves the original card and redemption request ID.
- An unknown zero-window response remains unconfirmed; a later retry uses the original identity and can confirm success.

These are synthetic contract checks. They do not establish provider entitlement rules or guarantee that a real reset request will consume a card.

## Run locally

Start a fresh fixture for each run to clear its in-memory state:

```sh
postman simulation run postman/simulations/openai-reset-expiry.sim.yaml --port 4910 --no-history --output ndjson
```

In another terminal:

```sh
cd backend
TEST_OPENAI_RESET_EXPIRY_SIMULATION_URL=http://127.0.0.1:4910/codex go test -tags=unit,simulation ./internal/service -run '^TestOpenAIResetExpiryPostmanSimulation$' -count=1
```

The test checks the fixture's `/_requests` endpoint to verify actual usage, credit-detail and targeted-consumption calls, including stable identities across retries. Stop the fixture after verification. No production requests or workspace publishing are required.
