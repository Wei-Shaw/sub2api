# 跨厂商会话与 Composite（mix）目标路由

## 本次改动

本功能包含两项改动：

1. 修复 Anthropic Messages → OpenAI Responses 的历史思考状态转换，以及明确的加密内容拒绝后的有限恢复。
2. 扩展现有 Composite 模型路由，支持指定普通账号组或单个账号，不改变调用方的计费和权限身份。

配置这些路由前，需要部署包含本功能的服务版本并完成数据库迁移。测试应使用独立环境和模拟凭据，不要直接在生产实例上试验。

## 1. 为什么同一会话切换厂商会报错

客户端可能一直发送 `claude-opus-5-5`，网关却把它映射到 `gpt-6.1-sol`。原历史中的 Anthropic `thinking.signature` 不能直接当作 OpenAI `reasoning.encrypted_content` 提交。

旧转换策略过滤 `gAAAA…`、转发其他非空签名，可能将原生 Claude 思考状态提交给 OpenAI。现在，实际账号平台决定重放策略，而不是客户端显示模型名：

- OpenAI：保留可识别的历史 `gAAAA…` 兼容候选，省略原生 Claude、已知 Anthropic 桥接封装和未知密文。
- Grok：保留既有的 xAI 兼容行为，同时排除可识别的 OpenAI／Claude 状态。
- 没有明确支持重放策略的其他 Responses 目标：默认不转发未知加密思考。
- 原生 Claude 转发：移除明确属于 OpenAI 的 `gAAAA…` 思考块，保留原生思考、正文和工具历史。

前缀只是兼容性启发式，**不是经过认证的来源证明，也不保证同厂商跨账号／跨模型密文可用**。上游仍是密文有效性的最终验证者。

`/v1/messages → /v1/responses` 新增一次恢复：明确的 `invalid_encrypted_content`（或严格匹配的既有签名错误）返回 HTTP 400／502 时，仅清理加密 reasoning 项重试一次。若错误发生在 Responses SSE 中，也只能在尚未向客户端提交内容时恢复。重试保留会话／Codex 身份头、工具调用及结果，不删除 compaction／compaction_summary。已经输出内容的流不能安全重放；第二次拒绝按正常错误处理。

隐藏思考显示、降低 effort、改模型显示名称都不等于清除历史密文。

## 2. 路由的三种目标

在后台 **分组管理 → Composite 类型分组 → 路由**，为一条客户端模型规则选择：

| 目标模式 | API 字段 | 行为 |
| --- | --- | --- |
| 当前 mix 账号池 | 两个目标 ID 都为空 | 保持原有行为，在源 mix 内按目标厂商调度 |
| 指定账号组 | `target_group_id` | 只在该普通账号组中选择合格账号；可以在组内故障转移 |
| 指定账号 | `target_account_id` | 只使用这一账号；不可用时明确失败，绝不自动换到其他账号 |

目标组和目标账号互斥，不能同时指定。旧规则没有新字段时保持兼容；PUT 中显式传两个 `null` 可以清除原有目标限制。

### 约束

- 目标组必须是存在、启用、与 `target_platform` 匹配的普通组；不能是自身或另一个 Composite 组，首版不支持嵌套 mix。
- 目标账号必须存在并匹配目标平台。管理员配置的规则允许选择源 mix 之外的账号。
- 指定目标并不绕过账号状态、模型支持、冷却、额度、并发、上游协议、源组隐私或利润准入检查。
- 目标被删除、禁用或改变平台时失败关闭，不会退回更宽的源 mix／全局账号池。
- 目标组只提供**账号池**。调用方的 API Key、原 mix 组、订阅、用户配额、计费倍率和使用记录组归属不变；不会借用目标组的订阅或身份。
- 同一会话的不同模型／路由使用隔离的粘性调度键，避免主代理和子代理的账号绑定相互覆盖。
- 使用现有厂商适配器：注册了厂商不代表每种模型、凭据和协议组合都受支持；本次不新增不存在的协议桥。
- 新目标作用域面向对话路由。异步 `/videos` 请求暂不支持跨组／指定账号目标：后续状态／内容查询没有模型信息可重新解析授权，创建入口会明确返回 400 `composite_async_destination_unsupported`，而不是创建后无法查询。原有不指定新目标的视频路由不变。

## 3. 主代理与子代理配置示例

下面只是配置示例，不会自动创建账号或修改当前后台。请使用实际账号、组和模型 ID。

