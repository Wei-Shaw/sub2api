# Sub2API 订阅账号转 API、LiteLLM 接入与高可用分析

> 分析对象：当前 Sub2API 仓库代码  
> 分析方法：静态代码审查与 LiteLLM 官方文档核对  
> 注意：本报告没有使用真实订阅账号向上游发送请求。

## 摘要

1. **Sub2API 不是把订阅账号提取或兑换成普通官方 API Key。**它保存订阅账号的 OAuth `access_token`、`refresh_token` 等凭据，在收到请求后选择账号、刷新令牌、改写请求，并调用 Anthropic 官方推理端点或 OpenAI 的 ChatGPT/Codex 后端端点。
2. **将 Sub2API 接入 LiteLLM 在技术上可行。**推荐链路是：`用户 → LiteLLM → Sub2API → Anthropic/OpenAI`。
3. 用户可以只访问 LiteLLM，但**底层推理请求仍然需要经过 Sub2API**。LiteLLM 不会因为保存了 Sub2API Key，就自动获得或接管底层订阅账号的 OAuth 凭据。
4. **Sub2API 订阅模式与直接使用官方 API 并不完全等价。**基础聊天、流式输出、工具调用通常可以兼容，但认证方式、上游端点、计费、配额、错误、模型映射、部分 beta 功能和稳定性语义存在差异。
5. **Sub2API 可以建设成高可用系统，但当前版本不能直接把同一个完整单体镜像无约束地扩成多个副本。**核心 API 数据面具备多实例基础，但 OAuth 临时会话、部分定时任务、渠道监控、备份恢复和部分 WebSocket 状态仍存在单机假设。

---

# 1. Sub2API 如何把订阅账号变成 API

## 1.1 支持的账号类型

代码定义了以下账号类型：

- `oauth`：完整 OAuth 账号；
- `setup-token`：仅推理权限的 OAuth Token；
- `apikey`：真正的官方或兼容 API Key；
- `upstream`：使用 Base URL 和 API Key 连接的自定义上游；
- `bedrock`：AWS Bedrock；
- `service_account`：例如 Google Vertex AI Service Account。

定义见：

- `backend/internal/domain/constants.go:19-37`

因此，需要区分两种本质不同的运行模式。

### 模式 A：真正的 API Key 代理

```text
用户 → Sub2API Key → Sub2API → 官方 API Key → 官方 API
```

这种情况下，Sub2API 是传统 API 网关、账号池和计费系统。

### 模式 B：订阅/OAuth 账号转换

```text
用户 → Sub2API Key → Sub2API
                       ├─ 选择订阅账号
                       ├─ 获取或刷新 OAuth access_token
                       ├─ 改写请求、模型和客户端特征
                       └─ 调用该订阅账号对应的推理端点
```

这是 Sub2API 将订阅账号能力转成统一 API 的核心模式。

---

## 1.2 Claude 订阅账号

### OAuth 授权参数

Claude OAuth 参数定义在：

- `backend/internal/pkg/oauth/oauth.go:16-34`

其中包括：

- 授权地址：`https://claude.ai/oauth/authorize`
- Token 地址：`https://platform.claude.com/v1/oauth/token`
- 回调地址：`https://platform.claude.com/oauth/code/callback`
- 使用 PKCE；
- 支持 `user:inference`、`user:sessions:claude_code` 等权限。

### 账号绑定流程

```text
管理员发起 Claude OAuth
  ↓
Sub2API 生成 state、PKCE verifier 和 challenge
  ↓
管理员在 claude.ai 完成授权
  ↓
Sub2API 使用 authorization code 换取：
  - access_token
  - refresh_token
  - expires_at
  - organization/account 信息
  ↓
将这些数据写入账号 credentials
```

相关实现：

- `backend/internal/service/oauth_service.go:64-120`
- `backend/internal/service/oauth_service.go:143-172`

项目还支持使用 Claude 网站的 `sessionKey` 自动完成授权：

1. 从 `claude.ai` 获取组织信息；
2. 使用 cookie/sessionKey 获取 authorization code；
3. 将 code 交换成 OAuth Token。

相关实现：

- `backend/internal/service/oauth_service.go:175-238`

### 它是否生成了普通官方 API Key

