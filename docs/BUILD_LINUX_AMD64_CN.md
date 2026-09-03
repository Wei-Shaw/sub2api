# 构建脚本使用说明（Windows → Linux x86_64）

本文说明如何使用仓库脚本 **`scripts/build-linux.ps1`**，在 **Windows 本机**交叉编译出可在 **Linux x86_64（amd64）** 服务器上运行的 Sub2API 二进制（含嵌入前端）。

完整部署步骤（上传、config、systemd、与云上 PG/Redis）见：

- [DEPLOY_BINARY_WINDOWS_CN.md](./DEPLOY_BINARY_WINDOWS_CN.md)

---

## 1. 适用场景

| 条件 | 说明 |
|------|------|
| 本机 | Windows，已装 **Go**、**Node.js**、**pnpm** |
| 本机 | **不需要 Docker** |
| 服务器 | `uname -m` 为 **`x86_64`** |
| 产物 | 单个可执行文件，内嵌管理台前端（`-tags embed`） |

若服务器是 ARM（`aarch64`），**不要用本脚本**，需将 `GOARCH` 改为 `arm64` 并单独改脚本或参数。

---

## 2. 前置检查

### 2.1 本机

```powershell
go version
pnpm --version
node --version
```

- Go 版本尽量与项目要求一致（见根目录 README / AGENTS.md，当前为 Go 1.25.x 一带）。
- 前端包管理**必须用 pnpm**，不要用 npm 装依赖后直接当生产构建来源。

### 2.2 服务器架构

在 Linux 上执行：

```bash
uname -m
# 期望输出: x86_64
```

---

## 3. 脚本位置与作用

| 路径 | 作用 |
|------|------|
| `scripts/build-linux.ps1` | 一键：前端 build + Linux amd64 交叉编译 |

脚本会：

1. 定位仓库根目录（要求脚本位于 `scripts/` 下）。
2. 在 `frontend/` 执行 `pnpm install` + `pnpm build`（产物到 `backend/internal/web/dist`）。
3. 设置 `GOOS=linux`、`GOARCH=amd64`、`CGO_ENABLED=0`，执行：
   ```text
   go build -tags embed -ldflags "..." -o <输出文件> ./cmd/server
   ```
4. **自动恢复**本机 `GOOS` / `GOARCH` / `CGO_ENABLED`，避免影响后续本机开发。
5. 打印输出路径与文件大小；过小会提示可能未 embed 成功。

默认输出：

```text
backend/sub2api-linux-amd64
```

---

## 4. 基本用法

在 **仓库根目录** 打开 PowerShell：

```powershell
cd E:\myGitRepositorysx\sub2api

# 若首次执行被策略拦截，可临时放开当前进程
Set-ExecutionPolicy -Scope Process -ExecutionPolicy Bypass

.\scripts\build-linux.ps1
```

或：

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\build-linux.ps1
```

成功时终端类似：

```text
==> 构建成功
    文件: E:\...\backend\sub2api-linux-amd64
    大小: xx.xx MB
```

---

## 5. 参数说明

| 参数 | 类型 | 默认 | 说明 |
|------|------|------|------|
| `-SkipFrontend` | switch | 否 | 跳过前端构建；要求已有 `backend/internal/web/dist/index.html` |
| `-SkipPnpmInstall` | switch | 否 | 跳过 `pnpm install`，只跑 `pnpm build`（依赖未变时可加速） |
| `-Output` | string | `backend/sub2api-linux-amd64` | 输出路径；相对路径相对**仓库根** |
| `-Version` | string | `custom-yyyyMMdd-HHmm` | 写入二进制的 `main.Version` |

### 示例

```powershell
# 完整构建（推荐发版使用）
.\scripts\build-linux.ps1

# 依赖没变，只重新 build 前端 + 编译
.\scripts\build-linux.ps1 -SkipPnpmInstall

