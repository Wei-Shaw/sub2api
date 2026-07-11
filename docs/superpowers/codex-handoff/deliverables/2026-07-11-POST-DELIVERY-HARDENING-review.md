# Sub2API 交付后产品硬化审查包

日期：2026-07-11
执行目录：`D:\sub2api-trunk`
分支：`wujie/video-capture-moat-20260702`
起始参考：`36de34b81c3a0981fd02fc1dc945d7dc60b587be` + 用户已有未提交交付修复

## 1. Verdict

**条件通过（内部 mock 可演示；不是生产 READY）。**

配置、worker 诊断、Qcanvas 契约、账务/对账目标 integration、前端 lifecycle、pnpm frozen lock、三份 Compose 解析和全量普通 Go 测试已得到本轮新鲜证据。官方根 Dockerfile 与 `Dockerfile.delivery` 的新镜像构建均被本机 Docker 镜像代理 `docker.xuanyuan.me` 的 HTTP 429 挡在基础镜像解析阶段，未进入项目构建步骤，因此本轮不能声称新镜像、容器 `/health` 或老板电脑完整一键试跑已重新通过。

未调用真实/付费 Provider，未触碰生产数据、真实支付或线上部署；未读取或打印交付包 `.env`、`SECRETS-BACKUP`、API Key、token、cookie。

## 2. 真相源阅读记录

业务代码改动前，按任务书第 3 节顺序完成读取：

| 顺序 | 真相源 | 初始状态 / 处理 |
|---:|---|---|
| 1 | `docs/superpowers/codex-handoff/deliverables/2026-07-11-DELIVERY-REHEARSAL-PROGRESS.md` | 已读；作为 mock 彩排事实源 |
| 2 | `docs/superpowers/specs/2026-07-10-reliability-core-design.md` | 已读；作为可靠性不变量主源 |
| 3 | `docs/superpowers/plans/2026-07-10-reliability-core-implementation.md` | 已读 |
| 4 | `docs/superpowers/codex-handoff/deliverables/2026-07-10-RELIABILITY-CORE-review.md` | 已读 |
| 5 | `docs/superpowers/codex-handoff/deliverables/2026-07-10-GROK45-RELIABILITY-AUDIT.md` | 已读 |
| 6 | `docs/api/video-gateway-contract.md` | 已读；初始乱码，本轮重写为可读 UTF-8 并加契约测试 |
| 7 | `docs/api/image-gateway-contract.md` | 已读；初始存在历史编码问题，本轮未改图像业务 |
| 8 | 本任务书 | 已读 |
| 9–16 | 根级入口、基线、现实、产品不变量、架构护栏、质量门禁、当前目标、最新 HTML | 初始全部缺失；按规则先重建最小可用真相源，再改业务代码 |
| 17 | `deploy/README.md`、`deploy/DOCKER.md`、`DEV_GUIDE.md` | 已读；前两者本轮更新 |

重建路径：`00_START_HERE.md`、`01_PROJECT_BASELINE.md`、`02_CURRENT_REALITY_STATUS.md`、`PRODUCT_INVARIANTS.md`、`ARCHITECTURE_GUARDRAILS.md`、`CODE_QUALITY_GATE.md`、`docs/goals/03_CURRENT_GOAL.md`、`docs/reviews/LATEST_REVIEW_PACKAGE.html`。维护规则：状态变化先更新现实/目标，再同步本审查包与 latest HTML；mock、外部门禁和生产证明必须分层记录。

## 3. 变更摘要（按产品风险）

### 启动、配置与诊断

- 为 video gateway / reliability core 补齐稳定 defaults、数值与组合校验、环境变量覆盖测试。
- 三份主线 Compose、`.env.example` 与 `config.example.yaml` 对齐同一组配置契约；示例不含真实密钥。
- 缺少专用视频加密密钥时，错误直接指向 `VIDEO_GATEWAY_ENCRYPTION_KEY`；仍禁止回退到 TOTP/JWT 密钥域。
- worker 默认开启；若显式关闭，启动日志固定输出 `video_gateway_worker_disabled` 和 queued 任务不会推进的后果。

### Docker 与老板电脑路径

- 修复 `frontend/pnpm-lock.yaml` 顶层 overrides 与 `package.json` 不一致，pnpm 9 frozen-lock 在隔离容器内通过。
- Windows 推荐 Compose 使用 Postgres/Redis/Sub2API named volumes；文档解释 NTFS bind mount 的 `initdb/chmod` 风险、首次 admin 合规确认、API Key 分组绑定、mock-only 边界和回滚。
- 明确根 Dockerfile 是主线/CI 从源码构建；`Dockerfile.delivery` 仅消费预构建 dist 的离线交付适配，不取代主线。
- entrypoint shell 语法、现有缓存交付镜像中的 `su-exec`/可执行文件/非 root 用户静态冒烟通过；本轮新镜像构建受外部 429 阻塞。

### Qcanvas 契约与前端生命周期

