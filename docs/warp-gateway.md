# Cloudflare WARP Gateway 接入方案（Phase 0–3）

本文对应已批准的 **sub2api × Cloudflare WARP** 方案落地说明。

## 架构

- **数据面**：账号出站仍走既有 `socks5h://` 代理（`proxyutil`）。
- **控制面**：`tools/warp-gateway` 管理 WARP/Mock 实例、健康检查、池化、rotate、exit_ip 去重告警。
- **sub2api**：`warp.*` 配置 + `WarpGatewayClient` / `BuildAttachPlan`（Phase 2–3）把网关实例映射为 Proxy 规格。

```
Account → Proxy / ProxyGroup → socks5h://127.0.0.1:4100x
                                    ↑
                            warp-gateway instances
```

## 快速本地测试（Mock，无需真实 WARP）

```bash
cd tools/warp-gateway
go test ./...
bash scripts/local-e2e.sh

# 完整本地部署联调：Postgres+Redis(docker) + warp-gateway + sub2api + 管理 API 落库
bash scripts/local-full-e2e.sh
# 工作目录默认：仓库根 .local-warp-e2e/
# 服务会保持运行；清理：docker rm -f sub2api-warp-e2e-pg sub2api-warp-e2e-redis
```

启动服务：

```bash
go run ./cmd/warp-gateway -listen 127.0.0.1:19798 -runtime mock -token dev
```

创建 3 实例池：

```bash
curl -sS -X POST http://127.0.0.1:19798/v1/pools \
  -H "Authorization: Bearer dev" -H "Content-Type: application/json" \
  -d '{"name_prefix":"warp","count":3}'
```

查看可挂到 ProxyGroup 的 SOCKS URL：

```bash
curl -sS http://127.0.0.1:19798/v1/pools/snapshot -H "Authorization: Bearer dev"
```

在 sub2api 管理端 **手动** 添加对应 `socks5h` 代理，或后续接 Admin API 自动 `BuildAttachPlan` 落库。

## Phase 对照

| Phase | 内容 | 状态 |
|------|------|------|
| 0 | sing-box 模板 + 运维脚本 | `scripts/phase0-singbox-template.json` |
| 1 | gateway MVP（实例 CRUD/start/stop/health/metrics） | `cmd/warp-gateway` |
| 2 | sub2api `warp` 配置 + HTTP client | `backend/internal/config` + `service/warp_gateway.go` |
| 3 | 池化、不健康摘除计划、rotate、exit_ip 去重 | `/v1/pools` + `BuildAttachPlan` + `/v1/alerts/duplicate-exit-ips` |

## 真实 WARP（sing-box + wgcf）

本机一键（依赖已有 `.local-warp-e2e` 栈 / `local-full-e2e.sh` 部署过）：

```bash
# 需要: ~/.local/bin/sing-box、wgcf
bash tools/warp-gateway/scripts/local-real-warp.sh
```

手动步骤：

1. 安装 `sing-box`（≥1.12，推荐 1.13+）与 `wgcf`。
2. 用 `wgcf register && wgcf generate` 得到 `wgcf-profile.conf`（每个出口账号一份）。
3. 启动网关（**userspace WireGuard，无需 CAP_NET_ADMIN**）：

```bash
export WARP_GATEWAY_RUNTIME=sing-box
export WARP_GATEWAY_SING_BOX=$(which sing-box)
export WARP_GATEWAY_PROBE_URL=https://1.1.1.1/cdn-cgi/trace   # 避免本地 fake-ip DNS
go run ./cmd/warp-gateway -runtime sing-box -listen 127.0.0.1:19798 -token dev
```

4. `POST /v1/instances` 时提交完整 `profile`（private_key + address + peers）。

sing-box 1.12+ 使用 **endpoint** 型 WireGuard（非旧版 outbound）。gateway 会：

- 生成 SOCKS inbound → `wg-ep` endpoint
- 将 DNS 走隧道内 `1.1.1.1`（规避 Clash fake-ip `198.18.0.0/15`）
- 把 `engage.cloudflareclient.com` 解析失败/假 IP 时回落到 anycast `162.159.192.1` / `162.159.195.1`

> **不要**把 `sing-box` 子进程塞进 sub2api 主进程的 systemd 加固单元；gateway 应独立服务。

免费 WARP 多账号常共享同一 `exit_ip`（duplicate 告警属正常）；需要不同出口 IP 时用 WARP+ 或分散注册区域。

## sub2api 配置示例

```yaml
warp:
  enabled: true
  gateway:
    base_url: "http://127.0.0.1:19798"
    token: "dev"
    timeout_ms: 3000
    reconcile_interval_sec: 15
  auto_detach_unhealthy: true
  alert_duplicate_exit_ip: true
  default_group_name: "warp-pool"
```

## 管理 API（自动落库）

需管理员 JWT。`warp.enabled=true` 且 gateway 可达。

