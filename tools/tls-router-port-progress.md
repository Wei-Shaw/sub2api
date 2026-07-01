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
| G | OpenAI HTTP 集成 | 🟡 G1+G3 done;G4/native 停(见 Blocker #2) | `21e18825`+`dedb31e6` | gofmt✅ wire-gen✅ build✅ vet✅ test✅ | G1+G3 覆盖 Anthropic+转换转发;**native OpenAI 路径走裸 Do 未覆盖=方案缺口**,记 Blocker #2 待人工 |
| H | OpenAI WS 集成 | ⬜ 未开始 | — | — | 硬骨头;连接池 key 须含指纹 |
| I | cmd/server 优雅关闭 | ⬜ 未开始 | — | — | |
| J | 代码生成 + 编译 | ⬜ 未开始 | — | — | |
| K | 前端 | ⬜ 未开始 | — | — | 工具链不可用则记待人工 |

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

每个 commit 均在 `golang:1.26.4-alpine` 容器内 `gofmt`+`go build ./...`+`go vet`+相关 `go test`(及涉及 ent/wire 处 `go generate`)全绿。**新增单测**:tlsfingerprint(3)、router service(6)、collector(4),均 PASS。

**未完成**:
- **G4 + native-OpenAI TLS 集成**:见 Blocker #2(方案缺口,需人工确认范围)。
- **G5 OAuth token 路径**:见 Blocker #2 第 7 点(需新建 OAuth 接口或评估)。
- **H WS 集成**:未开始(最高风险,连接池 key 须含指纹防串号;方案 §Phase H 有 scope-guard)。
- **I cmd/server 采集器优雅关闭**:未开始(小、独立、安全;collector 已可经 admin API Start/Stop,I 仅补进程退出时优雅 Stop)。
- **J 最终全量生成/编译**:每阶段已局部 `go generate`+全量 `go build` 绿;最终再跑一次确认即可。
- **K 前端**:未开始(~880 行弹窗等;前端工具链未在本批次验证,留人工或后续批次)。

**待人工验证(运行时,见方案 §5)**:迁移 158 测试库幂等;router CRUD+采集器 API+前端;HTTP/WS 抓包 JA3;OpenAI native 路径指纹(取决于 Blocker #2 决策);回归未命中回落;OAuth 换 token。

**如何续作**:先读 Blocker #2 决定 native-OpenAI 集成范围(强烈建议做,否则 OpenAI 主用例无指纹);其余按 §8 H→I→J→K。工作树干净,9 个 commit 均可独立 build 绿,可安全从任一处接续。
