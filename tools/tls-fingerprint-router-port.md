# 移植方案:TLS 指纹路由器 + 采集器(从 TokenFlux/TokenRouter → 本 fork)

> 目标:把 TokenRouter 的 **TLS 指纹路由器(UA→指纹规则)** 与 **运行时 TLS 指纹采集器** 移植进本 fork(MxyEI/sub2api),
> 覆盖 OpenAI 的 HTTP + WebSocket 两条上游路径。不切换部署、不动现有数据库数据。
>
> 本文基于对两仓库最新代码的实证核对(已纠正早期调研中的若干文件归属错误)。落地时仍以"按符号定位"为准,行号会随 main 同步漂移。

## 0. 关键决策(已与用户敲定)

- **解析范围:账号级 · 全平台**。把 `ResolveTLSProfile(account)` 统一换成路由感知的 `resolveTLSProfileForRequest(c, account)`,在所有带请求上下文的 `DoWithTLS` 处生效。**未绑路由器的账号(`router_id=0`)行为完全不变 —— 向后兼容**,完整覆盖 OpenAI,Claude 亦可顺带使用。
- **WS 纳入本期**:把 TokenRouter 给 `coder/websocket` 接 TLS 指纹的做法(强制 HTTP/1.1 + `DialTLSContext`)移植进 fork 的 `coderOpenAIWSClientDialer`。
- **采集器 token 复用行为原样照搬**(TokenRouter 生产已验证):入站 `ANTHROPIC_AUTH_TOKEN` / `x-api-key` 的 Bearer 复用为采集会话 token,不做收敛。
- **不移植** `datasharesession`(与本功能无关)。

## 0.5 无人值守(/goal)执行准则 —— 最高优先级,违反即停

本方案由 /goal 无人值守执行。以下是硬约束,优先级高于"尽快做完":

- **分支**:从 `feat/req-resp-archive` 切 `feat/tls-fingerprint-router`(与已部署的归档同源,日后可一并部署)。**已预建该分支并提交本方案 + 进度日志骨架**;开工先 `git switch feat/tls-fingerprint-router`,不要重建、不要从 main 切、不要碰 `feat/req-resp-archive` 本身。
- **验证后才提交**:每个 Phase 改完 → 容器内 `go generate ./ent` + `go generate ./cmd/server` + `go build ./...` + `go vet ./...` + 相关 `go test` 全绿,才 `git commit`(信息写明 Phase)。任一步红:先修;确实修不动 → 回滚本阶段改动 + 进度日志写明原因 + 停。
- **绝不假装通过**:凡声称"通过/完成",必须是刚跑过命令并看到输出;不得凭推断下结论。
- **不猜不编**:符号找不到 / `Profile` 结构对不齐 / 行为无法静态验证 / 与现状冲突 / 文档与代码不符 → 进度日志记录 + 停,**不臆测、不硬凑、不新建方案外的东西**。
- **不越界**:不 `push`、不部署、不动数据库实例、不暴露/启动采集器对真实流量;仅本地 commit 到新分支。
- **运行时正确性不可无人值守验证**:凡需抓包 / JA3 / 真实流量 / WS 串号判定的(见 §5 标【运行时】项),只做到"静态全绿 + commit",进度日志标注**"待人工灰度/抓包验证",不得宣称已验证**。
- **全程记录**:每个 Phase 完成或受阻,都追加 `tools/tls-router-port-progress.md`(阶段号与名、改动文件、所跑命令及结果、commit hash、遗留/风险);这是回来复盘的唯一依据。
- **顺序**:严格按 §8 的 A→K,有依赖不跳阶段、不并行。
- **环境自检**:开工前先在 `golang:1.26.4-alpine` 容器跑一次 `go build ./...` 确认工具链可用(命令见记忆 build-without-local-go / DEV_GUIDE);跑不通 → 记录 + 停,别硬来。

## 1. 范围与非目标

