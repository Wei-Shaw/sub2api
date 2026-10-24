# 跨厂商转换与 Composite 目标路由：验证说明

基础版本：v0.2.17，提交 `b0a02a3304a0a78cdd003ddda1dd23ba02c03084`。

## 可复现检查

工具链由 `backend/go.mod`、`frontend/package.json` 和锁文件确定。以下命令从仓库根目录执行；后端命令在独立代码块中显式切换目录。

```bash
pnpm --dir frontend install --frozen-lockfile
```

```bash
pnpm --dir frontend run lint:check
```

```bash
pnpm --dir frontend run typecheck
```

```bash
pnpm --dir frontend run test:run
```

```bash
pnpm --dir frontend run build
```

```bash
cd backend && go test ./internal/pkg/apicompat
```

```bash
cd backend && go test -tags unit ./internal/service ./internal/repository ./internal/server/routes ./internal/handler/admin ./migrations -run 'Composite|SchedulingScope|ForwardAsAnthropic|MessagesReasoningReplay|DropClearlyForeignAnthropicThinking|GatewayForward_NativeAnthropic|CountTokens' -count=1
```

```bash
cd backend && go test -race -tags unit ./internal/service -run 'TestCompositeScopeIsolation_|TestGeminiCompositeRoute|TestMessagesReasoningReplay|TestDropClearlyForeignAnthropicThinking' -count=1
```

```bash
cd backend && go vet ./internal/pkg/apicompat ./internal/service ./internal/repository ./internal/server/routes ./internal/handler/...
```

```bash
cd backend && go build -tags embed -o bin/sub2api ./cmd/server
```

## 回归范围

### 协议转换

- 根据实际目标账号选择加密推理兼容策略，不依赖客户端模型别名。
- OpenAI 原生 API-key 和 OAuth 重放 reasoning 时携带显式 `summary: []`。
- 保留正文、工具参数、调用 ID、结果配对和消息顺序。
- 明确的 HTTP 400／502 加密内容拒绝最多重试一次；覆盖流式和非流式客户端。
- 上游 SSE 中的对应错误只在下游尚未输出时恢复；输出后不重试。
- 重试保持会话、Codex 身份和 turn-state 头，不删除 compaction 项。
- 原生 Claude Messages 和 count_tokens 路径移除明确的外来 OpenAI 思考块，并保留调用方原始历史。
- `TestForwardAsAnthropicTLSReasoningRetry` 使用真实本地 TLS 服务器和专属可信客户端；保持 HTTPS 校验及仅限回环地址的 URL allowlist，验证流式／非流式模式下请求转换、HTTP 502 后一次重试、历史与身份头一致性，以及成功响应转换，不关闭证书验证。

### 路由与计费

- 兼容无目标 ID 的既有账号池路由。
- 组目标和账号目标互斥；无效 ID、自指、嵌套 Composite、平台不匹配被拒绝。
- Gateway 优先级／负载调度、OpenAI 旧／负载／高级调度、Gemini 选择路径都限制在显式目标内。
- 粘性绑定、previous response 和失败排除不能扩大目标账号池。
- 账号状态、冷却和新鲜成员关系仍参与准入。
- 源组计费、隐私策略和 API-key 身份不切换为目标组。
- 目标组刷新使用批量查询；预览会检查目标配置有效性。
- 管理接口覆盖创建、更新、清除目标、预览和类型化验证错误；仓库测试覆盖可空字段和迁移约束。

## 本地实际应用验证范围

使用独立 PostgreSQL 17、Redis 7 和嵌入前端的程序验证了：健康接口、首页、登录、测试对象创建、两类目标路由的 CRUD／预览、禁用规则行为、实际数据库迁移列及持久化值。

独立完整运行实例到模拟上游的 HTTPS 转发未验证。服务边界另有真实本地 TLS 网络回归，以及模拟传输层测试；这不等同于完整应用入口到真实厂商的端到端验证。没有使用真实模型凭据验证全部厂商组合。生产部署仍应先在预发布环境执行实际所需的上游、工具调用、账号固定失败和计费校验。

## Windows 基线失败

大范围后端回归在 Go 1.27.2 Windows 环境中出现下列基础版本已有失败；相同测试在独立未修改代码上复现，不能将该环境结果写为“后端全量通过”：

| 测试 | 基础版本重复结果 | 修改版本重复结果 |
| --- | --- | --- |
| `TestOllamaProbeCallback_StaleLongDoesNotOverrideNewShort` | 5/5 失败 | 5/5 失败 |
| `TestAliyunCaptchaVerifier_TransportError` | 5/5 失败 | 5/5 失败 |
| `TestCommandCodeGatewayPassesThroughCatalogProtocols` | 20 次中 16 次失败 | 20 次中 12 次失败 |

基线比较只确定这些失败先于功能修改存在，不确定其根因。CI 在其配置的 Linux 环境中运行完整检查。

## 能力边界

- 前缀过滤是兼容启发式，不是密文来源认证；未知格式或跨账号密文仍可能被上游拒绝。
- 依赖项目已有的厂商适配器，不宣称所有模型、凭据和协议组合均可转换。
- 主代理和子代理需要发送不同入站模型 ID；路由不是代理编排器。
- 首版不支持嵌套 Composite；固定账号失败不转移到其他账号。
- 异步视频的新目标路由暂不支持，创建入口明确拒绝，避免后续无模型查询无法恢复目标授权。

配置、迁移与回滚步骤见 [使用说明](composite-cross-provider-routing.zh-CN.md)。
