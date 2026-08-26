# 高可用调度开发者指南

本文面向需要开发、调试或维护高可用账号调度的人员。当前实现以 OpenAI 兼容网关为入口；未开启高可用的旧分组继续走 legacy 调度。新增配置和运行时状态均保持向后兼容，不会改变已有用户、账号和用量数据。

## 1. 生效条件与配置层级

高可用由三层配置共同决定：

| 层级 | 配置位置 | 作用 |
| --- | --- | --- |
| 服务级 | `data/config.yaml` 的 `gateway.openai_scheduler.strategy` | `legacy` 或 `high_availability`。服务级为 `legacy` 时不会启用 HA 健康调度。 |
| 设置级 | 管理后台“设置 → OpenAI 实验调度策略” | `openai_advanced_scheduler_enabled` 控制高级调度运行时逻辑；粘性加权、订阅优先、LB Top-K 是附加选项。 |
| 分组级 | `groups.scheduler`（数据库 JSON 字段） | 每个分组独立设置 `strategy` 和 `selection_mode`。`selection_mode` 为 `weighted`（加权探索）或 `strict_health`（严格健康优先）。 |

推荐的 HA 配置片段如下。将字段合并进现有文件，不要用示例文件覆盖线上配置：

```yaml
gateway:
  openai_scheduler:
    strategy: high_availability
    sticky_binding_mode: keep_original
    failover_on_health_escape: false
    high_availability_error_rate_weight: 1.5
    high_availability_ttft_weight: 1.5
    high_availability_total_latency_weight: 0.25
    health_circuit_enabled: true
    health_circuit_failure_threshold: 3
    health_circuit_window_seconds: 60
    health_circuit_cooldown_seconds: 30
    cold_start_probe_enabled: true
    cold_start_probe_quota_per_minute: 8
    sticky_escape_enabled: true
    sticky_escape_ttft_ms: 15000
    sticky_escape_error_rate: 0.5
```

关键字段：

- `sticky_binding_mode: keep_original`：粘性会话因健康逃逸临时换号后，健康恢复时仍回到原绑定账号。`rebind_on_failover` 才会在真正 failover 后迁移绑定；`failover_on_health_escape` 默认关闭，因此普通健康评分逃逸不会迁移绑定。
- `high_availability_*_weight`：只在 HA 模式覆盖对应评分权重。错误率和首字响应（TTFT）优先；同步请求总耗时按输出 token 平方根归一化后以低权重参与。
- `health_circuit_*`：账号级短路参数。达到连续失败阈值后，在冷却时间内暂时不调度；成功请求会清除短路状态。
- `cold_start_probe_*`：无近期样本账号的有限真实流量探索额度，不会额外生成上游请求。
- `sticky_escape_*`：粘性账号 TTFT 或错误率明显恶化时允许临时跳过。阈值为 0 时相应条件关闭。

基础评分权重仍位于 `gateway.openai_ws.scheduler_score_weights`（`priority`、`load`、`queue`、`error_rate`、`ttft`、`total_latency`、`reset`、`quota_headroom`、`upstream_cost`、`previous_response`、`session_sticky`）。HA 只覆盖上面三个 `high_availability_*_weight` 字段；其余因子沿用 `openai_ws` 配置。管理后台设置中的 LB Top-K 会在高级调度开关打开时覆盖 `gateway.openai_ws.lb_top_k`，严格健康模式会把候选全部纳入排序。

### 分组数据兼容

`groups.scheduler` 是新增字段，旧数据为空时由后端归一化为：

```json
{
  "strategy": "legacy",
  "selection_mode": "weighted",
  "sticky_binding_mode": "keep_original",
  "first_byte_failover": false,
  "probe_bypass_sticky": false,
  "max_account_switches": 0,
  "same_account_retry_attempts": 0
}
```

`max_account_switches: 0` 表示沿用服务级全局上限，不是“禁止切号”。分组 API 返回的 `scheduler.selection_mode` 应使用规范化后的值；前端编辑页只在打开“高可用优先”后显示账号选择模式。

## 2. 健康样本与评分

运行时样本保存在进程内健康统计，并按新鲜度过期：真实请求样本 TTL 为 15 分钟，定时测试样本 TTL 为 5 分钟。服务重启后这些内存样本会冷启动，不会修改用户、账号或用量数据。