- 范围:`tls_fingerprint_routers` 表/模型/repo/cache/service;采集器 service + handler;`tlsfingerprint` 包补 3 个文件;OpenAI HTTP + WS 集成;前端 router 弹窗 + 采集器 UI + 账号绑定字段 + i18n。
- 非目标:数据迁移、Claude 专属 UA 改写(Claude 可用路由器选 profile,但不改 Claude 的 UA/Originator)、`datasharesession`。

## 2. 前置事实(已核实)

| 事实 | 值 |
|---|---|
| utls 版本(两边一致) | `github.com/refraction-networking/utls v1.8.2` |
| WS 库 | `github.com/coder/websocket v1.8.14`(另有 gorilla 但 OpenAI WS 用 coder) |
| 迁移机制 | 启动时 `setup.go` → `repository.ApplyMigrations` 顺序应用内嵌 `backend/migrations/*.sql`,按 **文件名+checksum** 记于 `schema_migrations`。**生产 DDL 以 SQL 文件为准,非 ent auto-migrate。** |
| **迁移最高号(pull 后)** | **157** → 新迁移用 **158** |
| 代码生成 | `make generate` = `go generate ./ent` + `go generate ./cmd/server`(在 `golang:1.26.4-alpine` 容器内跑,无本地 Go) |
| 前端栈(两边一致) | Vue3 + Vite + Pinia + Tailwind + vue-i18n |
| fork `account.go` 现有 | `IsTLSFingerprintEnabled()` (~1666)、`GetTLSFingerprintProfileID()` (~1684);**无** `GetTLSFingerprintRouterID`、**无** `SupportsTLSFingerprint` |
| fork `tlsfingerprint` 包 | **仅 `dialer.go`**(`NewDialer`/`NewHTTPProxyDialer`/`NewSOCKS5ProxyDialer`/`DialTLSContext`);**缺** `HTTP1OnlyProfile`、`CacheKey`、`CapturedClientHello`/`ParseCapturedClientHello` |

## 3. ⚠️ 早期调研纠错(影响实施)

1. TLS profile 的 `ResolveTLSProfile`/`DoWithTLS` 调用点在 **`gateway_service.go`(共享)+ `gateway_forward_as_responses.go`/`gateway_forward_as_chat_completions.go`/`openai_apikey_responses_probe.go`/`upstream_models.go`**,**不在** `openai_gateway_service.go`。后者只负责"造请求"(设 UA/Originator)。
2. **WS 路径当前完全没有 TLS 指纹**(`coderOpenAIWSClientDialer` 仅在代理场景建自定义 client)。
3. fork 的 `tlsfingerprint` 包只有 `dialer.go`,WS 与采集器需要的 `HTTP1OnlyProfile`/`CacheKey`/`ParseCapturedClientHello` 全缺,需一并移植。

---

## 4. 实施阶段

### Phase A — `tlsfingerprint` 包补齐(基础依赖)

| 操作 | 文件 | 说明 |
|---|---|---|
| 新增 | `backend/internal/pkg/tlsfingerprint/profile_alpn.go` | 照抄 `HTTP1OnlyProfile(*Profile) *Profile`(去掉 ALPN 里的 `h2`,WS 用) |
| 新增 | `backend/internal/pkg/tlsfingerprint/profile_cache_key.go` | 照抄 `CacheKey(*Profile) string`(profile 的稳定 SHA256,做连接池隔离 key) |
| 新增 | `backend/internal/pkg/tlsfingerprint/clienthello_capture.go` | 照抄 `CapturedClientHello` 结构 + `ParseCapturedClientHello([]byte)`(采集器解析 ClientHello → JA3/cipher/curves/ext/ALPN) |
| 核对 | `backend/internal/pkg/tlsfingerprint/dialer.go` | 确认 fork 的 `Profile` 结构字段与 TokenRouter 一致(都是 utls v1.8.2)。**结构一致 → 3 个文件直接照抄;结构有差异 → 在进度日志记录确切差异并停,不要自行臆测字段映射**(无人值守下改 `Profile` 风险高) |

