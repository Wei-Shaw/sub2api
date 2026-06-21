# CLIProxyAPI gpt-image-2 原生生图映射修正与验证

时间：2026-06-21 16:18

## 背景

用户指出 CLIProxyAPI 中 `gpt-image-2` 的映射有问题：`gpt-image-2` 本身就是生图模型，不应该通过 Gemini / Antigravity 的 `gemini-3.1-flash-image` 来生图。

上一轮调查已确认 CLIProxyAPI 监听 `127.0.0.1:8317`，并存在 OpenAI 兼容图片端口：

- `POST /v1/images/generations`
- `POST /v1/images/edits`

## 根因

`/Users/wujianxiang/CodeSpace/CLIProxyAPI/config.yaml` 中存在运行配置：

```yaml
oauth-model-alias:
  antigravity:
    - name: "gemini-3.1-flash-image"
      alias: "gpt-image-2"
      fork: true
```

这会把请求里的 `gpt-image-2` 路由到 Antigravity / Gemini，而不是走 CLIProxyAPI 内置的 `gpt-image-2` 生图处理路径。

代码层面的正确路径已经存在：

- `/Users/wujianxiang/CodeSpace/CLIProxyAPI/sdk/api/handlers/openai/openai_images_handlers.go` 将 `gpt-image-2` 识别为 `/v1/images/generations` 和 `/v1/images/edits` 支持的图片模型。
- `/Users/wujianxiang/CodeSpace/CLIProxyAPI/internal/runtime/executor/codex_openai_images.go` 会把 OpenAI 图片端口请求转换为 Codex Responses 的 `image_generation` 工具调用。

因此本次不需要修改 Go 代码，只需要移除错误运行配置。

## 改动

已从 `/Users/wujianxiang/CodeSpace/CLIProxyAPI/config.yaml` 删除 `oauth-model-alias.antigravity` 中 `gpt-image-2 -> gemini-3.1-flash-image` 的别名映射。

未记录、未输出完整 API Key、OAuth token、HMAC secret 或管理员密码。

## 验证

1. 配置检查

   执行 `rg -n "oauth-model-alias|gpt-image-2|gemini-3.1-flash-image" /Users/wujianxiang/CodeSpace/CLIProxyAPI/config.yaml -S`，无结果，说明错误别名映射已从运行配置移除。

2. CLIProxyAPI 端口检查

   执行 `curl http://127.0.0.1:8317/`，返回的 endpoints 包含 `POST /v1/images/generations`。

3. 真实生图验证

   使用 CLIProxyAPI 当前配置中的现有 API Key，直连：

   `POST http://127.0.0.1:8317/v1/images/generations`

   请求摘要：

   ```json
   {
     "model": "gpt-image-2",
     "prompt": "A minimal verification image: a clean black square centered on a white background, no text.",
     "size": "1024x1024",
     "n": 1,
     "output_format": "png",
     "response_format": "b64_json"
   }
   ```

   返回摘要：

   ```json
   {
     "status": 200,
     "ok": true,
     "image_count": 1,
     "b64_json_length": 574148,
     "saved_file": "/tmp/cliproxyapi-gpt-image-2-real-test.png",
     "saved_bytes": 430610
   }
   ```

   解码后的文件检查：

   ```text
   /tmp/cliproxyapi-gpt-image-2-real-test.png: PNG image data, 1254 x 1254, 8-bit/color RGB, non-interlaced
   ```

   已视觉确认图片内容为白底居中的黑色方块，说明生图端口真实返回了图片结果。

## 结论

CLIProxyAPI 中错误的 `gpt-image-2 -> gemini-3.1-flash-image` 映射已移除。当前 CLIProxyAPI 本地端口 `127.0.0.1:8317/v1/images/generations` 可以使用 `gpt-image-2` 真实生成图片。

本次未修改 Sub2API 的套餐生图开关、分组价格或计费逻辑。若要让公网订阅用户使用，还需要单独确认并配置三个在售分组的 `allow_image_generation` 和图片价格。
