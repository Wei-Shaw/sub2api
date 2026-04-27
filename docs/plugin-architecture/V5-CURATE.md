# V5 Curator — 通用集成能力 SDK 化（决策书 + 任务图）

> 作用：基于 V5-INSPECT 把 5 项必做能力的"名字 / 接口签名 / proto 骨架 / 跨进程边界 / 失败降级 / 业界参考 / 验收锚点"全部敲死，给 Implementer 一份能直接拆并行 sub-agent 的任务图。
> 范围：**只做 5 项**通用集成能力 + 2 个示范功能（Channel Monitor / Available Channels）的迁移。
> 设计原则（用户原话，最高优先级）：**集成能力应该抽象、解耦、复用、统一，不要专门为了渠道功能定制**。SDK 的 API 名 / proto 名 / 文档表述里**绝对不能出现 channel 关键字**。
> base：`42ca2e49 Merge V5 inspector`（`feat/plugin-system-fixes`）
> 子分支：`feat/plugin-system-fixes--v5-curate`

---

## 0. 角色与边界（一句话再确认）

- 我是 Curator（Planner + Designer 合并）。
- 上游输入 = `V5-INSPECT.md`（5 项必做、4 个待决问题、迁移依赖矩阵、隐含的 V6+ HostServiceProxyCapability）。
- 下游产出 = Implementer 阶段可按任务图直接 fork 出多个并行 sub-agent。
- **本期不做**：能力 C/E/H/I/J/K/L/M/N（V6+），HostServiceProxyCapability（V6+），TimeSeriesStore（让 Channel Monitor 用 MigrationRunner 自管 history 表跑起来）。

---

## 1. 4 个 Inspector 提问 — 最终决策

| # | 问题 | 决策 | Why |
|---|------|------|-----|
| Q1 | JobScheduler job 跑哪？ | **plugin 进程跑 handler，host 进程负责 leader lock + 触发信号 + 优雅关停** | 维持插件隔离边界（plugin crash 不污染 host 内存）；leader lock 已在 host Redis（OpsCleanupService），plugin 拿不到也不该拿；触发由 host 经 stream RPC 推回 plugin，与现有 EventBus / LogProxy 一致的双进程范式 |
| Q2 | Settings schema 格式 | **JSON Schema Draft-07，参考 Backstage `configSchema` 子集** | 前端 react-jsonschema-form / vue-json-schema-form 生态已成熟；protobuf descriptor 给前端渲染表单需要额外 reflection 层；JSON Schema 还能直接喂 AJV 做 server-side 校验 |
| Q3 | plugin migration 失败要不要阻塞 plugin 启动 | **是，与 host migration 一致；失败则 plugin status=ErrorMigrationFailed，gateway 摘流，admin 页面显式标红** | migration 失败 = schema 不一致，继续跑业务等于数据腐败；与 host 一致符合最小惊讶原则；同时支持 `disabled=true` 跳过 migration（让运维有 escape hatch） |
| Q4 | 加密密钥分域 | **per-plugin HKDF derive：`pluginKey = HKDF-SHA256(masterKey, salt=plugin_name, info="sub2api-plugin-secret-v1")`** | 防"plugin A 解 plugin B 密文"的横向越权；HKDF 单次 ~1µs 不影响性能；与 AWS Hierarchical Keyring / Keeper 模式一致；master key 只放 host，plugin 进程拿不到原始 key |

---

## 2. 5 项能力 — 范围确认（每项 7 维）

### 2.1 W1: MigrationRunnerCapability（能力 A）

**A. 名字**：`MigrationRunnerCapability`（manifest 字符串常量 `pluginsdk.CapabilityMigrationRunner = "migration_runner"`）。proto service 名 `MigrationProxy`。

> 命名理由：动词 + 角色，不带 "schema/db/sql"（host 后续可能用同一能力推 ent 自动 migrate 或 mongo migrate）。

**B. SDK 接口签名**（Go）

```go
// plugin-sdk/migration.go
type MigrationFile struct {
    Filename string // e.g. "001_create_monitor.sql"，按字典序应用
    SQL      string // 完整 SQL 文本
    Checksum string // SHA-256 hex；SDK 自动算
}

type MigrationProvider interface {
    // ListMigrations 返回 plugin 内嵌的 SQL 集合（embed.FS 友好）
    ListMigrations() ([]MigrationFile, error)
}

// Plugin 在 Init 阶段把 provider 交给 SDK，SDK 在 grpc Manifest 阶段
// 把 filenames + checksums 发给 host，host 通过 GetMigration RPC 按需拉取 SQL 内容
sdk.RegisterMigrationProvider(p MigrationProvider) error
```

**C. proto 骨架**（追加到 `plugin-sdk/proto/sdk.proto`）

