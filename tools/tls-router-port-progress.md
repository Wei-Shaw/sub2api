# 进度日志:TLS 指纹路由器 + 采集器移植

> 配套方案:`tools/tls-fingerprint-router-port.md`。本文件是 /goal 无人值守执行的**唯一进度账本**。
> 规则:每个 Phase **完成或受阻**都必须在此追加一条;只勾【静态】验证,【运行时·人工】项一律留给人工。
>
> 分支:`feat/tls-fingerprint-router`(从 `feat/req-resp-archive` 切出)。基线 commit:`330c35c1`。

## 阶段状态总览

| Phase | 名称 | 状态 | commit | 静态验证(build/vet/test) | 备注 |
|---|---|---|---|---|---|
| 0 | 环境自检 `go build ./...` | ✅ 完成 | `e6ba7d0a` | gofmt✅ build✅ vet✅ routes-test✅ | Blocker #1 已按**方案 A**(人工拍板)修复;基线转绿,无隐藏编译错 |
| A | tlsfingerprint 包补 3 文件 | ✅ 完成 | `4488db9c` | gofmt✅ build✅ vet✅ pkg-test✅ | Profile 结构两边逐字一致;照抄 3 源 +2 测;`isGREASEValue` 依赖已存在 |
| B | ent schema + model + 生成 | ✅ 完成 | `ce925be7` | gofmt✅ generate✅ build✅ vet✅ | 表/列/索引与方案逐项核对一致;go.mod/sum 未被 codegen 污染 |
| C | 迁移 158_add_tls_fingerprint_routers.sql | ✅ 完成(静态+ephemeral PG) | `d6cec220` | build✅ vet✅ runner-test✅ + ephemeral PG 幂等✅ | postgres:18-alpine 跑两遍幂等通过,11 列/3 索引与 ent 一致;生产/测试库应用仍留人工 |
| D | repository(router repo + cache) | ✅ 完成 | `0e5c588d` | gofmt✅ build✅ vet✅ svc-test✅ | 含 router service(接口与 repo 互依,合并提交);6 子测全绿 |
| E | service(router + collector)+ config | ✅ 完成 | `0e5c588d`+`3161a1df` | gofmt✅ build✅ vet✅ svc-test✅ | router svc 随 D;本提交:collector+config+profile 编辑+wire providers。变参 provider/wire.Bind 推迟到 G(依赖 gateway/OAuth) |
| F | handler + 路由 + wire | ✅ 完成 | `8f57d2c6` | gofmt✅ wire-gen✅ build✅ vet✅ test✅ | wire 重生成成功;cmd/server wire_gen_test 通过 |
| G | OpenAI HTTP 集成 | ✅ 完成(含 G5) | `21e18825`+`dedb31e6`+`727f3bb7`+`ae324f7a`+`162b8377` | gofmt✅ build✅ vet✅ test✅ | HTTP 全主路径路由感知 TLS + UA/Originator;**G5(OAuth 换 token 专用 UA/profile)已完成**(`162b8377`,见下) |
| H | OpenAI WS 集成 | ✅ 完成(静态) | `ef966a8f` | gofmt✅ build✅ vet✅ ws-test✅ | 连接池每处取连接点都按 matchesTLSProfile 过滤;串号须人工抓包验证 |
| I | cmd/server 优雅关闭 | ✅ 完成 | `0f2624f7` | gofmt✅ wire-gen✅ build✅ vet✅ test✅ | 采集器 Stop 入 provideCleanup;先于 H 做(独立+安全,H 已记录待续) |
| J | 代码生成 + 编译 | ✅ 完成 | (无码改) | generate 无 drift✅ build✅ vet✅ | 全量 `go generate ./ent`+`./cmd/server` 后 git status 空=生成码与 schema/providers 完全一致 |
| K | 前端 | ✅ 完成(typecheck✅) | `49547eb3`+`9bfac46d`+`a71b135b` | pnpm typecheck✅(exit 0) | part1 RoutersModal+API+入口+i18n / part2 账号绑定下拉 / part3 ProfilesModal 采集器 UI 全部完成绿 |

状态图例:⬜ 未开始 / 🟡 进行中 / ✅ 完成(静态绿+commit)/ ⛔ 受阻(见下方记录)

## 执行日志(按时间追加)

<!-- 每条格式:
### Phase X — <名称> — <完成/受阻>
- 改动文件:...
- 命令与结果:go generate/build/vet/test → 通过 or 失败摘要
- commit:<hash>
- 遗留/风险:...
-->

### Phase 0 — 环境自检(`go build ./...`)— ⛔ 受阻(2026-07-01)

- 工具链:**可用**。`golang:1.26.4-alpine` 容器正常拉依赖、编译、报出真实错误(记忆 build-without-local-go 的命令有效)。
- 命令与结果:
  ```
  docker run --rm -e CGO_ENABLED=0 -e GOFLAGS=-mod=mod -v "$PWD":/app -w /app \
    -v sub2api-gomod:/go/pkg/mod -v sub2api-gobuild:/root/.cache/go-build \
    golang:1.26.4-alpine sh -c 'go build ./... && echo BUILD_OK'
  → exit 1:
  internal/server/routes/gateway.go:187:5: undefined: rejectGrokUnsupportedEndpoint
  internal/server/routes/gateway.go:204:5: undefined: rejectGrokUnsupportedEndpoint
  ```
