# GPT-Image-2 图像生成

使用 `gpt-image-2` 模型生成图像，支持文生图与参考图图生图，异步任务模式。

> **接入地址**
> 国内直连：`https://api.chenshuapi.top`
> 香港线路：`https://api.chenshuapi.com`
> 两者接口完全一致，按网络情况任选其一。

> **鉴权**
> 所有接口均使用 Bearer Token 认证，在请求头中添加 `Authorization: Bearer YOUR_API_KEY`。
> API Key 请在控制台创建，不要写入前端代码、公开仓库、日志或提示词。

> **重要：`size` 只决定宽高比，不决定像素总量。**
> 服务端会保留你请求的宽高比，把总像素拟合到约 `1.57 Mpx`（`1536x1024` 预算），因此**只有 `1536x1024` 与 `1024x1536` 两种尺寸会被原样交付**，其余请求尺寸都会得到一张等比缩放后的图。
> 请求 `2048x2048`、`3840x2160` 这类大尺寸**不会**得到更大的图。请先看「尺寸对照表」再决定请求值。

**特性**

* 通过 `model` 参数选择 `gpt-image-2`
* 提交后立即返回任务 ID（HTTP `202`），再轮询任务查询接口获取图片地址
* 支持文生图（`/v1/images/generations/async`）与图生图（`/v1/images/edits/async`）
* 按张计费：一次调用生成一张图、扣一张的费用，与请求尺寸、质量档位无关

## Authorizations

| 项目 | 值 | 说明 |
| --- | --- | --- |
| 认证方式 | Bearer Token | 请求头 `Authorization: Bearer YOUR_API_KEY` |
| 接口地址 | `https://api.chenshuapi.top` 或 `https://api.chenshuapi.com` | 两条线路等价 |
| 内容类型 | `application/json` 或 `multipart/form-data` | 图生图上传本地文件时使用 multipart |

```http
Authorization: Bearer YOUR_API_KEY
```

## 创建生图任务

```http
POST /v1/images/generations/async
```

### Body

| 字段 | 类型 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| `model` | string | 是 | — | 模型名称，本文档对应 `gpt-image-2` |
| `prompt` | string | 是 | — | 图像生成的文本描述 |
| `size` | string | 否 | `1536x1024` | 请求尺寸，格式 `宽x高`。**只决定宽高比**，实际交付像素见「尺寸对照表」 |
| `quality` | string | 否 | `auto` | 生成质量档位：`low`、`medium`、`high`、`auto` |
| `n` | integer | 否 | `1` | 生成数量，**固定为 `1`**，请勿传入其他数值 |
| `response_format` | string | 否 | `url` | 返回格式，固定使用 `url`，任务完成后返回图片地址 |

`quality` 三档均真实生效，可用响应里的 `usage.output_tokens_details.image_tokens` 核对：`1024x1024` 下 `low = 272`、`medium = 1056`、`high = 4160`。对延迟和成本敏感的场景推荐 `low`。

### 尺寸对照表

下表为服务端当前实际交付规则（同一提示词、`quality=low`、`n=1` 实测）：

| 请求 `size` | 比例 | 实际交付 `size` | 是否原样交付 |
| --- | --- | --- | --- |
| `1536x1024` | 3:2 | `1536x1024` | **是** |
| `1024x1536` | 2:3 | `1024x1536` | **是** |
| `1024x1024` | 1:1 | `1254x1254` | 否 |
| `1024x1280` | 4:5 | `1122x1402` | 否 |
| `1280x1024` | 5:4 | `1402x1122` | 否 |
| `960x1280` | 3:4 | `1086x1448` | 否 |
| `1280x960` | 4:3 | `1448x1086` | 否 |
| `720x1280` | 9:16 | `940x1672` | 否 |
| `1280x720` | 16:9 | `1672x941` | 否 |
| `768x1536` | 1:2 | `887x1774` | 否 |
| `1536x768` | 2:1 | `1774x887` | 否 |
| `720x1680` | 9:21 | `821x1916` | 否 |
| `1680x720` | 21:9 | `1916x821` | 否 |
| `2048x2048` | 1:1 | `1254x1254` | 否（与 `1024x1024` 结果相同） |
| `3840x2160` | 16:9 | `1672x941` | 否（与 `1280x720` 结果相同） |

