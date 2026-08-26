# 高可用分支正式切换

本文适用于通过官方 `docker-deploy.sh` 一键准备的 Docker 部署。默认正式目录为 `/opt/sub2api`，其中包含：

```text
/opt/sub2api/docker-compose.local.yml
/opt/sub2api/.env
/opt/sub2api/data/
/opt/sub2api/postgres_data/
/opt/sub2api/redis_data/
```

## 1. 选择方案

| 场景 | 推荐方案 | 特点 |
| --- | --- | --- |
| 用户量小、低峰期可接受短暂波动 | 方案 A：完整副本验证 | 复制全部持久化目录，恢复旧版本后用副本验证 |
| 用户量大、线上持续有写入 | 方案 B：原数据快速切换 | 提前构建镜像，最终只短暂停应用容器 |

副本创建后线上产生的新用户、账号、API Key、分组和用量记录不会自动同步。因此副本只用于验证，不能直接作为最终正式数据源。

**重要顺序：代码拉取、`docker build` 和正式 compose 覆盖文件必须在停机前完成。** `docker build` 可能耗时数分钟，不要把它放在正式容器停止之后执行。

## 2. 切换前确认

```bash
cd /opt/sub2api
docker compose --env-file .env -p sub2api -f docker-compose.local.yml ps
cat postgres_data/PG_VERSION
```

必须确认当前 compose 使用的是本地目录挂载，并且 PostgreSQL 镜像大版本与 `PG_VERSION` 一致。不要重新运行官方准备脚本，它可能覆盖 `docker-compose.local.yml` 和 `.env`。

## 3. 提前准备代码和镜像（不停机）

使用独立代码目录，不要把 Git 仓库放进正式数据目录：

```bash
cd /opt
git clone -b feature/ha-scheduler https://github.com/bianchen-xiaobai/sub2api.git sub2api-ha-src
cd /opt/sub2api-ha-src
git fetch origin
git reset --hard origin/feature/ha-scheduler
docker build -t sub2api:ha-scheduler .
```

后续更新已有代码目录时，重复 `git fetch`、`git reset` 和 `docker build`。确认镜像构建成功后再进入停机或复制步骤。

创建 `/opt/sub2api/docker-compose.ha-prod.yml`：

```yaml
services:
  sub2api:
    image: sub2api:ha-scheduler
```

正式启动只使用 `docker-compose.local.yml` 加这个覆盖文件。不要使用 `docker-compose.ha-test.yml`，它会替换容器名称、端口和持久化挂载目录。

## 4. 两种备份与切换方式

### 方案 A：完整副本验证（低用户量）

适合可以接受短暂波动的低峰期。复制 PostgreSQL 原始目录前必须停止 PostgreSQL：

```bash
BACKUP=/opt/sub2api-ha-data/$(date +%Y%m%d-%H%M%S)
mkdir -p "$BACKUP"

docker compose --env-file .env -p sub2api -f docker-compose.local.yml stop sub2api postgres redis

cp -a .env data postgres_data redis_data "$BACKUP/"

# 尽快恢复旧版本，减少线上波动
docker compose --env-file .env -p sub2api -f docker-compose.local.yml up -d
```

然后修改 `$BACKUP/data/config.yaml`，使用副本进行验证。副本验证期间，正式目录仍会产生新数据，因此验证完成后不能直接把副本替换回正式目录。

验证时使用临时 compose 覆盖文件，将应用、PostgreSQL、Redis 的容器名称和端口改为测试值，并把三个挂载目录指向 `$BACKUP`；使用独立项目名启动。不要把副本挂载到正式项目，也不要使用 `docker-compose.ha-test.yml` 代替正式配置。

同时建议再制作一份恢复备份：

```bash
STAMP=$(date +%Y%m%d-%H%M%S)
mkdir -p /opt/sub2api-backups/$STAMP
tar -czf /opt/sub2api-backups/$STAMP/sub2api-data.tar.gz \
  .env data postgres_data redis_data
```

复制时间取决于目录大小，不保证一定是几秒。最终切换仍使用正式目录，按照第 6 节执行；后端验证通过后再按第 8 节配置前端。

### 方案 B：原数据快速切换（高用户量）

不复制原始 PostgreSQL/Redis 目录作为测试数据。提前完成代码构建、镜像构建、正式配置修改和配置备份，最终只短暂停应用容器：

```bash
# 提前备份，具体可使用 pg_dump 和 Redis 持久化备份
mkdir -p /opt/sub2api-backups/$(date +%Y%m%d-%H%M%S)

# 低峰期切换时只停止应用，不停止数据库和 Redis
docker compose --env-file .env -p sub2api -f docker-compose.local.yml stop sub2api
```

上面的 `stop` 前先完成第 5 节和第 6 节的配置修改。停机后直接按第 7 节启动高可用镜像，不要在停机窗口内执行构建或编辑配置。这样新容器直接使用最新的正式数据，不会丢失切换期间的用户和账号变更。

两种方案都禁止执行 `docker compose down -v`，否则可能删除持久化卷。

## 5. 配置高可用策略

编辑正式持久化配置：

```text
/opt/sub2api/data/config.yaml
```

