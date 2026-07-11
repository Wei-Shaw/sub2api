# Codex / GPT-5.6 任务书：交付后产品硬化与最佳实践改进

## 直接交给 Codex 的开头语

```text
你现在是 Sub2API 的产品向工程负责人（不是只修编译错误的打补丁机器人）。

请进入 D:\sub2api-trunk，完整阅读：
docs/superpowers/codex-handoff/CODEX_TASK_GPT56_POST_DELIVERY_HARDENING_20260711.md

必须先按任务书第 3 节「真相源读取顺序」把能读到的真相源读完，再动手。
若真相源缺失，先重建最小可用真相源，再改业务代码。

本轮目标不是“再加功能”，而是：把昨晚交付试运行暴露的问题，升级成可重复、可审计、符合最佳实践的主线方案，并保证 Qcanvas mock 联调与老板电脑 Docker 一键试跑不被回退。

硬边界：
- 不调用真实/付费 Provider，不触碰生产数据与真实支付。
- 不读取、打印、提交 .env / SECRETS-BACKUP / API Key / token / cookie。
- 不 push、reset --hard、clean -fd、rebase、force push。
- 不把 mock 通过写成生产 READY。
- 保留用户已有 ?? .worktrees/ 与 sub2api-delivery/ 大产物；不要删除交付包。
- 改动要可验证：每个产品风险对应测试或可复现命令。

完成后按第 8 节输出审查包到：
docs/superpowers/codex-handoff/deliverables/2026-07-11-POST-DELIVERY-HARDENING-review.md
```

---

## 1. 产品背景（先建立共同世界观）

Sub2API 是内部 AI API / 视频网关控制面。当前最重要的外部调用方是 **Qcanvas（TapCanvas）**：它用 API Key 调 `/v1/video/tasks` 创建任务并轮询结果，**无 webhook**。

昨晚（2026-07-11）完成了「老板 Windows + Docker Desktop 本地试运行 + 同机 Qcanvas mock 联动」的前置工作：

- 交付包：`sub2api-delivery/`（镜像 tar、compose、start.ps1、手册）
- 本机彩排：**admin 登录 → 合规确认 → 建 sk Key → mock create/poll → succeeded**

产品口径仍然是：

> **内部 mock 可演示；非生产 READY；真实 Seedance / 计费 Provider 未启用。**

你的工作是：在不破坏这条已验证路径的前提下，把暴露出的工程与产品缺口，做成**主线级**改进（配置、构建、契约、集成测试、真相源、交付可重复性），并对照行业/仓库内最佳实践提出更优方案。

进度事实源（必读）：

- `docs/superpowers/codex-handoff/deliverables/2026-07-11-DELIVERY-REHEARSAL-PROGRESS.md`

---

## 2. 任务目标

### 2.1 必须达成（Done 定义）

1. **启动面不再靠口头记忆**
   - `video_gateway` / `reliability_core` 的 defaults、Validate、example、compose 透传成为主线且有测试防回归。
   - 缺少 `VIDEO_GATEWAY_ENCRYPTION_KEY` 时失败信息对运维可读；worker 关闭时有明确可观测信号（日志/指标/管理台提示至少一种）。

2. **标准 Docker 构建可重复**
   - 根 `Dockerfile` 的 pnpm frozen lock / overrides 问题被根治（修 lock 或修 Dockerfile 策略，二选一要有理由）。
   - `Dockerfile.delivery` 与官方 Dockerfile 的差异被文档化；能说明何时用哪个。

3. **Qcanvas 联调契约对齐**
   - 以 `docs/api/video-gateway-contract.md` 为产品契约，列出实现差异并收敛到「兼容或文档更新」二选一；禁止 silently 漂移。
   - mock 路径保持：create → poll → `succeeded` + `result_url`。

4. **集成测试可信**
   - 修复本轮暴露的 integration 失败（reaper SQL 类型、reconciliation 多语句、demo 账号污染）。
   - 消灭或显著削弱「无 Docker 时 integration exit 0 假绿」（G45-P2-004）。

5. **真相源可导航**
   - 重建或恢复最小真相源入口（至少：当前目标、产品不变量摘要、当前现实状态），让后人不必靠聊天记录决策。

6. **交付体验产品化**
   - 首次 admin 合规确认进入部署手册/启动检查清单（代码侧可考虑更友好的引导，但不强制改前端大改版）。
   - Windows Docker 路径优先 named volumes 或等价可靠方案，并写清原因。

### 2.2 明确非目标

