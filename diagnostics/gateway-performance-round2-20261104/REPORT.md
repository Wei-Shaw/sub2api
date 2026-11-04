# 第二轮优化候选与 Docker 证据

在 PR #177 的基础上，本轮已完成诊断快照准入、Chat→Responses 工具参数线性累积、图片计数及 WS 校验/解析复用。以下保留调查基线和实现边界，最终测试与基准见 [实施记录](implementation/README.md)。完整 tokenizer 继续作为独立 Rust 原型候选，需先确认实际预检/回退流量。

基线、原始输出、profile 和复跑方法见 [README.md](README.md)。以下为 3 次样本中位数；MB 为十进制累计分配量。各 helper 成本不可直接相加为网关延迟，也不代表生产负载的流量分布。

| 顺序 | 候选 | 调查基线成本 | 处理方向 |
| --- | --- | --- | --- |
| 1 / P0 | 无条件的请求诊断快照 | 16 MiB 拷贝 0.845 ms、16.78 MB / 次 | 先判断 OAuth、诊断开关及长度，再复制原始 body |
| 2 / P0 | Chat→Responses 工具参数逐片拼接 | 256 KiB 为 5.79 ms / 34.89 MB；1 MiB 为 75.55 ms / 542.44 MB | 按工具 index 线性累积参数，保留终止 JSON 校验和事件语义 |
| 3 / P1 | 纯文本事件进入图片计数全量扫描 | 2 KiB delta 为 11.06 µs / 4704 B；1 MiB 为 5.36 ms / 2.11 MB | 去掉两次全包字符串复制，复用解析视图、校验结果及已提取字段 |
| 4 / P1 | WS 重复 JSON 校验与大 envelope 复制 | 1 MiB 正常帧两次校验 4.03 ms；8 MiB envelope 3.40 ms / 8.40 MB | 在明确有效的路径复用 valid 状态，局部读取 envelope |
| 5 / 条件 P2 | 本地完整 tokenizer | 约 1 MiB 文本 54.77 ms / 65.18 MB / 约 130.6 万次分配 | 独立比较 Rust BPE/Count 实现，计入 FFI/IPC 和编码一致性成本 |

## 1. 按诊断准入创建原始快照

基线入口 [openai_gateway_forward.go](../../backend/internal/application/service/openai_gateway_forward.go) 在 timezone/协议处理前无条件执行 `bytes.Clone(body)`。该副本仅供两处 `observeOpenAIRequestIntegrity` 调用使用。诊断 owner 对非 OAuth、诊断关闭、超过 8 MiB 等情况直接返回。当前实现已在 [openai_request_integrity_observe.go](../../backend/internal/application/service/openai_request_integrity_observe.go) 中按诊断准入捕获快照。设置仍使用原缓存，本轮没有读取任何生产配置。

因此，在普通 API Key、关闭诊断或过长请求场景，这份副本不参与诊断。探针只隔离了 `bytes.Clone` 的成本，未运行完整 Forward：1 MiB 为 0.098 ms / 1.05 MB；16 MiB 为 0.845 ms / 16.78 MB。按 100 个同时存活的 16 MiB 副本计算，单这一份快照约占 1.56 GiB；这是理论存活量，未做实际并发 RSS 测试。

实施时仍需在任何可能改写 body 的函数之前捕获有效快照，并保留启用诊断时的数值精度、跨尝试隔离和差异检测。关闭诊断时不应为了判断是否复制而引入新的数据库热路径查询。

## 2. 修复另一条工具流的二次复杂度

基线 [chatcompletions_responses_bridge.go](../../backend/internal/shared/apicompat/chatcompletions_responses_bridge.go) 的 `ChatCompletionsChunkToResponsesEvents` 使用 `stored.Function.Arguments += fragment`。上一轮只修复了 Anthropic→Responses；本轮为 Chat→Responses 增加按工具 index 管理的 builder。

随着参数从 64 KiB 增至 256 KiB、1 MiB，累计分配分别约 2.37 MB、34.89 MB、542.44 MB，符合反复复制已有前缀的增长特征。[CPU profile](bridge-cpu-top.txt) 中 `runtime.memmove` 的 flat 占比为 45.79%；[分配 profile](bridge-alloc-top.txt) 的 99.81% flat 分配归属于该转换函数。

实现继续向公开 `Arguments` 字段提供快照，单片调用无需 builder，后续片段才分配累积状态。回归覆盖首片参数不翻倍、名称延迟到达、多个并行工具、custom/tool_search/namespace 调用、畸形参数拒绝、done 与终止 output 的内容一致性。

此外，Responses→Anthropic 的 Read 参数缓存、Chat→Anthropic 的延迟工具名称缓存仍有字符串 `+=`；这些特殊分支本轮只有静态线索，没有基准证据，不应与上述已测场景一并宣称完成。

## 3. 降低文本流的图片计数成本

基线 [image_output_accounting.go](../../backend/internal/application/service/image_output_accounting.go) 的 `AddSSEData` 无论是否为文本 delta，都执行字符串转换/TrimSpace、JSON 校验、`gjson.ParseBytes` 和多条计数路径读取；内部函数又校验一次。WS 和 HTTP SSE 共享这条计数路径。当前实现改用字节级 DONE 检查和局部投影，每次调用保留完整 JSON 校验，集中读取数量并独立保存去重 ID/尺寸。