风险:低(结构一致时)。无人值守提示:Profile 不一致属"需人工决策",停。

### Phase B — ent schema + model + 生成

| 操作 | 文件 | 说明 |
|---|---|---|
| 新增 | `backend/ent/schema/tls_fingerprint_router.go` | 字段:`name`(唯一)/`description`/`enabled`/`chatgpt_oauth_token_user_agent`/`chatgpt_oauth_token_tls_fingerprint_profile_id`(int64 nillable)/`codex_invite_reset_user_agent`/`codex_invite_reset_tls_fingerprint_profile_id`/`rules`(JSONB);索引 `enabled`;`TimeMixin`。JSONB 写法对齐 fork 的 `tls_fingerprint_profile.go` |
| 新增 | `backend/internal/model/tls_fingerprint_router.go` | `TLSFingerprintRouter`、`TLSFingerprintRouterRule`、match 常量(contains/prefix/exact/regex)、`Validate()`、`NormalizeTLSRouterMatchType()` |
| 生成 | (自动) | 容器内 `go generate ./ent` → 生成 `ent/tlsfingerprintrouter*.go` 等 + 更新 `ent/migrate/schema.go`(不手抄生成码) |

### Phase C — 数据库迁移(纯增量,安全)

| 操作 | 文件 | 说明 |
|---|---|---|
| 新增 | `backend/migrations/158_add_tls_fingerprint_routers.sql` | 把 TokenRouter 的 3 个迁移(151 建表 + 153 加 chatgpt_oauth 两列 + 161 加 codex_invite_reset 两列)合并为一,全部 `IF NOT EXISTS`;含 `idx_..._enabled`;核对 `migrations.go` 的 `embed.FS` 通配覆盖新文件 |

- 全新表 + 全新文件名 → 现有库零影响,可直接上线;先在测试库验证幂等。

### Phase D — repository(router repo + cache)

| 操作 | 文件 | 说明 |
|---|---|---|
| 新增 | `backend/internal/repository/tls_fingerprint_router_repo.go` | `List/GetByID/Create/Update/Delete` + `NewTLSFingerprintRouterRepository(*ent.Client)` |
| 新增 | `backend/internal/repository/tls_fingerprint_router_cache.go` | Redis key `tls_fingerprint_routers` + pubsub `tls_fingerprint_routers_updated` + TTL 24h + 本地缓存(预编译 regex);接口同 profile cache |
| 编辑 | `backend/internal/repository/wire.go`(~90、~125) | ProviderSet 加 `NewTLSFingerprintRouterRepository`、`NewTLSFingerprintRouterCache` |

### Phase E — service(router + collector)+ config

| 操作 | 文件 | 说明 |
|---|---|---|
| 新增 | `backend/internal/service/tls_fingerprint_router_service.go` | `MatchUserAgent(routerID, ua) → TLSFingerprintRouterMatchResult{Matched,RouterID,RouterName,RuleName,TLSFingerprintProfileID,UpstreamUserAgent,UpstreamOriginator}`、`GetRuntimeRouter`、CRUD、本地缓存+订阅刷新 |
| 新增 | `backend/internal/service/tls_fingerprint_collector_service.go` | 照抄(含 token 复用):独立 HTTPS 监听、自签 CA、内存采集会话(TTL/最多 N 条)、`Start/Stop/Status/CreateSession/ListCaptures/DeleteSession` |
| 编辑 | `backend/internal/service/tls_fingerprint_profile_service.go` | 加 `ResolveRoutableTLSProfileByID`、`ResolveTokenTLSProfileByID`(router 命中/OAuth-token 用;对齐 TokenRouter 的 -1 随机 / 0 内置 / >0 指定语义) |
| 编辑 | `backend/internal/config/config.go`(ServerConfig 内) | 加 `TLSFingerprintCollector TLSFingerprintCollectorConfig{host,port,public_base_url,cert_file,key_file,session_ttl_seconds,max_records_per_session}` + `viper.SetDefault`(参考本次 pull 里 `quota_headroom` 的 SetDefault/Validate 写法);默认安全(关闭或仅 127.0.0.1) |
| 编辑 | `backend/internal/service/wire.go` | 加 `NewTLSFingerprintRouterService`、`NewTLSFingerprintCollectorService`;变参 provider `ProvideOpenAIGatewayTLSFingerprintRouterServices`;`wire.Bind` 接口绑定(OAuth-token reader/profile resolver) |