- 不在本轮打通真实 Seedance production gate。
- 不修改 Qcanvas 仓库。
- 不引入新的独立微服务 / 外部 MQ。
- 不做大范围与本目标无关的重构（例如无关大文件拆分）。
- 不把 `sub2api-delivery` 的密钥与安装包提交进 git。

---

## 3. 真相源读取顺序（强制）

**未读完前禁止改业务代码。** 后文不能覆盖前文硬边界。

### 3.1 产品与可靠性（按序）

1. `docs/superpowers/codex-handoff/deliverables/2026-07-11-DELIVERY-REHEARSAL-PROGRESS.md`（本轮事实）
2. `docs/superpowers/specs/2026-07-10-reliability-core-design.md`（不变量主真相源）
3. `docs/superpowers/plans/2026-07-10-reliability-core-implementation.md`
4. `docs/superpowers/codex-handoff/deliverables/2026-07-10-RELIABILITY-CORE-review.md`
5. `docs/superpowers/codex-handoff/deliverables/2026-07-10-GROK45-RELIABILITY-AUDIT.md`
6. `docs/api/video-gateway-contract.md`
7. `docs/api/image-gateway-contract.md`
8. 本任务书

### 3.2 若存在则继续读（缺失则记录，不要编造）

9. `00_START_HERE.md`
10. `01_PROJECT_BASELINE.md`
11. `02_CURRENT_REALITY_STATUS.md`
12. `PRODUCT_INVARIANTS.md`
13. `ARCHITECTURE_GUARDRAILS.md`
14. `CODE_QUALITY_GATE.md`
15. `docs/goals/03_CURRENT_GOAL.md`
16. `docs/reviews/LATEST_REVIEW_PACKAGE.html`
17. `deploy/README.md` / `deploy/DOCKER.md` / `DEV_GUIDE.md`

**缺失处理规则：**

- 先在审查笔记中列出「缺失路径」。
- 再创建**最小**真相源补丁（短、可执行、不写小说），至少恢复：
  - 当前产品目标（mock 试运行 / Qcanvas 联调）
  - 不可破坏的不变量（任务 CAS、账务幂等、mock 契约、密钥域隔离）
  - 当前现实状态（已交付什么、未验证什么）
- 然后才进入代码改进。

---

## 4. 本轮已知问题 → 期望改进方向

以下不是要你逐条“打补丁交差”，而是要你用产品+工程判断，选择**最佳实践级**方案。

| ID | 现象 | 产品风险 | 期望方向（可挑战） |
|----|------|----------|-------------------|
| D1 | port 后丢失 video/reliability 配置 defaults/Validate/example | 服务起不来或任务假死 | 配置契约测试 + example 生成/校验；merge 检查清单 |
| D2 | `VIDEO_GATEWAY_ENCRYPTION_KEY` 硬失败但模板曾缺失 | 现场无法启动 | 模板、compose、启动错误文案三位一体 |
| D3 | worker 默认关闭时任务 queued 不动 | 演示翻车 | 默认策略产品化：试运行默认开 worker；或管理台强提示 |
| D4 | 根 Dockerfile pnpm frozen overrides 不匹配 | 无法打官方镜像 | 修 lockfile 或调整 install 策略；CI 复现 |
| D5 | 交付镜像曾缺 `su-exec` | 容器 127 循环重启 | 与官方最终 stage 对齐；镜像冒烟（entrypoint） |
| D6 | NTFS bind mount 导致 Postgres initdb chmod 失败 | Windows 一键失败 | named volumes 为默认；文档说明迁移方式 |
| D7 | 首次 admin 合规门闸未文档化 | “能登录却不能管” | 手册 + 可选 API/UI 引导 |
| D8 | Qcanvas reason 码/边界字段不一致 | 联调互相甩锅 | 契约测试锁定；文档或实现二选一收敛 |
| D9 | integration：reaper `$3` 类型不一致 | 账务回收不可信 | 修 SQL/参数绑定；加回归测试 |
| D10 | integration：reconciliation 多语句 prepared | 对账不可跑 | 拆语句或改执行方式 |
| D11 | demo provider 账号污染集成 | 假失败 | 测试自建账号；seed 与测试隔离 |
| D12 | integration 无 Docker 假绿 | 门禁说谎 | CI/本地：skip 必须非 0 或明确报告未执行 |
| D13 | 真相源断链 | 决策靠聊天 | 重建最小真相源入口 |
| D14 | 前端 lifecycle 单测丢失 | 轮询回归无网 | 恢复或等价覆盖 |

