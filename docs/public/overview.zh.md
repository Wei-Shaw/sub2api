# 概览

本服务是一个 AI API 网关。你只需要一个 API Key 和一个 Base URL，就能使用你已经
熟悉的请求格式——Anthropic Messages、OpenAI Chat Completions、OpenAI Responses
以及 Google Gemini，它们全部由同一个域名提供。

如果你的代码已经在调用 Anthropic、OpenAI 或 Gemini，只需要改两处：Base URL 和
Key。其余一切不变。

## Base URL

```
{{SITE_ORIGIN}}
```

该域名下有三个前缀：

| 前缀 | 协议 | 示例 |
| --- | --- | --- |
| `/v1` | Anthropic 与 OpenAI 兼容 | `POST /v1/messages`、`POST /v1/chat/completions` |
| `/v1beta` | Google Gemini | `POST /v1beta/models/{model}:generateContent` |
| `/backend-api/codex` | Codex 直连 | `POST /backend-api/codex/responses` |

部分 OpenAI 风格路径同时挂在根路径上，不带 `/v1` 前缀——`POST /responses`、
`GET /models`、`POST /messages/count_tokens`——因此硬编码了无版本路径的客户端
也能正常工作。

## Key 与分组

决定一个请求能做什么的有两件事：

- **你的 API Key** 标识身份并承载额度。它以 `sk-` 开头，在控制台的 **API 密钥**
  页面创建。
- **Key 所属的分组** 决定由哪个上游平台承接——Anthropic、OpenAI、Grok、Google，
  或按模型路由的复合分组——以及适用哪种计费倍率。

未分配分组的 Key 会被 `403` 拒绝，除非管理员明确允许未分组 Key 调度。如果你看到
*"API Key is not assigned to any group"*，请让运营方为其分配分组。

选择端点时分组很关键：Anthropic 平台的分组提供 `/v1/messages`；OpenAI 平台的分组
提供 `/v1/chat/completions`、`/v1/responses`、`/v1/embeddings` 以及图像、视频端点；
Google 分组提供 `/v1beta`。复合分组按请求体中的 `model` 字段分发，因此一个 Key
可以触达多个平台。

## 支持的能力

- **文本** —— Messages、Chat Completions、Responses，三者均支持流式。
- **Token 计数** —— `POST /v1/messages/count_tokens`，不计费。
- **向量嵌入** —— `POST /v1/embeddings`。
- **图像** —— 同步、异步与批量。见 [API 参考](/docs/api-reference)。
- **视频** —— 生成、编辑、续写，以及状态与内容轮询。
- **音频** —— TTS、STT 与自定义音色。
- **搜索** —— Grok 分组上的网页搜索与 X 搜索。

## 接下来读什么

- [快速开始](/docs/quickstart) —— 一分钟内跑通第一个请求。
- [认证](/docs/authentication) —— 该发哪个请求头，以及为什么 query 参数不可用。
- [API 参考](/docs/api-reference) —— 端点清单。
- [计费与用量](/docs/billing-and-usage) —— 通过 API 读取倍率与消耗。
- [错误](/docs/errors) —— 各协议的响应结构，以及各状态码的含义。
