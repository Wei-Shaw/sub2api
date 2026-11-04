# Codex 0.166.0 请求协议核验（2026-11-04）

## 结论与修复

对照官方 rust-v0.160.1、rust-v0.163.0、rust-v0.166.0 请求类型、序列化和调用代码，抓取本机独立 arm64 CLI 0.166.0 的隔离回环请求，发现并修复两项问题：

1. **基础指令迁移**：0.160.1 的 Responses/WS 类型包含顶层 instructions；0.163.0、0.166.0 已移除，基础指令变为 input 内带 msg ID 的 developer 消息。Sub2API 原先仍会补入旧模型默认 instructions，形成两份提示词。已修复普通 Forward、OAuth transform 和透传入口：已有非空 developer 指令、未提供顶层 instructions 时，不再额外注入。旧客户端显式 instructions 和没有指令的兼容请求仍保留原处理。
2. **有效流选项被误删**：官方已有 stream_options.reasoning_summary_delivery="sequential_cutoff"，原代码将所有 stream_options 作为不支持字段删除。已在普通请求、HTTP 透传和 WS 归一化中保留该选项，include_usage 等旧参数仍裁剪；旧 /responses/compact 继续删除流选项。这是审计发现的既存缺陷，不宣称 0.166.0 才新增该字段。

## 逐项核对

| 范围 | 官方差异/语义 | 本次结果 |
|---|---|---|
| HTTP 顶层字段 | 0.160.1→0.163.0 移除 instructions，调整路由字段序列化顺序；0.163.0→0.166.0 字段集合不变 | 指令重复注入已修复；未为无语义变化的 JSON 字段顺序重写全部请求 |
| HTTP 其余 15 字段 | model, stream, service_tier, input, tools, tool_choice, parallel_tool_calls, reasoning, store, stream_options, include, prompt_cache_key, text, client_metadata, access_programs | 全部对照官方类型；不新增顶层字段白名单，继续按平台和账号能力处理 |
| WS | 相应字段，加 previous_response_id/generate；预热与增量续链 | 现有请求构造/续链/重放测试，加 HTTP/WS 流选项测试 |
| 默认值 | store=false、stream=true、tool_choice=auto；并行工具受模型/RLite 能力影响 | 保持现有 OAuth 和 RLite 约束 |
| reasoning/text | effort/summary/context、verbosity、严格 JSON schema | 未发现对应类型改名；新增结构化输出保留断言 |
| 加密回放 | 多类 ResponseItem 增加 encrypted_content/status；功能开关可加 include=output.encrypted_content | 新测试确认 include 两项、message/function_call 密文和 encrypted_function_args 保留；不伪造密文、不强开功能 |
| assistant 内容 | 输出 annotations/logprobs 可回放；phase 可为 partial_answer | 新测试确认部分答案、输出内容及空注解数组保留 |
| 内部消息元数据 | 0.163.0→0.166.0 增加 content_item_metadata；官方标准 Responses 路径清理该归因字段，RLite 可用 | 不合成内部归因；原消息 map 继续保留扩展字段。此项仅源码检查，未做真实上游 RLite 验收 |
| 工具 | function/custom/namespace/tool_search、input.additional_tools | tool_spec.rs 对比无差异；AdditionalTools 的 Vec→Arc 不改变 JSON。现有 RLite/工具名/调用 ID 测试覆盖 |
| client_metadata/access_programs | session/thread/turn/window/root_turn_id 等；access_programs 在旧类型中已存在，形如 {"cyber":"standard"} | 新测试确认 root_turn_id、未知 metadata 键、access_programs 和未知顶层对象保留；保留不等于授予上游权限 |
| 模型列表 | endpoint 请求字段未新增；目录消息增加 history/notes/context_management/incremental_tools 等 | 上游目录按 RawMessage 处理，不用固定 struct 整体截断；合成目录不宣称自动复制所有官方提示模板 |
| 原生图片 | ImageGenerationRequest/ImageEditRequest 字段在所比版本一致：prompt/model/n/background/quality/size；编辑多 images | 当前 image_url/data URL 组装保持；未扩展 file_id 输入能力；原生不发送 Version/OpenAI-Beta 的行为保留 |
| compact | 当前 CLI 主路径使用 Responses 原生压缩/上下文项，不能等同于旧 /responses/compact | 保留旧端点兼容；现有 compaction/context_compaction 和续链测试覆盖 |
| 新响应事件 | GuardianAssessment、routing affinity 等变化 | 属于响应/客户端运行时，不是请求参数增删；本次未移植整个官方运行时或做真实事件验收 |

## 验证与边界

