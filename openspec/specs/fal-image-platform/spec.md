# fal-image-platform Specification

## Purpose
TBD - created by archiving change add-fal-async-image-platform. Update Purpose after archive.
## Requirements
### Requirement: fal 平台与账号接入

系统 SHALL 支持 `fal` 作为一等平台，并允许以 `apikey` 类型的账号承载 `FAL_KEY` 凭证，纳入与其它平台一致的账号调度。fal 上游请求 SHALL 使用 `Authorization: Key {FAL_KEY}` 进行认证。

#### Scenario: 创建 fal 平台账号
- **WHEN** 管理员创建一个 `platform=fal`、`type=apikey` 的账号并填入 `FAL_KEY`
- **THEN** 系统保存该账号并使其可被 fal 平台请求调度

#### Scenario: fal 上游认证
- **WHEN** 网关向 fal 上游发起请求
- **THEN** 请求头 SHALL 携带 `Authorization: Key {FAL_KEY}`

#### Scenario: 调度 fal 账号
- **WHEN** 一个命中 fal 平台的请求到达且存在可用 fal 账号
- **THEN** 系统 SHALL 按既有调度策略选中一个 fal 账号转发

### Requirement: OpenAI⇄fal 双向协议适配

系统 SHALL 提供 OpenAI Images 协议与 fal 协议之间的双向转换层，使门面协议与上游账号平台可以不同。转换 SHALL 覆盖请求字段（`prompt`、尺寸、质量、数量、图片输入、mask）与响应字段（图片列表）。

#### Scenario: OpenAI 请求转 fal 请求
- **WHEN** OpenAI 形态的图片请求需要由 fal 上游处理
- **THEN** 系统 SHALL 将 `size→image_size`、`quality→quality`、`n→num_images`、edits 的图片输入转换为 fal `image_urls[]`/`mask_url`

#### Scenario: fal 响应转 OpenAI 响应
- **WHEN** fal 返回 `images[]` 结果
- **THEN** 系统 SHALL 转换为 OpenAI Images 响应格式（`data[]` 含 url 或 b64）

#### Scenario: fal 请求转 OpenAI 请求
- **WHEN** fal 形态请求需要由 openai 上游处理
- **THEN** 系统 SHALL 将 fal 字段转换为 OpenAI Images 请求并将其响应转回 fal 格式

### Requirement: 双向 upstream 调度

系统 SHALL 支持四种「门面协议 × 上游账号平台」组合：openai 门面→fal 上游、fal 门面→openai 上游，以及两者各自直通。门面协议由命中的路由决定，上游协议由调度选中账号的 `platform` 决定；两者不一致时 SHALL 启用对应方向的转换。

#### Scenario: fal 账号挂在 openai 平台下做 upstream
- **WHEN** 客户端走 OpenAI 图片门面，且调度选中的上游账号 `platform=fal`
- **THEN** 系统 SHALL 以 OpenAI→fal→OpenAI 方向转换并返回 OpenAI 格式响应

#### Scenario: openai 账号挂在 fal 平台下做 upstream
- **WHEN** 客户端走 fal 原生门面，且调度选中的上游账号 `platform=openai`
- **THEN** 系统 SHALL 以 fal→OpenAI→fal 方向转换并返回 fal 格式响应

#### Scenario: 同平台直通
- **WHEN** 门面协议与上游账号平台一致
- **THEN** 系统 SHALL 直通转发，不做协议转换

### Requirement: 分组级反转优先级开关 image_prefer_fal

`platform=openai` 的分组 SHALL 支持布尔字段 `image_prefer_fal`，用于反转图片调度候选池的平台优先级排序。该开关 SHALL 仅对 `platform=openai` 分组生效；其他平台的分组写入该字段值为 `true` SHALL 被后端拒绝或忽略。

调度行为：

- 当分组 `image_prefer_fal=false`（默认）时，图片请求 SHALL 维持现状语义「openai 优先 + fal 兜底」。
- 当分组 `image_prefer_fal=true` 时，图片请求 SHALL 反转排序为「fal 优先 + openai 兜底」：调度 SHALL 优先从 fal 平台候选账号中选择，仅当 fal 候选池为空或全部不可用时才回退到 openai 候选账号。
- 反转开关 SHALL NOT 影响计费：无论实际承载请求的账号 `platform` 是 openai 还是 fal，金额一律按分组的定价配置计算（参见 `media-prepay-billing` 的「计费表按分组持有」需求）。

#### Scenario: 默认 false 维持现状
- **GIVEN** 分组 `platform=openai` 且 `image_prefer_fal=false`，openai 与 fal 账号均可用
- **WHEN** 图片请求到达
- **THEN** 系统 SHALL 选中一个 openai 账号承载

#### Scenario: 开启后 fal 优先
- **GIVEN** 分组 `platform=openai` 且 `image_prefer_fal=true`，openai 与 fal 账号均可用
- **WHEN** 图片请求到达
- **THEN** 系统 SHALL 选中一个 fal 账号承载

#### Scenario: fal 不可用时自动兜底 openai
- **GIVEN** 分组 `platform=openai` 且 `image_prefer_fal=true`，所有 fal 账号 schedulable=false 或不存在
- **WHEN** 图片请求到达
- **THEN** 系统 SHALL 选中一个 openai 账号承载，不报错

#### Scenario: 非 openai 平台分组写入被拒
- **GIVEN** 分组 `platform=fal`（或 anthropic / gemini / antigravity）
- **WHEN** 管理端尝试写入 `image_prefer_fal=true`
- **THEN** 系统 SHALL 拒绝该写入（返回校验错误）或忽略该字段并保持 false

#### Scenario: 反转不影响计费金额
- **GIVEN** 分组 `image_prefer_fal=true` 且 `image_pricing_matrix["1024x1024"]["high"] = 0.211`
- **WHEN** 调度因反转选中 fal 账号承载 `1024×1024 high` 请求
- **THEN** 系统 SHALL 按 0.211 计费（与 openai 账号承载该请求时金额一致）

### Requirement: 模型映射（内置 + 可配置）

系统 SHALL 内置默认模型映射（OpenAI 模型名 → fal slug，如 `gpt-image-2`/`gpt-image-2/edit`），并 SHALL 允许在账号或渠道上配置覆盖该映射。

#### Scenario: 使用内置映射
- **WHEN** 请求模型未在账号/渠道配置中显式映射
- **THEN** 系统 SHALL 使用内置默认映射解析为 fal slug

#### Scenario: 使用用户配置映射
- **WHEN** 账号或渠道配置了模型映射
- **THEN** 系统 SHALL 优先使用配置的映射并覆盖内置默认值
