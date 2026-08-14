# 快速开始

## 1. 获取 Key

注册并登录后打开 **API 密钥** 页面创建一个。Key 只展示一次，以 `sk-` 开头。请像
对待其他密钥一样保存它——控制台无法再次显示完整值。

在同一页面确认 Key 分配到了哪个分组。分组决定下面哪些端点会响应。

## 2. 发起请求

Base URL 是 `{{SITE_ORIGIN}}`。选择与 Key 所属分组匹配的协议。

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

## 3. 让 SDK 指向这里

每个官方 SDK 都支持自定义 Base URL，这是唯一需要改的地方。

### Python —— `openai`

```python
from openai import OpenAI

client = OpenAI(api_key="sk-...", base_url="{{SITE_ORIGIN}}/v1")

response = client.chat.completions.create(
    model="gpt-5",
    messages=[{"role": "user", "content": "Say hello in five words."}],
)
print(response.choices[0].message.content)
```

### Python —— `anthropic`

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

### Node —— `openai`

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

## 4. 流式输出

流式响应原样透传，所以你现有的 SDK 无需改动即可处理。裸 HTTP 下设置
`"stream": true` 并读取 server-sent events：

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

## 5. 查看可调用的模型

```bash
curl {{SITE_ORIGIN}}/v1/models -H "Authorization: Bearer $API_KEY"
```

返回的列表反映的是你这个 Key 所属分组，而不是整个平台，因此它就是
*"这个 Key 能调什么"* 的权威答案。

## 请求失败时

- `401` —— Key 缺失、错误或已停用。见 [认证](/docs/authentication)。
- `403` —— Key 未分组，或分组无法提供该端点。
- `429` —— 触发速率或并发限制，或额度已耗尽。

完整结构与状态码清单见 [错误](/docs/errors)。
