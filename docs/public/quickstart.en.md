# Quickstart

## 1. Get a key

Register, sign in, open **API Keys**, and create one. The key is shown once and
starts with `sk-`. Store it the way you store any other secret — the dashboard
cannot show it to you again.

Check which group the key is assigned to on the same screen. The group decides
which endpoints below will answer.

## 2. Send a request

The base URL is `{{SITE_ORIGIN}}`. Pick the protocol that matches your key's
group.

### Anthropic Messages

```bash
curl {{SITE_ORIGIN}}/v1/messages \
  -H "x-api-key: $API_KEY" \
  -H "anthropic-version: 2023-06-01" \
  -H "content-type: application/json" \
  -d '{
    "model": "claude-sonnet-4-5",
    "max_tokens": 256,
    "messages": [{ "role": "user", "content": "Say hello in five words." }]
  }'
```

### OpenAI Chat Completions

```bash
curl {{SITE_ORIGIN}}/v1/chat/completions \
  -H "Authorization: Bearer $API_KEY" \
  -H "content-type: application/json" \
  -d '{
    "model": "gpt-5",
    "messages": [{ "role": "user", "content": "Say hello in five words." }]
  }'
```

### Google Gemini

```bash
curl "{{SITE_ORIGIN}}/v1beta/models/gemini-2.5-pro:generateContent" \
  -H "x-goog-api-key: $API_KEY" \
  -H "content-type: application/json" \
  -d '{
    "contents": [{ "parts": [{ "text": "Say hello in five words." }] }]
  }'
```

## 3. Point an SDK at it

Every official SDK takes a base URL. That is the only change.

### Python — `openai`

```python
from openai import OpenAI

client = OpenAI(api_key="sk-...", base_url="{{SITE_ORIGIN}}/v1")

response = client.chat.completions.create(
    model="gpt-5",
    messages=[{"role": "user", "content": "Say hello in five words."}],
)
print(response.choices[0].message.content)
```

### Python — `anthropic`

```python
from anthropic import Anthropic

client = Anthropic(api_key="sk-...", base_url="{{SITE_ORIGIN}}")

message = client.messages.create(
    model="claude-sonnet-4-5",
    max_tokens=256,
    messages=[{"role": "user", "content": "Say hello in five words."}],
)
print(message.content[0].text)
```

### Node — `openai`

```javascript
import OpenAI from 'openai'

const client = new OpenAI({
  apiKey: process.env.API_KEY,
  baseURL: '{{SITE_ORIGIN}}/v1',
})

const response = await client.chat.completions.create({
  model: 'gpt-5',
  messages: [{ role: 'user', content: 'Say hello in five words.' }],
})
console.log(response.choices[0].message.content)
```

### Claude Code

```bash
export ANTHROPIC_BASE_URL="{{SITE_ORIGIN}}"
export ANTHROPIC_AUTH_TOKEN="sk-..."
claude
```

### Codex CLI

```bash
export OPENAI_BASE_URL="{{SITE_ORIGIN}}/v1"
export OPENAI_API_KEY="sk-..."
codex
```

## 4. Stream

Streaming is passed through unchanged, so the SDK you already use handles it.
On raw HTTP, set `"stream": true` and read server-sent events:

```bash
curl -N {{SITE_ORIGIN}}/v1/chat/completions \
  -H "Authorization: Bearer $API_KEY" \
  -H "content-type: application/json" \
  -d '{
    "model": "gpt-5",
    "stream": true,
    "messages": [{ "role": "user", "content": "Count to five." }]
  }'
```

## 5. Check which models you can call

```bash
curl {{SITE_ORIGIN}}/v1/models -H "Authorization: Bearer $API_KEY"
```

The list reflects your key's group, not the whole platform, so treat it as the
authoritative answer to *"what can this key call?"*.

## When something fails

- `401` — the key is missing, wrong, or disabled. See
  [Authentication](/docs/authentication).
- `403` — the key has no group, or the group cannot serve that endpoint.
- `429` — a rate or concurrency limit, or your quota is exhausted.

Full shapes and status list: [Errors](/docs/errors).