```proto
service MigrationProxy {
  // host 拉取 plugin 内嵌的 SQL 内容；plugin 实现端
  rpc GetMigration(GetMigrationRequest) returns (GetMigrationResponse);
}
message GetMigrationRequest { string filename = 1; }
message GetMigrationResponse { string sql = 1; string checksum = 2; }

// ManifestResponse 已含 migration_files []string；本期扩展
message MigrationDecl {
  string filename = 1;
  string checksum = 2; // SHA-256 hex
  bool   non_transactional = 3; // 是否包含 CREATE INDEX CONCURRENTLY 等
}
// 在 ManifestResponse 中新增字段：
//   repeated MigrationDecl migrations = 31; (旧 migration_files 保留为 deprecated 兼容字段)
```

**D. 跨进程边界**

- plugin 进程：`embed.FS` 读 SQL，算 checksum，注册到 SDK，通过 `GetMigration` RPC 按需返回内容。
- host 进程：在 plugin Init 之后、首次健康检查之前，按 `migrations` 顺序对 plugin 命名空间 schema 执行 SQL：
  1. `pg_advisory_xact_lock(hashtext('sub2api_plugin_migration_' || plugin_name))` 获取事务级 advisory lock
  2. 读 `plugin_schema_migrations(plugin_name, filename, checksum, applied_at)` 表
  3. 对未应用的 filename 调 `MigrationProxy.GetMigration` 拉 SQL，校验 checksum，执行
  4. 写一条 `plugin_schema_migrations` 记录
  5. 释放 lock（事务提交时自动）

**E. 失败/降级**

- checksum 不匹配 → 立即终止，记 `plugin_status=ErrorMigrationDrift`，admin 看到红色卡片
- SQL 执行失败 → plugin 启动失败（Q3 决策），不进入 ready 状态，gateway 摘流
- advisory lock 等待超时（默认 30s）→ 拒绝启动，等下次重启重试
- `non_transactional=true` 的 migration 自动在 lock 外单独跑（参考 pg-safe-migrate `transaction=auto`）
- 运维 escape hatch：`plugins.<name>.skip_migration=true` 配置项跳过

**F. 业界参考**

