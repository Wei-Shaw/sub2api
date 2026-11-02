# OpenAI Decisions

Clients call `POST /v1/decisions` with `model: "gpt-6-luna"` and the OpenAI
`input` / ordered `questions` contract. TypeSafe JEV remains at `/v1/systemone`.

OpenAI `apikey` and `upstream` accounts support the account credential field
`openai_decisions_protocol`. Its default is `openai`. Explicit capability lists
must include `decisions`. OAuth, PAT and Codex setup tokens are excluded.

For OpenRouter, select **OpenRouter** under **Decisions upstream protocol** in
Create, Edit or Bulk Edit. Configure the account's Base URL as
`https://openrouter.ai` (an existing `https://openrouter.ai/api/v1` also works)
and enter its key through the account form. Custom relay domains use the same
explicit protocol option and retain the outbound URL policy.

OpenRouter requests use `/api/alpha/decisions` and
`openai/gpt-6-luna-decisions`. Sub2API converts questions, typed choices, image
parts and answers; clients continue receiving the OpenAI schema. One inline
image is supported. Multi-image requests skip OpenRouter accounts and can use
native OpenAI accounts; requests with no eligible account return a clear error.
Failover always rebuilds the outbound body from the original client request.

Billing retains configured group/channel prices and multipliers. Otherwise the
OpenRouter Decisions model has its own fallback card: $0.10/M input, zero
output/cache charge. `usage.cost` never replaces the customer price. Requested
model and actual upstream model/endpoint are recorded separately.

Sources: [OpenRouter Decisions](https://openrouter.ai/docs/api/api-reference/alphadecisions/submit-a-decisions-request)
and [model](https://openrouter.ai/openai/gpt-6-luna-decisions).
