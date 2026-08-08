---
summary: 为单个 OpenAI 账号增加 fallback-only 会话粘性，使低优先级保底账号不会压住恢复可用的主账号池
doc_kind: spec
status: active
review_status: approved
---

# OpenAI 单账号 fallback-only sticky

## 这活为什么干

当前 OpenAI `session_hash` sticky 会在账号仍可调度时优先复用原账号，即使该账号只是低优先级 fallback。结果是主池 AI-INPUT 恢复后，已落到官方 Pro 账号的会话仍会持续消耗官方额度。完成后，默认账号继续保持现有 sticky；只有显式配置为 `fallback_only` 的账号会在存在更高优先级、支持当前模型且可立即接单的账号时退出 `session_hash` sticky。

## 领导已经拍板

- 只降低指定官方账号的 sticky，不改变 AI-INPUT 账号之间的正常 sticky。
- AI-INPUT 支持当前模型且可用时，应优先回到 AI-INPUT；AI-INPUT 不支持模型、限流、不可调度或没有可用并发时，官方账号仍作为 fallback。
- `previous_response_id` 继续强绑定原账号，避免 OpenAI Responses encrypted context 跨账号后无法验证。
- 先在隔离 worktree 开发并完成 review-commit 与本地验证，再按 `merge-main` 流程 `ff-only` 合入本地 `main`；随后部署本机 Sub2API、把指定官方账号设置为 `fallback_only` 并做 live 验证；全部通过后才创建上游 issue 和 PR。不 push `main`，不操作 `myclaw`。

## 我替领导拍的板（待确认）

- 新增账号字段 `openai_session_sticky_mode`，取值仅为 `normal`、`fallback_only`，默认 `normal`。选择 enum 而不是任意浮点权重，避免同一目标出现无法解释的概率性行为；猜错代价是如果领导确实需要连续权重，后续要扩 schema 和评分合同。
- `fallback_only` 只影响 OpenAI `session_hash` 命中，不影响其它平台、`previous_response_id`、已开始的请求或同优先级账号之间的 sticky。
- “更高优先级可用”定义为：同一调度 group 内，账号支持当前平台、模型、endpoint/transport/compact 能力，状态可调度，未处于账号级或模型级 cooldown，且当前有可立即获取的并发 slot。没有立即可用 slot 时继续使用可用的 fallback，避免为了主池排队而闲置保底容量。
- live 验证通过后先创建上游 issue；随后从最新 `origin/main` 建独立 public worktree/branch，只移植本功能的公开提交并独立验证，再以 ready-for-review PR 使用 `Closes #<issue>` 关联。PR 未关闭前只保留 public branch/worktree；本地 patches 和私有 Spec 不进入 public branch。

## 范围与非目标

- 允许修改 OpenAI 账号持久化 schema、账号 admin update/list DTO、前端账号编辑控件、scheduler metadata，以及 legacy/advanced OpenAI `session_hash` 调度路径和对应测试。
- 允许新增一条 versioned migration；历史账号迁移后全部为 `normal`，不改变现有行为。
- 不修改全局高级调度器默认值，不开启“订阅优先”，不把全局 sticky 权重设为 0。
- 不弱化 `previous_response_id`、encrypted context、tool continuation 的账号绑定。
- 允许按 `sub2api-upgrader` 只部署本机 arm64 Sub2API：保留全部 `origin/main..main` local-only patches，只重建 app 容器，保留升级前健康镜像作为回滚点；禁止访问或部署 `myclaw`。
- 允许通过受控 admin API 把 live 指定官方账号设置为 `fallback_only`，并验证字段 round-trip、账号状态、模型能力、priority 和无 secret 泄露；不直接手改数据库。
- 不顺手修改 group binding priority、模型映射、账号 concurrency 或其它账号池配置。

## 改完后是什么样

账号更新接口修改前没有该字段：

```json
{
  "name": "fallback-owner@example.invalid",
  "priority": 200
}
```

修改后可显式设置账号级策略：

