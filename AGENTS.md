# Sub2API 项目协作规范

本文件是项目级开发和自动化协作规范，适用于基于 `Wei-Shaw/sub2api` 的自有 Fork。
所有沟通、计划、代码说明和交付说明使用中文。修改代码前先阅读本文件，以及与任务相关的
[DEVELOPMENT.md](DEVELOPMENT.md)、[UPSTREAM_SYNC.md](UPSTREAM_SYNC.md) 和部署说明。

## 工作原则

- 先理解需求、技术栈、约束和验收目标；只提出必要问题。
- 复杂任务先给出执行计划，得到确认后再修改代码。
- 修改前检查 Git 状态、现有结构和相关实现，优先复用已有设计，避免无关重构。
- 不覆盖、删除或回退他人未提交的修改；不要使用破坏性 Git 命令处理不明确的变更。
- 完成后进行自检，明确列出实际测试、未执行项、潜在风险和后续操作。

## 分支与提交

- `main` 保存已验证的自有稳定版本；功能通过短期分支和 PR 合并。
- 功能分支使用 `feature-YYMMDD-功能说明`，修复使用 `fix/问题说明`，部署维护使用
  `chore/custom-deploy`，上游同步使用 `sync/upstream-vX.Y.Z`。
- 一次提交只处理一个清晰目的，建议使用 `feat:`、`fix:`、`chore:` 等前缀。
- 不向 `main` 强制推送。
- 上游同步 PR 必须使用普通 merge commit，不能使用 Squash 或 Rebase 合并，以保留上游祖先关系。
- 不执行 `git push --tags`；只推送明确的自有发布标签。

## 项目结构与实现边界

- `frontend/`：Vue 3、TypeScript、Vite、Pinia、Vue Router、vue-i18n，使用 pnpm。
- `backend/`：Go 服务，沿用 handler、service、repository 分层和 Ent 数据模型。
- `backend/migrations/`：数据库迁移；已在生产执行的迁移文件不得修改。
- `deploy/custom/`：自有镜像部署入口、示例配置和运维文档。
- `.github/workflows/custom-release.yml`：自有 GHCR 镜像发布流程。
- 具体 Go、Node、pnpm、Docker 版本以 `go.mod`、锁文件、Dockerfile 和 CI 配置为准。
- 本地 Docker 基础镜像默认使用 `docker.m.daocloud.io/library/`；需要时可用构建参数或 `.env` 覆盖。

业务规则放在 service 层，handler 负责协议和输入校验，repository 负责持久化。
前端复用现有 API 客户端、组件和状态管理；新增文案同步各语言资源并通过 i18n 检查。
不要为区分 Fork 批量修改 Go module/import 路径，不直接手改生成代码代替源定义修改。

## 编码与文件格式

- 修改前识别源文件编码、BOM 和换行格式，修改后保持原格式，避免整文件换行变化。
- 新文件使用 UTF-8 无 BOM；Go、Shell、YAML、TypeScript、Vue 等遵循 `.gitattributes` 的 LF 规则。
- Go 代码使用 `gofmt`，前端遵循现有 ESLint 和格式约定。
- 提交前执行 `git diff --check`，检查乱码、空白错误、无关格式变化和误删内容。
- `.env`、令牌、支付密钥、数据库密码和完整敏感请求不得提交、写入日志或测试快照。
  `.env.example` 只允许空值或明确占位符，真实配置放在本地文件或平台 Secrets。

## 功能、安全与兼容性

- 优先使用配置化和小范围扩展，保持默认行为及 API 兼容，不为假设需求过度抽象。
- 新接口沿用现有认证、权限、输入限制、错误响应和超时约定。
- 充值、支付和回调必须服务端验证金额、币种、订单归属和签名，并保证幂等与事务一致性。
  不信任前端金额和回调次数，不使用浮点数随意处理货币。
- 列表遵循分页规则，避免 N+1 查询、无界内存和无超时外部请求。
- 新增迁移前评估锁表、数据量、默认值、升级和回退影响；生产升级前确认备份可恢复。
- 不确定或可能变化的依赖行为先查证；无法验证时明确假设和限制。

## 上游同步与自有发布

上游同步按 [UPSTREAM_SYNC.md](UPSTREAM_SYNC.md) 执行：从 `upstream` 获取稳定 Tag，创建同步分支，
使用 `git merge --no-ff` 合并，逐项解决冲突并测试后提交 PR。重点复查充值功能、权限、支付回调、
余额计算、数据库迁移、工作流权限和 `deploy/custom/compose.yml` 的继承关系。

自有发布标签必须为 `vX.Y.Z-custom.N`，且标签提交已经进入默认分支。生产固定使用自己的版本镜像
或镜像摘要，不依赖 `latest`，不移动或复用已发布标签。自有发布流程推送到：
`ghcr.io/wklong/sub2api`。

上游仓库的在线更新逻辑可能下载 `Wei-Shaw/sub2api` 的二进制，不能用于自有部署。部署和升级统一按
[deploy/custom/README.md](deploy/custom/README.md) 重新拉取自有镜像；不要执行 `docker compose down -v`。

## 验证要求

根据改动范围选择有效验证；业务行为、资金逻辑、权限或错误处理变化应补充回归用例。

```sh
# 前端
pnpm --dir frontend install --frozen-lockfile
make test-frontend
pnpm --dir frontend run build

# 后端
make -C backend test-unit
make -C backend test-integration  # 涉及存储或集成时执行
make -C backend test              # 完整检查，需要 golangci-lint

# 自有部署
sh -n deploy/custom/deploy.sh
sh deploy/custom/deploy.sh check

# 所有修改
git diff --check
```

Docker、Make、POSIX Shell 或测试服务未安装时，明确说明未执行项，不能宣称验证通过。
已有测试失败时判断是否与本次修改相关，不顺带扩大改动范围。

## PR 与交付

PR 先说明问题和最终行为，再说明关键改动、配置变化和验证结果。上游同步 PR 还必须记录上游基线、
冲突处理、保留的自定义逻辑和数据库影响。交付说明包含文件位置、使用命令、实际测试结果、未验证项、
潜在风险和必要后续操作。