### 真实请求样本

- 成功请求记录成功样本、TTFT；同步成功请求额外记录总耗时。
- 总耗时按 `total_latency_ms / sqrt(output_tokens)` 归一化，避免长输出天然被判定为慢。
- 400 等请求级/请求范围错误不会污染账号错误率；429、认证错误、5xx、超时和网络错误会进入错误分类及健康 EWMA。

### 定时测试样本

账号定时测试计划中的“纳入健康样本”默认关闭。打开后，探测成功/失败和耗时以 `0.25` 的低权重混入健康快照；探测连续失败仍用于严格健康分层。只有探测模型、代理和上游链路与实际流量相近时才建议启用。

### 严格健康分层

`strict_health` 先分层，再在同层按评分排序：

- Tier 0：错误率 `< 20%`，正常候选。
- Tier 1：错误率 `20% - 50%`，降级候选。
- Tier 2：错误率 `>= 50%`，或定时测试连续失败至少 2 次，暂时不可用。

Tier 2 不会从计划中永久删除：当整个分组都不健康时仍作为最后的 best-effort 候选，避免直接返回“无可用账号”。同一层内综合优先级、负载、队列、错误率、TTFT、同步归一化总耗时及可选成本/额度因子排序。严格模式会把全部候选纳入排序，而不是只看 LB Top-K。

## 3. 请求路由与 failover 边界

一次请求的大致顺序是：请求解析和模型能力过滤 → 读取分组调度配置 → previous response/session sticky 命中 → 健康分层与评分 → 负载/槽位准入 → 向上游转发。首字节前发生允许 failover 的错误时，排除已尝试账号并按同一分组重新选择；首字节已经写出后不会重放整次请求。

常见可 failover 条件由 `shouldFailoverUpstreamError` 和请求上下文共同决定：401、403、429、529 以及其他 5xx 默认允许换号；请求体、模型或参数错误等请求级错误不会把账号判坏。非池模式 HA 请求遇到上游 524 或传输错误时，首字节前优先换其他账号；池模式保留账号内重试语义。客户端已断开、请求已产生语义输出、或达到切号上限时直接结束。

同一账号的 429 可能先按 `Retry-After` 做有限重试；达到该账号重试预算后才进入下一账号。`max_account_switches` 控制整次请求最多切换次数，默认使用服务级值。每次切号都会写入使用记录中的 `failover` 事件。

## 4. 日志与观测

### 开启 debug

在运行实例实际挂载的数据目录编辑 `data/config.yaml`：

```yaml
log:
  level: debug
```

重启应用容器使配置加载。Docker 部署可执行：

```bash
cd /opt/sub2api
docker compose --env-file .env -p sub2api -f docker-compose.local.yml -f docker-compose.ha-prod.yml up -d sub2api
```

`log.level` 是启动配置，修改后不会由管理后台热加载；仅修改数据库中的高级调度设置通常在约 5 秒缓存周期后生效，不需要重启。

开发/测试 Compose 可使用 `HA_LOG_LEVEL=debug`，它只影响测试覆盖配置，不要把 `HA_*` 变量带到正式环境。验证结束恢复 `info`，高流量长期 debug 会显著增加日志量和磁盘写入。

日志默认同时写容器 stdout 和 `data/logs/sub2api.log`（取决于 `log.output.to_file`）。实时查看：

```bash
docker compose --env-file .env -p sub2api -f docker-compose.local.yml -f docker-compose.ha-prod.yml logs -f --tail=200 sub2api
tail -F /opt/sub2api/data/logs/sub2api.log
```

### 调度日志事件

| 事件 | 级别 | 重点字段 |
| --- | --- | --- |
| `openai.ha_scheduler.candidates` | debug | `group_id`、`selection_mode`、`candidate_count`、`candidates[]`。数组包含 `account_id`、`score`、`error_rate`、`ttft_ms`、`total_latency_ms`、`load_rate`、`waiting_count`、`health_tier`、`health_reason`、`health_sample_source`。 |
| `openai.ha_scheduler.selection` | debug | `outcome`、`layer`、`selected_account_id`、`sticky_previous_hit`、`sticky_session_hit`、`scheduler_latency_ms`。 |
| `openai.ha_scheduler.account_result` | debug | `success`、`failure_category`、`health_error_rate`、`health_ttft_ms`、`health_sample_source`、`local_circuit_open`、`first_token_ms`。 |
| `openai.ha_scheduler.circuit_opened` | warn | `account_id`、`failure_category`、`circuit_until`、`distributed`。 |
| `openai.ha_scheduler.circuit_recovered` | info | `account_id`。 |
| `openai.ha_scheduler.health_cache_fallback` | warn | `operation`、`fallback=process_local`，表示共享健康缓存失败并降级为进程内统计。 |
| `openai.upstream_failover_switching` | warn | `upstream_status`、当前/下一个账号及切号计数（具体字段随协议路径略有差异）。 |

