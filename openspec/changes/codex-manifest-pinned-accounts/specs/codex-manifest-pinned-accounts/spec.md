## Purpose

让 OpenAI 分组管理员指定一组固定账号来获取普通模型列表与 Codex Model Manifest，使同一分组的 API Key 始终看到确定且合并后的模型列表，而不受调度器选账结果影响。

## ADDED Requirements

### Requirement: OpenAI 分组可配置固定账号获取 Codex Model Manifest
系统 SHALL 为平台为 `openai` 的分组提供 `codex_models_manifest_config` 配置，包含 `enabled`（默认 false）、`account_ids`（账号 ID 列表）和 `fallback_to_scheduler`（默认 false）。`enabled=false` 时系统 MUST 保持现有的本地目录优先、无本地目录时调度器获取 manifest 的行为；普通列表保持本地映射/默认列表行为。

#### Scenario: 默认关闭
- **WHEN** 分组未设置或 `enabled=false`
- **THEN** Codex Model Manifest 请求 MUST 走现有本地生成或调度器选账路径
- **THEN** `account_ids` 与 `fallback_to_scheduler` MUST 不影响任何运行时行为

#### Scenario: 非 OpenAI 平台分组
- **WHEN** 分组平台不是 `openai` 且请求携带 `enabled=true` 的配置
- **THEN** 系统 MUST 将该配置归一化为关闭状态后落库，而不是返回错误

### Requirement: 开启时必须至少选择一个分组内的 OpenAI 账号
当 `enabled=true` 时，管理端更新接口 MUST 校验 `account_ids` 去重后至少包含一个账号，且每个账号 MUST 是当前分组内状态为 active、平台为 `openai` 的账号；账号数量 MUST NOT 超过 10 个。校验失败 MUST 返回 400，不落库。

#### Scenario: 开启但未选择账号
- **WHEN** 管理员保存 `enabled=true` 且 `account_ids` 为空
- **THEN** 系统 MUST 返回 400，错误码为 `INVALID_CODEX_MODELS_MANIFEST_CONFIG`

#### Scenario: 选择了不属于当前分组的账号
- **WHEN** `account_ids` 包含未绑定到该分组、已停用或平台不是 `openai` 的账号
- **THEN** 系统 MUST 返回 400，错误信息指出无效账号 ID

#### Scenario: 创建分组时开启
- **WHEN** 创建分组请求携带 `enabled=true`
- **THEN** 系统 MUST 返回 400，提示创建后再在编辑中配置

#### Scenario: 重复账号 ID
- **WHEN** `account_ids` 含重复 ID
- **THEN** 系统 MUST 去重后保存，保持首次出现的顺序

### Requirement: 固定账号模式只使用选定账号获取 manifest
当 `enabled=true` 时，该分组 API Key 的 Codex Model Manifest 请求 MUST 只向选定账号发起上游请求，MUST NOT 调用调度器。选定账号的可用性判定 MUST 只考虑账号状态为 active、可调度开关打开、未因过期自动暂停；MUST 忽略优先级、负载因子、限流窗口与过载窗口。已从分组移除或已删除的账号 MUST 被跳过。

#### Scenario: 限流中的选定账号仍被使用
- **WHEN** 某个选定账号处于限流或过载窗口内
- **THEN** 系统 MUST 仍然使用该账号请求 manifest

#### Scenario: 停用的选定账号被跳过
- **WHEN** 某个选定账号状态为 inactive 或可调度开关关闭
- **THEN** 系统 MUST 跳过该账号，不向其发起请求

#### Scenario: 选定账号已不在分组内
- **WHEN** 配置中的账号 ID 已从分组解绑或已删除
- **THEN** 系统 MUST 跳过该 ID，并继续处理其余账号

### Requirement: 多个选定账号并发请求并合并模型列表
系统 MUST 对所有可用的选定账号并发发起 manifest 请求，并把各账号响应的 `models` 按 `slug` 取并集：同一 slug 以配置顺序中靠前账号的条目为准；顶层其余字段取配置顺序中第一个成功账号的响应。合并结果 MUST 继续应用分组自定义模型列表过滤；别名 MUST 仅来自成功拉取账号的映射投影，并基于最终响应体计算 ETag。

#### Scenario: 并集合并
- **WHEN** 账号 1 返回模型 A、B，账号 2 返回模型 A、C
- **THEN** 客户端 MUST 收到模型 A、B、C，且 A 的条目来自账号 1

#### Scenario: 部分账号失败
- **WHEN** 一个选定账号上游返回错误而其他账号成功
- **THEN** 系统 MUST 以成功账号的响应合并并返回 200
- **THEN** 系统 MUST 记录包含失败账号 ID 的警告日志

#### Scenario: 分组自定义模型列表仍然生效
- **WHEN** 分组启用了自定义 `/v1/models` 列表
- **THEN** 合并后的 manifest MUST 只保留列表中允许的模型

#### Scenario: ETag 条件请求
- **WHEN** 客户端 `If-None-Match` 与合并后最终响应体的 ETag 匹配
- **THEN** 系统 MUST 返回 304 且响应体为空

