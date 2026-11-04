# 第二轮优化实施与验证

本目录记录 [调查报告](../REPORT.md) 中前四项 Go 优化的实施结果。基线为 `68c96552b040b943f3a04484c798efd30110f2ed`，原始基准位于上级目录。本轮保持 Go 1.26.10 / `CGO_ENABLED=0`，不引入新依赖、配置或持久化协议。

## 已实施路径

1. `Forward` 调用诊断 owner 的准入函数，仅对启用观察且不超过 8 MiB 的 OpenAI OAuth 请求克隆 body。在 timezone/协议改写之前捕获；缺少快照时不启动后续对比。设置仍复用既有缓存，不为 API Key 请求查询设置。
2. Chat→Responses 按 tool index 累积参数。首片直接保留，第二片才分配 builder/map。复用上一轮的线性累积 helper；保留公开字符串快照、迟到名称、custom/tool_search/namespace 事件、首片不翻倍和最终参数校验。
3. 图片计数在完整 JSON 校验后同步读取局部投影，集中读取 root/response/data/data.response 的数量，避免全包字符串转换和每条路径反复扫描。重复父字段回退完整路径，保留 gjson 对嵌套缺失值的查找语义。去重 ID、尺寸独立复制，不把上游可复用缓冲的视图写入跨事件状态。
4. WS 修复入口同时返回校验状态；普通帧及独立解码的修复文档复用有效性证明，超过 repair limit 的单帧仍单独校验。终止事件有尾部文档时仍关闭连接。Envelope 的 response 仅供同步读取，持久 event type / response ID 独立保存。

## Docker 性能

与调查相同的离线 Docker 环境、2 CPU、4 GiB、`GOMAXPROCS=2` / `GOMEMLIMIT=1400MiB` / `GOFLAGS=-p=1`；200 ms × 3，取中位数。overlay 仅注入原 `probes/*.go` 测试 fixture，生产文件使用当前实现。对照在无其他验证并行时顺序运行。MB 为十进制累计分配量，不是 RSS。最终源码输出见 [probes-final-source.txt](../probes-final-source.txt)。

| 局部场景 | 耗时：优化前 → 后 | 分配：优化前 → 后 |
| --- | --- | --- |
| 1 MiB Chat 工具参数，1 KiB 分片 | 75.55 → 2.85 ms | 542.44 → 5.65 MB |
| 256 KiB Chat 工具参数，1 KiB 分片 | 5.79 → 0.68 ms | 34.89 → 1.29 MB |
| 1 MiB 无图片文本 delta 的图片计数 | 5.36 → 1.00 ms | 2,113,632 → 96 B |
| 2 KiB 无图片文本 delta 的图片计数 | 11.06 → 2.86 µs | 4704 → 96 B |
| 8 MiB WS terminal envelope | 3.40 → 1.42 ms | 8,397,160 → 40 B |

诊断关闭、API Key、超过 8 MiB 的快照准入都为 0 B/op、0 allocs/op；启用诊断的 1 MiB OAuth 请求仍分配独立的 1,048,576 B 副本，符合观察隔离要求。上级快照 baseline 只隔离 `bytes.Clone` 成本，不能把其耗时与准入函数当作完整 Forward 的前后延迟。

实际生产入口的独立对照见 [admission-validation.txt](admission-validation.txt)：`reuse_false` 是旧校验序列，`reuse_true` 是新入口，1 MiB 相同输入为 3.67 → 1.84 ms。它复用 classifier 的严格有效性证明，同时检查外围只含 JSON 空白；fuzz 发现的 form-feed 输入已固定到回归种子。该组使用 300 ms × 3。所有数字均为 helper/局部流程，不代表整体网关加速倍数。

完整样本见 [probes-final-source.txt](../probes-final-source.txt)，中位数与百分比见 [comparison.json](comparison.json)，验证入口索引见 [summary.json](summary.json)。

## 验证边界

针对性测试包含诊断开关、账号/大小准入和原始内容隔离，跨并行工具的 delta/done/terminal 一致性，图片报告数量/重复字段/缓冲复用，WS 畸形帧、拼接文档及超出 16 MiB 的合法帧的流式/非流式路径。完整测试与模糊测试结果在本目录记录。

最终证据仅保留以下结果：

- [unit.txt](unit.txt)：完整后端 `go test -tags=unit ./... -count=1` 通过（73 个有测试的包）。测试在容器内的临时后端副本运行，以允许 Ent 创建 `.entc`；宿主源码仍只读。
- [fuzz-image-accounting.txt](fuzz-image-accounting.txt)、[fuzz-ws-envelope.txt](fuzz-ws-envelope.txt)、[fuzz-ws-validation.txt](fuzz-ws-validation.txt)：每组 8 秒，共 33,138 次差分输入执行，无语义差异。
- [build.txt](build.txt)：最终源码 Linux arm64 release 编译通过，`CGO_ENABLED=0` 工具链元数据一并保存。
- [mcp-sdk.txt](mcp-sdk.txt)：`make test-unit` 的官方 Go MCP SDK OAuth 互操作补充检查通过。

复跑 fuzz 使用上级说明的相同容器参数，在可写的临时后端副本中执行 `go test ./internal/application/service -run '^$' -fuzz '^TARGET$' -fuzztime=8s -parallel=2 -timeout=4m`，依次替换 `TARGET` 为 `FuzzImageAccountingProjection`、`FuzzOpenAIWSEventEnvelope`、`FuzzOpenAIWSJSONValidation`。诊断基准和生产 observer 均不记录请求原文。

Tokenizer、额外 custom-input 解码、命名空间 stripping 和其他桥接分支未在本轮改动，继续按调查报告的条件评估。数据库/Redis repository 与结算事务没有变化，回退可直接回退代码。