- 修复 Seedance 普通 API-key 任务被错误标成 tiny-trial 的边界漂移；只有实际 `trial_mode=tiny_real` 才返回 tiny-trial boundary。
- 重写视频网关契约，锁定 `id/status/result_url/error_message/provider` 和 mock 边界字段/reason；未把 mock 写成生产能力。
- 恢复 6 项 lifecycle 单测：单请求、并发刷新合并、指数退避、页面隐藏暂停、scope 销毁 abort、archiving 慢轮询、local asset 优先与交付失败不回退。

### Integration 可信性

- 统一 reaper `$3::text`，拆分 PostgreSQL prepared execution 中的多语句 fixture。
- billable fake 测试自建离线占位 provider，避免依赖 demo seed；outbox handler 覆盖 complete/dead。
- 无 Docker 默认非零退出；CI 即使设置跳过变量也不能跳过。仅显式本地诊断 `SUB2API_ALLOW_INTEGRATION_SKIP=1` 可输出机器可读 `INTEGRATION_SKIPPED_DOCKER_UNAVAILABLE`。
- 目标矩阵在新 Testcontainers Postgres 中通过。扩大到全部历史 repository integration 时仍发现共享数据库污染（固定邮箱、空表/ID 假设）；该全包不是本轮绿色门禁，见残余风险。

## 4. D1–D14 处理结果

| 风险 | 状态 | 方案与证据 |
|---|---|---|
| D1 配置丢失 | 已修复 | defaults/Validate/env/example/Compose 契约测试覆盖 |
| D2 加密密钥现场硬失败 | 已修复 | 模板、Compose、错误文案三位一体；专用密钥测试 |
| D3 worker 关闭导致假死 | 已修复 | 默认 true；显式关闭有稳定事件和后果说明 |
| D4 根 Docker frozen overrides | 代码已修复，镜像复核受阻 | 隔离 pnpm 9 frozen-lock 通过；根镜像被语法镜像 429 阻塞 |
| D5 交付镜像缺 `su-exec` | 条件通过 | Dockerfile 已包含；entrypoint 语法与缓存镜像冒烟通过；新镜像受 429 阻塞 |
| D6 Windows NTFS bind mount | 已修复 | Windows 推荐 Compose 使用 named volumes，三份 Compose config 通过，文档含迁移边界 |
| D7 首次 admin 合规门闸 | 已修复 | `deploy/README.md` 增加首次登录、合规确认、分组 Key 检查清单 |
| D8 Qcanvas reason/边界漂移 | 已修复 | mapper 区分 production/tiny trial；handler/routes/doc 契约测试通过 |
| D9 reaper `$3` 类型 | 已修复 | SQL 显式 `::text`；目标 Postgres integration 通过 |
| D10 reconciliation 多语句 | 已修复 | fixture 一次一语句；过期预约残留多语句也一并拆分 |
| D11 demo provider 污染 | 已修复 | billable fake 自建合成 provider，占位凭据不发网络请求 |
| D12 无 Docker 假绿 | 已修复 | fail-closed policy 单测；CI 不允许跳过 |
| D13 真相源断链 | 已修复 | 8 个最小真相源/导航文件已重建 |
| D14 lifecycle 单测丢失 | 已修复 | 6 项 composable 单测；视频视图目标集共 14 tests 通过 |

## 5. 最佳实践审查

| 项 | 结论 | 具体判断 |
|---|---|---|
| 配置即契约 | 通过 | 默认值、校验、示例、env/Compose 透传和测试形成闭环；可靠性功能仍默认 dark |
| 失败可诊断 | 通过 | 缺密钥、worker 关闭、首次 admin 合规均有可搜索文案/手册；1 分钟内可定位 |
| 构建可重复 | 条件通过 | lock 与本机源码构建可重复；干净 Docker 构建被外部镜像代理 429 阻塞，需网络恢复后复核 |
| 契约兼容 | 通过 | Qcanvas 基础字段和 mock 边界由代码/文档测试锁定 |
| 测试诚实 | 目标矩阵通过；全包待治理 | Docker 缺失不再静默假绿；本轮目标 integration 真跑通过；历史全包共享 DB 污染明确失败 |
| 密钥域隔离 | 通过 | 视频密钥独立于 TOTP/JWT，错误文案和 round-trip 测试锁定 |
| 向后兼容 | 通过 | reliability flag-off、mock 路径、旧状态枚举未改；只纠正错误的 Seedance boundary |
| 文档可导航 | 通过 | `00_START_HERE.md` 单入口指向基线、现实、目标、不变量、门禁和审查包 |

策略选择：worker 采用“调度默认开启、真实可靠性功能 flag-off、关闭时强诊断”，避免 queued 假死且不扩大真实 Provider 权限；integration 采用“CI/默认 fail-closed、仅显式本地诊断可 skip”，避免门禁假绿；Docker 保留“根 Dockerfile 主线 + delivery 离线适配”双路径，不让交付捷径替代可重复主线。

## 6. 验证命令与退出码

