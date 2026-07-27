## Phase 0. 统一代理取值入口（纯重构，可独立合并）

本阶段零行为变化，不引入任何代理池概念，目的是把后续阶段的改造面从 58 处压缩到 1 处。**建议单独出 PR。**

- [x] 0.1 在 `backend/internal/service/account.go` 新增 `func (a *Account) ProxyURL() string`，守卫为 `a == nil || a.Proxy == nil` 返回空串
- [x] 0.2 复核不变量 `account.Proxy != nil ⟹ account.ProxyID != nil`：确认 `repository/account_repo.go:294`、`repository/account_repo.go:3170`、`service/grok_quota_service.go:583` 三处赋值点仍受 `ProxyID != nil` 保护
- [x] 0.3 为 `ProxyURL()` 增加单测，覆盖 nil 接收者、`Proxy == nil`、带认证代理、不带认证代理四种情形（见 `account_proxy_url_test.go`）
- [x] 0.4 替换 58 处调用点为 `account.ProxyURL()`，分布见下表；逐文件替换，不合并无关改动
- [x] 0.5 替换后确认无残留：`grep -rn "\.Proxy\.URL()" --include=*.go internal | grep -v _test.go` MUST 只剩 `account.go` 内的方法体
- [x] 0.6 `cd backend && go build ./...` 通过
- [x] 0.7 `cd backend && go test ./internal/service/ -count=1` 通过（`ok ... 95.321s`）；`TestVertexServiceAccountProxyURL` 同步改为锁定池前向语义
- [x] 0.8 人工复核 diff：主路径为取值写法替换；特殊守卫处（identity check / custom base URL / repo fallback）仅替换 `Proxy.URL()` 调用，保留原守卫逻辑

### Phase 0 调用点分布（58 处 / 43 文件）

| 文件 | 处数 |
| --- | --- |
| `service/account_test_service.go` | 10 |
| `service/gemini_messages_compat_service.go` | 3 |
| `service/openai_gateway_grok.go` | 2 |
| `service/grok_media.go` | 2 |
| `service/gateway_count_tokens.go` | 2 |
| `service/account_usage_service.go` | 2 |
| 其余 37 个文件 | 各 1 |

其余 37 个文件：`vertex_service_account.go`、`upstream_models.go`、`upstream_billing_probe.go`、`openai_ws_v2_passthrough_adapter.go`、`openai_ws_http_bridge.go`、`openai_ws_forwarder_v2.go`、`openai_ws_forwarder_ingress.go`、`openai_quota_service.go`、`openai_images_responses.go`、`openai_images.go`、`openai_gateway_passthrough.go`、`openai_gateway_messages.go`、`openai_gateway_grok_chat_bridge.go`、`openai_gateway_forward.go`、`openai_gateway_count_tokens.go`、`openai_gateway_chat_completions.go`、`openai_gateway_cc_pipeline.go`、`openai_embeddings.go`、`openai_codex_models_service.go`、`openai_apikey_responses_probe.go`、`openai_alpha_search.go`、`openai_agent_identity.go`、`ollama_cloud_usage.go`、`grok_quota_service.go`、`gemini_oauth_service.go`、`gemini_chat_completions_compat_service.go`、`gateway_websearch_emulation.go`、`gateway_upstream_request.go`、`gateway_forward.go`、`gateway_forward_as_responses.go`、`gateway_forward_as_chat_completions.go`、`gateway_bedrock.go`、`gateway_anthropic_passthrough.go`、`antigravity_gateway_upstream.go`、`antigravity_gateway_service.go`、`antigravity_gateway_gemini.go`、`antigravity_gateway_claude.go`

## Phase 1. 数据层