交付尺寸的宽高比与请求值一致（实测偏差小于 `0.1%`）。可按下面的公式预估：

```text
scale = sqrt(1572864 / (请求宽 × 请求高))
交付宽 = round(请求宽 × scale)
交付高 = round(请求高 × scale)
```

需要 9:16 竖图就请求 `720x1280`（得到 `940x1672`），需要 21:9 宽幅就请求 `1680x720`（得到 `1916x821`），依此类推。

### 图生图（参考图）

使用图片编辑接口，支持 `multipart/form-data`（本地文件）或 JSON（图片 URL）两种入参。

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `image` | file | multipart 必填 | 源图 |
| `mask` | file | 否 | 遮罩图，指定重绘区域 |
| `images[].image_url` | string | JSON 模式必填 | 源图 URL |
| `mask.image_url` | string | 否 | JSON 模式的遮罩图 URL |
| `model` / `prompt` / `size` / `quality` / `n` / `response_format` | — | 否 | 含义与生图接口相同 |

异步任务不支持 `stream=true`，也不支持 `file_id`，请改用 `image_url` 或直接上传 `image` 文件。

```bash
curl -X POST "$BASE_URL/v1/images/edits/async" \
  -H "Authorization: Bearer $API_KEY" \
  -F "model=gpt-image-2" \
  -F "prompt=保留主体结构，把画面改成赛博朋克风格，增强光影和细节" \
  -F "image=@source.png" \
  -F "size=1536x1024" \
  -F "quality=low" \
  -F "n=1" \
  -F "response_format=url"
```

## Response

成功提交返回 HTTP `202 Accepted`。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | string | 任务唯一标识，与 `task_id` 相同，用于查询任务状态 |
| `task_id` | string | 任务唯一标识 |
| `object` | string | 对象类型，固定为 `image.generation.task` |
| `status` | string | 任务状态：`processing`（处理中）、`completed`（已完成）、`failed`（失败） |
| `created_at` | integer | 任务创建时间（Unix 时间戳，秒） |
| `expires_at` | integer | 任务过期时间（Unix 时间戳，秒），过期后无法再查询 |
| `poll_url` | string | 任务查询地址 |

### 请求示例

```bash
curl --request POST \
  --url https://api.chenshuapi.top/v1/images/generations/async \
  --header 'Authorization: Bearer <API_KEY>' \
  --header 'Content-Type: application/json' \
  --data '{
    "model": "gpt-image-2",
    "prompt": "生成一张未来城市夜景海报，霓虹灯，电影感构图",
    "size": "1536x1024",
    "quality": "low",
    "n": 1,
    "response_format": "url"
  }'
```

```javascript
const response = await fetch('https://api.chenshuapi.top/v1/images/generations/async', {
  method: 'POST',
  headers: {
    'Authorization': 'Bearer YOUR_API_KEY',
    'Content-Type': 'application/json'
  },
  body: JSON.stringify({
    model: 'gpt-image-2',
    prompt: '生成一张未来城市夜景海报，霓虹灯，电影感构图',
    size: '1536x1024',
    quality: 'low',
    n: 1,
    response_format: 'url'
  })
});

const task = await response.json();
console.log(task.id, task.status);
```

### 响应示例

```json
{
  "id": "imgtask_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
  "task_id": "imgtask_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
  "object": "image.generation.task",
  "status": "processing",
  "created_at": 1790676904,
  "expires_at": 1790763378,
  "poll_url": "/v1/images/tasks/imgtask_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
}
```

## 查询任务

```http
GET /v1/images/tasks/{task_id}
```

建议每 3 至 5 秒查询一次，客户端总超时留 20 至 30 分钟。**必须以 `status` 为准**：HTTP `202` 与 `processing` 只代表任务已受理，不代表图片已经生成成功。

处理中：

