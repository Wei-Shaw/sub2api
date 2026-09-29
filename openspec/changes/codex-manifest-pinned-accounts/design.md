## Context

动机见 proposal.md。与方案相关的现状：

- Codex Model Manifest 入口是 `OpenAIGatewayHandler.CodexModels`：先尝试用账号模型映射本地生成；否则循环 `SelectAccountForModelWithExclusions` 选账号、`FetchCodexModelsManifest` 拉取、`CompleteAPIKeyCodexModelsManifestForClient` 补全、`MergeGroupConfiguredCodexModels` 做分组过滤与 ETag。
- `FetchCodexModelsManifest` 对 API Key 账号走 `fetchCachedAPIKeyCodexModelsManifest`（30 秒新鲜、5 分钟乐观、单飞后台刷新），对 OAuth 账号直接请求上游且带 agent identity 任务恢复逻辑，不缓存。
- 分组配置落地链路：ent schema → SQL 迁移 → `service.Group` → `group_repo` 创建与更新 setter → `api_key_repo` 分组字段投影与 `groupEntityToService` → 认证快照 `api_key_auth_cache.go` 结构体及 impl 两处映射 → 管理端 DTO / mapper → `admin_group.go` 创建与更新 → 分组复制。`models_list_config` 是完整样板。
- 前端 `GroupsView.vue` 已有 7000 行；模型路由的账号选择用「标签 + 搜索输入 + 下拉」内联实现，`Select.vue` 仅支持单选。`components/admin/group/` 下已有抽出的表单片段组件（如 `ReasoningEffortPolicyFields.vue`）。
- 后端管理端账号列表接口支持 `platform`、`group`、`search` 过滤。

## Goals / Non-Goals

**Goals:**
- 固定账号模式的运行时逻辑放在 service 层，handler 只做分支与错误映射。
- 合并逻辑是纯函数，可独立单测。
- 缓存策略只有一套实现，OAuth 与 API Key 账号共用；固定账号模式不引入第二层分组级缓存。
- 新字段在所有分组加载路径上都可见，特别是认证快照与 API Key 投影。

**Non-Goals:**
- 不改动调度器逻辑与普通请求的账号选择。
- 不为账号解绑或删除增加对该配置的级联清理，运行时容忍失效 ID。
- 固定账号关闭时保留本地生成 manifest 的路径；开启时以指定账号上游发现为先，映射在发现后应用。
- 创建分组对话框不提供该配置。

## Decisions

### D1：配置以单个 JSONB 列存储
`groups.codex_models_manifest_config`，领域类型 `domain.GroupCodexModelsManifestConfig{Enabled bool; AccountIDs []int64; FallbackToScheduler bool}`，JSON 键为 `enabled`、`account_ids`、`fallback_to_scheduler`。
- 备选：三个独立列。否决：三个字段语义耦合，JSON 保证原子写入，且与 `models_list_config` 一致。
- 迁移文件 `234_group_codex_models_manifest_config.sql`：`ADD COLUMN IF NOT EXISTS ... JSONB NOT NULL DEFAULT '{}'::jsonb`。ent schema 使用 `field.JSON(...).Default(domain.GroupCodexModelsManifestConfig{})`。

### D2：校验放在 admin service，创建路径拒绝开启
- 新文件 `group_codex_models_manifest.go` 提供 `normalizeCodexModelsManifestConfig`（平台非 openai 归零；去重保序；`enabled=false` 时保留列表便于再次启用）与 `validateCodexModelsManifestConfig(ctx, groupID, cfg)`（`enabled=true` 时：非空、≤10、全部属于 `accountRepo.ListByGroup(groupID)` 且平台 openai）。
- 创建路径：`enabled=true` 直接 400。原因：账号绑定发生在分组创建之后，创建时无法校验成员关系；前端也不展示。
- 备选：校验放 handler。否决：需要访问账号仓储，且创建与更新两条路径共用，service 是唯一收口。