| Method | Path | 说明 |
|--------|------|------|
| GET | `/api/v1/admin/warp/status` | 是否启用 |
| GET | `/api/v1/admin/warp/snapshot` | gateway 池快照 |
| GET | `/api/v1/admin/warp/instances` | gateway 实例列表 |
| GET | `/api/v1/admin/warp/attach-plan` | 干跑落库计划（不写库） |
| POST | `/api/v1/admin/warp/sync` | **同步落库** → proxies + proxy-group 成员 |
| POST | `/api/v1/admin/warp/pools` | 创建 N 实例并同步；`register:true` 自动注册 free WARP |
| POST | `/api/v1/admin/warp/register-pool` | **一键**注册真实 WARP + 建池 + 落库 |
| POST | `/api/v1/admin/warp/bind-accounts` | 账号一键绑定 `proxy_group_id` → warp-pool |
| POST | `/api/v1/admin/warp/health-sync` | 全量健康检查后同步（不健康可摘除） |
| POST | `/api/v1/admin/warp/instances/:id/rotate` | rotate（sing-box 会重注册 profile）后同步 |

### 落库规则

- 代理名：`warp-{instance.name}`，协议 `socks5h`，host/port 来自 gateway。
- 以 **host:port** 优先 upsert；否则按 name。
- 默认创建/更新代理组 `warp.default_group_name`（默认 `warp-pool`），策略 sticky。
- `auto_detach_unhealthy=true` 时，unhealthy/error 实例 **不进组成员**，并将 proxy status 标为 error。
- 重复 `exit_ip` 写入 `alerts` 并打日志。

### 示例

```bash
# 创建 3 个 WARP 出口并写入 DB + 代理池
curl -sS -X POST https://YOUR/api/v1/admin/warp/pools \
  -H "Authorization: Bearer $ADMIN_JWT" \
  -H "Content-Type: application/json" \
  -d '{"name_prefix":"warp","count":3,"group_name":"warp-pool"}'

# 仅同步（gateway 已有实例）
curl -sS -X POST https://YOUR/api/v1/admin/warp/sync \
  -H "Authorization: Bearer $ADMIN_JWT" \
  -H "Content-Type: application/json" \
  -d '{"group_name":"warp-pool"}'
```

账号侧把 `proxy_group_id` 绑到返回的 `group.id` 即可。

### 管理前端

- 路径：`/admin/warp`（侧栏：**IP管理 → Cloudflare WARP**）
- 功能：查看实例、创建出口池（可选自动注册）、**一键注册真实 WARP**、**账号一键绑定**、同步落库、健康检查同步、单实例 rotate

### 后台 Worker

`WarpSyncWorker`：当 `warp.enabled=true` 且 gateway 配置完整时自动启动，按 `warp.gateway.reconcile_interval_sec`（默认 15s）执行 `HealthAllAndSync`。

## Profile 加密落盘

- Gateway：`WARP_GATEWAY_PROFILE_KEY`（或回退 `token`）→ AES-256-GCM 加密 `instances.json` 中的 `private_key`（前缀 `enc:v1:`）。
- API 响应中的私钥一律脱敏为 `***`。
- sub2api 可选 `warp.profile_encryption_key`（与 gateway 独立；主存储仍在 gateway data-dir）。

## 多机控制面 TLS / mTLS

Gateway：

```bash
export WARP_GATEWAY_TLS_CERT=/path/server.crt
export WARP_GATEWAY_TLS_KEY=/path/server.key
export WARP_GATEWAY_CLIENT_CA=/path/client-ca.crt   # 启用 mTLS
```

sub2api `config.yaml`：

```yaml
warp:
  gateway:
    base_url: "https://warp-gw.internal:19798"
    token: "..."
    tls_ca_file: "/path/ca.crt"
    tls_cert_file: "/path/client.crt"   # mTLS 客户端证
    tls_key_file: "/path/client.key"
```

## 账号一键绑定

```bash
curl -sS -X POST https://YOUR/api/v1/admin/warp/bind-accounts \
  -H "Authorization: Bearer $ADMIN_JWT" \
  -H "Content-Type: application/json" \
  -d '{"account_ids":[1,2,3],"group_name":"warp-pool"}'
# 或绑定全部活跃账号
# {"bind_all_active":true,"group_name":"warp-pool"}
```

## warp-gateway API 一览

| Method | Path | 说明 |
|--------|------|------|
| GET | `/healthz` | 存活 |
| GET | `/metrics` | Prometheus 文本 |
| GET/POST | `/v1/instances` | 列表 / 创建 |
| POST | `/v1/pools` | 一键 N 实例（Phase 3） |
| GET | `/v1/pools/snapshot` | 池快照 + socks URL |
| POST | `/v1/instances/{id}/start\|stop\|restart\|rotate\|health` | 生命周期 |
| POST | `/v1/health/all` | 全量健康 + 不健康 ID + 重复 IP |
| GET | `/v1/alerts/duplicate-exit-ips` | exit_ip 去重告警 |

## 安全

- Control API 建议仅 `127.0.0.1` + Bearer token。
- SOCKS 仅监听 loopback。
- WARP profile 私钥勿写入 sub2api 日志；生产应加密落盘（gateway data dir `0600`）。
