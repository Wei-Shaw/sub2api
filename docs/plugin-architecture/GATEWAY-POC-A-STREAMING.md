# Gateway Extraction · POC-A · 流式转发 PoC 实验设计

> 状态：实验设计稿，未实施。
> 受众：实施 PoC 的 implementer（看完即可开工）。
> 关联：`docs/plugin-architecture/GATEWAY-EXTRACTION-PROPOSAL.md` §10 Q2

---

## 1. 目的与可接受阈值

回答 §10 Q2：把 SSE 流从"owner plugin 本地 forward"改成"经 host GatewayMediator 跨 gRPC bidi stream 中转到另一 provider plugin"，新增的 P99 端到端延迟是否 ≤ **2 ms**？

**为什么是 2 ms：**

1. 现状网关已经把"上游 latency / response latency / TTFB"分别埋点（参考 `backend/internal/handler/openai_gateway_handler.go:323-342` 的 `OpsRoutingLatencyMsKey` / `OpsUpstreamLatencyMsKey` / `OpsTimeToFirstTokenMsKey` / `OpsResponseLatencyMsKey`）。生产上 OpenAI ChatCompletions 流式 TTFB 中位数在 300-800 ms 量级（上游决定），P99 通常 1.5-3 s。**2 ms 相对 TTFB P99 ≤ 0.15%**，不会被任何客户端感知。
2. host 内 goroutine + channel 转发一帧 SSE 字节的开销实测在 5-20 μs；走一次 loopback gRPC bidi stream（含 protobuf 编码 / 解码 / HTTP/2 frame）的额外开销在历史 benchmark（gRPC 官方 `benchmark` 子模块、`grpc-go` 1KB payload loopback）大约 80-200 μs / 单向。一来一回大约 0.16-0.4 ms，给 5x 余量取 2 ms。
3. 阈值要"够紧"才能逼出实现问题（错误的同步 Recv / 多余的 buffer copy / 阻塞调度），但不能紧到与抖动同量级。OpenAI 流式 chunk-to-chunk 间隔通常 30-80 ms（模型生成节奏决定），2 ms 远低于该粒度，不会掩盖 chunk 排队抖动。

**合格定义**：在 §3.4 的测试矩阵下，跨进程模式相对直连 baseline，**TTFB P99 增量 ≤ 2 ms** 且 **chunk-to-chunk 间隔 P99 增量 ≤ 1 ms**，同时 host + plugin 进程总内存增量 ≤ 50 MB / 100 并发流。任一不满足即视为超标，按 §5 走回退方案。

---

## 2. 测试指标清单

| # | 指标 | 测量点 | 合格阈值 | 超标定义 |
|---|---|---|---|---|
| M1 | TTFB（首帧延迟） | client 发出请求到收到第一个 SSE `data:` 帧 | 跨进程 P99 - 直连 P99 ≤ 2 ms | 任一并发档 > 2 ms |
| M2 | chunk-to-chunk 间隔 | 相邻两 SSE 帧到达 client 的时间差 | 跨进程 P99 - 直连 P99 ≤ 1 ms | > 1 ms 或抖动方差翻倍 |
| M3 | 端到端 latency | client 发出到收到 `data: [DONE]` | 跨进程 P99 - 直连 P99 ≤ 5 ms（含 N 帧累计） | > 5 ms |
| M4 | 持续吞吐 | 单连接 frames/s（mock 上游固定 1000 帧 @ 50 帧/s） | 跨进程 ≥ 直连 × 0.98 | 跌 > 2% |
| M5 | host 进程 RSS 增量 | runtime.MemStats.HeapInuse + sys，100 并发流稳态 | ≤ 30 MB | > 30 MB |
| M6 | plugin 进程 RSS 增量 | 同上 | ≤ 20 MB | > 20 MB |
| M7 | host 进程 goroutine 数 | runtime.NumGoroutine()，100 并发稳态 - idle | ≤ 4 × 并发数（每流 ≤ 4 goroutine） | > 4× |
| M8 | gRPC stream 异常率 | 跨进程模式下 stream 提前 EOF / Cancel 占比 | ≤ 0.1%（baseline 同等） | > 0.1% |

