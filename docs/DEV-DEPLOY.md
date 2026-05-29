# 开发镜像部署指南

将未发布的开发分支临时部署到线上服务器进行测试，测试完成后还原到官方版本。

## 前置条件

- 本地安装 Docker（支持 `docker buildx`）
- 服务器 SSH 免密登录已配置
- 可选：设置环境变量避免每次传入服务器地址
  ```bash
  export SUB2API_DEPLOY_SERVER=root@1.2.3.4
  export SUB2API_DEPLOY_PORT=22
  ```

## 部署工具

脚本位于 `docs/TS-Deploy/` 目录下：

| 脚本 | 用途 |
|------|------|
| `deploy.sh build` | 构建开发镜像并缓存为 tar.gz |
| `deploy.sh deploy` | 将缓存的镜像部署到服务器 |
| `deploy.sh list` | 列出已缓存的构建版本 |
| `deploy.sh clean` | 清理所有本地缓存 |
| `rollback.sh` | 停止开发容器，还原到 systemd 官方服务 |

---

## 一、构建开发镜像

在项目根目录执行：

```bash
cd docs/TS-Deploy
./deploy.sh build
```

脚本会自动：
- 检测当前分支和 commit
- 从最近 git tag 生成版本号（如 `0.1.89.999-dev-abc1234`，`.999` 确保不触发自动更新）
- 构建 `linux/amd64` 镜像
- 导出为 `builds/sub2api-{版本}.tar.gz` 并创建 `latest` 软链接

构建完成后会显示可用的部署命令。

## 二、部署到服务器

```bash
# 部署最新构建（使用默认或环境变量中的服务器地址）
./deploy.sh deploy

# 指定服务器
./deploy.sh deploy root@1.2.3.4 22

# 部署指定版本
./deploy.sh deploy root@1.2.3.4 22 0.1.89.999-dev-abc1234
```

部署流程（需确认后执行）：
1. **上传并加载** — scp 上传 tar.gz，远程 `docker load`
2. **备份当前状态** — 备份 `docker-compose.yml` 和当前镜像信息到 `backups/` 目录（自动轮转，保留最近 5 个）
3. **切换镜像** — 修改 compose 文件中 sub2api 的镜像为 `sub2api:ts-dev`，重启服务
4. **健康检查** — 等待容器通过 Docker healthcheck（最多 30 秒）

### 一键构建+部署

```bash
# 兼容旧用法：构建完成后直接部署
./deploy.sh
./deploy.sh root@1.2.3.4 22
```

## 三、还原到官方版本

```bash
# 使用默认服务器
./rollback.sh

# 指定服务器
./rollback.sh root@1.2.3.4 22
```

还原流程：
1. **探测状态** — 检查 Docker 开发容器和 systemd 官方服务的运行状态
2. **停止开发容器** — `docker compose stop` + 等待端口释放
3. **还原 compose 文件** — 从 deploy 时创建的备份恢复
4. **启动官方服务** — `systemctl start sub2api`
5. **验证** — 确认 systemd 服务就绪后清理开发镜像；未就绪则保留镜像以便回退

## 四、查看与清理缓存

```bash
# 列出所有缓存的构建版本
./deploy.sh list

# 清理所有缓存（需确认）
./deploy.sh clean
```

---

## 架构说明

服务器上存在两套部署模式：

| | 官方部署 | 开发部署 |
|---|---------|---------|
| 运行方式 | systemd 二进制 | Docker Compose |
| 安装路径 | `/opt/sub2api/` | `/root/sub2api/` |
| 服务管理 | `systemctl start/stop sub2api` | `docker compose up/down` |
| 镜像/二进制 | GitHub Release | 本地构建 `sub2api:ts-dev` |

`deploy.sh` 部署时会停止 systemd 服务，启动 Docker 容器；`rollback.sh` 反向操作。两者通过备份机制保证 `docker-compose.yml` 状态可恢复。

> **注意**：服务器上可能存在其他 sub2api 实例（如 `sub2api-waibao`），脚本已做精确匹配，只操作主实例 `sub2api`，不会影响其他实例。
