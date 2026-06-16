# 请求/响应全量归档（Request/Response Archive）

把每一次网关请求体 + 响应体异步落盘，按天切片、zstd 压缩，便于月末拉到本地长期保存。
**默认关闭**，需显式开启。

## 设计

- 全异步、可丢弃：请求侧只把记录塞进有界队列，绝不阻塞业务热路径；队列按字节+条数双限流，溢出丢弃并周期 WARN。
- 单 writer 协程顺序追加，边写边 zstd 流式压缩，明文从不落盘，CPU 平稳。
- 响应捕获：网关中间件包裹 `c.Writer`，流式/非流式响应一并捕获（不改动任何 streaming 代码）。
- 磁盘水位保护：归档分区剩余空间低于阈值则停写，防写爆 DB 所在分区。

## 隐私（schema 固定）

- **存**：请求体、响应体、request_id、时间、内部 ID（user/api_key/account）、inbound 路径、model、stream、status、duration、usage。
- **客户端 IP**：仅存 `SHA256(salt+IP)`，盐在归档目录 `.ip_salt` 自动生成并持久化（跨重启稳定）。
- **不存**：上游端点/中转域名、任何密钥、原始 IP。
- **Header 白名单（默认拒绝，只存白名单）**：
  - 请求：`anthropic-version`、`anthropic-beta`、`openai-beta`、`content-type`、`idempotency-key`、`version`、`x-app`、`originator`、`user-agent`、`accept-language`、`x-stainless-*`
  - 响应：`request-id`/`x-request-id`/`anthropic-request-id`/`x-amzn-requestid`/`x-goog-request-id`、`retry-after`、`cf-ray`、`cf-mitigated`、`x-codex-*`、`anthropic-ratelimit-unified-*`

> 注：OpenAI Responses 的 WebSocket 路径不归档（连接被 Hijack，无法捕获响应帧）。

## 配置（config.yaml 的 `archive` 段，或等价环境变量）

| 键 | 默认 | 说明 |
|---|---|---|
| `archive.enabled` | `false` | 总开关 |
| `archive.dir` | `<DATA_DIR>/archive` | 归档目录 |
| `archive.max_shard_size_mb` | `512` | 单分片压缩后大小上限，超过切片 |
| `archive.queue_max_items` | `4096` | 队列最大条数 |
| `archive.queue_max_bytes` | `268435456` | 队列内存预算（256MB），超过丢弃 |
| `archive.max_response_bytes` | `16777216` | 单条响应体捕获上限（16MB），超出截断 |
| `archive.compression_level` | `3` | zstd 级别 1-4（1 最快 / 3 默认 / 4 最高压缩比） |
| `archive.flush_interval_ms` | `1500` | 周期 flush 间隔 |
| `archive.min_free_disk_gb` | `10` | 剩余空间低于此值停写（0=不检查） |
| `archive.ip_hash_salt` | `""` | 留空则自动生成持久盐 |

开启示例（环境变量，Docker 友好）：

```
ARCHIVE_ENABLED=true
```

## 目录与文件

```
<dir>/2026/06/16/reqlog-20260616-000.jsonl.zst
```

按天分目录，单文件超阈值再切片（`-000`、`-001` …）。月末按 `2026/06/` 整月选取。

## 月末拉取到本地（外接硬盘）

用仓库内脚本 `tools/archive-pull.sh`（rsync 增量、传完即删源、可限速）：

```bash
# 拉取 2026/06 整月，限速 3MB/s（与业务共享带宽时建议）
tools/archive-pull.sh root@vps /app/data/archive /Volumes/HDD/sub2api-archive 2026/06 3000
```

## 本地查看（无需专用工具）

```bash
brew install zstd jq                                   # 一次性安装
zstdcat 2026-06-16-000.jsonl.zst | jq                  # 边解压边美化浏览
zstd -d  2026-06-16-000.jsonl.zst                      # 解压成明文 .jsonl
zstdcat 2026/06/*/*.jsonl.zst | jq 'select(.request_id=="req_abc")'
```
