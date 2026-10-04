## Purpose

规定 Sub2API 作为 OpenCode Zen 上游客户端时，如何让免费层模型（模型 ID 以 `-free` 结尾）通过上游的「仅允许官方 OpenCode CLI 发起」门禁，并配套解决免费层模型的计费与目录可见性，使免费层模型在网关侧可用。

## ADDED Requirements

### Requirement: 账号级门禁开关
系统 MUST 支持账号凭据 `free_tier_gate`，取值 MUST 限于 `auto`、`always`、`off`。该凭据 MUST 存放于账号既有 `credentials` 字段，MUST NOT 新增数据库列。未配置时 MUST 等同 `auto`。非法取值 MUST 被拒绝并返回 400 与错误码 `INVALID_OPENCODE_FREE_TIER_GATE`。

#### Scenario: 未配置凭据的既有账号
- **WHEN** 一个 OpenCode 账号没有 `free_tier_gate` 凭据
- **THEN** 系统 MUST 按 `auto` 处理
- **THEN** 该账号的付费模型请求 MUST NOT 被门禁伪装改动

#### Scenario: 非法取值被拒
- **WHEN** 账号创建或更新请求携带 `free_tier_gate: "sometimes"`
- **THEN** 系统 MUST 返回 400 且错误码为 `INVALID_OPENCODE_FREE_TIER_GATE`
- **THEN** 系统 MUST NOT 持久化该账号

### Requirement: 门禁作用域判定
`auto` 模式下系统 MUST 仅对模型 ID 以 `-free` 结尾的请求启用门禁伪装。`always` 模式下系统 MUST 对该账号全部 OpenCode 请求启用。`off` 模式下系统 MUST NOT 启用。非 OpenCode 平台账号 MUST NOT 启用。模型 ID 判定 MUST 大小写不敏感，且 MUST 先剥离 `opencode-go/`、`opencode_go/`、`opencode/` 前缀。

#### Scenario: auto 模式下的免费模型
- **WHEN** 账号为 `auto` 且请求模型为 `mimo-v2.6-flash-free`
- **THEN** 系统 MUST 对该请求启用门禁伪装

#### Scenario: auto 模式下的付费模型
- **WHEN** 账号为 `auto` 且请求模型为 `deepseek-v4-flash`
- **THEN** 系统 MUST NOT 注入门禁会话头
- **THEN** 系统 MUST NOT 注入 `bash` / `read` 工具
- **THEN** 系统 MUST NOT 改写 `stream`

#### Scenario: off 模式
- **WHEN** 账号为 `off` 且请求模型为 `mimo-v2.6-flash-free`
- **THEN** 系统 MUST NOT 启用任何门禁伪装

### Requirement: 规范会话标识
启用门禁时，系统 MUST 向上游发送 `X-Opencode-Session`，其值 MUST 匹配 `^ses_[0-9a-f]{12}[0-9A-Za-z]{14}$`（长度恒为 30）。铸造 MUST 为确定性：对同一来源字符串 MUST 恒等，不同来源 MUST 以压倒性概率不同。铸造算法 MUST 为 `sha256(source)` 的前 6 字节转 12 位小写十六进制，其后 14 字节各自对 62 字符字母表 `0-9A-Za-z` 取模得 14 位后缀。客户端已提供的、形状合法的 `ses_` 值 MUST 原样保留，MUST NOT 被重新铸造。

既有会话解析链在 Zen 模式下 MUST NOT 被视为充分：`shouldGenerateOpenCodeSession` 仅在账号为 Go 订阅或目标路径含 `/zen/go` 时为真，Zen 账号（路径 `/zen/v1`）不满足。因此门禁 MUST 在既有一切来源都缺失时自行派生会话值，MUST NOT 依赖既有链路补值。派生顺序 MUST 为：已落地的会话头 → 请求体稳定会话字段（`prompt_cache_key` / `metadata.user_id`）→ 模型名加首条用户消息的内容派生值 → 随机值兜底。

#### Scenario: 客户端已提供合法会话
- **WHEN** 客户端 `X-OpenCode-Session` 为形状合法的 `ses_` 值
- **THEN** 系统 MUST 原样发送该值

#### Scenario: 客户端提供非法会话
- **WHEN** 客户端 `X-OpenCode-Session` 为 UUID 或不匹配规范的字符串
- **THEN** 系统 MUST 发送铸造出的规范 `ses_` 值

#### Scenario: 同一会话跨轮次稳定
- **WHEN** 同一会话标识的两个连续请求经过门禁
- **THEN** 两次出站 `X-Opencode-Session` MUST 相同

#### Scenario: 客户端不提供任何会话标识
- **WHEN** Zen 账号收到既无会话头、也无 `prompt_cache_key` / `metadata.user_id` 的免费层请求
- **THEN** 系统 MUST 自行派生出形状合法的 `X-Opencode-Session`
- **THEN** 出站请求 MUST NOT 因缺少会话头被上游拒绝