1. **pg-safe-migrate** ([dev.to](https://dev.to/defnotwig/pg-safe-migrate-stop-shipping-unsafe-postgres-migrations-1m5n)) — 采用其 advisory lock + checksum + auto-detect non-transactional 三件套
2. **Atlas Cloud plugin-driven migrations** + **Backstage `app-config-schema`** plugin → host 模式 — 采用其"plugin 提供 schema，host 统一执行"架构

**G. 验收锚点**

- `backend/migrations/125_channel_monitor.sql` … `128_*` → 改名挪到 plugin 包内 embed.FS，host 启动后能在 PG 看到这 4 张表
- `plugin_schema_migrations` 表里有 `channel-monitor / 001…004 / checksum / applied_at`

---

### 2.2 W2: JobSchedulerCapability（能力 B）

**A. 名字**：`JobSchedulerCapability`（`pluginsdk.CapabilityJobScheduler = "job_scheduler"`）。proto service 名 `JobScheduler`。

> 不叫 "Cron"：能力包含 ticker / fixed-delay / leader-locked once-per-day 三种触发器，cron 只是其中之一。

**B. SDK 接口签名**

```go
// plugin-sdk/jobs.go
type JobSpec struct {
    Name        string         // plugin 命名空间内唯一
    Trigger     JobTrigger     // 见下
    LeaderOnly  bool           // true = 只有 leader 节点跑（适合每日聚合）
    Concurrency int            // 同一 trigger 多实例上限（防重入），默认 1
    Timeout     time.Duration  // 单次执行最大耗时，超过取消 ctx
}

type JobTrigger struct {
    Kind        string         // "interval" | "cron" | "fixed_delay"
    Interval    time.Duration  // Kind=interval 用
    CronSpec    string         // Kind=cron 用，标准 5/6 字段 cron
    FixedDelay  time.Duration  // Kind=fixed_delay 用（执行结束后等 N 再下一次）
}

type JobHandler func(ctx context.Context, jobName string) error

sdk.Jobs().Register(spec JobSpec, h JobHandler) error
sdk.Jobs().Trigger(jobName string) error  // 手动触发（admin "Run now" 按钮用）
```

**C. proto 骨架**

```proto
service JobScheduler {
  // plugin 启动时声明 specs，返回流式 Trigger 信号（host 持有 leader lock，到点 push 信号）
  rpc Subscribe(stream JobMessage) returns (stream JobTrigger);
}
message JobMessage {
  oneof msg {
    JobRegistration register = 1;     // 启动时声明所有 spec
    JobAck          ack      = 2;     // handler 执行完上报结果
    ManualTrigger   manual   = 3;     // admin Run now 转发回 host
  }
}
message JobRegistration { repeated JobSpec specs = 1; }
message JobSpec {
  string name = 1; string kind = 2; int64 interval_nanos = 3;
  string cron_spec = 4; int64 fixed_delay_nanos = 5;
  bool leader_only = 6; int32 concurrency = 7; int64 timeout_nanos = 8;
}
message JobTrigger { string job_name = 1; string trigger_id = 2; int64 fire_time_unix_nano = 3; }
message JobAck { string trigger_id = 1; bool success = 2; string error = 3; int64 duration_nanos = 4; }
```

**D. 跨进程边界**

- **host**：维护 JobScheduler 实例（基于 `robfig/cron/v3`），持 leader lock（复用现有 `OpsCleanupService.acquireLeaderLock`）。到点对每个订阅 plugin push `JobTrigger`。
- **plugin**：收到 trigger 后在 plugin 进程跑 handler，限并发，超时取消，结束时 push `JobAck`。
- 优雅关停：plugin shutdown 时取消所有 in-flight handler，host 端把未 ack 的 trigger 标 cancelled。

**E. 失败/降级**

- plugin 进程 down → host 检测 stream EOF，标记其 specs inactive；plugin 重连后自动续订
- handler 返回 error → host 记入 `plugin_job_history` 表（用 W3 的 setting 决定 retry 策略；本轮先不做自动 retry，admin 看错误日志）
- leader_only 但本节点不是 leader → host 跳过这次触发（不 push 给 plugin）
- cron spec 解析失败 → plugin 启动时 `Register` 直接报错，传染到 W1 的"启动失败"

**F. 业界参考**

1. **River** ([riverqueue/river](https://github.com/riverqueue/river)) — 采用其"work function 注册 + 周期/cron 触发分离"概念
2. **Asynq scheduler** ([hibiken/asynq](https://github.com/hibiken/asynq)) — 采用其 leader 单例调度 + worker 多实例消费模型，对应到我们的 host=scheduler / plugin=worker

**G. 验收锚点**

- 删除 `backend/internal/service/channel_monitor_runner.go` 自管 ticker（291 行），改为 plugin 内 `sdk.Jobs().Register("monitor.run", interval=cfg.Interval, handler=runMonitor)`
- `RunDailyMaintenance` 改为 `sdk.Jobs().Register("monitor.daily-rollup", cron="0 2 * * *", LeaderOnly=true, handler=...)`
- admin "Run now" 走 `sdk.Jobs().Trigger("monitor.run")`

---

### 2.3 W3: SettingsExtensionCapability（能力 D）

**A. 名字**：`SettingsExtensionCapability`（`pluginsdk.CapabilitySettingsExtension = "settings_extension"`）。proto service 名 `SettingsExtension`。

> 不叫 "FeatureFlag"：能力既包含布尔开关也包含整数/枚举/嵌套对象等通用配置；feature flag 是子集。

**B. SDK 接口签名**

```go
// plugin-sdk/settings.go
type SettingsSchema struct {
    Namespace string          // 自动 = plugin name；admin 页面以此分 tab
    JSONSchema json.RawMessage // JSON Schema Draft-07，含 default / title / description / x-visibility
    Defaults   map[string]any  // 与 schema 内 default 等价的 Go 形式（host 缓存用）
}

type SettingsClient interface {
    Get(ctx context.Context, key string) (json.RawMessage, error)
    GetTyped(ctx context.Context, key string, out any) error // JSON unmarshal
    Watch(ctx context.Context, key string) (<-chan json.RawMessage, error) // 监听变更
}

sdk.RegisterSettingsSchema(s SettingsSchema) error
sdk.Settings() SettingsClient
```

**C. proto 骨架**

```proto
service SettingsExtension {
  rpc Get   (SettingsGetRequest)   returns (SettingsGetResponse);
  rpc Watch (SettingsWatchRequest) returns (stream SettingsChangeEvent);
}
message SettingsGetRequest  { string key = 1; }
message SettingsGetResponse { bytes value_json = 1; bool exists = 2; }
message SettingsWatchRequest { string key = 1; } // 空 key = 整个 namespace
message SettingsChangeEvent  { string key = 1; bytes value_json = 2; int64 revision = 3; }

// ManifestResponse 中新增：
//   bytes settings_schema_json = 32;   // JSON Schema Draft-07
//   bytes settings_defaults_json = 33; // map<string, any> 的 JSON
```

**D. 跨进程边界**

- **host**：扩展 `SettingService`：除当前硬编码 struct 外，新增"plugin 命名空间"分区，存到 `plugin_settings(plugin_name, key, value_jsonb, revision)` 表。admin 前端读 host 的 `GET /api/v1/admin/settings/plugins/:name`，渲染 schema 出表单，写回走 `PUT`。
- **plugin**：通过 `Settings().Get/Watch` 跨进程读取，host 把变更通过 Watch stream 推送（基于 PG LISTEN/NOTIFY 或内存 fan-out）。

**E. 失败/降级**

- schema 校验失败（admin 页面提交不合法值）→ host 拒绝写入，返回 422
- plugin 重启时 schema 与已存值不兼容（schema 收紧了） → host 校验失败的字段回退到 default，写入 `plugin_settings_drift` 审计表
- `Watch` 断流 → SDK 自动重连 + 全量同步当前值

**F. 业界参考**

1. **Backstage `configSchema`** ([backstage.io](https://backstage.io/docs/conf/defining/)) — 采用其"plugin package.json 声明 JSON Schema，host 拼接渲染"模型；`x-visibility` 关键字直接借用
2. **WordPress Settings API** — 采用其"register_setting → admin 自动渲染表单"语义

**G. 验收锚点**

- `backend/internal/service/setting_service.go` 中的 `GetChannelMonitorRuntime` / `GetAvailableChannelsRuntime` 删掉
- Channel Monitor plugin 的 `manifest.go` 内含 settings schema：`enabled`, `defaultIntervalSec`, `templateMaxBodyKB`, `dailyRollupHourUTC`
- admin 页 `/admin/settings` 出现"Channel Monitor"和"Available Channels"两个 tab

---

### 2.4 W4: SafeOutboundHTTPCapability（能力 F）

**A. 名字**：`SafeOutboundHTTPCapability`（`pluginsdk.CapabilitySafeOutboundHTTP = "safe_outbound_http"`）。SDK 不需要新 proto，**纯 SDK 内置工具**（不跨进程，DialContext 在 plugin 进程里跑，但黑名单从 host 同步）。

> 不叫 "SSRFProtect"：能力还含超时 / 代理 / redirect cap 等通用 HTTP 加固，SSRF 只是核心一项。

**B. SDK 接口签名**

```go
// plugin-sdk/outbound.go
type OutboundConfig struct {
    AllowedSchemes []string      // ["https"]，默认仅 https
    AllowedPorts   []int         // [443]，默认仅 443
    ExtraBlockedCIDRs []string   // 在 RFC1918 / 169.254 / 100.64 等默认黑名单上叠加
    AllowedHosts   []string      // 白名单 host（精确匹配，如 oauth.googleapis.com）
    MaxRedirects   int           // 默认 3
    MaxBodyBytes   int64         // 默认 1MiB（保护 plugin 内存）
    Timeout        time.Duration // 默认 30s
    Proxy          string        // 可选 HTTPS 代理
}

func sdk.NewSafeHTTPClient(cfg OutboundConfig) (*http.Client, error)
```

**C. proto 骨架**

无新 RPC。`PluginInitRequest.capabilities` 中含 `safe_outbound_http` 时，host 在 init 响应里把"管理员配置的全局 SSRF 黑名单"作为初始 `OutboundConfig.ExtraBlockedCIDRs` 透传给 plugin（通过现有 init RPC 字段扩展，不新增 service）。

```proto
// PluginInitResponse 新增：
//   message OutboundDefaults { repeated string blocked_cidrs = 1; ... }
//   OutboundDefaults outbound_defaults = 50;
```

**D. 跨进程边界**

- 实现完全在 plugin 进程：基于 `doyensec/safeurl` 包装 + 自定义 DialContext，**Time-of-Use 校验**（防 DNS rebinding）。
- host 只在 init 阶段下发"管理员额外的黑名单 CIDR"，plugin 缓存使用，settings 变更时通过 W3 Watch 推回，热更新。

**E. 失败/降级**

- 解析后 IP 命中黑名单 → 返回 `ErrBlockedTarget`，调用方负责处理
- DNS 解析失败 / 超时 → 标准 `*url.Error`
- 重定向次数超限 → `ErrTooManyRedirects`
- 响应 body 超 `MaxBodyBytes` → `io.LimitReader` 截断 + warn log

**F. 业界参考**

1. **doyensec/safeurl** ([doyensec/safeurl](https://github.com/doyensec/safeurl)) — 采用：DialContext Control hook 做 TOCTOU-safe 校验，黑/白名单结合
2. **daenney/ssrf** ([daenney/ssrf](https://github.com/daenney/ssrf)) — 参考其 zero-dep `Safe()` 实现作为 fallback（如果 doyensec 引入新依赖太重）

**G. 验收锚点**

- 删除 `backend/internal/service/channel_monitor_ssrf.go`（152 行 SSRF guard）
- `channel_monitor_checker.go` 的 `safeDialContext` 改为 `client := sdk.NewSafeHTTPClient(cfg)`
- 单测：调用 `http://169.254.169.254/`（云元数据 IP）必须返回 `ErrBlockedTarget`

---

### 2.5 W5: SecretEncryptionCapability（能力 G）

**A. 名字**：`SecretEncryptionCapability`（`pluginsdk.CapabilitySecretEncryption = "secret_encryption"`）。proto service 名 `SecretEncryption`。

> 不叫 "AESEncrypt"：未来可能切到 envelope encryption / KMS，能力名要描述"是什么"不是"怎么实现"。

**B. SDK 接口签名**

```go
// plugin-sdk/secrets.go
type SecretEncryptor interface {
    Encrypt(ctx context.Context, plaintext []byte) ([]byte, error) // 返回密文 = nonce(12)+ciphertext+tag
    Decrypt(ctx context.Context, ciphertext []byte) ([]byte, error)
}

sdk.Secrets() SecretEncryptor
```

**C. proto 骨架**

```proto
service SecretEncryption {
  rpc Encrypt(EncryptRequest) returns (EncryptResponse);
  rpc Decrypt(DecryptRequest) returns (DecryptResponse);
}
message EncryptRequest  { bytes plaintext  = 1; }
message EncryptResponse { bytes ciphertext = 1; }
message DecryptRequest  { bytes ciphertext = 1; }
message DecryptResponse { bytes plaintext  = 1; }
```

**D. 跨进程边界**

- **host**：在启动时初始化 `masterKey`（来自 `.env` `ENCRYPTION_KEY`，必须 32 bytes hex）。每个 plugin 第一次调用时按 Q4 决策派生 `pluginKey = HKDF-SHA256(masterKey, salt=plugin_name, info="sub2api-plugin-secret-v1")`，缓存。Encrypt/Decrypt 用 `pluginKey` 跑 AES-256-GCM。
- **plugin**：拿到的 ciphertext 已经是 plugin namespace 隔离的，plugin A 想 Decrypt plugin B 的密文 → host 用 A 的 pluginKey 解、自然失败。
- ciphertext 体积保护：单次 ≤ 64KiB（防滥用 RPC 当通用加密代理）。

**E. 失败/降级**

- masterKey 不是 32 bytes → host 启动失败（已是现状的最佳实践）
- plaintext 超长 → 返回 `ErrSecretTooLarge`
- Decrypt 校验失败（密文损坏 / 跨 plugin 篡改）→ 返回 `ErrSecretInvalid`，host 记 audit log（注意不记 plaintext / ciphertext）
- 不提供"换 key 后能解旧密文"的轮换能力（V6+ 议题）

**F. 业界参考**

1. **HashiCorp Vault Transit Engine** — 采用其"key 管理与 plaintext 暴露分离"语义
2. **AWS Hierarchical Keyring + HKDF** ([AWS docs](https://docs.aws.amazon.com/database-encryption-sdk/latest/devguide/use-hierarchical-keyring.html)) — 采用其 per-tenant HKDF 派生模型（plugin = tenant）

**G. 验收锚点**

- `backend/ent/schema/channel_monitor.go` 的 `api_key` 字段加密 → 改用 `sdk.Secrets().Encrypt`
- 单测：plugin A 加密的字符串，传给 plugin B 调 Decrypt 必须失败

---

## 3. 任务图（给 Implementer 拆并行 sub-agent 用）

> 类型缩写：`sdk` = SDK + proto + host server；`migrate` = 把现成功能挪进 plugin；`verify` = E2E 验收。
> 工作量：**小** ≈ 0.5 day，**中** ≈ 1-1.5 day，**大** ≈ 2+ day。

| Item | 类型 | 描述 | 依赖 | 工作量 | 可并行？ |
|------|------|------|------|--------|---------|
| **W1** | sdk | MigrationRunnerCapability：proto + SDK migration.go + host migration runner（advisory lock + plugin_schema_migrations 表 + checksum 校验 + non-tx 自动检测） | 无 | 中 | **是** |
| **W2** | sdk | JobSchedulerCapability：proto + SDK jobs.go + host scheduler service（基于 robfig/cron + leader lock 复用 OpsCleanupService） | 无 | 中 | **是** |
| **W3** | sdk | SettingsExtensionCapability：proto + SDK settings.go + host settings_extension_service.go + admin 前端动态 schema 渲染（react-jsonschema-form 或 vue-json-schema-form） | 无（前端部分可与 W3 后端并行） | 中 | **是** |
| **W4** | sdk | SafeOutboundHTTPCapability：SDK outbound.go（基于 doyensec/safeurl 包装）+ host init 下发黑名单 + 单测 | 无 | 小 | **是** |
| **W5** | sdk | SecretEncryptionCapability：proto + SDK secrets.go + host secret_encryption_service.go（HKDF 派生 + AES-GCM） | 无 | 小 | **是** |
| **W1a** | sdk | manifest.go 扩展 `MigrationDecl`，capability 常量集中 | 无 | 小 | **是**（与 W1 同 sub-agent） |
| **W6** | migrate | Channel Monitor 后端迁移：从 `git show 09fd83ab:<path>` 取代码，重构为 plugin → 4 张表 SQL 迁入 plugin embed.FS（用 W1）；runner 改为 W2 注册 jobs；checker 用 W4 的 SafeHTTPClient；api_key 加密改 W5；feature flag + 阈值改 W3 | **W1, W2, W4, W5, W3** | 大 | **否** |
| **W7** | migrate | Channel Monitor 前端迁移：admin/user 页面（304+172+9+7 个组件）走 SDK install(sdk) + plugin manifest 注册菜单 + 路由；i18n 走 sdk.i18n.registerNamespace；feature flag 走 W3 settings 客户端 | **W6（仅 API 形状契约）+ W3（前端 schema 渲染基础设施）** | 中 | **是**（与 W6 后端 admin handler 完成后并行） |
| **W8** | migrate | Available Channels 后端迁移：feature flag 改 W3；其他不阻塞 V5（不依赖 W1/W2/W4/W5）；用 SQLProxy 直查 host channel/group/pricing 表（与 V6+ HostServiceProxyCapability 接口风险点见 §4） | **W3** | 中 | **是**（与 W6 并行） |
| **W9** | migrate | Available Channels 前端迁移：127+110+29+214 行组件走 plugin manifest；channel.ts 常量打包入 plugin | **W8** | 小 | **是** |
| **W10** | verify | E2E 验收：Channel Monitor + Available Channels 两个 plugin 在 beta 环境跑通；按 §5 验收清单逐项打勾 | **W6, W7, W8, W9** | 小 | **否** |

### 依赖图（DAG，给 Implementer 拆 sub-agent 直接看）

```
              ┌─ W1 (Migration) ─┐
              ├─ W2 (Scheduler) ─┤
              ├─ W4 (SafeHTTP)  ─┼──→ W6 (Monitor backend) ──┐
              ├─ W5 (Encrypt)   ─┤                            │
[start] ─────►├─ W3 (Settings)  ─┼──→ W8 (Available backend) ─┤
              │                  │           │                │
              └──────────────────┘           ▼                ▼
                       │              W9 (Avail FE)     W7 (Monitor FE)
                       │                    │                 │
                       └────────────────────┴────┬────────────┘
                                                 ▼
                                              W10 (verify)
```

### 关键路径分析

- **关键路径** = `W1+W2+W3+W4+W5 (并行 ~1.5 day) → W6 (大, 2+ day) → W7 (中, 1.5 day) → W10`
- **非关键路径**：W8/W9 总耗 ~2 day，因 W6 路径更长所以不上关键路径
- **并行 sub-agent 拆分建议**：
  - Wave 1（5 个 SDK sub-agents 并发）：W1 / W2 / W3 / W4 / W5
  - Wave 2（2 个迁移 sub-agents 并发）：W6 / W8（W8 仅依赖 W3，可比 W6 早开工）
  - Wave 3（2 个前端 sub-agents 并发）：W7 / W9
  - Wave 4（1 个 verify sub-agent）：W10

---

## 4. 迁移策略（W6/W7/W8/W9 共用模式）

### 4.1 取代码

```bash
# 所有 Monitor + Available Channels 文件最后存在的 commit
git show 09fd83ab:backend/ent/schema/channel_monitor.go > <plugin-path>/...
git show 09fd83ab:backend/internal/service/channel_monitor_runner.go > <plugin-path>/...
# ... 总计 50 后端 + 25 前端文件，按 V5-INSPECT §1.1/§1.2 表格逐一拷贝
```

### 4.2 后端改造点（W6）

| 旧代码 | 改造后 |
|--------|--------|
| `import "github.com/Wei-Shaw/sub2api/backend/internal/service"` 拿 `SecretEncryptor` | `sdk.Secrets()` |
| `safeDialContext` 152 行 SSRF guard | 删除，改 `client := sdk.NewSafeHTTPClient(cfg)` |
| `runner.go` 自管 `time.Ticker` + pond pool | 删除，改 `sdk.Jobs().Register(JobSpec{Name:"monitor.run", Trigger:JobTrigger{Kind:"interval"}, Concurrency:5}, runOne)` |
| `OpsCleanupService.RunDailyMaintenance` 调 monitor service | 改 `sdk.Jobs().Register(JobSpec{Name:"monitor.daily-rollup", Trigger:JobTrigger{Kind:"cron",CronSpec:"0 2 * * *"}, LeaderOnly:true}, ...)` |
| `setting_service.GetChannelMonitorRuntime` host 硬编码 | 删除，plugin manifest 内 settings schema + `sdk.Settings().GetTyped(ctx, "")` |
| `backend/migrations/125-128_*.sql` | 移到 `plugins/channel-monitor/migrations/00{1-4}_*.sql`（embed.FS） |
| `dbent.Client` 全局 | 不能跨进程；plugin 用 ent 时只能在 plugin schema 子集，host 全局 ent client 不开放（保持隔离） |
| `repository/channel_monitor_repo.go` 走 `database/sql` 直查 | 走 SDK SQLProxy（已有），SQL 逻辑搬过去（不重构原生 SQL） |

### 4.3 删除项（host 端清理）

按 V5-INSPECT 表注的 `09fd83ab → bd04bfa7 已删除` 路径，host 端理论上已无残留；W6 只需在 plugin 端建立。但要核对：

```bash
# 验证 host 端确实没残留
grep -r "channel_monitor\|ChannelMonitor" backend/internal/ backend/ent/ \
  --exclude-dir=node_modules \
  | grep -v "^Binary" | head -20
# 期望：0 命中（除了 wire.go / register 文件被 V1 host-clean 处理过）
```

### 4.4 前端模式（W7/W9）

按 V1+V2+V3 已确立模式：

```ts
// plugins/channel-monitor/frontend/src/index.ts
import type { PluginSDK } from '@sub2api/plugin-sdk'

export default function install(sdk: PluginSDK) {
  sdk.i18n.registerNamespace('monitor', { en: en, zh: zh })
  // 路由 / 菜单已在 manifest 声明，sdk runtime 自动 mount
}
```

- **manifest 端**：声明菜单 `Section=admin/user`、路由 `ComponentPath="src/views/admin/MonitorView.vue"`、frontend 入口 `EntryJS="dist/monitor.js"`
- **i18n**：`monitor.*` 命名空间通过 SDK `registerNamespace` 注册
- **API client**：plugin 自己的 `api/monitor.ts`，走 plugin gateway（host 已有 V4 X-Plugin-User-* header 转发）

---

## 5. 验收清单（V1-V4 模板，10 项）

- [ ] W1 ✅ `plugin_schema_migrations` 表存在，channel-monitor 4 个 migration 记录写入，checksum 一致
- [ ] W2 ✅ `sdk.Jobs().Register` API 可用；admin "Run now" 按钮触发 plugin 执行；leader-only daily-rollup 在多副本部署只跑一次
- [ ] W3 ✅ admin `/admin/settings` 页出现 "Channel Monitor" 和 "Available Channels" 两个 tab，表单从 plugin JSON Schema 自动渲染；保存后 plugin 收到 Watch 事件
- [ ] W4 ✅ plugin 调 `http://169.254.169.254/` / `http://10.0.0.1/` / 单测覆盖 DNS rebinding case 全部返回 `ErrBlockedTarget`
- [ ] W5 ✅ plugin A 加密的密文，让 plugin B 调 Decrypt 必须失败（per-plugin HKDF 隔离生效）；host 启动时 masterKey 不合法直接 fail-fast
- [ ] W6 ✅ Channel Monitor 4 张表（monitor / history / daily_rollup / template）通过 W1 创建；UI 配置 monitor 后能看到检测明细
- [ ] W7 ✅ 管理页 `/admin/monitor` + 用户页 `/channel-status` 全部可见，i18n 中英切换正常，feature flag 关闭时菜单隐藏
- [ ] W8 ✅ Available Channels 用户接口 `GET /api/v1/channels/available` 正确返回（依赖 SQLProxy 直查 host channel/group/pricing 表）
- [ ] W9 ✅ 用户页 `/available-channels` 卡片网格能看到聚合数据，价格 + billing mode 颜色显示正确
- [ ] W10 ✅ 在 beta 环境（`/root/sub2api-beta`）部署后端 + 前端，端到端验证 Monitor 调度跑通 + Available Channels 列表正常 + admin settings 编辑生效

---

## 6. Out of Scope（V5 明确不做）

| 不做项 | 缺什么 / 风险 | 留给 V6+ 的契机 |
|--------|---------------|-----------------|
| **TimeSeriesStoreCapability**（C） | Channel Monitor 自管 history + daily_rollup 表，2 个以上类似 plugin 出现时再抽象 | 第 3 个 metrics-like plugin 出现时启动 |
| **PluginCacheBridge**（E） | plugin 直接用 SDK Redis Get/Set，性能可接受 | 出现 hot path cache miss 优化需求时 |
| **RealtimePushCapability**（H） | 5s 轮询；做 alerting 类 plugin 时再做 | 任何"事件触发 → 推前端"需求 |
| **PaginatedListHelpers**（I） | plugin 自己手写 page/size 解析；可选 V6 SDK 内 utility 包 | 不阻塞，随时加 |
| **AuthSubjectCapability**（J）扩展 group_ids | Available Channels 暂时通过 SQLProxy 自查 user → group | 第二个需要 group 交集的 plugin 出现时 |
| **DashboardWidgetSlot**（K） | plugin 用独立页面，不挂主 dashboard | 当 admin 抱怨"今日异常数要去 monitor 子页才能看"时 |
| **AuditLogCapability**（L） | host audit log 暂不开放给 plugin；plugin 自己 zap | 合规审计驱动 |
| **NotificationCapability**（M） | 跨 plugin alert 暂不做 | alert evaluator 重构时 |
| **FileBlobCapability**（N） | manifest IconPath 已够用 | 出现用户上传场景 |
| **HostServiceProxyCapability**（V5-INSPECT §6 隐含第 15 项） | Available Channels 后端只能走 SQLProxy 自己重写 mapping ∪ pricing 并联 + wildcard 展开（170 行） | **关键风险**：Designer 需评估 W8 重写成本是否可接受；不可接受则提升 HostServiceProxyCapability 到 V5.5 |
| 加密密钥轮换（W5 的演进） | 切 master key 后旧密文不可解 | 真发生 key rotation 需求时 |
| 自动 retry / 死信队列 / job priority（W2 演进） | handler 错就错 | 真出现 job 经常失败要重试时 |

---

## 7. 给 Designer 留的待决问题（≤3 个）

> Curator 已尽量拍板。以下 3 项保留给 Designer 是因为它们涉及具体接口形状的多种选择，且会在 Implementer 阶段产生大量 churn。

1. **W8 走 SQLProxy 还是等 V5.5 加 HostServiceProxyCapability？**
   - 走 SQLProxy = plugin 重写 170 行 mapping ∪ pricing 业务逻辑（风险：与 host 后续 channel mapping 演进容易脱钩）
   - 加 V5.5 = 把 V5 拖大一项；需要权衡时间盒
   - 建议：Designer 在 V5-DESIGN 里给出明确选择 + ETA

2. **W3 的 admin 表单渲染前端选 react-jsonschema-form 还是 vue-json-schema-form？**
   - 我们前端是 Vue 3，`@koumoul/vue-json-schema-form` 或 `@kpaxqin/json-schema-form-vue` 是候选
   - 与 Backstage（React）参考实现不一致，但语义可平移
   - Designer 需在 V5-DESIGN 里 PoC 一个最小可用 demo 后定

3. **W2 的 cron 库选 `robfig/cron/v3` 还是 `go-co-op/gocron`？**
   - 前者是 host 现有 leader_lock 机制最常配套的（轻量、无外部依赖）
   - 后者支持更友好的 fluent API + 内置分布式 lock
   - 建议 robfig（与现有 host code 风格一致），Designer 确认或翻案

---

## 附录 A：命名 plugin-agnostic 自检

对每个能力名 / proto service 名 / SDK 函数名做 grep：

```bash
grep -E "channel|Monitor|Available|Subscription|Order" \
  docs/plugin-architecture/V5-CURATE.md \
  | grep -v "^|" | grep -v "^>" | grep -v "（" | grep -v "Channel Monitor / Available"
# 期望：API 名 / proto 名命中 = 0；只在迁移示范段提到
```

5 个能力名最终敲定（无 channel 关键字）：

1. `MigrationRunnerCapability`
2. `JobSchedulerCapability`
3. `SettingsExtensionCapability`
4. `SafeOutboundHTTPCapability`
5. `SecretEncryptionCapability`

---

## 附录 B：业界参考索引

| 能力 | 主参考 | 次参考 |
|------|--------|--------|
| MigrationRunner | [pg-safe-migrate](https://dev.to/defnotwig/pg-safe-migrate-stop-shipping-unsafe-postgres-migrations-1m5n) | [Atlas Cloud](https://atlasgo.io) + [Backstage configSchema](https://backstage.io/docs/conf/defining/) |
| JobScheduler | [riverqueue/river](https://github.com/riverqueue/river) | [hibiken/asynq](https://github.com/hibiken/asynq) |
| SettingsExtension | [Backstage configSchema](https://backstage.io/docs/conf/defining/) | WordPress Settings API |
| SafeOutboundHTTP | [doyensec/safeurl](https://github.com/doyensec/safeurl) | [daenney/ssrf](https://github.com/daenney/ssrf) |
| SecretEncryption | [Vault Transit](https://developer.hashicorp.com/vault/docs/secrets/transit) | [AWS Hierarchical Keyring + HKDF](https://docs.aws.amazon.com/database-encryption-sdk/latest/devguide/use-hierarchical-keyring.html) |
