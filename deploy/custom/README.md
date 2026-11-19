# 自有镜像部署

本目录维护 wklong/sub2api 的部署入口。应用从自己的源码构建，镜像地址为
`ghcr.io/wklong/sub2api`，当前发布架构为 `linux/amd64`。
本地构建及自有 Compose 默认从 DaoCloud 国内镜像源拉取 Node、Go、Alpine、PostgreSQL
和 Redis 基础镜像；可在构建参数或 `.env` 中覆盖镜像地址。
Compose 通过 `extends` 复用 `../docker-compose.yml` 的应用、PostgreSQL、Redis、
健康检查和环境变量配置，只覆盖镜像、实例名称和必填密钥。

## 一、发布自己的镜像

1. 将这些配置通过 PR 合并到自己的默认分支，并确认现有 CI 通过。
2. 在 GitHub 仓库 Actions 页面启用工作流。手动运行 `Custom Image Release`
   只验证当前所选分支的构建，不上传镜像；工作流合并到默认分支后才会显示手动入口。
3. 为已合并的提交创建自己的版本标签。下面是示例，发布前替换版本号：

```sh
git switch main
git pull --ff-only origin main
git tag -a v0.2.23-custom.1 -m "自有版本首次发布"
git push origin v0.2.23-custom.1
```

标签必须符合 `vX.Y.Z-custom.N`，其中 N 是从 1 开始的修订号。
发布流程会检查标签提交属于默认分支，并构建完整前后端镜像。
成功后上传两个标签：完整版本标签，以及 `sha-<完整提交哈希>`。
不自动更新 `latest`，不创建 GitHub Release 或上传独立二进制。
已发布标签不要移动或复用，修复后递增自有修订号。

GHCR 上传使用工作流内置的 `GITHUB_TOKEN`，无需添加个人令牌。
仓库或组织策略仍须允许 `packages: write`。首次发布后检查 GitHub Packages
的包可见性；公开仓库不代表镜像包一定已公开。
若已有同名包且发布提示无权限，检查包关联仓库及 Actions 访问权限。

原 `Release` 工作流只允许在 `Wei-Shaw/sub2api` 运行；本 Fork 的镜像发布
由 `custom-release.yml` 负责。同步上游后需复查此限制。

## 二、在 Linux x86_64 服务器部署

需要 Docker Engine 和支持 `up --wait` 的 Docker Compose v2 插件。
服务器上保留仓库文件结构，至少保留 `deploy/docker-compose.yml` 和整个
`deploy/custom/`；自有 Compose 依赖父目录的基础文件。

```sh
git clone git@github.com:wklong/sub2api.git
cd sub2api
# 首次部署建议检出已发布版本，使部署配置与应用版本一致。
git checkout v0.2.23-custom.1
cp deploy/custom/.env.example deploy/custom/.env
chmod 600 deploy/custom/.env
```

编辑 `.env`，填写 `SUB2API_VERSION` 为刚发布的完整版本标签，例如
`v0.2.23-custom.1`。分别执行 `openssl rand -hex 32` 生成 PostgreSQL 密码、
Redis 密码、JWT 密钥和 TOTP 密钥，填写四个不同值。建议填写初始管理员邮箱和密码，
管理员配置仅首次初始化生效。脚本不会生成、显示或覆盖这些凭据。

`.env` 使用 Compose 格式，包含 `$` 或 `#` 等特殊字符的值应使用单引号包裹；
优先使用十六进制随机密码。避免在命令行传入真实密钥。

如果包未公开，先交互执行 `docker login ghcr.io -u wklong`，
按 GitHub 的要求使用有权读取该包的令牌作为密码。公开镜像一般可匿名拉取。

```sh
sh deploy/custom/deploy.sh check
sh deploy/custom/deploy.sh up
sh deploy/custom/deploy.sh status
sh deploy/custom/deploy.sh logs
```

本地刚用 Docker 构建的镜像不需要从 GHCR 拉取，可执行：

```sh
PULL_IMAGES=false sh deploy/custom/deploy.sh up
```

默认 `PULL_IMAGES=true`，适用于服务器按版本从 GHCR 更新镜像。

脚本可以从任意工作目录调用，始终读取脚本所在目录的 `.env`；
Shell 中已导出的同名环境变量会按 Compose 规则覆盖 `.env`。
默认只监听 `127.0.0.1:8080`，建议通过 HTTPS 反向代理访问。
需要外部直连时，在 `.env` 中明确设置 `BIND_HOST`，并配置服务器入站规则。
PostgreSQL 和 Redis 不向宿主机暴露端口。

## 三、持久化、更新和恢复

应用、PostgreSQL、Redis 分别使用命名卷。默认项目名称为 `sub2api-custom`，
例如应用卷为 `sub2api-custom_sub2api_data`；项目名和数据库凭据确定后应保持稳定。
旧部署的数据不会自动迁入这套新卷；迁移现有实例需先确认原卷名称、数据库版本及备份恢复方案。
不要执行 `docker compose down -v`，该参数会删除持久化卷。

更新流程：

1. 记录当前镜像版本、提交和配置；备份 PostgreSQL、应用数据卷及 `.env`。
   PostgreSQL 应使用经过验证的逻辑备份或一致性备份，不要直接拷贝运行中的数据库目录。
2. 发布新的自有镜像，检查更新说明及数据库迁移。
3. 检出新发布标签，使基础 Compose 和自有部署文件与新版源码一致。
   `.env` 被 Git 忽略，仍需检查是否有新增必填配置。
4. 修改 `.env` 中的 `SUB2API_VERSION`，重新执行 `check` 和 `up`。
5. 检查健康状态、登录、充值、支付回调、余额和关键 API。

更新前后都不要重新生成 JWT/TOTP 密钥。镜像构建、数据库迁移或健康检查失败时停止发布，
保留错误日志供排查；脚本失败不会自动回滚。已有数据库迁移时，改回旧镜像不一定兼容，
需要结合旧部署配置和已验证的数据库备份恢复。

**不要使用后台“在线更新”安装程序。** 当前应用的在线更新目标固定为
`Wei-Shaw/sub2api`，会下载上游二进制，可能覆盖自己的功能。本部署通过重新拉取
自有镜像更新，当前工作流也不发布供在线更新使用的二进制资源。

可选配置名称参考 `../.env.example`，实际可传递项以基础 Compose 的
`environment` 为准；仅在 `.env` 添加任意变量不代表它会自动进入应用容器。