| 命令（敏感值均为合成占位） | 退出码 | 结果 |
|---|---:|---|
| `go vet ./internal/config ./internal/service ./internal/handler ./internal/repository ./cmd/server` | 0 | 通过 |
| `go test ./internal/config ./internal/service ./internal/handler ./internal/server/routes ./cmd/server -count=1` | 0 | 5 个目标包通过 |
| `go test ./... -count=1` | 0 | 全量普通 Go 测试通过，77s |
| WSL `go test -tags=integration ./internal/repository -count=1 -run 'Test(Video\|BillingReservation\|BillingTransaction\|DomainOutbox\|Reliability\|GenerationContentRetention\|ExpiredInFlight)'` | 0 | 新 Postgres 容器中目标 integration 通过，9.497s |
| WSL `go test -tags=integration ./internal/repository -count=1` | 1 | 诚实失败：历史测试共享 DB 污染；同时发现并修复本轮过期预约多语句 fixture |
| `npx.cmd vitest run ...useVideoTaskLifecycle.spec.ts ...VideoTaskDetailView.spec.ts ...VideoTasksView.spec.ts ...VideoReliabilityFlow.spec.ts --reporter=basic` | 0 | 4 files / 14 tests 通过 |
| `pnpm.cmd run build` | 0 | `vue-tsc -b && vite build` 通过，920 modules；仅既有警告 |
| 三份 `docker compose -f ... config --quiet`，`COMPOSE_DISABLE_ENV_FILE=1` | 0 | main/local/dev 均通过；未读取本地 `.env` |
| 隔离 `node:24-alpine` + pnpm 9 `install --frozen-lockfile --lockfile-only --ignore-scripts` | 0 | overrides 契约通过 |
| `sh -n deploy/docker-entrypoint.sh` + 缓存交付镜像静态 entrypoint/`su-exec`/用户检查 | 0 | 静态冒烟通过；不是新镜像 health 证明 |
| `docker build --target frontend-builder -t sub2api:post-hardening-frontend .` | 1 | 外部代理拉 `docker/dockerfile:1.7` 返回 429，未进入源码构建 |
| `docker build -f Dockerfile.delivery -t sub2api:post-hardening-delivery .` | 1 | 外部代理拉 `golang:1.26.5-alpine` 返回 429，未进入源码构建 |
| `git diff --check` | 0 | 通过；仅有 Git 的 LF→CRLF 提示 |

前端截图：本轮没有视觉/UI 行为改动，也未在无法构建新镜像时启动新服务，因此未制作新截图。mock 浏览器彩排沿用第 1 个真相源的 2026-07-11 证据；本轮新增的是自动化契约与构建证据，不把旧截图冒充新镜像证明。

## 7. 残留风险、明确非声称与回滚

### 残留风险

1. Docker 镜像代理 429 未解除；网络恢复后必须用当前工作树重新构建根/交付镜像，启动 Compose，检查 `/health` 和 mock create/poll，才能关闭老板电脑新镜像门禁。
2. 全部历史 repository integration 尚不具备测试间完全隔离：固定邮箱、空表和 ID 假设会在同一共享数据库内互相污染。本轮目标 D9–D12 矩阵已隔离并通过，但不能把 `go test -tags=integration ./internal/repository` 写成全绿。
3. `docs/api/image-gateway-contract.md` 的历史编码问题仍存在；本轮只修视频契约，未改图像业务。
4. 工作树在本轮开始前已有用户暂存文件、`.worktrees/`、`sub2api-delivery/` 和交付大产物；全部保留。本轮未 stage、commit 或 push，避免把用户既有改动或敏感大产物混入提交。

### 明确非声称

- 不声称真实 Seedance/付费 Provider READY，不声称真实支付、生产数据、生产部署或公网暴露已验证。
- 不声称 `result_url` 存在就等于资产持久交付。
- 不声称缓存旧镜像静态冒烟等于本轮新镜像 `/health` 通过。
- 不声称 Qcanvas 仓库本身已被修改；本轮只维护 Sub2API 侧兼容契约。

### 回滚

先把本轮文件与用户起始 dirty baseline 分离成独立本地提交，再用定向 `git revert <commit>` 回滚；Compose 数据保持 named volumes，不删除卷。若只需回退行为，优先关闭 `RELIABILITY_CORE_VIDEO_ENABLED`，保留 mock 与诊断能力。禁止使用 `reset --hard`、`clean -fd`、删除 `.worktrees/` 或交付包。

## 8. 给老板 / Qcanvas 的一句话产品状态

**Qcanvas mock create/poll 契约与本地代码门禁保持可演示，老板电脑 Compose 配置已硬化，但须等 Docker 镜像代理恢复后补跑“新镜像构建 → `/health` → mock create/poll”，当前仍不是生产 READY。**

## 文件索引

- 单入口：`00_START_HERE.md`
- 当前现实：`02_CURRENT_REALITY_STATUS.md`
- 产品不变量：`PRODUCT_INVARIANTS.md`
- 质量门禁：`CODE_QUALITY_GATE.md`
- 视频契约：`docs/api/video-gateway-contract.md`
- 部署手册：`deploy/README.md`、`deploy/DOCKER.md`
- 实施计划：`docs/superpowers/plans/2026-07-11-post-delivery-hardening.md`

```text
POST_DELIVERY_HARDENING_STATUS: 条件通过
MOCK_PATH_INTACT: yes
TRUTH_SOURCES: restored
```
