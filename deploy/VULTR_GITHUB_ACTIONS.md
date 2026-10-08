# GitHub Actions → Vultr 自动部署

`custom-ui` 的正常推送继续执行现有 CI 和 GHCR 镜像构建。构建成功后，GitHub Runner 通过 SSH 直连 Vultr，部署 `ghcr.io/domenlee/sub2api:<完整提交 SHA>`。发布链路不再使用 AWS、SSM 或 AWS OIDC。

## GitHub 仓库配置

仓库 Variables：

| 名称 | 当前值 |
| --- | --- |
| `VULTR_HOST` | `45.77.27.100` |
| `VULTR_PORT` | `22` |
| `VULTR_USER` | `sub2api-deploy` |

仓库 Secrets：

- `VULTR_SSH_PRIVATE_KEY`：专用 Ed25519 部署私钥。
- `VULTR_SSH_KNOWN_HOSTS`：通过现有可信管理连接取得的服务器公钥，格式为 `45.77.27.100 ssh-ed25519 <服务器公钥>`。使用严格主机校验，不在每次部署时临时信任 `ssh-keyscan` 的结果。

私钥仅在 Runner 临时目录中落盘，权限由 `umask 077` 限制；作业结束时删除临时凭据。应用 JWT、TOTP、数据库和 Redis 密钥继续保留在 Vultr 的生产配置中。

## Vultr 部署入口

- 运维目录：`/opt/sub2api`，实际指向 `/opt/sub2api-trial`。
- 生产配置：`compose.production.json`，保留迁移后的数据库、挂载、网络及密钥。
- 专用系统账号：`sub2api-deploy`，无 Docker 组权限。
- SSH 强制命令：`/usr/local/bin/sub2api-deploy-command`，来自 `deploy/vultr-ssh-command.sh`。
- root 部署脚本：`/usr/local/sbin/sub2api-vultr-deploy`，来自 `deploy/vultr-ssh-deploy.sh`。
- 公钥通过 `restrict,command="/usr/local/bin/sub2api-deploy-command"` 禁止交互终端、端口转发及任意远程命令。
- sudo 仅允许执行上述 root 部署脚本；它只接受本仓库镜像和完整提交 SHA。

服务器脚本由管理员安装，工作流不从 SSH 输入或远程 URL 下载并执行任意脚本。修改部署脚本时，需要同时更新服务器上的对应文件；修改应用代码无需此操作。

## 每次发布行为

1. 获取独占发布锁，拉取指定提交镜像。
2. 从当前应用读取实际数据库名，备份 PostgreSQL、生产 Compose 和当前容器配置至 `release-backups/<时间>-<SHA>/`。
3. 仅替换生产 JSON 中的 `services.sub2api.image`，保留全部其他运行配置。
4. 仅重建应用容器，PostgreSQL、Redis、Caddy、CDK 和审核服务保持运行。
5. 等待健康状态，检查实际镜像、数据库连通性和启动日志。
6. 失败时恢复旧 Compose 和旧应用镜像。数据库备份保留，恢复数据需要人工处理，避免覆盖新写入。

## 本次启用范围

本次迁移提交使用 `[skip ci]`，不运行 CI、构建、SSH 部署试跑或线上验证；当前运行的应用镜像保持原版本。正常自动部署由下一次未跳过 CI 的 `custom-ui` 推送触发，也可随后手动运行 `Build Docker Image`。

AWS 原应用和旧部署锁继续保留，不应解除旧部署锁或直接恢复旧站写入。

GitHub 对跳过提交的说明：[Skipping workflow runs](https://docs.github.com/en/actions/how-tos/manage-workflow-runs/skip-workflow-runs)。