没有。虽然 OAuth scope 中出现 `org:create_api_key`，但该流程最终保存的是 OAuth Token：

- `access_token`
- `refresh_token`
- `expires_at`
- 组织和账号信息

Token 响应结构见：

- `backend/internal/pkg/oauth/oauth.go:185-205`

因此，不能将此机制理解为“从 Claude 订阅中提取 Console API Key”。

### 实际调用 Claude 上游

默认推理端点是：

```text
https://api.anthropic.com/v1/messages?beta=true
```

见：

- `backend/internal/service/gateway_service.go:30-33`

OAuth 账号使用：

```http
Authorization: Bearer <access_token>
anthropic-version: 2023-06-01
```

真正的 API Key 账号则使用 Anthropic API Key 认证头。相关请求构造逻辑：

- `backend/internal/service/gateway_upstream_request.go:21-48`
- `backend/internal/service/gateway_upstream_request.go:119-175`

因此，Claude 订阅路径虽然调用官方 Anthropic API 主机，但认证和请求形态更接近 Claude Code OAuth，而不是普通 Console API Key。

---

## 1.3 OpenAI/ChatGPT 订阅账号

### OAuth 授权参数

OpenAI OAuth 使用 Codex CLI 客户端参数：

- 授权地址：`https://auth.openai.com/oauth/authorize`
- Token 地址：`https://auth.openai.com/oauth/token`
- 使用 PKCE；
- scopes：`openid profile email offline_access`。

见：

- `backend/internal/pkg/openai/oauth.go:16-34`

Token 交换和账号信息解析：

- `backend/internal/service/openai_oauth_service.go:44-101`
- `backend/internal/service/openai_oauth_service.go:132-207`

它会取得并保存：

- `access_token`
- `refresh_token`
- `id_token`
- ChatGPT Account ID
- ChatGPT User ID
- Organization ID
- Plan Type
- Subscription Expiry

数据结构见：

- `backend/internal/service/openai_oauth_service.go:113-130`

项目还会访问 ChatGPT backend API，补全订阅类型、到期时间，并尝试关闭训练数据共享：

- `backend/internal/service/openai_oauth_service.go:257-295`

### 实际调用 OpenAI 上游

对于 OAuth/ChatGPT 订阅账号，Responses 请求发送到：

```text
https://chatgpt.com/backend-api/codex/responses
```

对于普通 API Key 账号，则发送到：

```text
https://api.openai.com/v1/responses
```

或者账号配置的自定义 Base URL。

相关代码：

- `backend/internal/service/openai_gateway_service.go:31-34`
- `backend/internal/service/openai_gateway_forward.go:991-1013`

OAuth 请求还会补充 ChatGPT Account ID、Host、Codex `originator`、`session_id`、`conversation_id` 等头：

- `backend/internal/service/openai_gateway_forward.go:1021-1079`

因此，对于 OpenAI 订阅账号，Sub2API 不是简单地将请求转到普通 `api.openai.com/v1/chat/completions`，而是适配 ChatGPT/Codex backend API。

---

## 1.4 用户调用 Sub2API 时的完整过程

主要公开入口集中在：

- `backend/internal/server/routes/gateway.go`

主要接口包括：

- `POST /v1/messages`
- `POST /v1/messages/count_tokens`
- `GET /v1/models`
- `POST /v1/responses`
- `POST /v1/chat/completions`
- `POST /v1/embeddings`
- Gemini 原生 `/v1beta/models/...`

相关路由：

- `backend/internal/server/routes/gateway.go:175-280`

调用流程大致如下：

```text
1. 用户携带 Sub2API 创建的 API Key
2. API Key 中间件验证用户、分组、订阅/余额和 IP ACL
3. 根据 API Key 所属 Group 决定目标平台
4. 根据模型、优先级、并发、限流和粘性会话选择账号
5. 获取或刷新该账号的上游 Token
6. 执行模型名映射和协议转换
7. 覆盖客户端认证头，注入真正的上游凭据
8. 调用 Anthropic、OpenAI、Gemini 等上游
9. 转换流式事件、Usage、工具调用和错误
10. 记录 Sub2API 本地用量并向用户返回结果
```

### Sub2API 对外认证

Sub2API 接受：

```http
Authorization: Bearer <sub2api-key>
```

或：