- 结论:**base 分支 `feat/tls-fingerprint-router` 自身编译不过**,且该错误**与本次 TLS 移植无关**(本分支仅 2 个 docs commit)。这违反契约前提「build 全绿才提交」,无法在此基线上做任何可验证的 Phase。
- 处置:依契约「环境自检跑不通 → 记录 + 停」「符号找不到 / 与现状冲突 → 记录 + 停,不猜不编」「不越界」,**停机**,不擅自修复(修复方向是行为决策,见 Blocker #1)。
- 改动文件:无(仅本账本)。工作树:干净(只读/grep,未改任何代码)。
- 待人工:决定 Blocker #1 的修复方向后,我方可在绿色基线上从 Phase A 续作。

### Phase 0 (续)— baseline fix:按方案 A 修复 Blocker #1 — ✅ 完成(2026-07-01)

- 人工拍板:**方案 A**(删除两处孤儿 Grok 拒绝块,使行为与 main 一致 / 恢复 Grok-CLI 兼容)。修在**本分支**(契约禁改 `feat/req-resp-archive`),作为独立 baseline-fix commit。
- 改动文件:`backend/internal/server/routes/gateway.go`
  - `gofmt -w`:把 `330c35c1` 合并残留的空格缩进归一为 tab(仅该破损区域,20±/20∓,纯空白)。
  - 删 `GET /responses` 闭包内 `if getGroupPlatform(c)==service.PlatformGrok { reject; return }`(4 行)→ 闭包仅 `h.OpenAIGateway.ResponsesWebSocket(c)`(保留 `archiveCapture`)。
  - 删 `POST /chat/completions` 闭包内同款 Grok 块 + 其后空行 → 落到 `isOpenAIResponsesCompatibleGatewayPlatform(c)` 分支(该闭包对 OpenAI+Grok 均返回 true,line 42-47),Grok 走 OpenAI 兼容处理(保留 `archiveCapture`)。
- 行为校验(静态):`isOpenAIResponsesCompatibleGatewayPlatform` 对 Grok 返回 true → Grok `/chat/completions` 进 `h.OpenAIGateway.ChatCompletions`;`GET /responses` 所有平台进 `ResponsesWebSocket`(与 `/v1` 组 line 107-109 一致)。无其它 undefined 符号。
- 命令与结果(容器内):
  - `gofmt -l internal/server/routes/gateway.go` → 空(clean)
  - `go build ./...` → **BUILD_OK**
  - `go vet ./...` → **VET_OK**
  - `go test ./internal/server/routes/...` → **ok 1.495s**
- commit:`e6ba7d0a`
- 遗留/风险:此修复仅在本 TLS 分支;父分支 `feat/req-resp-archive` 仍存同样破损(契约禁改,留待人工在源头处理或后续 rebase 携带)。Grok 端点行为变更(不再路由层拒绝)属【运行时·人工】可灰度复核项,但与 main 完全一致。

### Phase A — tlsfingerprint 包补齐 — ✅ 完成(2026-07-01)

- 前置核对(关键):fork 与 TokenRouter 的 `Profile` 结构**逐字一致**(11 字段:Name/CipherSuites/Curves/PointFormats/EnableGREASE/SignatureAlgorithms/ALPNProtocols/SupportedVersions/KeyShareGroups/PSKModes/Extensions,类型/顺序/注释全同)→ 满足「结构一致 → 直接照抄」。
  - `dialer.go` 两边有差异(TR 多 `stdtls`/`errors`/`strings` import 与 `HTTPProxyDialer.proxyTLSConfig` 字段),**属 dialer.go 范畴,本期不动**;已确认 3 个新文件不依赖这些差异。
- 依赖/碰撞核对:`clienthello_capture.go` 用到的 `isGREASEValue` 已存在于 fork `dialer.go:330`✅;新增符号(`SupportsHTTP2`/`HTTP1OnlyProfile`/`NegotiatedProtocol`/`CacheKey`/`CapturedClientHello`/`ParseCapturedClientHello` 及各解析 helper)与 fork 现有**无重名**✅;`test_types_test.go` 两边 identical(未动);新测试函数名与 fork 现有无碰撞✅;`NewDialer(profile,nil)` 签名匹配✅;`testify v1.11.1` 在 go.mod✅。
- 改动文件(从 TokenRouter 照抄,byte-identical):
  - 新增 `backend/internal/pkg/tlsfingerprint/profile_alpn.go`(`SupportsHTTP2`/`HTTP1OnlyProfile`/`NegotiatedProtocol`/内部 `effectiveALPNProtocols`/`profileHasALPNExtension`)
  - 新增 `backend/internal/pkg/tlsfingerprint/profile_cache_key.go`(`CacheKey`,Profile 稳定 SHA256)
  - 新增 `backend/internal/pkg/tlsfingerprint/clienthello_capture.go`(`CapturedClientHello`+`ParseCapturedClientHello`,解析 ClientHello → JA3/cipher/curves/ext/ALPN)
  - 新增测试 `profile_alpn_test.go`、`clienthello_capture_test.go`
- 注:Phase A 不触 ent/schema/wire → `go generate` N/A(留待 Phase B 起)。
- 命令与结果(容器内):
  - `gofmt -l <5 files>` → 空(clean)
  - `go build ./...` → **BUILD_OK**
  - `go vet ./internal/pkg/tlsfingerprint/...` → **VET_OK**
  - `go test ./internal/pkg/tlsfingerprint/...` → **ok 0.005s**(含新增 3 测试)
- commit:`4488db9c`
- 遗留/风险:无。纯增量、leaf 包,不影响既有 importer。

### Phase B — ent schema + model + 生成 — ✅ 完成(2026-07-01)

- 核对:fork 与 TR 的 `tls_fingerprint_profile.go` ent schema **逐字一致**(仅 import path)→ 约定对齐;router schema 直接照搬 TR,仅换 import path(`mixins`、`internal/model`)。model 文件 stdlib-only + `ValidationError`(fork 已有 `Field/Message`,error_passthrough_rule.go:69)→ 逐字照抄。
- 导入环检查:fork `internal/model` 无任何 ent import → `ent/schema`(及生成的 `ent`)依赖 `internal/model` 不成环(与 TR 同模式)。
- 改动文件:
  - 新增 `backend/internal/model/tls_fingerprint_router.go`(照抄 TR:match 常量 contains/prefix/exact/regex、`TLSFingerprintRouterRule`、`TLSFingerprintRouter`、`Validate()`×2、`NormalizeTLSRouterMatchType()`)
  - 新增 `backend/ent/schema/tls_fingerprint_router.go`(TimeMixin;字段 name 唯一/description/enabled/chatgpt_oauth_token_user_agent/chatgpt_oauth_token_tls_fingerprint_profile_id(int64 nillable)/codex_invite_reset_*/rules(jsonb);索引 enabled)
  - 生成(`go generate ./ent`):新增 `ent/tlsfingerprintrouter.go`+`ent/tlsfingerprintrouter/`+`_create/_delete/_query/_update.go`;改 `ent/{client,ent,mutation,tx,runtime/runtime,hook/hook,intercept/intercept,predicate/predicate,migrate/schema}.go`
- 生成结果核对(`migrate/schema.go`):表 `tls_fingerprint_routers`,列 id/created_at/updated_at/name(Unique,100)/description(Text,nullable)/enabled(Bool,default true)/chatgpt_oauth_token_user_agent(512,default "")/chatgpt_oauth_token_tls_fingerprint_profile_id(Int64,nullable)/codex_invite_reset_user_agent(512,default "")/codex_invite_reset_tls_fingerprint_profile_id(Int64,nullable)/rules(jsonb,nullable);索引 `tlsfingerprintrouter_enabled` on enabled。**与方案逐项一致**。
- 命令与结果(容器内):`gofmt -l` 空;`go generate ./ent` exit 0(go.mod/go.sum **未改**,codegen 工具 cobra/tablewriter 等只进缓存);`go build ./...` → **BUILD_OK**;`go vet ./ent/... ./internal/model/...` → **VET_OK**;`go test ./internal/model/...` → no test files。
- 单测:TR **无** router model 单测 → 无可照抄,未新建(契约「不新建方案外的东西」;Validate/Normalize 逻辑后续经 service 测试间接覆盖)。
- commit:`ce925be7`
- 遗留/风险:无。建表 DDL 留待 Phase C 手写迁移(生产以 SQL 文件为准,非 ent auto-migrate)。

### Phase C — 迁移 158_add_tls_fingerprint_routers.sql — ✅ 完成(静态)(2026-07-01)

- 改动文件:新增 `backend/migrations/158_add_tls_fingerprint_routers.sql`
- 内容:合并 TR 的 151(建表)+153(chatgpt_oauth_token 两列)+161(codex_invite_reset 两列)为一,去重 `SET LOCAL`。全部 `IF NOT EXISTS`(表/索引/4 个 ADD COLUMN)→ 幂等、对现有库零影响(全新表)。
- 核对:
  - 列类型与 Phase B ent 生成结果**逐项一致**(name VARCHAR(100) UNIQUE / description TEXT / enabled BOOLEAN default true / rules JSONB / chatgpt_oauth_token_user_agent VARCHAR(512) default '' / chatgpt_oauth_token_tls_fingerprint_profile_id BIGINT / codex_invite_reset_* 同 / 索引 idx_tls_fingerprint_routers_enabled)。
  - embed:`migrations.go` 用 `//go:embed *.sql` → 158 自动覆盖,无需改 embed。
  - 风格对齐 fork 兄弟迁移 `080_create_tls_fingerprint_profiles.sql`(SET LOCAL + IF NOT EXISTS);`SET LOCAL` 是 fork 既有约定(10 个迁移在用),非 _notx 文件默认在事务内执行 → `SET LOCAL` 有效。
  - runner 校验:`validateMigrationExecutionMode` 仅对非 _notx 文件禁止 `CONCURRENTLY`(158 无)→ 合法。
  - baseline:`latestMigrationBaseline` 动态取最高号(无硬编码 157),`ensureAtlasBaselineAligned` 亦动态 → 158 自然成为新 baseline,无需改常量;无 checksum 兼容规则(158 是全新文件,非编辑旧迁移)。
- 命令与结果(容器内):`go build ./...` → **BUILD_OK**;`go vet ./internal/repository/...` → **VET_OK**;`go test ./internal/repository/`(非 DB 的 runner 单测:ValidateMigrationExecutionMode/LatestMigrationBaseline/ApplyMigrationsFS/Checksum/EnsureAtlasBaselineAligned 等)→ **ok 0.021s**。
- commit:`d6cec220`
- 遗留/风险:**生产/测试库应用属【运行时·人工】**(方案 §5)。
- **补充验证(2026-07-01,镜像拉取完成后补跑)**:ephemeral `postgres:18-alpine` 容器内 `psql --single-transaction -v ON_ERROR_STOP=1 -f 158.sql` **连跑两遍均成功(幂等 ✅)**;结果 11 列(id/name/description/enabled/rules/created_at/updated_at/chatgpt_oauth_token_user_agent/chatgpt_oauth_token_tls_fingerprint_profile_id/codex_invite_reset_user_agent/codex_invite_reset_tls_fingerprint_profile_id)+ 3 索引(pkey / name_key 唯一 / idx_tls_fingerprint_routers_enabled),类型与 Phase B ent 生成**逐项一致**。容器用完即删。**仍不宣称生产/测试库已验证**(那需在真实库跑,留人工;但语法+幂等+结构已在真 PG 引擎上验证)。

### Phase D(+E 的 router service)— repository + router service — ✅ 完成(2026-07-01)

- 关键决策(顺序):router **repo/cache** 的构造函数返回 `service.TLSFingerprintRouterRepository`/`service.TLSFingerprintRouterCache` 接口,而这两个接口定义在 TR 的 `tls_fingerprint_router_service.go`(E 文件)里。二者**互依**,无法各自单独 build 绿。故把 router service 文件随 D 一并引入并合并提交(faithful 照抄 TR 文件布局,不拆分接口);E 余下 collector/config/profile 编辑/service-wire 仍单独做。
- 前置核对:fork 与 TR 的 `tls_fingerprint_profile_repo.go`、`tls_fingerprint_profile_cache.go` **逐字一致**(仅 import path)→ repo/cache 基础设施(ent client、redis 包装、cache+pubsub 模式)完全对齐;`logger.LegacyPrintf` fork 已有(logger.go:477,签名同);repository→service import 是 fork 既有无环模式。
- 改动文件(TR 照抄 + import path swap):
  - 新增 `internal/repository/tls_fingerprint_router_repo.go`(List/GetByID/Create/Update/Delete + toModel;Update 用 Clear* 处理 nil)
  - 新增 `internal/repository/tls_fingerprint_router_cache.go`(redis key `tls_fingerprint_routers` + pubsub `tls_fingerprint_routers_updated` + TTL 24h + 本地缓存)
  - 新增 `internal/service/tls_fingerprint_router_service.go`(接口×2、`TLSFingerprintRouterMatchResult`、`TLSFingerprintRouterService`:MatchUserAgent/GetRuntimeRouter/CRUD/本地缓存+订阅刷新+预编译 regex)
  - 新增 `internal/service/tls_fingerprint_router_service_test.go`(照抄 TR,6 子测)
  - 编辑 `internal/repository/wire.go`:ProviderSet 加 `NewTLSFingerprintRouterRepository`(repo 段)、`NewTLSFingerprintRouterCache`(cache 段)
- 命令与结果(容器内):`gofmt -l` 空;`go build ./...` → **BUILD_OK**;`go vet ./internal/repository/... ./internal/service/...` → **VET_OK**;`go test ./internal/service/ -run TestTLSFingerprintRouter` → **ok**(MatchUserAgent 5 子用例[exact/contains/prefix/regex/未命中]+ 大小写敏感 + 校验规则 + Create 归一化 + ProfileID 校验,全 PASS)。
- 注:`NewTLSFingerprintRouterService` 暂未被 wire 消费(service/wire.go 留待 E);repo/cache provider 已入 ProviderSet 但未重生成 wire_gen(未消费 → build 不受影响,wire 重生成留待 Phase F/J)。
- commit:`0e5c588d`
- 遗留/风险:无。E 余下部分见下阶段。

### Phase E(余下)— collector + config + profile 编辑 + service wire — ✅ 完成(2026-07-01)

- 改动文件:
  - 编辑 `internal/config/config.go`:`ServerConfig` 加 `TLSFingerprintCollector` 字段 + `TLSFingerprintCollectorConfig` 结构(host/port/public_base_url/cert_file/key_file/session_ttl_seconds/max_records_per_session)+ 7 个 `viper.SetDefault`。**安全默认偏离 TR**:host=`127.0.0.1`(TR 为 `0.0.0.0`),且默认不自动启动(需 collector/start)——遵方案「默认安全」。
  - 新增 `internal/service/tls_fingerprint_collector_service.go`(841 行,TR 照抄+path swap):独立 HTTPS 监听、自签 CA、内存采集会话(TTL/最多 N 条)、Start/Stop/Status/CreateSession/ListCaptures/DeleteSession;**token 复用行为原样照搬**(captureTokenFromRequest 读 query/header/Authorization Bearer/X-Api-Key)。所有 helper(writeCollectorJSON/detectTLSFingerprintClientKind/tlsFingerprintProfileToYAML/summarize*/strconvItoa/hostFromURL 等)均在本文件内,YAML 为手工拼接(无外部 yaml 库)。
  - 新增 `internal/service/tls_fingerprint_collector_service_test.go`(TR 照抄):lifecycle+capture(loopback 实跑 HTTPS)、captureToken 解析、session limits、大 ClientHello。
  - 编辑 `internal/service/tls_fingerprint_profile_service.go`:加 `ResolveRoutableTLSProfileByID`(正数不存在→ok=false 回退;-1 随机;0 内置默认)、`ResolveTokenTLSProfileByID`(不依赖账号开关;-1/0/正数同义)。**仅加 2 方法,不动现有 `ResolveTLSProfile`**(fork 是内联写法,无需 TR 的 ResolveTLSProfileByID 重构;GetProfileByID/getRandomProfile fork 已有)。
  - 编辑 `internal/service/wire.go`:ProviderSet 加 `NewTLSFingerprintRouterService`、`NewTLSFingerprintCollectorService`(repo/cache 构造直接返回接口,无需 wire.Bind;cfg 已提供)。
- **推迟到 G**:`ProvideOpenAIGatewayTLSFingerprintRouterServices`(变参 provider)+ `wire.Bind(OpenAIOAuthTokenRouterReader/ProfileResolver)`——这些依赖 gateway service 消费 router service 及 OAuth-token 接口(fork 暂无这些接口),放 G 一并做,避免引用不存在的类型。
- 命令与结果(容器内):`gofmt -l` 空;`go build ./...` → **BUILD_OK**;`go vet ./internal/service/... ./internal/config/...` → **VET_OK**;`go test ./internal/service/ -run TLSFingerprint` → **ok**(router 6 子测 + collector lifecycle/token/limits/大 ClientHello 全 PASS)。
- 注:新 provider 暂未被消费、未重生成 wire_gen(build 不受影响);wire 重生成留待 F(handler 消费 collector/router)+ J。
- commit:`3161a1df`
- 遗留/风险:collector 真实抓包/JA3/对端验证属【运行时·人工】(只静态跑了 loopback 自测)。

### Phase F — handler + 路由 + wire — ✅ 完成(2026-07-01)

- 改动文件:
  - 新增 `internal/handler/admin/tls_fingerprint_router_handler.go`(照抄 TR:List/GetByID/Create/Update/Delete + DTO + `nullableInt64Patch` 区分 null/缺省)。`response.{Success,ErrorFrom,BadRequest,NotFound}` fork 均有。
  - 编辑 `internal/handler/admin/tls_fingerprint_profile_handler.go`:加 `net/http` import + `collector` 字段 + 构造函数改可变参数(`collectors ...`,使既有调用仍可编译)+ 6 个采集器方法(CollectorStatus/Start/Stop/CreateSession/ListCaptures/DeleteSession)。`response.Error` fork 有。
  - 编辑 `internal/server/routes/admin.go`:`/tls-fingerprint-profiles` 下加 `collector/{status,start,stop,sessions,sessions/:token/captures,DELETE sessions/:token}`;新增 `registerTLSFingerprintRouterRoutes`(5 CRUD)并在注册处调用。
  - 编辑 `internal/handler/handler.go`:`AdminHandlers` 加 `TLSFingerprintRouter` 字段。
  - 编辑 `internal/handler/wire.go`:`ProvideAdminHandlers` 加 router handler 参数 + 返回赋值;ProviderSet 把 `admin.NewTLSFingerprintProfileHandler` 换成 `ProvideTLSFingerprintProfileHandler`(注入 collector 的非变参包装)+ 加 `admin.NewTLSFingerprintRouterHandler`;新增 `ProvideTLSFingerprintProfileHandler` 包装函数。
  - 重生成 `cmd/server/wire_gen.go`(`go generate ./cmd/server`)。
- 采集器注入模式(照抄 TR):profile handler 构造用变参 `collectors ...`,wire 用非变参包装 `ProvideTLSFingerprintProfileHandler(profileService, collector)` 注入,既保留既有调用兼容又让 wire 直接解析 collector。
- 命令与结果(容器内):`go generate ./cmd/server` → wire 成功写 wire_gen.go(go.mod/go.sum **未污染**;wire_gen 含 2 个新 provider 引用);`gofmt -l`(我的文件)空(`auth_current_user_test.go` 是**既有**未格式化文件,非本次改动,未碰);`go build ./...` → **BUILD_OK**;`go vet ./internal/handler/... ./internal/server/...` → **VET_OK**;`go test ./internal/server/routes/...` **ok**;`./internal/service/ -run TLSFingerprint` **ok**;`./cmd/server/...`(含 wire_gen_test)**ok**。
- commit:`8f57d2c6`
- 遗留/风险:CRUD/采集器 API 端到端 + 多实例 pubsub 缓存失效属【运行时·人工】。变参 provider/wire.Bind(OAuth)仍待 G。

### Phase G1 — account getters — ✅ 完成(2026-07-01)

- 改动文件:`internal/service/account.go`
  - 加 `SupportsTLSFingerprint() bool`(Anthropic OAuth/SetupToken 或 OpenAI OAuth)。
  - 加 `GetTLSFingerprintRouterID() int64`(镜像 profile getter 的 float64/int64/int/json.Number 转换;0=未绑路由器)。
  - **改 `IsTLSFingerprintEnabled`** 的门控:`IsAnthropicOAuthOrSetupToken()` → `SupportsTLSFingerprint()`。**这是方案 §0「完整覆盖 OpenAI」所必需**(resolvers 都查 IsTLSFingerprintEnabled;不改则 OpenAI 账号无法启用 → 路由器对 OpenAI 失效),且与 TR 实现一致。**向后兼容**:Anthropic 行为完全不变(SupportsTLSFingerprint 对 Anthropic OAuth/SetupToken 恒为 true);OpenAI OAuth 仅在管理员显式置 `enable_tls_fingerprint` 时才生效,存量账号无此标志 → 行为不变。
- 命令与结果(容器内):`gofmt -l` 空;`go build ./...` → **BUILD_OK**;`go vet ./internal/service/...` → **VET_OK**;`go test -run "TLSFingerprint|Account"` → **ok**。无既有测试引用这些方法(不破坏)。
- commit:`21e18825`
- 遗留/风险:G2-G5 未做(gateway helper/struct/wire 变参、call-site 替换、UA/Originator 改写、OAuth token 路径)。

### Phase G3 — GatewayService 路由感知 TLS 解析 — ✅ 完成(2026-07-01)

- 架构核对(关键 · 与 TR 不同):fork 的 `ResolveTLSProfile`/`DoWithTLS` 调用点在**共享 `GatewayService`**(gateway_service.go)+ 两个 forward 文件;TR 则在 `OpenAIGatewayService`。故 helper+field 加在 `GatewayService`(fork 实际解析点),非照搬 TR 的归属。
- 全量核对解析点(grep 实证):gateway_service.go 5 处(4999 源 + 5615/9990/10017/10114 内联)+ forward_as_responses:128 + forward_as_chat_completions:129 —— **全部 enclosing func 都有 `c *gin.Context`**(Forward/forwardAnthropicAPIKeyPassthroughWithInput/ForwardCountTokens/forwardCountTokensAnthropicAPIKeyPassthrough/ForwardAsResponses/ForwardAsChatCompletions)。4999 的 `tlsProfile` 变量被 5048/5126/5167/5246 复用 → 只改 4999 源。**后台无 c 的调用点(account_test_service/account_usage_service/openai_apikey_responses_probe/upstream_models)保持原样**(probe 无入站 UA,改不改等价)。
- 改动文件:
  - `internal/service/gateway_service.go`:struct 加 `tlsFPRouterService` 字段;`NewGatewayService` 末尾加可变参 `tlsFPRouterServices ...` + 提取赋值;加 import `pkg/tlsfingerprint`;新增方法 `matchTLSFingerprintRouter(c,account)`(无 router/无 c → Matched=false)、`resolveTLSProfileForRequest(c,account)`(命中→ResolveRoutableTLSProfileByID;否则回落 ResolveTLSProfile);替换 5 处解析点(源 4999 + 4 处 DoWithTLS 内联)。**注意避开 helper 自身的 fallback 行,防自递归**。
  - `internal/service/gateway_forward_as_responses.go` / `gateway_forward_as_chat_completions.go`:各 1 处内联替换为 `s.resolveTLSProfileForRequest(c, account)`。
  - `internal/service/wire.go`:加 `ProvideTLSFingerprintRouterServices(x) []*TLSFingerprintRouterService`(单实例包 slice,供两个变参构造共享,避免两个同类型 provider 冲突)+ 入 ProviderSet。
  - 重生成 `cmd/server/wire_gen.go`:`v := ProvideTLSFingerprintRouterServices(...)`;`NewGatewayService(..., v...)`。
- 命令与结果(容器内):`go generate ./cmd/server` OK(go.mod/go.sum **未污染**);`gofmt -l` 空;`go build ./...` → **BUILD_OK**;`go vet ./internal/service/...` → **VET_OK**;`go test -run "TLSFingerprint|Gateway|Forward"` → **ok 30s**。
- 向后兼容:router_id=0 或无 router service → matchTLSFingerprintRouter 返回 Matched=false → 回落 ResolveTLSProfile,行为同现状。
- commit:`dedb31e6`
- 遗留/风险:G4(openai_gateway_service.go 的 UA/Originator 改写,buildUpstreamRequest)+ G5(OAuth token 路径)未做。HTTP 出站指纹/JA3 属【运行时·人工】。

## 待人工验证(运行时)

<!-- 无人值守做完后,把所有【运行时·人工】项列在此,供回来逐项验证 -->

- 待补:Phase C 迁移幂等(测试库)、G HTTP JA3、H WS 不串号、采集器抓取、OAuth 换 token 等(见方案 §5)。

## Blockers / 需人工决策

<!-- 任何"不猜不编"触发的停机点记在此:是什么、卡在哪、需要什么决策 -->

### Blocker #3 — ✅ 已解决(用户拍板「修全部 8 处」,commit `073be243`)(2026-07-01)

> **解决**:用户选「修全部 8 处」。已把 8 处 `Do`→`DoWithTLS(..., s.resolveOpenAITLSProfile(account, s.matchTLSFingerprintRouter(c, account)))`(与既有 :3247/:3537 逐字同款);排除 embeddings(TR 也裸 Do)/grok(非 OpenAI 账号)。容器内 gofmt 空 / `go build ./...` BUILD_OK / `go vet ./...` VET_OK(编译全部测试→所有 mock 已支持 DoWithTLS)/ `go test` service(OpenAI 网关全路径+TLS 36.8s)+ handler(全)全绿;diff 精确 8 文件各 +1/-1。**出站 JA3 属【运行时·人工】**。下方为原始诊断,保留供追溯。

**性质**:文档与代码不符——前一批账本称"native-OpenAI TLS ✅ 完成(解 Blocker #2)",但实证发现前一批只改了 `openai_gateway_service.go` 的 2 处(Forward:3247 / forwardOpenAIPassthrough:3537),**遗漏了另外 7-8 个 live OpenAI 出站路径**,它们仍走裸 `s.httpUpstream.Do(...)`,既不发账号级 TLS 指纹、也不受 TLS Router 影响。这与方案 §0「完整覆盖 OpenAI」目标不符。

**实证(TR 同名文件对照,TR 对应处均为 `DoWithTLS`)**:
| fork 文件:行 | enclosing(均 `*OpenAIGatewayService` 方法,均有 `c *gin.Context`) | TR 同名文件 |
|---|---|---|
| `openai_gateway_chat_completions.go:264` | `ForwardAsChatCompletions` | DoWithTLS |
| `openai_gateway_messages.go:297` | `ForwardAsAnthropic` | DoWithTLS |
| `openai_images.go:608` | `forwardOpenAIImagesAPIKey` | DoWithTLS |
| `openai_ws_http_bridge.go:215` | `proxyOpenAIWSHTTPBridgeTurn` | DoWithTLS |
| `openai_gateway_chat_completions_raw.go:174` | `forwardAsRawChatCompletions` | DoWithTLS |
| `openai_images_responses.go:1544` | `forwardOpenAIImagesOAuth` | DoWithTLS |
| `openai_gateway_responses_chat_fallback.go:145` | `forwardResponsesViaRawChatCompletions` | DoWithTLS |
| `openai_gateway_count_tokens.go:95` | `ForwardCountTokensAsAnthropic` | **TR 无此文件**(fork 特有,同语义账号出站) |

- **非漏(已核实一致)**:`openai_embeddings.go:89` TR 也是裸 `Do`(TR 未给 embeddings 上 TLS)→ fork 保持一致,不改;`openai_gateway_grok.go:59` 是 Grok 非 OpenAI 账号。

**可行性(已核实,同构不臆测)**:8 处 enclosing 全是 `*OpenAIGatewayService` 方法、全有 `c *gin.Context`;`matchTLSFingerprintRouter(c,account)`(openai_gateway_service.go:2564)+ `resolveOpenAITLSProfile(account, routerMatch...)`(:2577)前一批已加、就绪。修复=每处 `Do(upstreamReq, proxyURL, account.ID, account.Concurrency)` → `DoWithTLS(..., s.resolveOpenAITLSProfile(account, s.matchTLSFingerprintRouter(c, account)))`,与既有 :3247/:3537 逐字同款。**风险**:改 `Do`→`DoWithTLS` 后,跑到这些路径的测试其 `HTTPUpstream` mock 必须实现 `DoWithTLS`(前一批已给多数 OpenAI 测试 mock 加委托,但走到这 8 条路径的测试需逐一核实,同 Blocker #2 补充的教训)。

**为何停而不擅改**:① 契约本次范围=K part3 + G5 两次要项,修此属**扩大到前一批 G3 工作**;② 契约「文档与代码不符 → 记录 + 停」;③ 改 8 处热点 + 测试适配是一个完整子批次,范围决策应人工拍板。**已记录,待用户决定是否修**(修则照 TR 同构改 8 处〔或仅 7 处有对照者〕+ 测试适配 + 容器内 build/vet/test 全绿 + commit)。

### Blocker #4 — ✅ 已解决(2026-07-01 收尾批次 = A1;后端 `13e0c929` + 前端 `ad4df180`)——OAuth 换 token/refresh 请求本身不传 router

- **现状**:G5 service 底座已就绪且**已存在账号的自动刷新(`RefreshAccountToken` → 读 `account.GetTLSFingerprintRouterID()`)已按账号 router 应用专用 UA/指纹**。缺的是「用授权码换 token / 手动 refresh 端点」那一次请求携带 router:TR 在 handler DTO(`OpenAIExchangeCodeRequest`/`OpenAIRefreshTokenRequest` 的 `tls_fingerprint_router_id` 字段)+ 前端(`useOpenAIOAuth.ts`/`accounts.ts` 的 payload + Create/Edit/ReAuth modal 调用点)整条链传入;fork 这条链未接(`ExchangeCode` 恒 nil、`RefreshToken` 走无 router 的旧方法、`RefreshTokenWithClientIDAndRouter` 未实现)。
- **影响**:小。账号级绑定(K part2 经账号表单写 `extra.tls_fingerprint_router_id`)+ 自动刷新已覆盖主要场景;缺口仅在「首次建号换 token」「手动 refresh 端点」这两次一次性请求的握手指纹。
- **待用户决策**:是否补(后端 handler DTO + service `RefreshTokenWithClientIDAndRouter` + 前端 useOpenAIOAuth/accounts.ts/3 modal 接线)。

### 自查小项(非阻塞,记录供参考)
- **有意跳过(已在 K part3 记录,非遗漏)**:profile 列表行「Copy as YAML」按钮 + `buildProfileYaml`/`handleCopyYaml`(TR ProfilesModal:298-306);采集器面板内复制 YAML 仍在。
- **前端 `api/admin/index.ts`**:未再导出 3 个采集器类型(`TLSFingerprintCollectorStatus/Session/CaptureRecord`)。**零功能影响**(ProfilesModal 直接从模块导入),仅聚合导出不全;可顺手补。
- **`BulkEditAccountModal.vue`**:fork 无任何 TLS 绑定(profile 也无)——与 fork **既有** profile 处理一致(fork 该文件本就不含 TLS),非本 port 回归;TR 有 router+profile 批量绑定。
- **账号表单 i18n `routerHint`**:fork 缺该提示文案(有 `router`/`noRouter`,下拉可用)。
- **测试**:fork 缺 `tls_fingerprint_profile_service_test.go`(TR 有);`resolveChatGPTOAuthTokenRequestOptions` 无直接单测。

### Blocker #1 — ✅ 已解决(方案 A,2026-07-01)— base 分支编译不过:`330c35c1` 合并误删 `rejectGrokUnsupportedEndpoint` 定义

> **解决**:人工拍板方案 A,已在本分支 baseline-fix commit 修复,基线 `go build ./...`/`vet`/`routes test` 全绿。详见上方「Phase 0 (续)」。下方为原始诊断,保留供追溯。

**现象**:`go build ./...` 报 `internal/server/routes/gateway.go:187 & :204: undefined: rejectGrokUnsupportedEndpoint`(调用存在、定义缺失)。

**根因(已实证)**:
- `rejectGrokUnsupportedEndpoint` 原是 `RegisterGatewayRoutes` 内的**局部闭包**。
- main 的 `4a7148e2 "fix: support grok cli compatibility routes"`(Heatherm Huang,06-29)**故意删除**了该闭包及其全部调用点(净 -40 行),改走 `isOpenAIResponsesCompatibleGatewayPlatform` 路由 + `sanitizeGrokResponsesUnsupportedFields`——目的就是让 Grok CLI 能用这些端点。
- `feat/req-resp-archive`(LeGo,合并前 `70588e1e`)仍保留 Grok 拒绝(闭包 + 调用)。
- 合并 `330c35c1 "Merge branch 'main' into feat/req-resp-archive"`(LeGo,07-01 00:24)**冲突解决出错**:保留了 2 处调用(`gateway.go:186-189`、`202-205`,空格缩进=手工冲突残留),却采纳了 main 对定义区的删除 → 调用无定义。
- blame 实证:186-189 行归属 `330c35c1`(空格缩进),周围为 tab。

**影响面**:`feat/req-resp-archive` HEAD **同样破损**(调用在 187/204、无定义)——即父分支也编译不过(记忆称其"已部署",推测部署版本早于 07-01 此次合并)。`main` 无此问题。本 TLS 分支的 2 个 commit 仅 docs,未引入此错。

**为何停而不修(契约)**:修复方向是**用户可见的行为决策**,不可无人值守臆断:
- **方案 A(推荐)— 删除 2 处孤儿调用块**(各 3 行 `if getGroupPlatform(c)==service.PlatformGrok { rejectGrokUnsupportedEndpoint(...); return }`,保留 `archiveCapture` 中间件与其余逻辑)。结果:这两个端点对 Grok 组**不再路由层拒绝**,与 `main` 完全一致。**理由**:`4a7148e2` 标题与改动证明删除是为"支持 Grok CLI 兼容路由";恢复拒绝(方案 B)等于回退该兼容修复。这也正是"merge main into archive"应得的结果(取 main 改动 + 保留 archive 新增的 archiveCapture)。
- **方案 B — 恢复闭包定义**(从 `4a7148e2^:gateway.go` 可取回原文,非 fabricate)。结果:保持 Grok 在这两个端点被拒绝(archive 分支合并前的行为),但**与 main 的 Grok-CLI 兼容意图冲突**,可能再次破坏 Grok CLI。

**精确位置(当前 worktree `gateway.go`)**:
```
185  r.GET("/responses", ...archiveCapture..., func(c *gin.Context) {
186    if getGroupPlatform(c) == service.PlatformGrok {        ← 方案A删此3行
187      rejectGrokUnsupportedEndpoint(c, "Responses WebSocket API")
188      return
189    }
190    h.OpenAIGateway.ResponsesWebSocket(c)
191  })
...
201  r.POST("/chat/completions", ...archiveCapture..., func(c *gin.Context) {
202    if getGroupPlatform(c) == service.PlatformGrok {        ← 方案A删此3行
203      rejectGrokUnsupportedEndpoint(c, "Chat Completions API")
204      return
205    }
206    （空行）
207    if isOpenAIResponsesCompatibleGatewayPlatform(c) { ... }
```
> 旁注:这两处的空格缩进与文件其余 tab 不一致,建议修复时一并改回 tab(`gofmt`)。

**需人工决策**:选 A 还是 B?并决定**在哪修**——(i) 在本 TLS 分支修(作为独立的 baseline-fix commit,我可代劳),还是 (ii) 在源头 `feat/req-resp-archive` 修好再让本分支 rebase/merge(更干净,避免两分支各修一次)。

**残余不确定**:`go build ./...` 在 routes 包即失败,**其后的包是否还有别的编译错误尚未可知**;修好本处后需重跑全量 build 才能确认基线真绿。

**续作路径**:人工给出 A/B 决策并使基线 `go build ./...` 全绿后,我从 Phase A 按 §8 顺序续作;本账本 Phase 0 行改 ✅,继续逐 Phase 记录。

---

### Blocker #2 — ⚠️ 方案 G 与代码不符:**native OpenAI HTTP 路径不走 DoWithTLS,G3 未覆盖**(2026-07-01,实证发现)

**这是方案的一处实质性缺口,影响"完整覆盖 OpenAI"目标,需人工确认后再续作 G4/native-OpenAI。**

**实证**:
- fork 的 **native OpenAI 转发路径**(`OpenAIGatewayService.Forward` @ openai_gateway_service.go:2550、`forwardOpenAIPassthrough` @ 3323)出站用 `s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)`(:3187、:3477)——**完全不带 TLS profile**。
- 方案 §3/§G3 列的解析点全部在 `GatewayService` + forward 文件,是基于 grep `ResolveTLSProfile` 得来;而 native OpenAI 路径根本没调 `ResolveTLSProfile`(用裸 `Do`),故**既未被方案列出,也未被我 G3 覆盖**。
- **TR 对照**:TR 的同名路径用 `s.httpUpstream.DoWithTLS(..., s.resolveOpenAITLSProfile(account, tlsRouterMatch))`(TR openai_gateway_service.go:3454、:3761)。TR 的 `OpenAIGatewayService` 同时持有 `tlsFPProfileService`(:481)与 `tlsFPRouterService`(:482)。

**结论**:G1+G3 已覆盖 **Anthropic 原生 + Anthropic→OpenAI/responses 转换转发**两类 HTTP 路径(经 `GatewayService`);但**以 OpenAI 格式打到 OpenAI 账号的 native 路径(OpenAI 覆盖的主用例)仍未应用 TLS 指纹**。要达成 §0「完整覆盖 OpenAI」,必须给 native 路径补 TLS 集成。

**续作所需(faithful 参照 TR,scope-guard:只移 TLS+UA,不带 codex policy / user-prompt-replacement / ingress-session)**:
1. `OpenAIGatewayService` 加 **两个**字段:`tlsFPProfileService *TLSFingerprintProfileService`(当前**没有**)+ `tlsFPRouterService *TLSFingerprintRouterService`。
2. `NewOpenAIGatewayService` 加 `tlsFPProfileService`(必填参)+ `tlsFPRouterServices ...*TLSFingerprintRouterService`(可变参,复用已建的 `ProvideTLSFingerprintRouterServices` slice provider)。**注意**:加必填参 `tlsFPProfileService` 会破坏约 5 处非 wire 调用方(多为测试),需一并补参。
3. 加 helper:`matchTLSFingerprintRouter(c,account)`(同 GatewayService 版)+ `resolveOpenAITLSProfile(account, routerMatch ...TLSFingerprintRouterMatchResult) *tlsfingerprint.Profile`(照抄 TR @ :621:命中→ResolveRoutableTLSProfileByID,否则 ResolveTLSProfile)。
4. `Forward`(2550)、`forwardOpenAIPassthrough`(3323):各 `m := s.matchTLSFingerprintRouter(c, account)` 一次,把 `Do(...)`(:3187、:3477)改成 `DoWithTLS(..., s.resolveOpenAITLSProfile(account, m))`。
5. **G4 UA/Originator**:`buildUpstreamRequest`(4377;UA~4461 originator~4438)、`buildUpstreamRequestOpenAIPassthrough`(3587;UA~3678 originator~3660):就地 `m := s.matchTLSFingerprintRouter(c, account)`,`m.Matched && m.UpstreamUserAgent!="" ` 覆盖 UA、`m.UpstreamOriginator!=""` 覆盖 originator(优先级高于账号默认/resolveOpenAIUpstreamOriginator)。
6. wire:`NewOpenAIGatewayService` 变参注入 + 必填 `tlsFPProfileService`;重生成 wire_gen;补测试调用方参数。
7. **G5(OAuth token)**:`openai_oauth_service.go` 换 token 用 router 的 `ChatGPTOAuthTokenUserAgent`+`ChatGPTOAuthTokenTLSFingerprintProfileID`(经 `GetRuntimeRouter`/`ResolveTokenTLSProfileByID`)。需 `wire.Bind` OAuth-token reader/profile resolver 接口——**fork 当前无 `OpenAIOAuthTokenRouterReader`/`OpenAIOAuthTokenProfileResolver` 接口**,照搬要新建,属 §G5「若不臆测无法修复则回滚 G5 并停」范畴,建议人工评估。Codex invite-reset:fork 经查**无** `codex_invite_reset_service.go`,按方案跳过。

**为何停**:① 这是「文档与代码不符」(契约明令记录+停);② native 集成在最热文件 `openai_gateway_service.go`,需加 2 个服务依赖 + 改 4 处热点 + 动约 5 个测试调用方,且出站指纹正确性本就属【运行时·人工】;③ 方案 G3 的前提(覆盖 OpenAI)不成立,人工应先知悉再决定 native 集成的取舍/范围。**不臆测、不硬凑热点路径**。

**补充(2026-07-01,试做后回滚的实证)**:曾按上述清单在 `openai_gateway_service.go` 内完成源改(struct 2 字段 + 构造变参 + `matchTLSFingerprintRouter`/`resolveOpenAITLSProfile` 两 helper + 两处 `Do`→`DoWithTLS(..., resolveOpenAITLSProfile(account, matchTLSFingerprintRouter(c,account)))` @ Forward:3226/forwardOpenAIPassthrough:3516),**源改本身干净可行**。但 `NewOpenAIGatewayService` 加必填参 `tlsFPProfileService` 会波及 **5 个测试调用方**(`openai_gateway_handler_test.go`×2、`openai_images_failover_test.go`、`openai_ws_protocol_forward_test.go`、`openai_gateway_record_usage_test.go`,均需补一个 `nil` 实参),且 `Do`→`DoWithTLS` 后,**跑到 Forward/forwardOpenAIPassthrough 的测试其 `HTTPUpstream` mock 必须实现 `DoWithTLS`**——已确认 `openai_ws_protocol_forward_test.go` 的 mock `DoWithTLS` 委托给 `Do`(✅ 可过),但 handler / record_usage 测试的 mock 与断言(是否区分 Do/DoWithTLS 调用)**未逐一核实**。**无法在不逐测核实的前提下保证全绿 → 已 `git checkout` 回滚该文件(工作树干净,build 仍绿)**。续作者:先给这些 mock 补 `DoWithTLS`→`Do` 委托,再逐测跑 `go test ./internal/handler/... ./internal/service/ -run "OpenAIGateway|Forward|Images|RecordUsage"` 确认,方可提交。

### Phase G(native-OpenAI TLS)— 解 Blocker #2 — ✅ 完成(2026-07-01)

- 背景:Blocker #2 发现 native OpenAI 路径(`OpenAIGatewayService.Forward`/`forwardOpenAIPassthrough`)走裸 `Do` 无 TLS。本次照 TR 补齐,并把之前受阻的测试面一并做绿。
- 改动文件:
  - `internal/service/openai_gateway_service.go`:struct 加 `tlsFPProfileService`+`tlsFPRouterService` 两字段;`NewOpenAIGatewayService` 加必填参 `tlsFPProfileService` + 可变参 `tlsFPRouterServices ...`(复用 `ProvideTLSFingerprintRouterServices` slice provider);import `pkg/tlsfingerprint`;新增 `matchTLSFingerprintRouter`+`resolveOpenAITLSProfile`(照 TR:命中→ResolveRoutableTLSProfileByID,否则 ResolveTLSProfile;service/profileService 为 nil 时安全返回 nil);两处 `Do`→`DoWithTLS(..., resolveOpenAITLSProfile(account, matchTLSFingerprintRouter(c,account)))`(Forward + forwardOpenAIPassthrough)。
  - 5 个测试调用方补 `nil`(tlsFPProfileService)实参:`openai_gateway_handler_test.go`×2、`openai_images_failover_test.go`、`openai_ws_protocol_forward_test.go`、`openai_gateway_record_usage_test.go`(perl 按行号插入,防 anchor 歧义)。
  - 重生成 `cmd/server/wire_gen.go`(`NewOpenAIGatewayService(..., v...)`,tlsFPProfileService 由 NewTLSFingerprintProfileService 提供)。
- **测试面核实(之前受阻点已解)**:改 `Do`→`DoWithTLS` 后,跑到 Forward/forwardOpenAIPassthrough 的测试其 mock 均正常——`openai_ws_protocol_forward` mock 的 DoWithTLS 委托给 Do;handler 测试 httpUpstream 传 nil 但未走到该 site;images 测试走 images 路径不受影响。**全部 OpenAI/handler 测试实跑通过**。
- 命令与结果(容器内):`go generate ./cmd/server` OK(go.mod/go.sum **未污染**;wire_gen `v...`);`gofmt -l` 空;`go build ./...` → **BUILD_OK**;`go vet ./internal/service/... ./internal/handler/...` → **VET_OK**;`go test ./internal/service/ -run "OpenAI|TLSFingerprint|Gateway|Forward|RecordUsage"` → **ok 37s**;`go test ./internal/handler/ -run "OpenAI|Images|Gateway"` → **ok**。
- commit:`727f3bb7`
- 遗留/风险:native OpenAI 出站现按 router/账号解析 TLS profile(向后兼容:router_id=0/无 service → resolveOpenAITLSProfile 回落 ResolveTLSProfile,OpenAI APIKey 账号 IsTLSFingerprintEnabled=false → nil,行为同现状)。**G4 UA/Originator 改写未做**(下一步);出站指纹/JA3 属【运行时·人工】。

### Phase G4 — OpenAI UA/Originator 改写 — ✅ 完成(2026-07-01)

- 改动文件:`internal/service/openai_gateway_service.go`(仅此一处,无签名/wire/测试改动)。
  - `buildUpstreamRequest`(native)与 `buildUpstreamRequestOpenAIPassthrough`(透传):在各自 UA/originator 逻辑末尾(`overrideBrowserUserAgent` 之后、content-type 兜底之前)加:`if m := s.matchTLSFingerprintRouter(c, account); m.Matched { m.UpstreamUserAgent!=""→set user-agent; m.UpstreamOriginator!=""→set originator }`。
- 设计选择:采**方案 G4 的就地 match + 最小内联覆盖**(非 TR 的 `applyOpenAIUpstreamUserAgent`/`resolveOpenAIUpstreamOriginator` 大重构,避免带入 codex policy 纠缠)。放最末=命中且规则给值时**优先级最高**(压过账号自定义 UA / ForceCodexCLI / resolveOpenAIUpstreamOriginator / 浏览器兜底),确保出站 UA 与所选 TLS 指纹一致;规则未给值(空)则完全不动既有逻辑(向后兼容)。match 与 TLS 解析读同一 UA header + 同一缓存 router,结果一致。
- 命令与结果(容器内):`gofmt -l` 空;`go build ./...` → **BUILD_OK**;`go vet ./internal/service/...` → **VET_OK**;`go test ./internal/service/ -run "OpenAI|TLSFingerprint|Gateway|Forward"` → **ok 37.9s**。
- commit:`ae324f7a`
- 遗留/风险:仅剩 G5(OAuth token 路径,需新建接口,见 Blocker #2 第 7 点)。UA/指纹一致性属【运行时·人工】抓包复核。

### Phase G5 — OAuth token 专用 UA/profile — ⏸️ 主动推迟(2026-07-01,方案允许)

- 方案 §G5 明示:「G5 非 OpenAI 主路径,可后补」;若 wire 无法不臆测修复则回滚+记录+停。
- 实证 TR 做法:`OpenAIOAuthService` 加 `tlsFPRouterReader OpenAIOAuthTokenRouterReader` + `tlsFPProfileResolver OpenAIOAuthTokenProfileResolver` 两接口(setter `SetTokenTLSRouterDeps` 注入,非构造)+ `resolveChatGPTOAuthTokenRequestOptions` 生成 `OpenAIOAuthTokenRequestOptions{UserAgent, AccountID, AccountConcurrency, TLSProfile}`,再**把该 options 变参穿过 `ExchangeCode`/`RefreshToken` 传入 `OpenAIOAuthClient`,由客户端对 token HTTP 请求应用 UA + `DoWithTLS(profile)`**。
- 为何推迟(非硬阻塞,是范围/风险权衡):
  1. 需改 **`OpenAIOAuthClient` 接口 + 其 HTTP 实现**接收 options 并应用 UA/TLS——这是 token 交换的底层 HTTP 层,**fork 的 oauthClient 结构与 TR 不同**,照搬需逐一适配,存在臆测风险。
  2. 属**次要路径**(token 交换/刷新,非主 API 请求路径;主路径 G1-G4 已完成生效)。
- 续作清单(不臆测的前提下):① 在 service 包定义两接口(`GetRuntimeRouter(int64) *model.TLSFingerprintRouter` / `ResolveTokenTLSProfileByID(int64)(*tlsfingerprint.Profile,bool)`——实现已在 router/profile service 上现成);② `OpenAIOAuthService` 加 2 字段 + `SetTokenTLSRouterDeps` setter(在 `ProvideOpenAIOAuthService` wrapper 里调 setter 注入,避免构造签名变更 + 免 wire.Bind);③ 加 `OpenAIOAuthTokenRequestOptions` + `resolveChatGPTOAuthTokenRequestOptions`;④ **核对 fork `OpenAIOAuthClient` 接口能否接 options 应用 UA/DoWithTLS**——这步是关键,若 fork 客户端无对应能力需先加(此处最易触发臆测,务必对照 fork 现状);⑤ 调用点(ExchangeCode/RefreshToken/RefreshAccountToken)传 options;⑥ build/vet/test。Codex invite-reset:fork **无** `codex_invite_reset_service.go`,按方案跳过。

### Phase G5 — OAuth token 专用 UA/profile — ✅ 完成(build/vet/test 绿)(2026-07-01)

- **关键实证(解除推迟顾虑)**:fork 的 OAuth 客户端(`repository/openai_oauth_service.go`)**恰是 TR 客户端去掉 G5 的版本**——两者都用 `imroc/req/v3` 的 `getSharedReqClient`、`shouldReturnOpenAINoProxyHint`/`newOpenAINoProxyHintError` 逐字一致、同样的兜底 UA `codex-cli/0.91.0`。故「OpenAIOAuthClient 接口+HTTP 实现接 options 应用 UA/DoWithTLS」是**faithful 移植而非臆造**。逐项核对 fork 现状均满足(**无一处臆测**):
  - `service.HTTPUpstream.DoWithTLS(req, proxyURL, accountID, concurrency, profile)` 签名与 TR 用法**逐字一致**;
  - `service.WithHTTPUpstreamProfile` / `service.HTTPUpstreamProfileOpenAI` 在 fork 已导出存在(openai_images/openai_gateway_* 在用);
  - `SettingService.GetOpenAICodexUserAgent(ctx)` 存在(setting_service.go:1092);
  - `NewHTTPUpstream(cfg) service.HTTPUpstream` 直接返回接口(**无需 wire.Bind**),且 `NewHTTPUpstream` 已在 repository ProviderSet → 客户端可消费;
  - `TLSFingerprintRouterService.GetRuntimeRouter(int64) *model.TLSFingerprintRouter` / `TLSFingerprintProfileService.ResolveTokenTLSProfileByID(int64)(*tlsfingerprint.Profile,bool)` 与两接口签名**逐字一致**(concrete 直接满足 interface,setter 注入免 wire.Bind);
  - `model.TLSFingerprintRouter` 有 `Enabled`/`ChatGPTOAuthTokenUserAgent`/`ChatGPTOAuthTokenTLSFingerprintProfileID *int64`;`Account.ID int64`/`Account.Concurrency int`/`GetTLSFingerprintRouterID()`(G1)均在;service 包已 import model+tlsfingerprint(无新环);`valueFromInt64Ptr` 无重名。
- 改动文件:
  - `internal/service/oauth_service.go`:加 3 类型/接口(`OpenAIOAuthTokenRequestOptions`{UserAgent/TLSProfile/AccountID/AccountConcurrency}、`OpenAIOAuthTokenRouterReader`.GetRuntimeRouter、`OpenAIOAuthTokenProfileResolver`.ResolveTokenTLSProfileByID);import model+tlsfingerprint;`OpenAIOAuthClient` 三方法加 `options ...OpenAIOAuthTokenRequestOptions` 变参(变参=对既有调用向后兼容)。
  - `internal/repository/openai_oauth_service.go`:`NewOpenAIOAuthClient(httpUpstream)` + struct 加 `httpUpstream` 字段;三方法加变参;把内联 POST 抽成 `postTokenForm`(无 TLS:原 req/v3 路径 + `resolveOpenAIOAuthTokenUserAgent(option)` 取 UA)+ `postTokenFormWithTLS`(`http.NewRequestWithContext` + `WithHTTPUpstreamProfile(OpenAI)` + `httpUpstream.DoWithTLS(..., option.TLSProfile)` + 手动读 body/json 解析);加 `firstOpenAIOAuthTokenRequestOption`;import encoding/json+io。保留 fork 既有中文注释(fork 后端约定)。
  - `internal/service/openai_oauth_service.go`:struct 加 `settingService`/`tlsFPRouterReader`/`tlsFPProfileResolver`;`SetTokenTLSRouterDeps` setter;`OpenAIExchangeCodeInput` 加 `TLSFingerprintRouterID *int64`;`ExchangeCode` 解析 options 并 `ExchangeCode(..., tokenOptions...)`;`RefreshTokenWithClientID` 拆出私有 `refreshTokenWithClientID(..., routerID, account)`;`resolveChatGPTOAuthTokenRequestOptions`(命中 enabled router → tokenUA/tokenProfileID,UA 空则回落 codex UA,account 填 ID/并发,profileID→ResolveTokenTLSProfileByID;全空→nil)+ `valueFromInt64Ptr`;`RefreshAccountToken` 改走 `refreshTokenWithClientID(..., account.GetTLSFingerprintRouterID(), account)`。
  - `internal/service/wire.go`:`ProvideOpenAIOAuthService` 加 3 参(settingService/tlsFPRouterService/tlsFPProfileService)+ 末尾 `svc.SetTokenTLSRouterDeps(...)`(镜像既有 SetPrivacyClientFactory 注入模式)。
  - 3 个测试 stub(state/refresh/auth_url)各 3 方法补变参;`repository/openai_oauth_service_test.go` 的 `NewOpenAIOAuthClient()`→`NewOpenAIOAuthClient(nil)`。
  - 重生成 `cmd/server/wire_gen.go`:`NewOpenAIOAuthClient(httpUpstream)` + `ProvideOpenAIOAuthService(..., settingService, tlsFingerprintRouterService, tlsFingerprintProfileService)`——**无 wire 环**(两 service 早已被 gateway/openai 构造,wire 复用);go.mod/go.sum **未污染**。
- **范围守卫(faithful,不越界)**:**未**移 TR 的 `RefreshTokenWithClientIDAndRouter`(TR 唯一调用方是其 handler 用 `req.TLSFingerprintRouterID`,而 fork 该 handler DTO **无**此字段;续作清单⑤只列 ExchangeCode/RefreshToken/RefreshAccountToken);ExchangeCode 的 `TLSFingerprintRouterID` 字段已加但 handler 暂不填(nil→no-op,向后兼容,待日后 DTO/前端补)。Codex invite-reset 按方案跳过(fork 无该 service)。
- 命令与结果(容器内 golang:1.26.4-alpine):`gofmt -l`(8 改动文件)空;`go generate ./cmd/server`(wire)OK;`go build ./...` → **BUILD_OK**;`go vet ./...` → **VET_OK**(编译全部含测试→证明所有 mock/调用方一致);`go test ./internal/repository/... ./cmd/server/...` → **ok**;`go test ./internal/service/ -run "OpenAIOAuth|OAuth|TLSFingerprint|Refresh|Exchange|Passthrough|Codex"` → **ok 11.7s**;`go test ./internal/handler/...` → **ok**(admin openai_oauth handler 等全过);`go test ./internal/service/ -run "TokenRefresh|TokenProvider|RefreshAccount|OpenAIToken"` → **ok**。
- commit:`162b8377`
- 遗留/风险:**OAuth 换 token 的真实出站(JA3 指纹 + 专用 UA)属【运行时·人工】**抓包验证(静态不可证,未宣称已验证)。向后兼容:router_id=0 / router 未 enabled / 未设 token UA+profile → resolveChatGPTOAuthTokenRequestOptions 返回 nil → 客户端用默认 UA、无 TLS profile,行为同现状(无 TLS 路径仍走原 req/v3 shared client)。ExchangeCode 的 router 绑定当前恒 nil(handler 未填),仅 RefreshAccountToken 路径实际生效。

### Phase H — OpenAI WS 集成 — ✅ 完成(静态)(2026-07-01)

- 按下方 TR 清单整体实现,一次做完,build/vet/ws-test 全绿。**核心防串号:连接池每一处取连接点都加了 `matchesTLSProfile` 过滤**(见下),不同指纹连接不复用。
- 改动文件(源):
  - `openai_gateway_service.go`:新增 `resolveOpenAIWSTLSProfile(account, routerMatch...)`(resolveOpenAITLSProfile + HTTP1OnlyProfile + scope key:tls-router-{id}-{pid}/tls-router-random/tls-random/CacheKey)。
  - `openai_ws_client.go`:`openAIWSClientDialer.Dial` 接口加 `profile` 参;`coderOpenAIWSClientDialer.Dial` 有 profile 时也建自定义 client;`proxyHTTPClient(proxy, profile)` 缓存键 `proxy+"|tls:"+CacheKey(profile)`;新增 `buildOpenAIWSHTTPTransport`(强制 HTTP/1.1 + 按无代理/SOCKS5/HTTP 设 DialTLSContext);import crypto/tls + tlsfingerprint。
  - `openai_ws_pool.go`:`openAIWSAcquireRequest` 加 `TLSProfile`+`TLSProfileKey`;`openAIWSConn` 加 `tlsProfileKey`;`newOpenAIWSConn` 收 profile+key;新增 `matchesTLSProfile`+`openAIWSTLSProfileKey`+`pickOldestIdleMismatchedTLSConnLocked`;`pickLeastBusyConnLocked` 加 profile/key 参 + 过滤;**acquire 全部取连接点过滤**:forcePreferred(matchesTLSProfile 否则 unavailable)、preferred 复用(&& matchesTLSProfile)、pickLeastBusy(2 调用点传 profile/key)、fallback loop(!matchesTLSProfile→continue);容量满时新增淘汰指纹不匹配空闲连接;`dialConn` 传 profile 给 Dial + newOpenAIWSConn。clone 用结构体拷贝自动带新字段。
  - `openai_ws_forwarder.go`:两处 acquire(forwardOpenAIWSV2 + 另一处)前 `resolveOpenAIWSTLSProfile(account, matchTLSFingerprintRouter(c,account))` 写入 req;`buildOpenAIWSHeaders` 末尾加 UA/Originator router 覆写(同 G4)。
  - `openai_ws_v2_passthrough_adapter.go`:直连 passthrough dial 也传 wsTLSProfile。
- 改动文件(测试适配):`Dial` 签名变更波及 6 个 dialer mock(加 `_ *tlsfingerprint.Profile` 参)+ `newOpenAIWSConn` ~45 处调用(加 `nil, ""`)+ `proxyHTTPClient` 14 处调用(加 `nil`)+ 3 个测试文件补 tlsfingerprint import;`ProxyClientCacheIdleTTL` 测试改用新缓存键 `proxy+"|tls:"+CacheKey(nil)` 查表。
- 命令与结果(容器内):`gofmt -l` 空;`go build ./...` → **BUILD_OK**;`go vet ./...` → 无报错;`go test ./internal/service/ -run "OpenAIWS|WS|OpenAI|Forward|TLSFingerprint|FastPolicy|Dialer|Pool"` → **ok ~34s**;go.mod/go.sum 未污染。
- commit:`ef966a8f`
- 遗留/风险:**不同指纹连接不串号 / WS wss 握手 JA3 / 真实 Codex WS 流量 属【运行时·人工】抓包验证**(方案 §5/§6),静态不可证,**未宣称已验证**。向后兼容:router_id=0/无指纹 → resolveOpenAIWSTLSProfile 返回 (nil,"") → 连接池键回落 CacheKey(nil),行为同现状。

<details><summary>(原「记录待续」清单,保留供追溯)</summary>

### Phase H(原记录)— OpenAI WS 集成 — ⏸️ 曾记录待续,现已实现

- **为何未做**:H 是全案最深最险的一步,其**核心正确性(不同指纹连接不串号)只能人工抓包/JA3 验证**(方案 §Phase H/§6 明示「务必抓包验证」「运行时留人工」);且**半成品有害**——若给 WS dialer 传了 profile 却没把连接池复用键纳入指纹,不同指纹会复用同一连接=串号,比不做更糟。故 H 必须**整体一次做完**,不宜在预算紧张时草率半做。**不臆测、不硬凑连接池核心逻辑**。
- **TR 实证的完整改动清单(照抄参照,scope-guard:只移 TLS 符号,勿带 user-prompt-replacement/ingress-session/hook 签名)**:
  1. `openai_gateway_service.go`:新增 `resolveOpenAIWSTLSProfile(account, routerMatch...) (*tlsfingerprint.Profile, string)`(TR @ :634)= `resolveOpenAITLSProfile` + `tlsfingerprint.HTTP1OnlyProfile(...)`(WS 强制 HTTP/1.1),返回 profile + scope key(`tls-router-{routerID}-{profileID}` / `tls-random`(profileID==-1)/ 空→`CacheKey`)。
  2. `openai_ws_client.go`:① `openAIWSClientDialer.Dial` 接口签名末尾加 `profile *tlsfingerprint.Profile`;② `coderOpenAIWSClientDialer.Dial`:`profile != nil` 时也建自定义 `http.Client` 设 `opts.HTTPClient`(现仅代理场景建);③ 新增 `buildOpenAIWSHTTPTransport(proxy, parsedURL, profile)`:`HTTP1OnlyProfile` 后按 无代理/SOCKS5/HTTP 选 `NewDialer`/`NewSOCKS5ProxyDialer`/`NewHTTPProxyDialer` 设 `DialTLSContext`;④ `proxyHTTPClient` 缓存 key 改 `proxy + "|tls:" + CacheKey(profile)`。
  3. `openai_ws_pool.go`(**核心防串号**):① `openAIWSAcquireRequest` 加 `TLSProfile *tlsfingerprint.Profile` + `TLSProfileKey string`(TR @ :68-69);② `openAIWSConn` 加 `tlsProfileKey` 字段(:231),`newOpenAIWSConn` 收 profile+key 并存 `openAIWSTLSProfileKey(profile,key)`(:246/252);③ 新增 `openAIWSTLSProfileKey`(:527)+ `(*openAIWSConn).matchesTLSProfile`(:520);④ **所有取连接处按 `matchesTLSProfile` 过滤**:`pickLeastBusyConnLocked` 加 profile/key 参(:1270,过滤 :1277/1290)及其全部调用方(:945/1054);preferredConn 校验(:842/923);⑤ 新增 `pickOldestIdleMismatchedTLSConnLocked`(:1127)在容量满时优先淘汰指纹不匹配的空闲连接(:1004);⑥ `dialConn` 的 `clientDialer.Dial(...)` 传 `req.TLSProfile`(:1545),`newOpenAIWSConn(..., req.TLSProfile, req.TLSProfileKey)`(:1561);⑦ `cloneOpenAIWSAcquireRequest`(fork :1665)复制两新字段。
  4. `openai_ws_forwarder.go`:两处 acquire 构造(fork :1906 / :2959)前 `tlsProfile, tlsProfileKey := s.resolveOpenAIWSTLSProfile(account, s.matchTLSFingerprintRouter(c, account))` 写入 `openAIWSAcquireRequest.TLSProfile/TLSProfileKey`;UA/originator(:1156/:1172)按 routerMatch 覆写(同 G4)。
- **验证要求**:build+vet+`go test ./internal/service/ -run "OpenAIWS|WS|Forward"` 全绿(静态);**串号/指纹须人工抓包**,不得宣称已验证。
- **续作提示**:fork 的连接池 picking 逻辑与 TR 可能不完全同构,逐点对照 fork 现有 `acquire`/`pickLeastBusyConnLocked`/account pool 结构再改;`matchesTLSProfile` 过滤**务必覆盖每一处取连接点**,漏一处即串号。建议整段单独一个批次、full budget 做。

</details>

### Phase G(变参 provider 复用说明)
G3 建的 `ProvideTLSFingerprintRouterServices`([]*T slice provider)已被 GatewayService 与 OpenAIGatewayService 两个变参构造共享(wire 各生成 `v...`),无重复 provider 冲突。H 已复用 router service。

### Phase I — cmd/server 采集器优雅关闭 — ✅ 完成(2026-07-01)

- 说明:先于 H 做——I 只依赖 collector service(E 已完成),与 H(WS)无依赖;H 已记录待续,故此处做独立且安全的 I,补全采集器生命周期(经 admin API 可 Start/Stop,I 补进程退出时优雅 Stop)。
- 改动文件:
  - `cmd/server/wire.go`:`provideCleanup` 加参 `tlsFingerprintCollector *service.TLSFingerprintCollectorService`(archiveService 之后);parallelSteps 加 `{"TLSFingerprintCollectorService", func() error { if tlsFingerprintCollector != nil { return tlsFingerprintCollector.Stop(ctx) }; return nil }}`(ArchiveService 步之后)。
  - 重生成 `cmd/server/wire_gen.go`(provideCleanup 收到 collector;wire 由 NewTLSFingerprintCollectorService 提供)。
  - `cmd/server/wire_gen_test.go`:手写的 provideCleanup 直调补 `nil // tlsFingerprintCollector` 实参(ArchiveService 之后)。
- 命令与结果(容器内):`go generate ./cmd/server` OK(go.mod/go.sum **未污染**;wire_gen 含 3 处 TLSFingerprintCollectorService 引用);`gofmt -l` 空;`go build ./...` → **BUILD_OK**;`go vet ./cmd/server/...` → **VET_OK**;`go test ./cmd/server/...`(含 wire_gen_test)→ **ok**。
- commit:`0f2624f7`
- 遗留/风险:无。

### Phase J — 全量生成 + 编译确认 — ✅ 完成(2026-07-01)

- 容器内跑 `go generate ./ent` + `go generate ./cmd/server` 后 **git status 为空**——即所有已提交的生成码(ent 全套 + wire_gen)与当前 schema/providers **完全一致,无 drift**(强完整性校验)。`go build ./...` → **BUILD_OK**;`go vet ./...` → 无报错;`go test ./internal/pkg/tlsfingerprint/...` → ok。
- 无码改(纯确认),仅本账本记录。

### Phase K — 前端 — ⏸️ 记录待续(2026-07-01)

- 未做原因:K 是大件(router 弹窗 ~880 行 + 采集器 UI + 账号绑定下拉 + i18n ~74 键 + AccountsView 挂载),且需前端工具链(pnpm/vite type-check/build)验证;本批次预算已深,留作专门批次。方案 §K 明允「前端工具链不可用则记待人工构建验证」。
- 续作清单(方案 §K 文件表,照抄 TR 前端但**对照 fork 现有 `TLSFingerprintProfilesModal.vue` 约定**,勿引 fork 不存在的组件/工具):
  1. 新增 `frontend/src/api/admin/tlsFingerprintRouter.ts`(router CRUD + 类型)。
  2. 编辑 `frontend/src/api/admin/tlsFingerprintProfile.ts`(补采集器 6 函数 + 3 类型:status/session/captureRecord)。
  3. 编辑 `frontend/src/api/admin/index.ts`(导出 routerAPI + 类型)。
  4. 新增 `frontend/src/components/admin/TLSFingerprintRoutersModal.vue`(列表/编辑/规则编辑器/YAML 粘贴 + chatgpt_oauth_token / codex_invite_reset 两槽)。
  5. 编辑 `frontend/src/components/admin/TLSFingerprintProfilesModal.vue`(加采集器 UI:start/stop/建会话/capture URL+CA PEM/命令/采集列表/存为 profile)。
  6. 编辑 `frontend/src/views/admin/AccountsView.vue`(加 "TLS Routers" 入口 + 弹窗挂载 + openTLSFingerprintRouters + state;按符号定位)。
  7. 编辑 `frontend/src/components/account/EditAccountModal.vue`、`CreateAccountModal.vue`(加 `tls_fingerprint_router_id` 下拉「不使用路由器」+ 提交写入 `extra`)。
  8. 编辑 `frontend/src/i18n/locales/en.ts`、`zh.ts`(补 `admin.tlsFingerprintRouters.*` + 账号表单 router 键)。
  9. 验证:`pnpm -C frontend type-check`/`build`(读 `frontend/package.json` scripts);工具链不可用则记「前端待人工构建验证」。
- 后端已就绪:router CRUD API(`/admin/tls-fingerprint-routers`)+ 采集器 API(`/admin/tls-fingerprint-profiles/collector/*`)+ 账号 `extra.tls_fingerprint_router_id` 字段均可用,前端直接对接即可。

### Phase K(part 1)— 前端 router CRUD UI — ✅ 完成(typecheck 绿)(2026-07-01)

- 前端工具链**可用**(本机 node v22 + pnpm 10.30;`pnpm install` 后 `pnpm typecheck`=vue-tsc --noEmit)。
- 改动文件:
  - 新增 `frontend/src/api/admin/tlsFingerprintRouter.ts`(照抄 TR,API client 模式与 fork 完全一致,verbatim)。
  - 编辑 `frontend/src/api/admin/tlsFingerprintProfile.ts`(fork 是 TR 的严格子集 → 直接用 TR 版覆盖,补 3 采集器类型 + 6 采集器函数)。
  - 编辑 `frontend/src/api/admin/index.ts`(镜像 profile:import + aggregate + re-export + type export)。
  - 新增 `frontend/src/components/admin/TLSFingerprintRoutersModal.vue`(883 行,照抄 TR;依赖 `useClipboard`/`Select.vue`/`BaseDialog`/`ConfirmDialog`/`Icon` fork 均有)。
  - 编辑 `frontend/src/views/admin/AccountsView.vue`(镜像 profiles:工具菜单按钮 + 弹窗挂载 + import + `showTLSFingerprintRouters` state + closeAll 守卫 + `openTLSFingerprintRouters`)。
  - 编辑 `frontend/src/i18n/locales/en.ts`、`zh.ts`(从 TR 抽取 `admin.tlsFingerprintRouters` 整块,插入到 profiles 块之后)。
- 命令与结果:`pnpm install` exit 0(未改 pnpm-lock);`pnpm typecheck` → **exit 0,无错误**(RoutersModal 对 fork 组件的 prop 用法全部匹配)。
- commit:`49547eb3`
- 遗留:①账号绑定下拉(EditAccountModal/CreateAccountModal 加 `tls_fingerprint_router_id`)②ProfilesModal 采集器 UI(fork 与 TR 该文件 diff 618 行,须手术式合并,非整档覆盖)——见下。

### Phase K(part 2)— 账号 router 绑定下拉 — ✅ 完成(typecheck 绿)(2026-07-01)

- 改动文件:
  - `frontend/src/types/index.ts`:`Account` 加 `tls_fingerprint_router_id?: number | null`(供 populate 时类型安全)。
  - `frontend/src/components/account/EditAccountModal.vue`:镜像 profile 绑定 —— 模板加 router `<select>`(不使用路由器 + 路由器列表,**不 gate 在 tlsFingerprintEnabled**,因 router 绑定独立);state `tlsFingerprintRouterId`/`tlsFingerprintRouters`;`loadTLSProfiles` 补加载 routers;reset/populate(`account.tls_fingerprint_router_id`);submit 在 TLS if/else **之后**独立写 `extra.tls_fingerprint_router_id`(有值写、无值删)。
  - `frontend/src/components/account/CreateAccountModal.vue`:同上(create 无 populate;load 用 .then/.catch;submit 独立写 extra)。
  - `frontend/src/i18n/locales/en.ts`、`zh.ts`:账号表单 `admin.accounts.quotaControl.tlsFingerprint` 加 `router`/`noRouter` 键。
- 命令与结果:`pnpm typecheck` → **exit 0,无错误**。
- commit:`9bfac46d`
- 遗留:仅 ProfilesModal 采集器 UI(part 3)。

### Phase K(part 3)— ProfilesModal 采集器 UI — ⏸️ 记录待续(2026-07-01)

- 未做原因:TR 的 `TLSFingerprintProfilesModal.vue`(1068 行)含**深度交织的采集器 UI**(127 处 collector 引用,从模板第 27 行状态横幅到脚本第 811 行,贯穿模板+脚本),而 fork 同文件(625 行)与 TR **diff 达 618 行**(注释语言等分歧)→ 不能整档覆盖,须**手术式抽取采集器块合并**。**采集器本身已可经 API 全用**(collector/status|start|stop|sessions|captures|delete 后端+前端 API 均就绪),此 UI 属管理便利、次要;近预算上限不宜草率合并有分歧大文件(防未验证破坏)。
- 续作清单(照 TR `TLSFingerprintProfilesModal.vue` 手术合并,对照 fork 约定):
  1. 模板:顶部采集器状态横幅(running/stopped + start/stop)+ 采集区(建会话→capture URL+CA PEM+命令;采集列表;每条「存为 profile」)。
  2. 脚本:state + 函数(start/stop/createSession/listCaptures/deleteSession/copy CA/save-as-profile/detect client kind)——API 已在 `api/admin/tlsFingerprintProfile.ts`(part1 已加 6 函数+3 类型)。
  3. i18n:补 `admin.tlsFingerprintProfiles.collector.*`(en/zh,从 TR 抽)。
  4. `pnpm typecheck` 验证。
- 注:采集器 token 复用/抓包/存 profile 属【运行时·人工】。

### Phase K(part 3)— ProfilesModal 采集器 UI — ✅ 完成(typecheck 绿)(2026-07-01)

- 手术式合并(**非整档覆盖**),对照 fork 现有写法。前置实证:fork 同名文件(625 行)只有 profile CRUD、无采集器;TR(1068 行)采集器 UI 深度交织。采集器前端 API(`tlsFingerprintProfile.ts` 6 函数+3 类型)+ `adminAPI.tlsFingerprintProfiles` 聚合(index.ts:64)part1 已就绪;8 个采集器图标(beaker/play/x/copy/download/check/refresh/plus)在 `Icon.vue` 全部存在;`common.copy`/`common.refresh` 在 `common:` 块存在。
- 改动文件:
  - `frontend/src/components/admin/TLSFingerprintProfilesModal.vue`:
    - **模板**:header 与 profiles table 之间插入采集器块(状态横幅 running/stopped + refresh/start/stop;监听/外部地址/证书信息;last_error;建会话→capture URL+过期时间;Claude/Codex 命令 + Codex 配置片段各带复制;复制/下载 CA;实时采集列表每条带 copy-YAML + Fill-Form)。HTML 注释 `<!-- 收集器 -->`→ 英文 `<!-- Collector -->`(对齐 fork 英文注释约定)。
    - **脚本**:import 加 `computed`/`onUnmounted` + 3 采集器类型;加采集器 state(status/session/captures/4 loading flag/pollTimer);加 computed(isCollectorRunning/activeCAPEM/claudeCommand/codexCommand/codexConfigSnippet);watch 增开时 loadCollectorStatus、关时 stopCollectorPolling + onUnmounted 清理;加函数 loadCollectorStatus/toggleCollector/createCollectorSession/refreshCollectorCaptures/start+stopCollectorPolling/applyCapture/copyText/downloadCA/formatDateTime/formatClientKind;**抽出 `fillFormFromProfile`**(handleEdit 与 applyCapture 共用,handleEdit 改为调用它)。注释全用英文(fork 约定);CA 下载文件名 `tokenrouter-*`→`sub2api-*`(fork 品牌,tls.sub2api.org)。
  - `frontend/src/i18n/locales/en.ts` / `zh.ts`:在 `admin.tlsFingerprintProfiles` 的 columns 与 form 之间插入 `collector.*`(37 键,verbatim 抄 TR)。
- **范围守卫(faithful,不越界)**:**未**移植 TR 的 profile 行内「Copy as YAML」按钮及其 helper(buildProfileYaml/handleCopyYaml/formatYaml* 系列)——非采集器 UI、不在续作清单;**未**加 TR 的 `openCreateModal`(fork 现有 create 按钮 `showCreateModal=true` 流程正确,applyCapture 自行管理 state);**未**接线 `deleteCollectorSession` 按钮(**TR 的 ProfilesModal 本身也未调用它**——照抄即无此 UI,API 函数保留可用;不臆造 TR 没有的 UI);fork 现有 `parseYamlInput` 保持原样(TR 的 description/parseYamlScalar 增强非采集器范畴)。
- 命令与结果:`pnpm typecheck`(vue-tsc --noEmit,node v22.22.3 / pnpm 10.30.3)→ **exit 0,无错误**。自检:无重复声明;组件引用的全部 `collector.*` i18n 键在 en/zh 均存在;git status 仅 3 文件。
- commit:`a71b135b`
- 遗留/风险:**采集器真实抓取 / token 复用 / JA3 / 存为 profile / 建会话拿 CA / 命令可用性 属【运行时·人工】**(静态不可证,未宣称已验证)。收集器默认 host=127.0.0.1 且不自动启动(Phase E 安全默认),需管理员经 UI 显式 start 才监听。

## 阶段性总结 / 续作指南(2026-07-01 无人值守批次)

**已完成并验证(静态全绿 + 已 commit;运行时验证一律留人工)**:
| Phase | 内容 | commit |
|---|---|---|
| 0(baseline) | 修 base 分支预存编译错(Grok 孤儿调用,方案 A) | e6ba7d0a |
| A | tlsfingerprint 包补 3 文件(HTTP1OnlyProfile/CacheKey/ClientHello capture)+2 测 | 4488db9c |
| B | ent schema + model + 生成(tls_fingerprint_routers 表) | ce925be7 |
| C | 迁移 158(静态;DB 幂等待人工) | d6cec220 |
| D | repository repo+cache + router service(+6 子测) | 0e5c588d |
| E | collector service(+测)+ config(安全默认 127.0.0.1)+ profile resolvers + service wire | 3161a1df |
| F | admin router handler + profile collector 端点 + 路由 + handler wire(wire 重生成) | 8f57d2c6 |
| G1 | account getters(SupportsTLSFingerprint/GetTLSFingerprintRouterID;IsTLSFingerprintEnabled 改门控) | 21e18825 |
| G3 | GatewayService 路由感知 TLS 解析(Anthropic + 转换转发 HTTP 路径) | dedb31e6 |

> **注**:上表是首批(停在 G3)的快照;下方「最新状态」为完整现状。

**最新状态(2026-07-01 批次收尾)——已完成并验证(静态全绿 + 已 commit)**:
- Phase 0/A/B/**C(+ephemeral PG 幂等)**/D/E/F 全部 ✅。
- **Phase G 全部 ✅(含 G5)**:G1(getters)+G3(GatewayService 解析,`dedb31e6`)+**native-OpenAI TLS**(解 Blocker #2,`d9c213ec`)+**G4 UA/Originator**(`577d4e88`)+**G5 OAuth 换 token 专用 UA/profile**(`162b8377`)。即 **OpenAI HTTP 三类路径(Anthropic 原生 / Anthropic→OpenAI 转换转发 / native OpenAI)全部路由感知 TLS + UA 改写,且 OAuth 换 token 亦按账号 router 应用专用 UA/指纹**。
- **Phase H ✅(静态)**(`1c9dfae5`,WS 集成:连接池按指纹隔离防串号;串号/JA3 留人工抓包)。
- **Phase I ✅**(`dc6896ff`,采集器优雅关闭)。
- **Phase J ✅**(全量 generate 无 drift + build/vet 绿)。
- 每 commit 容器内 gofmt+build+vet+相关 test(及 ent/wire 处 generate)全绿;**新增单测**:tlsfingerprint(3)、router service(6)、collector(4),均 PASS;OpenAI/gateway/handler/WS 既有测试全过。

- **Phase K 前端全部 ✅(typecheck 绿)**:part1 router API/RoutersModal/AccountsView 入口/i18n(`49547eb3`)+ part2 账号绑定下拉(`e8140f32`/`9bfac46d`)+ **part3 ProfilesModal 采集器 UI(`a71b135b`)**。

**后端全部完成 + 前端全部完成**(G1-G4 HTTP 三路径 + H WS + 采集器 + I 优雅关闭 + 迁移/schema/repo/service/handler/wire;前端 router CRUD + 账号绑定 + **采集器 UI** typecheck 绿)。**TLS 指纹路由器 + 采集器功能端到端可用**(管理员 UI 建路由器/规则 + 绑定账号 → 后端 HTTP/WS 出站按 UA 路由指纹;管理员 UI 启动内置采集器抓真实 ClientHello → 存为 profile)。

**移植主线(方案 §8 A→K + G5)全部完成**并静态验证:前次批次遗留的 K part3 采集器 UI、G5 OAuth token 本批次均已做绿并 commit(`a71b135b`、`162b8377`),自查发现的 native-OpenAI 8 处漏 DoWithTLS 亦已补(`073be243`)。**但收尾自查(对照 TR 全量符号)另发现若干与 TokenRouter 的未对齐点,多为次要/有意跳过/架构差异——完整分级清单见文末「## 与 TokenRouter 未对齐清单(权威版)」**,其中唯一有实际功能意义的是 A1(OAuth 建号/手动导入/ReAuth 时带 router),待人工决定是否补齐。

**待人工验证(运行时,方案 §5,静态不可证一律留人工)**:迁移 158 生产/测试库应用;router CRUD + 采集器 API + 前端 UI(含采集器 start/建会话/抓取/存 profile/命令可用性);**HTTP 抓包 JA3(含 native OpenAI + UA/指纹一致)——特别是 Blocker#3 新补的 8 条出站路径(ForwardAsChatCompletions/ForwardAsAnthropic/images(APIKey+OAuth)/ws_http_bridge/raw_chat_completions/responses_chat_fallback/count_tokens)**;**WS 抓包 + 不同指纹不串号**;未命中回落;**OAuth 换 token 出站 JA3 + 专用 UA(G5)**。

**如何续作/收尾**:工作树干净,所有 commit 各自 build/vet/typecheck 绿,可安全接续。**核心功能端到端完整**:router HTTP(三路径 + native 8 补漏)+WS+OAuth 换 token 自动刷新(G5)+ 账号绑定 + CRUD UI + 采集器 UI + i18n。**剩余代码续作项**:见文末「## 与 TokenRouter 未对齐清单(权威版)」—— 主要是 A1(OAuth 建号带 router,前后端一整条);其余为有意跳过/架构差异/琐碎。运行时验证一律留人工(见上)。

---

## 与 TokenRouter 未对齐清单(权威版,2026-07-01 收尾自查)

> 依据:对照 TR 全量 TLS 符号 + 2 个审计子代理 + 逐点 git/grep 实证。**核心功能已全部对齐**(HTTP 三路径 + native 8 补漏 + WS + 采集器 + router CRUD + 账号绑定 + 已存账号 token 自动/按账号刷新带 router)。下列为剩余差异,按性质分档。**新会话补齐从 A1 开始。**

### A. 真实功能差异(TR 有、fork 无)

**A1 — ✅ 已完成(2026-07-01 收尾批次;后端 `13e0c929` + 前端 `ad4df180`;详见文末「## 收尾批次」)** — OAuth「建号 / 手动 rt 导入 / 重新授权」时带 router(= Blocker #4)
稳态刷新(后台定时 / 账号页刷新 / 按账号 :id / CRS 同步,均走 `RefreshAccountToken`)**已对齐**,只差这几处一次性请求。TR 参照 → fork 缺口:
- **后端 handler** `internal/handler/admin/openai_oauth_handler.go`:
  - `OpenAIExchangeCodeRequest` 加 `TLSFingerprintRouterID *int64 json:"tls_fingerprint_router_id"`(TR :76)→ 传入 `OpenAIExchangeCodeInput`(TR :94);fork 两处 ExchangeCode(:87/:246)现未传。
  - `OpenAIRefreshTokenRequest` 加同字段(TR :110)→ 改调 `RefreshTokenWithClientIDAndRouter(..., req.TLSFingerprintRouterID)`(TR :163);fork :160 现调无 router 的 `RefreshTokenWithClientID`。
  - `CreateAccountFromOAuth`:建号时把 router 传 ExchangeCode(TR :256)**并**写入 `account.Extra["tls_fingerprint_router_id"]`(TR :278-281);核对 fork 建号 handler 现状(fork 靠账号表单二次编辑写 Extra,建号链未写)。
- **后端 service** `internal/service/openai_oauth_service.go`:加公开 `RefreshTokenWithClientIDAndRouter(ctx, refreshToken, proxyURL, clientID, routerID *int64)`(TR :233)→ 内部转 `refreshTokenWithClientID(..., valueFromInt64Ptr(routerID), nil)`(**fork 已有该私有方法 + valueFromInt64Ptr,G5 加的,直接复用**)。
- **后端 DTO 回显** `internal/handler/dto/{types.go,mappers.go}`:`AccountResponse` 加 `TLSFingerprintRouterID *int64 json:"tls_fingerprint_router_id,omitempty"`(TR types.go:260)+ mappers 从 `a.GetTLSFingerprintRouterID()` 填充(TR mappers.go:343-344);fork 实测两文件均无。
- **前端**:
  - `src/composables/useOpenAIOAuth.ts`:`exchangeAuthCode(..., tlsFingerprintRouterId?)`(TR :159)写 payload(TR :184-185);`validateRefreshToken(..., tlsFingerprintRouterId?)`(TR :210)。
  - `src/api/admin/accounts.ts`:`exchangeCode` payload 加 `tls_fingerprint_router_id?`(TR :426);`refreshOpenAIToken(..., tlsFingerprintRouterId?)`(TR :719, 写 :735-736)。
  - `src/components/account/CreateAccountModal.vue`:加 `selectedOpenAITokenTLSRouterId()`(TR :5447,开关开启时取 router 下拉值)+ 建号时选 router 的 UI + 传给 `exchangeAuthCode`(TR :5602)/`validateRefreshToken`(TR :5848)。
  - `src/components/account/ReAuthAccountModal.vue`(account 版 + `admin/account/` 版):`exchangeAuthCode(..., props.account.tls_fingerprint_router_id)`(TR :366 / :380)。
  - `types/index.ts`:`Account` 已有 `tls_fingerprint_router_id`(K part2 加);账号表单 router 下拉已存在(K part2),复用。

**A2 — ✅ 已完成(2026-07-01,用户拍板做;commit `f8a81017`;详见文末「## 收尾批次」)** — `BulkEditAccountModal` 批量设 TLS profile/router:TR 有(实测 37 处命中,`src/components/account/BulkEditAccountModal.vue`);fork **0 处**。⚠️ **注意**:fork 该文件**连原有的 profile 绑定也没有**(是 fork 与 TR 的既有差异,非本 port 回归)。补则需同时加 profile+router 两者,面较大;**非必要,低优先**。

### B. 有意跳过(已记录,scope 决策,非遗漏 —— 除非明确要,否则勿动)
- **profile 列表行「Copy as YAML」按钮** + `buildProfileYaml`/`handleCopyYaml`/`formatYaml*`(TR ProfilesModal:298-306, :998):K part3 有意跳过(采集面板内复制 YAML 仍在)。
- **codex_invite_reset** — ⏸️ **用户拍板暂不搬(2026-07-01)**:它是 TR 的独立业务(679 行 service + handler + 3 端点 status/invite/consume 打 chatgpt.com backend-api),**不是 TLS 路由器核心**;TLS 路由器只给它留了指纹槽,而"接槽"物理上长在该 service 上、装饰其自身请求,**无法与功能分离**(fork 无这些请求→槽无处可贴)。故二选一:搬整功能 or 保持现状。用户选**保持现状**——fork 无 `codex_invite_reset_service.go`;schema 2 预留列 + 前端 RoutersModal 预留槽仍在(能存不生效)。**注**:quota 改造(见 C 档)已复用这 2 列(codex_invite_reset UA/profile)作配额出站指纹,故这两列现在对 quota **有效**(对 invite-reset 仍不生效,因功能未搬)。**勿为它新建服务/臆造调用。**

### C. 架构差异(fork 既有,非 port 漏搬,勿在本 port 强改)
- **`openai_quota_service.go`** — ✅ **已完成(2026-07-01,用户拍板 faithful TR;commit `a175ab4c`)**:配额查询/重置出站从 `privacyClientFactory`(resty 通用伪装)改成 `httpUpstream.DoWithTLS(账号/router 解析的 profile)`+ Codex Desktop UA(复用 router 的 codex_invite_reset UA/profile 槽,回落账号 `ResolveTLSProfile`),与主网关路径一致。**注意(待人工)**:账号无 router 且未启用 TLS → profile=nil → DoWithTLS 回落 plain `Do`(丢原通用伪装),配额打 chatgpt.com 可能被 Cloudflare 拦(与 TR 及主网关同行为);建议给这类账号配 router codex 槽或账号 profile。
- **`openai_embeddings.go` 裸 `Do`**:TR 也裸 `Do`,两边**一致**(非未对齐,勿改)。

### D. 琐碎(可顺手,零/低风险)
- ✅ **已补(收尾批次 `8c3d8280`)**:账号表单 i18n `routerHint`(en/zh,照抄 TR)+ Create/Edit 模板 `<p class="input-hint">` 提示元素(K part2 只加了下拉未加提示,fork 无组件引用该键 → 连模板一并补,避免挂空键)。
- ✅ **已补(收尾批次 `8c3d8280`)**:`src/api/admin/index.ts` 聚合再导出 3 个采集器类型(`TLSFingerprintCollectorStatus/CollectorSession/CaptureRecord`)。
- ✅ **已补(2026-07-01,commit `95e34cc9`)**:测试——`tls_fingerprint_profile_service_test.go`(照抄 TR,2 测:ResolveTLSProfile OpenAI OAuth/APIKey + gateway resolveOpenAITLSProfile router 命中/回退)+ `openai_oauth_token_options_test.go`(`resolveChatGPTOAuthTokenRequestOptions` 表驱动 7 例 + 2 stub)。容器 gofmt/vet/test 全绿。

---

## 收尾批次(2026-07-01):未对齐清单 A1 + D 补齐 — ✅ 完成(静态全绿)

> 本批次只做文末「未对齐清单」的 **A1**(唯一有实际功能意义)+ **D**(琐碎零风险);**A2 评估后暂缓**(见下)。核心功能上一批已全部对齐。每项容器内 build/vet/test(后端)或 pnpm typecheck(前端)全绿后 commit。运行时(出站 JA3 / 专用 UA)一律留人工。

### A1 后端 — OAuth 建号/手动 refresh/换 token 带 router — ✅ 完成 — commit `13e0c929`
- 前置核对(不臆测):fork 的 `openai_oauth_handler.go` 与 TR **结构逐字同构**(inline 建号 struct、helper、CreateAccountInput 用法全同)→ 满足「结构对齐→照抄」,非硬凑;G5 已在 service 备好私有 `refreshTokenWithClientID(routerID, account)` + `valueFromInt64Ptr` + `OpenAIExchangeCodeInput.TLSFingerprintRouterID`,只差公开包装。
- 改动文件(4):
  - `internal/service/openai_oauth_service.go`:加公开 `RefreshTokenWithClientIDAndRouter(ctx, rt, proxyURL, clientID, routerID *int64)`(转私有 `refreshTokenWithClientID(..., valueFromInt64Ptr(routerID), nil)`;TR :232-235 逐字)。
  - `internal/handler/admin/openai_oauth_handler.go`:`OpenAIExchangeCodeRequest`/`OpenAIRefreshTokenRequest`/`CreateAccountFromOAuth` inline struct 各加 `TLSFingerprintRouterID *int64 json:"tls_fingerprint_router_id"`;两处 `ExchangeCode` 传入 input;`RefreshToken` 改走 `RefreshTokenWithClientIDAndRouter(..., req.TLSFingerprintRouterID)`;建号在 CreateAccountInput 前构造 `extra{tls_fingerprint_router_id}`(仅 routerID>0)并 `Extra: extra`。
  - `internal/handler/dto/types.go`:`Account`(AccountResponse)加 `TLSFingerprintRouterID *int64 json:"...,omitempty"`。
  - `internal/handler/dto/mappers.go`:`AccountFromServiceShallow` 在 profile 回显后加 `if routerID := a.GetTLSFingerprintRouterID(); routerID > 0 { out.TLSFingerprintRouterID = &routerID }`。**按 fork 现有风格**(`>0`、无 TR 的 `SupportsTLSFingerprint()` 包裹——fork router 绑定独立于开关,与 K part2 一致),非照搬 TR 结构。
- 调用方/mock 核对:service 公开 `RefreshTokenWithClientID` 仍被内部 `RefreshToken` 用(不成孤儿);`*_test.go` 里的 `RefreshTokenWithClientID` 是 **client 接口**方法(非 service),未受影响;`NewOpenAIOAuthHandler` 仅 wire.go 构造(concrete,无 mock);无 golden-JSON 测试断言 TLS DTO 字段。**无 wire/签名变更 → 无需 go generate**。
- 命令与结果(容器内 golang:1.26.4-alpine):`gofmt -l`(4 文件)空;`go build ./...` BUILD_OK;`go vet ./internal/handler/... ./internal/service/...` VET_OK;`go test ./internal/handler/... ./internal/handler/dto/...` ok(handler 22.2s / admin / dto 全过);`go test ./internal/service/ -run "OpenAIOAuth|OAuth|Refresh|Exchange|Token|TLSFingerprint"` ok;`go test ./internal/repository/ -run "OpenAIOAuth|OAuth"` ok。
- 范围澄清:fork 前端**不调** `create-from-oauth` 端点(UI 走 exchangeCode + 常规建号,router 经账号 extra 在常规 submit 持久化)。`CreateAccountFromOAuth` 的 Extra 写入是 **faithful 照 TR**(供 API 直连/未来 UI),当前 UI 未触达——非缺陷,记录备查。
- 遗留/风险:OAuth 换 token 出站 JA3 + 专用 UA 属【运行时·人工】。

### A1 前端 — OAuth 建号/手动 RT 导入/重新授权 带 router — ✅ 完成 — commit `ad4df180`
- 前置核对:fork `useOpenAIOAuth.ts` / `accounts.ts` 的 exchangeCode/refreshOpenAIToken 与 TR **除 TLS 增量外逐字一致**;TR 另有的批量授权 session 逻辑(`authSessions`/`appendAuthUrl`/`removeAuthSession`/`OpenAIOAuthSession`)**非本 port 范畴,未移**(scope-guard)。
- 改动文件(5):
  - `src/composables/useOpenAIOAuth.ts`:`exchangeAuthCode(..., tlsFingerprintRouterId?)` 写 payload.tls_fingerprint_router_id;`validateRefreshToken(..., tlsFingerprintRouterId?)` 透传 refreshOpenAIToken 第 5 参。
  - `src/api/admin/accounts.ts`:`exchangeCode` payload 类型加 `tls_fingerprint_router_id?`;`refreshOpenAIToken(..., tlsFingerprintRouterId?)` 加参+payload(TR :424/:714 逐字)。
  - `src/components/account/CreateAccountModal.vue`:`handleOpenAIExchange` 的 `exchangeAuthCode` / `handleOpenAIBatchRT` 的 `validateRefreshToken` 传 **K part2 的 `tlsFingerprintRouterId.value`**(直接复用账号 router 下拉,独立于 TLS 开关——与 fork submit 5633 一致;未引入 TR 的 `selectedOpenAITokenTLSRouterId()` 开关门控,因 fork router 绑定独立)。
  - `src/components/account/ReAuthAccountModal.vue` + `src/components/admin/account/ReAuthAccountModal.vue`:`exchangeAuthCode(..., props.account.tls_fingerprint_router_id)`(账号已绑定的 router;TR :366/:380)。`Account` 类型 K part2 已含该字段。
- 命令与结果:`pnpm typecheck`(vue-tsc --noEmit,node v22 / pnpm 10.30)exit 0。
- 遗留/风险:换 token/重新授权/RT 导入的真实出站 JA3 + 专用 UA 属【运行时·人工】。

### D — index.ts 补导出 + 账号表单 routerHint — ✅ 完成 — commit `8c3d8280`
- 改动文件(5):`src/api/admin/index.ts`(聚合再导出 3 采集器类型,零功能影响)+ `CreateAccountModal.vue`/`EditAccountModal.vue`(router 下拉后加 `<p class="input-hint">routerHint</p>`)+ `i18n/locales/en.ts`/`zh.ts`(`admin.accounts.quotaControl.tlsFingerprint.routerHint`,照抄 TR 文案)。
- 命令与结果:`pnpm typecheck` exit 0。

### A2 — BulkEditAccountModal 批量设 TLS — ✅ 已完成(2026-07-01,用户拍板对齐核心功能)— commit `f8a81017`
- 背景:用户澄清「目标是对齐 TR 核心功能点,不因体量大而跳过」。A2 是 TLS 路由器功能的批量形态,遂实施(下方保留原评估作追溯)。
- 前置实证:fork `BulkEditAccountModal.vue` **TLS 引用 0 处**(profile+router 皆无);TR 37 处。**后端无需改**——bulkUpdate 走 `UpdateAccountExtra` JSONB key 级 merge(account_handler.go:999),已接受任意 extra key(fork 早已批量写 openai_passthrough/ws/codex/base_rpm 等)。
- 改动(纯前端 `BulkEditAccountModal.vue`):① 门控 `allTLSFingerprintCapable = allOpenAIOAuth || allAnthropicOAuthOrSetupToken`(镜像后端 `SupportsTLSFingerprint`);② TLS section:enable 复选 → 固定指纹开关 + profile 下拉(开关门控,0=默认/-1=随机)+ router 下拉(**独立于开关**,与 fork 单账号 Edit 一致,非 TR 的开关门控)+ routerHint;③ state/computed + `loadTLSFingerprintProfiles/Routers`(复用 `adminAPI.tlsFingerprint*.list`,同 Create/Edit 写法);④ `buildUpdatePayload` 写 `extra.enable_tls_fingerprint`/`tls_fingerprint_profile_id`(开关?id:0)/`tls_fingerprint_router_id`(routerId??0),0/false 显式重置(JSONB merge 语义);⑤ hasAnyFieldEnabled/reset/open-load(show=true 且 capable 时加载列表)接线。
- 命令与结果:`pnpm typecheck` exit 0。批量出站 JA3/UA 属【运行时·人工】。
- (原评估,保留追溯):曾判 A2「移植 fork 从未有的批量功能、非纯 TLS port、改 bulk-edit UX」故暂缓;用户纠正后实施。

### 本批次收尾状态
- 工作树:干净(A1 后端 `13e0c929` + A1 前端 `ad4df180` + D `8c3d8280` 各自静态全绿已 commit;本账本 docs 另提交;无半成品)。
- 已解:Blocker #4(= A1)✅。
- **仍待人工验证(运行时,静态不可证)**:OAuth 换 token / 手动 RT 导入 / 重新授权的出站 **JA3 指纹 + 专用 UA**(经 exchange-code / refresh-token 端点,按所选/账号绑定 router 的 ChatGPTOAuthTokenUserAgent + ChatGPTOAuthTokenTLSFingerprintProfileID);前端点测(建号/编辑/重新授权下拉与 routerHint 显示);账号 DTO 回显 tls_fingerprint_router_id。
- 未对齐清单剩余(有意不做):**A2**(评估暂缓,见上);**B 档**(Copy YAML / codex_invite_reset,有意跳过);**C 档**(openai_quota_service / embeddings 架构差异,勿改);**D 档测试**补充(可选)。
- **结论**:「未对齐清单」中唯一有实际功能意义的 A1 已前后端补齐并静态全绿;D 琐碎项已补;A2 评估暂缓待人工;B/C 档有意保留。TLS 指纹路由器移植的**功能对齐至此完成**。

---

## 第二轮收尾(2026-07-01):用户要求"对齐 TR 核心功能,不因体量大而跳过" → A2 + quota + D + codex 决策

> 用户纠正了"差异大就不做"的思路,要求逐项按"是不是 TLS 路由器核心功能"重判。据此:A2 做了(见上,`f8a81017`);quota、D 做了(下);codex_invite_reset 查清是独立功能、用户拍板暂不搬。

### C① quota 出站带 TLS 指纹 — ✅ 完成 — commit `a175ab4c`
- 前置实证(TR 参照):TR quota 用 `httpUpstream.DoWithTLS`,且**复用 router 的 codex_invite_reset UA/profile 槽**(注释"限流重置走 Codex Desktop 后台接口,复用邀请重置专用 UA")回落 `ResolveTLSProfile(account)`。fork model 已有这 2 列(Phase B),故可 faithful 复用而无需搬 codex 功能。fork 无 `openai_quota_service_test.go`(无测试调用方);`httpUpstream`/`TLSFingerprintRouterService`/`TLSFingerprintProfileService` wire 处均就绪;`WithHTTPUpstreamProfile(OpenAI)` 是 DoWithTLS 调用方统一模式。
- 改动(`openai_quota_service.go` + `wire.go` + 重生成 `wire_gen.go`):struct/constructor 把 `privacyClientFactory` 换成 `httpUpstream`+`tlsFPRouterReader`+`tlsFPProfileService`;`prepareUpstreamCall` 多返回 `*Account`;`QueryUsage`/`ResetCredit` resty→`http.NewRequestWithContext`+`DoWithTLS`+`json.Unmarshal`(**保留 fork 单 POST reset 语义,不引 TR 的 pick-credit 流程**);新增 `resolveRuntimeRouter`/`resolveQuotaTLSProfile`(codex 槽→账号回退)/`resolveQuotaUserAgent`/`doQuotaRequest`(`WithHTTPUpstreamProfile`+DoWithTLS+读 body)/`applyQuotaHeaders`;`ProvideOpenAIQuotaService` 换参(concrete `*TLSFingerprintRouterService` 满足 `OpenAIOAuthTokenRouterReader` 接口,免 wire.Bind)。
- 命令与结果(容器):gofmt 空;`go build ./...` BUILD_OK;`go vet` VET_OK;`go test`(service/handler/admin/dto/cmd)全绿;go.mod/go.sum **未污染**。
- **待人工(运行时)**:配额出站 JA3 抓包;**尤其**账号无 router 且未启用 TLS 时 profile=nil → DoWithTLS 回落 plain `Do`(丢原 privacy client 通用伪装)→ chatgpt.com 可能被 Cloudflare 拦(与 TR 及主网关同行为)。建议这类账号配 router codex 槽或账号 profile。

### B① codex_invite_reset — ⏸️ 用户拍板暂不搬(2026-07-01)
- 查清:TR `codex_invite_reset_service.go`(679 行)+ handler(85 行)+ 3 端点(`/accounts/:id/codex/invite-reset/{status,invite,consume}`)打 chatgpt.com backend-api(`/referrals/invite/eligibility`、`/wham/rate-limit-reset-credits`、`/wham/referrals/invite`、`.../consume`)。是**独立 Codex 业务**,TLS 路由器只给它留指纹槽;"接槽"是长在该 service 上的 3 个 resolver 方法、装饰其自身请求 → 与功能不可分离(fork 无这些请求)。故要么搬整功能、要么保持现状。
- 决策:用户选**保持现状**。fork 不新建该 service;router 2 预留列 + 前端预留槽保留(对 invite-reset 不生效,但现已被 quota 复用生效)。

### D 测试 — ✅ 完成 — commit `95e34cc9`
- `tls_fingerprint_profile_service_test.go`(照抄 TR,swap import path):ResolveTLSProfile(OpenAI OAuth 返内置默认 / API Key 返 nil)+ `OpenAIGatewayService.resolveOpenAITLSProfile`(router 命中优先 / 目标不可用回退账号固定模板)。
- `openai_oauth_token_options_test.go`:`resolveChatGPTOAuthTokenRequestOptions` 表驱动 7 例(routerID<=0 / router 未找到 / 未启用 / 无UA无profile / 仅UA / 仅profile / UA+profile+account)+ 2 stub(router reader / profile resolver)。
- 容器 gofmt/vet/test 全绿。

### 第二轮收尾状态
- **与 TR 未对齐清单现状**:A1 ✅ / A2 ✅ / C① quota ✅ / D 测试 ✅ / B② Copy-YAML 有意留 / **B① codex_invite_reset 用户拍板暂不搬** / C② embeddings 假差异(两边一致)。**功能性缺口已清零**(除用户明确不搬的 codex_invite_reset 独立功能)。
- 工作树:干净。第二轮 commit:A2 `f8a81017` + 账本 `b75d620c` + quota `a175ab4c` + D 测试 `95e34cc9`(+ 本账本)。
- **待人工验证(运行时,静态不可证)**:① A1 OAuth 换 token/RT 导入/重新授权出站 JA3+UA;② A2 批量设 TLS 后账号 extra 生效 + 前端点测;③ **quota 出站 JA3(尤其无 profile 回落 plain Do 的 Cloudflare 风险)**;④ 迁移 158 生产库;⑤ 各前端 UI 点测。

---

## 独立审查 + MEDIUM 修复(2026-07-02)

> 用户要求审查移植是否有问题、是否影响现有项目。6 路并行 agent 审查 + 容器实测,结论:**移植整体正确、可安全上线,未启用账号零影响**;发现并修复 1 个 MEDIUM,其余仅 LOW(非阻塞)。

### 静态验证(容器 golang:1.26.4-alpine + 本机 node v22)
- gofmt(核心文件)clean;`go build ./...` BUILD_OK;`go vet ./...` VET_OK。
- `go test`:service 46s / handler 22s / repository / config / routes / cmd/server / tlsfingerprint / model / dto —— **全 ok,无 FAIL**。
- 前端 `vue-tsc --noEmit` → exit 0。

### 审查结论(未启用零影响的四条硬证据)
1. `DoWithTLS(nil)` 首行 `if profile==nil { return Do(...) }`,`http_upstream.go` 本移植未改 → 无 profile 账号走原路径。
2. `resolveTLSProfileForRequest` 无 router 命中 → 逐字回落旧 `ResolveTLSProfile`。
3. gating `IsAnthropicOAuthOrSetupToken`→`SupportsTLSFingerprint` 是严格超集;OpenAI 仍需显式 `enable_tls_fingerprint` extra(存量账号无)→ 不变。
4. WS 连接池每处 checkout 过 `matchesTLSProfile`,不串号;router service 构造函数 reloadFromDB 失败仅 log 不 panic/不阻断启动。
- 触及现有行为仅 2 点:Grok baseline fix(e6ba7d0a=对齐 main,父分支 archive 遗留破损,非本移植引入)+ gating 扩容(需显式 flag)。LOW 项:OpenAI 每请求重复 match router(cosmetic)、ForceNewConn 满池淘汰 profile-agnostic(仅 churn)、random 模式单一 stable pool key(有意粘性)。

### MEDIUM 修复 — CreateAccountModal cookie 建号漏持久化 router — ✅ 完成
- **现象**:`handleCookieAuth`(Anthropic cookie/session-key 建号)写 `enable_tls_fingerprint`+`profile_id` 却漏 `tls_fingerprint_router_id`;router 下拉无条件显示,选了被静默丢弃。
- **深层实证(与 TR 逐行比对)**:非"漏抄一行"。TR 的 `handleCookieAuth`(6302)那段带 `form.platform==='openai' && oauth-based` guard,而 cookie=Anthropic 端点 → TR 该段永不触发(inert 死代码),即 TR 自己 cookie 路径也不写 router。fork 真正原因是**主动把 router 扩到全平台**:后端 Anthropic 网关改 router-aware(TR 走裸 `ResolveTLSProfile`)+ 主提交路径 5636 去掉 OpenAI guard、独立于 `if(tlsFingerprintEnabled)` 写;但 cookie 支路未同步扩 → fork 自身两条 Anthropic 建号路径不一致。
- **修法**:cookie 路径照抄主路径 5635-5638 的独立 router 块(**无平台 guard**,勿照抄 TR 带 guard 版本 —— 那对 Anthropic 仍无效)。改 `CreateAccountModal.vue`(+5 行)。
- **兄弟路径已核实无同类缺口**:EditAccountModal(无 cookie 路径,主 4065-4080 正常)、ReAuth ×2(传后端)、OpenAI OAuth 换码 5025 / 手动 RT 导入 5273(传后端 `CreateAccountFromOAuth` 持久化 Extra)。
- **验证**:`pnpm typecheck` exit 0。
- **待人工(运行时)**:此前所有 JA3/WS/迁移/前端点测项仍留人工(静态不可证)。
