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
| B | ent schema + model + 生成 | ✅ 完成 | `<B待填>` | gofmt✅ generate✅ build✅ vet✅ | 表/列/索引与方案逐项核对一致;go.mod/sum 未被 codegen 污染 |
| C | 迁移 158_add_tls_fingerprint_routers.sql | ⬜ 未开始 | — | — | |
| D | repository(router repo + cache) | ⬜ 未开始 | — | — | |
| E | service(router + collector)+ config | ⬜ 未开始 | — | — | |
| F | handler + 路由 + wire | ⬜ 未开始 | — | — | |
| G | OpenAI HTTP 集成 | ⬜ 未开始 | — | — | 硬骨头;运行时验证留人工 |
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
- commit:`<B待填>`
- 遗留/风险:无。建表 DDL 留待 Phase C 手写迁移(生产以 SQL 文件为准,非 ent auto-migrate)。

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
