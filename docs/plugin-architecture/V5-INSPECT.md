# V5 Inspector — 通用集成能力调研

> 作用：把 Channel Monitor / Available Channels 当成"驱动需求样本"反推 SDK / core 还缺什么**通用能力**；产出**插件无关**的能力清单，供 V5 Curator 决策。
> 范围：只调研、不写代码；命名禁止绑定渠道概念。
> base：`bd04bfa7 Merge bugfix-batch-1: importmap order + sidebar dual-highlight`（`feat/plugin-system-fixes`）
> 子分支：`feat/plugin-system-fixes--v5-inspect`

---

## 1. 驱动需求：Channel Monitor + Available Channels 代码盘点

下表所有路径在 commit `09fd83ab fix(monitor): clean up unused updatedAt/updatedLabel after label removal` 上仍存在；当前 `bd04bfa7` 上已不存在（被 V1 host 副本删除清理裹挟掉）。所有"最后存在 commit"统一为 `09fd83ab`（除非另注）。

### 1.1 Channel Monitor（管理员配置 + 后台调度 + 用户只读）

| 文件 | 行数 | 用途 |
|---|---:|---|
| `backend/ent/schema/channel_monitor.go` | 81 | ent schema：encrypted api key、provider、interval、template_id |
| `backend/ent/schema/channel_monitor_history.go` | 64 | 检测明细（每模型每次） |
| `backend/ent/schema/channel_monitor_daily_rollup.go` | ~50 | 历史聚合（7d 可用率） |
| `backend/ent/schema/channel_monitor_request_template.go` | ~50 | 请求模板（headers/body 复用） |
| `backend/ent/channelmonitor*` （生成代码） | 数千 | ent generated |
| `backend/internal/repository/channel_monitor_repo.go` | 450 | CRUD + 批量聚合 SQL（消 N+1） |
| `backend/internal/repository/channel_monitor_template_repo.go` | ~120 | 模板 repo |
| `backend/internal/service/channel_monitor_service.go` | 539 | 主服务 + Encrypt API key + RunDailyMaintenance |
| `backend/internal/service/channel_monitor_runner.go` | 291 | 调度器：每 monitor 一个 ticker + pond worker pool(5) + inFlight |
| `backend/internal/service/channel_monitor_checker.go` | 443 | HTTP 检测 + SSRF safe transport + sanitize |
| `backend/internal/service/channel_monitor_aggregator.go` | 292 | 视图聚合（批量 latest/availability/timeline） |
| `backend/internal/service/channel_monitor_ssrf.go` | 152 | SSRF 防护：私网/loopback/云元数据 IP 拒绝 + DialContext |
| `backend/internal/service/channel_monitor_validate.go` | 99 | 端点 https-only、IP 校验 |
| `backend/internal/service/channel_monitor_challenge.go` | 80 | 随机 challenge 校验上游真返回了 LLM 内容 |
| `backend/internal/service/channel_monitor_const.go` | 137 | 超时常量 / 阈值 |
| `backend/internal/service/channel_monitor_types.go` | 161 | service 层 model 结构 |
| `backend/internal/service/channel_monitor_template_*.go` | ~250 | 模板服务 |
| `backend/internal/handler/admin/channel_monitor_handler.go` | ~400 | admin CRUD + Run + History |
| `backend/internal/handler/admin/channel_monitor_template_handler.go` | ~250 | 模板 admin |
| `backend/internal/handler/channel_monitor_user_handler.go` | 127 | 用户只读 List + GetStatus |
| `backend/internal/handler/dto/channel_monitor.go` | 10 | 共享 DTO |
| `backend/migrations/125–128_*.sql` | 4 个文件 | DDL：表、聚合、模板、watermark |
| `frontend/src/views/admin/ChannelMonitorView.vue` | 304 | 管理页 |
| `frontend/src/views/user/ChannelStatusView.vue` | 172 | 用户卡片网格页 |
| `frontend/src/components/admin/monitor/*.vue` | 9 个 | 表单/筛选/操作/模板 dialog |
| `frontend/src/components/user/monitor/*.vue` | 7 个 | Card/Hero/Timeline/Provider 图标 |
| `frontend/src/components/user/MonitorDetailDialog.vue` | 114 | 详情弹窗 |
| `frontend/src/api/admin/channelMonitor.ts` | 190 | admin API client |
| `frontend/src/api/admin/channelMonitorTemplate.ts` | ~120 | 模板 API client |
| `frontend/src/api/channelMonitor.ts` | 74 | 用户 API client |
| `frontend/src/composables/useChannelMonitorFormat.ts` | 97 | 共享格式化 |
| `frontend/src/constants/channelMonitor.ts` | 35 | 状态常量 |
| `frontend/src/i18n/locales/{en,zh}.ts` | ~120 行/语言 | `admin.monitor.*` / `monitorCommon.*` 命名空间 |

