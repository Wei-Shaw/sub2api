# 验证 — add-devin-platform（2026-09-14）

## 自动验证

### 后端
- `go build ./...`：通过。
- `gofmt -l`（全部改动文件）：0 文件待格式化。
- `GOCACHE=/tmp/sub2api-go-cache go test -tags=unit ./internal/pkg/devin/...`：全部通过
  （proto 编解码、Connect 帧、auth/session、catalog 分组、adapter 编解码、
  anthropic/openai-chat/openai-responses codec、WSSession 12 项状态机用例）。
- `go test ./internal/service/ -run Devin`：5 项通过（auth-url、token 直贴
  有/无会话、PKCE 交换、state 拒绝）。
- `go test ./internal/handler/ -run Devin`：failover 分类用例通过
  （invalid→400 不换号、bare transport→502 failover）。
- `GOCACHE=… go test -tags=unit ./...`：全绿，唯一失败为
  `internal/repository.TestAliyunCaptchaVerifier_TransportError`——环境性失败
  （本机 `http_proxy=127.0.0.1:10808` 使关断端口经代理返回而非直连拒绝）；
  `env -u http_proxy …` 下单测通过；本次改动未触及 internal/repository。
- `go vet ./internal/handler/`、`./internal/pkg/devin/...`：0 findings。
- golangci-lint：本机二进制以 go1.25 构建，低于 go.mod 目标 1.27.0，无法本地
  运行（CI golangci-lint v2.9 会执行）；已用 go vet + gofmt 覆盖基础检查。

### 前端
- `vue-tsc --noEmit`：0 错误。
- ESLint（全部改动文件）：0 findings。
- vitest：14 个 critical spec 文件 189 用例 + account/admin 组件目录
  28 个 spec 文件 388 用例全部通过；platforms.spec、localeKeyCompleteness 更新后通过。

## 手工验证（待用户）

- 管理端创建 devin 账号：PKCE 授权码流 + `devin-session-token$…` 直贴两条路径。
- devin 分组 API Key：`POST /v1/responses`、`POST /v1/chat/completions`、
  `POST /v1/messages`、`GET /v1/models`、`GET /v1/responses`（WS）。
- 额度单元格展示（24h/7d 剩余、credits、ACU）。

## 验证边界

- 未访问真实 Devin 上游；wire 字段号按插件抓包（v3000.10.21）交叉验证。
- WS 端到端（含 prewarm、replacement replay）未经真实客户端联调。
- `client_version` 门控、ACU/credit 计费语义需真实账号回归。
