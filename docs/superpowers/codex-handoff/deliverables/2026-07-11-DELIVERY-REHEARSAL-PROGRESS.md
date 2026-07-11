# 2026-07-11 交付试运行进度（老板电脑本地 + Qcanvas 联动）

状态：**本机构建机彩排通过（mock）**；口径仍是「内部 mock 试运行」，非生产 READY。
分支：`wujie/video-capture-moat-20260702`
基线 HEAD（开始本轮时）：`36de34b8`（`merge(origin/main): port video/reliability moat onto upstream`）
工作树：本轮有未提交修复与交付产物（见下文）。

---

## 1. 产品结论（先看这个）

| 问题 | 产品含义 | 本轮结果 |
|------|----------|----------|
| 明天能否在老板 Windows + Docker Desktop 上本地试跑？ | 老板能打开管理台、能登录 | **能**（本机 compose 彩排已绿） |
| Qcanvas 同机能否打通视频链路？ | 画布侧 create→poll→拿到结果 | **能（仅 mock）** |
| 能否宣称真实 Seedance / 计费生产可用？ | 对外/对老板口径 | **不能** |
| 可靠性内核是否仍成立？ | 不丢账、不重复扣、可恢复 | 审计曾「条件通过」；port 后出现配置/资产回退，本轮已现场修补启动面 |

一句话：**交付包已备好，mock 全链路已彩排；真实 Provider 与部分 DB 集成门禁仍是遗留风险。**

---

## 2. 本轮做了什么

### 2.1 调查（Grok 4.5 四路并行）

1. 部署与环境
2. 鉴权与管理员初始化
3. 交付就绪度审查
4. Qcanvas 契约

关键发现：

- 管理员靠 `AUTO_SETUP` / Setup Wizard，不是迁移预置。
- Gateway 用用户 `sk-` API Key（须绑分组）；后台用 JWT。
- `VIDEO_GATEWAY_ENCRYPTION_KEY` 为空则 **Wire 启动失败**。
- `worker_enabled` 若为 false，任务会卡在 `queued`（假死）。
- port 到 `origin/main` 后丢失 `video_gateway` / `reliability_core` 的 viper 默认值、Validate、`config.example.yaml` 段。
- 根级真相源（`PRODUCT_INVARIANTS.md` 等）与 `docs/goals/03_CURRENT_GOAL.md` 在当前树中缺失或断链。

### 2.2 现场修复（阻塞交付级）

| 修复 | 路径 |
|------|------|
| 恢复 video/reliability `SetDefault` + `Validate` + EncryptionKey trim | `backend/internal/config/config.go` |
| 补 `deploy/config.example.yaml` / `.env.example` / `docker-compose.local.yml` 透传 | `deploy/*` |
| 恢复缺失前端：`productMode.ts`、货币 composables、`usd_cny_rate` 类型 | `frontend/src/**` |
| 修复 port 后测试编译：`Total() int64`、`NewBillingCacheService` 参数、`provideCleanup`、`dashboardSettingRepoStub` | 若干 `*_test.go` |
| 恢复 `testdata/ark_poll_succeeded.json` | `backend/internal/service/testdata/` |
| 交付专用镜像：`Dockerfile.delivery`（跳过坏掉的 pnpm frozen lock；补 `su-exec`/非 root） | 仓库根 |
| 交付 compose：独立容器名 + **named volumes**（规避 WSL/NTFS 下 Postgres chmod 失败） | `sub2api-delivery/` |

### 2.3 验证

| 门禁 | 结果 |
|------|------|
| 前端 `pnpm run build` | 通过（产物进 `backend/internal/web/dist`） |
| 视频相关 Vitest | 通过 |
| 聚焦后端单测（config/service/handler/routes/cmd） | 修复后通过（含 Seedance poll fixture） |
| `go test -tags=integration ./internal/repository`（WSL） | **有失败**：billing reaper SQL 参数类型、reconciliation 多语句 prepared、demo 账号污染等（见 §4） |
| 本机 Docker compose 彩排 | **通过** |

### 2.4 交付产物

