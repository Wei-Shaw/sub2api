# OpenAI 请求的 JSON 批量编辑

`openAIRequestView.ApplyPatches` 使用 `github.com/MoeForge/moego/ojson` 的
`Doc` 合并同一请求上的多次字段修改，最后只生成一次完整结果。依赖固定为
`v0.1.0-alpha.1`（`d016502783037edc58e422331be2b6775d578c13`）。

这项优化不替换其他 gjson/sjson 调用、encoding/json 解码、协议转换或用量提取。
未知字段、顺序、重复键、数字拼写和未修改文本由字节级差分测试保护。

## 适用范围

批量路径只接受有效 JSON 对象、2–16 次标量修改、最多四级 ASCII 标识符路径。
根对象及被修改路径经过的原始父对象最多各 64 个成员。大 input、图片 data URL
或其他未修改内容不因其大小或内部字段数量而被排除。

下列情况沿用已有的顺序 sjson 实现：单次编辑，查询/索引/非 ASCII 路径，
异常 JSON，字符串父节点，父子路径交叠，容器或自定义序列化值，以及密集对象。
这些限制是优化资格判定，不增加请求拒绝规则。

限制对象成员数量很重要：Doc 会为经过的容器建立临时成员索引。一个约 40 KB、
4096 个短字段的对象，在直接使用 Doc 的实验中会累计分配约 720 KB，原 sjson
约 82 KB。现在这种对象回到原实现，不再增加该索引。预检只借用原始输入，
不通过 `Doc.Get` 检查已经修改的父对象，避免其重新物化大型容器。

## 所有权和预算

- 输入 body 在调用期间不可修改；Doc 和预检结果不会跨调用保存。
- `ApplyPatches` 的每种策略（批量 Doc、单次编辑、顺序 sjson）都产生一份独立
  输出，统一计入预算：编辑前先按输入长度预留，预算不足时在任何编辑工作之前拒绝；
  生成后按实际输出长度多退少补，失败则全部退回。批量和非批量请求的拒绝条件相同。
- Forward 为每次 patch 输出打开 `WithRequestBodyMemoryScope` 子额度。新输出取代
  旧输出时归还旧额度，Forward 返回时归还最后一份，因此账号切换重试不会在 handler
  返回前累积改写后的 body。ingress 额度仍随 handler 的既有释放函数归还。
- 该额度覆盖 patch 输出，不是整个进程堆的上限：顺序 sjson 的中间副本、
  decode+marshal 回退以及 Doc 成员索引不单独计入，后者通过上面的资格限制控制。
- Doc 拒绝某次编辑时改用顺序 sjson；sjson 也无法表达时（例如在数组下设置键），
  Forward 把待处理的 patch 重放到解码后的 map 上，走既有 marshal 回退，不会重复
  执行同一批 patch。
- 预算耗尽立即返回，不通过另一种序列化方式绕过预算，也不切换账号重试。
  未提交 HTTP 响应时返回 503 和 `Retry-After: 1`；已有 SSE 心跳时发出
  `response.failed`，保留预期 503 的错误记录。
- 调用方继续决定取消和上游用量排空策略；编辑函数没有新增提前取消语义。

## 兼容性边界

上游 alpha 在 Go 1.27.3 下的差分测试存在非法 UTF-8 路径键的输出字节差异：
实际 U+FFFD 和 `\ufffd` 解码后值相同，但字节不同。此类路径不进入本次批量路径。
所有结构体/自定义值也保留原实现，避免改变自定义序列化的调用次数。

## 验证和基准

从 `backend` 运行：

