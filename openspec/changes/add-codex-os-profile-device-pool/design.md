## Context

当前系统已经具备账号 seed 生命周期、`off/device/session/full` 指纹模式、API Key isolation scope、HTTP/TLS client cache 分区、WS account+scope pool、账号级 WS budget、scoped response state 和第二回合 429 当前轮重放。新能力必须建立在这些边界之上，不能把共享设备槽位误写成共享连接或共享状态。

账号创建链目前存在三个独立提交：账号行、分组绑定、后续隐私/探测。账号行默认 `status=active`、`schedulable=true`，调度缓存 miss 又会回源数据库，因此导入尚未完成即可被选中。

现有 `codexFingerprintIDs` 只有改写后 ID，没有 Profile、slot、epoch、session policy 或双向映射；全局 `enforceCodexIdentityHeadersWithUA` 还会在最后覆盖 Profile 身份。新实现不能继续堆字段到旧结构。

## Goals / Non-Goals

**Goals:**

- 账号创建、编辑和所有导入入口使用一份可验证、可事务提交的配置快照。
- 支持四类 OS 与受约束 surface/arch；禁止跨 OS Adapter。
- 同一 API Key + OS 在同一 OAuth 账号内稳定绑定设备槽位。
- 设备可有限共享，session/thread/turn 由管理员策略决定，但不同 API Key 的连接和运行状态永不共享。
- 账号切换时保持同 OS Profile，更新粘性绑定并完整清理旧账号状态。
- 所有身份改写可在结构化响应中安全恢复。
- 默认关闭并保持旧模式行为。

**Non-Goals:**

- 不承诺规避上游政策、风控或账号限制。
- 不修改 PR #2 的全局 gateway runtime 配置。
- 不让客户端或导入文件提供 system-managed seed、slot ID、binding ID 或 alias。
- 不通过任意自由文本伪造 UA/Profile。
- 不在本 change 部署生产。
- 不在导入弹窗内提供逐行策略编辑器；JSON 文件可为每个账号携带独立策略，界面提供显式全局覆盖和逐账号结果，CRS 提供统一策略。

## Decisions

### 1. Provisioning state 与业务 status 分离

`accounts.provisioning_state` 使用 `pending|active`。存量账号迁移为 `active`。调度资格必须同时满足：

```text
provisioning_state=active
status=active
schedulable=true
```

完整本地 spec 可以在一个数据库事务内先插入 pending，再写 groups/policy/profiles/slots，最后更新 active 并写 scheduler outbox；事务提交前其他连接不可见。需要外部多步骤验证的流程可持久化 pending，但 scheduler cache 和数据库查询必须拒绝它。

### 2. AccountProvisioningSpec 是唯一写边界

```go
type AccountProvisioningSpec struct {
    Account       AccountMutableSpec
    GroupIDs      []int64
    Identity      *CodexIdentityPolicySpec
    FinalStatus   string
    Schedulable   bool
}
```

创建、编辑、OAuth、PAT、Codex session、RT、JSON、CRS 和批量入口只负责解析输入，随后调用 Provisioning Service。Service 负责 proxy FK、mixed-channel、header override、seed、Profile、session policy 和 slot 校验。每个批量账号独立事务，允许逐行成功/失败，但单账号不得半提交。

### 3. 使用关系表承载生命周期，不把新系统塞入 extra

- `account_codex_identity_policies`: mode、binding scope、session policy、TTL、unsupported policy、版本。
- `codex_identity_templates` 及 profile/slot 子表：设置页的命名权威模板与 revision。
- `account_codex_profiles`: 账号运行投影；`OS + canonical surface`、arch、profile proxy、slot count、epoch。
- `account_codex_device_slots`: profile、slot index、slot proxy、Codex client version mode/value、epoch、active/draining。
- `account_codex_device_bindings`: account + api key + OS + surface 到 slot 的稳定绑定。

`accounts.extra` 继续承载旧模式。新模式与旧 `codex_fingerprint_mode != off` 互斥。

### 4. Profile 使用封闭枚举和后端版本目录

OS: `windows|macos|linux|generic`。

Surface: `desktop|cli|sdk|third_party`。Profile 唯一键为 `(OS, surface)`；Generic 可同时启用 SDK/Third-party，其他 OS 可同时启用 Desktop/CLI。每个 surface 独立配置 Arch、槽位和代理。Arch 使用受约束枚举并由 Profile catalog 验证。Linux Desktop 同时支持 x86_64 和 arm64。

每个 Profile 使用 version epoch；同 epoch 的 UA/originator/version/body 规则必须同源。管理员不填写任意 UA。每个实际 slot 可选择 `client_version_mode=inherit|pinned`：`inherit` 按“管理员全局覆写 → 自动同步稳定版 → 内置版本”解析，`pinned` 必须填写不低于 `0.144.0` 的合法版本。生效版本同时驱动 User-Agent 版本段、`version` header 与结构化 client/turn metadata，但不选择模型，也不改 Desktop app build。`catalog_version` 仅表示封闭身份目录格式，不是 Codex 客户端版本。

slot 版本模式或固定值属于运行时物料。修改时只推进对应 OS/surface Profile 的 epoch，新槽位承接新请求，旧槽位按原版本排空。