你可以对上表提出更好的替代方案，但必须写清：**产品收益、回归风险、验证方式**。

---

## 5. 建议工作流（可调整，不可跳过阅读）

```text
Phase 0  读真相源 + 写缺失清单 +（如需）重建最小真相源
Phase 1  固化启动/配置/构建可重复性（D1–D6）
Phase 2  契约与联调体验（D7–D8）
Phase 3  集成测试可信化（D9–D12）
Phase 4  前端/文档收口（D13–D14）
Phase 5  验证 + 审查包
```

每完成一个 Phase，跑与该 Phase 相关的最小验证，不要堆到最后一次“赌运气”。

---

## 6. 验证命令（最低集）

在 Windows PowerShell / 或 WSL 中按环境选择：

```powershell
# 后端
cd D:\sub2api-trunk\backend
go vet ./internal/config ./internal/service ./internal/handler ./internal/repository ./cmd/server
go test ./internal/config ./internal/service ./internal/handler ./internal/server/routes ./cmd/server -count=1

# 有 Docker（WSL 内更稳）时：
# go test -tags=integration ./internal/repository -count=1 -run 'Test(Video|BillingReservation|BillingTransaction|DomainOutbox|Reliability|GenerationContentRetention)'

# 前端
cd D:\sub2api-trunk\frontend
pnpm run build
npx vitest run src/views/admin/video --reporter=basic
```

交付冒烟（若动到 Dockerfile/compose/entrypoint）：

```powershell
# 构建交付镜像（已有前端 dist 时）
# wsl: docker build -f Dockerfile.delivery -t sub2api:local .
# 然后 compose up 后：
# GET /health
# mock video create/poll（可用已有脚本思路，但不要打印密钥）
```

禁止：`-tags=realsmoke`、真实 Provider、读取交付包密钥文件内容写入审查包。

---

## 7. 最佳实践审查清单（你要主动回答）

在审查包中用专节回答，不要空话：

1. **配置即契约**：关键开关是否有默认值、校验、示例、环境变量透传、测试？
2. **失败可诊断**：启动失败 / worker 未开 / 合规未确认，操作者能否在 1 分钟内定位？
3. **构建可重复**：同一提交在干净环境能否打出可运行镜像？
4. **契约兼容**：Qcanvas 只依赖文档字段时，是否仍稳定？
5. **测试诚实**：skip / 假绿是否被消灭？失败是否指向真实产品风险？
6. **密钥域隔离**：`VIDEO_GATEWAY_ENCRYPTION_KEY` 与 TOTP/JWT 是否仍分离？
7. **向后兼容**：flag-off / mock / 旧 API 状态枚举是否保持？
8. **文档可导航**：新人能否从单一入口知道“现在能做什么、不能做什么”？

若你认为某项应采用与现状不同的最佳实践（例如：worker 默认策略、integration 门禁策略、Docker 多阶段策略），给出对比表后选择，并实施你选择的方案。

---

## 8. 输出契约

唯一主审查包：

`docs/superpowers/codex-handoff/deliverables/2026-07-11-POST-DELIVERY-HARDENING-review.md`

必须包含：

1. Verdict：`通过` / `条件通过` / `需修复` / `已阻塞`
2. 真相源阅读记录（存在/缺失/你重建了什么）
3. 变更摘要（按产品风险分组，不要只贴文件列表）
4. 对 D1–D14 的处理结果表
5. 最佳实践专节（第 7 节逐条）
6. 验证命令与退出码
7. 残留风险与明确非声称（不得声称生产 READY）
8. 给老板/Qcanvas 的一句话产品状态

可选：若重建了真相源文件，在审查包中列出路径与维护规则。

---

## 9. 固定范围

| 项 | 值 |
|----|-----|
| Repo | `D:\sub2api-trunk` |
| Branch | `wujie/video-capture-moat-20260702` |
| 起始参考 HEAD | `36de34b8` + 本轮未提交交付修复 |
| 产品成功标准 | mock 联调与 Docker 试跑路径不回退；配置/构建/测试/真相源显著硬化 |
| 禁止提交 | `sub2api-delivery/.env*`、`SECRETS*`、`*.tar`、Docker Desktop 安装包、真实密钥 |

---

## 10. 完成口令

当你认为完成时，在审查包末尾写：

```text
POST_DELIVERY_HARDENING_STATUS: <通过|条件通过|需修复|已阻塞>
MOCK_PATH_INTACT: <yes|no|unverified>
TRUTH_SOURCES: <restored|partial|missing>
```
