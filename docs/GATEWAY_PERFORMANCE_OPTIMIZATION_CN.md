# 网关性能优化与 Rust 评估

本文面向维护者，记录大 JSON、WebSocket 和协议转换的优化边界及 Docker 验证方法。起始版本为 `af1bcc3fce7aafcee6a908a407977e46455e52e3`。单次 helper benchmark 不能代表线上端到端吞吐，实际收益取决于请求大小、协议、并发和上游等待时间。

## 优化建议与实施顺序

| 优先级 | 区域 | 当前决定 |
| --- | --- | --- |
| P0 | 大 Responses JSON 的完整 map 解码与多次改写 | 保留复杂转换的完整解码；优先减少简单改写的整包复制。Rust SIMD JSON codec 为后续候选，需与优化后的 Go 路径对比。 |
| P0 | WS payload 重复序列化 | 在请求内复用完成 metadata 投影后的握手快照；prewarm 改写后失效。 |
| P1 | Anthropic / Responses / Chat 流式编解码 | 先消除文本、参数、未签名思考的逐片字符串复制，并减少每个 SSE delta 的动态 map 编码。 |
| P2 | 本地 token counting | 当前流式首包采用字节估算；只有完整 tokenizer 的实际 CPU 占比显著时再评估 Rust。 |
| 暂缓 | 鉴权、Top-K 调度、Redis、数据库和可靠计费 | 已有缓存、候选索引及有界算法；数据库/Redis 往返、锁和事务成本不能靠换语言消除。 |

2026-11-03 的探索性测试中，16 MiB fixture 完整解码为 `map[string]any` 约 76 ms、28.5 MB 分配；该数字只描述完整解码 helper，不能推导所有网关请求都会触发该成本。当前轻量字段提取和 raw patch 路径已经避免部分完整解码。

## 本轮实现

1. **合并大请求 set patch。** 对至少 64 KiB、包含多个普通 set 的请求，把涉及的小字段复制到局部 JSON，由既有 sjson 按原顺序应用嵌套改写，再一次性拼回完整请求。未触及的 input、未知扩展字段、大整数、JSON 空白及字符串转义直接保留原始字节。删除、特殊路径、重复根字段、被改写的大子树以及小请求继续使用既有路径。实现位于 [openai_request_set_patches.go](../backend/internal/application/service/openai_request_set_patches.go)，入口仍为 `openAIRequestView.ApplyPatches`。
2. **复用 WS 握手快照。** `forwardOpenAIWSV2` 在 turn metadata 投影后，为 session、Responses Lite 和语义头共享一次编码。早于 metadata 清理的 OAuth 身份读取仍使用当时的 payload；连接预热可能生成新的 `previous_response_id`，因此后续日志缓存必须失效，发送仍编码最终 payload。实现位于 [openai_ws_forwarder_v2.go](../backend/internal/application/service/openai_ws_forwarder_v2.go)。
3. **线性累积流片段。** `AnthropicEventToResponsesState` 的文本、工具参数及未签名思考使用请求内 `strings.Builder`，保留原公开字符串快照。块结束时释放 builder，已生成的 done/terminal 事件继续持有其字符串内容。原有签名 envelope、计费、结束原因及断流结算语义沿用既有 owner。实现位于 [anthropic_to_responses_response.go](../backend/internal/shared/apicompat/anthropic_to_responses_response.go)。
4. **用固定结构编码高频 SSE 事件。** text、reasoning、function arguments 和 custom input 的 delta/done 使用小型 typed wire struct，避免每次分配 map、排序 key 和装箱数字。保持空文本、零 index、可选标识字段和原有 JSON key 顺序。较少发生的 item/part 及未知事件沿用原路径。定义位于 [responses_stream_text_wire.go](../backend/internal/shared/apicompat/responses_stream_text_wire.go)。

这些改动没有数据库迁移、配置开关或新运行时依赖。回退可直接回退代码；共享缓存与持久化协议未变化。

## Docker 证据