```json
{
  "id": "imgtask_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
  "task_id": "imgtask_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
  "object": "image.generation.task",
  "status": "processing",
  "http_status": 202,
  "created_at": 1790676904,
  "expires_at": 1790763378
}
```

已完成：

```json
{
  "id": "imgtask_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
  "task_id": "imgtask_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
  "object": "image.generation.task",
  "status": "completed",
  "http_status": 200,
  "image_url": "https://img.chenshuapi.com/images/<file>.png",
  "created_at": 1790676904,
  "completed_at": 1790676978,
  "expires_at": 1790763378,
  "result": {
    "created": 1790676917,
    "data": [
      {
        "revised_prompt": "生成一张未来城市夜景海报，霓虹灯，电影感构图",
        "url": "https://img.chenshuapi.com/images/<file>.png"
      }
    ],
    "usage": {
      "input_tokens": 27,
      "output_tokens": 340,
      "total_tokens": 367,
      "input_tokens_details": { "text_tokens": 27, "image_tokens": 0, "cached_tokens": 0 },
      "output_tokens_details": { "text_tokens": 0, "image_tokens": 340, "reasoning_tokens": 0 }
    }
  }
}
```

| 字段 | 说明 |
| --- | --- |
| `result.data[].url` | 图片地址，与顶层 `image_url` 相同。图片固定为 PNG |
| `result.data[].revised_prompt` | 上游改写后的提示词，可用于排查「出的图和描述不一致」 |
| `result.usage.output_tokens_details.image_tokens` | 本次生成的图片 token 数，由**请求尺寸与 `quality`** 共同决定（与最终交付像素无关） |

失败：

```json
{
  "id": "imgtask_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
  "task_id": "imgtask_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
  "object": "image.generation.task",
  "status": "failed",
  "http_status": 500,
  "error": {
    "type": "server_error",
    "code": "server_error",
    "message": "..."
  }
}
```

## 错误响应

| HTTP | `type` | 场景 |
| --- | --- | --- |
| `400` | `invalid_request_error` | 请求体为空、JSON 非法、`n` 小于等于 0、提交异步任务时传了 `stream=true` |
| `401` | `authentication_error` | API Key 缺失或无效 |
| `403` | `permission_error` | 当前 API Key 所属分组未开发生图能力 |
| `404` | `not_found_error` | 路径或任务 ID 不存在、任务已过期 |
| `413` | `invalid_request_error` | 请求体超过大小限制 |

## 计费

* 按张计费：每次成功调用生成一张图，扣一张的费用；同一分组内不同尺寸、不同质量档位**价格相同**
* 具体单价取决于 API Key 所属分组，以控制台显示为准
* 可用任务查询接口返回的 `usage.output_tokens_details.image_tokens` 核对本次实际生成档位

## 常见问题

**为什么 `n` 只能填 1？**
异步任务每次调用只生成一张图片，需要多张时请并发提交多个任务。

**为什么我请求 `2048x2048`，拿到的图不是 2048？**
`size` 只决定宽高比。服务端统一把总像素拟合到约 `1.57 Mpx`，因此 `2048x2048` 与 `1024x1024` 得到的是同一尺寸的图。需要精确尺寸时请只用 `1536x1024` 或 `1024x1536`。

**图片体积和 `quality`、尺寸档位有关吗？**
无关。`quality` 影响的是生成过程（体现在 `image_tokens`，`1024x1024` 下 low / medium / high 分别为 272 / 1056 / 4160），同一交付尺寸下三个档位的文件大小互相穿插，没有高低之分。由于总像素恒定，请求 1K / 2K / 4K 也不改变文件大小；实际体积主要由**宽高比**与**画面内容复杂度**决定。

**任务一直停在 `processing` 怎么办？**
轮询到 `completed` 或 `failed` 为止，总超时建议 20 至 30 分钟；超时后可重新提交。请保留 `task_id` 便于排查，不要用相同参数反复堆积任务。

**返回的图片是什么格式？**
PNG，地址在 `image_url` 与 `result.data[0].url`，两者相同。
