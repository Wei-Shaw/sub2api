# OpenCode Fast Variant 设计

## 背景

当前这套本机 OpenCode 自定义 provider（`sub2api-openai`）与 `sub2api` 前端导出的推荐配置，已经支持：

- `gpt-5.4`
- `gpt-5.4-Sys`
- 各类 reasoning variants（如 `low`、`medium`、`high`、`xhigh`）

但它们还没有提供“Fast 模式”入口。

在这条线里，有一个容易混淆但已经通过官方 Codex 开源代码确认的事实：

1. 在官方 Codex 配置语义层里，Fast 模式可以表现为 `ServiceTier::Fast`。
2. 但在官方 Rust CLI 真正构造发给 Responses API 的请求时，`ServiceTier::Fast` 最终会被映射成 wire-level 的 `service_tier = "priority"`，而不是字面量 `fast`。

因此，当前我们要补的不是“把 `fast` 原样塞进请求”，而是：

- 在用户入口层提供清晰的 Fast 变体；
- 在真正落线时使用正确的 `serviceTier: "priority"`；
- 同时保留现有的 reasoning effort 组合能力。

## 目标

本次要完成 3 件事：

1. 给本机 `~/.config/opencode/opencode.jsonc` 里的 `sub2api-openai` provider 增加 Fast 组合 variant。
2. 给 `sub2api` 前端导出的 OpenCode 推荐配置增加同样的 Fast 组合 variant。
3. 保证用户侧看到的是“Fast + 推理强度”的组合语义，但真正发往线上的是 `serviceTier: "priority"`，不是 `fast`。

## 非目标

本次明确不做以下事情：

1. 不修改 `sub2api` 后端对 `service_tier` 的抽取/归一化逻辑。
2. 不把 Fast 支持扩展到所有模型，只处理 `gpt-5.4` 和 `gpt-5.4-Sys`。
3. 不新增新的模型 ID（例如 `gpt-5.4-fast`）。
4. 不在这一轮引入新的 OpenCode provider。

## 行为契约

### 1. 用户入口形态

本次采用“组合 variant”方案，不新增新模型名。

只对以下两个模型提供新的 Fast 组合 variant：

- `gpt-5.4`
- `gpt-5.4-Sys`

新增的 variant 名字为：

- `fast-low`
- `fast-medium`
- `fast-high`
- `fast-xhigh`

这些 variant 的用户语义是：

- `fast-*` = Fast 模式
- `low/medium/high/xhigh` = reasoning effort

### 2. 请求落线语义

这些 Fast 组合 variant 在真正生成 OpenCode provider 配置时，不应发送：

```json
"serviceTier": "fast"
```

而应发送：

```json
"serviceTier": "priority"
```

并同时保留对应的：

```json
"reasoningEffort": "low|medium|high|xhigh"
```

也就是说：

- 用户配置语义：Fast
- wire-level 请求语义：priority

## 实现边界

### 1. `sub2api` 前端推荐配置

`sub2api` 前端当前已经有一套集中生成 OpenCode OpenAI 模型配置的函数。

本次继续沿用这套“单一模型清单源”思路，不再分散手写第二套 Fast 配置。

实现上：

1. 在现有 `gpt-5.4` / `gpt-5.4-Sys` 模型配置上派生新的 `fast-*` 组合 variant。
2. 这些 variant 的 payload 必须显式包含：
   - `serviceTier: "priority"`
   - 对应的 `reasoningEffort`
   - 保持与现有 variants 一致的 `reasoningSummary` / `include` 等必要字段。

### 2. 本机 OpenCode 配置

本机 `~/.config/opencode/opencode.jsonc` 中的 `sub2api-openai` provider，要与前端推荐配置保持一致。

本次只补：

- `gpt-5.4`
- `gpt-5.4-Sys`

及其 `fast-low/fast-medium/fast-high/fast-xhigh`。

### 3. 后端范围

这轮不改 `sub2api` 后端 `service_tier` 归一化逻辑。

原因是：

1. 官方客户端的真实 wire-level 请求本来就不会把 `fast` 直接送进来。
2. 本次重点是把 OpenCode 配置入口改成与官方 wire 语义一致。

如果后续要兼容“非官方客户端手工发 `fast`”这种请求，那是单独的后端兼容议题，不在这份设计里处理。

## 风险与验证

### 风险 1：组合 variant 命名不被 OpenCode 识别

这次引入的是新的命名：

- `fast-low`
- `fast-medium`
- `fast-high`
- `fast-xhigh`

如果 OpenCode 对 variant 命名有隐藏约束，可能出现：

- provider 能加载，但 variant 不生效；
- 或模型列表显示正常，但请求 payload 没带上对应字段。

### 风险 2：`serviceTier` 字段层级放错

如果 `serviceTier: "priority"` 没放在 OpenCode provider 期望的位置，可能出现：

- 配置文件看上去有 Fast 变体；
- 实际请求仍然没带 `priority`；
- 线上依然无法复现正确的 Fast 兼容行为。

## 验证方案

验证分 3 层：

### A. 本机配置结构验证

确认本机 `opencode.jsonc` 中：

- `sub2api-openai/gpt-5.4`
- `sub2api-openai/gpt-5.4-Sys`

都能看到：

- `fast-low`
- `fast-medium`
- `fast-high`
- `fast-xhigh`

并且这些 variant 里包含：

- `serviceTier: "priority"`
- 对应 `reasoningEffort`

### B. `sub2api` 前端推荐配置验证

通过测试锁定：

1. 推荐配置会输出这组新的 `fast-*` variants。
2. 生成结果里 wire-level 字段是 `serviceTier: "priority"`，不是 `fast`。

### C. 真实请求验证

如果前两层都通过，再用新的 Fast variant 发一条真实请求，确认：

1. 不再出现 `Unsupported service_tier: fast`。
2. reasoning effort 仍按 variant 指定值生效。

## 最终结论

本次设计选择：

1. 不新增新模型 ID；
2. 只给 `gpt-5.4` / `gpt-5.4-Sys` 增加 `fast-*` 组合 variant；
3. 用户入口层仍然使用 Fast 语义；
4. wire-level 统一发 `serviceTier: "priority"`；
5. reasoning effort 与 Fast 同时组合表达；
6. 先改 OpenCode 配置入口，不碰后端 `service_tier` 归一化逻辑。

这样既能让本机配置和 `sub2api` 推荐配置保持一致，也能避免继续把官方配置语义层的 `fast` 错误地下沉为线上请求体的字面量 `fast`。