1 MiB 无图片 delta 的 [CPU profile](image-cpu-top.txt) 中，`gjson.parseString` 为 63.81%，`gjson.validstring` 为 26.03%。[分配 profile](image-alloc-top.txt) 显示约 49.53% 来自 `ParseBytes`，约 49.04% 直接来自 `AddSSEData`，与两次全包字符串复制一致。

建议先用字节级 `[DONE]` 检查和受控只读 JSON 视图消除复制，再让已验证的事件复用解析状态，集中提取 root/response/data 下的计数。不能因为是文本事件就无条件忽略上游附带的计数字段。必须保留 `usage.images` / `tool_usage.image_gen.images`、没有 output item 的上游报告数量、嵌套返回、data 数组、去重及断连后最终用量；这属于计费正确性边界。

## 4. WS 校验和 envelope 读取

基线 [openai_ws_forwarder_v2.go](../../backend/internal/application/service/openai_ws_forwarder_v2.go) 先调用 splitter，再调用 `json.Valid`，丢弃了 classifier 的 valid 状态。当前 [openai_sse_json_documents.go](../../backend/internal/application/service/openai_sse_json_documents.go) 为 WS 提供准备入口，复用有效性证明并保留严格外围空白检查和超长单帧校验。调查原型的 1 MiB 对照为 4.03 → 2.00 ms；最终生产入口的独立对照为 3.67 → 1.84 ms。

这不等于可以删除全部校验：超过 16 MiB 时 classifier 不执行普通校验，拼接修复后的首帧/队列中后续帧也有不同来源。实现需明确记录每个 payload 的校验状态，保留畸形 JSON、最大长度、修复文档数及终止事件后尾部帧导致连接不可复用等契约。

基线 [openai_ws_forwarder_logutil.go](../../backend/internal/application/service/openai_ws_forwarder_logutil.go) 的 `parseOpenAIWSEventEnvelope` 用 `gjson.GetManyBytes` 提取整个 `response`。8 MiB terminal 的基线为 3.40 ms / 8.40 MB；只读视图原型为 1.44 ms / 40 B，最终实现为 1.42 ms / 40 B。

原型只对大终止、root ID 回退和转义 ID fixture 做结果对照；最终实现增加了与原函数的差分 fuzz 及缓冲复用测试。小 ID 独立保存，JSON response 视图只用于当前读取阶段，不能在复用输入缓冲后继续使用。

## 5. Tokenizer 适合作为有条件的 Rust 原型

入口 [openai_gateway_count_tokens.go](../../backend/internal/application/service/openai_gateway_count_tokens.go) 的完整估算用于 `responses/input_tokens`、兼容 `count_tokens` 或不支持上游的回退；普通生成首包已有字节估算，不应重新引入完整 tokenizer。

预热词表后，约 1 MiB 合成英文文本的完整估算为 54.77 ms / 65.18 MB；直接复用 codec 后，仅 `codec.Count` 仍为 50.31 ms / 64.11 MB，因此只缓存 codec 不能解决主要成本。[分配 profile](tokenizer-alloc-top.txt) 的 80.54% flat 分配来自 `regexp2.newMatch`。CPU 涉及 Unicode 分类、生成的 split 正则和大量分配。

可以比较低分配的 Go Count 实现与 Rust BPE/Count 库，先以独立、计数专用接口做实验。必须验证 o200k/cl100k、中文、代码、Unicode、特殊 token 和原有估算中的消息/工具开销，并计入传输或 FFI 成本。当前没有 Rust 对照结果；只有实际预检流量足以构成 CPU 瓶颈时，才值得承担额外编译和部署维护。

## 后续候选与验证范围

- Custom/freeform input 增量解码：现有 64-byte 分片 benchmark 在 1 MiB 时约 8.03 ms / 10.22 MB / 65,574 次分配。每片 `json.Unmarshal` 可进一步检查，但应排在工具参数的二次复制之后，并保留跨片 JSON 转义、代理对和 UTF-8 边界。
- Namespace stripping：带 namespace 标记的 8 MiB 历史，即便保持内容不变也约 6.1 ms，但为 0 B/op；实际删除约 8.56 ms / 16.79 MB。只在相关兼容路径有足够流量时提升优先级，现有行为已经是线性。
- Content session seed 的大历史 fixture 约 0.325–0.329 ms、不到 2.3 KB 分配；token 事件类型分类约 3.62 ns、0 分配。相较本轮热点，继续重构这些 helper 的收益较低。

Docker 的调查基准及相关协议回归执行成功。前四项 Go 优化已实施，实施结果见 [implementation/README.md](implementation/README.md)，优化后原始输出、fuzz 和完整包测试均在其目录。Tokenizer、额外 custom-input 解码和命名空间 stripping 未在本轮改动。推荐实施顺序已落地为：诊断快照准入 → 工具参数线性累积 → 图片计数及 WS 解析复用；Rust tokenizer 原型作为独立实验。

与调查相同的 helper 对照中，最终源码的 1 MiB Chat 工具参数从 75.55 ms / 542.44 MB 降至 2.85 ms / 5.65 MB；1 MiB 无图片文本计数从 5.36 ms / 2.11 MB 降至 1.00 ms / 96 B；8 MiB WS envelope 从 3.40 ms / 8.40 MB 降至 1.42 ms / 40 B。完整 service/apicompat unit、三组 fuzz 和 CGO_ENABLED=0 编译在实施证据中记录。