### Phase F — handler + 路由 + wire

> 采集器**不是** ent 实体:无 repo/cache/独立 CRUD 路由,端点挂在 profile handler 上(与 TokenRouter 一致)。

| 操作 | 文件 | 说明 |
|---|---|---|
| 新增 | `backend/internal/handler/admin/tls_fingerprint_router_handler.go` | `List/GetByID/Create/Update/Delete` + DTO(含 `nullableInt64Patch` 区分 null/缺省) |
| 编辑 | `backend/internal/handler/admin/tls_fingerprint_profile_handler.go` | 加采集器方法 `CollectorStatus/StartCollector/StopCollector/CreateCollectorSession/ListCollectorCaptures/DeleteCollectorSession`;构造函数多注入 `*service.TLSFingerprintCollectorService` |
| 编辑 | `backend/internal/server/routes/admin.go`(~619-628 区) | 加 `/tls-fingerprint-routers` 5 个 CRUD;在 `/tls-fingerprint-profiles` 下加 `collector/status`、`collector/start`、`collector/stop`、`collector/sessions`、`collector/sessions/:token/captures`、`DELETE collector/sessions/:token` |
| 编辑 | `backend/internal/handler/wire.go`(~34、~68、~183) | `ProvideAdminHandlers` 加 router handler 参数;`AdminHandlers` 加字段;ProviderSet 加 `admin.NewTLSFingerprintRouterHandler`;profile handler 构造补注入 collector |

### Phase G — ⚠️ OpenAI **HTTP** 集成(核心 · 账号级解析)

> 这是最易随 main 漂移的部分,保持改动**集中在新 helper**,降低未来合并冲突。

**G1. account getter** — `backend/internal/service/account.go`(在 `GetTLSFingerprintProfileID` 后):
- 加 `GetTLSFingerprintRouterID() int64`,镜像 profile getter 的类型转换(float64/int64/json.Number)。
- 加 `SupportsTLSFingerprint() bool`(fork 当前无):Anthropic OAuth/SetupToken 或 OpenAI OAuth 返回 true(供前端门控 + resolve)。

**G2. 路由感知解析 helper**(放在 gateway 服务上,集中维护):
- `matchTLSFingerprintRouter(c, account) TLSFingerprintRouterMatchResult`:读入站 `User-Agent` → `tlsFPRouterService.MatchUserAgent(account.GetTLSFingerprintRouterID(), ua)`(无 router/无 c → `Matched=false`)。
- `resolveTLSProfileForRequest(c, account) *tlsfingerprint.Profile`:`m := matchTLSFingerprintRouter(c, account)`;命中 → `ResolveRoutableTLSProfileByID(account, m.TLSFingerprintProfileID)`,否则 → 现有 `tlsFPProfileService.ResolveTLSProfile(account)`。
- 在 gateway 服务结构体加 `tlsFPRouterService *TLSFingerprintRouterService`,构造函数用变参注入(对齐 wire 变参 provider)。

**G3. 替换所有"带请求上下文"的 profile 解析点**(把 `s.tlsFPProfileService.ResolveTLSProfile(account)` → `s.resolveTLSProfileForRequest(c, account)`):
- `gateway_service.go`:`tlsProfile := ...ResolveTLSProfile(account)`(~4999,随后在 ~5048/5126/5167/5246 重试链复用,只需改这一处源头)+ 内联点 ~5615 / ~9990 / ~10017 / ~10114。
- `gateway_forward_as_responses.go:~128`、`gateway_forward_as_chat_completions.go:~129`。
- `openai_apikey_responses_probe.go:~157`(probe 一般无入站 UA → 自然回落账号默认)。
- **无请求上下文的后台调用保持原样**:`account_usage_service.go:~1210`、`upstream_models.go:~362/364`、`gateway_service.go:~6468`(传 nil)。