```http
x-api-key: <sub2api-key>
```

Gemini 还支持：

```http
x-goog-api-key: <sub2api-key>
```

见：

- `backend/internal/server/middleware/api_key_auth.go:58-100`

客户端传入的 Key 不是 Anthropic/OpenAI 原始凭据。Sub2API 构造上游请求时，会删除或覆盖入站认证头。

### 账号池调度

调度器考虑：

- 分组和平台；
- 模型支持和模型映射；
- 账号状态；
- 优先级；
- 当前并发；
- RPM；
- 配额窗口；
- Sticky Session；
- 已排除或失败的账号；
- Failover。

主要入口：

- `backend/internal/service/gateway_scheduling.go:97-230`

粘性账号从 Redis 获取：

- `backend/internal/service/gateway_scheduling.go:131-141`

### OAuth Token 自动刷新

Claude 和 OpenAI 都有 Token Provider：

- Claude：`backend/internal/service/claude_token_provider.go:54-161`
- OpenAI：`backend/internal/service/openai_token_provider.go:133-270`

它们会：

1. 优先从 Redis Token Cache 读取；
2. 在过期前刷新；
3. 使用分布式锁尽量避免多个实例同时刷新；
4. 将更新后的 Token 写回 PostgreSQL 和 Redis；
5. 缺少 Refresh Token 或永久失效时暂停账号。

---

## 1.5 Sub2API 不是透明反向代理

当 Sub2API 的 `/v1/messages` 被路由到 OpenAI 账号时，会发生：

```text
Anthropic Messages 请求
  → OpenAI Responses 请求
  → ChatGPT/Codex 或 OpenAI 上游
  → Anthropic Messages 响应
```

代码说明和转换入口：

- `backend/internal/service/openai_gateway_messages.go:25-43`
- `backend/internal/service/openai_gateway_messages.go:111-120`

转换函数：

```go
apicompat.AnthropicToResponses(...)
```

同样，Chat Completions 入口可能被转换为 Responses：

```text
OpenAI Chat Completions
  → OpenAI Responses
  → 上游
  → Chat Completions 响应
```

见：

- `backend/internal/service/openai_gateway_chat_completions.go:43-52`
- `backend/internal/service/openai_gateway_chat_completions.go:164-175`

如果某个第三方 OpenAI-compatible 上游不支持 Responses API，代码才会改为直接请求 `/v1/chat/completions`。

这使 Sub2API 能提供较强的协议兼容性，但也意味着它不是字节级透明代理。

---

## 1.6 凭据存储风险

账号 `credentials` 中包含 OAuth Access Token、Refresh Token、API Key 等高敏感数据。代码将整个对象保存到 PostgreSQL JSONB：

- 创建账号：`backend/internal/repository/account_repo.go:102-113`
- 更新凭据：`backend/internal/repository/account_repo.go:754-808`

在这些账号凭据读写路径中，没有看到字段级 AES 加密。项目虽有 AES-GCM 加密器，但主要用于 TOTP、备份对象存储密钥等其他 Secret。

因此应将 PostgreSQL及其备份视为持有完整供应商账号凭据的高敏感系统：

- 严格限制数据库访问；
- 启用磁盘、快照和备份加密；
- 使用最小权限数据库账号；
- 管理接口启用强认证和审计；
- 最好为 `credentials` 增加字段级加密或外部 Secrets Manager；
- Refresh Token 泄漏的影响可能接近整个订阅账号会话被接管。

---

# 2. 接入 LiteLLM 的可行性

## 2.1 推荐架构

技术上可行，推荐链路：

```text
                         ┌─ Sub2API Claude OAuth 账号池 ─ Anthropic
用户 ─ LiteLLM ─ Sub2API ├─ Sub2API OpenAI OAuth 账号池 ─ ChatGPT/Codex
                         └─ Sub2API API Key 账号池 ─ 官方/兼容 API
```

### LiteLLM 的职责

- 给最终用户签发 Virtual Key；
- 用户、团队和预算管理；
- 模型别名；
- 多 Upstream 路由；
- 统一日志、审计和 Fallback。

### Sub2API 的职责

- 持有订阅账号 OAuth 凭据；
- 刷新 Token；
- 账号池调度；
- 并发、RPM 和配额控制；
- 请求指纹/Header 改写；
- ChatGPT/Codex 内部端点适配；
- Anthropic/OpenAI/Gemini 协议转换。

