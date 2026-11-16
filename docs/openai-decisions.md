# OpenAI Decisions

The OpenAI gateway accepts `POST /v1/decisions` and the `/decisions` alias.
Requests use the same Sub2API key authentication, group model restrictions,
concurrency limits, billing checks and account failover as other gateway APIs.

## Account setup

Use an **OpenAI API Key** account whose upstream supports `/v1/decisions`.
In the account's endpoint capabilities, enable **Decisions**. Existing accounts
with an explicit capability list must opt in; accounts without a capability list
are unrestricted. ChatGPT/Codex OAuth accounts are excluded from this endpoint.

Set the upstream base URL to `https://api.openai.com` or an appropriate compatible
provider. A base URL ending in `/v1` is also accepted. Model aliases work through
the existing channel and account mappings. Composite groups can select
`decisions` as the route's endpoint scope (migration 251).

## Request

```bash
curl http://127.0.0.1:8080/v1/decisions \
  -H "Authorization: Bearer $SUB2API_API_KEY" \
  -H 'Content-Type: application/json' \
  --data-binary '{
    "model": "gpt-6-luna",
    "input": "The customer reports two identical charges for one order.",
    "questions": [{
      "type": "predicate",
      "name": "duplicate_charge",
      "instructions": "Does the customer report a duplicate charge?"
    }]
  }'
```

The gateway preserves native `input`, `questions`, images, JSON evidence and
prompt-cache controls, and returns the upstream JSON without converting it to
Chat Completions or Responses. Decisions is synchronous; streaming is rejected.
The multimodal request-body limit applies, including for images in question
instructions. Prompt auditing covers shared evidence and question text.

## Billing

Reported input tokens include cache reads and writes. Billing splits these into
disjoint buckets to avoid charging cached tokens again as uncached input.
Input and cache pricing use the normal model/channel pricing and group
multipliers. Decisions output tokens are recorded but incur no output charge.

For the official `gpt-6-luna` rates, see the
[Decisions guide](https://developers.openai.com/api/docs/guides/decisions) and
[pricing](https://developers.openai.com/api/docs/pricing). Custom channel prices
continue to apply to input and caching.

Source changes require rebuilding and updating the running server/image.
Updating a checkout does not update an existing deployment.
