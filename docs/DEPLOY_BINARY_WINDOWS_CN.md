# Sub2API 二进制部署指南（Windows 本机构建 → 阿里云 Linux）

> 适用场景：
>
> - 基于官方仓库 fork 做**定制化开发**，部署的是自己的代码，不是官方 Release 包
> - **本机 Windows** 开发；本机**不能/不想装 Docker**
> - 服务器配置较低，**不适合在服务器上编译或跑 Docker 编排**
> - 本地开发已连接**阿里云上的 PostgreSQL / Redis**，上线后数据库与 Redis **不变**
>
> 相关文档：
>
> - [JAVA_DEV_ONBOARDING_CN.md](./JAVA_DEV_ONBOARDING_CN.md) — 项目上手（Vue/Spring 对照）
> - [../DEV_GUIDE.md](../DEV_GUIDE.md) — 本地环境与常见坑
> - [../deploy/README.md](../deploy/README.md) — 官方 Docker / 安装脚本（本文场景一般不用）

---

## 一、总体思路

```
Windows 本机
  1) pnpm build 前端 → 输出到 backend/internal/web/dist
  2) GOOS=linux 交叉编译 Go（-tags embed）→ 得到 Linux 可执行文件
  3) scp/sftp 传到 ECS
阿里云 ECS（低配）
  4) 放好 config.yaml（指向现有 PG/Redis）
  5) 运行二进制 / 注册 systemd
  6) 安全组放行端口
```

| 组件 | 位置 | 是否变化 |
|------|------|----------|
| 定制代码 | 你的 Git 仓库 / 本机 | 随开发更新 |
| 构建 | **仅 Windows 本机** | 不在服务器构建 |
| Sub2API 进程 | 阿里云 ECS（单个二进制） | 换文件发版 |
| PostgreSQL | 阿里云（已有） | **不变** |
| Redis | 阿里云（已有） | **不变** |

Go 支持**交叉编译**：在 Windows 上可直接编出 **Linux amd64** 可执行文件，服务器只负责运行。

**本文排除：** Docker、官方 `install.sh` 下载的官方包、在服务器上 `go build` / `pnpm build`、再部署一套 PG/Redis。

---

## 二、本机需要具备的工具

| 工具 | 用途 |
|------|------|
| **Go**（与项目接近，官方 README 要求 1.25.x 一带） | 交叉编译后端 |
| **Node.js + pnpm** | 构建 Vue 前端（必须用 pnpm） |
| **Git** | 你的 fork 代码 |
| **scp / WinSCP / 宝塔上传等** | 把二进制传到服务器 |

**不需要：** Docker、本机 Linux 虚拟机（有更好，但不是必须）。

### 确认服务器 CPU 架构

在 ECS 上执行：

```bash
uname -m
# x86_64  → 本机编译用 GOARCH=amd64
# aarch64 → 本机编译用 GOARCH=arm64
```

阿里云常见为 **x86_64 / amd64**。

---

## 三、本机构建前端（必须先做）

前端构建产物目录由 `frontend/vite.config.ts` 指定为：

```text
backend/internal/web/dist
```

后端使用 `-tags embed` 时会把该目录打进二进制（见 `backend/internal/web/embed_on.go`）。  
**若未 build 前端或未加 embed，服务器上将只有 API、没有管理台页面。**

在 PowerShell 中（路径按你的仓库实际位置修改）：

```powershell
cd E:\myGitRepositorysx\sub2api\frontend
pnpm install
pnpm build
```

成功后应存在：

```text
backend\internal\web\dist\index.html
backend\internal\web\dist\assets\...
```

---

## 四、本机交叉编译 Linux 二进制

```powershell
cd E:\myGitRepositorysx\sub2api\backend

$env:CGO_ENABLED = "0"
$env:GOOS = "linux"
$env:GOARCH = "amd64"   # ARM 服务器改为 arm64

# 版本号可自定，方便以后区分
$ver = "custom-" + (Get-Date -Format "yyyyMMdd-HHmm")
$ldflags = "-s -w -X main.Version=$ver -X main.Commit=local -X main.Date=$(Get-Date -Format o) -X main.BuildType=release"

go build -tags embed -ldflags $ldflags -trimpath -o sub2api-linux-amd64 ./cmd/server
```

产物：`backend\sub2api-linux-amd64`（**没有** `.exe` 后缀，这是 Linux 程序）。

检查文件大小（含前端时通常为数十 MB 量级；过小可能未 embed 成功）：

```powershell
Get-Item .\sub2api-linux-amd64 | Select-Object Name, Length, LastWriteTime
```

### 编译后清理环境变量

避免之后本机开发误用 Linux 目标：

