# 验证：OpenCode Zen 免费层门禁伪装（2026-09-30，第二轮 2026-10-04）

## 已实现

- 账号级 `free_tier_gate` 凭据（`auto` 缺省 / `always` / `off`），存放于既有 `credentials` JSONB，无数据库迁移。
- 门禁伪装：规范 `ses_` 会话标识（含缺失时自行派生，并镜像到 `X-Session-Affinity` / `X-Session-Id`）、满足版本下限的出站 UA、按方言补齐官方 6 件套工具并按名排序、无工具请求补 `tool_choice` 禁止调用、强制 `stream:true`。
- 覆盖三条原生协议出站 funnel（Chat Completions / Responses / Anthropic Messages）与 4 条管理端连通性测试路径。
- 官方免费层模型目录（11 个）。**定价逻辑未改动**（`billing_service.go` 零 diff）。

## 第二轮：与参考实现对保真度（2026-10-04）

门禁的验收标准只是四项硬性要求，但参考实现 cpa-plugin-opencodezen 的伪装范围更大。对照它补齐三处差异，改动前先用真实上游验证每一项可行。

### 实测证据

| 组合 | 结果 |
| --- | --- |
| 6 官方工具 + `X-Session-Affinity` / `X-Session-Id` | **200** |
| 6 官方工具 + `tool_choice: "none"` | **200** |
| 6 官方工具 + `tool_choice: "none"`，只发 `X-Opencode-Session`（无 affinity） | **200** |

第三项证明 **affinity 头不是门禁要求**，补齐它纯粹是为了与官方客户端形态一致。

### 补齐的三处

| 项 | 参考实现 | 补齐前 | 补齐后 |
| --- | --- | --- | --- |
| 会话头 | `X-Opencode-Session` + 镜像到两个 affinity 头 | 只发 `X-Opencode-Session` | 镜像到 `X-Session-Affinity` / `X-Session-Id` |
| 工具集 | 官方 6 个 + 官方 schema + 按名排序 | bash+read，最小 schema，不排序 | 官方 6 件套 + 官方 schema + 字母序 |
| 无工具请求 | 注入全套 + `tool_choice: none` | 注入 bash+read，不设 `tool_choice` | 补 `tool_choice`（Anthropic 用 `{"type":"none"}`） |

`tool_choice` 的意义：补齐工具集会让原本无工具的请求（压缩/总结、纯聊天客户端）带上工具，模型可能去调用客户端无法执行的工具。客户端已显式声明 `tool_choice` 时不覆盖。

### 未采纳的一项

参考实现还发送 `X-Opencode-Request` / `X-Opencode-Client` / `X-Opencode-Project`。本能力实测这三者**非门禁必需**（各自缺失仍 200），故保持不发，避免无谓放大与官方流量的形态差异。

### 第二轮测试

- 新增单元测试：工具按名排序、`tool_choice` 三态（无工具补 none / 有工具不覆盖 / Anthropic 对象形状）、affinity 镜像；工具数断言更新为 6 与 7。
- live 测试新增「无工具请求组」，用真实上游验证 `tool_choice:"none"` 组合。
- 全量 `go test -tags=unit ./...`：仅 `TestOllamaProbeCallback_StaleLongDoesNotOverrideNewShort` 失败（既有失败）。
  - 首轮全量跑还出现一次 `TestGatewayService_StreamingKeepaliveUsesNoopDeltaDuringToolUseForAffectedClaudeCodeVersion` 失败，单独重跑 3 次与全量重跑均通过，确认为**偶发**，与本改动无关（该用例走 Claude Code 路径，不经过 OpenCode 门禁）。

## 门禁规格的来源

规格不是从 CPA 插件照抄的，而是对真实上游做消融实验测出来的。以下每一项都同时有「拒绝」与「放行」两侧证据：

| 要求 | 拒绝侧 | 放行侧 |
| --- | --- | --- |
| `X-Opencode-Session` 形状 | UUID → 403；`abc` → 403；`ses_`+26 字符 → 403 | `ses_`+12hex+14alnum → 200 |
| UA 版本 ≥ 1.18.0 | `opencode/1.17.9` → 426；`opencode/1.0.0` → 426 | `1.18.0` / `1.18.31` / `1.19.0` → 200 |
| `bash` + `read` 同时存在 | 无工具 → 403；仅 `bash` → 403；仅 `read` → 403 | 两者齐备 → 200 |
| `stream: true` | `stream:false` → 403 | `stream:true` → 200 |

另测得 `X-Opencode-Request` / `Client` / `Project` 非必需（各自缺失仍 200），因此未实现，以免无谓放大与官方流量的形态差异。

## 自动验证

### 1. 真实上游端到端联调（最强证据）

`internal/service/openai_opencode_freetier_live_test.go`（`-tags=live`，不进常规 CI）用**实现本身产出的**出站头与 body 直接请求 `https://opencode.ai/zen/v1/chat/completions`：

```
go test -tags=live ./internal/service/ -run TestLiveOpenCodeFreeTierGate -v -count=1
```