LiteLLM 不能自动替代 Sub2API 的这些订阅账号能力。

---

## 2.2 用户请求是否仍经过 Sub2API

**仍然需要经过 Sub2API。**

用户表面上只请求 LiteLLM：

```text
POST https://litellm.example.com/v1/chat/completions
Authorization: Bearer <litellm-virtual-key>
```

但内部调用仍然是：

```text
用户 → LiteLLM → Sub2API → 上游供应商
```

这意味着：

- 用户不需要知道 Sub2API 地址和 Key；
- 每次请求仍有一次 LiteLLM 到 Sub2API 的网络调用；
- Sub2API 不可用时，对应 LiteLLM Deployment 也不可用；
- LiteLLM 只是把 Sub2API 当作自定义 Upstream；
- Sub2API Key 不会将底层 OAuth Token 转交给 LiteLLM。

### 可以绕过 Sub2API 的情况

只有以下情况可以直接由 LiteLLM 转发：

1. 在 LiteLLM 中另行配置真正的 Anthropic/OpenAI 官方 API Key；
2. 配置 Bedrock、Vertex AI 等其他 Provider；
3. 为 LiteLLM 编写 Custom Provider，重新实现 Sub2API 的 OAuth、刷新、指纹、账号池和内部协议逻辑。

第三种方案实际上是在 LiteLLM 中重写 Sub2API，一般没有必要。

---

## 2.3 推荐的 LiteLLM 配置

### OpenAI-compatible 入口

为 OpenAI Group 创建一个独立 Sub2API Key：

```yaml
model_list:
  - model_name: gpt-via-sub2api
    litellm_params:
      model: openai/gpt-5-codex
      api_base: https://sub2api.example.com/v1
      api_key: os.environ/SUB2API_OPENAI_KEY
```

LiteLLM 会向以下地址发送 Chat Completions 请求：

```text
https://sub2api.example.com/v1/chat/completions
```

`api_base` 应包含 `/v1`，但不要包含 `/chat/completions`。

### Anthropic-compatible 入口

为 Anthropic Group 创建另一个 Sub2API Key：

```yaml
model_list:
  - model_name: claude-via-sub2api
    litellm_params:
      model: anthropic/<sub2api实际公开的Claude模型名>
      api_base: https://sub2api.example.com
      api_key: os.environ/SUB2API_ANTHROPIC_KEY
```

LiteLLM Anthropic Provider 会在 Base URL 后添加 `/v1/messages`。

建议通过 Sub2API 的以下接口获取实际模型名：

```text
GET /v1/models
```

不要仅凭供应商官方模型目录猜测，因为 Sub2API 支持账号级和 Group 级模型映射。

### 为什么应按平台使用独立 Key

Sub2API 的目标平台主要由 API Key 所属 Group 决定：

- `backend/internal/server/routes/gateway.go:48-78`

因此建议：

- Anthropic Group 使用一个 Sub2API Key；
- OpenAI Group 使用另一个 Key；
- Gemini 使用单独 Key；
- LiteLLM 每个 Model Deployment 使用对应平台的 Key。

除非已经明确配置 Sub2API Composite Group 并理解其路由规则，否则不建议使用一个普通 Key 同时承担所有平台。

---

## 2.4 Responses、Codex 和原生协议

对于标准聊天和常规工具调用，LiteLLM 的 `openai/...` 或 `anthropic/...` Provider 通常足够。

但以下功能不建议默认经过 LiteLLM 标准 Completion 转换层：

- OpenAI Responses API 的 Provider-specific 字段；
- `previous_response_id`；
- Responses WebSocket；
- Codex Compact 子路径；
- 特殊 `originator`、session、conversation Header；
- Anthropic 最新 beta Header 和不常见 Content Block；
- 必须完全保留原始 SSE 事件的客户端。

因为 LiteLLM 标准 Provider 会解析并规范化请求和响应，不保证未知字段与错误体逐字透传。

### LiteLLM HTTP Pass-through 示例

```yaml
general_settings:
  pass_through_endpoints:
    - path: /sub2api
      target: https://sub2api.example.com
      headers:
        Authorization: Bearer os.environ/SUB2API_KEY
      include_subpath: true
```

