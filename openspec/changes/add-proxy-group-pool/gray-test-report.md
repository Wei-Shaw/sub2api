# 代理池灰度测试报告

- 时间：2026-07-27T13:16:44
- 环境：本地 Docker 灰度栈（非生产）
- 入口：http://127.0.0.1:18080
- 编排：`deploy/docker-compose.dev.yml` + `deploy/docker-compose.gray.yml` + `deploy/.env`
- 镜像：`deploy-sub2api:latest`（本仓库源码构建）

## 前置检查

| 项 | 结果 |
|---|---|
| 生产 SSH `root@185.158.137.77` | **不可用**（Permission denied publickey/password） |
| 连接池隔离 | `GATEWAY_CONNECTION_POOL_ISOLATION=account_proxy` |
| max_upstream_clients | `5000` |
| 迁移 `proxy_groups` 表 | **已存在** |
| `accounts.proxy_group_id` 列 + FK | **已存在** |

## API 冒烟（/tmp/sub2api_gray_test.py）

**19/19 PASS**

覆盖：
1. health / admin login / compliance 签署
2. 创建 3 个成员代理
3. 创建代理组（round_robin → 后改 sticky）
4. list/all、get detail、成员数=3
5. 成员密码不泄露
6. `/admin/proxies/:id` 路由不冲突
7. 删除在用组 → `409 PROXY_GROUP_IN_USE`
8. 非 grok 账号绑定 `proxy_group_id`、回读、用 `0` 清除、再绑定
9. 组开启 `sticky_by_account`
10. grok 账号绑定 sticky 组（`proxy_id` 为空，仅 `proxy_group_id`）

## 数据库落库抽查

```
proxy_groups: id=1 strategy=sticky sticky_by_account=t
proxies: 1/2/3 group_id=1
accounts: anthropic#1 + grok#2 → proxy_group_id=1, proxy_id NULL
```

## 未在本环境执行（需真实上游/生产流量）

| 项 | 原因 |
|---|---|
| 真实出口 IP 抓取（中继 vs OAuth 同出口） | 成员代理为 127.0.0.x 占位，无真实上游 |
| grok OAuth token 刷新成功率 / CAS | 账号为假 apikey，无有效 refresh token |
| 连接池客户端数量与 `errUpstreamClientLimitReached` | 无真实上游流量 |
| 生产灰度步骤 3–5 | 生产 SSH 无密钥；需你提供访问方式后在 185.158.137.77 执行 |

## 紧急止血（已验证 API）

账号 `PUT proxy_group_id=0` 可清除组绑定（本测步骤 11 PASS）。

## 环境保留

```bash
# 查看
docker ps | grep sub2api
curl -s http://127.0.0.1:18080/health

# 停止（需要时）
cd deploy && docker compose -f docker-compose.dev.yml -f docker-compose.gray.yml --env-file .env down

# 凭证
cat /tmp/sub2api-gray-creds.txt   # 仅本机灰度用
```
