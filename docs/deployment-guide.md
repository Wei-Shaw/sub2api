# Deployment Guide

**生成时间:** 2026-04-28  
**扫描级别:** Quick Scan  
**来源:** `README.md`、`Dockerfile`、`deploy/*`、`.github/workflows/*`

## 部署形态

Sub2API 支持三类主要部署方式：

- 安装脚本：`deploy/install.sh` 或 README 中的一键脚本，适合 Linux systemd。
- Docker Compose：`deploy/docker-compose*.yml`，包含 PostgreSQL、Redis 和应用服务。
- Release 镜像/二进制：GitHub Actions + GoReleaser 构建。

## 生产镜像构建

根目录 `Dockerfile` 是生产多阶段入口：

1. Node `24-alpine` 构建前端。
2. Go `1.26.2-alpine` 构建后端，复制前端 dist，使用 `-tags embed`。
3. PostgreSQL `18-alpine` 提供版本匹配的 `pg_dump` 和 `psql`。
4. Alpine `3.21` 作为运行镜像，非 root 用户 `sub2api`，暴露 8080，健康检查 `/health`。

注意：`backend/Dockerfile` 不是主要生产入口，除非专门维护后端单独镜像。

## Docker Compose

部署目录包含：

- `deploy/docker-compose.yml`
- `deploy/docker-compose.local.yml`
- `deploy/docker-compose.dev.yml`
- `deploy/docker-compose.standalone.yml`
- `deploy/.env.example`
- `deploy/config.example.yaml`
- `deploy/docker-entrypoint.sh`

关键配置：

- PostgreSQL 密码必须设置。
- `JWT_SECRET` 和 `TOTP_ENCRYPTION_KEY` 留空会导致重启后会话或 2FA 失效。
- 默认 timezone 语义为 `Asia/Shanghai`，影响统计和过期边界。
- 反代到网关时 Nginx 需要 `underscores_in_headers on;`，否则 sticky session 相关 header 会被丢弃。

## Systemd 安装

`deploy/install.sh` 会下载 release、安装到 `/opt/sub2api`、创建 systemd service、配置用户和权限。`deploy/sub2api.service` 是服务模板。

常用命令：

```bash
sudo systemctl start sub2api
sudo systemctl enable sub2api
sudo systemctl status sub2api
sudo journalctl -u sub2api -f
sudo systemctl restart sub2api
```

## CI/CD

`.github/workflows/backend-ci.yml`：

- Go 后端：setup-go 使用 `backend/go.mod`，校验 `go1.26.2`，运行 unit/integration。
- 前端：Node 20、pnpm 9、`pnpm install --frozen-lockfile`、`make test-frontend`。
- Lint：golangci-lint v2.9。

`.github/workflows/release.yml`：

- tag `v*` 或 workflow dispatch 触发。
- 更新 `backend/cmd/server/VERSION`。
- 构建前端并上传 `backend/internal/web/dist` artifact。
- 下载 artifact 后运行 GoReleaser。
- 发布 GHCR/DockerHub 镜像和 release artifact。

`.github/workflows/security-scan.yml`：

- 安全扫描入口，结合 `tools/check_pnpm_audit_exceptions.py` 和 `.github/audit-exceptions.yml` 管理前端 audit 例外。

## 发布构建注意

- 前端必须先构建并放入 `backend/internal/web/dist`。
- 后端 release 构建需要 `-tags embed`。
- Go 版本应以 `backend/go.mod` 和 CI 为准。
- Dockerfile 当前使用 `GOLANG_IMAGE=golang:1.26.2-alpine`。

## 回滚与更新

后端包含 admin dashboard 更新能力，管理端可检查更新、执行更新、回滚、重启。相关接口在 `/api/v1/admin/system`，前端页面在 `/admin/settings` 或相关系统管理入口。

## 部署风险清单

- 不要在生产放开不安全 HTTP、私网 SSRF 或过宽 URL allowlist。
- CSP、trusted proxies、上游响应头过滤和 Turnstile 配置是安全边界。
- 数据库备份工具版本通过 Dockerfile 从 PostgreSQL 镜像复制，升级数据库版本时同步检查备份/恢复。
- 支付 webhook 要确认公网回调地址、签名配置和幂等处理。