| 客户端请求模型 | 目标厂商 | 目标资源 | 上游模型示例 | 用途 |
| --- | --- | --- | --- | --- |
| `claude-opus-5-5` | OpenAI | 指定 OpenAI 普通组 | `gpt-6.1-sol` | 主代理或复杂任务 |
| `claude-sonnet-4-6` | Anthropic | 指定 Claude 普通组 | 该组支持的 Claude 模型 | 代码执行子代理 |
| `claude-haiku-4-5-20251001` | DeepSeek | 指定 DeepSeek 账号 | 账号实际支持的模型 | 简单检索／整理子代理 |

Claude Code 中应让不同子代理实际请求不同模型；以真实请求中的 `model` 为准。

**路由按模型和端点匹配，不会自动识别“主代理／子代理角色”。** 主代理和子代理若发送相同模型 ID 和端点，会命中同一条规则。网关不会替你创建、编排或分配子代理。

规则匹配仍沿用原有优先级：精确优先、端点特定规则优先、更长前缀优先，然后按 priority 和 ID 排序。界面预览使用已保存规则，不是尚未保存的表单。

### 管理 API 示例

使用既有管理 API，鉴权方式不变：

`POST /api/v1/admin/groups/<mix_id>/composite-routes`

```json
{
  "public_model": "claude-opus-5-5",
  "match_type": "exact",
  "target_platform": "openai",
  "target_group_id": 20,
  "target_account_id": null,
  "upstream_model": "gpt-6.1-sol",
  "endpoint": "messages",
  "priority": 100,
  "enabled": true,
  "notes": "主代理走 OpenAI 目标账号组"
}
```

指定账号时改用 `target_account_id`，并将 `target_group_id` 设为 `null`。读取、更新、删除和预览仍使用既有 composite-routes 接口。

## 4. 部署与回滚

### 部署前

1. 备份 PostgreSQL 数据库，并保留当前镜像或二进制、环境配置和部署编排文件。
2. 先在独立测试实例验证：旧规则无变化、新规则目标正确、工具调用正常、源组计费不变。
3. 本仓库要求 Go **1.27.2**；前端使用 pnpm 9 的锁文件。
4. 确认待部署镜像或二进制确实包含本功能。仅更新后台配置不会使旧版本获得新目标路由能力。

### 构建

从仓库根目录执行前端生产构建：

```bash
pnpm --dir frontend install --frozen-lockfile
```

```bash
pnpm --dir frontend run build
```

从 `backend` 目录编译包含前端资源的服务：

```bash
go build -tags embed -o bin/sub2api ./cmd/server
```

也可以按仓库现有 Dockerfile 构建自己的镜像；不要直接覆盖运行容器的数据目录。

### 数据库迁移

新增 `backend/migrations/247_composite_route_destinations.sql`，由正常启动迁移流程执行：

- 新增两个可空目标 ID；旧数据保持为空。
- 数据库校验互斥、正数和禁止自指。
- 外键使用 `ON DELETE RESTRICT`，不会因为删除目标就把显式规则变成无限制账号池。
- 目标资源删除前，应先清理或调整引用规则。软删除目标后，运行时仍会拒绝不可用目标。

不要在未备份的生产库中反复手动执行迁移文件。多实例部署应遵循仓库既有迁移／升级流程。

### 回滚

- 首先停用新规则／切回原有 pool 规则，再回滚服务镜像或二进制。
- 新增列是可空的，旧服务不会识别新目标约束，**旧服务仍会忽略这些目标字段，按旧账号池行为执行**。因此不能只回滚程序却留下仍被使用的显式目标规则。
- 保留备份；不要为了程序回滚立即删除列或外键。需要回滚数据库时，在维护窗口制定独立迁移方案。

## 5. 验证入口

本次包含转换策略、HTTP／SSE 有限重试、工具结构、源组计费、账号组／账号固定调度、粘性隔离、旧行为兼容、管理 CRUD 和数据库字段回归测试。

从 `backend` 目录运行相关回归：

```bash
go test ./internal/pkg/apicompat
```

```bash
go test -tags unit ./internal/service ./internal/repository ./internal/server/routes ./internal/handler/admin ./migrations -run 'Composite|SchedulingScope|ForwardAsAnthropic|MessagesReasoningReplay|DropClearlyForeignAnthropicThinking|GatewayForward_NativeAnthropic' -count=1
```

从仓库根目录检查前端：

```bash
pnpm --dir frontend run lint:check
```

```bash
pnpm --dir frontend run typecheck
```

```bash
pnpm --dir frontend exec vitest run src/views/admin/__tests__/GroupsView.compositePlatforms.spec.ts src/i18n/__tests__/localeKeyCompleteness.spec.ts
```

测试环境结果与验证范围见 [验证说明](cross-provider-verification.zh-CN.md)。模拟上游测试通过并不代表所有真实厂商、模型及账号组合均已联网验证。
