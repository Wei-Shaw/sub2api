# Tasks — add-devin-platform

## 1. Wire 层（internal/pkg/devin）
- [x] proto.go：最小 wire 编解码 + 字段号常量（对照插件抓包 v3000.10.21）
- [x] connect.go：Connect 帧编解码、ConnectError、InvalidRequestError、transient 判定
- [x] metadata.go / randid.go：CLI 元数据头、PKCE、确定性 UUID/PrefixedID
- [x] catalog.go：ListModels 解码 + GroupModels 族聚合 + thinkingLevelMap
- [x] chat.go / auth.go / auth_session.go / ratelimit.go：GetChatMessage/AssignModel/GetUserStatus/交换 RPC、会话 store、限流分类
- [x] 单元测试（proto/connect/chat/randid）

## 2. IR + adapter
- [x] internal/pkg/devin/llm：RequestMessages/Message/Content/ResponseEvent IR
- [x] internal/pkg/devin/adapter：session 派生、request_encoder、tool_definition、response_decoder、sanitize、stream、overflow 重试、本地错误 InvalidRequest 包装
- [x] 单元测试

## 3. 入站 codec（internal/pkg/devin/api）
- [x] anthropic messages codec
- [x] openai chat completions codec
- [x] openai responses codec（含 WS 会话机 ws_session.go + 12 项单测）

## 4. 平台注册 + 管理端 OAuth
- [x] PlatformDevin 常量；Account.IsDevin/GetDevinToken/GetDevinBaseURL/GetDevinClientVersion
- [x] DevinOAuthService + /admin/devin/oauth/{auth-url,exchange-code} 路由 + Wire
- [x] exchange-code 对 devin-session-token$… 直贴免会话
- [x] 账号连通性测试 testDevinAccountConnection
- [x] service 层单测（5 项）

## 5. Gateway 转发
- [x] DevinGatewayService（adapterForAccount/Stream/ListModels/DevinFailoverError）
- [x] DevinGatewayHandler：Messages/ChatCompletions/Responses/Models/CountTokens + forward + pumpDevinStream + submitDevinUsage
- [x] routes/gateway.go devin 分派（/api/v1 + 根别名）
- [x] 账号级 model_mapping → 上游模型改写（响应回填客户端模型）
- [x] DeriveUpstreamEndpoint: devin/GetChatMessage

## 6. Responses WebSocket
- [x] WSSession 状态机移植 + 单测
- [x] devin_gateway_ws.go：升级、ingress lease、读超时、逐回合 forward 复用、prewarm、commit/replacement replay
- [x] GET /responses 根别名 devin 分派

## 7. 额度 + 前端 + 复合路由
- [x] DevinQuotaFetcher + UsageInfo.devin_quota + 缓存/singleflight
- [x] composite_platform.go：devin/cognition 别名、swe-*/devin-* 前缀、isConcreteRequestPlatform
- [x] admin_group.go：devin 默认模型候选 + 复合合并列表；group/target_platform oneof
- [x] 前端：api/composable/types/constants/model whitelist/icon/badge/colors/modals/usage cell/i18n
- [x] platforms.spec / locale completeness 更新

## 8. 文档与全量验证
- [x] openspec/changes/add-devin-platform：proposal/design/tasks/verification
- [x] make -C backend test-unit（仅环境性 aliyun 代理失败）+ gofmt/go vet（本机 golangci-lint 版本低于 go.mod 目标）
- [x] 前端 typecheck + lint + critical vitest
- [ ] Conventional Commit + PR checklist（DEV_GUIDE 坑11）
