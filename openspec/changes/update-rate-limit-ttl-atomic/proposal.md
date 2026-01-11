# Change: 原子化 Redis 限流 TTL 设置

## Why
当前限流逻辑使用 `INCR` 后再 `EXPIRE`，非原子且未处理 `EXPIRE` 失败，会导致 key 可能永久存在，引发长期限流或 Redis 内存增长。

## What Changes
- 使用 Lua 脚本原子化 `INCR` + 过期设置
- 当检测到 TTL 缺失时补设过期，修复历史脏数据
- 支持 Redis 故障策略配置（默认放行，特定接口可 fail-close）
- 新增 `limit-requests` capability，用于描述限流行为与故障策略

## Impact
- Affected specs: 新增 `specs/limit-requests/spec.md`
- Affected code: `backend/internal/middleware/rate_limiter.go`
