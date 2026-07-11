# Video Gateway Contract

> 状态：内部 mock 可演示 / 待复核 / 非生产 READY
> 适用方：QCanvas（TapCanvas）与其他内部调用方
> 最后更新：2026-07-11

本文档是 Sub2API 视频网关的产品契约。实现或 reason code 变化必须同步更新契约测试与本文档，禁止 silent drift。

## 1. 响应包络

成功响应：

```json
{
  "code": 0,
  "message": "success",
  "data": {}
}
```

错误响应：

```json
{
  "code": 400,
  "message": "human-readable diagnostic",
  "reason": "VIDEO_INVALID_CONTENT",
  "metadata": {}
}
```

调用方以 HTTP 状态与 `reason` 做机器判断，以 `message` 做诊断展示；不要解析 message 文案。

## 2. 端点

### 2.1 Admin JWT 视频端点

| 场景 | Method | Path | Auth |
|---|---|---|---|
| 创建任务 | POST | `/api/v1/video/tasks` | 管理台 JWT |
| 查询任务 | GET | `/api/v1/video/tasks/{id}` | 管理台 JWT |
| 取消任务 | POST | `/api/v1/video/tasks/{id}/cancel` | 管理台 JWT |
| 列表/仪表盘/Provider 管理 | GET/POST | `/api/v1/video/*` | 管理台 JWT |

Admin 路径用于控制台操作与运维诊断，不替代 QCanvas API Key 契约。

### 2.2 QCanvas API-Key 端点

| 场景 | Method | Path | Auth |
|---|---|---|---|
| 创建任务 | POST | `/v1/video/tasks` | `Authorization: Bearer <api-key>` |
| 查询任务 | GET | `/v1/video/tasks/{id}` | 同一 API Key 身份 |
| 取消任务 | POST | `/v1/video/tasks/{id}/cancel` | 同一 API Key 身份 |
| 查询可用视频通道 | GET | `/v1/video/providers` | API Key |

当前 QCanvas 采用 create → poll，不使用 webhook。API Key 必须绑定允许的视频分组；密钥只出现在调用方运行环境与 Authorization header，不进入前端源码、日志或审查包。

## 3. 创建请求

`POST /v1/video/tasks`（QCanvas）或 `POST /api/v1/video/tasks`（Admin）

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `provider` | string | 否 | 当前现场联调只使用 `mock`。代码中存在受 gate 保护的 `seedance` 路径，但不代表已授权或已验证。 |
| `task_type` | string | 是 | `text_to_video` / `image_to_video` / `reference_to_video` |
| `model` | string | 否 | mock 可用 `mock-video-v1` |
| `prompt` | string | 是 | 最长 8000 字符 |
| `negative_prompt` | string | 否 | 最长 4000 字符 |
| `reference_image_url` | string | 否 | 兼容字段；服务端归一化到 `content[]` |
| `reference_video_url` | string | 否 | 兼容字段；服务端归一化到 `content[]` |
| `content` | array | 否 | 多模态输入；见下表 |
| `aspect_ratio` | string | 否 | 例如 `16:9` / `9:16` / `1:1` |
| `duration` | int | 否 | mock 与 provider 校验规则由后端执行 |
| `resolution` | string | 否 | `480p` / `720p` / `1080p`，以 provider 能力为准 |
| `generate_audio` | bool | 否 | 可选回显 |
| `watermark` | bool | 否 | 可选回显 |
| `camera_fixed` | bool | 否 | 可选回显 |
| `return_last_frame` | bool | 否 | 成功后请求 `last_frame_url` |
| `trial_mode` | string | 否 | 仅受控 Seedance 最小试跑使用 `tiny_real`；本轮不执行 |

### 3.1 content[] 数量与组合限制

| `type` | 允许字段/role | 约束 |
|---|---|---|
| `text` | `text` | 最多一条 |
| `image_url` | `url` + `first_frame` / `last_frame` / `reference_image` | 必须通过 SSRF/allowlist 校验 |
| `video_url` | `url` + `reference_video` | 必须通过 SSRF/allowlist 校验 |
| `audio_url` | `url` + `reference_audio` | 不能单独使用 |

组合规则：

- 首尾帧模式不能与 reference 模式混用。
- `image_to_video` 下旧 `reference_image_url` 归一化为 `role=first_frame`。
- `reference_to_video` 下旧 `reference_image_url` 归一化为 `role=reference_image`。
- `reference_video_url` 转为 `role=reference_video`，并设置 `has_video_input=true`。
- 旧字段仍会在响应中原样回显，避免破坏既有 QCanvas 请求。