```json
{
  "name": "fallback-owner@example.invalid",
  "priority": 200,
  "openai_session_sticky_mode": "fallback_only"
}
```

账号响应返回同一字段；旧客户端不传时保留现值，新建账号不传时写入 `normal`。数据库新增列：

```sql
ALTER TABLE accounts
  ADD COLUMN openai_session_sticky_mode varchar(32) NOT NULL DEFAULT 'normal';
```

字段 writer 是账号 admin update；reader 是账号 DTO、账号编辑页、scheduler metadata 和 OpenAI session sticky 选择器；当前决策用途仅为判断是否允许低优先级 `session_hash` sticky 覆盖当前可立即接单的高优先级候选。update payload 使用可区分 omitted 的 pointer：不传字段必须保持原值，只传该字段不得修改其它持久化字段。未知值必须在 API 边界拒绝，数据库默认值保证历史行行为不变。

## 现状与任务 0

- 2026-08-08 实测本机 Sub2API：官方账号 `accounts.priority=200`，三个 AI-INPUT 为 `1`；官方账号仍可因 `session_hash`/Responses 连续请求再次承接 Sol/Terra。
- 当前源码只提供全局 `openai_advanced_scheduler_weight_session_sticky` 与 `...previous_response`，账号 schema 没有 sticky policy 字段。
- 上游 [#2872](https://github.com/Wei-Shaw/sub2api/pull/2872) 只实现 TTFT/error/concurrency 健康度逃逸；[#3154](https://github.com/Wei-Shaw/sub2api/pull/3154) 修复 Responses session hash；[#2997](https://github.com/Wei-Shaw/sub2api/pull/2997) 固化 HTTP response ID 账号绑定；未发现账号级 fallback-only sticky 实现。
- 2026-08-08 fetch 后仓库根 `main` 为 `27725215b`，相对 `origin/main` ahead 2 / behind 88，且只有用户已有 `.gitignore` 脏改；执行不得接管或覆盖该改动。该数字只作审核时基线，任务 0 必须重新 fetch 后固定真实 tip/差异。

## 任务 0：前提核验

- 结果：先 fetch 并记录 immutable local main/origin main tip、真实 ahead/behind 和完整 local patch 集，再固定 approved Spec 对应的 immutable integration tip；同时确认工作区 ownership、现有账号 schema、两条 OpenAI sticky 路径和 targeted test 基线。
- 依赖：无。
- `write ownership`：默认只读；仅当前提不成立时允许写 integration worktree 根目录的 `BLOCKED.md`，其它路径保持只读。
- 接口：向任务 1/2 提供固定 tip、字段命名、现有 migration 序号、targeted tests 数量与通过基线。
- focused verification：`git fetch origin main --tags && git rev-parse main origin/main && git rev-list --left-right --count origin/main...main && git log --reverse --format='%H %s' origin/main..main && git status --short --branch && git worktree list && rg -n "StickyWeighted|tryStickySessionHit|selectBySessionHash|previous_response_id" backend/internal/service backend/internal/handler && cd backend && go test -tags=unit ./internal/service ./internal/handler/admin`；命令存在、测试退出 0、skip/todo 为 0，根工作区已有 `.gitignore` 改动保持不变；ahead/behind、patch 集、write ownership 或 public branch 基线与审核时事实不同且会改变执行合同时，写入 `BLOCKED.md` 并阻断后续任务。
- 集成顺序：所有后续任务的 ready 前置；失败时阻断全部后续 ready。
- 失败后果：在 integration worktree 的 `BLOCKED.md` 记录 tip、差异和失败输出，不得继续写生产代码。
- 反向验证：在测试中先引用尚不存在的 `OpenAISessionStickyModeFallbackOnly`，保存编译失败证据；实现后同一测试转绿。

## 任务 1：后端账号合同与调度行为

- 结果：新增 versioned migration、Ent/account/service/DTO 字段及校验；legacy 与 advanced scheduler 均实现 `fallback_only` 规则；scheduler snapshot 不丢字段；历史账号默认行为不变。
- 依赖：任务 0。
- `write ownership`：`backend/ent/schema/account.go`、对应生成的 `backend/ent/**`、新 migration、`backend/internal/service/**`、`backend/internal/repository/**`、`backend/internal/handler/admin/**`、`backend/internal/handler/dto/**` 及这些目录的 focused tests。
- 接口：读写 `openai_session_sticky_mode`；只在 OpenAI `session_hash` sticky 命中时读取；输出调度 decision/日志能区分 `fallback_only` 逃逸，不输出 secret。
- focused verification：新增测试必须覆盖 normal 保持 sticky、fallback-only 在高优先级有立即 slot 时逃逸、只有同优先级时保持 sticky、高优先级模型不兼容/限流/满并发时保留 fallback、`previous_response_id` 仍硬绑定、snapshot round-trip；执行 `cd backend && go test -tags=unit ./internal/service ./internal/repository ./internal/handler/admin`，退出 0 且无 skip/todo。
- 集成顺序：任务 2 可按已批准 API 合同并行；后端先于综合验证集成。
- 失败后果：任何路径让 `previous_response_id` 跨账号、未知 enum 被静默接受或历史账号默认改变，均 `BLOCK`。
- 反向验证：把官方测试账号模式改回 `normal` 时，高优先级恢复场景必须重新命中原 sticky；恢复 `fallback_only` 后转绿。

## 任务 2：前端账号编辑设置

- 结果：OpenAI 账号编辑页可选择“正常粘性”或“仅作保底时粘性”并回显当前值；非 OpenAI 账号不显示该控件；API types 与后端字段一致。创建页和批量编辑不新增该设置。
- 依赖：任务 0，使用本 Spec 已批准的 API 合同。
- `write ownership`：`frontend/src/api/admin/accounts.ts`、`frontend/src/types/index.ts`、`frontend/src/components/account/EditAccountModal.vue`、该组件现有测试、`frontend/src/i18n/locales/**`。
- 接口：编辑表单发送 `openai_session_sticky_mode` 并回显服务端值；字段缺失或保存后 round-trip 不一致时按 API 合同失败并提示，不得伪造 normal 或把静默忽略当成功。只有用户提交该编辑表单时才随既有 payload 发送，不产生批量覆盖。
- focused verification：执行 `cd frontend && pnpm typecheck && pnpm test:run --runInBand`（若仓库 test runner 不接受该参数，任务 0 记录真实命令并只替换参数）；退出 0，相关控件测试覆盖 OpenAI/非 OpenAI 与回显/提交，创建和批量编辑 diff 为零，skip/todo 为 0。
- 集成顺序：后端之后集成；字段冲突退回本任务 writer，不在父 worktree 临时改合同。
- 失败后果：字段名漂移、非 OpenAI 显示、默认值导致无意批量覆盖或无法回显均 `BLOCK`。
- 反向验证：删除 API type 字段时 typecheck 必须失败；恢复后转绿。

## 任务 3：综合验证、review-commit 与本地 main 合入

- 结果：integration worktree 综合验证通过，普通独立 reviewer `PASS`，按 `review-commit` 形成原子提交；随后执行 `merge-main` Gate，rebase 到最新本地 `main`、重新验证和独立 review 后 `ff-only` 合入。
- 依赖：任务 1、任务 2。
- `write ownership`：仅允许修复任务 1/2 reviewer 指出的阻塞问题、更新本 Spec/`PROGRESS.md`/`BLOCKED.md` 证据和执行本地 Git cutover；不得 push `main`。
- 接口：输出 immutable feature tip、commit list、验证证据和本地 `main` 新 tip，供任务 4 构建镜像。
- focused verification：`git diff --check && cd backend && go test -tags=unit ./... && cd ../frontend && pnpm typecheck && pnpm test:run`，全部退出 0；reviewer `PASS`；提交后 feature worktree clean；`merge-main` 后 `main == feature_tip` 并再次运行同一 broad verification。
- 集成顺序：任务 1/2 后执行；通过后任务 4 ready。
- 失败后果：验证、review、rebase 或 ff-only 任一失败时不得部署；保留 branch/worktree 和恢复点继续修复。
- 反向验证：至少保留任务 1/2 的 RED→GREEN 证据；merge Gate 故意使用旧 tip 做一次 compare-and-swap 检查并证明漂移会阻断，随后用最新 immutable tip 通过。

## 任务 4：部署本机、设置账号与 live 验证

- 结果：按 `sub2api-upgrader` 基于正式 upstream tag 和本地 `main` 全部 local-only patches 构建新的 arm64 image，只切换本机 app 容器；通过 admin API 把指定官方账号设置为 `fallback_only`；真实流量证明 AI-INPUT 可用时新 `session_hash` 请求不再持续命中该官方账号，AI-INPUT 不可用时 fallback 仍成立。
- 依赖：任务 3。
- `write ownership`：`/Users/admin/sub2api/docker-compose.yml` 仅允许更新 `services.sub2api.image`；本机 Docker app container/image；本机 Sub2API 指定账号的 `openai_session_sticky_mode`；`docs/sub2api/运行与升级.md` 记录 live image 与回滚镜像。不得修改 PostgreSQL/Redis 容器、其它账号字段或 `myclaw`。
- 接口：部署输入是任务 3 已验证的本地 `main` tip 与完整 patch manifest；输出新/旧 image、binary version、health、restart count、账号字段 round-trip 和脱敏调度证据。
- focused verification：按 `sub2api-upgrader` 完成 patch 枚举、targeted tests、frontend build、Linux arm64 binary/image 自检、`/health`、container health/restart count。live 写入前以本地非敏感 account ID 定位目标并 fail closed 断言：唯一记录、platform=openai、type=oauth、priority=200、初始 mode 已记录；不得输出 name/email。admin API GET/PUT/GET 证明仅提交 `openai_session_sticky_mode`，其它字段指纹不变且 mode=`fallback_only`。使用虚构 session 标识的受控 live 请求和 usage/account 只读查询证明主池可用时选择 priority 1；AI-INPUT 不可用时的 fallback 由隔离自动测试证明，不修改生产 AI-INPUT 状态制造故障。
- 集成顺序：本地 main 合入后执行；live 全部通过后任务 5 ready。
- 失败后果：容器不健康、migration/API round-trip 失败、local patches 缺失或主路径行为不成立时，立即切回升级前健康 image 并恢复健康。账号 post-write 或调度验证失败时，仅通过 admin API PUT 回已记录的原 mode 并重新 GET/验活；恢复失败立即阻断并保留证据，不得直接改 DB。
- 反向验证：部署前在旧 live API 上读取新字段/提交新 enum，必须得到字段缺失或不支持的预期 RED；部署后同一路径 GREEN。真实 fallback 失败分支只用自动测试或隔离环境，不破坏 live 主池。

## 任务 5：上游 issue 与 PR

- 结果：live 验证通过后创建不含私有信息的上游 issue；从最新 `origin/main` 创建 public branch，只移植公开功能提交并独立验证，push 到 fork 后创建 ready PR。PR 正文包含问题、行为合同、兼容边界、migration、测试、live 脱敏结论和 `Closes #<issue>`。
- 依赖：任务 4。
- `write ownership`：独立 public worktree/branch、GitHub issue、fork public branch、upstream PR；不得 push `main` 或携带 local-only patches。
- 接口：issue/PR 只引用虚构账号和公开代码，不包含真实邮箱、token、请求体、私有日志或 server_setup 路径。
- focused verification：`origin/main` 是 `public_tip` 祖先；`origin/main..public_tip` 只含本功能公开提交，路径/commit 来源/Spec/真实邮箱/secret 扫描零异常；在 public worktree 重新运行 backend/frontend broad verification；issue 与 PR URL 可读取，PR head 精确指向 `public_tip`，checks 已启动；仅 public branch/worktree 因开放 PR 保留。
- 集成顺序：最后。
- 失败后果：secret/privacy 扫描失败时禁止创建；push/issue/PR 临时失败时保留本地完成结果重试，不回滚已健康部署。
- 反向验证：创建前对 diff、issue body 和 PR body 运行真实邮箱/secret 模式扫描并要求零命中。

## 协作与集成

- Sol 先创建唯一 integration worktree，并从 approved Spec 固定 immutable integration tip；后端与前端 lane 从同一 tip 创建独立 branch/worktree，分别由 worker 实现并由直接 reviewer 独立审核。
- `ready = 依赖已完成`；任务 0 通过后任务 1/2 可并行，write ownership 不重叠。Terra `PASS` 前不得集成。
- 综合验证通过前保留 lane worktree；`merge-main` 只做本地 `ff-only`，不 push `main`。本地 cutover 和部署完成后可按 Gate 清理 integration/lane 资源；上游 PR 未关闭前只保留从 `origin/main` 创建的 public branch/worktree。

## 规矩

- 不得用全局 sticky 权重、关闭 sticky 或“订阅优先”模拟单账号行为。
- 不得解析日志、prompt 或自由文本决定账号策略；只读 typed account 字段、typed request capability 和 scheduler state。
- 不得打印 credentials、request body、OAuth token、API key 或真实邮箱；测试使用虚构账号。
- 不得 skip/todo、放宽断言、mock 掉被测调度器、删除测试、修改验收阈值、使用 `|| true` 或降低测试数。
- 不新增第三方依赖；同一验收连续失败 3 次停止该路径并记录 `BLOCKED.md`。

## 完成条件

- 对一个 `fallback_only`、priority 200 的测试账号，存在支持模型且有立即 slot 的 priority 1 账号时，下一次 `session_hash` 请求选择 priority 1；priority 1 不可用时选择 fallback；normal 模式与同优先级 sticky 行为不变。
- `previous_response_id` 全部既有回归测试继续通过；新字段 migration、单字段 omission/round-trip、snapshot round-trip 和前端账号编辑页回显/提交均有自动测试，创建页与批量编辑无改动。
- broad backend/frontend 验证、独立 reviewer、review-commit、本地 `merge-main` 后复验、本机部署与回滚点、live 账号设置/调度验证、GitHub issue/PR 全部有实际证据；任何一项缺失不得宣称完成。
- 根工作区原有 `.gitignore` 改动与其它无关 worktree/branch 不被修改或提交；私有 Spec 只保留审核所需的非 secret、脱敏 live 基线且不得进入 public branch；public branch、commit、issue 和 PR 不得包含 secret、真实邮箱、私有路径或 live 数据。

## Goal

- 验收摘要: 自动测试用虚构的 priority 1 主账号与 priority 200 fallback 账号跑 OpenAI session sticky 调度到账号选择边界；随后受控部署本机并只更新目标账号 mode；看到主账号恢复即回切、主账号不可用才保底、normal/previous-response 不回归、live 单字段写入可恢复、公开 PR 不含私有内容才算通过。
- 执行依据: `docs/superpowers/specs/2026-08-08-openai-account-fallback-only-sticky.md` 的任务 0 → 5。
- 真实验收: backend scheduler tests｜legacy + advanced 两条路径｜`cd backend && go test -tags=unit ./internal/service ./internal/repository ./internal/handler/admin`｜成功=退出 0 且 RED→GREEN 场景齐全｜证据=`PROGRESS.md`。
- 真实验收: admin UI/API｜账号编辑页与单字段 round-trip｜`cd frontend && pnpm typecheck && pnpm test:run`｜成功=退出 0、创建/批量编辑无 diff 且无 skip/todo｜证据=`PROGRESS.md`。
- 非回归: `cd backend && go test -tags=unit ./... && cd ../frontend && pnpm typecheck && pnpm test:run`。
- 边界: 允许测试、feature push、GitHub issue/PR、本地 ff-only main、本机 app 容器部署和指定账号单字段 admin API 更新；禁止直接改 DB、修改其它账号字段、访问/部署 myclaw 和 push main。