结果：**PASS**（7.68s），三个子测试：

- 对照组（无门禁）→ `403 FreeTierError`，证明上游门禁当前确实生效。
- 实验组（经 `applyOpenCodeFreeTierGateBody` + `applyOpenCodeFreeTierGateSession` 处理，模拟既有链路先落一个 UUID 会话值）→ **200**，收到 SSE 帧，且响应不含 `FreeTierError`。
- 边界组（客户端不提供任何会话标识，`http.Header{}` 全空）→ **200**。这一组验证的是实现过程中发现并修复的真实缺口，见下节。

这证明伪装在真实上游生效，而非仅在单元测试里自洽。

## 实现过程中发现并修复的缺口

**Zen 账号在客户端不提供会话标识时不会得到会话头，免费模型必然 403。**

`applyOpenCodeSessionHeader` 只在 `shouldGenerateOpenCodeSession` 为真时才生成兜底会话值，而该条件（`openai_opencode_session.go:91-102`）为：

1. 账号为 Go 订阅（`IsOpenCodeGoPlan()`），或
2. 目标路径含 `/zen/go`

**Zen 账号两条都不满足**——Zen 的 base URL 是 `/zen/v1`，路径里没有 `/zen/go`。因此「Zen 账号 + 客户端未提供 `X-OpenCode-Session`、`prompt_cache_key`、`metadata.user_id`」这一组合下，出站请求完全没有会话头，门禁的第一项硬要求直接不成立。

初版实现只做「把已落地的会话值规范化」，对上述组合无值可规范化，会静默放行一个必然 403 的请求。修复方式：门禁在既有一切来源都缺失时自行派生，顺序为

已落地会话头 → 请求体稳定会话字段 → 模型名 + 首条用户消息的内容派生值 → 随机值兜底

内容派生保证同一会话跨轮次稳定（首条用户消息不变），从而不击穿上游 prompt cache。该缺口已由单元测试的「无会话头时自行补出规范值」「无会话头时按内容派生且跨轮次稳定」与 live 边界组共同覆盖。

## 计费侧：调查结论是「不改动」

### 结论一：查不到价不会拒绝请求（初版提案此处写错）

初版提案写了「`-free` 模型会在计费环节 fail-closed，功能实际不可用」——**这句是错的**。实测与代码核对表明：查不到价不会拒绝请求。

`openai_gateway_usage.go:253-267` 捕获 `ErrModelPricingUnavailable` 后记零成本并继续：

```go
if err != nil {
    if !isUsagePricingUnavailableError(err) {
        return err
    }
    logger.L().Warn("openai_usage.pricing_missing_record_zero_cost", zap.Error(err))
    cost = &CostBreakdown{BillingMode: string(BillingModeToken)}
}
```

同时确认**内置价卡刻意没有通用兜底价**：`getFallbackPricing` 以 `return nil` 收尾（`billing_service.go:1223`），且注释明确「未知型号不回退以避免误计价」（`:1055-1056`）。运营侧的兜底手段是分组/渠道定价的尾随 `*` 通配条目（`model_pricing_resolver.go:161`），pattern `*` 可匹配任意模型。

### 结论二：原状态是「不一致」，不是「不可用」，且不改动

在干净 HEAD（`dc5adafd9`）上用独立工作树实测 11 个官方免费层模型的定价解析：

| 模型 | HEAD 行为 |
| --- | --- |
| `deepseek-v4-flash-free` | **按付费价计费**（input 1.5e-07 / output 6e-07），因含 `deepseek-v4-flash` 子串命中兜底价卡分支 |
| 其余 10 个 | `pricing not found` → 记零成本 + 告警，请求正常服务 |

实测结果是**同一个免费层里有的模型被扣费、有的不扣**，取决于模型名是否恰好包含某个付费家族子串。这是既有行为，**本次不做改动**：定价是资金安全敏感的既有子系统，免费层不是引入新定价语义的正当理由。需要统一口径时用分组/渠道定价（含 `*` 通配）配置。

### 结论三：两次定价尝试均已撤销

设计过程中先后尝试过两种改法：在 `getFallbackPricing` 顶部加「`-free` 一律零价」；以及「剥离 `-free` 后缀后递归走正常定价链」。两者都被否决，共同问题是**擅自新增了一套原代码没有的定价规则**。

其中第二种尝试还引入过一个具体缺陷：为阻止 `deepseek-v4-flash-free` 被 DeepSeek 分支重新计价，曾在 `applyModelSpecificPricingPolicyEx` **顶部**加无条件归零守卫；而该函数同样被分组/渠道自定义定价调用（调用点传 `forceDeepSeekRates=false`，语义为「保留运营者配置」），守卫会把运营者显式配置的价格一并清零。该缺陷由 `TestOpenCodeFreeTierPreservesOperatorPricing` 以 TDD 方式先复现（测试失败：期望 3e-06，实际 0）。

**最终决定：全部撤销。** `billing_service.go` 回到与 HEAD 零 diff，随之新增的定价测试（`opencode_freetier_pricing_scope_test.go` 与 `openai_opencode_freetier_test.go` 中的三个定价用例）一并删除。

