# 认证

每个网关请求都需要 API Key。网关按以下优先顺序接受三种请求头，因此你现有的客户端
无需改动即可使用。

## 支持的请求头

### `Authorization: Bearer` —— OpenAI 风格

```bash
curl {{SITE_ORIGIN}}/v1/chat/completions \
  -H "Authorization: Bearer sk-..."
```

scheme 大小写不敏感，`bearer` 同样有效。

### `x-api-key` —— Anthropic 风格

```bash
curl {{SITE_ORIGIN}}/v1/messages \
  -H "x-api-key: sk-..."
```

### `x-goog-api-key` —— Gemini CLI 兼容

```bash
curl "{{SITE_ORIGIN}}/v1beta/models/gemini-2.5-pro:generateContent" \
  -H "x-goog-api-key: sk-..."
```

只发一个即可。若同时存在，优先级为 `Authorization` > `x-api-key` >
`x-goog-api-key`。

## `api_key` query 参数已被拒绝

把 Key 放在 URL 中会被直接拒绝，返回 `400`：

```json
{
  "code": "api_key_in_query_deprecated",
  "message": "API key in query parameter is deprecated. Please use Authorization header instead."
}
```

这是有意为之。出现在 query string 中的 Key 会进入浏览器历史、代理日志和
referrer 头。请改用请求头。

## 缺少 Key

完全没有可识别的请求头时，网关返回 `401`：

```json
{
  "code": "API_KEY_REQUIRED",
  "message": "API key is required in Authorization header (Bearer scheme), x-api-key header, or x-goog-api-key header"
}
```

## 分组分配

认证只证明你是谁，本身并不授予访问权：Key 还必须分配到某个分组。未分配的 Key 会被
`403` 拒绝，并以所在协议自身的错误结构返回：

```json
{
  "type": "error",
  "error": {
    "type": "permission_error",
    "message": "API Key is not assigned to any group and cannot be used. Please contact the administrator to assign it to a group."
  }
}
```

运营方可以在系统设置中允许未分组 Key，此时该校验不会触发。如果你遇到它，需要在
运营侧解决，改代码没有用。

## 安全地使用 Key

- 把 Key 放在环境变量或密钥管理服务中，绝不要放进代码仓库或前端产物。出现在已发布
  客户端代码里的 Key 等于已经公开。
- 每个应用一个 Key，这样吊销其中一个不会牵连其他。
- 轮换顺序：先创建新 Key，部署，再删除旧 Key。
- 一旦怀疑泄露立即删除。删除对新请求即时生效。
- 控制台只在创建时展示 Key 的完整值。