> 注意:每个请求只 match 一次(放在 profile 首次解析处),重试链复用结果。

**G4. UA/Originator 改写(OpenAI 专属)** — `openai_gateway_service.go`:
- `buildUpstreamRequest`(~4377;UA ~4461、originator ~4438)与 `buildUpstreamRequestOpenAIPassthrough`(~3587;UA ~3678、originator ~3660):当 `routerMatch.Matched` 且规则给了 `UpstreamUserAgent`/`UpstreamOriginator` 时覆盖之(优先级高于账号默认 UA / `resolveOpenAIUpstreamOriginator`)。这两个函数都有 `c`,就地 `matchTLSFingerprintRouter(c, account)`。

**G5. OAuth token / Codex invite-reset 专用路径**:
- `openai_oauth_service.go`:换 token 请求用 router 的 `ChatGPTOAuthTokenUserAgent` + `ChatGPTOAuthTokenTLSFingerprintProfileID`(经 `GetRuntimeRouter`)。需 `wire.Bind` OAuth-token reader/profile resolver 接口。
- Codex invite-reset:先 grep 确认 fork 是否有对应服务(TokenRouter 在 `codex_invite_reset_service.go`)。**fork 无该服务 → 跳过并在进度日志记录"fork 无 codex_invite_reset,跳过",不要为它新建服务或臆造调用路径。** `openai_codex_pat_service.go:61` 现写死 `originator=codex_cli_rs`,本期不动(留作后续)。
- 无人值守提示:G5 涉及 OAuth 刷新链路,若 wire 接口绑定改动后 `wire_gen` 报错且无法在不臆测的前提下修复 → 回滚 G5、记录、保留 G1–G4 成果后停(G5 非 OpenAI 主路径,可后补)。

### Phase H — ⚠️ OpenAI **WebSocket** 集成

> fork WS 走 `coder/websocket`,当前无任何 TLS 指纹。移植 TokenRouter 做法:给 dialer 传 profile → 自定义 `http.Client` 的 `Transport.DialTLSContext` 用 `tlsfingerprint` 拨号器,强制 HTTP/1.1。
>
> ⚠️ **作用域守护**:TokenRouter 的 `openai_ws_forwarder.go` 现已混入与本移植**无关**的功能(`ApplyUserPromptReplacement` 用户提示词替换、`BeforeRequest` 钩子返回值签名改动等)。Phase H **只移植下表列出的 TLS 相关符号**(`resolveOpenAIWSTLSProfile` / 拨号器传 `TLSProfile`+`TLSProfileKey` / 连接池 key 含指纹 / `matchTLSFingerprintRouter` / UA·Originator 覆写),**不要连带 user-prompt-replacement、ingress-session 或 hook 签名改动**——fork 没有这些,带进来会编译失败或扩大作用域。