### D3：运行时入口与回退策略
- handler 在本地生成分支之前判断 `group.Platform == openai && cfg.Enabled`，调用 `gatewayService.FetchPinnedCodexModelsManifest(ctx, group, clientVersion)`。
- 返回值：`(*OpenAIModelsResponse, *Account, error)`，`*Account` 为配置顺序中第一个成功账号，用于 `setOpsSelectedAccount`。
- 「无可用账号」以哨兵错误 `ErrNoPinnedCodexModelsAccounts` 表示；「全部失败」返回最后一个上游错误。handler 根据 `FallbackToScheduler` 决定：true 时跌入现有调度器循环；false 时无可用账号返回 503、全部失败按 `infraerrors.Code(err)` 返回。
- 可用性判定：`IsActive() && Schedulable && !(AutoPauseOnExpired && 已过期)`。刻意不使用 `IsSchedulable()`，因为它包含限流、过载与临时不可调度窗口。
- 账号来源：`accountRepo.ListByGroup(group.ID)`（active 成员），按配置顺序筛选并跳过缺失 ID。
- 备选：复用 `BuildGroupConfiguredCodexModelsManifest` 已加载的账号列表以省一次查询。否决：那两个列表是可调度集合，会漏掉限流中的账号；一次按分组的查询成本可接受。

### D4：并发拉取与纯函数合并
- 用 `sync.WaitGroup` 对每个可用账号并发执行：`FetchCodexModelsManifest(ctx, acc, clientVersion, "")` → API Key 账号再 `CompleteAPIKeyCodexModelsManifestForClient`。结果写入按配置顺序索引的切片，失败记录到同下标的错误切片；各账号独立完成，不因单账号失败取消其他请求。
- Codex 合并函数 `mergeCodexModelsManifestBodies(bodies [][]byte) ([]byte, error)`：以第一个 body 的顶层信封为基底，`models` 按 slug 并集，先出现者优先；slug 为空或解析失败的条目按出现顺序保留一次。输出后设置 `ETag = codexModelsManifestBodyETag(body)`，再交给 `MergeGroupConfiguredCodexModels` 做分组过滤与 304 判断。
- 部分失败：`slog.Warn` 带 group_id 与失败账号 ID 列表。
- 实现放在新文件 `openai_codex_models_pinned.go`，避免继续膨胀 2300 行的主文件。

### D5：缓存统一并调整时效
- 将 `fetchCachedAPIKeyCodexModelsManifest` 泛化为 `fetchCachedOpenAIModels(ctx, request, fetch, ifNoneMatch)`，`fetch func(ctx, ifNoneMatch) (*CodexModelsManifest, error)` 由调用方提供：API Key 路径传 `fetchCodexModelsManifestUpstream`；OAuth 路径传一个包含 agent identity 任务恢复逻辑的闭包。`handleCodexModelsManifestAccountAuthError` 在 OAuth 闭包返回错误时照旧调用。
- 常量：`openAIModelsCacheTTL` 30s → 60s；`openAIModelsCacheStaleTTL` 保持 5 分钟；超过 5 分钟 `get` 删除条目返回 miss，调用方同步等待单飞结果，这与「超期强制等待上游刷新」一致，无需新状态。
- 缓存键已包含 Authorization 与 Version 头，令牌刷新自然失效旧条目；`client_version` 不同的客户端各占一个条目。
- 缓存键不含分组信息，多个分组共用同一账号时共享同一条目，新鲜期内零上游请求，乐观期与超期刷新均按键单飞，同一时刻一个账号只发一次上游请求；分组级处理（并集合并、自定义列表过滤、别名合并）都在缓存体的克隆上进行，不写回缓存。
- `openAIModelsCacheMaxEntries` 64 → 512。OAuth 路径接入后条目数为「账号数 × 客户端版本数」，64 条在大规模部署下会被淘汰导致额外上游请求；单条清单通常几十 KB，最坏情况内存占用为几十 MB 量级。
- 后台刷新使用 `context.Background()` 加上游 15 秒超时，与现状一致。
- 备选：为固定账号模式增加分组级合并结果缓存。否决：账号级缓存命中后合并只是内存操作，再加一层会引入两套时效与失效问题。

### D6：前端拆出独立组件
- 新组件 `components/admin/group/CodexManifestAccountsField.vue`：props 为 `groupId`、`modelValue`（`{enabled, account_ids, fallback_to_scheduler}`）与已选账号名称映射；内部实现开关、标签列表、带防抖的搜索输入与下拉、回退子开关。搜索调用 `adminAPI.accounts.list(1, 20, {search, platform: 'openai', group: String(groupId)})`。
- `GroupsView.vue` 只在编辑对话框 OpenAI 区块挂载组件，打开编辑时用 `adminAPI.accounts.getById` 解析已存 ID 的名称，失败则显示 `#<id>`。提交时开关打开且列表为空则 toast 报错并阻止。
- 备选：在 GroupsView 内联复制模型路由的搜索状态。否决：会再引入一套按 key 索引的搜索状态，文件已过大。

