## ADDED Requirements
### Requirement: 原子化限流计数与过期
限流中间件 SHALL 在单个原子操作中完成 Redis 计数增量与过期设置，并且仅在首次创建或 TTL 缺失时设置过期，避免刷新已有 TTL；过期时间以毫秒为单位向下取整，最小为 1ms。

#### Scenario: 首次请求创建计数器
- **WHEN** 第一次请求命中该限流 key
- **THEN** 计数增量为 1 且 key 过期时间设置为窗口值

#### Scenario: 窗口小于 1ms
- **WHEN** 限流窗口小于 1ms
- **THEN** 过期时间按 1ms 设置

#### Scenario: 窗口包含非整数毫秒
- **WHEN** 限流窗口包含非整数毫秒
- **THEN** 过期时间按毫秒向下取整

#### Scenario: 已有 TTL 的计数器继续计数
- **WHEN** 计数器已存在且 TTL 正常
- **THEN** 计数递增且 TTL 不被刷新

#### Scenario: 计数器缺失 TTL
- **WHEN** 计数器存在但 TTL 为 -1
- **THEN** 系统为该 key 补设窗口过期时间

### Requirement: Redis 故障策略可配置
限流中间件 SHALL 支持为每个限流 key 配置 Redis 故障策略，支持 fail-open 与 fail-close，默认 fail-open，配置由调用方在注册限流时提供。

#### Scenario: fail-open 策略
- **WHEN** 配置为 fail-open 且 Redis 脚本执行返回错误或连接不可用
- **THEN** 请求继续处理且不执行限流阻断

#### Scenario: fail-close 策略
- **WHEN** 配置为 fail-close 且 Redis 脚本执行返回错误或连接不可用
- **THEN** 请求被限流阻断并返回 429

### Requirement: 优惠码验证接口 fail-close
系统 SHALL 对 `/auth/validate-promo-code` 的限流在 Redis 故障时采用 fail-close。

#### Scenario: 验证优惠码时 Redis 不可用
- **WHEN** 请求 `/auth/validate-promo-code` 且 Redis 不可用
- **THEN** 请求返回 429