合计：**后端 ~50 文件 / ~4000 业务行（不含 ent 生成）**；**前端 ~25 文件 / ~2500 行**；**migrations 4 份**。

### 1.2 Available Channels（用户只读聚合视图）

| 文件 | 行数 | 用途 |
|---|---:|---|
| `backend/internal/service/channel.go` `(*Channel).SupportedModels` | ~170 | mapping ∪ pricing 并联，wildcard 展开 |
| `backend/internal/service/channel_available.go` | 84 | `ChannelService.ListAvailable` 聚合渠道+活跃组+合成定价 |
| `backend/internal/service/channel_available_test.go` | 119 | 单测 |
| `backend/internal/handler/available_channel_handler.go` | 216 | 用户三层过滤（status / group 交集 / 平台过滤）+ 字段白名单 + feature flag |
| `backend/internal/handler/available_channel_handler_test.go` | 121 | 单测 |
| `backend/internal/handler/admin/available_channel_handler.go` | 99（**已于 commit `88e1dd6e` 主动删除**，仅留参考） | admin DTO，与用户视图等价但额外暴露 BillingModelSource |
| `backend/internal/server/routes/user.go` | 6 行注册 | `GET /api/v1/channels/available` |
| `frontend/src/views/user/AvailableChannelsView.vue` | 127 | 用户页 |
| `frontend/src/components/channels/AvailableChannelsTable.vue` | 110 | 表格 |
| `frontend/src/components/channels/PricingRow.vue` | 29 | 价格行 |
| `frontend/src/components/channels/SupportedModelChip.vue` | 214 | 模型 chip + 计费 mode 颜色 |
| `frontend/src/api/channels.ts` | 60 | 用户 API client |
| `frontend/src/constants/channel.ts` | 22 | billing-mode 常量 |
| `frontend/src/i18n/locales/{en,zh}.ts` | ~75 行/语言 | `availableChannels.*` |

合计：**后端 ~10 文件 / ~900 行**；**前端 ~7 文件 / ~600 行**。

**两功能跨 commits 数**：grep `ChannelMonitor`/`AvailableChannels` 在 git 历史命中 **28 + 13 = 41** 个 commit，其中 ~6 个为后续迭代（feature flag、daily rollup、template 子集 picker、card grid 重设计、平台爆破等）。

---

## 2. host 能力使用模式分析（10 维度）

