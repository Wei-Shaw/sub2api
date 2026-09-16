# Kiro 渠道（本 fork 附加）

本仓库是 [Wei-Shaw/sub2api](https://github.com/Wei-Shaw/sub2api) 的 fork，在上游之上合入了 Kiro 渠道接入。

上游 PR [#5058「支持 Kiro GPT」](https://github.com/Wei-Shaw/sub2api/pull/5058) 于 2026-07-31 被关闭，
理由是「之前有几个熟悉的项目引入 kiro 反代导致项目被 DMCA 下架，因此不会考虑 kiro 相关的」。
PR 作者转为在 [`nianzs/sub2api`](https://github.com/nianzs/sub2api) 长期维护并持续与上游合并，
本 fork 的 Kiro 代码即来自该仓库。

> ⚠️ Kiro 是 AWS 的产品，反代其接口可能违反 AWS 服务条款，上游正是因下架风险拒绝这套代码。
> 是否启用由使用者自行判断并承担风险。

## 三个远端

| remote | 指向 | 用途 |
|---|---|---|
| `origin` | `sangkaka/sub2api` | 本 fork，日常推送与开 PR |
| `upstream` | `Wei-Shaw/sub2api` | 官方上游，只读同步 |
| `kiro` | `nianzs/sub2api` | Kiro 上游，只读同步 |

## 同步流程

`nianzs/sub2api` 会持续跟上游合并（有时甚至**领先**——2026-09 那次它已经包含了
`upstream/main` 并在其上加了 Adobe），所以增量同步的冲突面能长期维持在几十个文件量级。

日常同步走 `/sync-upstream`（`.claude/commands/sync-upstream.md`）。手工做的话：

```bash
git fetch upstream kiro
git checkout -b sync/upstream-vX.Y.Z main
git merge upstream/main      # 第一段：只和官方对齐
# 解冲突 → 重跑生成物 → 本地验证 → commit
git merge kiro/main          # 第二段：只剩 kiro 自有增量
# 同上 → commit → push 开 PR，让 CI 先绿再合 main
```

**两段合并的顺序不要改**：先 upstream 后 kiro，冲突被拆成两批，
每批都能单独跑绿、单独回滚；反过来或一次合完会把两类语义揉在一个提交里。

### 冲突解决的既定规则

这些规则是首次合并时定下的，重复同步时照做即可，不必每次重新判断：

1. **生成物不手工合**：`backend/cmd/server/wire_gen.go` 与 `backend/ent/` 下的生成文件，
   随便留一版让文件语法完整，然后 `cd backend && make generate` 重新生成，以生成结果为准。
2. **平台枚举/白名单一律取并集**，保留 fork 的 `kiro` / `adobe`，也保留上游后来加的平台
   （截至 v0.2.5 共 12 个：anthropic / openai / gemini / antigravity / kiro / grok / adobe /
   kimi / zhipu / deepseek / minimax / opencode_go）。
   漏掉任何一处都会静默不生效或直接编译失败，合完必须逐个回看：
   - `backend/internal/domain/constants.go`、`internal/service/domain_constants.go`
   - `internal/service/domain_constants.go` 的 `AllowedQuotaPlatforms`（多处已改为从它派生）
   - `internal/model/error_passthrough_rule.go` 的 `AllPlatforms()`
   - `internal/handler/admin/group_handler.go` 的 **3 处** `binding:"...oneof=..."`
     （Create / Update 含 `kiro`，composite 的 `target_platform` 不含）
   - `internal/service/scheduler_snapshot_service.go` 的 `schedulerSnapshotPlatforms()`
   - `internal/repository/simple_mode_default_groups.go` 的 `requiredByPlatform`
   - `internal/service/oauth_only_platforms.go`
   - `backend/ent/schema/user_platform_quota.go` 的 `Validate`
   - `backend/ent/schema/channel_monitor.go` / `channel_monitor_request_template.go` 的
     `field.Enum("provider")`（**不含 adobe**，与 kiro 的 239 号迁移一致）
   - 前端：`frontend/src/constants/platforms.ts` 的 `CONCRETE_PLATFORM_OPTIONS`、
     `frontend/src/types/index.ts` 的 `GroupPlatform` / `AccountPlatform` 联合、
     `frontend/src/utils/platformColors.ts` 的 10 个穷尽 `Record<Platform, string>`、
     `frontend/src/utils/keyGroupProviders.ts` 的 `Record<GroupPlatform, KeyGroupProvider>`
     （后两者是**穷尽映射**，漏 key 直接 TS 编译失败）。
     其余选择器都从 `platforms.ts` 派生，**不要**把 fork 侧那几份硬编码数组抄回来。
3. ~~scheduler 快照测试取 fork 侧的参数化断言~~ —— **本条已作废**（2026-09 同步 v0.2.5 时）。
   上游自己也把断言参数化了（`schedulerCanonicalBucketCount()` /
   `schedulerCanonicalAccountQueryCount()`），kiro 侧则进一步把
   `schedulerSnapshotPlatforms()` 改成由 `AllowedQuotaPlatforms` 派生。
   现在的规则是：**这一族测试整体取 kiro 侧**，平台数变化只需改 `AllowedQuotaPlatforms`，
   断言会自洽。fork 自有的守卫用例 `service/scheduler_snapshot_platforms_test.go` 保留。
4. **网关/调度/计费共享文件以上游为骨架**，把 fork 的 `kiro` 分支逻辑并进去，
   不要整文件 `--theirs` 覆盖掉上游的修复。
5. **fork 的私有改动一律丢弃**：根目录 agent 工作笔记（`findings.md`/`progress.md`/`task_plan.md`）、
   README 的 fork 品牌与赞助商、`Dockerfile` 的 npmmirror 默认源、`.gitignore` 里指向不存在
   文件的 `scripts/` 规则、`release.yml` 被剥掉的 VERSION 同步任务。
6. **绝不接受 fork 对已应用迁移的就地修改**。`migrations_runner` 按 `filename + SHA256`
   校验不可变性，改动已应用的迁移文件会让已部署库直接启动失败。
   fork 曾就地改过 `157_user_platform_quotas_add_grok.sql` 和
   `224_user_platform_quotas_add_cn_providers.sql`——同一问题已由前向迁移
   `227_user_platform_quotas_restore_kiro.sql` 修好，就地改是多余且有害的。
7. **上游每次加平台都可能把 `kiro` / `adobe` 从 CHECK 约束里抹掉**。上游的
   `237_add_minimax_platform.sql` / `238_opencode_go_platform.sql` 用 `DROP CONSTRAINT` +
   重建全量列表的写法加新平台，抄的是上游自己的平台清单，于是第三次重复了 224 号事故
   （145 → 224 抹掉 → 227 补回 → 237/238 又抹掉）。约束缺平台时，注册预填充配额的
   多行 INSERT 整条违约 → 快照 fail-open 仅 warn → 新用户拿到零条配额记录。
   **每次同步完都必须跑 `go test -tags=unit ./migrations/`**：
   `TestUserPlatformQuotaPlatformCheckFinalStateCoversAllPlatforms` 会取文件名序最后一个
   重建该约束的迁移并校验其平台列表，是这条链路唯一的自动护栏。
   当前终态由 kiro 的 `239_fork_platform_constraints_superset.sql` 提供（四张表、12 平台）。

## 代码位置

**后端**

| 路径 | 内容 |
|---|---|
| `backend/internal/pkg/kiro/` | 协议翻译（`translator.go`）、OAuth、指纹、签名、websearch、image tokens |
| `backend/internal/pkg/kirocooldown/` | 账号冷却状态存储 |
| `backend/internal/service/kiro_*.go` | runtime、模拟缓存、credits 用量、token provider/refresher、错误分类、profile resolver |
| `backend/internal/handler/admin/kiro_oauth_handler.go` | 管理端 OAuth 接口 |

**前端**：`frontend/src/api/admin/kiro.ts`、`composables/useKiroOAuth.ts`、
`utils/kiroAccount.ts`、`constants/kiroRegions.ts`。

**管理端路由**（`backend/internal/server/routes/admin.go`）：
`/admin/kiro/oauth/{auth-url,idc-auth-url,exchange-code,refresh-token,import-token}`、
`/admin/accounts/kiro/default-model-mapping`。

## 请求怎么走到 Kiro

**Kiro 没有独立的网关路径前缀**。不像 antigravity 有 `/antigravity/v1` + `middleware.ForcePlatform`，
Kiro 走的是标准 Anthropic 入口（`/v1/messages`），由 **API Key 所属分组的 `platform` 字段**
决定是否转发到 Kiro（`isKiroGroup`，`internal/service/gateway_service.go`）。

也就是说：建一个 `platform=kiro` 的分组，把 Kiro 账号挂进去，用该分组下发的 Key 打
`/v1/messages` 即可，客户端侧不需要改 base URL 后缀。

## 账号类型

`platform=kiro` 下 `type=apikey` 有两种语义，靠 `credentials.base_url` 是否为空区分
（见 `frontend/src/utils/kiroAccount.ts`）：

- **直连 AWS**（`base_url` 为空）：用 `ksk_` 直连 `q.{region}.amazonaws.com`，展示 Kiro credits。
- **外部中转**（`base_url` 非空）：转发到外部 Anthropic 兼容上游 `{base_url}/v1/messages`，
  不直连 AWS、无 Kiro credits，作为分组兜底/灾备。

另有 `type=oauth`（AWS Builder ID / IDC）与 Token 导入。

## 分组级配置

`platform=kiro` 的分组独有以下字段（`backend/ent/schema/group.go`），对其他平台不生效：

| 字段 | 说明 |
|---|---|
| `kiro_cache_emulation_enabled` | 是否为 Kiro 流量模拟 Anthropic Prompt Cache 用量 |
| `kiro_cache_emulation_ratio` | 模拟比例，0–1 |
| `kiro_cache_emulation_mode` | `uniform` / `independent` |
| `kiro_cache_creation_emulation_ratio` | independent 模式下的 cache creation 比例 |
| `kiro_cache_read_emulation_ratio` | independent 模式下的 cache read 比例 |
| `kiro_auto_sticky_enabled` | 是否启用自动会话粘性路由 |
| `kiro_sticky_session_ttl_seconds` | 粘性绑定 TTL |
| `kiro_endpoint_mode` | `q` = AWS Q（`q.{region}.amazonaws.com`）/ `krs` = Kiro Runtime Service（`runtime.us-east-1.kiro.dev`） |

## 其他 fork 私有能力（KIRO.md 长期漏记）

这份文档原本只写 Kiro，但本 fork 相对 `Wei-Shaw/sub2api` 还带着几条独立链路。
同步时它们和 Kiro 一样需要保住，列在这里免得再靠 `git log` 现查。

### prompt-rules（提示词规则）

管理端按「分组 + 模型」配置规则，在网关入口对请求体做 prepend / append / replace
（plain 与 regex 两种匹配），跨 Anthropic / OpenAI-Chat / Responses / Gemini 四种协议。
作者是 `nianzs`，随 Kiro 一起进来，但与 Kiro 无关。

- 后端：`backend/ent/schema/prompt_rule.go` + 生成物、`internal/model/prompt_rule.go`、
  `internal/repository/prompt_rule_repo.go` / `prompt_rule_cache.go`、
  `internal/service/prompt_rule_service.go` / `prompt_rule_inject.go`、
  `internal/handler/prompt_rule_injection.go`、`internal/handler/admin/prompt_rule_handler.go`
- 路由：`internal/server/routes/admin.go` 的 `registerPromptRuleRoutes` → `/admin/prompt-rules`
- 迁移：`180_prompt_rules.sql`、`188_prompt_rule_replace_action.sql`
- 前端：`api/admin/promptRules.ts`、`views/admin/PromptRulesView.vue`、
  `i18n/locales/{en,zh}/admin/promptRules.ts`
- **合并时最容易漏的是 9 个注入点**（上游对这批文件改动很频繁）：
  `handler/gateway_handler.go`、`gateway_handler_chat_completions.go`、
  `gateway_handler_responses.go`、`handler/openai_chat_completions.go`、
  `handler/openai_gateway_handler.go`（4 处）、`handler/gemini_v1beta_handler.go`。
  合完 `grep -rn injectMatchingPromptRules backend/internal/handler` 应当有 10 行
  （9 个调用点 + `prompt_rule_injection.go` 里的实现）。

### 原生控件禁用与守卫

全站禁用原生 `<select>` 与 `window.alert/confirm/prompt`，统一走
`components/common/Select.vue` 与 `appStore.showConfirm` / `ConfirmDialog`。
守卫是 `frontend/src/__tests__/nativeControls.spec.ts`（扫描全部生产源码，零容忍）。

上游每次加 UI 都会带进新的原生控件，解冲突时的正确做法是**取上游骨架、再把替换套一遍**，
然后跑这条用例。给用例写 stub 时复用 `EditAccountModal.spec.ts` 里的 `SelectStub`
（它渲染真实 `<select>` 并 `v-bind="$attrs"`，`data-testid` 和 `setValue()` 都能照常用）。

### 其他

Codex Responses remote compaction v2、Grok→Claude Code 风格护栏、
管理员删除 Ops 错误日志、Responses 自定义工具调用、用量清理按模型维度、
按筛选结果全选账号、xxhash 哈希、`anthropictokenizer` 包，
以及三条防合丢守卫用例（`service/wire_merge_preservation_test.go`、
`service/scheduler_snapshot_platforms_test.go`、
`frontend/src/i18n/__tests__/forkLocalePreservation.spec.ts`）。

## Adobe Firefly 渠道（随 kiro 远端引入）

2026-09 同步时随 `nianzs/sub2api` 引入的第 12 个平台 `adobe`：Firefly 出图，
cookie 条件合并 token、网关与后台刷新共用 provider、出图准入与共享并发槽、
合成分组调度与用户配额纳入 Adobe。

> ⚠️ 与 Kiro 同类的合规风险：反代 Adobe Firefly 同样可能违反其服务条款，
> 是否启用由使用者自行判断并承担风险。

注意 `adobe` **不在** `channel_monitors` / `channel_monitor_request_templates` 的
provider 枚举里（ent schema 与 kiro 的 239 号迁移一致），只在
`user_platform_quotas` 与 `composite_model_routes` 的 CHECK 约束里。

## 已知约束与踩坑

### govulncheck：webp 解码器的可达性

`backend/internal/pkg/kiro/image_tokens.go` 在 `init` 里注册了 `golang.org/x/image/webp` 解码器。
上游代码（`internal/service/user_service.go` 的头像压缩）虽然调用 `image.Decode`，但因为
**没有任何包注册 webp**，govulncheck 的可达性分析判定 x/image 的 webp 漏洞不可达，
上游 CI 因此是绿的。

合入 Kiro 后 webp 被注册，同样的漏洞立刻变成可达路径，`backend-security` 直接报红
（GO-2026-6222 / GO-2026-5061 / GO-2026-4961）。已通过把 `golang.org/x/image` 升到
`v0.45.0` 解决。

**结论**：本 fork 的 `x/image` 版本下限比上游高，同步时若上游 go.mod 把它降回去，
需要重新升上来，否则 CI 必红。

### 本地自验必须覆盖全部 build tag

仓库里有 433 个 `//go:build unit`、78 个 `//go:build integration`、3 个 `//go:build e2e` 文件，
不打 tag 的 `go test ./...` **一个都不会编译**。CI 的 `test` job 跑的是
`make test-unit` + `make test-integration`，只跑裸 `go test ./...` 必然漏检：

```bash
cd backend
go test ./...                      # 无 tag
go test -tags=unit ./...           # CI: make test-unit
go vet  -tags=integration ./...    # 至少保证编译；真跑起来要 PG
go vet  -tags=e2e ./...
```

首次合并时就是漏了这一步，连吃两次 CI 红灯：先是 `unit` 下的
`TestUpdateUserPlatformQuotas_Success` 断言数不对，再是 `integration` 下的孤儿测试编译失败。

### fork 遗留的孤儿测试

`nianzs/sub2api` 的 `integration` 套件在他们那边就是编译不过的。fork 提交
`ddf063352「feat(ops): 错误日志 key 归因与早退字段补全」` 加了一整条「已删除 key 归因」
链路（`opsRepository.LookupDeletedKeyAudit` + `service.OpsPort` + `OpsService` +
`ops_error_logger.go` 调用方），后来在某次上游合并里实现被整条覆盖掉，
只剩 `ops_repo_lookup_deleted_key_audit_integration_test.go` 成了孤儿。

合并时已删除该测试文件，且**不恢复实现**——它在合并后的代码里零调用方，
恢复等于把一个 fork 自己已经丢弃的私有功能重新引入，不属于 Kiro 接入范围。
上游保留了 `deleted_api_key_audits` 表（迁移 145）与写入侧，缺的只是**反查**那一半；
若将来需要「认证失败时用明文反查已删除 Key 的原所有者」，实现可从 `git show ddf063352` 取回。

### 分组模型白名单：v0.2.5 的语义变更

上游 `feat(groups)!`（迁移 `235_group_model_allowlist.sql`）把 `groups.models_list_config`
**原地重命名**成 `model_allowlist`，数据原样保留，但语义从「只影响 `/v1/models` 展示」
变成了「请求准入」——由 `internal/server/middleware/group_model_allowlist.go` 在
`/v1`、`/v1beta`、根别名、`/backend-api/codex/*`、antigravity 各入口统一拦截，
不在白名单里的模型直接 404。

对 Kiro 分组尤其要留意：Kiro 把 `*-thinking` 当作独立 model ID
（`internal/pkg/kiro/models.go`），管理员以前填的「展示列表」通常没列全，
升级后这些模型会被拒。**升级前务必核对 `groups.model_allowlist` 里 `enabled=true` 的分组**，
必要时先把 `enabled` 置回 false 再逐个确认。

### 迁移文件同号不同名

合入的 Kiro 迁移与上游存在同号不同名的情况（例如两个 `153_*.sql`）。
`migrations_runner` 按**文件名字典序**执行、按 `filename + checksum` 去重，
因此可以共存，不要为了「看着整齐」重编号——重编号等于换文件名，
会让已部署库重复执行同一段 DDL。

新增的 Kiro 迁移：`135` / `145` / `151` / `152` / `153`×2 / `192` / `227` / `239`×2
（`239_add_kiro_to_platform_checks.sql` 与 kiro 的 `239_fork_platform_constraints_superset.sql`，
后者按文件名序在后、且是前者的超集，终态以后者为准）。
