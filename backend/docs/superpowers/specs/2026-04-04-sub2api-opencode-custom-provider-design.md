# sub2api OpenCode 独立 Provider 与 Sys 模型设计

## 背景

当前 `sub2api` 前端里的 OpenCode 配置示例会直接生成内置 `openai` provider 的配置。这会带来两个问题：

1. 会占用用户现有的官方 `openai` provider，和“保留原始 OpenAI provider 不动”的诉求冲突。
2. 示例模型列表没有显式提供 `-Sys` 后缀模型，无法直接匹配 `sub2api` 已支持的 Sys 路由别名能力。

同时，OpenCode 官方文档已经明确了一个约束：

- 内置标准 provider（如 `openai`）会自动从 models.dev 获取模型元数据。
- 自定义 provider 需要在 `provider.<id>.models` 中自行声明模型与 `limit` 信息。

因此，这次设计不尝试伪造“自定义 provider 自动继承 models.dev”，而是优先满足“独立 provider”这一更高优先级需求。

## 目标

本次要实现两个一致的结果：

1. 用户本机 OpenCode 配置中新增一个独立 provider，例如 `sub2api-openai`，其请求目标是 `https://oai.jwyihao.top/v1`，并使用用户提供的 API key。
2. `sub2api` 前端导出的 OpenCode 示例配置也改为输出同名独立 provider，而不是继续复用内置 `openai` provider。

额外要求：

- OpenCode 示例和本机配置都必须提供 `-Sys` 版本模型。
- 不改动用户现有的内置 `openai` provider。
- 第一版先做稳定可用，不追求自动同步 models.dev。

## 非目标

以下内容不在本次范围内：

1. 为自定义 provider 增加自动从 models.dev 拉取最新模型的机制。
2. 修改 OpenCode 上游实现，让自定义 provider 共享内置 `openai` provider 的模型注册表。
3. 改动 `sub2api` 的路由语义、计费语义或 provider 鉴权逻辑。
4. 为 Anthropic/Gemini/Antigravity 的 OpenCode 示例同步引入同样的独立 provider 策略。

## 设计概览

### 1. OpenCode 本机配置

直接修改本机配置文件：

- 路径：`~/.config/opencode/opencode.jsonc`
- 新增 provider id：`sub2api-openai`
- `options.baseURL`：`https://oai.jwyihao.top/v1`
- `options.apiKey`：用户提供的 `sk-71c02f94211ea843311ab07e497ea141b987e44a042837ef5ced5c629d29e7c9`
- `npm`：`@ai-sdk/openai`

模型引用形态将变成：

- `sub2api-openai/gpt-5.4`
- `sub2api-openai/gpt-5.4-Sys`
- `sub2api-openai/gpt-5.3-codex`

这样的好处是：

- 和现有内置 `openai` provider 完全隔离。
- 不影响当前已经在 OpenCode 中使用的其他 provider，例如 `github-copilot`。
- 可以明确把 `sub2api` 当成一个单独来源来选择模型。

### 2. sub2api 的 OpenCode 示例生成

当前仓库中 OpenCode 示例来源位于：

- `frontend/src/components/keys/UseKeyModal.vue`

OpenAI 平台的 OpenCode 示例当前走的是：

- `generateOpenCodeConfig('openai', apiBase, apiKey)`

本次要把它改成：

- 输出独立 provider id `sub2api-openai`
- 输出同一套手工维护的模型清单
- 在普通模型后补齐对应 `-Sys` 别名模型

这样用户从前端复制出来的配置，将与本机最终落地的 OpenCode 配置保持同一种结构，而不是出现“前端示例还是 `openai`，本机实际配置却是自定义 provider”的割裂状态。

### 3. 模型清单组织方式

为了避免 OpenCode 示例中的 OpenAI 模型定义散落在多个地方，本次把模型清单收敛到单一来源。

建议做法：

1. 保留一份基础 OpenAI/Codex 模型定义列表。
2. 由同一处生成逻辑派生出 `-Sys` 版本模型。

至少覆盖当前用户明确需要的这一批模型：