| # | 维度 | Channel Monitor 用 | Available Channels 用 | host 能力位置 |
|---|---|---|---|---|
| 1 | DB schema / ent | 自定义 4 张表 + ent schema；外键到 host 几乎无（隔离良好） | 复用 host 的 `Channel` / `Group` / `ModelPricing` ent | host 的 ent 客户端通过 `dbent.Client` 全局注入 |
| 2 | DB query 复杂度 | 原生 SQL：`GROUP BY` + 时间窗 availability + `ON CONFLICT` upsert daily rollup | 多 table join + 跨 platform 过滤 + wildcard 前缀展开 | service 层手写 SQL，repo 走 `database/sql` 而非 ent |
| 3 | Redis | 无直接使用（间接：leader lock 走 Redis） | 无 | `OpsCleanupService.acquireLeaderLock` |
| 4 | Background scheduler | `ChannelMonitorRunner`：自管 goroutine + `time.Ticker(per-monitor)` + `pond.Pool(5)` + `inFlight` 防重；daily maintenance 由 `OpsCleanupService` cron 触发，**复用 host 的 leader lock + heartbeat** | 无 | host 没有"插件可注册定时任务"的接口，runner 在 wire 里硬编码 |
| 5 | Real-time push | 无（前端 5s 轮询） | 无（一次拉取） | host 无 SSE/WS 通用通道 |
| 6 | Metrics | 无（latency 写到 history 行而非 prometheus） | 无 | V4-INSPECT 已记：host /metrics = 0 hit |
| 7 | User context (handler 入参) | admin/user 两套 handler 直接读 `middleware.GetAPIKeyFromContext` / `middleware.GetUserFromContext`，但 service 层接受 `ctx`+ ID 入参（无强耦合） | 用户 handler 用 `apiKeyService.ListUserAPIKeys` + group 交集计算可见性；强依赖 host 的 user/group 数据模型 | gin context + middleware 包，**未抽象**给 plugin |
| 8 | Authorization | 三档：admin（middleware）/ 已认证用户（API Key）/ feature flag（settingService） | 同上 + feature flag | 中间件硬绑路由组，plugin 路由当前走 X-Plugin-User-Role header（V4 起） |
| 9 | Pagination / filtering | List 入参 `Page/Size/Provider/Enabled/Search`，repo 自己拼 SQL；admin handler 重复 `parseInt64Param`/`bindJSONOrError`（V3 抽过 helper，但未对 plugin 暴露） | 无分页（一次性） | helper 在 `handler/admin` 包，plugin 拿不到 |
| 10 | Timeseries 存储 | `channel_monitor_history` 是事实上的时间序列：每模型每次写一行，跨 monitor/model/time 维度查询；`channel_monitor_daily_rollup` 是物化降采样 | 无 | host 没有"插件可声明 timeseries 表 + 自动滚动 retention"的能力 |
| 额外 | Cache 通用接口 | 无 cache（Redis 也没用） | 无（每次查 ListAll + ActiveGroups） | 现有 SDK Redis API 是 raw key 级别，没有 "GetOrLoad" / "TTL cache" 抽象 |
| 额外 | Secret / Encryption | `SecretEncryptor` 接口，AES-256-GCM 加密 api_key | 无 | host 在 `service.SecretEncryptor` 注入；**SDK 未透出**：plugin 想存敏感信息只能自己实现 AES |
| 额外 | SSRF / 安全 dial | 自管 `safeDialContext` + 私网 IP 黑名单 + DNS rebinding 防御 | 不需要（不外发请求） | host 通用 HTTP 工具未提供 SSRF-safe client；下一个调外部 LLM 的 plugin 都要重写 |
| 额外 | Settings / feature flag | `settingService.GetChannelMonitorRuntime(ctx)` + `GetAvailableChannelsRuntime(ctx)`，写在 host 的 setting domain | 同左 | host SettingService 是 hardcode 字段集合；plugin 想加 feature flag 必须改 host 代码（V4 INSPECT §2.4 提到的 Global Config 缺口的下游表现） |
| 额外 | Background daily job | `RunDailyMaintenance` 由 `OpsCleanupService` cron + leader lock 调用 | 无 | host cron 是 service 包内硬编码注册，plugin 无法挂上去 |

---

## 3. 抽象通用集成能力清单（plugin-agnostic）

按照"任何未来 plugin（订阅监控 / 流量分析 / 用户告警 / 文件审计 / …）都可能需要"的视角列出。命名禁止绑定渠道。

