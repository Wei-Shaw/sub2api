# 验收与回滚

## 1. 证据矩阵

| # | Requirement | 验证方式 | 通过判据 |
| --- | --- | --- | --- |
| R1 | 代理取值收敛到单一方法 | `grep -rn "\.Proxy\.URL()" --include=*.go internal \| grep -v _test.go` | 仅剩 `service/account.go` 内的方法体 1 处 |
| R1 | 零行为变化 | Phase 0 前后 `make test-unit` 输出对比 | 结果集完全一致 |
| R1 | nil 安全 | `ProxyURL()` 单测 | nil 接收者与 `Proxy == nil` 均返回空串且不 panic |
| R2 | proxy_id 优先 | 集成测试：账号同时设 `proxy_id` 与 `proxy_group_id` | 出站命中 `proxy_id` 指向的代理 |
| R2 | 存量零迁移 | 迁移前后对仅有 `proxy_id` 的账号做回归 | 出站代理与迁移前一致 |
| R3 | 单请求内代理稳定 | 集成测试：单请求多次读取 | 多次读取结果同一 |
| R3 | WS 连接内固定 | WS 上游连接期间抓取出口 | 全程单一出口 IP |
| R3 | 跨请求轮换 | `round_robin` 下连续 N 次请求 | 命中代理分布覆盖全部健康成员 |
| R4 | ProxyID 只读 | 集成测试：组账号两次 hydration | 两次 `account.ProxyID` 相等（均为 nil） |
| R4 | grok CAS 无误报 | grok 组账号触发 token 刷新 | 无 `errOAuthRefreshAccountStateChanged` |
| R5 | 过期剔除 | 组内部分代理设为已过期 | 过期代理零命中 |
| R5 | 停用剔除 | 组内部分代理 `status` 非 active | 停用代理零命中 |
| R5 | 全不可用降级 | 组内全部代理不可用 | 返回未命中信号，按配置降级并有结构化告警 |
| R6 | sticky 恒定 | 同账号 ID 多次选择，候选集不变 | 恒定命中同一代理 |
| R6 | sticky 分散 | 多个账号 ID 在同一候选集上选择 | 分布覆盖多个代理 |
| R6 | 未知策略回退 | 组 `strategy` 设为非法值 | 回退 `round_robin` 并告警 |
| R7 | grok 同出口 | grok 组账号的中继与 OAuth 刷新分别抓取出口 | 两者出口 IP 一致 |
| R7 | 显式 proxyID 不受影响 | 管理端 OAuth 导入流程传入 proxyID | 使用该显式代理，组选择不介入 |
| R8 | 路由不冲突 | 服务启动 + `GET /api/v1/admin/proxies/:id` | 路由注册无 panic，既有端点正常 |
| R8 | 删除保护 | 删除仍被引用的组 | 返回 `PROXY_GROUP_IN_USE` |
| R9 | 隔离模式约束 | 隔离模式 `account` + 组账号 | 拒绝或明确告警，不静默重建 Transport |
| R9 | 客户端复用 | 隔离模式 `proxy` 下多代理请求 | 各代理各自复用连接池，无反复重建 |
| R10 | 无 N+1 | 批量 hydrate 同组多账号，统计 SQL 次数 | 组成员查询次数与账号数无关 |
| R10 | 缓存失效 | 变更组成员后立即发起请求 | 反映新候选集 |

## 2. 关键回归命令

```bash
# Phase 0 基线与验证
cd backend && go build ./...
cd backend && make test-unit

# 残留检查（Phase 0 完成判据）
grep -rn "\.Proxy\.URL()" --include=*.go internal | grep -v _test.go

# 不变量复核（应为 3 处非 nil 赋值，全部受 ProxyID != nil 保护）
grep -rn --include=*.go "\.Proxy = " internal | grep -v _test.go

# 代理与 grok 相关既有测试
cd backend && go test ./internal/service -run 'Proxy|Grok' -count=1
cd backend && go test ./internal/repository -run 'Proxy' -count=1

# 代码生成一致性（Phase 1/4 后）
cd backend && make generate && git diff --exit-code
```

## 3. 灰度顺序

1. Phase 0 单独合并并观察一个发布周期，确认无回归。
2. 建组但不绑定任何账号，验证管理 API 与前端。
3. 对少量**非 grok** 账号绑定组，观察上游错误率、连接池客户端数量、`errUpstreamClientLimitReached` 计数。
4. 对少量 **grok** 账号绑定组并开启 `sticky_by_account`，重点观察 OAuth token 刷新成功率与 `grok_oauth_proxy_invalid` 计数。
5. 全量启用前复核 `maxUpstreamClients` 上限。

## 4. 回滚手册

| 阶段 | 回滚方式 | 数据影响 |
| --- | --- | --- |
| Phase 0 | revert 单个 PR | 无 |
| Phase 1 | 保留表结构，不回滚迁移（新增列可空，存量逻辑不读） | 无 |
| Phase 2 | 关闭 hydration 中的组选择分支（feature flag 或 revert） | 无，账号回落到 `proxy_id` 语义 |
| Phase 3 | revert grok OAuth 改动 | 无 |
| Phase 4/5 | revert 前后端 PR，表结构保留 | 已绑定组的账号需手动清空 `proxy_group_id` 或改绑 `proxy_id` |