## Risks / Trade-offs

- [固定账号模式下每次缓存超期会向 N 个账号并发请求] → 账号数上限 10；账号级缓存 1 分钟新鲜期把稳态请求量压到低于现状。
- [限流中的账号被强制用于拉取 manifest 可能加重其 429] → 这是需求明确要求的行为；manifest 请求不占并发槽，且被缓存吸收；失败时按部分合并处理。
- [部分账号失败导致模型列表短暂缺项] → 记录警告日志；下一次超期刷新恢复。用户已确认接受。
- [OAuth manifest 首次接入缓存，乐观期内令牌已被撤销仍会返回旧内容最多 4 分钟] → manifest 非敏感数据，且撤销后的下一次刷新会失败并按现有 401 处理流程处理账号。
- [配置引用的账号被解绑或删除后成为脏 ID] → 运行时跳过；编辑对话框显示 `#<id>` 提示管理员清理；全部失效时按回退配置处理。
- [认证快照漏投影新字段会让开关静默失效] → 仓库已有投影对账集成测试，任务中包含更新该测试。

## Migration Plan

1. 部署包含迁移 234 的后端版本；迁移幂等，默认值 `{}` 反序列化为关闭状态，存量分组行为不变。
2. 缓存时效变化随部署即时生效，无需数据迁移。
3. 回滚：回退代码即可，列保留无副作用；如需彻底清理可手工 `DROP COLUMN`。

### D7：普通模型列表共用固定账号来源

- `GatewayHandler.Models` 在 OpenAI 分组开启固定账号时调用 `FetchPinnedOpenAIModelsList`；普通 `/models` 别名共用该入口。
- 共享原有配置、固定账号成员筛选、并发收集与部分失败规则；标准列表失败且允许回退时用现有 OpenAI 调度器选账，再调用普通目录获取。无账号为 503，全部上游失败保留上游错误语义，不以静态默认列表掩盖失败。
- API Key 使用标准模型 URL，不添加 Codex `client_version` 或身份请求头；从管理端抽出 API Key 请求构造。OAuth 复用 `FetchCodexModelsManifest` 和规范客户端版本，保留凭据引用、Agent Identity 恢复、401 状态处理。
- 响应/缓存元数据类型泛化为 `OpenAIModelsResponse`；抽出原始 HTTP 响应获取，Codex 专用转换/元数据补全仅在 manifest 路径执行。两种请求使用同一缓存实现，以响应格式、完整 URL、账号、凭据、代理和请求头区分条目；相同 OAuth 请求共享条目。
- 标准列表保留上游 `id`、`created`、`owned_by` 等字段。OAuth slug 转成标准条目；无创建时间采用 0。有效空数组是成功结果，缺失或错误的数组结构是上游错误。

### D8：固定账号目录的映射与过滤

- 每个成功账号先基于它的原始目录做公开名称投影，随后按配置顺序取并集。透传或无映射账号保留上游具体 ID；有映射账号仅暴露映射目标存在于该账号目录的具体公开名称。
- 具体映射键以及分组选择的具体名称可作为别名候选；通配映射可匹配上游/分组提供的具体候选，但通配规则本身不作为模型 ID 返回。复用 `ResolveMappedModel` 的匹配优先级。
- Codex 别名继承对应上游条目的能力元数据；固定账号开启时不再注入其他分组账号的本地别名。固定账号回退选出的账号也应用相同投影。
- 分组列表过滤最后执行，遵守配置顺序；过滤后为空返回 200 空数组，不使用静态默认模型。最终响应体计算 ETag 并处理 304。
- 前端文案说明该配置覆盖普通模型列表和 Codex Model Manifest。持久化配置名称和结构不变。

### D9：扩展验证

新增路由/handler 与 service 测试覆盖普通两条路由、固定账号优先、原始媒体和未知模型、映射与通配别名、部分/全部失败及调度器回退、凭据/请求协议隔离、OAuth 缓存复用、三段缓存时效、分组过滤隔离与空目录、最终 ETag。运行受影响 Go 测试（含 race）、完整 unit 检查及前端类型/组件检查。
