# Redis 内存与高并发配置

Sub2API 默认使用适合小机器的 Redis 配置。Redis 只保存缓存、限流状态和可重建的待计费叠加值；计费任务先提交到 PostgreSQL WAL，PostgreSQL 才是计费事实来源。因此 Redis 可以关闭 RDB/AOF，并在达到内存上限时使用 `allkeys-lru`，Redis 重启或键被淘汰不会丢失已接收的计费任务。

## 默认档：普通和小内存机器

默认 Compose 配置为：

```dotenv
REDIS_MEM_LIMIT=2g
REDIS_MAXMEMORY=1536mb
REDIS_MAXCLIENTS=50000
REDIS_POOL_SIZE=1024
REDIS_MIN_IDLE_CONNS=10
```

`REDIS_MEM_LIMIT` 是容器总内存限制，`REDIS_MAXMEMORY` 只限制 Redis 键空间。两者不能设成相同值；默认保留约 512MB 给客户端连接缓冲区、复制/命令临时内存、分配器碎片和 Redis 自身开销，避免容器在 Redis 开始淘汰键之前被 OOM Kill。

`REDIS_MAXCLIENTS=50000` 只是连接数上限，不会预先创建 50000 个连接。实际常驻连接主要由应用连接池控制，所以普通机器不应为了这个上限把 `REDIS_POOL_SIZE` 一并放大。

## 高并发档：50k+ RPM

当实际压测显示 Redis 键空间、连接数或淘汰率接近默认档上限时，可在 `.env` 中覆盖为：

```dotenv
REDIS_MEM_LIMIT=12g
REDIS_MAXMEMORY=10gb
REDIS_MAXCLIENTS=50000
REDIS_POOL_SIZE=4096
REDIS_MIN_IDLE_CONNS=256
```

这保留了 2GB 容器余量，适合较大的缓存、更多并发连接和 `50k+ RPM` 部署。`50k RPM` 不等于 50000 个同时连接；如果请求很快或连接复用率高，默认连接池可能已经足够。先观察连接池等待和 Redis 指标，再逐步增加池大小，避免过多连接造成额外内存占用和上下文切换。

Compose 已固定以下稳定性参数，无需在 `.env` 重复配置：

```text
--save ""
--appendonly no
--maxmemory-policy allkeys-lru
--tcp-backlog 65535
nofile soft/hard 100000
net.core.somaxconn 65535
```

Redis 密码继续由 `REDIS_PASSWORD` 注入；健康检查通过 `REDISCLI_AUTH` 鉴权，不会把密码放到 `redis-cli -a` 的进程参数中。

## 调整原则

1. 保证 `REDIS_MAXMEMORY < REDIS_MEM_LIMIT`。高连接数或大响应场景应预留更多容器余量。
2. 多个 Sub2API 实例共用 Redis 时，连接池总量约为 `实例数 * REDIS_POOL_SIZE`，必须低于 `REDIS_MAXCLIENTS` 并保留运维余量。
3. 只有出现连接池等待时才增加 `REDIS_POOL_SIZE`；只有冷启动或突发流量的建连延迟明显时才增加 `REDIS_MIN_IDLE_CONNS`。
4. 如果宿主机总内存不足以覆盖 Redis、PostgreSQL、Sub2API 和系统页缓存，应继续下调 Redis，而不是依赖 swap 承受长期压力。

## 监控与检查

查看生效配置和内存状态：

```bash
docker compose exec redis redis-cli CONFIG GET maxmemory
docker compose exec redis redis-cli CONFIG GET maxmemory-policy
docker compose exec redis redis-cli INFO memory
docker compose exec redis redis-cli INFO clients
docker compose exec redis redis-cli INFO stats
```

配置密码后，Compose 会通过容器内的 `REDISCLI_AUTH` 自动鉴权。重点关注：

| 指标 | 含义 | 建议 |
|------|------|------|
| `used_memory` / `maxmemory` | 键空间内存压力 | 长期接近上限时增加内存或接受缓存淘汰 |
| `evicted_keys` | 因 `allkeys-lru` 淘汰的键数 | 持续快速增长时检查缓存容量和命中率 |
| `connected_clients` | 当前连接数 | 接近 `maxclients` 时检查各实例连接池总量 |
| `rejected_connections` | 被拒绝的连接累计数 | 非零增长通常表示连接上限或突发建连问题 |
| `mem_fragmentation_ratio` | 内存碎片比例 | 结合容器 RSS 判断，不能只看单一比值 |

修改 `.env` 后重建 Redis 容器使限制生效：

```bash
docker compose up -d --force-recreate redis
docker compose up -d sub2api
```

Redis 是易失的可重建层，重建容器不需要迁移 Redis 数据；不要给 Redis 添加持久化卷作为计费可靠性的替代方案。计费完整性由 PostgreSQL 中的 durable billing jobs 和幂等入账保证。