**只统计 warm-up 后的稳态数据**：每档先打 30 s 流量预热，再采集 60 s。

---

## 3. 实验路径设计

选 **OpenAI ChatCompletions stream**（`POST /v1/chat/completions` + `stream:true`）作为唯一被测协议：

- 已在 `openai_gateway_service.go` 独立成一套（`Forward` 入口 `backend/internal/service/openai_gateway_service.go:2080`），不耦合 Anthropic / Antigravity god object，PoC 可不接 GatewayService 主流程。
- SSE 协议比 Anthropic event-stream 简单（单一 `data:` 帧 + `[DONE]`），mock 上游 50 行内可写完。
- WebSocket 路径（OpenAI Realtime）流量与延迟特性差异大，**不放入 PoC**；待 OpenAI 抽离插件实施期单独评估。理由：bidi WS over gRPC bidi 的封装已在 `openai_ws_forwarder.go` 验证可行，但 PoC 只需回答"流式转发"这一普适问题，SSE 数据足够外推，不必双重投入。

### 3.1 mock 上游

写一个最简 SSE server（独立 binary `cmd/poc-mock-openai`），监听 127.0.0.1:18080：

```go
// 收到 POST /v1/chat/completions 后:
//   Content-Type: text/event-stream
//   每隔 frameInterval（默认 50 ms） 发一帧
//   data: {"choices":[{"delta":{"content":"<seq>"}}]}\n\n
//   总共 frameCount（默认 100） 帧，最后发 data: [DONE]\n\n
//   query 参数 ?frames=N&interval=Nms 覆盖默认值
```

为何用 mock 不用现网账号：(1) 排除上游网络抖动（单纯测 host/plugin 转发开销）；(2) 测试可复现；(3) 不消耗真实 token。**只在 §6 的最后一项做一次现网 smoke**，确认 mock 与真实流量行为不背离。

### 3.2 baseline（直连模式）

模拟"owner == provider，本进程调用"。**新增**测试 binary `cmd/poc-baseline`：

```
client → host:18443 (gRPC server) → 直接调本进程的 forwarder.Forward(account, body) → 把 mock SSE 字节透传给 client
```

forwarder 内部用 `net/http` 客户端 stream 到 mock server 18080，把响应体 chunk 转发给 client。完全不引入 gRPC bidi，是延迟下界。

### 3.3 跨进程模式（mediator）

```
client → host:18443 → host GatewayMediator → gRPC bidi stream 转发到 plugin:18444 →
plugin 内部用 net/http stream mock 上游 → SSE 字节 wrap 到 ForwardChunk → 经 stream 回传到 host →
host 解 wrap 后透传给 client
```

**实现参考**：

- mediator 的 host 侧 stub 对照 `backend/internal/plugin/pricing_extension_client.go` 的 `runWatchOnce`（`pricing_extension_client_watch.go:52`）：单 stream + `Recv()` 循环。差异是 PoC 还要双向送 init message（first send 携带 account / body），后续单向 server-stream 即可（PoC 不测 client→provider 的多帧请求场景）。
- plugin 侧 server 对照 `plugin-sdk/proto/sdk.proto:742`（`WatchPricingOverrides`）的 server-stream 写法。
- 反向代理写 SSE 响应可参考 `backend/internal/plugin/router_middleware.go:302-316` 的 `proxyTo`（虽然那里是 HTTP 反代，PoC 不复用其代码，只参考 hop-by-hop 处理思路）。

PoC 的 proto（仅 PoC 内部，不进 sdk.proto）：

```proto
service PoCGatewayProvider {
  rpc Forward(stream ForwardRequest) returns (stream ForwardChunk);
}
message ForwardRequest {
  oneof payload { ForwardInit init = 1; bytes body_chunk = 2; }
}
message ForwardInit { string model = 1; bool stream = 2; bytes request_body = 3; }
message ForwardChunk { bytes data = 1; bool done = 2; }
```

### 3.4 测试矩阵 / 重复次数 / 并发度

| 维度 | 取值 |
|---|---|
| 模式 | direct / mediator |
| 并发流 | 1 / 10 / 100 |
| 帧总数 / 帧间隔 | 100 帧 @ 50 ms（普通对话） · 1000 帧 @ 10 ms（高频 token） |
| 帧负载 | 200 B（典型 delta） · 4 KB（含 tool_call 大块） |

