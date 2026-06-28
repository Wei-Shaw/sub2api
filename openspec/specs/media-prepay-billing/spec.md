# media-prepay-billing Specification

## Purpose
TBD - created by archiving change add-fal-async-image-platform. Update Purpose after archive.
## Requirements
### Requirement: 先扣费后退款

系统 SHALL 在任务提交时按预估金额预扣费（先扣费模式），并在 `async_media_tasks.held_cost` 记录预扣金额。任务成功 SHALL 结清费用；任务失败或真超时 SHALL 退还已扣金额。

#### Scenario: 提交时预扣费
- **WHEN** 任务提交成功并落库
- **THEN** 系统 SHALL 预扣对应金额并记录 `held_cost`

#### Scenario: 成功结算
- **WHEN** 任务成功完成
- **THEN** 系统 SHALL 确认扣费并记录 `final_cost`

#### Scenario: 失败退费
- **WHEN** 任务被判定失败或真超时
- **THEN** 系统 SHALL 退还已扣金额，退费与终态更新 SHALL 在同一事务内完成

### Requirement: usage_logs 记录扣费/退费状态

`usage_logs` SHALL 新增字段 `task_id`、`image_urls`、`cos_url`、`billing_status`(charged|refunded)，并在任务终态时追加一条记录以保持只追加语义。

#### Scenario: 成功写 usage_log
- **WHEN** 任务成功
- **THEN** 系统 SHALL 追加一条 `billing_status=charged` 的 usage_log，含 `task_id`、成功图片 url 与 `cos_url`

#### Scenario: 退费写 usage_log
- **WHEN** 任务退费
- **THEN** 系统 SHALL 追加一条 `billing_status=refunded` 的 usage_log，含 `task_id`

#### Scenario: usage_log 关联任务
- **WHEN** 查询某条 usage_log
- **THEN** 该记录 SHALL 通过 `task_id` 关联到对应 `async_media_tasks`

### Requirement: 按分辨率×质量二维计费

系统 SHALL 在 image 计费模式下按 (image_size 档位 × quality) 二维定价。定价表 SHALL 由分组（Group）持有：分组的 `image_pricing_matrix` 字段 SHALL 以 JSON 形式承载 6 档 size × 3 档 quality 共 18 个价格格，键为「`{width}x{height}` 字符串」，值为 `{ "low": <price>, "medium": <price>, "high": <price> }`。

系统 SHALL 在计费查找时按以下顺序回退：

1. 命中分组 `image_pricing_matrix[size_tier][quality]` → 使用该价格。
2. 未命中时按 `size_tier` 归并到分组旧字段 `image_price_1k`/`image_price_2k`/`image_price_4k`，命中且非空 → 使用该价格（与 quality 无关）。
3. 仍未命中 → 回退到 LiteLLM 默认价。

#### Scenario: 二维定价命中
- **WHEN** 请求归一后命中分组 `image_pricing_matrix["1024x1024"]["high"] = 0.211`
- **THEN** 系统 SHALL 按 0.211 计费

#### Scenario: 矩阵未配置某格回退到旧字段
- **WHEN** 请求归一为 `("1024x1024", "high")`，但 `image_pricing_matrix` 中 `1024x1024.high` 缺失，分组 `image_price_1k = 0.04` 已配置
- **THEN** 系统 SHALL 按 0.04 计费（不论 quality）

#### Scenario: 矩阵与旧字段都未配置回退到 LiteLLM
- **WHEN** 分组未配置 `image_pricing_matrix`，且对应档位的旧字段也为空
- **THEN** 系统 SHALL 使用 LiteLLM 默认价计费

#### Scenario: 兼容存量单维定价
- **WHEN** 分组从未配置过 `image_pricing_matrix`，仅有旧 `image_price_1k/2k/4k`
- **THEN** 系统 SHALL 按 `size_tier` 归并到对应旧字段计费，行为与本次变更前完全一致

### Requirement: image_size 6 档归一与向上取档

系统 SHALL 在 image 计费时将请求的实际像素总数 (width × height) 归一到 6 个固定档位之一，按像素总数升序「向上取档」，超过最大档位时封顶到 4K 档。档位 key 即代表分辨率字符串，与 `image_pricing_matrix` 顶层键一一对应：

| 档位 key       | 代表分辨率   | 像素总数上限（含） |
| -------------- | ------------ | ------------------- |
| `1024x768`     | 1024×768     | 786,432             |
| `1024x1024`    | 1024×1024    | 1,048,576           |
| `1024x1536`    | 1024×1536    | 1,572,864           |
| `1920x1080`    | 1920×1080    | 2,073,600           |
| `2560x1440`    | 2560×1440    | 3,686,400           |
| `3840x2160`    | 3840×2160    | 8,294,400（封顶）   |

#### Scenario: 边界精确命中
- **WHEN** 请求 `1024×1024` (像素总数 1,048,576)
- **THEN** 系统 SHALL 归档为 `1024x1024`

#### Scenario: 介于两档之间向上取档
- **WHEN** 请求 `1280×720` (像素总数 921,600，介于 786,432 与 1,048,576 之间)
- **THEN** 系统 SHALL 归档为 `1024x1024`

#### Scenario: 超出最大档位封顶
- **WHEN** 请求 `5120×2880` (像素总数 14,745,600，超过 8,294,400)
- **THEN** 系统 SHALL 归档为 `3840x2160`

#### Scenario: 横竖图同像素同档
- **WHEN** 请求 `1536×1024` (像素总数 1,572,864)
- **THEN** 系统 SHALL 归档为 `1024x1536`，与 `1024×1536` 同档

### Requirement: quality 归一规则

