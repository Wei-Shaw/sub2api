## 1. 门禁核心实现

- [x] 1.1 新增 `backend/internal/service/openai_opencode_freetier.go`
- [x] 1.2 定义常量：`openCodeSessionHeader`（复用既有 `X-OpenCode-Session`，Go 的 canonical 化后线上即为 `X-Opencode-Session`）、`openCodeUpstreamUserAgent`、`openCodeGateToolNames = ["bash","read"]`、规范 ID 正则 `^ses_[0-9a-f]{12}[0-9A-Za-z]{14}$`
- [x] 1.3 实现 `canonicalOpenCodeID(prefix, source string) string`：`sha256` 前 6 字节转 12 位小写 hex，其后 14 字节对 62 字符表取模得 14 位后缀
- [x] 1.4 实现 `isOpenCodeFreeTierModel(model string) bool`：复用 `normalizeOpenCodeGoModelID` 剥离前缀后判断 `-free` 后缀
- [x] 1.5 实现 `openCodeFreeTierGateMode(account *Account) string`，读取凭据 `free_tier_gate`，缺省 `auto`，非法值回落 `auto`
- [x] 1.6 实现 `shouldApplyOpenCodeFreeTierGate(account *Account, model string) bool`
- [x] 1.7 实现 `applyOpenCodeFreeTierGateSession(account, headers, body)`：形状合法的客户端 `ses_` 值原样保留，否则用确定性铸造值覆盖
- [x] 1.8 实现 `applyOpenCodeFreeTierGateBody(account, body, dialect) ([]byte, error)`：强制 `stream:true`；按方言补齐缺失的 `bash` / `read` 工具；形状不符时原样返回
- [x] 1.9 实现 `openCodeGateToolJSON(name, dialect)`：chat 用 `function` 嵌套，responses 用扁平 `parameters`，anthropic 用扁平 `input_schema`
- [x] 1.10 方言由调用方显式传入（三条 funnel 各自已知协议），不做启发式嗅探
- [x] 1.11 修复实现过程中发现的缺口：Zen 账号的 `shouldGenerateOpenCodeSession` 为 false（只认 `/zen/go`），既有链路不会补会话头。门禁改为自行派生：已落地会话头 → 请求体稳定字段 → 模型名 + 首条用户消息的内容派生值 → 随机值兜底，保证跨轮次稳定

## 2. 接入三条出站 funnel

- [x] 2.1 `openai_gateway_cc_pipeline.go`：`ensureDeepSeekChatReasoningPlaceholders` 之后接入 body 伪装，会话伪装紧随 `applyOpenCodeSessionHeader`
- [x] 2.2 `openai_gateway_forward.go`：`normalizeDeepSeekResponsesRequestBody` 之后接入 body 伪装，会话伪装紧随 `applyOpenCodeSessionHeader`
- [x] 2.3 `openai_gateway_messages_anthropic_native.go`：`clampOllamaCloudAnthropicMessagesMaxTokens` 之后接入 body 伪装，会话伪装紧随 `applyOpenCodeSessionHeader`
- [x] 2.4 `account_test_service.go` 与 `account_test_service_cn_adaptive.go` 的 4 条连通性测试路径复用同一门禁逻辑
- [x] 2.5 `openai_gateway_passthrough.go` 经核实对 OpenCode 不可达（要求 `Platform == PlatformOpenAI`），且该处改写 body 需重建 `req.Body`，故不接入

## 3. UA 版本下限修复

- [x] 3.1 将 `openCodeUpstreamUserAgent` 从 `opencode/1.0.0` 提升为 `opencode/1.18.31 ai-sdk/provider-utils/4.0.23 runtime/bun/1.3.14`，注释记录 426 UpgradeRequired 下限来源
- [x] 3.2 核实无测试断言旧 UA 字面量（6 处均引用常量），无需改动

## 4. 定价（不改动）与模型目录

