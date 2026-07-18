# Grok-only Gateway

独立的 Grok 网关服务，包含一个内置 Web 管理界面。

## 功能

- Grok OAuth PKCE 授权、code 换 token、refresh token 导入。
- 多 Grok 账号轮询、冷却和简单失败切换。
- 用户 API Key 管理。
- OpenAI-compatible 聊天接口：
  - `POST /v1/responses`
  - `POST /v1/chat/completions`
  - `POST /v1/messages`
- 图片接口：
  - `POST /v1/images/generations`
  - `POST /v1/images/edits`
- 视频接口：
  - `POST /v1/videos/generations`
  - `GET /v1/videos/{request_id}`
- 内置界面可操作账号、API Key、聊天、生图和视频。

## 运行

```powershell
cd grok-only-gateway
go run .
```

默认地址：

```text
http://127.0.0.1:8088
```

可选环境变量：

```text
GROK_ADDR=127.0.0.1:8088
GROK_DATA_PATH=data/state.json
GROK_ADMIN_TOKEN=change-me
GROK_API_KEY=grok-local-test-key
GROK_MAX_BODY_BYTES=67108864
GROK_REQUEST_TIMEOUT_SECONDS=180
XAI_BASE_URL=https://api.x.ai/v1
XAI_OAUTH_REDIRECT_URI=http://127.0.0.1:56121/callback
```

如果设置了 `GROK_ADMIN_TOKEN`，管理接口和界面请求需要在页面顶部填写 Admin Token。

## 调用示例

```powershell
curl.exe -X POST "http://127.0.0.1:8088/v1/chat/completions" `
  -H "Authorization: Bearer grok-local-test-key" `
  -H "Content-Type: application/json" `
  -d '{"model":"grok-4.3","messages":[{"role":"user","content":"你好"}]}'
```
