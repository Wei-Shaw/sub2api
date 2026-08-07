# 渠道监控 V2 设计

完整设计稿见 [`docs/channel-monitor-v2-design.md`](docs/channel-monitor-v2-design.md)（若仓库未跟踪 `docs/*`，以本文件 + 代码为准）。

## 核心结论

系统设置 `channel_monitor_enabled` + `channel_monitor_mode`（`v1`|`v2`）互斥选择实现。mode=`v2` 时 V1 scheduler/手动探测不产生上游流量；mode=`v1` 时 V2 分钟聚合不运行。V2 只消费 `usage_logs` 与最终用户错误 `ops_error_logs`，绝不生成上游请求；上游尝试从 `upstream_errors` 单独统计且只返回管理员。错误按 `request_id` 去重，并保留 `cyber_policy` 的 HTTP 200 流式失败语义。

## 实现要点（与代码对齐）

| 项 | 当前实现 |
|----|----------|
| 固定 rollup | 300s / 3600s / 43200s / 86400s（5m / 1h / 12h / 1d） |
| 展示 bucket | 90m→5m，24h→1h，7d→12h，30d→24h |
| 默认 refresh | 300 秒（migration 201；仍可配置 60） |
| 默认 mode 升级 | migration 195 写入 `channel_monitor_mode=v2`（**现网 V1 舰队需在 release note 中说明**） |
| API | snapshot / models / matrix / errors / users / dimensions + admin config |
| 隐私 | 非 admin：绝对量清零；`/errors` **不含 details**；snapshot config 去掉 group_ids / model lists / ignored categories；他人身份匿名 |
| 忽略错误类 | 只下调 `error_rate`（健康分）；`success_rate` 仍为真实 success/request |

## 分层保留（storage vs 有效性）

聚合每次 `RecomputeRange` 结束按 **表 + rollup 粒度** 裁剪，不再统一 35 天：

| 数据 | 保留 | 对齐 |
|------|------|------|
| `user_metrics_1m` | **3 天** | 用户维最重；榜单主要靠 user rollup |
| `metrics_1m` / `error_1m` / `hist_1m` | **7 天** | 迟到写入重叠重算 + 近期 rollup 重建 |
| rollup 300s (5m) | **7 天** | 90m 视图 + 近周细趋势 |
| rollup 3600s (1h) | **30 天** | 24h 视图 |
| rollup 43200s (12h) | **45 天** | 7d 视图 |
| rollup 86400s (1d) | **90 天** | 30d 视图 + 略长审计余量 |
| backfill / watermark 上界 | **90 天** | 与最长 1d rollup 一致 |

写入：周期重算最近 ~10 分钟；幂等 delete 窗口 + 再 insert；回填可短时写入 1m 再建 rollup，随后 1m 按 TTL 丢弃。

默认信息层级：概览 KPI → 错误率/TTFT 趋势（矩阵或折线）→ 模型/错误原因/用户排行。空筛选表示全部；模型名单非空时名单外归 `__other__`；健康色由后端阈值计算，样本不足为 unknown。