| # | 能力 | 当前 SDK 是否提供 | 缺口（一句话） | 工作量 | 业界参考 |
|---|---|---|---|---|---|
| A | **MigrationRunnerCapability** — plugin 声明 SQL 文件清单 → host 在启动时读、上 advisory lock、写 schema_migrations | 部分（manifest 已有 `MigrationFiles` 字段，但 `manager.go` 只打日志，**未真正执行**） | 没有 RPC 让 plugin 把 SQL 内容传过来；host migration runner 只跑 host embed.FS | 中 | etcd `migrate` 库的 versioned migrations / Atlas Cloud 的 plugin-driven migrations |
| B | **JobSchedulerCapability** — plugin 注册"每 N 秒触发 Handler / cron / leader-locked once-per-day"，host 提供 ticker / leader lock / 优雅关停 | 否 | runner 现在每个 plugin 自己起 goroutine + ticker；leader lock 是 host service 私有；Channel Monitor 这种 per-record-ticker 模型完全无法复用 | 中 | River（Go）/ Asynq / GoCron — 都是 host-side scheduler + worker registration，对应 plugin world 的就是 gRPC `RegisterJob(spec) returns stream Trigger` |
| C | **TimeSeriesStoreCapability** — plugin 声明 schema，host 提供 append / range query / retention / downsample | 否 | history 表 + daily rollup + watermark + 软删 = plugin 自己造一遍轮子；下一个想存 metrics 的 plugin 还得再造 | 大 | TimescaleDB hypertables / VictoriaMetrics PromQL via plugin / Grafana datasource plugin SDK |
| D | **SettingsExtensionCapability** — plugin 声明 setting key 集合（含 schema / 默认值 / feature flag），host SettingService 自动暴露到 admin "系统设置"页 + `Get/Set` API + 缓存 | 否（V4 INSPECT §2.4 已记） | 增加任何 feature flag 都要改 host `setting_service.go` + `settings_view.go` + dto + 前端 SettingsView，**非加性** | 中 | Backstage `app-config-schema` / Sentry plugin config schema / WordPress plugin Settings API |
| E | **PluginCacheBridge** — host 暴露 `GetOrLoad(key, ttl, loader func() any)` + 标签失效；底层走 host Redis 但语义是 cache | 部分（Redis Get/Set 已有，但是 raw kv，无 GetOrLoad / lock-on-load / 标签失效） | plugin 想 cache "active groups list"，得自己写 SetEx + 反序列化 + 失效逻辑 | 小 | groupcache / ristretto + Sentry's `request_cache` |
| F | **SafeOutboundHTTPCapability** — plugin 调外部 API 时，host 提供 SSRF-safe http.Client（私网拒绝 + DNS rebinding 防御 + 可选代理 + 超时） | 否 | Channel Monitor 自己实现 152 行 SSRF 黑名单；下一个 plugin（如 webhook 验证、外部 OAuth callback、LiteLLM 同步）都得自己写 | 小-中 | Stripe `forward-rules` / GitHub Actions self-hosted runner SSRF guard / hashicorp/go-cleanhttp |
| G | **SecretEncryptionCapability** — plugin 想存"用户提供的 api_key"，host 暴露 `Encrypt/Decrypt(plaintext)`（AES-256-GCM，密钥 host 持有） | 否 | Channel Monitor 通过依赖注入的 `service.SecretEncryptor` 工作，但 plugin 进程**拿不到** SecretEncryptor（不在 SDK 里） | 小 | Vault Transit secrets engine / AWS KMS Encrypt API |
| H | **RealtimePushCapability** — plugin 想给 plugin frontend 推数据（如监控状态变化、订阅事件），host 提供 SSE / WS 通道 | 否（EventBus 是 plugin↔plugin / plugin↔host backend，不到 frontend） | 当前 frontend 都是 5s 轮询；做 alerting / live tail 类 plugin 完全堵死 | 大 | Phoenix Channels / Hasura subscription plugins / Postgres LISTEN/NOTIFY → SSE bridge |
| I | **PaginatedListHelpers** — plugin handler 解析 page/size/sort、构造响应、字段白名单 | 否 | admin/user handler 都重复手写；V3 抽出 `ParseInt64Param`/`BindJSONOrError` 在 host 包，plugin 用不到 | 小 | Buffalo `paginator` / strapi REST plugin params |
| J | **AuthSubjectCapability** — plugin handler 拿到结构化 subject（user_id, role, api_key_id, group_ids, scopes），不必自己解析 header | 部分（V4 给了 X-Plugin-User-ID / Role；缺 group_ids / api_key_id 的可见性 / scopes） | Available Channels 需要 "用户的 visible group ids" 计算交集；当前只能自己再调 admin API | 小 | OAuth2 Resource Server SDKs / Keycloak adapters |
| K | **DashboardWidgetSlot** — plugin 想在 host admin 主仪表盘 / 用户首页插入卡片 | 否 | Channel Monitor 当前是独立页面，无法把"今日异常数"挂到主 dashboard | 中 | Backstage `Dashboard` extension points / Grafana plugin panels / WordPress dashboard widgets |
| L | **AuditLogCapability** — plugin 操作（创建监控、删除模板）写入统一审计 | 否 | host 有自己的 audit log，但 plugin 用不到；运维想追"谁改了监控配置"只能 grep zap 日志 | 小-中 | OpenTelemetry log signal + audit semantic conventions |
| M | **NotificationCapability** — plugin 发邮件 / 站内信 / webhook / IM 推送 | 否 | host 有 alert evaluator service，plugin 接不上 | 中 | Sentry plugin `notify_users` / Grafana alerting routing |
| N | **FileBlobCapability** — plugin 上传 / 下载 二进制（icon、报告 PDF、CSV 导出） | 否 | manifest 有 `Frontend.IconPath` 但只是 plugin 自己的 file system；用户上传场景没有 | 中 | S3 presigned URLs / Mattermost FileInfo API |