全局 Codex 版本自动同步/管理员覆写是 `inherit` 槽位的运行时输入，不是逐账号的槽位配置变更：正在执行的 attempt 保留已解析版本，后续 attempt 读取新的合法版本并保留原设备 epoch。这模拟真实客户端升级，不触发全账号 epoch 扇出；只有 slot 的 `inherit|pinned` 或固定值发生变化时才创建新 epoch 并排空旧槽位。

### 5. 同 OS Adapter 必须完整双向

`CodexProfileAdapter` 接收原始可信请求快照和目标 Profile，输出 body/header transformation plan；不允许跨 OS。Adapter 必须覆盖 HTTP 非透传、HTTP 透传、WS v2/bridge/ingress、client metadata、turn metadata、workspace/path、prompt cache 和错误响应。

Profile 改写完成后使用 `enforceCodexIdentityHeadersWithProfile`，不能再调用会覆盖 Profile 的单一 canonical identity 收口。

### 6. Attempt 级计划替代旧单向 IDs

```go
type CodexIdentityAttemptPlan struct {
    AccountID int64
    APIKeyScope string
    Profile CodexResolvedProfile
    Slot CodexResolvedSlot
    SessionPolicy CodexSessionPolicy
    RequestMappings []CodexIdentityMapping
}
```

每次账号 attempt 新建一份，failover 必须覆盖旧计划。映射使用 HMAC-SHA256 domain separation。响应还原只在已知 JSON 字段和 SSE/WS JSON event 中执行，不扫描普通输出文本。

### 7. 会话策略可配置但带约束

- `conversation_isolated`: 默认；API Key + Profile + 原 conversation 假名化。
- `api_key_shared`: 每 API Key + Profile 固定 session。
- `session_pool`: 每设备固定 1-3 个 session 身份，API Key + conversation 粘性映射；`sessions_per_device` 是身份池大小，不是并发数。
- `device_shared`: 每设备一个 session；仅此模式要求每个实际设备槽位同一时刻最多一个进行中的 HTTP/SSE 请求流或一个 WS 会话，并关闭跨 Key continuation。请求流/会话结束即释放槽位；客户端断开但主机仍在排空上游以完成计费时，租约继续保留。它不是账号全局并发 1，其他三种策略也没有这层额外互斥。

### 8. Proxy 分层且变化创建新 epoch

Profile 与 slot 的代理路由使用显式 `proxy_mode=inherit|proxy|direct`；只有 `proxy` 可携带 `proxy_id`。有效路由按 slot > profile > account default 解析，`direct` 是终止继承的显式直连，不能用 `proxy_id=null` 同时表示继承和直连。代理到期回退为 direct 时必须持久化 `direct`，即使账号仍配置默认代理也不得重新继承。

导入通过当前部署稳定 proxy key/name 映射，不能信任外部数值 ID。代理路由或 canonical Profile 改变必须建立新 epoch并排空旧槽位。已发布 migration 不改写；三态路由通过后续 migration 增量回填和约束。

### 9. 独立 Profile affinity namespace

新 sticky key包含 API Key scope、OS、conversation 和 policy version；不修改 legacy OpenAI sticky namespace。兼容过滤必须覆盖 previous-response、session sticky 和普通候选。429 换号只绑定同 OS Profile 的新账号，并在 TTL 内不自动反跳。

### 10. PR #2 隔离是不可削弱的不变量

即使两个 API Key 绑定同一账号同一设备槽位，它们的 HTTP client cache key、WS pool key、response/account、response/conn、session/turn-state 和 session/conn key仍必须不同。账号级 WS budget不得按 Key倍增。

### 11. 混合池保持 default-off 兼容

`mode=off` 账号继续使用 legacy 身份与 sticky 语义，不能因为同组另一个账号启用了新模式就被全局排除。启用了 `os_profile_device_pool` 但没有请求 OS 的账号视为不兼容；一旦对话建立了 Profile affinity，故障转移只能进入同 OS 的新模式账号，不得降级回 `off`。因此，管理员若要求“无兼容 Profile 必须返回 `DEVICE_PROFILE_UNSUPPORTED`”的严格口径，应将该调度组内相关候选账号全部切换到新模式。

### 12. UI 使用一个受控编辑器覆盖所有入口

`CodexIdentityPolicyEditor` 使用同一 `AccountProvisioningSpec`，在 create/edit/import-default/bulk-patch context 下提供差异化交互。OAuth 授权前完成 spec 校验；JSON 导入保留文件内逐账号策略并提供显式全局 override，CRS 提供统一策略，两者都返回逐账号结果。Seed/alias/slot ID永不进入前端。

## Failure Semantics

- 无兼容账号：`DEVICE_PROFILE_UNSUPPORTED`。
- spec 不完整：`ACCOUNT_PROVISIONING_INVALID`。
- pending 账号被读取为候选：当作不可调度并从单账号 cache 删除。
- slot draining：旧绑定可续用，新绑定禁止进入。
- 响应 payload malformed：不执行猜测性替换；保留协议错误并记录稳定代码。

## Rollout

新模式默认 off。独立 Draft PR 完成本地和 GitHub CI，不部署生产。生产 canary 需要用户另行授权。
