## Why

PR #2 已为 HTTP/TLS、OpenAI WebSocket 和 response state 建立下游 API Key 隔离，但现有 Codex 指纹模式仍只能在“原样透传”“每 Key 独立设备/会话”和“账号级过度收敛”之间选择。多人、多客户端、多 OAuth 账号场景缺少一个同时保持客户端协议一致、限制上游可见设备身份、保留对话独立性并让账号导入原子激活的完整边界。

当前账号创建还会先写入 `active + schedulable` 账号，再单独绑定分组和执行异步隐私初始化。新导入账号可能在 Profile、槽位、代理和会话策略尚未完成前被数据库回源调度选中。该竞态不能由前端提交顺序解决，必须建立后端强类型 Provisioning 契约和原子激活事务。

本变更参考 CPA 的凭据粘性与确定性假名化、Cockpit Tools 的账号级设备和结构化身份改写、New API 的 prompt-cache 渠道亲和，但不照搬其缺少租户隔离或过度透传的部分。

## What Changes

- 新增默认关闭的 `os_profile_device_pool` 身份模式，保留现有 `off/device/session/full` 语义。
- 新增 Windows、macOS、Linux、Generic 四类 OS Profile，支持 Desktop、CLI、SDK/Third-party surface；Linux Desktop 为正式 Profile。
- 每个 OpenAI OAuth 账号可配置启用的 OS、同 OS canonical surface、arch、设备槽位数、会话策略、账号默认代理及 Profile/槽位代理覆盖。
- 新增强类型 `AccountProvisioningSpec` 和统一 Provisioning Service；创建、编辑、OAuth/PAT、Codex 单/批量导入、通用 JSON 导入、CRS 同步都必须经过同一边界。
- 新增独立 provisioning state。账号只有在凭据、seed、策略、Profile、槽位、代理和分组原子提交后才可进入调度器。
- 新增 Profile 分类、兼容账号过滤、API Key + OS 设备槽位粘性、独立 Codex Profile sticky namespace 和 429 后受控重绑定。
- 新增 Attempt 级双向身份计划，结构化处理 HTTP JSON、SSE 和 WS 中的 installation/session/thread/turn/prompt-cache/workspace 身份，并恢复客户端原 ID；禁止全局字符串替换。
- 保持 PR #2 的 HTTP/TLS、WS pool、previous response、response state 和 turn-state API Key 隔离以及账号级 WS 总预算。
- 新增完整创建/编辑/单导入/批导入前端策略编辑器、排空状态和逐账号导入结果。
- 新增不可变 SQL migration、Ent schema、后端/前端/迁移/并发/HTTP/SSE/WS 测试和独立 Draft PR CI。

## Capabilities

### New Capabilities

- `codex-account-provisioning`: 强类型账号配置、原子创建/编辑/导入和不可调度的 provisioning 生命周期。
- `codex-os-profile-device-pool`: OS Profile、设备槽位、代理覆盖、稳定设备 ID 和会话策略。
- `codex-profile-affinity`: API Key + OS + conversation 到账号/槽位的粘性、兼容过滤和故障重绑定。
- `codex-identity-roundtrip`: HTTP/SSE/WS 请求身份改写和结构化响应恢复。
- `codex-identity-console`: 创建、编辑、导入、批量和排空管理 UI。

### Modified Capabilities

- PR #2 连接与状态隔离只增加不变量测试，不改变其配置和运行语义。

## Impact

- **数据库**：新增 provisioning state 及 Codex identity policy/profile/slot/binding 表。
- **后端**：新增 provisioning、profile classifier/adapter、slot resolver、affinity 和 response restorer 模块；所有账号入口改走统一 service。
- **调度**：新模式仅选择兼容 Profile 且 provisioning active 的账号；旧模式调度不变。
- **前端**：新增共享策略编辑器并接入创建、编辑、OAuth/PAT、Codex session、JSON/CRS 导入和批量编辑。
- **兼容性**：新字段可选、功能默认关闭、存量账号迁移为 provisioning active；旧指纹模式逐字保持。
- **发布**：作为以 PR #2 分支为 base 的独立 Draft PR；本变更不部署生产。