---

## 4. V5 必做 5 项（Curator 输入）

排序依据：**(a) Channel Monitor / Available Channels 迁移是否阻塞** + **(b) ROI（影响所有未来 plugin）** + **(c) 工作量可控**。

1. **MigrationRunnerCapability**（能力 A） — *阻塞* Channel Monitor 迁移：4 张表的 DDL 不跑就什么都做不起来。manifest 已埋字段，工作量集中在 RPC + advisory lock + plugin namespace。**核心工作 ~300 行 + 1 RPC**。
2. **JobSchedulerCapability**（能力 B） — *阻塞* Channel Monitor：每 monitor 一个 ticker + leader-locked daily maintenance 是 plugin 没法自己造的（leader lock 在 host Redis 里）。Available Channels 不需要，但下一个"订阅过期检查""配额 reset"plugin 都需要。**接口设计是关键**：`RegisterJob(JobSpec) returns stream Trigger` + plugin handler 负责具体业务。
3. **SettingsExtensionCapability**（能力 D） — *阻塞* Available Channels 的 feature flag、Channel Monitor 的多个 toggle；当前任何 plugin 想给系统设置加 tab 都要改 host。属于"新增 plugin 数量增长后，每次都要改 host"的瓶颈点。
4. **SafeOutboundHTTPCapability**（能力 F） — *阻塞* Channel Monitor 检测器（SSRF 防护是非功能性必备，不能让 plugin 进程裸调外部）。下一个调 LiteLLM / OAuth callback / webhook 验证的 plugin 都需要。**150 行**实现 + 1 SDK helper。
5. **SecretEncryptionCapability**（能力 G） — *阻塞* Channel Monitor（存 api_key），且**安全敏感**：plugin 各自实现 AES 是审计灾难。host 已有 `SecretEncryptor`，只需通过 gRPC 暴露 `Encrypt/Decrypt` RPC。**~100 行**。