JSON 日志可用 `jq` 快速筛选：

```bash
docker compose ... logs --since=10m sub2api | jq 'select(.msg|test("openai\\.ha_scheduler"))'
docker compose ... logs --since=10m sub2api | jq 'select(.msg=="openai.ha_scheduler.candidates") | {time:.ts,group:.group_id,mode:.selection_mode,candidates:.candidates}'
```

console 格式没有统一 JSON 字段时，使用：

```bash
docker compose ... logs --since=10m sub2api | rg 'openai\.ha_scheduler\.(candidates|selection|account_result)|openai\.upstream_failover_switching'
```

判断“慢账号为何仍被选中”时，必须同时查看 `health_tier`、`health_reason` 和 `health_sample_source`，不能只比较原始 `score`：Tier 0 一定优先于 Tier 1/2；若全组都是 Tier 2，才会按分数做兜底排序。

### 运行时指标快照

服务内部可通过 `SnapshotOpenAIAccountSchedulerMetrics()` 获取进程级快照，包含选择次数、粘性命中率、切号次数、调度延迟、成功/失败分类、短路开关次数及定时测试成功/失败计数。该快照不是持久化历史，重启后归零；长期趋势应结合使用记录、探测记录和日志采集系统。

### 代码入口

- 配置结构和默认值：`backend/internal/config/config.go`
- 分组调度归一化和数据库映射：`backend/internal/service/group.go`、`backend/internal/repository/group_repo.go`
- 候选过滤、评分、健康分层：`backend/internal/service/openai_account_scheduler.go`
- 调度日志和指标：`backend/internal/service/openai_account_scheduler_observability.go`
- failover 判断和请求级 524/传输错误处理：`backend/internal/service/gateway_forward.go`、`backend/internal/handler/failover_loop.go`
- 定时测试健康样本入口：`backend/internal/service/scheduled_test_runner_service.go`

## 5. 开发与测试注意事项

- 不要把健康 EWMA、短路状态或 sticky 绑定写入用户/账号主表；它们必须可丢失、可重建。
- 新增字段优先使用已有 `scheduler` JSON 和配置默认值，确保旧数据反序列化后仍为 legacy/weighted。
- 变更 failover 判断时同时覆盖 HTTP 同步、SSE、WebSocket、Anthropic/Gemini 兼容路径，并增加“首字节前/后”和客户端取消测试。
- 定时测试默认不污染健康样本；涉及该行为的测试必须显式设置 `include_in_health_samples=true`。
- 调度 debug 日志只记录账号 ID、评分和分类，不记录令牌、授权头或完整请求体。
- 修改后至少运行后端相关单测、前端 `vue-tsc`/静态检查和 `git diff --check`；合并 upstream 时优先 rebase，再检查迁移编号和生成代码。

## 6. 快速排障清单

1. 确认 `gateway.openai_scheduler.strategy`、设置级高级调度开关和目标分组 `scheduler.strategy` 均已生效。
2. 调用 `/api/v1/admin/groups` 检查返回的 `scheduler.selection_mode`，不要只看编辑表单默认值。
3. 开启 debug，确认是否出现 `openai.ha_scheduler.candidates`；没有事件通常表示请求没有进入 HA 路径或日志级别未加载。
4. 查看候选的 `health_sample_source`：`real`、`probe`、`real_and_probe` 或 `cold`。
5. 查看 `health_tier`/`health_reason`；连续探测失败至少两次应为 Tier 2。
6. 查看 `openai.upstream_failover_switching` 的状态码、切号计数和是否已达到上限。
7. 检查 `health_cache_fallback`、数据库/Redis 错误和容器重启时间；进程内健康样本在重启后会重新冷启动。