客户端请求：

```text
POST https://litellm.example.com/sub2api/v1/responses
```

这种模式下，LiteLLM 更接近 HTTP 入口网关，Sub2API 继续负责协议细节。但仍需实测：

- SSE；
- 流式中途错误；
- Header 冲突；
- 非 2xx 状态码；
- Usage 终态事件。

---

## 2.5 与直接使用官方 API 的区别

| 维度 | 官方 API Key | Sub2API 订阅账号模式 |
|---|---|---|
| 凭据 | Console/API 平台 Key | 用户订阅 OAuth Access/Refresh Token |
| OpenAI 上游 | `api.openai.com` | OAuth 时通常是 `chatgpt.com/backend-api/codex` |
| Claude 上游 | Anthropic API Key | OAuth Bearer + Claude Code 请求特征 |
| 计费 | 官方 API Token 计费 | 订阅权益、上游配额和 Sub2API 本地计费 |
| 限流 | 官方 API Tier | 订阅账号配额 + Sub2API 并发/RPM |
| 模型 | 官方 API 明确开放的模型 | 取决于订阅账号、内部端点和模型映射 |
| SLA | 官方 API SLA 和支持体系 | 叠加 Sub2API、OAuth 和内部端点风险 |
| 错误 | 官方错误结构 | 可能被 Sub2API/LiteLLM 转换或清洗 |
| 功能上线 | 官方文档为准 | Sub2API 和 LiteLLM 均需适配 |
| 请求透明性 | SDK 直接发送 | 请求可能被改写 |
| 合规风险 | 官方 API 商业使用条款 | 还需确认订阅/OAuth 自动化和转售条款 |

### 主要技术差异

#### 请求会被修改

Claude OAuth 的非 Claude Code 客户端请求可能被重写：

- System Prompt；
- Metadata；
- Cache Control；
- 工具名；
- 客户端指纹；
- Anthropic beta Header。

相关实现：

- `backend/internal/service/gateway_forward.go:161-244`
- `backend/internal/service/gateway_upstream_request.go:55-189`

#### 模型可能被映射

- Anthropic OAuth 模型会被规范化；
- OpenAI 存在客户端模型、计费模型和上游模型的区别。

见：

- `backend/internal/service/gateway_forward.go:261-299`
- `backend/internal/service/openai_gateway_messages.go:58-61`

#### 错误可能改变

例如 OpenAI Passthrough 会将部分上游 401/403 转换为下游 502，并隐藏上游认证细节：

- `backend/internal/service/openai_gateway_passthrough.go:529-543`

接入 LiteLLM 后，错误还可能被 LiteLLM 再包装一次。因此，不应依赖原始上游 Error Body 的逐字一致性。

#### Usage 不一定等于官方账单语义

Sub2API 会读取或转换上游 Usage，并应用自己的计费和缓存 Token 归类。协议转换、终态 Usage 缺失或上游兼容不完整时，都可能产生差异。

在 Chat Completions 流中，Sub2API 会强制输出 Usage，以防下一级代理计费为零：

- `backend/internal/service/openai_gateway_chat_completions.go:505-510`

#### 并非完整官方 API

Sub2API 对外主要实现 Messages、Responses、Chat Completions、Models、部分 Embeddings、Image 和 Video 等接口，但不能假定 Files、Batches、Admin API 和所有 beta Endpoint 都完整兼容。

#### 增加故障点和延迟

直接官方：

```text
用户 → 官方
```

Sub2API：

```text
用户 → Sub2API → 官方
```

LiteLLM + Sub2API：

```text
用户 → LiteLLM → Sub2API → 官方
```

需要考虑：

- 额外网络延迟；
- 多层超时边界；
- SSE 断流；
- 重试叠加；
- 双层限流；
- 双层错误转换；
- 双层 Usage 统计。

### 上线前必须测试

至少验证：

1. `/v1/messages` 非流式；
2. `/v1/messages` SSE；
3. `/v1/chat/completions` 非流式和 SSE；
4. `/v1/responses`；
5. 单个 Tool Call；
6. 并行 Tool Calls；
7. Tool Result 回传；
8. Thinking 和 Cache Control；
9. 最后一个 Usage Chunk；
10. 401、403、429、500、529；
11. 流式中途断开；
12. 模型名映射；
13. 长对话；
14. `previous_response_id`。