### 3.2 Mock 请求示例

```json
{
  "provider": "mock",
  "task_type": "reference_to_video",
  "model": "mock-video-v1",
  "prompt": "QCanvas contract prompt",
  "content": [
    {"type": "text", "text": "QCanvas contract prompt"},
    {"type": "image_url", "role": "reference_image", "url": "https://example.invalid/ref.png"}
  ],
  "aspect_ratio": "9:16",
  "duration": 5,
  "resolution": "720p",
  "return_last_frame": true
}
```

建议调用方发送稳定的 `Idempotency-Key`。同 key + 同 payload 返回同一任务；同 key + 不同 payload 返回 `IDEMPOTENCY_KEY_CONFLICT`。未发送时服务端生成 key，并通过响应 header 与创建响应的可选 `idempotency_key` 返回。

## 4. 模式矩阵

| 路径 | `provider` | `trial_mode` | `provider_boundary` | `real_provider_dispatch_count` | 当前产品状态 |
|---|---|---|---|---|---|
| API Key mock | `mock` | 无 | `api-key-video-mock-only` | 必须为 `0` | 本轮已彩排 |
| Seedance production | `seedance` | 无 | `api-key-video-seedance-production` | 真实 dispatch 后递增 | 需 `production_authorized=true` 与全部真实调用 gate；本轮未授权、未验证 |
| Seedance tiny trial | `seedance` | `tiny_real` | `api-key-video-seedance-tiny-trial` | 真实 dispatch 后递增 | 需独立试跑授权、全局 gate、脱敏日志、媒体 allowlist 与每日上限；本轮不执行 |

不带 `trial_mode` 的 Seedance 请求不是 tiny trial。任何真实路径 gate 失败都必须返回可诊断 reason，且不能静默降级成 mock。取消响应必须保留创建时的 provenance events；lookup 失败不得默认输出 production boundary。

## 5. 任务响应 Schema

QCanvas 稳定依赖以下字段，保持 snake_case：

| 字段 | 类型 | 语义 |
|---|---|---|
| `id` | number | Sub2API 本地任务 ID；作为 QCanvas taskId |
| `provider` | string | 当前现场应为 `mock` |
| `model` | string | 请求/路由后的模型 |
| `task_type` | string | 任务类型 |
| `status` | string | `queued` / `submitted` / `running` / `succeeded` / `failed` / `cancelled` |
| `result_url` | string | 成功结果 URL；mock 为同源相对路径 |
| `error_message` | string | 失败原因；成功时为空 |
| `usage.total_tokens` | number | provider 成功响应中的真实 token；用于计费。旧任务无值时为 `0` |
| `actual_resolution` | string | provider 回报的实际分辨率 |
| `actual_duration` | number/null | provider 回报的实际时长 |
| `last_frame_url` | string | 请求 `return_last_frame=true` 且成功后可读 |
| `cost_estimate` | number | 任务成本。Seedance 成功任务按真实 usage 计费 |
| `mock_only` | bool | mock 任务为 `true` |
| `provider_boundary` | string | 见模式矩阵 |
| `real_provider_dispatch_count` | number | mock 路径必须为 `0` |
| `idempotency_key` | string | 可选；创建时回显 |

可选可靠性字段：`dispatch_state`、`settlement_status`、`archive_status`、`capture_status`、`delivery_status`、`next_action`、`local_asset_available`、`local_asset_path`。旧客户端忽略这些字段不受影响。

### 5.1 证据层分离

以下不是同一回事，禁止互相等同：

| 证据 | 含义 |
|---|---|
| `status=succeeded` | 生成状态机到达成功终态 |
| `result_url` 存在 | 当前可引用的结果地址（可能是临时远程 URL 或 mock 相对路径） |
| 可预览/下载 | 调用方或管理台此刻能打开/下载该资产 |
| 持久交付 | `delivery_status=deliverable` 且本地资产可用，或未过期远程结果仍可交付 |

补充：

- `delivery_status=archiving`：生成完成，归档中。
- `delivery_status=deliverable`：本地资产或未过期远程结果可用。
- `delivery_status=delivery_failed`：生成终态保留，但当前无可交付资产。
- `settlement_status=error`：账务待处理，不得把 succeeded 改写为 failed。