- `gpt-5-codex`
- `gpt-5.1-codex`
- `gpt-5.1-codex-max`
- `gpt-5.1-codex-mini`
- `gpt-5.2`
- `gpt-5.4`
- `gpt-5.4-mini`
- `gpt-5.4-nano`
- `gpt-5.3-codex-spark`
- `gpt-5.3-codex`
- `gpt-5.2-codex`
- `codex-mini-latest`

并为每个适用模型额外生成：

- `<model>-Sys`

例如：

- `gpt-5.4` -> `gpt-5.4-Sys`
- `gpt-5.3-codex` -> `gpt-5.3-codex-Sys`

显示名也同步追加 `(Sys)`，例如：

- `GPT-5.4` -> `GPT-5.4 (Sys)`

## 配置策略

### Provider 名称

配置中的 provider key 使用：

- `sub2api-openai`

UI 中显示名使用：

- `sub2api OpenAI`

这样既容易识别，也避免与官方 `OpenAI` provider 混淆。

### API Key 写法

第一版直接写入本机配置文件中的 `options.apiKey`。

原因：

1. 当前目标是尽快形成一个稳定可用的独立 provider。
2. 这是用户本机的用户级配置，不属于仓库内共享配置。
3. 相比 `/connect` 或环境变量，这样最少变动、最容易验证是否真正生效。

后续如果用户希望降低本地明文存储风险，再单独切换到：

- `/connect` 管理凭据，或
- `options.apiKey: "{env:...}"`

本次不把这个切换纳入范围。

## 需要修改的位置

### 本机环境

- `C:\Users\34404\.config\opencode\opencode.jsonc`

### sub2api 仓库

- `frontend/src/components/keys/UseKeyModal.vue`
- `frontend/src/components/keys/__tests__/UseKeyModal.spec.ts`
- `frontend/src/i18n/locales/zh.ts`
- `frontend/src/i18n/locales/en.ts`

如果当前 OpenCode OpenAI 模型清单是写在 `UseKeyModal.vue` 内部的，则本次应顺手抽成同文件内单一常量或局部 helper；如果已有独立常量来源，则复用已有来源，不做无关重构。

## 验证方案

### A. 本机配置验证

验证点：

1. `opencode.jsonc` 中存在 `provider.sub2api-openai`
2. `baseURL` 指向 `https://oai.jwyihao.top/v1`
3. `apiKey` 已更新为新 key
4. 模型列表中同时包含普通模型与 `-Sys` 模型

如果 OpenCode 本机有模型/提供商查看命令，可额外做一次读取验证。

### B. sub2api 前端测试验证

新增或更新前端测试，锁定以下行为：

1. OpenCode 示例输出的 provider key 不再是 `openai`，而是 `sub2api-openai`
2. OpenCode 示例中包含 `gpt-5.4-Sys` 等 `-Sys` 模型
3. 普通模型与 `-Sys` 模型的显示名正确

### C. 前端构建验证

执行前端相关验证，确保示例修改不会打坏复制配置功能或 UI 文案：

- 定向 vitest
- `pnpm build`

## 风险与取舍

### 风险 1：模型清单会过期

这是本方案已接受的 trade-off。因为用户优先级是“独立 provider”，不是“自动继承 models.dev”。

缓解方式：

- 让模型清单集中在一处，后续只改一个来源

### 风险 2：示例配置与本机配置再次漂移

缓解方式：

- 本次就把 `sub2api` 前端示例改成和本机落地结构一致
- 不再继续输出占用官方 `openai` provider 的旧示例

### 风险 3：`-Sys` 模型过多导致示例冗长

这是可接受成本。因为 `-Sys` 是当前 `sub2api` OpenAI 路由的明确能力，示例里不提供会直接降低可用性。

## 最终结论

第一版采用：

- 独立 provider `sub2api-openai`
- 本机配置直接写入 `apiKey`
- `sub2api` 前端 OpenCode 示例同步改成独立 provider
- 普通 OpenAI/Codex 模型 + `-Sys` 别名模型一起输出
- 不尝试自动继承 models.dev

这能最小成本满足以下 4 件事：

1. 不占用官方 `openai` provider
2. OpenCode 本机立即可用
3. `sub2api` 前端示例和本机实际配置一致
4. `-Sys` 模型能直接被 OpenCode 选择和使用