> 五项里 1/2 是"基础设施 RPC 增加"，3 是"扩展点 + DB schema"，4/5 是"安全工具透出"。覆盖了"plugin 能不能跑（A、B）"、"plugin 能不能配（D）"、"plugin 能不能安全调外部 + 存敏感（F、G）"。

---

## 5. V6+ 后续议题（本轮不做）

- **TimeSeriesStoreCapability**（C） — 大；可让 Channel Monitor 先用 MigrationRunner + 自管 history 表跑起来，有 2-3 个 plugin 都需要时再抽象
- **PluginCacheBridge**（E） — 小但收益单点；plugin 自己用 Redis Get/Set 也能撑住
- **RealtimePushCapability**（H） — 大；轮询 5s 不影响功能可用性
- **PaginatedListHelpers**（I） — 小；可作为 SDK 内 utility 包随时加
- **AuthSubjectCapability**（J） — 部分能力 V4 已做（user_id/role）；扩展到 group_ids 留给后续 PR
- **DashboardWidgetSlot**（K） — 中；plugin 各有独立页面已可用
- **AuditLogCapability**（L） — 中；host 自己 audit log 与 plugin 解耦更好
- **NotificationCapability**（M） — 中；alert evaluator 跨 plugin 是大改造
- **FileBlobCapability**（N） — 中；目前没有 plugin 真实需求

---

## 6. Channel Monitor + Available Channels 迁移工作量（依赖关系）

| 功能 | 阻塞它的 V5 能力 | 不阻塞但需要的 |
|---|---|---|
| **Channel Monitor 后端** | A（migration）、B（scheduler）、F（SSRF）、G（encryption）、D（feature flag） | 自管 timeseries 即可（不等 C），cache 可省（不等 E） |
| **Channel Monitor 前端** | 不阻塞（V4 已给 tailwind preset），但**需要 i18n 命名空间注册**（plugin SDK 已有 `sdk.i18n.registerNamespace`） | DashboardWidgetSlot 不必（独立页面够用） |
| **Available Channels 后端** | D（feature flag）；其余仅依赖 host 既有的 Channel/Group/ModelPricing service（plugin 通过 SDK 远程调用 host service 即可，**当前 SDK 无此能力**） | — |
| **Available Channels 前端** | 不阻塞 | — |

**关键发现**：Available Channels 后端有一个隐含依赖 — plugin 想读 host 的 `Channel.SupportedModels()` 和 `groupRepo.ListActive()`，但 SDK 当前**没有"调用 host service 方法"的 RPC**（只有 SQLProxy 直接读表）。如果走 SQLProxy 直查，plugin 要重新实现 mapping ∪ pricing 并联 + wildcard 展开（170 行业务逻辑）。

> 这暗示 V6+ 还有第 12 项隐含能力："**HostServiceProxyCapability** — plugin 通过 gRPC 调用 host 注册的 service 方法（业务逻辑复用，不只是 raw DB）"。本轮不做，但 Curator 应在 V5-CURATE 风险点里记一笔。

---

## 关键提问给 Curator

1. **JobSchedulerCapability 的语义边界**：plugin 的 job 在 plugin 进程跑还是在 host 进程跑？前者保留 plugin 隔离但 leader lock 跨进程困难；后者污染 host 内存但 leader lock 简单。建议**前者**（plugin 进程跑，host 只发 trigger 信号 + 持有 leader lock）。
2. **SettingsExtensionCapability 的 schema 格式**：用 JSON Schema 还是 protobuf descriptor？前端 admin 渲染表单需要 schema → 倾向 JSON Schema（前端生态成熟）。
3. **MigrationRunner 的失败策略**：plugin migration 失败要不要让 plugin 启动失败？建议**是**（与 host 一致），并支持"plugin disable 时跳过 migration"。
4. **SecretEncryptionCapability 的密钥分域**：所有 plugin 共用 host master key，还是 per-plugin derive？建议**per-plugin derive**（HKDF with plugin name as salt），避免某 plugin 解出别人的密文。