#### Scenario: 内容派生值跨轮次稳定
- **WHEN** 同一会话的后续轮次保持首条用户消息不变、仅追加历史消息
- **THEN** 两次派生的会话值 MUST 相同，以保持上游 prompt cache 命中

#### Scenario: 会话值镜像到 affinity 头
- **WHEN** 系统发送 `X-Opencode-Session`
- **THEN** 系统 MUST 把同一值同时写入 `X-Session-Affinity` 与 `X-Session-Id`
- **THEN** 该镜像 MUST 与官方客户端行为一致（门禁本身不校验这两个头，实测只发 `X-Opencode-Session` 仍 200）

#### Scenario: 铸造值形状
- **WHEN** 系统铸造会话标识
- **THEN** 值 MUST 以 `ses_` 开头，总长度 MUST 为 30，第 5 至 16 字符 MUST 为小写十六进制，末 14 字符 MUST 属于 `0-9A-Za-z`

### Requirement: 客户端版本下限
发往官方 OpenCode 上游的 `User-Agent` MUST 为 `opencode/<version>` 形态且 version MUST NOT 低于 1.18.0。该约束 MUST 同时覆盖 `go` 与 `zen` 两种账号模式。

#### Scenario: 出站 UA 满足下限
- **WHEN** 系统向 opencode.ai 发起任意 OpenCode 请求
- **THEN** 出站 `User-Agent` MUST 含 `opencode/` 且解析出的版本 MUST ≥ 1.18.0

#### Scenario: 账号级覆写优先
- **WHEN** 账号 `header_overrides` 显式配置了 `user-agent`
- **THEN** 该显式值 MUST 保持最终决定权

### Requirement: 出站 body 门禁伪装
启用门禁时，系统 MUST 保证出站 body 的 `stream` 为 `true`，且 `tools` 数组中 MUST 同时存在名为 `bash` 与 `read` 的工具（这两项是门禁的硬性要求，实测缺一即 403）。

系统 MUST 把工具集补齐到官方客户端的 6 个核心工具（`bash`、`edit`、`glob`、`grep`、`read`、`write`），MUST 使用与官方客户端一致的参数 schema，并按工具名排序。已存在的同名工具 MUST NOT 被重复注入或改动，客户端自带的其他工具 MUST NOT 丢失。

原本不带任何工具的请求（压缩/总结、纯聊天客户端）在补齐工具集后 MUST 同时禁止工具调用（Chat Completions 与 Responses 用 `"none"`，Anthropic 用 `{"type":"none"}`），否则模型会调用客户端无法执行的工具。客户端已显式声明 `tool_choice` 时 MUST NOT 覆盖。

Chat Completions 方言 MUST 使用 `{"type":"function","function":{"name":...}}` 嵌套形状，Responses MUST 使用扁平 `{"type":"function","name":...}` 形状，Anthropic MUST 使用 `{"name":...,"input_schema":...}` 形状。body 不是合法 JSON 或结构不符时系统 MUST 原样返回，MUST NOT 报错中断请求。

#### Scenario: 无工具时补齐到官方 6 件套
- **WHEN** 出站 body 的 `tools` 缺失且模型为免费层
- **THEN** 出站 `tools` MUST 同时含 `bash` 与 `read`
- **THEN** 出站 `tools` MUST 为官方 6 件套且按名称排序
- **THEN** 出站 `tool_choice` MUST 为禁止调用工具的取值

#### Scenario: 仅缺一个工具
- **WHEN** 出站 body 已有 `bash` 但缺 `read`
- **THEN** 系统 MUST 只补缺失的官方工具
- **THEN** 已有的 `bash` 定义 MUST 保持不变

#### Scenario: 工具按名称排序
- **WHEN** 客户端以非字母序提供工具
- **THEN** 出站 `tools` MUST 按工具名升序排列

#### Scenario: 客户端自带工具不丢失
- **WHEN** 客户端提供了自定义工具 `my_tool`
- **THEN** 出站 `tools` MUST 保留 `my_tool`
- **THEN** 系统 MUST NOT 重复注入官方工具

#### Scenario: 原本有工具时不改 tool_choice
- **WHEN** 客户端请求已带 `tools` 且显式声明 `tool_choice`
- **THEN** 出站 `tool_choice` MUST 保持客户端取值

#### Scenario: 强制流式
- **WHEN** 客户端请求 body 的 `stream` 为 `false` 或缺失且启用门禁
- **THEN** 出站 body 的 `stream` MUST 为 `true`

#### Scenario: 畸形 body 不中断请求
- **WHEN** 出站 body 不是合法 JSON
- **THEN** 系统 MUST 原样返回该 body
- **THEN** 系统 MUST NOT 因门禁伪装而向客户端返回错误