### Requirement: 选定账号全部不可用或全部失败时按配置回退
当没有任何可用的选定账号，或所有可用账号的上游请求全部失败时：`fallback_to_scheduler=false` 时系统 MUST 返回上游错误（全部失败时）或 503（无可用账号时）；`fallback_to_scheduler=true` 时系统 MUST 回退到现有调度器选账路径。

#### Scenario: 默认不回退
- **WHEN** `fallback_to_scheduler=false` 且所有选定账号都不可用
- **THEN** 系统 MUST 返回 503，错误类型为 `upstream_error`

#### Scenario: 全部失败且不回退
- **WHEN** `fallback_to_scheduler=false` 且所有可用选定账号的上游请求均失败
- **THEN** 系统 MUST 返回最后一个上游错误对应的状态码与信息

#### Scenario: 开启回退
- **WHEN** `fallback_to_scheduler=true` 且所有选定账号不可用或全部失败
- **THEN** 系统 MUST 使用调度器选择账号并按现有流程返回 manifest

### Requirement: 管理端展示与编辑固定账号配置
分组编辑对话框在平台为 `openai` 时 MUST 展示该配置：一个开关；开关打开后展示带搜索的多选账号下拉与「全部不可用时回退调度器」子开关。账号下拉 MUST 只列出当前分组内平台为 `openai` 的账号，并支持按名称搜索。分组创建对话框 MUST NOT 展示该配置。

#### Scenario: 开启后未选账号即提交
- **WHEN** 管理员打开开关但未选择任何账号并点击保存
- **THEN** 前端 MUST 阻止提交并提示至少选择一个账号

#### Scenario: 回显已保存账号
- **WHEN** 打开一个已配置固定账号的分组编辑对话框
- **THEN** 已选账号 MUST 以名称标签展示；无法解析名称的账号 MUST 以 `#<id>` 展示

#### Scenario: 分组复制
- **WHEN** 管理员复制一个开启了固定账号配置的分组
- **THEN** 新分组的该配置 MUST 为关闭且账号列表为空

### Requirement: 普通模型列表复用固定账号配置
当 OpenAI 分组开启固定账号时，普通 `/v1/models` 与 `/models` 请求 MUST 使用同一组指定账号向上游发现模型，并输出 `{object:"list",data:[...]}`。API Key 请求 MUST 使用标准模型列表端点，不添加 `client_version`；OAuth MUST 使用已有 Codex manifest 链路，将 slug 转为标准 ID。普通列表 MUST 保留上游媒体模型与未知具体模型。

#### Scenario: 普通客户端发现特殊模型
- **WHEN** 不带 `client_version` 的请求使用开启固定账号的分组，所选账号返回非内置的模型
- **THEN** 普通列表 MUST 包含该模型且 MUST NOT 请求未选账号

#### Scenario: API Key 与 OAuth 账号混合
- **WHEN** 所选账号包含 API Key 与 OAuth
- **THEN** 系统 MUST 按各自协议获取目录并按 ID 合并，重复 ID MUST 取配置顺序靠前账号的条目

#### Scenario: 普通请求回退
- **WHEN** 固定账号全不可用或全失败且开启回退
- **THEN** 系统 MUST 经由调度器选账号获取普通模型目录，而不是返回本地默认列表

### Requirement: 固定账号发现先于本地目录生成
固定账号开启时，普通列表与 Codex manifest MUST 先获取指定账号目录，再按照来源账号的模型映射生成公开名称，最后应用分组列表过滤。MUST NOT 因为存在显式模型映射而跳过上游发现。透传账号 MUST 忽略残留模型映射；普通模式的具体别名仅在映射目标存在于来源账号目录时可见。MUST NOT 注入未成功拉取账号的别名或把通配模式作为模型 ID 返回。

#### Scenario: 有映射的固定账号
- **WHEN** 所选账号配置 public-name 到 upstream-name 的映射且上游返回 upstream-name
- **THEN** 系统 MUST 请求该账号上游并以 public-name 返回该模型，Codex 条目 MUST 继承上游能力元数据

#### Scenario: 固定账号开启但列表为空
- **WHEN** 运行时配置开启但账号 ID 列表为空
- **THEN** 系统 MUST 按无可用固定账号的失败/回退规则处理，MUST NOT 静默使用本地目录

#### Scenario: 成功空目录或过滤为空
- **WHEN** 上游返回有效空数组或分组过滤排除了全部模型
- **THEN** 普通模型列表 MUST 返回 200 且 data 为空数组，MUST NOT 返回默认模型或触发失败回退

### Requirement: 普通模型列表的条件请求
普通固定账号列表 MUST 基于映射、合并和过滤后的最终响应体计算 ETag。

#### Scenario: 普通列表 ETag 匹配
- **WHEN** 客户端 If-None-Match 与最终普通列表响应的 ETag 匹配
- **THEN** 系统 MUST 返回 304 且无响应体