```powershell
Remove-Item Env:GOOS -ErrorAction SilentlyContinue
Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue
```

### 要点

- 必须加 **`-tags embed`**
- 必须先 **`pnpm build`** 再编译
- `CGO_ENABLED=0` 便于静态链接、减少服务器依赖
- 架构必须与 ECS 一致（`amd64` / `arm64`）

---

## 五、配置：连接现有阿里云 PG / Redis

二进制会在工作目录（或环境变量 `DATA_DIR`）下查找 `config.yaml`。  
本地开发若已连接同一套云库，**优先复用本机那份 `config.yaml`**，仅按需调整服务器监听相关项。

### 服务器推荐目录

```text
/opt/sub2api/
  sub2api              # 上传的二进制（建议改名为此）
  config.yaml          # 数据库 / Redis / JWT 等
  .installed           # 若本地已初始化过，可一并上传，避免再进安装向导
  data/                # 可选，运行期数据
```

### config.yaml 示意

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  mode: "release"
  # 有域名后建议填写（邮件链接等）
  # frontend_url: "https://你的域名"

database:
  host: "你的PG地址"      # ECS 与库同 VPC 时优先用内网地址
  port: 5432
  user: "..."
  password: "..."
  dbname: "..."
  sslmode: "disable"     # RDS 要求 SSL 时再改为 require 等

redis:
  host: "你的Redis地址"
  port: 6379
  password: "..."
  db: 0

# jwt.secret 等与当前环境保持一致（库中已有数据时不要随意更换）
```

也可用环境变量覆盖部分配置，但**维护一份 `config.yaml` 通常最省事**。

### 与「已有库」的关系

| 情况 | 建议 |
|------|------|
| 本地已初始化过，且要连同一套库 | 上传本地 `config.yaml` + `.installed`（若有）；一般直接进主服务，不再走安装向导 |
| 服务器上没有 `.installed` | 首次可能进入 Setup：务必填**同一套** PG/Redis，勿指向空库 |
| 连接地址 | 本机可用公网；ECS 建议改用**内网 host**（同一份数据，host 可以不同） |

### 本地与线上共用同一套库时注意

1. **JWT / TOTP 等密钥**与现网一致，避免全员掉登录或加密字段异常。  
2. **尽量避免**本机 `go run` 与 ECS 生产进程**长期同时**写同一 Redis（并发槽、粘性会话、缓存会互相干扰）。发版验证时建议先停本机后端。  
3. 改表结构 / 迁移：新二进制启动可能对**同一套库**执行迁移；发版前建议备份 RDS。  
4. **不要**在 ECS 上再起一套 PostgreSQL/Redis，否则数据分裂。

---

## 六、上传到服务器

PowerShell 示例（已安装 OpenSSH 客户端时）：

```powershell
# 二进制
scp E:\myGitRepositorysx\sub2api\backend\sub2api-linux-amd64 root@你的公网IP:/opt/sub2api/sub2api

# 配置（路径按本机实际位置修改）
scp E:\path\to\config.yaml root@你的公网IP:/opt/sub2api/config.yaml
```

也可用 WinSCP、宝塔文件管理等上传到 `/opt/sub2api/`。

在服务器上：

```bash
sudo mkdir -p /opt/sub2api
sudo chmod +x /opt/sub2api/sub2api
# 建议不要长期用 root 跑业务，可自建系统用户：
# sudo useradd -r -s /sbin/nologin sub2api
# sudo chown -R sub2api:sub2api /opt/sub2api
```

先手动试跑（确认能连库、进程能起来）：

```bash
cd /opt/sub2api
./sub2api
# 或
DATA_DIR=/opt/sub2api ./sub2api
```

浏览器访问：`http://公网IP:8080`  
确认无误后 Ctrl+C 停止，再交给 systemd。

---

## 七、systemd 常驻运行

参考仓库 `deploy/sub2api.service`，可在服务器创建：

```bash
sudo tee /etc/systemd/system/sub2api.service << 'EOF'
[Unit]
Description=Sub2API Custom
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/sub2api
Environment=DATA_DIR=/opt/sub2api
Environment=GIN_MODE=release
ExecStart=/opt/sub2api/sub2api
Restart=always
RestartSec=5
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable --now sub2api
sudo systemctl status sub2api
sudo journalctl -u sub2api -f
```

### 阿里云安全组

| 端口 | 说明 |
|------|------|
| 22 | SSH（建议限制来源 IP） |
| 8080 | 直接访问应用时放行 |
| 80 / 443 | 若前面有 Nginx/Caddy 反代 |

**不要**把 PostgreSQL `5432`、Redis `6379` 对公网 `0.0.0.0/0` 长期开放。