笛卡尔积 = 2 × 3 × 2 × 2 = **24 组**。每组 warm-up 30 s + 采集 60 s + 重复 3 次（取 3 次 P99 的中位数，消除单次抖动）。

客户端：自写 Go client（`cmd/poc-client`），用 `golang.org/x/sync/errgroup` 起 N 个并发 goroutine 各自 POST 一个 stream，记录每帧 `time.Now()` 时间戳，结束后输出 ndjson 到磁盘。理由：curl 不能精确记录 per-frame 时间戳；Python httpx 引入额外语言开销；hey/wrk 不支持 SSE per-frame 测量。

数据处理：单独的 `cmd/poc-analyze` 读 ndjson 算 P50/P95/P99，输出 markdown 表。

---

## 4. PoC 代码骨架

**分支**：`feat/plugin-poc-streaming`，从 `feat/plugin-system-fixes--upstream-sync-115-121` 切。

**目录**：

```
backend/cmd/poc-mock-openai/main.go        // §3.1 mock 上游
backend/cmd/poc-baseline/main.go           // §3.2 直连 host
backend/cmd/poc-host/main.go               // §3.3 host with mediator
backend/cmd/poc-plugin/main.go             // §3.3 plugin server
backend/cmd/poc-client/main.go             // 并发 client + ndjson 采样
backend/cmd/poc-analyze/main.go            // ndjson → P50/P95/P99 markdown
backend/internal/poc/streaming/
    proto/poc.proto                        // §3.3 内部 proto
    proto/poc.pb.go (生成)
    proto/poc_grpc.pb.go (生成)
    mediator.go                            // host 侧 stream 转发
    provider_server.go                     // plugin 侧 stream 实现
    forwarder.go                           // 复用的 mock-上游 HTTP client
    metrics.go                             // 内存 / goroutine 采样
docs/plugin-architecture/poc-a-results.md  // 实验结果（implementer 填）
```

**采集工具**：

- 延迟：自写 client 记 per-frame timestamp（不用 prometheus，PoC 不需要可视化，ndjson 足够）。
- 内存 / goroutine：每 5 s 采 `runtime.ReadMemStats` + `runtime.NumGoroutine()` 输出 csv。
- 不上 prometheus / wrk / hey：PoC 一次性脚本，避免引入额外构建依赖。

**mediator 实现要点**（参考方案，implementer 可调整）：

1. host 收到 client HTTP 请求 → 在 GatewayMediator 内 dial plugin gRPC（连接复用，进程生命期单连接 + multiplex）→ 开 bidi stream → 发 ForwardInit → 起 1 个 goroutine 循环 `stream.Recv()` 把 ForwardChunk.data 直接 `ResponseWriter.Write` + flush。
2. **不要在 mediator 解析 SSE 帧边界**：plugin 端把上游 HTTP body chunk 原样塞进 `ForwardChunk.data` 即可（zero-copy 思路）；host 端透传到 ResponseWriter，client 自己按 SSE 协议 parse。这是阈值能否达标的关键设计。
3. plugin 端用 `http.Client` 的 `resp.Body` 流式 `Read` 到固定 buffer（推荐 4 KB），每读一段 `stream.Send(&ForwardChunk{Data: buf[:n]})`。

---

## 5. 失败回退方案

若 §3.4 任一并发档 P99 增量超 2 ms，按代价从低到高评估：

### 方案 A：透明字节流（不解码 protobuf payload）

mediator 不走 protobuf message，host 与 plugin 之间用 HTTP/2 raw stream（或 gRPC `[]byte` payload + 极简单 framing）做 `io.Copy`。

**简评**：省一次 protobuf 编解码（每帧约 30-80 μs）。改动量小（PoC 内已用 `bytes` payload，已经接近这种方案；进一步可换成裸 HTTP/2 server）。**决策依据**：若 P99 超标在 0.5-1.5 ms 量级，先试此方案；超标 > 3 ms 则跳到方案 B。

### 方案 B：plugin 监听本地 HTTP，host 用现有反向代理（绕过 gRPC）

