# 后端热点 API 性能优化审计与行动计划（2026-02-22）

## 1. 目标与范围

本次文档用于沉淀后端热点 API 的性能审计结果，并给出可执行优化方案。

重点链路：
- `POST /v1/messages`
- `POST /v1/responses`
- `POST /sora/v1/chat/completions`
- `POST /v1beta/models/*modelAction`（Gemini 兼容链路）
- 相关调度、计费、Ops 记录链路

## 2. 审计方式与结论边界

- 审计方式：静态代码审阅（只读），未对生产环境做侵入变更。
- 结论类型：以“高置信度可优化点”为主，均附 `file:line` 证据。
- 未覆盖项：本轮未执行压测与火焰图采样，吞吐增益需在压测环境量化确认。

## 3. 优先级总览

| 优先级 | 数量 | 结论 |
|---|---:|---|
| P0（Critical） | 2 | 存在资源失控风险，建议立即修复 |
| P1（High） | 2 | 明确的热点 DB/Redis 放大路径，建议本迭代完成 |
| P2（Medium） | 4 | 可观收益优化项，建议并行排期 |

## 4. 详细问题清单

### 4.1 P0-1：使用量记录为“每请求一个 goroutine”，高峰下可能无界堆积

证据位置：
- `backend/internal/handler/gateway_handler.go:435`
- `backend/internal/handler/gateway_handler.go:704`
- `backend/internal/handler/openai_gateway_handler.go:382`
- `backend/internal/handler/sora_gateway_handler.go:400`
- `backend/internal/handler/gemini_v1beta_handler.go:523`

问题描述：
- 记录用量使用 `go func(...)` 直接异步提交，未设置全局并发上限与排队背压。
- 当 DB/Redis 变慢时，goroutine 数会随请求持续累积。

性能影响：
- `goroutine` 激增导致调度开销上升与内存占用增加。
- 与数据库连接池（默认 `max_open_conns=256`）竞争，放大尾延迟。

优化建议：
- 引入“有界队列 + 固定 worker 池”替代每请求 goroutine。
- 队列满时采用明确策略：丢弃（采样告警）或降级为同步短路。
- 为 `RecordUsage` 路径增加超时、重试上限与失败计数指标。

验收指标：
- 峰值 `goroutines` 稳定，无线性增长。
- 用量记录成功率、丢弃率、队列长度可观测。

---

### 4.2 P0-2：Ops 错误日志队列携带原始请求体，存在内存放大风险

证据位置：
- 队列容量与 job 结构：`backend/internal/handler/ops_error_logger.go:38`、`backend/internal/handler/ops_error_logger.go:43`
- 入队逻辑：`backend/internal/handler/ops_error_logger.go:132`
- 请求体放入 context：`backend/internal/handler/ops_error_logger.go:261`
- 读取并入队：`backend/internal/handler/ops_error_logger.go:548`、`backend/internal/handler/ops_error_logger.go:563`、`backend/internal/handler/ops_error_logger.go:727`、`backend/internal/handler/ops_error_logger.go:737`
- 入库前才裁剪：`backend/internal/service/ops_service.go:332`、`backend/internal/service/ops_service.go:339`
- 请求体默认上限：`backend/internal/config/config.go:1082`、`backend/internal/config/config.go:1086`

问题描述：
- 队列元素包含 `[]byte requestBody`，在请求体较大且错误风暴时会显著占用内存。
- 当前裁剪发生在 worker 消费时，而不是入队前。

性能影响：
- 容易造成瞬时高内存与频繁 GC。
- 极端情况下可能触发 OOM 或服务抖动。

优化建议：
- 入队前进行“脱敏 + 裁剪”，仅保留小尺寸结构化片段（建议 8KB~16KB）。
- 队列存放轻量 DTO，避免持有大块 `[]byte`。
- 按错误类型控制采样率，避免同类错误洪峰时日志放大。

验收指标：
- Ops 错误风暴期间 RSS/GC 次数显著下降。
- 队列满时系统稳定且告警可见。

---

### 4.3 P1-1：窗口费用检查在缓存 miss 时逐账号做 DB 聚合

证据位置：
- 候选筛选多处调用：`backend/internal/service/gateway_service.go:1109`、`backend/internal/service/gateway_service.go:1137`、`backend/internal/service/gateway_service.go:1291`、`backend/internal/service/gateway_service.go:1354`
- miss 后单账号聚合：`backend/internal/service/gateway_service.go:1791`
- SQL 聚合实现：`backend/internal/repository/usage_log_repo.go:889`
- 窗口费用缓存 TTL：`backend/internal/repository/session_limit_cache.go:33`
- 已有批量读取接口但未利用：`backend/internal/repository/session_limit_cache.go:310`

问题描述：
- 路由候选过滤阶段频繁调用窗口费用检查。
- 缓存未命中时逐账号执行聚合查询，账号多时放大 DB 压力。

性能影响：
- 路由耗时上升，数据库聚合 QPS 增长。
- 高并发下可能形成“缓存抖动 + 聚合风暴”。

优化建议：
- 先批量 `GetWindowCostBatch`，仅对 miss 账号执行批量 SQL 聚合。
- 将聚合结果批量回写缓存，降低重复查询。
- 评估窗口费用缓存 TTL 与刷新策略，减少抖动。

验收指标：
- 路由阶段 DB 查询次数下降。
- `SelectAccountWithLoadAwareness` 平均耗时下降。

---

### 4.4 P1-2：记录用量时每次查询用户分组倍率，形成稳定 DB 热点

