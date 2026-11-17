## 1. 数据模型与持久化

- [x] 1.1 在 `backend/internal/domain` 新增 `GroupCodexModelsManifestConfig{Enabled, AccountIDs, FallbackToScheduler}`，JSON 键为 `enabled`、`account_ids`、`fallback_to_scheduler`
- [x] 1.2 在 `backend/ent/schema/group.go` 新增 JSONB 字段 `codex_models_manifest_config`（默认空结构体），执行 `go generate ./ent`
- [x] 1.3 新增迁移 `backend/migrations/234_group_codex_models_manifest_config.sql`（`ADD COLUMN IF NOT EXISTS ... JSONB NOT NULL DEFAULT '{}'::jsonb`）
- [x] 1.4 `service.Group` 新增 `CodexModelsManifestConfig` 字段并添加类型别名；`group_repo.go` 创建与更新两处 setter 写入该字段
- [x] 1.5 `api_key_repo.go`：分组字段投影列表与 `groupEntityToService` 加入新字段
- [x] 1.6 `api_key_auth_cache.go` 快照结构体与 `api_key_auth_cache_impl.go` 两处映射加入新字段
- [x] 1.7 `admin_group_duplicate.go` 复制分组时将该配置重置为关闭且账号列表为空
- [x] 1.8 更新 API Key 分组投影对账集成测试，覆盖新字段

## 2. 管理端配置接口与校验

- [x] 2.1 新增 `backend/internal/service/group_codex_models_manifest.go`：`normalizeCodexModelsManifestConfig`（非 openai 平台归零、去重保序）与 `validateCodexModelsManifestConfig`（开启时非空、≤10、全部为分组内 active 的 openai 账号），错误码 `INVALID_CODEX_MODELS_MANIFEST_CONFIG`
- [x] 2.2 `admin_service.go` 的 `CreateGroupInput`/`UpdateGroupInput` 与 `admin_group.go` 创建、更新路径接入归一化与校验；创建路径 `enabled=true` 返回 400
- [x] 2.3 `handler/admin/group_handler.go` 的创建与更新请求结构体、`dto/types.go` 响应结构体、`dto/mappers.go` 增加 `codex_models_manifest_config`
- [x] 2.4 在 `admin_service_group_test.go` 增加测试：开启但空列表被拒、非成员或非 openai 账号被拒、超过 10 个被拒、非 openai 平台被归零、重复 ID 去重保序、创建时开启被拒

## 3. 缓存策略统一

- [x] 3.1 将 `codexModelsManifestCacheTTL` 调整为 60 秒，保持 `codexModelsManifestCacheStaleTTL` 为 5 分钟，将 `codexModelsManifestCacheMaxEntries` 提高到 512，并更新常量注释说明三段时效与容量依据
- [x] 3.2 将 `fetchCachedAPIKeyCodexModelsManifest`/`refreshCachedAPIKeyCodexModelsManifest` 泛化为接受 `fetch` 闭包的 `fetchCachedCodexModelsManifest`/`refreshCachedCodexModelsManifest`
- [x] 3.3 `FetchCodexModelsManifest` 的 OAuth 分支改为经缓存调用，闭包内保留 agent identity 任务恢复逻辑，错误时仍调用 `handleCodexModelsManifestAccountAuthError`
- [x] 3.4 在 `openai_codex_models_service_test.go` 增加或调整测试：OAuth 账号新鲜期内零上游请求、乐观期返回旧值且单飞后台刷新、超期后同步等待并写回、超期上游失败返回错误、令牌变化后缓存未命中、If-None-Match 基于缓存 ETag 返回 304、同一账号被两个分组同时请求时只发一次上游请求且各分组过滤互不影响
- [x] 3.5 检查现有测试对 30 秒 TTL 或 OAuth 不缓存的隐含假设并修正

## 4. 固定账号模式运行时

