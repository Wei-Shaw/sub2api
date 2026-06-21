# 使用教程分支 Review 与合并结果

## 分支与提交

- 保留分支：`codex/usage-guide-review-merge-20260621`。
- 已有功能提交：
  - `0e084add feat: 新增用户使用方法页面`
  - `acc11de5 chore: 忽略本地运行备份文件`
- Review 后补充提交：
  - 遮挡教程截图顶部浏览器栏、账户邮箱和浏览器个人信息区域。
  - 强制纳入本次 `docs/ai/context` 计划与结果文档，避免 `AGENTS.md` 引用的上下文只停留在本地忽略文件中。

## Review 结论

- 路由 `/usage-guide` 要求登录，且不要求管理员权限，符合用户侧教程页面定位。
- 普通用户侧边栏包含「使用方法」入口；管理员个人区未新增该入口。
- 页面只展示 8 个步骤和 10 张截图，没有引入后端、支付、订阅、兑换码、API Key 或计费逻辑改动。
- 发现并处理一个隐私风险：原始截图顶部包含浏览器个人信息和一个 QQ 邮箱展示。已在当前资产中遮挡相关区域，保留教程主体内容。
- `.gitignore` 已补充 `.tmp-*`、`*.dump`、`*.sqlite`、`deploy/.env.*`，避免本地数据库备份和运行态环境文件进入仓库。

## 验证

- `pnpm --dir frontend test:run UsageGuideView AppSidebar`：通过，2 个测试文件、5 个测试。
- `pnpm --dir frontend typecheck`：通过。
- `pnpm --dir frontend build`：通过；仍有项目既有 Vite chunk、Browserslist 和 Node deprecation 警告。
- 视觉抽查：
  - `step-03-subscription-plans.png`
  - `step-05-create-api-key.png`
  - `step-07-cc-switch-edit-provider.png`
  - `step-07-cc-switch-provider-list.png`
- 本地备份和运行态环境文件已被 `.gitignore` 忽略，未提交。

## 合并说明

- 本次只做本地分支合并，不推送远端、不创建 PR。
- 当前本地 `main` 仍与 `origin/main` 存在较大分叉，后续推送前需要单独处理远端同步策略。