---

# 3. Sub2API 高可用能力

## 3.1 总体判断

**可以建设成高可用系统，但需要部署约束和部分代码改造。**

更准确地说：

- 核心 API 数据面已经有较好的多实例基础；
- 完整单体控制面仍有多个单机状态；
- 仓库现有 Docker Compose 不是高可用方案；
- 仓库没有现成 Kubernetes Deployment、HPA、PDB、Readiness 等清单。

---

## 3.2 已具备多实例基础的部分

### PostgreSQL

持久化权威数据在 PostgreSQL。应用可以连接外部 HA PostgreSQL Writer Endpoint。

启动迁移使用 PostgreSQL Advisory Lock 串行执行：

- `backend/internal/repository/migrations_runner.go:48-58`
- `backend/internal/repository/migrations_runner.go:124-146`

多个实例启动时不会同时修改 Schema。不过生产仍建议使用独立 Migration Job，而不是让所有应用副本启动时竞争迁移锁。

### Redis 全局协调

Redis 保存：

- API Key 二级缓存；
- 缓存失效 Pub/Sub；
- Sticky Session；
- 并发槽位；
- RPM；
- Session Limit；
- OAuth Token Cache 和 Refresh Lock；
- Scheduler Snapshot；
- Batch Image Queue；
- 部分 Leader Lock。

账号并发使用 Redis Lua 和 Redis `TIME`，具备跨副本原子性：

- `backend/internal/repository/concurrency_cache.go:63-180`

### API Key 缓存失效

设计为：

```text
DB Outbox
  → Worker Claim
  → Redis L2 删除
  → Pub/Sub
  → 每实例 L1 失效
```

Claim 使用 `FOR UPDATE SKIP LOCKED`：

- `backend/internal/service/auth_cache_invalidation_outbox.go:101-219`
- `backend/internal/repository/auth_cache_invalidation_outbox_repo.go:22-67`

### Batch Image 和 Usage Cleanup

Batch Image 队列包含 Reserve、Job Lock、Heartbeat 和 Stale Recovery；Usage Cleanup 使用数据库 Claim。这些部分适合多 Worker。

---

## 3.3 当前阻碍完全无状态多副本的问题

### A. OAuth 临时 Session 在进程内

Claude OAuth：

- `backend/internal/pkg/oauth/oauth.go:47-117`

OpenAI OAuth：

- `backend/internal/pkg/openai/oauth.go:52-124`

它们都是进程内 Map，TTL 为 30 分钟。

如果授权开始落到 Pod A，而 OAuth 回调落到 Pod B，Pod B 找不到 State 和 PKCE Verifier，会返回 Session Not Found 或 Expired。

**建议：**

- 推荐将 OAuth Session 保存到 Redis 并原子消费；
- 未改造前，对管理端 OAuth 路由使用 Cookie Affinity；
- 普通推理 API 不需要 Affinity。

### B. Scheduled Test Runner 会重复执行

每个实例都会周期性查找 Due Plan：

- `backend/internal/service/scheduled_test_runner_service.go:45-67`
- `backend/internal/service/scheduled_test_runner_service.go:87-146`

数据库查询没有 Claim/Lease：

- `backend/internal/repository/scheduled_test_repo.go:51-84`

多个实例可能同时测试同一计划、重复调用上游和重复写入结果。

**建议：**实现 `FOR UPDATE SKIP LOCKED` + Lease/Claim，或在改造前只启用一个 Scheduler 副本。

### C. Channel Monitor 是每实例本地调度

- `backend/internal/service/channel_monitor_runner.go:35-47`
- `backend/internal/service/channel_monitor_runner.go:115-198`
- `backend/internal/service/channel_monitor_runner.go:236-295`

多个副本会对同一渠道重复探测；管理员更新某个 Monitor 后，只有处理该请求的实例更新了本地 Timer。

**建议：**

- 单独 Singleton Worker；或
- 每个 Monitor 使用 Redis/PG Lease；
- 配置变更使用 DB Outbox/Pub/Sub 通知所有 Scheduler。

### D. Backup/Restore 只有进程内锁