- [x] 1.1 新建 `backend/ent/schema/proxygroup.go`，Annotations 指定表名 `proxy_groups`，Mixin 照 `ent/schema/proxy.go:25-30` 使用 TimeMixin + SoftDeleteMixin
- [x] 1.2 定义字段：`name`（唯一、非空）、`strategy`（MaxLen 20，默认 `round_robin`）、`status`（默认 `active`）、`sticky_by_account`（bool，默认 false）、`description`（可选）
- [x] 1.3 在 `ent/schema/proxy.go` 增加 `group_id`（Optional + Nillable）及 `edge.From("group", ProxyGroup.Type).Ref("proxies").Field("group_id").Unique()`
- [x] 1.4 在 `ent/schema/account.go` 增加 `proxy_group_id`（Optional + Nillable）及索引
- [x] 1.5 新建迁移 `backend/migrations/191_add_proxy_groups.sql`，风格照 `149_proxy_expiry_fallback.sql`：全程 `IF NOT EXISTS`，索引命名 `{表}_{列}_idx`，外键 `ON DELETE SET NULL`；并加 name 部分唯一索引
- [x] 1.6 确认迁移与 ent schema 双写一致（本仓库两者不自动同步）
- [x] 1.7 `cd backend && make generate` 重生成 ent 客户端与 `wire_gen.go`（`proxygroup*.go` 已生成；`go build ./...` 通过）
- [x] 1.8 迁移幂等性测试：重复执行 MUST 不报错（待有集成测试环境时补）

## Phase 2. 选择器与注入

- [x] 2.1 新建 `backend/internal/service/proxy_group.go`：`ProxyGroup` 领域模型、策略常量、`ProxyGroupWithProxies`、`ErrProxyGroupNotFound`、`ErrProxyGroupInUse`
- [x] 2.2 在同文件定义 `ProxyGroupRepository` 接口（遵循 `proxy_service.go:17` 的依赖倒置约定：接口在 service 包，实现在 repository 包）
- [x] 2.3 新建 `backend/internal/service/proxy_selector.go`，实现无 I/O 纯函数 `SelectProxyFromGroup(candidates []Proxy, strategy string, now time.Time, seed uint64) (*Proxy, bool)`
- [x] 2.4 候选过滤复用 `service/proxy.go:33,38` 的 `IsActive()` / `IsExpired(now)`
- [x] 2.5 实现 `round_robin`（seed 为原子计数器）、`random`、`sticky`（seed 为 accountID 哈希）三策略；未知策略回退 `round_robin` 并告警
- [x] 2.6 选择器单测：空候选集、全部过期、全部停用、单候选、多候选轮询分布、sticky 恒定性、sticky 候选集变化后重映射、未知策略回退
- [x] 2.7 新建 `backend/internal/repository/proxy_group_repo.go`，照 `proxy_repo.go:20-28` 的 ent + 裸 SQL 双通道模式；裸 SQL MUST 手写 `deleted_at IS NULL`
- [x] 2.8 在 `service/proxy_service.go` 的 `ProxyRepository` 接口增加 `ListByGroupID` / `CountByGroupID`；oauth/gemini mock 已同步
- [x] 2.9 实现组成员缓存与失效：`DefaultProxyGroupResolver` 进程内 TTL 缓存 + `InvalidateGroup`；管理端变更时调用失效（Phase 4 接线）；outbox 广播留待多实例增强
- [x] 2.10 改造 `repository/account_repo.go` GetByIDs：`proxy_id` 优先；否则 `applyProxyGroupSelection` 填入 `out.Proxy`；**不写 `out.ProxyID`**
- [x] 2.11 同步改造 `accountsToService` 批量路径
- [x] 2.12 确认 `proxyProbeIdentity` **未**加入 `group_id`
- [x] 2.13 配置校验告警：`gateway.connection_pool_isolation=account` 时 slog.Warn（与代理组不兼容；推荐 proxy/account_proxy）。未做「存在组账号则拒绝启动」以免打断存量部署
- [x] 2.14 `max_upstream_clients` 已可配置（默认 5000，`gateway.max_upstream_clients`）；灰度前按账号×成员上界复核，不足再调高（无需改默认）
- [x] 2.15 解析器/选择器单测覆盖轮询与 sticky；DB 集成测试待有环境时补

## Phase 3. grok 专项