```sh
go test -race ./internal/service -run '^(TestOpenAIRequestPatches_|TestOpenAIRequestView_|TestOpenAIGatewayService_OAuthLegacy_UpstreamRequestIgnoresClientCancel)' -count=1
go test -race ./internal/pkg/httputil -run 'Test(ReserveRequestBodyMemory|RequestBodyMemoryBudget)' -count=1
go test -race ./internal/handler -run '^TestOpenAIForwardRequestBodyMemoryOverload' -count=1
go test -race github.com/MoeForge/moego/ojson -run '^TestDoc' -count=1
go test ./internal/service -run '^$' -bench '^BenchmarkOpenAIRequestPatches' -benchmem -benchtime=150ms -count=3 -cpu=1
```

`SequentialSJSON` 保留原顺序实现作为基准和字节级 oracle。`ApplyPatches` 包含
优化资格检查和完整 JSON 验证成本；两个基准均不包括 request view 构造。
既有 `BenchmarkOpenAIResponses_LargeInputRawPatch` 则包含 view 构造。

基准报告的 B/op 是每次调用累计分配字节，不能当作驻留堆或 RSS，也不能直接推断
整站吞吐、首 token 延迟或上游请求耗时。单次修改应保持相同分配；密集对象回退
会有额外预检成本。ARM64 的当前 ojson 没有 AMD64 的 AVX2 路径，应单独测量。

### 2026-11-03 实测

Go 1.27.3，每组 150ms × 3，单 CPU，中位数。Linux 使用支持 AVX2 的
Xeon Skylake，并固定 CPU；Mac 使用 Apple M2。六次修改包含模型、reasoning、
删除 previous_response_id、输出上限及 service_tier，输入主体是长文本。

| 平台 / 输入 / 修改次数 | 原顺序实现 | 批量实现 | 原累计分配 | 新累计分配 |
|---|---:|---:|---:|---:|
| Linux AMD64 / 1 MiB / 6 | 3.816 ms | 0.756 ms | 6,341,302 B | 1,058,416 B |
| Linux AMD64 / 8 MiB / 6 | 23.162 ms | 5.353 ms | 50,381,747 B | 8,398,448 B |
| Mac ARM64 / 1 MiB / 6 | 2.446 ms | 1.263 ms | 6,341,291 B | 1,058,416 B |
| Mac ARM64 / 8 MiB / 6 | 12.023 ms | 9.290 ms | 50,381,720 B | 8,398,448 B |

单次修改在两个平台保持相同累计分配。4096 字段的密集根对象回退后，Linux
耗时约 170 μs、累计分配 82,081 B，与原实现相当；密集嵌套对象预检增加约 6%
耗时，分配相同。三个重复嵌套叶节点修改在 Linux 上约快 2.1 倍；Mac 上约慢 8%，
但两个平台累计分配都从 3,171,120 B 降到 1,057,520 B。收益取决于输入与架构。

相关服务、handler、预算和上游 Doc 定向 race 测试通过；Linux AMD64 构建、
`go vet` 通过。当前本机 golangci-lint 2.13.2 的导入器最多支持 export data v4，
无法读取 Go 1.27.3 的 v5，lint 未完成。`go mod tidy -diff` 仅提示清理项目已有的
旧依赖校验和，本次仅加入 moego 所需校验和，没有混入这些无关清理。

## Messages 与 Responses 热点优化（2026-11-04）

这批优化减少重复工作，保留现有解析器和兼容处理的语义：

- `collectCacheControlPaths` 在同步扫描内借用 system/messages/tools 的原始视图，
  返回的路径和日志字符串仍拥有独立存储。断点数量限制和 TTL 顺序检查仍分两遍，
  保留重复键删除后露出下一个值的既有行为。
- OAuth Responses 的 orphan 清理先检查是否可能需要修改。没有工具输出或存在
  有效 `previous_response_id` 时跳过完整解码；重复相关键、转义键/类型及疑似工具
  输出继续走原 decoder/sanitizer。预检复用已有字节范围扫描，不展开大段转义文本。
  原先的首个 input 数组资格判断、末重复键解码和非法/尾随 JSON 错误保持不变。
