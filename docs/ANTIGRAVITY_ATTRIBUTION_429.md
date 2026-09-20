# Antigravity：Claude Desktop attribution 元数据触发 429 的排查记录

本文记录一次实际问题的定位和修复，适用于 Claude Desktop / Claude Code 经 Anthropic Messages 协议转到 Antigravity 的场景。它不是所有 `429 RESOURCE_EXHAUSTED` 的通用解释：额度耗尽、请求频率、上游容量等仍应独立排查。

## 现象与环境

- 实测部署为 Sub2API 0.2.0，Claude Desktop 2.110.0，内嵌 Claude Code 2.1.271，CC Switch 3.20.3。
- 请求链路：Claude Desktop → CC Switch 本地代理 → Sub2API Antigravity → Google 上游；映射模型为 `gemini-3.8-flash-high`。
- 新对话中的简单问候也返回 `429 RESOURCE_EXHAUSTED`，上游信息为 `Resource has been exhausted (e.g. check quota).`。
- 随后账号进入模型冷却，单账号组还可能出现 `503 No available accounts`。
- 独立构造的极简请求成功，但真实客户端请求失败。客户端即使只发送一句问候，也会附带 system 提示词和工具定义。

## 定位方法与对照结果

保留一次失败请求，在相同账号、模型和上游端点下逐项删减内容。不要仅用手写的短请求判断完整客户端链路已经恢复，也不要在公开 issue / PR 上传原始请求或访问令牌。

| 对照项 | 本次观察 |
| --- | --- |
| 原始客户端请求（约 187 KB、106 个工具） | 429 |
| 移除工具定义，保留 system 内容 | 429 |
| 移除 messages 中的 system 角色消息 | 429 |
| 直接请求 Google，保留原始顶层 system 内容 | 429 |
| 同一 Google 请求改用简单 system 内容 | 200 |
| 分别测试 system 文本块 | attribution 元数据块返回 429，其余指令块返回 200 |
| 原始请求仅移除 attribution 元数据块，保留模型、工具及其他内容 | 200，收到完整回复 |

触发问题的文本形如：

```text
x-anthropic-billing-header: cc_version=2.1.271.4bf; cc_entrypoint=claude-desktop-3p;
```

这里的 “header” 是请求 JSON 的 `system` 文本中的一行，**不是 HTTP 请求头**。因此，不需要在 Claude Desktop 的 “Custom inference headers” 里增删同名字段；也不应通过修改正常工作的 Bearer 认证来处理这个问题。

这些对照支持“该元数据文本在本次 Google 请求中触发拒绝”的判断，但不能据此确定 Google 内部策略，也不能推断所有账号或模型都具有相同行为。

## 本次修复

在 Antigravity 的 Claude → Gemini 请求转换中，移除顶层 `system` 字符串或文本块**开头**的 `x-anthropic-billing-header:` 行：

- 支持字符串和文本块数组，兼容 LF / CRLF / CR 换行及前导空白。
- 如果同一文本块后面还有指令，原样保留后续内容；仅元数据的空块不再发送。
- 不匹配缺少冒号的普通文本、其他字段名，以及正常指令中间引用的相同文本。
- 不修改用户消息、工具定义、模型映射、认证和账号额度状态。
- 处理限于 Antigravity 转换器，不修改原生 Anthropic 转发路径。

上线前的临时修复采用相同范围的反向代理 JSON 过滤，并通过原始完整请求和真实 Claude Desktop 会话验证成功。本文 PR 将处理放入转换器，避免用户必须自行维护代理脚本。原部署曾尝试降低账号并发，但未解决问题；定位后已恢复原配置。

如需在旧版本临时处理，应只对目标 Antigravity Messages 路径修改 JSON 的顶层 system 文本。不要用全局字符串替换，否则可能误改消息、工具参数或代码示例。代理实现还需保持空数组和空对象类型、更新请求体长度，并覆盖实际使用的 token-count 路径。面板重新生成代理配置时也可能覆盖自定义过滤规则。

## 不要全局禁用 attribution

Claude Code 提供 [`CLAUDE_CODE_ATTRIBUTION_HEADER`](https://code.claude.com/docs/en/env-vars) 环境变量，但桌面端是否继承该配置需要实测。本次仅修改全局 Claude 设置并未消除 Desktop 请求中的元数据。

更重要的是，原生 Anthropic OAuth 线路有相反的报告：禁用 attribution 会造成 429，见 [issue #6344](https://github.com/Wei-Shaw/sub2api/issues/6344)。因此，不建议为这个 Antigravity 问题统一设置 `CLAUDE_CODE_ATTRIBUTION_HEADER=0` 或在所有上游请求中删除该文本。

## 验证与仍需排查的情况

回归测试覆盖两种 system 格式、换行与空白、保留后续指令、保留普通文本和用户 / 工具内容，以及不修改原请求。已有的无冒号 billing-header 文本保留测试继续适用。

修复后应使用真实客户端新建会话验证完整响应，并确认已有冷却已到期。如果仍返回 429，继续检查原始上游错误、实际额度、并发和模型映射；不要用无限重试或反复清空冷却替代定位。