- [x] 4.1 新增 `backend/internal/service/openai_codex_models_pinned.go`：`mergeCodexModelsManifestBodies` 纯函数（第一个信封为基底、`models` 按 slug 并集且先出现者优先）
- [x] 4.2 同文件实现 `FetchPinnedCodexModelsManifest(ctx, group, clientVersion)`：`ListByGroup` 取成员、按配置顺序筛选可用账号（active、Schedulable、未过期；忽略限流与过载）、errgroup 并发拉取与补全、按下标收集结果、部分失败记录 `slog.Warn`、全部不可用返回 `ErrNoPinnedCodexModelsAccounts`、全部失败返回最后一个错误、成功时设置合并体 ETag 并返回首个成功账号
- [x] 4.3 `openai_codex_models_handler.go`：本地生成分支之后加入固定账号分支，成功时 `setOpsSelectedAccount` 后调用 `MergeGroupConfiguredCodexModels` 并写响应；按 `FallbackToScheduler` 决定回退调度器循环或返回 503 / 上游错误
- [x] 4.4 为合并函数编写单元测试：并集与顺序、重复 slug 取靠前账号条目、信封字段来源、无 slug 条目处理
- [x] 4.5 在 `openai_codex_models_handler_test.go` 增加测试：两个账号返回不同模型时客户端收到并集且未调用调度器、限流账号仍被使用、停用账号被跳过、不在分组的 ID 被跳过、部分失败仍 200、全部不可用时默认 503、开启回退时走调度器、自定义模型列表过滤仍生效、ETag 匹配返回 304

## 5. 前端

- [x] 5.1 `frontend/src/types/index.ts` 新增 `CodexModelsManifestConfig` 类型，并加入 `AdminGroup`、创建与更新请求类型
- [x] 5.2 新增 `frontend/src/components/admin/group/CodexManifestAccountsField.vue`：开关、已选账号标签、带防抖的搜索下拉（`adminAPI.accounts.list` 按 `platform=openai` 与 `group` 过滤）、回退子开关
- [x] 5.3 `GroupsView.vue` 编辑对话框 OpenAI 区块挂载组件；打开编辑时解析已存账号名称（失败显示 `#<id>`）；提交时开关打开且列表为空则提示并阻止；更新请求携带该字段；创建请求固定发送关闭状态
- [x] 5.4 中英文 i18n（`overview.ts` 的 `admin.groups.codexModelsManifest.*`）：标题、开关文案、启用与未启用提示、账号标签、搜索占位、回退开关文案、至少选择一个账号的错误提示
- [x] 5.5 为新组件编写 Vitest 测试：开关切换显示、选择与移除账号、开启后空列表触发校验

## 6. 验证

- [x] 6.1 `cd backend && go test -tags=unit ./...` 与 `golangci-lint run ./...` 通过
- [x] 6.2 `cd backend && go test -tags=integration ./internal/repository/...` 通过（含投影对账测试）
- [x] 6.3 `cd frontend && pnpm test` 与类型检查通过
- [ ] 6.4 本地启动后手动验证：编辑 OpenAI 分组开启固定账号并选择两个权限不同的账号，Codex 客户端拉取 manifest 得到并集；1 分钟内重复请求不打上游

## 7. 普通模型列表扩展

- [x] 7.1 泛化模型响应/缓存基础，抽出原始上游传输和 API Key 标准请求构造，OAuth 复用现有认证与缓存
- [x] 7.2 固定账号筛选与并发获取共用，新增普通模型目录聚合及调度器回退
- [x] 7.3 实现来源账号映射投影、分组最终过滤、空目录与 ETag；Codex 固定账号优先于本地生成
- [x] 7.4 普通 `/v1/models` 与 `/models` 接入固定账号逻辑，中英文 UI 文案覆盖两个入口
- [x] 7.5 添加两种上游、映射优先级、失败回退、跨分组及跨协议缓存、三段时效和 ETag 回归测试
- [x] 7.6 完成受影响测试、race、unit/lint、前端类型与组件验证及 OpenSpec 校验，记录验证范围
