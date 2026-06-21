# 使用教程分支提交、Review 与合并计划

## 目标

将当前工作区中用户“使用方法 / Usage Guide”前端功能提交到新的 `codex/` 分支，完成本地 review 和验证后合并回本地 `main`。

## 当前确认

- 当前本地分支为 `main`，本地 `main` 已有多条未推送提交。
- 当前应入库的有效改动是：
  - 新增用户使用教程页面 `frontend/src/views/user/UsageGuideView.vue`。
  - 新增教程截图资产 `frontend/src/assets/usage-guide/*.png`。
  - 新增 `/usage-guide` 路由。
  - 在普通用户侧边栏增加“使用方法”入口。
  - 增加中英文导航和页面标题描述文案。
  - 增加对应 Vitest 测试。
- 当前不应入库的文件是本地运行态备份/环境文件：
  - `.tmp-*.dump`
  - `.tmp-*.sqlite`
  - `.tmp-*.sql`
  - `.tmp-subscription-migration-stamp`
  - `deploy/.env.scheme-a.runtime`
- 这些本地文件可能包含数据库内容、运行态配置或敏感信息；本次不提交，改为补充 `.gitignore` 规则。

## 执行方案

1. 从当前本地 `main` 创建新分支 `codex/usage-guide-review-merge-20260621`。
2. 补充 `.gitignore`：
   - 忽略根目录 `.tmp-*` 本地迁移备份。
   - 忽略 `*.dump`、`*.sqlite`。
   - 忽略 `deploy/.env.*`，但不影响模板文件。
3. 运行前端相关测试：
   - `pnpm --dir frontend test:run UsageGuideView AppSidebar`
   - 如测试暴露当前测试或实现问题，先修复再提交。
4. 分阶段 review：
   - 自查 `git diff --stat` 和关键 diff。
   - 检查路由、侧边栏入口、i18n key、截图引用和测试是否一致。
   - 使用代码 review 视角列出问题；发现 Critical/Important 则先修复。
5. 提交应入库改动：
   - 前端功能文件。
   - `.gitignore` 安全忽略规则。
   - 本计划文档和最终结果文档。
   - `AGENTS.md` 运行记录。
6. 合并回本地 `main`：
   - 切回 `main`。
   - 使用 `--no-ff` 合并新分支，保留合并记录。
   - 合并后再次运行关键验证。

## 验证标准

- 使用教程测试和侧边栏测试通过。
- 构建或类型检查若可行则补跑；如因既有问题失败，记录具体原因。
- `main` 合并后包含新分支提交。
- 临时 dump/sqlite/env 文件不被提交。

## 风险

- 教程截图较大，需确认全部为前端静态资产且不含敏感密钥。
- 当前本地 `main` 已与 `origin/main` 分叉较大；本次只做本地分支合并，不推送远端、不创建 PR。