# 前端刚 build 过，只重编 Go（最快，务必保证 dist 是当前代码）
.\scripts\build-linux.ps1 -SkipFrontend

# 指定版本号与输出名
.\scripts\build-linux.ps1 -Version "1.0.0-myfork" -Output "release\sub2api-linux-amd64"
```

查看帮助：

```powershell
Get-Help .\scripts\build-linux.ps1 -Full
```

---

## 6. 构建成功后上传与替换（摘要）

以下为常用命令，细节与 config/systemd 见 [DEPLOY_BINARY_WINDOWS_CN.md](./DEPLOY_BINARY_WINDOWS_CN.md)。

### 本机上传

```powershell
scp .\backend\sub2api-linux-amd64 root@你的服务器IP:/opt/sub2api/sub2api.new
```

### 服务器替换并重启

```bash
sudo systemctl stop sub2api
sudo mv /opt/sub2api/sub2api /opt/sub2api/sub2api.bak
sudo mv /opt/sub2api/sub2api.new /opt/sub2api/sub2api
sudo chmod +x /opt/sub2api/sub2api
sudo systemctl start sub2api
sudo systemctl status sub2api
```

回滚：

```bash
sudo systemctl stop sub2api
sudo mv /opt/sub2api/sub2api.bak /opt/sub2api/sub2api
sudo systemctl start sub2api
```

`config.yaml` 与云上 PostgreSQL/Redis 保持现有配置即可，**发版通常只换二进制**。

---

## 7. 常见问题

| 现象 | 处理 |
|------|------|
| 无法运行脚本 / 被策略禁止 | 使用 `powershell -ExecutionPolicy Bypass -File ...`，或当前会话 `Set-ExecutionPolicy -Scope Process Bypass` |
| `未找到 go` / `未找到 pnpm` | 安装并加入 PATH；pnpm：`npm install -g pnpm` |
| `pnpm build` 失败 | 先在 `frontend/` 单独执行 `pnpm install` 与 `pnpm build` 看报错；勿用 npm 混装锁文件 |
| 文件小于约 5MB 的警告 | 多半未嵌入前端：去掉 `-SkipFrontend` 全量构建 |
| 服务器 `cannot execute binary file` | 架构不符（非 x86_64）或文件损坏/传成文本模式 |
| 服务器只有 API 没有页面 | 未用 `-tags embed` 或 dist 缺失；用本脚本完整构建即可 |
| 本机之后 `go build` 异常 | 本脚本会恢复环境变量；若手动设过 `GOOS=linux`，请 `Remove-Item Env:GOOS` 等 |

---

## 8. 推荐发版检查清单

- [ ] `uname -m` 为 `x86_64`
- [ ] 本机 `.\scripts\build-linux.ps1` 成功
- [ ] 输出文件大小合理（通常数十 MB）
- [ ] 已备份服务器当前二进制（`sub2api.bak`）
- [ ] 上传后 `chmod +x` 并 `systemctl restart/start`
- [ ] 管理台页面可打开；关键 API / 网关调用抽测通过
- [ ] 发版验证期间本机尽量停掉连同一 Redis 的 `go run`（可选，避免并发干扰）

---

## 9. 与其它文档的关系

| 文档 | 内容 |
|------|------|
| **本文** | 构建脚本怎么用 |
| [DEPLOY_BINARY_WINDOWS_CN.md](./DEPLOY_BINARY_WINDOWS_CN.md) | 无 Docker 二进制部署全流程（配置、systemd、共用库注意点） |
| [JAVA_DEV_ONBOARDING_CN.md](./JAVA_DEV_ONBOARDING_CN.md) | 项目结构与运行逻辑上手 |

---

## 10. 一句话

```text
Windows:  .\scripts\build-linux.ps1
       →  backend/sub2api-linux-amd64
       →  scp 到 x86_64 Linux
       →  替换 /opt/sub2api/sub2api 并重启服务
```