- `backend/internal/service/backup_service.go:131-148`
- `backend/internal/service/backup_service.go:383-473`
- `backend/internal/service/backup_service.go:742-753`

多副本可能同时创建备份、覆盖备份记录，甚至同时执行恢复。

**建议：**

- Backup/Restore 放入独立受控 Worker；
- 使用 PostgreSQL Advisory Lock 或持久 Lease；
- Restore 时从负载均衡器摘除业务流量；
- 只有极少数运维身份拥有 Restore 权限。

### E. Sub2API 用户 Refresh Token 轮转竞态

当前轮转流程大致是：

```text
GET old token
  → 验证
  → DEL old token
  → 创建新 token pair
```

这不是一个原子 Redis 操作。两个副本同时收到同一个 Refresh Token 时，可能都在删除前通过验证。

**建议：**使用 Lua 原子 Consume/Rotation，并加入 Token Reuse Detection。

### F. 上游 OAuth Refresh Lock 不够严格

正常情况下有 Redis 分布式锁，但仍有以下风险：

- Redis Lock 获取失败时，部分路径会降级为无分布式锁继续刷新；
- Lock Value 固定；
- Release 是无条件 `DEL`；
- 没有 Owner Token Compare-and-delete。

**建议：**

- Redis 不可用时 Refresh Fail-closed；
- 使用随机 Owner Token；
- Lua Compare-and-delete；
- 必要时续租；
- Provider QPS 改为集群级控制。

### G. OpenAI WebSocket 部分状态是本地的

部分共享状态存 Redis，但以下内容仍是本实例状态：

- 上游 Connection Pool；
- Response → Connection；
- Session → Turn State；
- Session → Connection。

单条长连接建立后天然固定在一个 Pod，通常没有问题；但断线重连到另一 Pod 后，严格的 `previous_response_id` 或 Turn Continuation 可能降级。

**建议：**

- 对 WebSocket 使用连接/会话亲和；
- 或提供专门 WS Gateway；
- Pod 终止时提供足够的 Drain 时间。

### H. `/health` 不是 Readiness

现有 `/health` 固定返回：

```json
{"status":"ok"}
```

见：

- `backend/internal/server/routes/common.go:9-14`

它不检查：

- PostgreSQL；
- Redis；
- Migration；
- Scheduler Snapshot；
- 必需 Secret；
- Worker/Outbox Lag。

应拆分为：

```text
/livez   只检查进程是否存活
/readyz  检查 PG、Redis、Migration 和关键初始化
```

### I. 本地文件和初始化状态

Setup 依赖本地：

- `config.yaml`
- `.installed`
- `DATA_DIR`

相关代码：

- `backend/internal/setup/setup.go:26-74`
- `backend/internal/setup/setup.go:160-179`

多个 Pod 各自使用 EmptyDir 时，每个 Pod 都可能认为系统尚未安装。

**建议：**

- 多副本 API 禁用 `AUTO_SETUP`；
- 使用一次性初始化或 Migration Job；
- 配置来自 ConfigMap/Secret；
- 不将 `.installed` 视为集群状态；
- Pricing、Pages 等文件应打入不可变镜像或存入共享对象存储。

---

## 3.4 推荐高可用拓扑

```text
                 CDN / WAF / Load Balancer
                            |
             +--------------+--------------+
             |              |              |
         API Pod A      API Pod B      API Pod C
             |              |              |
             +------ HA PostgreSQL Writer Endpoint
             |
             +------ HA Redis Primary Endpoint
             |
             +------ S3 / R2 / GCS

独立 Worker/Scheduler：
  - Batch Image Worker：可多副本
  - Usage/Outbox Worker：可多副本
  - Scheduled Test：当前单副本
  - Channel Monitor：当前单副本
  - Backup/Restore：严格单副本 + 分布式锁
```

### API 层建议

- 至少 2～3 个副本；
- 跨可用区；
- Pod Anti-affinity 或 Topology Spread；
- PodDisruptionBudget；
- 真正的 Readiness；
- 长流和 WebSocket 预留足够的 `terminationGracePeriodSeconds`；
- 日志输出到 stdout/stderr；
- 禁用应用内在线更新；
- 所有副本使用一致的 Secret；
- 普通 API 不要求 Sticky Session；
- 管理端 OAuth 在共享 Session Store 改造前使用 Cookie Affinity。