### 撤销的验证

- `git diff --stat -- backend/internal/service/billing_service.go` 无输出（零 diff）。
- 免费层门禁相关测试全部通过；定价相关测试已不存在。
- 本能力引入前后，免费层模型的计费结果完全一致。

### 2. 单元测试

```
go test -tags=unit ./internal/service/ -run 'OpenCode|Billing|Pricing' -count=1
```

结果：**ok**（2.919s）。覆盖：

- `canonicalOpenCodeID`：长度恒为 30、`ses_` 前缀、12 位小写 hex、14 位字母数字、幂等、不同来源不碰撞、前缀参与结果、与上游正则一致。
- `isOpenCodeFreeTierModel`：大小写、路由前缀、空白；`deepseek-v4-flash` 等付费 ID 不得误判为免费。
- `shouldApplyOpenCodeFreeTierGate`：缺省 auto × 免费/付费、`always`、`off`、非法值回落、非 OpenCode 平台、空账号。
- body 伪装：三方言形状（chat `function` 嵌套 / responses 扁平 `parameters` / anthropic `input_schema` 无 `type` 包装）、只补缺失、保留客户端工具、不重复注入、`stream:false` 被改写、畸形 body 与 tools 非数组原样返回。
- 会话伪装：合法 `ses_` 原样保留、UUID 被改写、同来源跨轮次稳定、付费模型不改写、无会话头时自行补出规范值、无会话头时按内容派生且跨轮次稳定、付费模型无会话头时不补。
- 定价：**无相关测试**——定价逻辑未改动，随之新增的定价用例已全部移除；`git diff --stat -- backend/internal/service/billing_service.go` 无输出。
- 回归：`auto` 与 `off` 模式下付费模型出站 body 逐字节不变。

### 3. 全量回归

```
go test -tags=unit ./internal/service/... ./internal/handler/... ./internal/server/... -count=1
```

`internal/service` 出现 1 个失败：`TestOllamaProbeCallback_StaleLongDoesNotOverrideNewShort`。**经干净 HEAD 工作树复现**（`git worktree add --detach` 到 `dc5adafd9`），确认为既有失败，与本次改动无关：该测试属 Ollama 429 探针的 CAS 语义，不经过本次改动的任何代码路径。其余各包（`handler`、`handler/admin`、`handler/dto`、`handler/quotaview`、`server`、`server/middleware`、`server/routes`、`openai_ws_v2`）全部通过。

### 4. 静态检查

- `gofmt -l`（12 个改动文件）：无输出。
- `go vet ./internal/service/`：通过。

### 5. 前端

- `vue-tsc --noEmit`：通过。
- `vitest run src/i18n/__tests__/localeKeyCompleteness.spec.ts`：通过（3 个用例），中英文键完整。
- `eslint`（7 个改动文件）：通过。
- 新增 `OpenCodeFreeTierGateSelect.spec.ts`：10 个用例通过（三选项渲染、提示随模式变化、change 事件、未知值回落 auto、凭据助手 create/edit 写入语义与往返一致性）。
- 受影响的既有用例：`CreateAccountModal.spec.ts`、`EditAccountModal.spec.ts`、`credentialsBuilder.spec.ts`、`credentialsBuilder.cnAdaptive.spec.ts` 共 173 个用例全部通过。

## 验证边界

1. **`golangci-lint` 未能运行**。本机未安装，`go install ...@v2.13.0` 因传递依赖 `github.com/ryanrolds/sqlclosecheck@v0.6.0` 校验和与 `sum.golang.org` 不匹配而失败（供应链完整性告警）。**未绕过校验和验证**——绕过模块校验本身就是需要避免的做法。因此 CI 的 lint 关卡未被本地覆盖，`gofmt` 与 `go vet` 是当前可用的替代。
2. **Responses 方言无法真实联调**。`muse-spark-1.3-contributor-free` 与 `mimo-v2.6-flash-free` 在 `/zen/v1/responses` 上返回 `403 RegionError`（"This model is not available in your country."），属地理封锁而非门禁失败。该方言的实现按 Chat 方言同构推导，仅由单元测试覆盖注入形状。
3. **`/zen/go` 付费链路的版本下限未判定**。`public` 密钥在该路径返回 `401 AuthError`，认证先于版本检查失败，故无法确认 426 下限是否同样作用于付费链路。改用 ≥ 1.18.0 的 UA 严格优于原 `opencode/1.0.0`，按缺陷修复处理。
4. **未做真实网关进程联调**。端到端验证直接调用实现函数并打真实上游，未启动完整网关进程（需数据库与 Redis）。三条 funnel 的**接线**由代码审查与既有测试覆盖，未由 live 测试覆盖。
5. **注入工具对模型可见**。`bash` / `read` 会出现在模型上下文中，模型理论上可能发起同名工具调用。默认只作用于 `-free` 模型，`off` 可完全关闭；响应侧过滤未实现，列为后续跟进项。
