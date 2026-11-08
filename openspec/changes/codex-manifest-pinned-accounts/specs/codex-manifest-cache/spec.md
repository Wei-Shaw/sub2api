## Purpose

规定 Codex Model Manifest 按账号缓存的时效策略，使所有账号类型的 manifest 拉取在新鲜期内不打上游、乐观期内先返回缓存再后台刷新、超期后同步刷新。

## ADDED Requirements

### Requirement: 所有账号类型的 manifest 拉取都经过缓存
系统 MUST 对 OAuth 账号与 API Key 账号的 Codex Model Manifest 上游拉取统一使用同一套按账号缓存。缓存键 MUST 区分账号、凭据账号、代理与请求头（含授权头与客户端版本），凭据变化后 MUST 视为新的缓存项。缓存键 MUST NOT 包含分组信息，多个分组共用同一账号时 MUST 共享同一缓存项；固定账号模式下每个选定账号的拉取 MUST 同样经过该缓存。缓存 MUST 至少容纳 512 个条目。

#### Scenario: OAuth 账号命中缓存
- **WHEN** 同一 OAuth 账号在新鲜期内被再次用于获取 manifest
- **THEN** 系统 MUST 直接返回缓存内容，MUST NOT 向 chatgpt.com 发起请求

#### Scenario: 多个分组共用同一账号
- **WHEN** 两个分组在同一时刻通过同一账号请求 manifest 且缓存已超期
- **THEN** 系统 MUST 只向上游发起一次请求，两个分组 MUST 各自基于该结果做分组级处理
- **THEN** 一个分组的过滤结果 MUST NOT 影响另一个分组收到的内容

#### Scenario: 凭据刷新后
- **WHEN** 账号的访问令牌被刷新
- **THEN** 下一次拉取 MUST 视为缓存未命中并同步请求上游

### Requirement: 新鲜期 1 分钟内强制使用缓存
缓存项写入后 1 分钟内 MUST 视为新鲜：系统 MUST 直接返回缓存内容且 MUST NOT 触发任何上游请求（包括后台刷新）。

#### Scenario: 新鲜期内并发请求
- **WHEN** 缓存写入后 30 秒内收到 10 个同账号 manifest 请求
- **THEN** 上游请求次数 MUST 为 0

### Requirement: 1 到 5 分钟乐观返回并后台刷新
缓存项写入后超过 1 分钟且不超过 5 分钟时 MUST 视为过期但可用：系统 MUST 立即返回缓存内容，并在后台对同一缓存键做单飞刷新。后台刷新 MUST 携带上游 ETag，上游返回 304 时 MUST 续期现有缓存项。

#### Scenario: 乐观期返回旧值
- **WHEN** 缓存写入后 3 分钟收到请求
- **THEN** 系统 MUST 立即返回缓存内容
- **THEN** 系统 MUST 触发一次后台上游刷新，同一时刻多个请求 MUST 只触发一次

#### Scenario: 后台刷新失败
- **WHEN** 乐观期后台刷新失败
- **THEN** 已返回给客户端的响应 MUST 不受影响
- **THEN** 缓存项 MUST 保持不变直到超期

### Requirement: 超过 5 分钟强制同步刷新
缓存项写入后超过 5 分钟 MUST 视为失效：系统 MUST 丢弃该缓存项，同步等待上游响应后再返回；上游失败时 MUST 向客户端返回错误而不是旧缓存。

#### Scenario: 超期后同步等待
- **WHEN** 缓存写入后 6 分钟收到请求
- **THEN** 系统 MUST 等待上游响应后返回新内容
- **THEN** 新内容 MUST 写入缓存并重新开始 1 分钟新鲜期

#### Scenario: 超期后上游失败
- **WHEN** 缓存已超期且上游请求失败
- **THEN** 系统 MUST 返回上游错误，MUST NOT 返回旧缓存

### Requirement: 条件请求基于缓存内容的 ETag
客户端携带的 `If-None-Match` MUST 与返回给客户端的最终响应体 ETag 比较，而不是透传给上游。缓存命中且 ETag 匹配时 MUST 返回 304。

#### Scenario: 缓存命中且 ETag 匹配
- **WHEN** 客户端 `If-None-Match` 等于当前缓存内容的 ETag
- **THEN** 系统 MUST 返回 304 且不请求上游

### Requirement: 普通模型发现使用相同缓存策略
普通固定账号模型列表的上游获取 MUST 使用本规范的同一套按账号缓存与单飞实现。完整请求 URL MUST 参与缓存键；API Key 的普通请求与 Codex 协议请求 MUST 分离。OAuth 普通列表使用规范版本获取的 manifest MUST 与相同请求条件的 Codex manifest 共享缓存。分组后处理 MUST NOT 修改缓存。

#### Scenario: OAuth 两种列表共用缓存
- **WHEN** 相同 OAuth 账号的普通列表与相同版本 Codex manifest 在新鲜期内连续请求
- **THEN** 系统 MUST 仅发起一次上游请求，并输出各自正确的响应结构

#### Scenario: API Key 两种协议分离
- **WHEN** 相同 API Key 账号先后被用于普通列表和带 client_version 的 Codex manifest
- **THEN** 各请求 MUST 获取并缓存自己的上游响应，MUST NOT 复用另一协议已转换的内容
