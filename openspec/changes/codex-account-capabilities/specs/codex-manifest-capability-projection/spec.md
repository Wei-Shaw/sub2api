# Spec Delta

## Purpose

让下发给 Codex 客户端的模型 manifest 中的能力字段反映分组内账号的真实能力并集与分组白名单，使客户端在且仅在网关能够履行时发送 Daybreak 与 service tier 字段。

## ADDED Requirements

### Requirement: 能力字段按分组账号并集投影
OpenAI 分组下发 Codex manifest 时，对每个模型，系统 SHALL 逐账号用该账号自身的模型映射把公开别名解析回上游 slug，再查该账号的能力记录；若任一账号的记录含其解析出的 slug，则 `service_tiers` 与 `available_access_programs.cyber` MUST 替换为各记录该 slug 的并集，不保留 manifest 原值；所有记录都不含该 slug 时保留原值。并集 MUST 来源于已持久化的能力记录，不得为此发起额外上游请求。投影 MUST 应用于调度器路径、固定账号路径与本地生成路径三种 manifest 来源。Composite 分组没有直属账号，MUST 跳过并集步骤。`service_tiers` 条目的名称与描述沿用 manifest 原有条目或内置描述。

#### Scenario: 混合账号分组
- **WHEN** 分组内账号 A 的记录中 `gpt-6-sol` 带 `daybreak_blue`，账号 B 的记录中同一 slug 只带 `standard`，manifest 由账号 B 拉取
- **THEN** 下发的 `gpt-6-sol` 的 cyber 列表为 `["standard","daybreak_blue"]`

#### Scenario: 单一 slug 的 Ultrafast
- **WHEN** 只有账号 A 的记录在 `gpt-6.1-sol` 上带 `ultrafast`
- **THEN** 下发的 `gpt-6.1-sol` 的 `service_tiers` 包含 `priority` 与 `ultrafast`，其他 slug 不因此获得 `ultrafast`

#### Scenario: 分组内没有任何能力记录
- **WHEN** 分组内所有账号都没有能力记录
- **THEN** 能力字段保持上游或内置描述原值，不做并集，但白名单裁剪仍然执行

#### Scenario: 缓存中的旧能力不复活
- **WHEN** 账号 A 的 Ultrafast 已被上游撤销且记录已更新，但分组 manifest 缓存体中 `gpt-6.1-sol` 仍带 `ultrafast`
- **THEN** 下发的 `gpt-6.1-sol` 不含 `ultrafast`

#### Scenario: 本地生成路径
- **WHEN** 分组配置了模型映射并走本地生成 manifest，内置模板为 `gpt-5.6-sol` 带 `ultrafast`，而分组内所有记录的该 slug 都不含 `ultrafast`
- **THEN** 下发的 `gpt-5.6-sol` 不含 `ultrafast`

#### Scenario: 公开别名
- **WHEN** 账号 A 把公开别名 `my-codex` 映射到 `gpt-6-sol` 且记录在 `gpt-6-sol` 上带 `daybreak_blue`，账号 B 把同一别名映射到 `gpt-6-luna` 且记录在 `gpt-6-luna` 上不带 Daybreak
- **THEN** 下发的 `my-codex` 的 cyber 列表包含 `daybreak_blue`，且不会因 A 在 `gpt-6-luna` 上的记录或 B 在 `gpt-6-sol` 上的记录而改变

### Requirement: 投影受分组白名单约束
分组白名单开启时，系统 SHALL 从 manifest 中移除被禁用的 Daybreak 等级与 service tier；该裁剪 MUST 与能力记录是否存在无关，且对 OpenAI 与 Composite 分组都生效；`standard` MUST 保留。

#### Scenario: 白名单禁用 Ultrafast
- **WHEN** 白名单开启且 tier 集合不含 `ultrafast`
- **THEN** 下发 manifest 中任何模型的 `service_tiers` 都不含 `ultrafast`

#### Scenario: 白名单禁用全部 Daybreak
- **WHEN** 白名单开启且 Daybreak 集合为空
- **THEN** 下发 manifest 中每个模型的 cyber 列表只保留 `standard`

### Requirement: 固定账号合并后再投影
固定账号模式按 slug 合并多个账号 manifest 后，系统 SHALL 对合并结果执行同样的投影，能力字段不再取首个账号的原值。

#### Scenario: 首账号无 Daybreak
- **WHEN** 固定账号列表中第一个账号无 Daybreak，第二个账号支持 Blue
- **THEN** 合并后下发的 cyber 列表包含 `daybreak_blue`

### Requirement: ETag 基于投影后的 body
系统 SHALL 以投影后的 manifest body 计算 ETag，并据此判断 304。

#### Scenario: 能力变化导致 ETag 变化
- **WHEN** 分组内某账号新获得 Ultrafast 且记录已更新
- **THEN** 下一次 manifest 的 ETag 与此前不同，携带旧 ETag 的请求不会得到 304