- 本机原生二进制 --version 返回 codex-cli 0.166.0。
- 独立临时 CODEX_HOME，清除认证、代理变量和 TERM，抓取完整回环请求；探针返回 400 后停止，未发起真实账号推理。
- 默认实测请求无顶层 instructions，包含 input developer、store=false、stream=true、reasoning.encrypted_content；可选功能另按源码和合成 fixture 核验。
- 新增 openai_codex_166_protocol_test.go，覆盖普通/透传 Forward、加密回放、部分答案、输出注解、结构化输出、扩展字段、HTTP/WS 流选项与旧 compact。
- 全量检查还发现 5 处图片测试仍断言旧版本 0.163.0，已改用共享版本常量，防止再次漂移。
- 修改前新增测试失败，修改后专项通过；准确命令、输出、完整包测试和回滚证据见外部 VERIFICATION.txt。
- 不将所有旧兼容请求强制改成与新 CLI 完全相同。保留模型能力、账号隔离和显式配置语义。
- 尚未部署；未核验线上配置，未做真实上游推理/图片/WS/RLite 端到端验收。

## 可追溯证据

- [0.166.0 请求类型](https://github.com/openai/codex/blob/rust-v0.166.0/codex-rs/codex-api/src/common.rs)
- [0.166.0 请求组装](https://github.com/openai/codex/blob/rust-v0.166.0/codex-rs/core/src/client.rs)
- [0.166.0 消息和回放模型](https://github.com/openai/codex/blob/rust-v0.166.0/codex-rs/protocol/src/models.rs)
- [0.166.0 图片类型](https://github.com/openai/codex/blob/rust-v0.166.0/codex-rs/codex-api/src/images.rs)

本地目录：/Users/jinfeijie/.codex/codex-version-align-20261104/protocol-audit

request-field-inventory.json 是三个版本字段清单；official-source.diff 是选定协议文件差异；
request.json/all-headers.json 是隔离回环抓包；SOURCE_HASHES.txt 是下载源码哈希。
猜测路径产生的 404 不作为证据，实际核验采用官方 tag 完整归档和真实文件路径。

最终结果：GOTOOLCHAIN=go1.27.2 go test ./internal/service ./internal/pkg/openai -count=1 通过（service 119.658s、openai 0.008s）；最终协议专项通过（0.042s）；git diff --check 通过。

## 正常调用流程复核（按用户收窄后的范围）

本轮不再扩展极端数字、畸形输入、压力或 race 测试，只检查实际 CLI 的正常请求。

- 使用本机原生 codex-cli 0.166.0，独立临时 CODEX_HOME，禁用插件推荐后台行为；CLI 指向本地 provider。CLI 本身生成请求，不是手写请求体代替客户端。
- 链路：真实 CLI → 本地 HTTP 接入 → Sub2API 的 Forward/透传实际代码 → 确定性模拟上游 SSE → CLI 执行 printf normal-tool-ok（退出 0）→ 工具结果进入第二次请求 → CLI 输出 normal-flow-ok。
- 普通转发和 openai_passthrough=true 分别完整跑通；每条链路均有两次请求。
- 新发现实际遗漏：普通/透传白名单缺少 session-id、thread-id、x-client-request-id；它们由 CLI 正常发出，却被丢弃。现已加入两个白名单，继续由现有账号命名空间逻辑处理，不原样跨账号复用。
- 校验 User-Agent=codex_exec/0.166.0 ... (codex_exec; 0.166.0)、Version=0.166.0、Originator=codex_exec、Content-Type、Accept 和上游路径。
- 校验 session-id/thread-id/x-codex-window-id/x-codex-turn-metadata 与请求体对应字段一致，beta features 保留。
- 对照 CLI 入站和最终出站：model、tool_choice、parallel_tool_calls、reasoning、include、text、stream/store 及工具定义/消息内容保留；不增加顶层 instructions。本机模型采用 RLite 时工具在 input.additional_tools 内，顶层 tools 缺省属于真实客户端形态。
- 既有账号隔离会改变 prompt_cache_key/client_metadata；既有调用 ID 归一化会同步改写 call/output 的 call_id。这些属于网关既有语义，不宣称与 CLI 请求逐字节相同；测试检查工具调用关联一致。
- 实际 CLI 普通/透传链路通过；WS 正常转发、OAuth 身份、图片生成/编辑、会话隔离及默认版本的相关回归通过。
- 新增可复用的 opt-in 测试 openai_codex_normal_flow_test.go；只有设置 SUB2API_CODEX_CLI 时才运行真实客户端。

这验证的是默认代码路径和本地完整协议循环，使用模拟模型响应。尚未部署，未验证运行中服务的管理员覆盖配置、认证调度入口或真实 OpenAI 模型端到端结果。

本轮证据目录：/Users/jinfeijie/.codex/codex-version-align-20261104/protocol-audit/normal-flow
其中 baseline-capture 保留修复前抓取，passthrough-false/true 保存 CLI 入站、Sub2API 最终出站及 CLI 执行日志，认证头未写入证据。
