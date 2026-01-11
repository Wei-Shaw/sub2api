## Context
限流中间件当前采用 `INCR` 后 `EXPIRE` 的两步操作，且未处理 `EXPIRE` 失败，导致计数 key 可能没有过期时间。该情况一旦发生，计数会持续累加，触发长期限流并造成 Redis key 膨胀。

## Goals / Non-Goals
- Goals:
  - 原子化 Redis 计数与过期设置
  - 修复 TTL 缺失的历史 key
  - 支持按接口配置 Redis 故障策略（fail-open/fail-close）
  - 为需要强制保护的接口启用 fail-close
- Non-Goals:
  - 改变现有固定窗口限流算法
  - 调整限流 key 格式或前缀
  - 引入新的外部依赖

## Decisions
- 使用 Lua 脚本在 Redis 内部原子执行 `INCR`、`TTL` 与 `PEXPIRE`
- 过期时间统一采用毫秒精度窗口（`window.Milliseconds()` 向下取整）以保持精度一致
- 当毫秒窗口小于 1 时，按 1ms 设置过期，避免 0 导致立即过期
- 当 `count == 1` 或 `TTL == -1` 时设置过期，避免刷新已有 TTL
- 新增 `RateLimitOptions` 并提供 `LimitWithOptions`，由调用方显式配置故障策略
- `Limit` 默认使用 fail-open 以保持兼容
- 当 fail-close 生效时，Redis 执行失败直接返回 429

## Alternatives considered
- 使用 `MULTI/EXEC` 事务封装 `INCR` + `EXPIRE`：原子性可保证，但无法在同一事务内便捷修复 `TTL == -1`，且仍需额外判断逻辑
- 使用 `SET` + `EX`/`NX` 组合：无法保留计数累加语义

## Risks / Trade-offs
- Lua 脚本会带来轻微 CPU 开销，但可接受
- TTL 修复会在首次访问时设定过期，可能缩短历史脏 key 的“无限期”状态，这是期望的修复效果

## Migration Plan
- 上线后脚本在请求路径上自动修复 TTL 缺失的 key
- 如需回滚，恢复原有两步命令即可

## Open Questions
- 无