### 5.2 Mock 成功示例

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": 12345,
    "provider": "mock",
    "model": "mock-video-v1",
    "task_type": "reference_to_video",
    "status": "succeeded",
    "result_url": "/api/v1/video/mock-assets/12345.svg",
    "last_frame_url": "/api/v1/video/mock-assets/12345-last-frame.png",
    "error_message": "",
    "usage": {"total_tokens": 0},
    "actual_resolution": "720p",
    "actual_duration": 5,
    "mock_only": true,
    "provider_boundary": "api-key-video-mock-only",
    "real_provider_dispatch_count": 0
  }
}
```

QCanvas 应把相对 `result_url` 按 Sub2API Base URL 解析，不直接读取交付包或本机文件路径。

## 6. 轮询与取消

1. 创建成功后保存 `data.id`。
2. 轮询 `GET /v1/video/tasks/{id}`。
3. `queued/submitted/running` 继续轮询；`succeeded/failed/cancelled` 是生成终态。
4. `succeeded + delivery_status=archiving` 可低频继续轮询交付状态。
5. `dispatch_state=unknown` 时不得重复创建；按 `next_action=reconcile_dispatch` 交给运维确认。
6. 取消只允许非终态任务；终态取消返回 `VIDEO_TASK_NOT_CANCELABLE`。
7. 取消响应必须带回 provenance events，以便正确区分 production 与 tiny trial boundary。

## 7. 计费、reservation、settlement 与 idempotency

- 创建成功后可产生 billing reservation；任务失败或取消应按策略 release，不得把 active reservation 静默吞掉。
- Seedance 成功任务按 provider poll 响应 `usage.total_tokens` 计费：`tokens / 1,000,000 * 每 M tokens 单价`。
- `has_video_input=true` 时，Seedance 2.0 使用含视频输入价；`false` 时使用不含视频输入价。
- 任务失败不计费，费用为 0。
- Seedance 价格币种为 CNY，归总展示时按系统设置 `usd_cny_rate` 折算为 USD。
- 当前任务书核价：Seedance 2.0 不含视频输入 46 / M tokens，含视频输入 28 / M tokens；Seedance 2.0 fast 不含视频输入 37 / M tokens，含视频输入 22 / M tokens。
- `settlement_status` 描述账务侧状态，与生成 `status` 解耦；settlement error 不得改写 generation status。
- Idempotency：同 `Idempotency-Key` + 同 payload 返回同一任务；同 key + 不同 payload 返回 `IDEMPOTENCY_KEY_CONFLICT`。

## 8. Error reasons

| HTTP | reason | 触发条件 |
|---|---|---|
| 400 | `VALIDATION_ERROR` | JSON 或字段类型无效 |
| 400 | `VIDEO_MISSING_PROMPT` | prompt 为空 |
| 400 | `VIDEO_INVALID_PROVIDER` | provider 枚举无效 |
| 400 | `VIDEO_INVALID_TASK_TYPE` | task_type 无效 |
| 400 | `VIDEO_INVALID_CONTENT` | content/mode/数量/分辨率等校验失败 |
| 400 | `VIDEO_UNSAFE_REFERENCE_URL` | 媒体 URL 未通过 SSRF/allowlist |
| 400/403 | `VIDEO_PROVIDER_DISABLED` | provider 未配置、停用或不允许该 API-key 路径 |
| 403 | `VIDEO_PRODUCTION_NOT_AUTHORIZED` | Seedance production account 未授权 |
| 403 | `VIDEO_TRIAL_BLOCKED` | tiny trial 的环境、日志、allowlist、账号或时长 gate 未满足 |
| 403 | `VIDEO_TRIAL_LIMIT_EXCEEDED` | tiny trial 超出每日限制 |
| 404 | `VIDEO_TASK_NOT_FOUND` | 任务不存在或调用方无权访问 |
| 409 | `IDEMPOTENCY_KEY_CONFLICT` | 同 key 使用不同 payload |
| 409 | `VIDEO_TASK_NOT_CANCELABLE` | 任务已经进入终态 |
| 503 | `VIDEO_MOCK_PROVIDER_UNAVAILABLE` | mock provider 不可用 |

## 9. 非声称与边界

本契约的 mock 测试、Docker 本地试跑与 QCanvas create/poll 通过，不证明真实 Seedance、真实计费、生产数据、生产部署、公网访问或跨机器 QCanvas 已可用。

硬边界：

- 当前产品口径最多是：内部 mock 可演示 / 非生产 READY。
- 禁止在未授权情况下调用真实/付费 Provider。
- 不要再读取 `ResultURL`；API-key 响应已移除该 PascalCase 重复字段。
- 需要续拍时，创建请求带 `return_last_frame=true`，轮询成功后读取 `last_frame_url`。
- 构造 content 编辑器时，先按模式约束 UI，避免把首尾帧模式与 reference 模式混用。

真实 Provider 验证必须另行授权预算、账号、停止条件与审查证据。