目录：`D:\sub2api-trunk\sub2api-delivery\`

- `start.ps1` / `stop.ps1`
- `docker-compose.local.yml`（`sub2api:local` + named volumes）
- `.env` / `SECRETS-BACKUP.txt`（预生成密钥；**勿提交远端**）
- `images/*.tar`（sub2api / postgres18 / redis8）
- `env/Docker Desktop Installer.exe`
- `部署手册.md` / `Qcanvas对接说明.md` / `README.md`

### 2.5 彩排证据（mock）

1. `/health` → `ok`
2. admin 登录成功
3. 首次必须 `POST /api/v1/admin/compliance/accept`（否则管理 API `ADMIN_COMPLIANCE_ACK_REQUIRED`）
4. 创建绑 `default` 分组的 `sk-` Key
5. `POST /v1/video/tasks`（`provider=mock`）→ poll → **`succeeded`**
6. `result_url=/api/v1/video/mock-assets/1.svg`

状态机观测：`queued → submitted → running → succeeded`。

---

## 3. 产品视角：明天现场 Runbook（已验证）

1. 拷贝 `sub2api-delivery` 到老板电脑
2. 如需：装 Docker Desktop → 引擎就绪
3. `powershell -ExecutionPolicy Bypass -File .\start.ps1`
4. 登录 → **先做合规确认** → 建 API Key（绑分组）
5. Qcanvas：`Base URL=http://localhost:8080`，`Authorization: Bearer sk-...`，**只用 mock**

口径：**内部 mock 可演示；非生产、非真实计费 Provider。**

---

## 4. 遗留问题清单（交给下一轮 Codex 改进）

### P0 / 启动与交付面（本轮已缓解，需固化进主线）

1. **配置回退复发风险**：port/merge 后再次丢失 video/reliability defaults 的防护不足。
2. **官方 Dockerfile 的 pnpm `--frozen-lockfile` 与 overrides 不匹配** → 标准镜像构建失败；现靠 `Dockerfile.delivery` 绕过。
3. **真相源断链**：根级 invariants / goals / review package 缺失，产品决策与工程门禁失去单一事实源。

### P1 / 演示与联调体验

4. **管理员合规门闸**未写入早期交付文档，现场易卡在「能登录但不能调管理 API」。
5. **Qcanvas 契约 vs 实现**仍有 reason 码 / `provider_boundary` / 本地资产下载路径差异。
6. **前端 lifecycle 单测**（`useVideoTaskLifecycle.spec.ts`）在 port 后丢失。
7. **Integration harness 假绿**：无 Docker 时 `os.Exit(0)` 跳过仍显示成功（G45-P2-004 仍在）。

### P1 / 数据与账务正确性（集成失败，需修）

8. `billing_reservation` reaper：`pq: inconsistent types deduced for parameter $3`
9. reliability reconciliation seed：`pq: cannot insert multiple commands into a prepared statement`
10. video finalization 集成被 migration demo 账号（凭证未配置 / disabled）污染

### P2

11. Migration 号撞车（同号不同文件名，按 filename 可跑但运维易混）。
12. A3–A6（真实 Provider / 支付 / 浏览器闭环）未做。
13. `RUN_MODE=simple` 试运行跳过余额，与 flag-on reliability 计费路径的产品叙事需对齐。

---

## 5. 给下一任执行者的硬边界

- 不要把 mock 彩排写成生产 READY。
- 不要默认开启真实 Seedance；正式路径仍有 production gate。
- 不要提交 `sub2api-delivery/.env`、`SECRETS-BACKUP.txt`、`QCANVAS-API-KEY.txt`、镜像 tar、Docker Desktop 安装包到 git。
- 改进必须以**产品不变量 + 契约兼容（Qcanvas）**为先，再谈实现花样。

---

## 6. 相关文档

- 设计：`docs/superpowers/specs/2026-07-10-reliability-core-design.md`
- 计划：`docs/superpowers/plans/2026-07-10-reliability-core-implementation.md`
- Grok 审计：`docs/superpowers/codex-handoff/deliverables/2026-07-10-GROK45-RELIABILITY-AUDIT.md`
- 契约：`docs/api/video-gateway-contract.md`、`docs/api/image-gateway-contract.md`
- 下一任务书：`docs/superpowers/codex-handoff/CODEX_TASK_GPT56_POST_DELIVERY_HARDENING_20260711.md`