| 操作 | 文件 | 说明 |
|---|---|---|
| 新增 | `backend/internal/service/openai_gateway_service.go` | `resolveOpenAIWSTLSProfile(account, routerMatch) (*tlsfingerprint.Profile, string)` = `resolveTLSProfileForRequest` + `tlsfingerprint.HTTP1OnlyProfile(...)`,返回 profile + 缓存 scope key(`tls-router-{routerID}-{profileID}` / `tls-random` / `CacheKey`) |
| 编辑 | `backend/internal/service/openai_ws_client.go` | ① `openAIWSClientDialer.Dial` 接口签名加 `profile *tlsfingerprint.Profile`;② `coderOpenAIWSClientDialer.Dial`:`profile != nil` 时也建自定义 client 设 `opts.HTTPClient`;③ 新增 `buildOpenAIWSHTTPTransport(proxy, parsedURL, profile)`:`HTTP1OnlyProfile` 后按 无代理/SOCKS5/HTTP 选 `NewDialer`/`NewSOCKS5ProxyDialer`/`NewHTTPProxyDialer` 设 `DialTLSContext`;④ `proxyHTTPClient` 缓存 key 改成 `proxy + "|tls:" + CacheKey(profile)` |
| 编辑 | `backend/internal/service/openai_ws_pool.go` | `openAIWSAcquireRequest`(~62)加 `TLSProfile *tlsfingerprint.Profile`(浅拷贝处 ~1668 一并复制);`clientDialer.Dial(...)`(~1545 对应点)传 `req.TLSProfile`;**WS 连接池 key 纳入 tlsProfileKey**,避免不同指纹的连接被复用 |
| 编辑 | `backend/internal/service/openai_ws_forwarder.go` | 取 `tlsProfile, tlsProfileKey := s.resolveOpenAIWSTLSProfile(account, tlsRouterMatch)` 写入 acquire 请求(TokenRouter 有 2 处 ~1960/~3077,找 fork 等价点);UA/Originator(~1156)按 routerMatch 覆写 |

风险:**高**。连接池 key 漏带指纹会串号;务必抓包/JA3 验证。

### Phase I — cmd/server wire:采集器优雅关闭

| 操作 | 文件 | 说明 |
|---|---|---|
| 编辑 | `backend/cmd/server/wire.go` | 优雅关闭表加 `{"TLSFingerprintCollectorService", func() error { return tlsFingerprintCollector.Stop(ctx) }}`;注入 collector 实例(参考归档功能的注册方式) |

### Phase J — 代码生成 + 编译(容器内)

1. `go generate ./ent`  2. `go generate ./cmd/server`(= `make generate`)  3. `go build ./...` + `go vet ./...` + `go test ./internal/service/... ./internal/pkg/tlsfingerprint/...`
- 易错:wire 变参 provider/bind 签名不匹配会在 `wire_gen` 阶段报错——照搬 TokenRouter 的 provider 形态。

### Phase K — 前端(Vue3,可近乎照抄)

| 操作 | 文件 | 说明 |
|---|---|---|
| 新增 | `frontend/src/api/admin/tlsFingerprintRouter.ts` | router CRUD + 类型 |
| 编辑 | `frontend/src/api/admin/tlsFingerprintProfile.ts` | 补采集器 6 函数 + 3 类型(status/session/captureRecord) |
| 编辑 | `frontend/src/api/admin/index.ts` | 导出 `tlsFingerprintRouterAPI` + 类型 |
| 新增 | `frontend/src/components/admin/TLSFingerprintRoutersModal.vue` | router 列表/编辑/规则编辑器/YAML 粘贴 + chatgpt_oauth_token / codex_invite_reset 两槽(~880 行,照抄) |
| 编辑 | `frontend/src/components/admin/TLSFingerprintProfilesModal.vue` | 加采集器 UI(start/stop、建会话、capture URL + CA PEM、Claude/Codex 命令、采集列表、存为 profile) |
| 编辑 | `frontend/src/views/admin/AccountsView.vue` | 加 "TLS Routers" 入口 + 弹窗挂载 + `openTLSFingerprintRouters` + state(注意 pull 后行号漂移,按符号定位) |
| 编辑 | `frontend/src/components/account/EditAccountModal.vue`、`CreateAccountModal.vue` | 加 `tls_fingerprint_router_id` 绑定下拉(含"不使用路由器")+ 提交写入 `extra` |
| 编辑 | `frontend/src/i18n/locales/en.ts`、`zh.ts` | 补 `admin.tlsFingerprintRouters.*`(~74 键)+ 账号表单 router 键 |