### PostgreSQL 建议

- 多 AZ Primary/Standby；
- 自动 Failover；
- 稳定 Writer Endpoint；
- PITR；
- TLS；
- 按副本数计算连接池总量；
- 定期恢复演练；
- Migration 使用单独 Job。

### Redis 建议

当前客户端使用普通 `redis.NewClient` 并接受单一地址，不直接配置 Sentinel/Cluster 节点列表。

因此建议使用：

- 托管 Redis 的稳定 Primary Endpoint；
- 或使用 VIP/代理屏蔽主从切换；
- AOF 或托管服务等价持久化；
- 禁止会轻易淘汰关键 Token、Lock 和 Queue 的 Eviction 策略；
- TLS、ACL 和网络隔离。

不建议未经完整集成测试直接使用 Redis Cluster，因为项目包含不少多 Key Lua 脚本，无法默认确认所有 Key 都处于同一 Hash Slot。

### Secret 一致性

所有副本必须使用相同且固定的：

```text
TOTP_ENCRYPTION_KEY
JWT Secret
数据库凭据
Redis 凭据
对象存储凭据
```

`TOTP_ENCRYPTION_KEY` 不只用于 TOTP，也被多个持久 Secret 加密路径复用。不同副本使用不同值会导致已有密文无法解密。

---

# 4. 推荐落地方案

如果目标是将订阅账号统一提供给内部用户，推荐：

```text
用户
  → LiteLLM（Virtual Key、预算、团队、路由）
  → Sub2API（订阅账号池、OAuth、刷新、协议适配）
  → 上游供应商
```

上线前至少完成：

1. 按平台创建独立 Sub2API Key；
2. LiteLLM 分别配置 OpenAI 和 Anthropic Deployment；
3. Responses/Codex 特殊功能走 Pass-through 或直接访问 Sub2API；
4. 锁定 LiteLLM 和 Sub2API 版本；
5. 完成 Streaming、Tool、Usage、Error 的端到端测试；
6. 确认供应商关于订阅账号自动化、共享和转售的条款；
7. PostgreSQL 和 Redis 使用 HA 服务；
8. Scheduler、Channel Monitor、Backup 从 API 副本隔离；
9. OAuth 临时状态迁移到 Redis，或临时启用管理端 Affinity；
10. 增加真正的 Readiness 和长连接优雅下线；
11. 修复 Refresh Token 原子轮转；
12. 加强账号 `credentials` 的数据库安全和加密保护。

---

# 5. LiteLLM 参考资料

- [OpenAI-compatible endpoints](https://docs.litellm.ai/docs/providers/openai_compatible)
- [LiteLLM Proxy configuration](https://docs.litellm.ai/docs/proxy/configs)
- [LiteLLM Proxy quickstart](https://docs.litellm.ai/docs/proxy/quick_start)
- [Function calling](https://docs.litellm.ai/docs/completion/function_call)
- [Anthropic provider](https://docs.litellm.ai/docs/providers/anthropic)
- [Anthropic native pass-through](https://docs.litellm.ai/docs/pass_through/anthropic_completion)
- [Custom pass-through endpoints](https://docs.litellm.ai/docs/proxy/pass_through)

---

# 6. 最终判断

## 是否能接入 LiteLLM

**能。**最适合的方式是让 LiteLLM 作为用户侧 Key、预算和统一路由层，让 Sub2API 继续作为订阅凭据、账号池和协议适配层。

## 用户调用是否仍经过 Sub2API

**会经过。**用户只需要知道 LiteLLM，但 LiteLLM 到供应商之间仍会调用 Sub2API。除非在 LiteLLM 中另外配置真正的官方 API Key，否则无法绕过 Sub2API。

## 是否等同官方 API

**不完全等同。**常见聊天和工具调用可以做到高度兼容，但认证、端点、配额、计费、错误、Beta 功能、模型映射和稳定性语义不同。

## 是否能高可用部署

**核心 API 数据面可以水平扩容，但当前完整单体不能无约束地复制多个副本。**完成 OAuth Session 共享、单例任务隔离、分布式锁加强、Readiness、Secret 一致性和外部 HA PostgreSQL/Redis 后，可以达到较可靠的生产高可用基线。