- [x] 3.1 将 `RefreshAccountToken` 改为 `accountProxyURL`（优先 hydrate 的 `account.Proxy`）
- [x] 3.2 确认管理端显式 proxyID 调用点仍走 `proxyURL(ctx, proxyID)` 不变
- [x] 3.3 `resolveProxyURL` 优先 `account.ProxyURL()`，再回退 ProxyID 查库
- [x] 3.4 `accountHasConfiguredProxy` 覆盖 `ProxyID` / `ProxyGroupID` / hydrate `Proxy`，修正 provider/account 失败归因
- [x] 3.5 单测锁定：`accountProxyURL` 优先 hydrate Proxy（池账号 ProxyID=nil）；`accountHasConfiguredProxy` 覆盖组；C1 由 applyProxyGroupSelection 单测锁定。生产 OAuth 联调见灰度清单
- [x] 3.6 代码路径：中继 `ProxyURL()` + OAuth `accountProxyURL()` + quota `resolveProxyURL` 均优先 hydrate Proxy；sticky 单测覆盖恒定性。生产出口 IP 抓取见灰度清单

## Phase 4. 管理 API

- [x] 4.1 新建 `backend/internal/service/proxy_group_service.go`（独立服务）
- [x] 4.2 新建 `backend/internal/handler/admin/proxy_group_handler.go`
- [x] 4.3 Create 幂等 + List/Get/Update/Delete/SetMembers
- [x] 4.4 Create 走 `executeAdminIdempotentJSON(c, "admin.proxy_groups.create", ...)`
- [x] 4.5 独立顶层路由 `/api/v1/admin/proxy-groups`
- [x] 4.6 DTO + mapper：`ProxyGroup` / `ProxyGroupWithProxies`
- [x] 4.7 删除保护：有成员或账号绑定时 `PROXY_GROUP_IN_USE`
- [x] 4.8 DI：repo/service/handler + wire_gen
- [x] 4.9 `make generate` 通过
- [ ] 4.10 导入导出自然键（延后）
- [ ] 4.11 export 审计映射（延后）
- [x] 4.12 服务层单测：创建/删除保护/策略归一；账号 `proxy_group_id` 创建与更新已接通

## Phase 5. 前端

- [x] 5.1 在 `frontend/src/types/index.ts:826` 附近新增 `ProxyGroup`、`CreateProxyGroupRequest`、`UpdateProxyGroupRequest` 类型
- [x] 5.2 新建 `frontend/src/api/admin/proxyGroups.ts`，仿 `proxies.ts` 的方法命名与导出结构（具名函数 + 聚合对象 + default）
- [x] 5.3 扩展 `frontend/src/components/common/ProxySelector.vue` 支持组模式；调整 `:72`、`:75`、`:83`、`:153` 四处 `modelValue === x` 严格相等判断
- [x] 5.4 在 `frontend/src/components/account/EditAccountModal.vue` 增加 `proxy_group_id`：模板 `:1415`、form `:3151`、回填 `:3240`、提交 `:4040`
- [x] 5.5 提交逻辑 MUST 沿用 `0 = 清除` 哨兵约定（见 `EditAccountModal.vue:4040-4042` 注释），否则清空代理组失效
- [x] 5.6 新建 `frontend/src/views/admin/ProxyGroupsView.vue`，复用 `ProxiesView.vue:972-984` 的组件依赖模式（TablePageLayout + DataTable + BaseDialog + ConfirmDialog）
- [x] 5.7 注册路由与侧栏入口，补 i18n 文案
- [x] 5.8 前端单测：选择器组模式、EditAccountModal 的 `proxy_group_id` 回填与清除


## Phase 5 补充（创建路径）

- [x] CreateAccountModal 全量 `accounts.create` 路径带 `proxy_group_id`
- [x] Grok SSO / Codex Session / Codex PAT 前后端请求体支持 `proxy_group_id`（OAuth exchange 临时 `proxyConfig` 仍仅 `proxy_id`，仅影响兑换出口）
- [x] 账号 i18n：`proxyGroup` / `noProxyGroup` / `proxyGroupHint`

## Phase 6. 验收与灰度

- [x] 6.1 逐条核对 `verification.md` 的证据矩阵
- [x] 6.2 确认 design.md §6 的四个开放问题均已决策并回写文档
- [x] 6.3 灰度手册已写入 verification.md（运维执行项；代码侧已具备）
- [x] 6.4 灰度手册已写入 verification.md（运维执行项；代码侧 sticky + 同出口已具备）
- [x] 6.5 默认 5000 可配置；验收文档给出估算公式与检查项
