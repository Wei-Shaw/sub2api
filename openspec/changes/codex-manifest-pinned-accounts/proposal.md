## Why

OpenAI 分组在没有账号模型映射时，每次 Codex 客户端请求 Model Manifest 都会经由调度器挑选一个账号向上游拉取。调度结果受优先级、负载因子和限流窗口影响，而分组内账号的模型权限并不一致（例如部分账号拥有 Trusted Access for Cyber 的特殊模型），导致同一个 API Key 前后看到的模型列表不确定。此外 OAuth 账号的 manifest 拉取完全没有缓存，每次客户端刷新都直接打到 chatgpt.com。

## What Changes

- 为平台为 OpenAI 的分组新增「使用特定账号获取 Codex Model Manifest」配置：一个默认关闭的开关、一个必须至少选择一个账号的多选账号列表，以及「选定账号全部不可用时回退调度器」的子选项（默认关闭，即返回 503）。
- 开关打开后，该分组的 API Key 请求 Codex Model Manifest 时只使用选定账号向上游拉取，忽略优先级、负载因子、限流与过载窗口；多个账号并发请求，`models` 按 slug 取并集后返回。部分账号失败时以成功部分合并；全部失败按配置返回 503 或回退到现有调度器路径。
- 管理端分组编辑对话框的 OpenAI 区块新增该配置的开关与带搜索的多选账号下拉，账号来源限定为当前分组内的 OpenAI 账号。创建对话框不展示该配置（分组创建时尚无账号绑定）。
- 分组复制时该配置重置为关闭。
- 普通 `/v1/models` 与 `/models`（无非空 `client_version`）复用同一固定账号配置；API Key 请求标准上游模型列表，OAuth 从现有 Codex manifest 提取 ID，输出 OpenAI 列表。
- **BREAKING**：固定账号开启时，普通列表和 Codex manifest 都优先进行指定账号发现，随后按来源账号模型映射生成公开目录并应用分组过滤；显式模型映射不再短路固定账号发现。
- **BREAKING**（行为层面）：Codex Model Manifest 缓存统一为 1 分钟新鲜（命中即返回，不请求上游）、1 到 5 分钟乐观返回缓存并后台刷新、超过 5 分钟同步等待上游刷新后再返回。缓存覆盖范围从仅 API Key 账号扩展到 OAuth 账号，固定账号模式的每个账号也走同一缓存。

## Capabilities

### New Capabilities
- `codex-manifest-pinned-accounts`：OpenAI 分组的固定账号 Manifest 配置（数据模型、管理端校验与 UI）、运行时的固定账号并发拉取与合并、不可用时的回退策略。
- `codex-manifest-cache`：Codex Model Manifest 的按账号缓存策略：新鲜期、乐观期与强制刷新期的行为，以及对所有账号类型生效。

### Modified Capabilities
<!-- openspec/specs 目前为空，没有既有能力需要修改。 -->

## Impact

- **数据库**：`groups` 新增 JSONB 列 `codex_models_manifest_config`，新增迁移文件；ent schema 与生成代码更新。
- **后端**：`Group` 领域模型、分组仓储的创建与更新、API Key 加载分组的字段投影、认证快照缓存、管理端分组 DTO 与校验、分组复制逻辑；`openai_codex_models_handler.go` 新增固定账号分支；`openai_codex_models_service.go` 新增固定账号并发拉取与合并、缓存策略调整并扩展到 OAuth 路径。
- **管理端 API**：`POST/PUT /admin/groups` 请求与响应新增 `codex_models_manifest_config` 字段；`GET /admin/accounts` 已支持按分组过滤，无需改动。
- **前端**：`GroupsView.vue` 编辑对话框 OpenAI 区块、`types/index.ts`、中英文 i18n。
- **上游影响**：固定账号模式下每次缓存失效会向 N 个账号并发请求；缓存新鲜期延长到 1 分钟后，稳态上游请求量低于当前。