对照固定为 Linux arm64、`golang:1.26.10-alpine`、`CGO_ENABLED=0`、2 CPU、3 GiB 内存、`GOMAXPROCS=2`。源码只读挂载，容器禁网，使用预热后的 Go module/build 缓存。每项运行 300 ms、重复 3 次，汇总取中位数。时间为 helper/pipeline 的局部结果，B/op 是累计分配量，不是 RSS。

原始日志、fixture 边界和复跑命令见 [Docker 性能证据](../diagnostics/gateway-performance-20261104/README.md)。其中 WS fixture 使用内存中的上游连接，测量真实 forward/header 准备流程；不包含网络等待及真实连接发送时的 JSON 编码。大 JSON raw patch 与完整 map 解码是不同操作，不能将其耗时差直接称为同一功能的加速比。

同一容器约束下的中位数对照如下（MB 为十进制累计分配量）：

| 场景 | 耗时：优化前 → 后 | 分配量：优化前 → 后 |
| --- | --- | --- |
| 16 MiB 请求的两项 raw set patch | 11.13 → 7.60 ms（减少 31.7%） | 33.57 → 16.79 MB |
| 256 KiB WS 请求准备 | 1.17 → 0.70 ms（减少 40.5%） | 1.22 → 0.33 MB |
| 256 KiB 分片文本累积 | 5.15 → 0.14 ms（减少 97.3%） | 34.57 → 1.24 MB |
| 256 KiB 分片工具参数累积 | 5.30 → 0.18 ms（减少 96.7%） | 34.57 → 1.24 MB |
| 256 KiB 分片未签名思考累积 | 5.74 → 0.18 ms（减少 96.8%） | 34.57 → 1.24 MB |
| 单个 output_text.delta 编码 | 1414 → 796 ns（减少 43.7%） | 1360 → 624 B；21 → 4 次分配 |

多项 raw set 的小对象管理使总分配次数从 9 增至 24，但 4–32 MiB fixture 的分配字节减少约一半、耗时减少约 32–37%；该路径仅用于大请求。完整 map 解码仍保留，其原有成本没有被本轮改写器消除。基准只证明上述具体场景的收益，不代表线上整体加速倍数。

验证完成：Docker 全量 `go test -tags=unit ./... -count=1` 通过（73 个有测试的包），JSON 批量改写与原有 sjson 的对照模糊测试通过（20 秒、35,003 次执行），官方 Go MCP SDK OAuth 互操作检查和 `CGO_ENABLED=0` Linux arm64 编译通过。针对性测试覆盖空字符串、零 index、Unicode/无效 UTF-8 编码、分片快照、元数据、流式/非流式用量及不修改 failover 原始请求体。

本轮没有修改前端。仓库既有门禁仍有三处问题：`docs/README.md` 引用缺失的 PR169 文档，`payment_fulfillment_test.go` 有 1201 行，`pricing_service.go` 有 1218 行；`make check-docs` / `make check-layout` 因此失败。新增/更新的专题文档链接、Go 格式及 diff 空白检查通过。

## 继续引入 Rust 的条件

优先试验边界清晰的 JSON/SSE codec，而不是迁移 HTTP/TLS、计费和账号调度。Go 当前发布链路使用 `CGO_ENABLED=0`；FFI 会增加构建/交叉编译维护成本，sidecar 会增加进程通信和故障处理成本。Rust 原型需计入输入/输出复制及 FFI/IPC 开销，并验证 HTTP/WS、流式/非流式、工具续链、签名思考、未知字段、整数精度、错误及断流用量。

只有在优化后的 Go 基线仍占据显著 CPU、且 Rust 原型在代表性负载下改善整体 CPU 或尾延迟时再合入。当前证据不包含 Rust 实现的对照，不能承诺换语言后的倍数收益。

## 第二轮调查

合并 PR #177 后的下一批候选、实施改动与新 Docker profile 见 [第二轮性能调查](../diagnostics/gateway-performance-round2-20261104/REPORT.md)。诊断快照准入、Chat→Responses 工具参数线性累积、图片计数及 WS 校验/解析复用已落地；完整 tokenizer 仍作为有条件的 Rust 原型候选。