### Requirement: 覆盖全部原生协议出站路径
门禁伪装 MUST 在三条原生协议出站路径上生效：Chat Completions、Responses、Anthropic Messages。任一 ingress（`/v1/chat/completions`、`/v1/responses`、`/v1/messages`）经协议转换后落到任一出站路径时，出站请求 MUST 满足全部四项门禁要求。

#### Scenario: Chat Completions ingress 落到 Chat 出站
- **WHEN** 客户端以 `/v1/chat/completions` 请求免费层模型且路由到 Chat Completions
- **THEN** 出站请求 MUST 同时满足会话形状、UA 下限、`bash`+`read` 工具与 `stream:true`

#### Scenario: Responses ingress 落到 Responses 出站
- **WHEN** 客户端以 `/v1/responses` 请求免费层模型且路由到 Responses
- **THEN** 出站请求 MUST 满足四项门禁要求，且工具 MUST 使用 Responses 扁平形状

#### Scenario: Anthropic ingress 落到 Messages 出站
- **WHEN** 客户端以 `/v1/messages` 请求免费层模型且路由到 Anthropic Messages
- **THEN** 出站请求 MUST 满足四项门禁要求，且工具 MUST 使用 Anthropic 扁平形状

### Requirement: 管理端可配置门禁模式
账号创建与编辑界面 MUST 提供门禁模式选择，选项 MUST 为 `auto`、`always`、`off`。界面的提示文案 MUST 随所选模式变化，并 MUST 说明各模式的副作用。创建账号时若模式为缺省的 `auto`，凭据 MUST NOT 写入该字段，使后端缺省语义与存储内容一致；编辑账号时 MUST 始终写入，使运营者能把 `always` / `off` 改回 `auto`。界面遇到未知模式值 MUST 回落显示 `auto`。

#### Scenario: 创建账号保留缺省
- **WHEN** 运营者创建 OpenCode 账号且未改动门禁模式
- **THEN** 提交的凭据 MUST NOT 包含 `free_tier_gate`
- **THEN** 后端 MUST 按 `auto` 处理

#### Scenario: 编辑账号改回缺省
- **WHEN** 账号凭据中 `free_tier_gate` 为 `off`，运营者在编辑界面改回「仅免费模型」
- **THEN** 提交的凭据 MUST 包含 `free_tier_gate: auto`

#### Scenario: 未知模式回落
- **WHEN** 账号凭据中的 `free_tier_gate` 为界面不认识的取值
- **THEN** 界面 MUST 显示为 `auto`

### Requirement: 不改动定价逻辑
本能力 MUST NOT 引入任何针对免费层（`-free` 后缀）的定价特判，MUST NOT 修改 `getFallbackPricing`、`applyModelSpecificPricingPolicyEx` 或任何定价解析路径的既有行为。免费层模型的计费结果 MUST 与本能力引入前逐一致：名称恰好包含某付费家族子串者按该家族价计费，其余按「无价可循」记零成本。

#### Scenario: 含付费家族子串的免费模型保持原行为
- **WHEN** 系统解析 `deepseek-v4-flash-free` 的定价
- **THEN** 结果 MUST 与本能力引入前一致（因含 `deepseek-v4-flash` 子串而命中该家族价）

#### Scenario: 无家族匹配的免费模型保持原行为
- **WHEN** 系统解析 `nemotron-3-ultra-free` 的定价
- **THEN** 结果 MUST 与本能力引入前一致（无价可循，按零成本落账并告警）

#### Scenario: 运营者定价不受影响
- **WHEN** 运营者为任意模型（含免费层）配置分组或渠道定价
- **THEN** 计费 MUST 使用运营者配置的价格，行为 MUST 与本能力引入前一致

### Requirement: 免费层模型目录可见
OpenCode Zen 模式的模型目录回退 MUST 包含官方免费层模型 ID，使这些模型 MUST 能在 `/v1/models` 与账号白名单预填中出现。`muse-spark-*-free` MUST 命中 `muse-spark-*` 规则从而路由到 Responses，其余免费模型 MUST 路由到 Chat Completions。

#### Scenario: 免费模型出现在目录回退中
- **WHEN** 上游模型列表尚未同步且账号为 Zen 模式
- **THEN** 目录回退 MUST 包含 `mimo-v2.6-flash-free`、`deepseek-v4-flash-free`、`nemotron-3-ultra-free` 等官方免费层 ID

#### Scenario: 免费模型协议路由
- **WHEN** 系统解析 `muse-spark-1.3-contributor-free` 的原生协议
- **THEN** 结果 MUST 为 Responses

#### Scenario: 非 muse 免费模型协议路由
- **WHEN** 系统解析 `mimo-v2.6-flash-free` 的原生协议
- **THEN** 结果 MUST 为 Chat Completions