- [x] 4.1 实测干净 HEAD 上 11 个免费层模型的定价：`deepseek-v4-flash-free` 因 `deepseek-v4-flash` 子串命中而按付费价计费，其余 10 个 `pricing not found`（记零成本，请求仍正常）
- [x] 4.2 核实「查不到价不会拒绝请求」：`openai_gateway_usage.go:253-267` 捕获 `ErrModelPricingUnavailable` 后记零成本继续
- [x] 4.3 核实内置价卡刻意无通用兜底（`billing_service.go:1223` `return nil`，注释「未知型号不回退以避免误计价」）；运营侧兜底为分组/渠道定价的尾随 `*` 通配（`model_pricing_resolver.go:161`）
- [x] 4.4 评审结论：**不引入任何免费层定价特判**。曾两次尝试（`-free` 一律零价；剥离后缀递归定价）均被否决——属于擅自新增原代码没有的定价规则
- [x] 4.5 撤销全部定价改动，`billing_service.go` 回到与 HEAD 零 diff；同时移除随之新增的定价测试
- [x] 4.6 `opencode_go.go` 新增 `DefaultOpenCodeZenFreeModelIDs()`（官方目录 11 个免费层模型），并入 `DefaultOpenCodeGoModelIDs()`
- [x] 4.7 核实 `muse-spark-*-free` 命中 `muse-spark-*` → Responses，其余免费模型走 Chat Completions
- [x] 4.8 实现 `NormalizeOpenCodeFreeTierGateCredentials`，非法取值返回 400 `INVALID_OPENCODE_FREE_TIER_GATE`，接入 `admin_account.go` 4 条账号写入路径

## 5. 测试

- [x] 5.1 `canonicalOpenCodeID` 形状断言（长度 30、前缀、12 位小写 hex、14 位字母数字、幂等、不碰撞、前缀参与结果）
- [x] 5.2 `isOpenCodeFreeTierModel` 表驱动：官方免费 ID、大写、前缀、空白；付费 ID（含 `deepseek-v4-flash`）不得误判
- [x] 5.3 `shouldApplyOpenCodeFreeTierGate` 表驱动：缺省 auto × 免费/付费、always、off、非法值回落、非 OpenCode 平台、空账号
- [x] 5.4 body 伪装：补齐、只补缺失、保留客户端工具、不重复注入、`stream:false` 改写、畸形 body 与 tools 非数组原样返回
- [x] 5.5 responses（扁平 `parameters`）与 anthropic（`input_schema`、无 `type` 包装）方言形状断言
- [x] 5.6 会话伪装：合法 `ses_` 原样保留、UUID 被改写、同来源跨轮次稳定、付费模型不改写、无会话头时不新增
- [x] 5.7 端到端 body 形状断言：四项硬要求同时成立且 body 仍为合法 JSON
- [x] 5.8 回归：`auto` 模式下付费模型出站 body 逐字节不变；`off` 模式同样不变
- [x] 5.9 定价测试全部移除，与「不改动定价」一致；`billing_service.go` 相关测试文件零改动

## 6. 前端

- [x] 6.1 `credentialsBuilder.ts` 新增 `OpenCodeFreeTierGateMode`、`OPENCODE_FREE_TIER_GATE_KEY`、`DEFAULT_OPENCODE_FREE_TIER_GATE`、`resolveOpenCodeFreeTierGate`、`applyOpenCodeFreeTierGate`（create 时缺省值不落库，edit 时始终写入以便改回 auto）
- [x] 6.2 新增 `OpenCodeFreeTierGateSelect.vue`：三选项下拉 + 随模式变化的提示文案，`data-testid="opencode-free-tier-gate-select"`
- [x] 6.3 `CreateAccountModal.vue` 与 `EditAccountModal.vue` 在 OpenCode 区块挂载该组件，接入凭据读取、写入与重置逻辑
- [x] 6.4 中英文 i18n（`admin.accounts.opencodeGo.freeTierGate.*`）：标题、三个选项文案、三段随模式变化的提示
- [x] 6.5 新增 `OpenCodeFreeTierGateSelect.spec.ts`：三个选项渲染、提示随模式变化、change 事件、未知值回落 auto、凭据助手 create/edit 写入语义与往返一致性

## 7. 验证

- [x] 7.1 `go build ./...` 通过
- [x] 7.2 `go test -tags=unit` 覆盖 `internal/service`、`internal/handler`、`internal/server` 各包；唯一失败 `TestOllamaProbeCallback_StaleLongDoesNotOverrideNewShort` 经干净 HEAD 工作树复现，确认为既有失败，与本次改动无关
- [x] 7.3 `gofmt -l` 无输出；`go vet ./internal/service/` 通过
- [x] 7.4 真实上游联调（`openai_opencode_freetier_live_test.go`，`-tags=live`）：三组全部通过——对照组无门禁 403 FreeTierError、实验组返回 200、边界组（客户端不提供任何会话标识）返回 200
- [x] 7.5 前端：`vue-tsc --noEmit` 通过；i18n 完整性检查通过；ESLint 通过；受影响的 4 个测试文件 173 个用例与新增 10 个用例全部通过
- [x] 7.6 记录验证边界：Responses 方言因地理封锁（`403 RegionError`）无法从当前网络真实联调；`/zen/go` 付费链路的版本下限因认证先行失败（401）无法判定；`golangci-lint` 因传递依赖校验和不匹配无法安装，CI 的 lint 关卡未被本地覆盖


