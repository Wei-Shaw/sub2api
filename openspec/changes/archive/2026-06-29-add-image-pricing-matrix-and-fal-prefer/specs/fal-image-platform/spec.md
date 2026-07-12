## ADDED Requirements

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
