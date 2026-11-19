# 项目开发规范

适用于基于 Wei-Shaw/sub2api 维护的自有 Fork。目标是在持续同步上游的同时，
保留定制功能并独立发布和部署。沟通、方案和交付说明使用中文。

## 1. 工作流程和分支

- 开发前明确需求、技术栈、约束和验收条件，只询问必要问题。
- 复杂任务先提出实施计划，得到确认后修改；已经确认的范围持续推进。
- 修改前检查 Git 状态、现有结构和最近的相关实现，复用设计，避免无关重构。
- main 保存通过验证的自有版本，日常通过短期分支和 PR 合并。
- 功能分支：`feature-YYMMDD-功能说明`；修复：`fix/问题说明`；
  部署维护：`chore/custom-deploy`；上游同步：`sync/upstream-vX.Y.Z`。
- 一次提交只处理一个清晰目的，尽量分离业务功能与部署维护。
  提交格式建议 `feat: 调整充值入口`、`fix: 修复回调幂等`、`chore: 配置镜像发布`。
- 不擅自覆盖、删除其他人的未提交修改，不向 main 强制推送。

## 2. 目录与技术栈

- `frontend/`：Vue 3、TypeScript、Vite、Pinia、Vue Router、vue-i18n，包管理使用 pnpm。
- `backend/`：Go 服务，按现有 handler/service/repository 分层；数据模型沿用 Ent。
- `backend/migrations/`：数据库迁移，按既有命名及执行规则新增。
- `deploy/custom/`：自有部署入口、示例配置和操作手册。
- `.github/workflows/custom-release.yml`：自有镜像发布入口。
- 技术版本以 `backend/go.mod`、锁文件、Dockerfile 和 CI 为准，勿仅凭本文猜测版本。
- 本地 Docker 基础镜像默认使用 `docker.m.daocloud.io/library/`；发布环境可按网络策略覆盖。

业务规则放在 service 层，handler 负责协议和校验，repository 负责持久化。
前端复用现有 API 客户端、组件和状态管理；新增文案同步各语言资源并通过现有 i18n 检查。
不要为 Fork 名称批量改 Go module/import，不手改生成代码代替源定义修改。

## 3. 编码和格式（必须遵守）

修改前识别源文件编码、BOM 和换行格式，修改后保持一致。
禁止以系统默认编码批量覆盖文件，也不要因少量修改产生整文件换行差异。
新文件使用 UTF-8 无 BOM；Shell、YAML、Go、TypeScript、Vue 等按
`.gitattributes` 使用 LF。Go 代码使用 gofmt，前端沿用现有 ESLint/格式约定。
提交前使用 `git diff --check` 和差异审阅确认没有乱码、无关格式变化或误删。

## 4. 功能、安全与兼容

- 优先配置化或小范围扩展，保持默认行为和 API 兼容；不要为将来可能的需求过度抽象。
- 新接口沿用现有认证、权限校验、输入限制、错误响应和超时约定。
- 充值、支付、回调需在服务端验证金额、币种、订单归属和签名，并确保幂等与事务一致性。
  不信任前端金额和回调重复次数，不使用浮点运算随意处理货币。
- 列表遵循分页规则，避免 N+1 查询、无界内存和无超时外部请求。
- 不把令牌、账户凭据、支付密钥、数据库密码或完整敏感请求写入代码、日志、测试快照或文档。
  `.env.example` 仅存空值或明确的占位符，实际配置放 `.env` 或平台 Secrets。
- 新增迁移保持历史迁移不变，评估锁表、数据量、默认值和旧版本回退兼容性。
  生产升级前验证备份可恢复；应用回滚不能代替数据库恢复。
- 不确定的依赖行为和可能变化的信息需查证；无法验证时明确假设及限制。

## 5. 上游同步与发布

按 [同步手册](UPSTREAM_SYNC.md) 将稳定 Tag 合并到同步分支，保留 merge 历史。
同步上游后复查自有充值功能、数据库迁移、CI 权限和部署继承行为。
自有发布标签为 `vX.Y.Z-custom.N`，只允许发布已进入默认分支的提交。
禁止移动已发布标签，禁止使用 `git push --tags` 批量发布缓存标签。
生产固定使用自有版本镜像或经过记录的摘要，不依赖 `latest`。
部署按 [自有部署说明](deploy/custom/README.md) 操作，不使用指向上游的后台在线更新。
发布前确认关联 CI 全部通过；当前镜像发布工作流不会替代现有完整业务测试。

## 6. 验证标准

按改动影响选择有价值的验证；不为简单文案或可逆配置编写仅复述实现的测试。
修改业务行为、资金逻辑、权限或错误处理时，应补充相应回归用例。
以下命令在具备对应工具和依赖的环境执行：

| 范围 | 建议验证 |
| --- | --- |
| 前端 | `pnpm --dir frontend install --frozen-lockfile`，然后 `make test-frontend` 和 `pnpm --dir frontend run build` |
| Go 后端 | `make -C backend test-unit`；涉及存储或集成时执行 `make -C backend test-integration` |
| 后端完整检查 | `make -C backend test`（需要 golangci-lint） |
| 自有部署 | `sh -n deploy/custom/deploy.sh`、`sh deploy/custom/deploy.sh check` |
| 镜像 | 手动运行 Custom Image Release 验证构建；正式发布后在目标架构验证健康与功能 |
| 所有修改 | `git diff --check`，人工检查最终差异与敏感信息 |

`make`、POSIX Shell、Docker 或测试服务未安装时，说明未执行项及原因，不能宣称通过。
已有测试失败需判断与本次修改的关系并明确报告，不顺带扩大修改范围。

## 7. PR 与交付

PR 描述先说明原问题与最终行为，再列关键改动、配置变化和验证结果。
上游同步 PR 补充上游基线版本、冲突处理、保留的定制逻辑和数据库影响。
代码审阅重点：正确性、异常路径、幂等、权限、兼容性及生产操作可恢复性。
交付说明包含文件位置、使用命令、实际测试结果、未验证项、潜在风险和必要后续操作。
