# 错误

## 三种结构，各随协议

错误会按你调用的那个协议的结构返回，因此你现有的客户端不需要为本网关写特例解析。

### Anthropic 路径 —— `/v1/messages` 及网关端点

```json
{
  "type": "error",
  "error": {
    "type": "permission_error",
    "message": "..."
  }
}
```

`error.type` 取值为 `authentication_error`、`permission_error`、
`invalid_request_error`、`not_found_error` 或 `api_error`。

### OpenAI 兼容路径

```json
{
  "error": {
    "message": "...",
    "type": "insufficient_quota",
    "param": null,
    "code": "insufficient_quota"
  }
}
```

### Gemini 路径 —— `/v1beta`

```json
{
  "error": {
    "code": 403,
    "message": "...",
    "status": "PERMISSION_DENIED"
  }
}
```

### 网关层拒绝

在到达协议处理器之前就被拦下的请求——例如 Key 请求头写法不合规——使用扁平结构：

```json
{
  "code": "api_key_in_query_deprecated",
  "message": "..."
}
```

## 状态码

| 状态 | 含义 | 该怎么做 |
| --- | --- | --- |
| `400` | 请求格式错误、参数超出范围，或请求体超过大小上限。 | 修正请求。读 `message`，它会指出问题字段。文本端点的请求体上限比图像、视频端点更严。 |
| `401` | 无 Key、未知 Key，或 Key 已停用。 | 检查请求头。见 [认证](/docs/authentication)。 |
| `403` | Key 未分组，或其分组不提供该端点。 | 让运营方分配分组，或改调你所在平台支持的端点。 |
| `404` | 路径不存在，或该功能在本部署中未开启。 | 查阅 [API 参考](/docs/api-reference)。 |
| `429` | 速率限制、并发限制，或额度耗尽。 | 退避后重试——但 `insufficient_quota` 除外，重试无用。 |
| `5xx` | 网关或上游故障。 | 带退避重试。若持续出现，问题不在你的请求。 |

## 重试

- 重试 `429` 与 `5xx`。不要重试 `400`、`401`、`403`——同样的请求只会同样失败。
- 使用带抖动的指数退避。大量客户端同步重试会把一次短暂抖动放大成一次故障。
- 把 `insufficient_quota` 当作终态。它状态码是 `429`，但成因是余额而非流量，等待
  再久也不会自行恢复。
- 流式响应可能在首字节之后失败。提前结束的流即使状态行是 `200` 也是失败；请按处理
  `5xx` 的方式处理被截断的流。

## 快速定位

1. `curl {{SITE_ORIGIN}}/v1/models -H "Authorization: Bearer $API_KEY"` ——
   若能正常返回，说明 Key 与分组没问题，问题在请求体或具体端点。
2. 若返回 `401`，Key 有问题。若 `403`，Key 需要分组。
3. 把你发送的模型名与该端点返回的列表比对。请求看起来正确却被拒绝，最常见的原因就是
   模型不在你所属分组的可用范围内。
