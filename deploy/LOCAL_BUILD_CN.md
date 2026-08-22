# Sub2API 本地镜像构建与部署

本文档适用于当前本地开发环境：PostgreSQL 和 Redis 已作为独立 Docker 容器运行，三者连接到 `docker_app-network`。

所有命令均在本目录执行：

```bash
cd /Users/fushui/sub2api/deploy
```

## 1. 构建本地镜像

```bash
./build_image.sh
```

默认生成镜像：

```text
sub2api:latest
```

Docker 多阶段构建会自动完成：

1. 安装前端 pnpm 依赖并运行 `vue-tsc -b`、`vite build`。
2. 将前端产物嵌入 Go 后端。
3. 生成最终 Alpine 运行镜像。

无需在宿主机单独安装 Go 或前端依赖。

### 可选构建参数

指定显示版本：

```bash
VERSION=0.1.162-custom ./build_image.sh
```

指定镜像标签：

```bash
IMAGE_NAME=sub2api:dev ./build_image.sh
```

指定目标平台：

```bash
PLATFORM=linux/amd64 ./build_image.sh
```

不传 `VERSION` 时，版本优先取当前 Git 精确标签，否则读取 `backend/cmd/server/VERSION`。

## 2. 部署本地镜像

```bash
./deploy_local_image.sh
```

该脚本只重建 `sub2api` 应用容器，并执行以下保护：

- 使用本地 `sub2api:latest` 镜像。
- 复用 `deploy_sub2api_data` 数据卷。
- 复用 `docker_app-network` 网络。
- 读取现有 `deploy/.env` 配置。
- 不创建、停止或重建 PostgreSQL 和 Redis 容器。
- 等待容器健康检查并请求 `/health`。

访问地址：

```text
http://127.0.0.1:8080
```

## 3. 修改后构建并部署

```bash
./build_image.sh && ./deploy_local_image.sh
```

## 4. 状态和日志

```bash
docker ps --filter name=sub2api --filter name=postgres --filter name=redis
docker logs -f sub2api
docker exec sub2api wget -q -O - http://127.0.0.1:8080/health
```

## 5. Compose 文件

本地应用部署组合使用：

```text
docker-compose.standalone.yml
docker-compose.sub2api-network.yml
docker-compose.local-image.yml
```

`docker-compose.local-image.yml` 只负责把官方镜像覆盖为本地的 `sub2api:latest`。

不要对当前环境执行 `docker compose down -v`，该命令可能删除 Compose 管理的数据卷。
