# 使用 Magpie 接入 sub2api / Magpie setup guide

[Magpie](https://github.com/yetone/magpie) 可以将模型服务接入 Claude Code、Codex、OpenCode 等客户端。sub2api 的现有兼容接口可作为 Magpie 的自定义服务使用。

## 手动配置（当前版本可用）

1. 在 sub2api 的「API 密钥」页面创建密钥，并为其分配分组。
2. 在 Magpie 中添加自定义服务，填写服务名称、对应协议的 Base URL 和 sub2api API Key。
3. 获取该服务的模型列表，选择一个可用模型，再连接需要使用的客户端。

以下地址以 `https://relay.example` 为服务根地址。管理员配置了 API Base URL 时，应使用该地址；部署在子路径时保留该路径。

| 密钥分组 | 推荐接入协议 | Base URL |
| --- | --- | --- |
| Anthropic | Anthropic Messages | `https://relay.example` |
| Antigravity | Anthropic Messages | `https://relay.example/antigravity` |
| Gemini | OpenAI Chat Completions | `https://relay.example/v1` |
| OpenAI、Grok、Kimi、Zhipu、DeepSeek、MiniMax、OpenCode Go、合成分组 | OpenAI Chat Completions、Responses | `https://relay.example/v1` |

Anthropic 地址不带 `/v1`；OpenAI 地址带 `/v1`。可用模型列表由服务的 `/v1/models` 提供，Antigravity 使用 `/antigravity/v1/models`。Magpie 可为使用其他协议的客户端转换请求；实际可用模型、额度和权限仍由密钥分组与上游账号决定。合成分组还需要管理员配置对应模型路由。

Gemini 分组在此集成中使用 Chat Completions；不要将 `/v1/responses` 配置为其可用协议。需要 Responses 的客户端可由 Magpie 转换到已配置的协议。

仅允许 Claude Code 的分组不适合普通 Magpie 接入：客户端限制仍然有效，应选择允许此类请求的分组。TypeSafe 的 System One 协议不在此接入范围内。

Magpie 要求公网服务使用 HTTPS，仅本机和内网等官方允许的地址可以使用 HTTP。容器部署的 API Base URL 应填写 Magpie 所在电脑可以访问的服务地址。

## 一键导入（待合并功能）

[PR #7830](https://github.com/Wei-Shaw/sub2api/pull/7830) 提供 API 密钥列表的「导入到 Magpie」按钮。安装包含该功能的版本后，点击按钮，在 Magpie 中检查服务地址、密钥并确认添加。导入链接需要 Magpie 0.1.8 或更新版本。

按钮使用站点配置的 API Base URL，未配置时使用当前站点地址。导入不固定模型名称，由 Magpie 获取模型列表。

导入链接包含 API Key，请勿分享或放入公开文档。官方 HTTPS 导入链接把参数放在 fragment（`#` 后），不会作为 URL 查询参数发送到导入网站的服务器。Magpie 会显示将要添加的服务，用户确认后才保存。

官方参考：[Magpie 导入文档](https://usemagpie.ai/docs/import)。相关需求：[Issue #7937](https://github.com/Wei-Shaw/sub2api/issues/7937)。

## English

### Manual setup (available now)

Create a sub2api API key, assign it to a group, and add a custom provider in Magpie using your key and the following endpoint. Preserve any deployment path prefix and use the administrator-configured API Base URL when present.

| Key group | Recommended API | Base URL |
| --- | --- | --- |
| Anthropic | Anthropic Messages | `https://relay.example` |
| Antigravity | Anthropic Messages | `https://relay.example/antigravity` |
| Gemini | OpenAI Chat Completions | `https://relay.example/v1` |
| OpenAI, Grok, Kimi, Zhipu, DeepSeek, MiniMax, OpenCode Go, Composite | OpenAI Chat Completions and Responses | `https://relay.example/v1` |

Discover models from `/v1/models` (`/antigravity/v1/models` for Antigravity), select an available model, and connect your agents in Magpie. Magpie translates requests for other APIs; Gemini groups should not advertise Responses. Models, quota and permissions depend on the group and upstream accounts. Composite groups also require model routing configured by the administrator.

Claude Code-only restrictions still apply; choose a group that permits Magpie requests. TypeSafe System One is outside this integration. Public endpoints must use HTTPS; only supported local endpoints may use HTTP. For container deployments, use an address reachable from the computer running Magpie.

### One-click import (pending)

[PR #7830](https://github.com/Wei-Shaw/sub2api/pull/7830) adds **Import to Magpie** on the API Keys page. Once running a version containing that change, click the button and confirm the endpoint and key in Magpie (0.1.8+). Models are discovered rather than hard-coded.

Import links contain the key; keep them private. The official web link stores parameters in the fragment, which is not sent to the import server as a URL query. Magpie saves the provider only after confirmation. See the [official import documentation](https://usemagpie.ai/docs/import).