### 生产建议（可选）

- 域名 + HTTPS 反代到 `127.0.0.1:8080`  
- Nginx 若服务 Codex 等客户端，`http` 块建议：`underscores_in_headers on;`（避免粘性会话相关头被丢）  
- 配置 `server.frontend_url` 为对外域名  

更细的反代与可信 IP 见 `deploy/EDGE_SECURITY.md`。

---

## 八、日常发版流程（固定套路）

### 本机

```powershell
cd E:\myGitRepositorysx\sub2api\frontend
pnpm build

cd ..\backend
$env:CGO_ENABLED = "0"
$env:GOOS = "linux"
$env:GOARCH = "amd64"
go build -tags embed -ldflags "-s -w -X main.Version=custom" -trimpath -o sub2api-linux-amd64 ./cmd/server

scp .\sub2api-linux-amd64 root@你的公网IP:/opt/sub2api/sub2api.new
```

### 服务器

```bash
sudo systemctl stop sub2api
sudo mv /opt/sub2api/sub2api /opt/sub2api/sub2api.bak   # 便于回滚
sudo mv /opt/sub2api/sub2api.new /opt/sub2api/sub2api
sudo chmod +x /opt/sub2api/sub2api
sudo systemctl start sub2api
sudo systemctl status sub2api
```

### 回滚

```bash
sudo systemctl stop sub2api
sudo mv /opt/sub2api/sub2api.bak /opt/sub2api/sub2api
sudo systemctl start sub2api
```

**一般只需替换二进制**，不必每次上传 `config.yaml`。仅配置变更时再更新 yaml。

---

## 九、本机构建脚本（已提供）

仓库已内置 **Linux x86_64（amd64）** 一键构建脚本：

| 文件 | 说明 |
|------|------|
| [`scripts/build-linux.ps1`](../scripts/build-linux.ps1) | Windows 下前端 build + 交叉编译 |
| [`docs/BUILD_LINUX_AMD64_CN.md`](./BUILD_LINUX_AMD64_CN.md) | 脚本参数与使用说明 |

```powershell
cd E:\myGitRepositorysx\sub2api
powershell -ExecutionPolicy Bypass -File .\scripts\build-linux.ps1
# 产物默认: backend\sub2api-linux-amd64
```

---

## 十、常见问题

| 问题 | 原因 / 处理 |
|------|-------------|
| 只有 API、没有管理页 | 未 `pnpm build`，或编译时未加 `-tags embed` |
| 服务器无法执行 | 编成了 Windows `.exe`，或 `GOARCH` 与机器不符 |
| 找不到 config | `WorkingDirectory` / `DATA_DIR` 未指向放 yaml 的目录 |
| Permission denied | 未 `chmod +x` |
| 本机 go 编译异常 | 交叉编译后未清理 `GOOS`/`GOARCH` |
| 全站登录失效 | 更换了 `JWT_SECRET` 等与现网不一致 |
| 行为怪异（限流/会话） | 本机与 ECS 同时写同一 Redis，停掉一侧再测 |

---

## 十一、与其它部署方式的对比（便于选型）

| 方式 | 是否适合本文场景 |
|------|------------------|
| 官方 `install.sh` 下载 Release | ❌ 不是你的定制代码 |
| Docker Compose（自带 PG/Redis） | ❌ 本机无 Docker、服务器低配、库已在外部 |
| Docker standalone 自建镜像 | ❌ 本机无 Docker |
| **Windows 交叉编译 + ECS 跑二进制** | ✅ **本文推荐** |
| 在 ECS 上 git clone 后编译 | ⚠️ 低配服务器不推荐 |

---

## 十二、检查清单

- [ ] 本机 `pnpm build` 成功，`backend/internal/web/dist` 存在  
- [ ] `go build -tags embed`，`GOOS=linux`，`GOARCH` 与服务器一致  
- [ ] 二进制已上传并 `chmod +x`  
- [ ] `config.yaml` 指向现有 PG/Redis（ECS 优先内网）  
- [ ] JWT 等密钥与现网一致  
- [ ] `systemctl status sub2api` 为 active  
- [ ] 浏览器可打开管理台；网关 API Key 调用正常  
- [ ] 安全组最小化；5432/6379 不对公网裸奔  
- [ ] 发版时保留 `sub2api.bak` 便于回滚  

---

## 十三、一句话总结

> **在 Windows 上构建「嵌入前端的 Linux 二进制」，上传到低配 ECS 运行，配置指向已有的阿里云 PostgreSQL 与 Redis。**  
> 服务器不构建、不起数据库容器、不使用官方 Docker 镜像；定制代码完全由你本机构建产物决定。