- `RequestBodyRef` 保存成功解析的只读快照，包含独立标量字符串、协议和字段偏移，
  不额外引用旧 body。未变化的 `CloneForBody`/`ReplaceBody` 恢复快照；显式
  `RequestBodyRef.Replace` 即使传入同一 slice 也会失效。每次重试仍创建独立 Ref，
  清空上游接受回调。手工构造、解析失败、协议或 body 改变均继续解析。
- 复用除了要求 slice 起点和长度相同，还要求内容的 `maphash` 指纹与解析时一致。
  指纹每次复用需要一遍哈希，远低于完整解析；它让违反只读约定的原地修改退回
  重新解析，而不是恢复过期的模型、流式标记或字段偏移。

body 的输入与 `Bytes()` 结果均为只读借用；任何仍被请求或重试持有的底层存储都不应
原地修改。快照的恢复也会覆盖临时改动过的派生标量，与原先重新解析的效果一致。
快照增加少量固定元数据，不是新的跨请求缓存，没有后台生命周期或大 body 保留。

专用基准：

```sh
go test ./internal/service -run '^$' -bench '^(BenchmarkCacheControlPathsLargeContent|BenchmarkOpenAIResponsesOrphanPrecheck|BenchmarkGatewayRequestBodyReuse)$' -benchmem -benchtime=150ms -count=3 -cpu=1
```

`GatewayRequestBodyReuse` 从已经完成首次解析的请求开始，对比一次账号尝试的 clone
和两次未变化的刷新。`Reparse` 强制走完整解析，包含新增快照的固定成本，不能当作
旧提交的精确分配基线；`Reuse` 不代表整个请求已不需要 JSON 解析。
`OpenAIResponsesOrphanPrecheck` 仅比较 orphan 阶段；cache_control 的旧版基线可以
通过 Go overlay 使用修改前的 `gateway_claude_oauth_body.go` 测量。

实测环境为 Go 1.27.3、Linux AMD64 Xeon Skylake，固定单核、150ms × 3，中位数：

| 组件 / 合成输入 | 对照耗时 | 优化后耗时 | 对照累计分配 | 优化后累计分配 |
|---|---:|---:|---:|---:|
| cache_control 无需修改 / 1 MiB | 5.854 ms | 5.119 ms | 2,114,374 B | 476 B |
| cache_control 无需修改 / 8 MiB | 45.250 ms | 41.575 ms | 16,794,468 B | 476 B |
| Responses orphan / 4 MiB / 无 previous_response_id | 38.238 ms | 14.168 ms | 28,676,176 B | 10 B |
| Responses orphan / 4 MiB / 有 previous_response_id | 38.070 ms | 8.252 ms | 28,676,204 B | 6 B |
| Messages 三次后续解析 / 8 MiB / 强制解析与复用 | 63.068 ms | 0.000234 ms | 840 B | 240 B |

cache_control 输入含固定三个断点和变长普通文本，主要收益是消除复制；两遍扫描仍在。
Responses 输入含大段转义文本，无需修改，优化后各规模均为 0 allocs/op；少量 B/op
包含运行时摊销，不能把它解释为整个网关零分配。Messages 一行不包括首次解析，
也不包括实际执行兼容处理、网络传输或上游推理。这些结果不用于推断整站吞吐。

Messages 一行测量于加入内容指纹之前。加入指纹后每次复用需要一遍 `maphash`：
Apple M2 上 8 MiB 复用约 2.1 ms（强制解析约 60 ms），1 MiB 约 0.26 ms，
分配不变（240 B/op）。

服务层相关 unit/race 回归、handler 的取消/切换/预算回归、最终边界用例 race 复验、
`go vet` 和 Linux 基准程序编译均通过。仓库 fuzz 执行 1,393 次通过；将当前源函数
原样提取到临时模块后的 15 秒 isolated fuzz 执行 34,363 次通过。`golangci-lint`
仍受 Go 1.27.3 export data v5 与当前导入器仅支持 v4 的工具链兼容问题阻断。