前端验证(无人值守):大文件(~880 行弹窗)照抄后,**逐项核对 import 路径、API 调用名、i18n key 与 fork 既有约定一致**(对照 fork 现有的 `TLSFingerprintProfilesModal.vue` 写法,不要照搬 TokenRouter 里 fork 不存在的组件/工具函数)。改完跑前端类型检查/构建验证(如 `pnpm -C frontend type-check` 或 `build`,具体命令读 `frontend/package.json` scripts);**若前端工具链不可用,完成代码后在进度日志记录"前端待人工构建验证",不阻塞,不宣称已验证**。

---

## 5. 验证清单(【静态】=无人值守可验;【运行时】=须人工灰度/抓包)

- [ ] 【静态】容器内 `make generate` 无报错;`go build ./...` + `go vet ./...` + 相关 `go test` 通过
- [ ] 【静态】router/collector 移植了对应单测(参照 TokenRouter 用例),`go test` 全绿
- [ ] 【静态】前端类型检查/构建通过(工具链不可用则记录待人工)
- [ ] 【运行时·人工】`158_*.sql` 在测试库幂等执行,表结构/索引正确,`schema_migrations` 记一条
- [ ] 【运行时·人工】router CRUD API + 前端弹窗正常;多实例 pubsub 缓存失效生效
- [ ] 【运行时·人工】**HTTP**:账号绑 router,入站 UA 命中 → 出站对应 TLS 指纹 + 改写 UA/Originator(抓包/对端 JA3)
- [ ] 【运行时·人工】**WS**:Codex WS 按 UA 命中 → 指纹生效;不同指纹连接不串号(连接池 key 含指纹)
- [ ] 【运行时·人工】回归:未命中 → 回落账号 profile;`router_id=0`/未启用 TLS → 行为同现状;归档不受影响
- [ ] 【运行时·人工】采集器:Start → Claude Code/Codex 指向 → 抓到指纹存为 profile;Stop 后内存清空
- [ ] 【运行时·人工】OAuth 换 token /(若有)invite-reset 用 router 专用 UA+profile

> 无人值守只勾【静态】项;【运行时·人工】项一律不勾,在进度日志列为"待人工验证"。

## 6. 风险总览

| 风险 | 等级 | 缓解 |
|---|---|---|
| WS 连接池 key 漏带指纹 → 串号 | **高** | key 纳入 tlsProfileKey;抓包验证 |
| HTTP 集成跨 ~6 文件、易随 main 漂移 | **高** | 改动集中在 `resolveTLSProfileForRequest`/`matchTLSFingerprintRouter` helper;尽早做 |
| `Profile` 结构两边不一致 | 中 | Phase A 先对齐再编译 |
| wire 变参 provider/bind 不匹配 | 中 | 照搬 TokenRouter provider 形态 |
| 采集器端口暴露鉴权 token | 中(运维) | 默认不暴露,按需启停 |
| 迁移 | 低 | 纯增量 + IF NOT EXISTS |

## 7. 运维须知

- `158_*.sql` 启动自动应用(幂等增量),现有数据零影响;先测试库后 VPS。
- 采集器监听独立端口(默认 `:8443`),采集期间内存短暂持有鉴权 token。**默认不映射该端口或仅绑内网**,取指纹时临时 `Start`、用完 `Stop`;`config.yml` 加 `server.tls_fingerprint_collector` 段并给安全默认。

## 8. 建议执行顺序

A(包)→ B(ent)→ C(迁移)→ D(repo)→ E(service/config)→ F(handler/路由/wire)→ **G(HTTP 集成)→ H(WS 集成)** → I(优雅关闭)→ J(生成/编译)→ K(前端)→ 验证。
G/H 是硬骨头:各自移植 TokenRouter 对应单测(`openai_gateway_service_test.go` / WS 用例)做**静态验证**,build+vet+test 全绿后 commit;**运行时灰度/抓包验证留人工**(见 §0.5、§5)。无人值守做到此为止,不得宣称运行时已验证。

> 小提示:每个 Phase 之间是天然的"安全停靠点"——若某阶段受阻,前面阶段已各自 commit、build 绿,工作树干净,人工接手时从进度日志续作即可。
