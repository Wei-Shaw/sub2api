# 日志专项审计与整理（2026-02-12）

## 1. 全量扫描结论

- 扫描范围：`backend/` + `frontend/`
- 日志相关调用总量（粗统计）：约 `4100` 处
- 后端标准库日志（`log.Printf/Println/Fatal*`）：`808` 处（本轮整改后剩余 `269` 处）
- 前端 `console.*`：`180` 处

关键观察：

1. 后端大量业务日志仍走标准库 `log`，在当前初始化流程里会被统一当作 `INFO` 输出，导致“错误/告警等级失真”。
2. 网关关键链路（OpenAI/Gemini/Sora）原有日志以格式化字符串为主，上下文字段（`request_id/user_id/group_id/model/account_id`）不完整，排障时需要人工拼接上下文。
3. Token 刷新服务同时混用 `log` 与 `slog`，同类事件日志风格不一致，不利于检索与聚合。
4. 前端 `console.error/warn` 使用量高，缺少统一封装，生产环境噪音和敏感信息泄漏风险较高。

## 2. 本次已落地整改

### 2.1 全局层（后端标准库日志分级修复）

- 修改：`backend/internal/pkg/logger/logger.go`
- 结果：
  1. 替换原 `zap.RedirectStdLogAt(..., INFO)` 机制，改为自定义 `stdlog bridge`。
  2. 对标准库日志自动推断等级（`DEBUG/WARN/ERROR/INFO`），并打上 `legacy_stdlog=true` 标记。
  3. 规范化消息文本（去换行、压缩空白），提升可读性和检索稳定性。
  4. 调整初始化顺序：先桥接 `slog`，再桥接 `stdlog`，避免 `slog.SetDefault` 覆盖标准库桥接。
  5. 新增 `logger.LegacyPrintf(component, format, ...args)`，用于后端历史 `printf` 日志的平滑迁移，自动推断等级并打 `legacy_printf=true` 标记。

### 2.2 核心请求链路结构化改造

- 新增：`backend/internal/handler/logging.go`
  - 统一提供请求级 logger 获取入口，继承中间件注入的 `request_id` 上下文。

- 改造文件：
  - `backend/internal/handler/gateway_handler.go`
  - `backend/internal/handler/openai_gateway_handler.go`
  - `backend/internal/handler/gemini_v1beta_handler.go`
  - `backend/internal/handler/sora_gateway_handler.go`
  - `backend/internal/service/antigravity_gateway_service.go`
  - `backend/internal/service/gateway_service.go`
  - `backend/internal/service/gemini_oauth_service.go`
  - `backend/internal/service/auth_service.go`
  - `backend/internal/setup/setup.go`
  - `backend/internal/service/usage_cleanup_service.go`
  - `backend/internal/service/pricing_service.go`
  - `backend/internal/repository/account_repo.go`
  - `backend/internal/service/openai_gateway_service.go`
  - `backend/internal/service/scheduler_snapshot_service.go`
  - `backend/internal/service/gemini_messages_compat_service.go`
  - `backend/internal/service/dashboard_aggregation_service.go`
  - `backend/internal/service/billing_cache_service.go`
  - `backend/internal/repository/claude_oauth_service.go`
  - `backend/internal/service/admin_service.go`
  - `backend/internal/handler/admin/ops_ws_handler.go`

- 改造内容：
  1. 把关键日志从字符串拼接改为结构化字段。
  2. 统一带上 `component/user_id/api_key_id/group_id/model/account_id` 等字段。
  3. 按语义拆分等级：
     - 预期业务拒绝（如账单校验失败、队列满）使用 `Info`
     - 降级路径/可恢复异常（如抢槽失败、粘性会话绑定失败）使用 `Warn`
     - 真正故障（如转发失败、使用量记录失败）使用 `Error`
  4. 新增请求完成日志（`*.request_completed`）用于链路闭环追踪。
  5. 对高密度 `log.Printf` 完成批量迁移到 `logger.LegacyPrintf`（本轮累计 511 处），并统一组件字段：
     - `component=service.antigravity_gateway`
     - `component=service.gateway`
     - `component=service.gemini_oauth`
     - `component=service.auth`
     - `component=setup`
     - `component=service.usage_cleanup`
     - `component=service.pricing`
     - `component=repository.account`
     - `component=service.openai_gateway`
     - `component=service.scheduler_snapshot`
     - `component=service.gemini_messages_compat`
     - `component=service.dashboard_aggregation`
     - `component=service.billing_cache`
     - `component=repository.claude_oauth`
     - `component=service.admin`
     - `component=handler.admin.ops_ws`
  6. OpenAI 透传断流相关两条关键告警统一回到新日志系统输出（`service.openai_gateway`），并通过兼容逻辑保证测试环境可捕获。

### 2.3 后台任务日志统一

- 改造：`backend/internal/service/token_refresh_service.go`
- 结果：
  1. 统一改为 `slog` 结构化输出。
  2. `retry/cycle/account` 等事件改为字段化日志，便于按账号和批次检索。
  3. 对“无实际刷新活动”的周期日志降级到 `Debug`，减少噪音。

### 2.4 测试保障

- 新增：`backend/internal/pkg/logger/stdlog_bridge_test.go`
  - 覆盖标准库日志等级推断、消息标准化、输出路由行为。
- 已验证：
  - `go test ./internal/pkg/logger ./internal/handler ./internal/service` 通过。

## 3. 仍需继续整改（建议下一批）

### 3.1 后端剩余 `std log` 高密度区域（优先级 P1）

建议优先处理以下文件（调用量高）：

1. `backend/internal/service/usage_cleanup_service.go`（26）
2. `backend/internal/service/pricing_service.go`（26）
3. `backend/internal/repository/account_repo.go`（24）
4. `backend/internal/service/openai_gateway_service.go`（23）
5. `backend/internal/service/scheduler_snapshot_service.go`（20）

（以上已完成。当前 Top 5 已变为：`backend/cmd/server/main.go`、`backend/internal/service/openai_tool_corrector.go`、`backend/internal/service/email_queue_service.go`、`backend/internal/config/config.go`、`backend/internal/service/ops_cleanup_service.go`）

目标：逐步替换为结构化日志，减少对 `legacy_stdlog` 兼容桥接的依赖。

### 3.2 前端日志治理（优先级 P1）

建议新增统一前端日志工具（如 `src/utils/logger.ts`）并分三步替换：

1. `console.error/warn/debug/log` 全部收敛到统一 API；
2. 生产环境默认降噪（仅保留关键告警/错误）；
3. 统一字段（模块名、请求ID、用户ID、路由、错误码）并避免打印敏感数据。

### 3.3 日志规范与门禁（优先级 P2）

建议补充：

1. 日志规范文档（等级定义、字段最小集、脱敏要求）；
2. CI 检查规则：限制新增裸 `log.Printf` / `console.*`；
3. 面向运营告警的事件白名单（例如 `*.forward_failed`、`*.retry_exhausted*`）。

## 4. 本次整理后可直接使用的检索建议

1. 过滤历史兼容日志：`legacy_stdlog=true`
2. 网关入口故障：`component=handler.* AND level in (WARN,ERROR)`
3. 请求闭环：按 `request_id` + `*.request_completed` + `*.forward_failed`
4. token 刷新故障：`component=*token_refresh* AND (retry_attempt_failed OR set_error_status_failed)`
