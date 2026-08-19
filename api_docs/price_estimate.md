# 价格评估 API

## 概览

该接口用于在提交真实生成任务前，预估指定图片生成模型的本次请求费用。

当前入口挂在 fal 兼容模型网关下：

- `POST /api/v1/model/{endpoint}/estimate_pricing`

接口需要有效的 Sub2API API Key。它只返回价格预览，不会创建上游任务、冻结余额、扣除余额或写入使用记录。

预估结果基于：

- API Key 所属分组和该分组可用的模型映射；
- 分组图片价格档位或渠道模型价格；
- 请求尺寸和可选 `quality`；
- 图片数量；
- 分组倍率或独立图片倍率。

如果真实提交前价格配置、分组倍率、模型映射或计费规则发生变化，最终实扣可能与本接口返回的预估值不同。

## 接口

```http
POST /api/v1/model/{endpoint}/estimate_pricing
Authorization: Bearer sk-...
Content-Type: application/json
```

路径参数：

| 字段 | 必填 | 说明 |
|------|------|------|
| `endpoint` | 是 | 模型 endpoint 路径，例如 `fal-ai/flux/dev`。API Key 所属分组必须直接支持该模型，或通过通配符模型映射支持该模型。 |

该路由即使在视频功能开关关闭时也可访问，因为它只是价格工具接口；但仍然需要 API Key 鉴权和分组绑定。

## 请求体

请求体必须是 JSON object。接口支持扁平请求体，也支持 fal 常见的 `input` 包裹格式。

扁平请求体：

```json
{
  "image_size": {
    "width": 800,
    "height": 800
  },
  "num_images": 2,
  "quality": "high"
}
```

`input` 包裹格式：

```json
{
  "input": {
    "image_size": "square_hd",
    "num_images": 1
  }
}
```

字段说明：

| 字段 | 必填 | 说明 |
|------|------|------|
| `image_size` | 条件必填 | 图片尺寸。可以是包含 `width` / `height` 的对象、`1024x1024` 这类尺寸字符串，或已知 fal 尺寸别名。 |
| `size` | 条件必填 | `image_size` 的别名；当 `image_size` 缺失时使用。 |
| `resolution` | 条件必填 | `image_size` 的别名；当 `image_size` 和 `size` 都缺失时使用。 |
| `width` | 条件必填 | 顶层宽度。当没有尺寸字段时与顶层 `height` 一起使用。必须是正整数。 |
| `height` | 条件必填 | 顶层高度。当没有尺寸字段时与顶层 `width` 一起使用。必须是正整数。 |
| `num_images` | 否 | 预估图片数量，必须是正整数，默认 `1`。 |
| `n` | 否 | `num_images` 的别名。 |
| `image_count` | 否 | `num_images` 的别名。 |
| `quality` | 否 | 可选质量档位，例如 `high`。当价格档位区分质量时参与匹配。 |

至少需要提供一种完整的尺寸来源。若尺寸以对象形式传入，必须包含正整数 `width` 和 `height`。

已知 fal 尺寸别名：

| 别名 | 分辨率 |
|------|--------|
| `square` | `512x512` |
| `square_hd` | `1024x1024` |
| `portrait_4_3` | `768x1024` |
| `portrait_16_9` | `576x1024` |
| `landscape_4_3` | `1024x768` |
| `landscape_16_9` | `1024x576` |

## 响应

```json
{
  "endpoint": "fal-ai/flux/dev",
  "billing_mode": "image",
  "pricing_source": "group",
  "tier": "1K",
  "resolution": {
    "width": 800,
    "height": 800
  },
  "image_count": 2,
  "unit_price": 0.1,
  "total_cost": 0.2,
  "rate_multiplier": 1.5,
  "estimated_price": 0.3
}
```

字段说明：

| 字段 | 说明 |
|------|------|
| `endpoint` | 被预估的模型 endpoint。 |
| `billing_mode` | 本次预估使用的计费模式，通常为 `image`；如果命中了渠道价格覆盖，可能体现解析后的计费模式。 |
| `pricing_source` | 价格来源，例如分组价格或渠道/模型价格。 |
| `tier` | 命中的价格档位，例如 `1K`、`2K`、`4K`。 |
| `resolution` | 归一化后的请求尺寸。 |
| `image_count` | 本次预估包含的图片数量。 |
| `unit_price` | 倍率前的标准单张价格。 |
| `total_cost` | 倍率前的标准费用：`unit_price * image_count`。 |
| `rate_multiplier` | 本次预估采用的实际扣费倍率。 |
| `estimated_price` | 倍率后的用户侧预估费用。 |

## 示例

使用明确尺寸对象预估：

```bash
curl -X POST "https://example.com/api/v1/model/fal-ai/flux/dev/estimate_pricing" \
  -H "Authorization: Bearer sk-..." \
  -H "Content-Type: application/json" \
  -d '{
    "image_size": { "width": 800, "height": 800 },
    "num_images": 2,
    "quality": "high"
  }'
```

使用 fal 尺寸别名预估：

```bash
curl -X POST "https://example.com/api/v1/model/fal-ai/flux/dev/estimate_pricing" \
  -H "Authorization: Bearer sk-..." \
  -H "Content-Type: application/json" \
  -d '{
    "input": {
      "image_size": "landscape_4_3",
      "n": 1
    }
  }'
```

## 错误响应

尺寸或请求体非法：

```http
HTTP/1.1 400 Bad Request
```

```json
{
  "error": {
    "type": "invalid_request_error",
    "message": "width is required"
  }
}
```

API Key 分组不支持该模型：

```http
HTTP/1.1 404 Not Found
```

```json
{
  "error": {
    "type": "not_found_error",
    "message": "group does not support model: made-up/provider/model"
  }
}
```

缺少或传入无效 API Key：

```http
HTTP/1.1 401 Unauthorized
```

```json
{
  "error": {
    "type": "authentication_error",
    "message": "Invalid API key"
  }
}
```

常见校验错误：

| HTTP | Type | 原因 |
|------|------|------|
| `400` | `invalid_request_error` | 请求体不是 JSON object。 |
| `400` | `invalid_request_error` | `width`、`height`、`num_images`、`n` 或 `image_count` 不是正整数。 |
| `400` | `invalid_request_error` | 请求尺寸无法匹配任何已配置价格档位。 |
| `401` | `authentication_error` | API Key 缺失或无效。 |
| `404` | `not_found_error` | API Key 所属分组不支持请求的 endpoint。 |

## 注意事项

- 客户端展示预估费用时应优先使用 `estimated_price`。
- `total_cost` 是倍率前标准费用，主要用于排查配置或展示公式。
- 该接口无副作用，可以安全重试。
- 成功拿到预估结果不代表真实生成请求已经获得容量预留；实际提交时仍可能因为无可用账号而失败。