证据位置：
- `backend/internal/service/gateway_service.go:5316`
- `backend/internal/service/gateway_service.go:5531`
- `backend/internal/repository/user_group_rate_repo.go:45`

问题描述：
- `RecordUsage` 与 `RecordUsageWithLongContext` 每次都执行 `GetByUserAndGroup`。
- 热路径重复读数据库，且与 usage 写入、扣费路径竞争连接池。

性能影响：
- 增加 DB 往返与延迟，降低热点接口吞吐。

优化建议：
- 在鉴权或路由阶段预热倍率并挂载上下文复用。
- 引入 L1/L2 缓存（短 TTL + singleflight），减少重复 SQL。

验收指标：
- `GetByUserAndGroup` 调用量明显下降。
- 计费链路 p95 延迟下降。

---

### 4.5 P2-1：Claude 消息链路重复 JSON 解析

证据位置：
- 首次解析：`backend/internal/handler/gateway_handler.go:129`
- 二次解析入口：`backend/internal/handler/gateway_handler.go:146`
- 二次 `json.Unmarshal`：`backend/internal/handler/gateway_helper.go:22`、`backend/internal/handler/gateway_helper.go:26`

问题描述：
- 同一请求先 `ParseGatewayRequest`，后 `SetClaudeCodeClientContext` 再做 `Unmarshal`。

性能影响：
- 增加 CPU 与内存分配，尤其对大 `messages` 请求更明显。

优化建议：
- 仅在 `User-Agent` 命中 Claude CLI 规则后再做 body 深解析。
- 或直接复用首轮解析结果，避免重复反序列化。

---

### 4.6 P2-2：同一请求中粘性会话账号查询存在重复 Redis 读取

证据位置：
- Handler 预取：`backend/internal/handler/gateway_handler.go:242`
- Service 再取：`backend/internal/service/gateway_service.go:941`、`backend/internal/service/gateway_service.go:1129`、`backend/internal/service/gateway_service.go:1277`

问题描述：
- 同一会话映射在同请求链路被多次读取。

性能影响：
- 增加 Redis RTT 与序列化开销，抬高路由延迟。

优化建议：
- 统一在 `SelectAccountWithLoadAwareness` 内读取并复用。
- 或将上层已读到的 sticky account 显式透传给 service。

---

### 4.7 P2-3：并发等待路径存在重复抢槽

证据位置：
- 首次 TryAcquire：`backend/internal/handler/gateway_helper.go:182`、`backend/internal/handler/gateway_helper.go:202`
- wait 内再次立即 Acquire：`backend/internal/handler/gateway_helper.go:226`、`backend/internal/handler/gateway_helper.go:230`、`backend/internal/handler/gateway_helper.go:232`

问题描述：
- 进入 wait 流程后会再做一次“立即抢槽”，与上层 TryAcquire 重复。

性能影响：
- 在高并发下增加 Redis 操作次数，放大锁竞争。

优化建议：
- wait 流程直接进入退避循环，避免重复立即抢槽。

---

### 4.8 P2-4：`/v1/models` 每次走仓储查询与对象装配，未复用快照/短缓存

证据位置：
- 入口调用：`backend/internal/handler/gateway_handler.go:767`
- 服务查询：`backend/internal/service/gateway_service.go:6152`、`backend/internal/service/gateway_service.go:6154`
- 对象装配：`backend/internal/repository/account_repo.go:1276`、`backend/internal/repository/account_repo.go:1290`、`backend/internal/repository/account_repo.go:1298`

问题描述：
- 模型列表请求每次都落到账号查询与附加装配，缺少短时缓存。

性能影响：
- 高频请求下持续占用 DB 与 CPU。

优化建议：
- 以 `groupID + platform` 建 10s~30s 本地缓存。
- 或复用调度快照 bucket 的可用账号结果做模型聚合。

## 5. 建议实施顺序

### 阶段 A（立即，P0）
- 将“用量记录每请求 goroutine”改为有界异步管道。
- Ops 错误日志改为“入队前裁剪 + 轻量队列对象”。

### 阶段 B（短期，P1）
- 批量化窗口费用检查（缓存 + SQL 双批量）。
- 用户分组倍率加缓存/上下文复用。

### 阶段 C（中期，P2）
- 消除重复 JSON 解析与重复 sticky 查询。
- 优化并发等待重复抢槽逻辑。
- `/v1/models` 接口加入短缓存或快照复用。

## 6. 压测与验证建议

建议在预发压测以下场景：
- 场景 1：常规成功流量（验证吞吐与延迟）。
- 场景 2：上游慢响应（验证 goroutine 与队列稳定性）。
- 场景 3：错误风暴（验证 Ops 队列与内存上限）。
- 场景 4：多账号大分组路由（验证窗口费用批量化收益）。

建议监控指标：
- 进程：`goroutines`、RSS、GC 次数/停顿。
- API：各热点接口 p50/p95/p99。
- DB：QPS、慢查询、连接池等待。
- Redis：命中率、RTT、命令量。
- 业务：用量记录成功率/丢弃率、Ops 日志丢弃率。

## 7. 待补充数据

- 生产真实错误率与错误体大小分布。
- `window_cost_limit` 实际启用账号比例。
- `/v1/models` 实际调用频次。
- DB/Redis 当前容量余量与瓶颈点。

---

如需进入实现阶段，建议按“阶段 A → 阶段 B → 阶段 C”分 PR 推进，每个阶段都附压测报告与回滚方案。