在现有 `gateway:` 节点下合并：

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
```

不要用示例配置覆盖现有 `config.yaml`。正式 compose 使用 `BIND_HOST`，不要添加 `HA_BIND_HOST`、`HA_SERVER_PORT` 等测试环境变量。分组的 `selection_mode: strict_health` 保存在数据库中，继续使用后台分组配置。

## 6. 启动新版本（最终使用正式数据）

```bash
cd /opt/sub2api
docker compose \
  --env-file .env \
  -p sub2api \
  -f docker-compose.local.yml \
  -f docker-compose.ha-prod.yml \
  up -d
```

启动时会自动执行尚未应用的数据库迁移。**无论采用哪个方案，最终都必须使用 `/opt/sub2api/data`、`/opt/sub2api/postgres_data`、`/opt/sub2api/redis_data` 这三个正式目录；不能使用副本目录。** 现有用户、账号、分组、API Key 和 Redis 数据继续保留。

## 7. 验证

```bash
docker compose --env-file .env -p sub2api \
  -f docker-compose.local.yml -f docker-compose.ha-prod.yml ps

docker compose --env-file .env -p sub2api \
  -f docker-compose.local.yml -f docker-compose.ha-prod.yml logs --tail=200 sub2api

curl http://127.0.0.1:8080/health
```

重点检查：

- 容器为 `running`，健康检查通过
- 日志无数据库迁移或配置解析错误
- 分组已设置 `high_availability` 和 `strict_health`
- 调度日志中的 `health_tier`、`health_reason` 正常出现
- 主动探测连续失败账号不会再次成为首选

如需观察候选调度日志，可临时将 `data/config.yaml` 的 `log.level` 设为 `debug`，验证后恢复为 `info`。

## 8. 前端配置高可用策略

确认后端容器已成功启动、`/health` 正常后，再在管理后台配置。前端配置保存在数据库中，不要在后端尚未启动时提前操作。

### 8.1 设置：OpenAI 实验调度策略

进入 **管理后台 → 设置 → OpenAI 实验调度策略**：

1. 打开 **OpenAI 实验调度策略**。
2. **粘性加权**按需要开启；开启后 `session_hash` 和 `previous_response_id` 会参与高级调度评分。
3. **订阅优先**按业务需要选择，不影响健康分层。
4. 调度权值建议先保持默认值。

后端 `config.yaml` 中的 `gateway.openai_scheduler.strategy: high_availability` 仍应保留；设置页面的开关与该配置共同决定运行时行为。

### 8.2 分组：高可用优先和账号选择模式

进入 **管理后台 → 分组管理**，编辑需要启用高可用的分组：

1. 打开 **高可用优先**。
2. 开启后，在出现的 **账号选择模式** 中选择：
   - **加权探索**：按权值进行探索式选择。
   - **严格健康优先**：先按健康层级，再在同层内综合错误率、首字响应、同步总耗时和负载排序。
3. 保存后重新打开分组编辑窗口，确认显示为 **高可用优先** 和目标账号选择模式。

每个分组独立生效。未开启高可用的分组继续使用旧版调度。

### 8.3 账号：定时测试和健康样本

进入账号管理中账号的 **定时测试** 面板：

1. 为同一分组的账号选择实际使用的模型并创建测试计划。
2. 设置合理的 Cron 频率；频率过高可能触发上游限流。
3. 打开 **启用**。
4. 需要让主动探测参与健康评分时，打开 **纳入健康样本**。该数据只以低权重参与评分，默认关闭（**建议开启，对于低流量无充足样本时可用性较好**）。
5. 在测试结果中确认状态、耗时和错误信息持续更新。

只有探测请求与真实用户请求使用相同模型、上游链路和代理时，才建议纳入健康样本。

### 8.4 渠道监控（可选）

进入 **管理后台 → 渠道管理 → 渠道监控**：

1. 为目标分组或账号配置监控项。
2. 使用与真实请求一致的 `base_url`、模型、请求模式和代理。
3. 设置合理的超时和检测间隔，避免监控请求造成额外限流。

渠道监控失败表示本次探测失败；是否将账号定时测试纳入健康样本由定时测试中的开关控制。

### 8.5 前端配置验证

分组接口应返回：

```json
{"scheduler":{"strategy":"high_availability","selection_mode":"strict_health"}}
```

真实请求日志应出现 `selection_mode`、`health_tier` 和 `health_reason`。连续探测失败账号应进入 `health_tier=2`，即使响应很快也不再作为首选。

## 9. 回滚

停止新容器并恢复旧镜像标签：

```bash
docker compose --env-file .env -p sub2api \
  -f docker-compose.local.yml -f docker-compose.ha-prod.yml stop sub2api

# 将旧版本镜像重新标记为正式镜像后启动
docker tag <旧镜像ID或旧镜像标签> sub2api:ha-scheduler

docker compose --env-file .env -p sub2api \
  -f docker-compose.local.yml -f docker-compose.ha-prod.yml up -d
```

如果发生数据库迁移失败，停止服务后使用 `/opt/sub2api-backups/<时间戳>/` 中的备份恢复。数据库迁移是前向迁移，不能用 Git 回退代替数据库恢复。

单容器重启无法保证正在执行的请求不中断，应选择低流量时段；需要真正无感切换时，应在反向代理后运行蓝绿实例并先排空旧实例。