系统 SHALL 在 image 计费时将请求的 `quality` 字段归一到 `low`/`medium`/`high` 三档之一：`auto`、空字符串、缺失值 SHALL 归一为 `high`；`low`/`medium`/`high` SHALL 透传。OpenAI 协议入口与 fal 协议入口的 `quality` 字段 SHALL 视为同义。

#### Scenario: auto 归一为 high
- **WHEN** 请求携带 `quality=auto`
- **THEN** 系统 SHALL 按 `high` 档计费

#### Scenario: 缺失字段归一为 high
- **WHEN** 请求未携带 `quality` 字段
- **THEN** 系统 SHALL 按 `high` 档计费

#### Scenario: 显式 low/medium/high 透传
- **WHEN** 请求携带 `quality=low`
- **THEN** 系统 SHALL 按 `low` 档计费

#### Scenario: OpenAI 与 fal quality 同义
- **WHEN** OpenAI 形态请求 `quality=medium` 与 fal 形态请求 `quality=medium` 命中同分组同 size_tier
- **THEN** 系统 SHALL 按相同价格计费

### Requirement: 计费表按分组持有，与承载账号平台无关

系统 SHALL 保证同一分组内同一 (size_tier, quality) 的请求金额完全由该分组的定价配置决定，与调度最终选中的上游账号 `platform` 无关。即：分组路由到 openai 账号或 fal 账号承载请求时 SHALL 使用同一张 `image_pricing_matrix` 计费。

#### Scenario: openai 账号承载按分组矩阵计费
- **GIVEN** `platform=openai` 分组配置 `image_pricing_matrix["1024x1024"]["high"] = 0.211`
- **WHEN** 调度选中 openai 账号承载该请求
- **THEN** 系统 SHALL 按 0.211 计费

#### Scenario: fal 账号承载按相同分组矩阵计费
- **GIVEN** 同上分组与矩阵
- **WHEN** 调度选中 fal 账号承载该请求（如 `image_prefer_fal=true` 或 openai 账号不可用兜底到 fal）
- **THEN** 系统 SHALL 按 0.211 计费（与 openai 账号承载结果完全一致）

### Requirement: 回包图片分辨率自检（base64）

系统 SHALL 为 `platform=openai` 的分组提供 `image_decode_size_on_rsp` 布尔配置，默认 `false`。当配置为 `true` 且 `images/generations` 上游回包某张图的 `size` 字段缺失或等于 `auto` 时，系统 SHALL 在异步记账阶段对该张图的 `b64_json` 内容做最小代价的头部解码（仅解析尺寸元数据，不解码像素），并以解码出的 `{width}x{height}` 作为该张图的 size 进入 size 6 档归档计费。配置为 `false` 或 `platform != openai` 的分组 SHALL NOT 触发解码，保持现状默认档兜底行为。

解码 SHALL 仅作用于 base64 模式（`b64_json` 字段非空）；URL 模式的回包不在本需求范围内（此场景下视同解码失败）。解码失败（不可识别的图片格式、解码异常、payload 缺失等）时系统 SHALL NOT 中断或报错，应保持现网默认档兜底语义并按 `warn` 级别记录可观测性日志。已携带有效 size（非空且非 `auto`）的图片 SHALL NOT 被解码覆盖。

#### Scenario: 开关关闭时不解码
- **GIVEN** 分组 `image_decode_size_on_rsp = false`
- **WHEN** 上游回包 `size` 缺失，仅返回 `b64_json` 内容
- **THEN** 系统 SHALL 沿用默认 2K 档计费，不对 b64 内容做解码

#### Scenario: 开关开启且 size 缺失时解码命中
- **GIVEN** `platform=openai` 分组 `image_decode_size_on_rsp = true`，回包某张图 `size` 字段缺失但 `b64_json` 为合法 PNG（实际 1920×1080）
- **WHEN** 异步记账阶段处理该回包
- **THEN** 系统 SHALL 解码并以 `1920x1080` 进入 6 档归档（命中 `1920x1080` 档），按矩阵 `1920x1080` 行计费

#### Scenario: 开关开启且 size=auto 时解码覆盖
- **GIVEN** 同上分组开关，回包 `size = "auto"` 且 `b64_json` 为合法 JPEG（实际 1024×1024）
- **WHEN** 异步记账阶段处理该回包
- **THEN** 系统 SHALL 把该张图视为 `1024x1024` 进入归档计费

#### Scenario: 上游已返回有效 size 时不覆盖
- **GIVEN** 分组开关 `image_decode_size_on_rsp = true`，回包同时携带 `size = "1024x1024"` 与合法 `b64_json`
- **WHEN** 异步记账阶段处理该回包
- **THEN** 系统 SHALL 直接使用上游 `1024x1024`，不对 b64 内容做解码

#### Scenario: 解码失败回退默认档
- **GIVEN** 分组开关开启，回包 `size` 缺失且 `b64_json` 为不可识别的格式或损坏内容
- **WHEN** 异步记账阶段尝试解码
- **THEN** 系统 SHALL 不报错、不阻塞计费，沿用现状默认档兜底（默认 2K），并记录 warn 级别 `openai.images.size_decode_failed` 日志

#### Scenario: URL 模式不触发解码
- **GIVEN** 分组开关开启，回包仅返回 `url` 字段，不返回 `b64_json`
- **WHEN** 异步记账阶段处理该回包
- **THEN** 系统 SHALL NOT 拉取远程图片，按 size 缺失的现状语义走默认档兜底

#### Scenario: 平台不匹配时开关无效
- **GIVEN** `platform=fal` 分组写入 `image_decode_size_on_rsp = true`（如配置面误写）
- **WHEN** 该分组承载 fal 协议请求
- **THEN** 系统 SHALL NOT 触发解码逻辑，保持 fal 既有计费路径不变