复用 `backend/internal/plugin/router_middleware.go:302` 的 `httputil.ReverseProxy` 模式。plugin 把 GatewayProvider 暴露成 HTTP 端点（`POST /__provider/forward`），host 直接反代过去。SSE 透传是 ReverseProxy 原生能力，零新代码。

**简评**：完全规避 gRPC stream。代价：失去 protobuf 类型安全 + bidi 双向能力（PoC 测的 SSE 是单向 server-stream，问题不大；但 OpenAI Realtime WebSocket / Anthropic 多轮 stream 上行需要 bidi 时此方案不够）。**决策依据**：若 SSE PoC 用方案 B 能达标且 OpenAI Realtime 后续可单独走"ws over host 反代"通路，整体可接受。

### 方案 C：抽离计划缩水到 owner-only（provider 不抽）

provider 永远 == owner（即 `gateway-anthropic` 也实现 antigravity provider），跨 plugin 转发场景不存在，host mediator 不需要。

**简评**：彻底回避问题，但牺牲 §1.3 设计目标（"禁用 antigravity plugin 自动从混调池摘除"等运维收益消失），且每个 owner plugin 都得带全部 platform 的 forward 实现，god object 没真的拆开。**决策依据**：仅当方案 A、B 都失败（P99 增量 > 5 ms）才考虑；本质上是放弃整体抽离方案，需重写 §3、§4。

**决策树**：
- P99 增量 ≤ 2 ms：直接进入阶段二实施。
- 2-3 ms：试方案 A。
- 3-5 ms：试方案 B。
- > 5 ms：上方案 C，重审提案。

---

## 6. 实验执行清单（implementer 按此 checklist 跑）

- [ ] step 1：从 `feat/plugin-system-fixes--upstream-sync-115-121` 切 `feat/plugin-poc-streaming`
- [ ] step 2：实现 `cmd/poc-mock-openai`，单元自测能打 100 帧 @ 50 ms（curl 验）
- [ ] step 3：实现 `cmd/poc-baseline` + `cmd/poc-client`，跑一次 1 并发 / 100 帧，确认 ndjson 输出格式
- [ ] step 4：实现 `internal/poc/streaming/proto`，`buf generate` 出 pb 代码
- [ ] step 5：实现 `cmd/poc-host` + `cmd/poc-plugin`（mediator + provider server），1 并发 smoke 通过
- [ ] step 6：实现 `cmd/poc-analyze`，输出表格格式与 §3.4 矩阵对齐
- [ ] step 7：跑完 24 组 × 3 次重复，原始 ndjson + 内存 csv 落盘到 `tmp/poc-a/`
- [ ] step 8：分析输出 `docs/plugin-architecture/poc-a-results.md`：每组 P50/P95/P99 / TTFB / chunk-gap / 内存 / goroutine
- [ ] step 9：判定 §1 阈值是否达标；若超标按 §5 决策树回退并重测
- [ ] step 10：在 results 文档末尾给出"是否进入阶段二"的明确建议（达标/降级方案/缩水）
- [ ] step 11（可选 smoke）：拿一个 sandbox 现网 OpenAI key 跑 5 次跨进程 stream，确认 TTFB 与 mock 数据无量级差异；只为兜底现网偏差，不参与判定
- [ ] step 12：把 PoC 代码打 `poc/streaming-v1` tag 后保留分支不合 main（PoC 代码不入主干）

---

## 附录 A：边缘观察

调研中注意到的现状代码点，**不在 PoC 范围内修复**，仅记录：

- `backend/internal/service/openai_gateway_service.go:335-339` 的 `responseLatencyMs = forwardDurationMs - upstreamLatencyMs` 计算在 `upstreamLatencyMs == 0`（流式中途异常时未写入）情况下退化为 forwardDurationMs，会污染 `OpsResponseLatencyMsKey`。PoC 不依赖此字段，但若后续把这些 metric 作为生产 baseline 比较对象，需先理清异常路径的语义。
- `pricing_extension_client.go:135` 的 `Start` 把 `loopCtx` 从 `parentCtx` 派生而非 Start 入参 ctx——PoC mediator 需借鉴此模式（请求 ctx 不能控制后台 stream loop 生命期），否则 client 断连时整个 stream 立即崩。
