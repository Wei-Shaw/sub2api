## 1. Implementation
- [x] 1.1 在限流中间件中引入 Lua 脚本原子化计数与过期设置（使用 PEXPIRE 毫秒窗口）
- [x] 1.2 脚本内检测 `TTL == -1` 时补设过期，修复历史脏 key
- [x] 1.3 引入 `RateLimitOptions` 与 `LimitWithOptions`，`Limit` 保持默认 fail-open
- [x] 1.4 为 `/auth/validate-promo-code` 配置 fail-close 策略
- [x] 1.5 添加测试覆盖首次请求、已有 TTL、TTL 缺失、非整数毫秒窗口与故障策略（使用 Redis 集成测试/testcontainers 方案）
