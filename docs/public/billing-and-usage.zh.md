# 计费与用量

两个端点让客户端无需打开控制台就能回答 *"这次调用要花多少"* 和
*"我已经花了多少"*。

## 计费倍率

```bash
curl {{SITE_ORIGIN}}/v1/sub2api/billing \
  -H "Authorization: Bearer $API_KEY"
```

```json
{
  "object": "sub2api.key_billing",
  "schema_version": 1,
  "billing_scope": "token",
  "group_rate_multiplier": 1.0,
  "resolved_rate_multiplier": 0.8,
  "peak_rate_enabled": true,
  "peak_start": "09:00",
  "peak_end": "18:00",
  "peak_rate_multiplier": 1.5,
  "applied_peak_multiplier": 1.5,
  "effective_rate_multiplier": 1.2,
  "timezone": "Asia/Shanghai",
  "observed_at": "2026-08-14T10:30:00Z"
}
```

从最后一项往上读——`effective_rate_multiplier` 才是给你下一个请求定价的数字，其余
字段解释它是怎么算出来的：

| 字段 | 含义 |
| --- | --- |
| `billing_scope` | 计量对象。`token` 表示按 token 计费。 |
| `group_rate_multiplier` | Key 所属分组的基础倍率。 |
| `user_rate_multiplier` | 仅当你有个人覆盖倍率时出现。 |
| `resolved_rate_multiplier` | 应用个人覆盖后的倍率。 |
| `peak_rate_enabled` | 该分组是否对高峰时段单独定价。 |
| `peak_start`、`peak_end`、`timezone` | 高峰时段窗口，按分组所在时区。 |
| `peak_rate_multiplier` | 高峰加价系数。 |
| `applied_peak_multiplier` | 仅当当前处于窗口内时出现。 |
| `effective_rate_multiplier` | resolved × 生效高峰系数。当前实际倍率。 |

可选字段是省略而不是置为 null。没有 `user_rate_multiplier` 表示你没有个人覆盖；
没有 `applied_peak_multiplier` 表示当前不在高峰时段。

响应带 `Cache-Control: no-store`，因为高峰窗口开始或结束时生效倍率会变。需要时再读，
不要缓存一整天。

有两种情况会返回错误而非数据：

- `403` `permission_error` —— Key 未分组，无法解析倍率。
- `404` `not_found_error` —— 该部署运行在 simple 模式，根本没有计费模型。

## 消耗

```bash
curl "{{SITE_ORIGIN}}/v1/usage?days=7" \
  -H "Authorization: Bearer $API_KEY"
```

统计范围限于发起调用的这个 Key。`days` 可选，必须在 1 到 90 之间；超出范围返回
`400`：

```json
{
  "type": "error",
  "error": {
    "type": "invalid_request_error",
    "message": "Invalid days, allowed range is 1-90"
  }
}
```

响应包含该 Key 的总量以及按天的明细。用量统计是尽力而为采集的：若统计存储短时不可用，
该端点仍会返回基础字段而不是让请求失败，因此明细缺失应理解为 *"暂时取不到"*，
而不是零。

## 不计费的调用

- `POST /v1/messages/count_tokens` —— 校验额度与订阅，不记录用量，也不占用并发槽位。
- 轮询异步图像任务或视频状态。费用在提交时已计，读取结果免费。

## 额度耗尽

额度耗尽返回 `429`。在 OpenAI 兼容路径上使用 OpenAI 自己的结构，因此 SDK 的重试逻辑
能够识别：

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

请充值，或让运营方提高上限。在这两件事发生之前重试没有意义——见
[错误](/docs/errors)。