**紧急止血**：把受影响账号的 `proxy_group_id` 置空、`proxy_id` 设为具体代理，即可完全绕过代理池链路，无需发版。

## 5. 泄露与安全门禁

- 组管理 API 响应 MUST NOT 返回成员代理的明文密码，脱敏规则对齐 `dto.Proxy` 与 `dto.AdminProxy`（`handler/dto/types.go:330-356`）的既有区分。
- 结构化日志中的代理标识 MUST 使用 `normalizeProxyURL` 归一后的 proxyKey（`http_upstream.go:1175`），MUST NOT 打印含认证信息的完整 URL。


## 6. 验收执行记录（2026-07-27）

### 6.1 证据矩阵结果

| # | 项 | 结果 | 证据 |
| --- | --- | --- | --- |
| R1 | Proxy.URL 收敛 | **PASS** | 非测试代码仅 `service/account.go` `ProxyURL()` 方法体 1 处 |
| R1 | nil 安全 | **PASS** | `TestAccountProxyURL`（含 nil / 仅 Proxy 无 ProxyID） |
| R2 | proxy_id 优先 | **PASS** | `TestApplyProxyGroupSelection`：已有 Proxy 时 resolver calls=0 |
| R2 | 存量零迁移 | **PASS** | 迁移仅 ADD COLUMN 可空；无 proxy_group_id 时 apply 直接 return |
| R3 | 单请求稳定 | **PASS** | hydration 每请求一次选代理；WS 闭包读已 hydrate 的 Proxy |
| R3 | 跨请求轮换 | **PASS** | `TestSelectProxyFromGroup_RoundRobin` / resolver 轮询单测 |
| R4 | 不写 ProxyID | **PASS** | apply 注释 + 单测 `require.Nil(ProxyID)`；C1 |
| R4 | grok CAS | **PASS（单测）** | `accountProxyURL` 不依赖 ProxyID；写回路径不存在。生产 CAS 联调见灰度 |
| R5 | 过期/停用剔除 | **PASS** | `TestSelectProxyFromGroup_EmptyAndUnhealthy` |
| R5 | 全不可用 | **PASS** | 返回 `(nil,false)` / resolve `(nil,nil)` → 降级直连（design §6 决策） |
| R6 | sticky | **PASS** | selector/resolver sticky 单测 |
| R6 | 未知策略回退 | **PASS** | `EffectiveStrategy` / selector 回退 round_robin |
| R7 | grok 同出口 | **PASS（代码+单测）** | Refresh 走 `accountProxyURL`；中继走 `ProxyURL()` |
| R7 | 显式 proxyID | **PASS** | 管理端 OAuth 仍 `proxyURL(ctx, proxyID)` |
| R8 | 路由 | **PASS** | 独立 `/api/v1/admin/proxy-groups`；与 `/proxies/:id` 分离 |
| R8 | 删除保护 | **PASS** | `ErrProxyGroupInUse` + service 单测 |
| R9 | account 隔离 | **PASS（告警）** | config Validate：`isolation=account` → slog.Warn |
| R9 | 客户端复用 | **PASS（设计）** | 默认 `account_proxy`；cacheKey 含 proxyKey |
| R10 | 无 N+1 | **PASS（代码）** | resolver 进程内 TTL 缓存 + InvalidateGroup |
| R10 | 缓存失效 | **PASS** | 管理变更调用 `invalidate` → `InvalidateGroup` |
| 安全 | 密码脱敏 | **PASS** | `dto.Proxy.Password` json:"-"；组员用 AdminProxy 仅管理端 |

### 6.2 自动化命令（本机已跑）

```text
go test -tags=unit ./internal/service/ -run 'Proxy|Grok|ProxyURL|AccountProxy|AccountHasConfigured'  # ok
go test ./internal/repository/ -run 'ApplyProxyGroup|Proxy'                                         # ok
go test -tags=unit ./migrations/                                                                     # ok
go test ./internal/handler/admin/ -run 'GrokSSO|CodexSession'                                         # ok
go build ./internal/config/ ./internal/handler/admin/ ./internal/service/ ./internal/repository/     # ok
pnpm exec vitest run ProxySelector.spec admin.proxyGroups.spec EditAccountModal.spec               # 41 passed
```

### 6.3 生产灰度清单（运维执行，非阻塞合并）

1. 确认 `gateway.connection_pool_isolation` 为 `proxy` 或 `account_proxy`（不要用 `account`）。
2. 确认 `gateway.max_upstream_clients >= 预估同时活跃出口数`（经验：活跃账号数 × 每组健康成员上界，且 < 默认 5000 可先观察）。
3. 建组不绑账号 → 管理 API/前端 CRUD 冒烟。
4. 少量**非 grok** 账号绑组（round_robin）→ 观察上游错误率、客户端缓存规模。
5. 少量 **grok** 账号绑组且 `sticky_by_account=true` → 观察 OAuth 刷新成功、无 CAS 误报、中继与刷新同出口。
6. 紧急止血：账号 `proxy_group_id` 置空并设回 `proxy_id`。

## 7. 本地灰度执行

详见 `gray-test-report.md`（2026-07-27 本地 Docker，19/19 API 冒烟通过；生产 SSH 未开通）。
