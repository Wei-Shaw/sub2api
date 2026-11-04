# ChatGPT Web 生图通道

把 `/v1/images/generations`、`/v1/images/edits` 交给 `chatgpt.com` 网页链路执行，消耗**网页侧
`image_gen` 额度**，与 Codex/Responses 后端额度相互独立。Codex 额度耗尽后网页额度仍可用的场景下，
生图不再中断。

## 可用凭据（实测结论）

基础请求使用 Chrome TLS 指纹，不启动完整浏览器。若 Sentinel 返回 `turnstile.required=true`，还需要配置下述本地 VM 解算命令。基础凭据：

1. **账号的 ChatGPT access_token**：sub2api 的 OpenAI OAuth 账号里已有。
2. **真 Chrome 的 TLS 指纹**（`tlsfingerprint.Profile{Preset: PresetChrome}`）：`chatgpt.com` 前置
   Cloudflare，指纹不对会被 `403 cf-mitigated: challenge` 拦掉。

指纹这一条是 A/B 对照实验测出来的，不是推测。同一出口 IP、同一时刻、同一 cookie，交替请求
`/api/auth/session`：

| 指纹 | 结果 |
| --- | --- |
| 项目默认 Node.js 风格手工指纹（GREASE + 手工扩展顺序） | 0/5 通过，全部 `403 cf-mitigated: challenge` |
| uTLS 内置 Chrome ClientHello（`PresetChrome`，ALPN 限 http/1.1） | 5/5 通过，全部 200 |

所以问题出在指纹本身（缺 ALPS、compress_certificate，扩展顺序也不是 Chrome 的），跟出口 IP 无关。
实现里因此固定用 Chrome preset，并且 **spec 每次握手重新生成**——`ApplyPreset` 会就地改写 spec
（GREASE 化），复用同一份会让第二次握手以 `tls: internal error` 失败。

## 链路

```
GET  /                                            -> script src 与 data-build（PoW 指纹素材）
POST /backend-api/sentinel/chat-requirements/prepare   {"p": <requirements token>}
      -> proofofwork{seed,difficulty} / turnstile{required,dx} / prepare_token
POST /backend-api/sentinel/chat-requirements/finalize
      -> token（OpenAI-Sentinel-Chat-Requirements-Token）
POST /backend-api/f/conversation/prepare          {"system_hints":["picture_v2"]}  -> conduit_token
POST /backend-api/f/conversation                  (SSE) -> 工具消息里的图片指针
GET  /backend-api/files/download/{id}?conversation_id={cid} | /backend-api/conversation/{cid}/attachment/{aid}/download
      -> download_url -> 匿名 HTTPS 签名地址直返，或由网关下载需鉴权的第一方地址
DELETE /backend-api/conversation/id/{cid}          -> 异步清理生图会话
GET  /backend-api/conversation/init               -> limits_progress[feature_name=image_gen] 剩余额度
```

参考图编辑额外走三段式上传：

```
POST /backend-api/files                      {file_name,file_size,use_case:"multimodal",width,height}
PUT  <upload_url>                            x-ms-blob-type: BlockBlob
POST /backend-api/files/{file_id}/uploaded   确认
```

随后 conversation 里以 `multimodal_text` + `file-service://{file_id}` 引用。

图片只在**工具或助手输出消息**中被采信：`author.role` 为 `tool` / `assistant`，
或 `metadata.async_task_type == "image_gen"`；指针形如 `file-service://file_x` / `sediment://file_x`。
用户输入附件被排除，参考图不会被当成输出。
SSE 未给出图片时回退轮询 `/backend-api/conversations/{id}?num_turns=10&include_has_versions=true`（300s 上限）。

## Sentinel 现状

- **PoW**：使用浏览器协议的 FNV1a32 + avalanche 哈希，本地求解。
- **Turnstile**：仅在上游要求时执行。通过 `SUB2API_CHATGPT_WEB_TURNSTILE_COMMAND` 配置命令 argv 的 JSON 数组；未配置、超时或解算失败会终止请求，不会跳过校验。
  配置示例及运行依赖见 [Turnstile 运行说明](../backend/scripts/chatgpt-web-turnstile.README.md)。挑战通过 stdin 传入，账号凭据不传给命令。
- **Cloudflare**：入口挑战与 Sentinel 校验分别识别。Chrome TLS preset 不保证永远免于挑战，挑战页不按账号凭据失效冷却。

## 代码结构

| 文件 | 职责 |
| --- | --- |
| `chatgpt_web_pow.go` | 指纹 JSON 构造、PoW 求解、首页 sentinel 资源解析 |
| `chatgpt_web_client.go` | sentinel 流程、参考图上传、生成/编辑、指针收集、下载、额度读取 |
| `chatgpt_web_images.go` | `/v1/images/*` 接入层，合成 Responses 事件复用既有下游 |
| `chatgpt_web_account.go` | 账号级开关（extra 方式）与稳定设备指纹 |

设计要点：`response_format=url` 优先直返可匿名访问的跨域 HTTPS 签名地址；
若 ChatGPT 返回必须携带账号凭据的第一方地址，则由网关下载后回退为 data URL。
其他情况拿到图片字节后**合成上游 Responses 事件**，再交给既有的
`handleOpenAIImagesOAuthNonStreamingResponse` / `handleOpenAIImagesOAuthStreamingResponse`，
因此 `b64_json`/`url` 转换、usage、计费与错误分类与原生 OAuth 生图保持一致。
生图会话在结果已取得或失败退出后进入异步最佳努力清理，不阻塞客户端响应。

设备指纹（`OAI-Device-Id` / `OAI-Session-Id`）由账号标识派生 UUID，跨请求稳定。

## 启用

两种方式，**推荐第一种**。

**方式一：`web-image` 账号类型（限流状态与 OAuth 隔离）**

面板：账号管理 → 新建 → 平台选 OpenAI → 类型卡片里选 **ChatGPT Web**。选中后走的就是与 OAuth
相同的凭据输入流程（access_token / refresh_token / Codex 会话导入 / PAT 都可用），提交时写入
`type=web-image`；账号列表的类型徽章会显示 `Web Image`。

命令行等价写法——平台 `openai`、类型填 `web-image`：

```bash
curl -sS -X POST "https://<域名>/api/admin/accounts" \
  -H "Authorization: Bearer <管理员JWT>" -H 'Content-Type: application/json' \
  -d '{
    "name": "web-image-1",
    "platform": "openai",
    "type": "web-image",
    "credentials": {"access_token": "...", "refresh_token": "..."},
    "group_ids": [<生图分组ID>]
  }'
```

类型本身就是开关，不需要 extra。它只服务图片端点——`SupportsOpenAIEndpointCapability` 对
`type=web-image` 只放行空 capability（即图片请求），chat/responses 永远选不到它。于是同一 ChatGPT 账号
可以建两条记录：`oauth` 那条跑 Codex 文本，`web-image` 那条跑网页生图，两者的
`RateLimitResetAt` 互不影响——Codex 额度耗尽打出账号级 429 时，生图照样有号可用。

**方式二：OAuth 账号 + extra 开关**（保留，向后兼容）

`extra` 加 `{"chatgpt_web_image_generation": true}`，可选 `chatgpt_web_image_model`（缺省 `auto`）。
不设置时行为与原先完全一致。

## 凭据来源

`refresh_token` 只能来自 OpenAI OAuth 授权或已有 Codex session，浏览器 cookie 只适合临时换取
`access_token`。管理面板在选择 **ChatGPT Web Image** 后支持三条创建路径，均保持
`type=web-image`：

- OAuth 授权码：在有浏览器的电脑完成登录，面板交换 code 后创建账号。
- 手动 RT：输入一个或多行 refresh token，验证后创建账号。
- Codex session：导入 `~/.codex/auth.json` / session JSON；后端仅在同类型内查重，已有 OAuth
  记录不会被覆盖。

后台刷新候选、令牌缓存、OpenAI token refresher 和 OAuth refresh service 均包含 web-image；AT 临期后
使用 RT 续期，不依赖服务器浏览器。

## 验证

```bash
cd backend
go test -tags=unit ./internal/service ./internal/handler/admin ./internal/repository

# 直接给 token，或给浏览器分区 cookie 由测试现场换取 token
CGPT_WEB_E2E_COOKIE="<session cookie>" CGPT_WEB_E2E_PROXY="http://127.0.0.1:7890" \
CGPT_WEB_E2E_REF_IMAGE=/path/to/reference.png \
  go test -tags=e2e -v -timeout 900s -run 'TestChatGPTWebE2E' ./internal/service/
```

验证覆盖三层：

- 管理创建与导入：`web-image` 类型、AT/RT 持久化、Codex session 跨类型隔离。
- 调度与刷新：Codex 账号全局 429 时仍选中独立 web-image 记录；AT 临期用 RT 轮换。
- 转发：`SentinelAndImageQuota`、`GenerateImage` / `EditImage`、测试弹窗及最终标准 `b64_json` 响应。

离线 fixture 还覆盖了 `buildAccountForCreate → SelectAccountWithSchedulerForImages → ForwardImages →
ChatGPT Web 协议 → 图片响应`，无需真实凭据即可回归完整服务路径。

- `SentinelAndImageQuota`：sentinel 全流程与 `image_gen` 额度读取。
- `GenerateImage` / `EditImage`：session 层生成与参考图编辑，含图片字节格式校验。
- `ForwardImagesGenerations` / `ForwardImagesEdits`：**转发函数层**——直接调
  `forwardOpenAIImagesChatGPTWeb`，断言客户端最终拿到的是标准 `b64_json` 响应，
  覆盖"合成 Responses 事件 → 既有解析 → 编码写出"的完整下游。

## 失败语义

只按 HTTP 状态码与"是否为拦截页"归因，不猜错误文案：

| 上游信号 | 归因 | 动作 |
| --- | --- | --- |
| 401 / 403（非拦截页） | 账号凭据失效 | 写账号冷却 + 换号 |
| 429 / 402 | 网页额度或速率限制 | 写账号冷却 + 换号 |
| 5xx | 上游瞬时故障 | 换号 |
| Cloudflare 拦截页（`cf-mitigated` 或 HTML） | 出口被挑战 | 原样报错，不换号（换号无用） |
| 其它 4xx | 请求侧问题 | 原样报错 |

账号冷却是 10 分钟（`openai:image_generation` 模型级）：网页额度耗尽与短时速率限制在响应上都是
429，握手上无法区分，冷却期短一些让真正耗尽额度的账号会自动恢复调度。

对客户端的状态码：只有限额回 429，其余账号侧问题一律 502——不把"某个账号的凭证失效"暴露成
客户端错误。不带状态码的本地错误（网络/代理）保持既有语义：原样报错，不换号。

会话轮询的网络错误或 5xx 最多连续尝试 3 次，每次间隔 2 秒，始终查询原会话；鉴权、限额和挑战错误立即返回。成功查询会重置连续失败次数。上传初始化和确认接口使用同一分类逻辑；签名存储上传错误不作为 ChatGPT 账号失效处理。

## 面板测试

后台「测试账号连接」对网页通道账号会走网页链路：分流依据与转发完全同一处
（`Account.UsesChatGPTWebImageChannel()`），所以**测试通过即代表线上转发可用**，消耗的也是网页
`image_gen` 额度。

- `web-image` 类型账号：直接走网页测试。
- `oauth` / `setup-token` 账号：开了 `chatgpt_web_image_generation` 走网页测试，没开仍走 Codex
  `/responses` 的 `image_generation` 工具（原行为不变）。

测试模型选带 `gpt-image-` 前缀的条目即可（如显示为 "GPT Image 2.5 Sunburst"、ID 是
`gpt-image-2.5-sunburst`），模式会自动识别为生图测试。

## 已知限制

- **mask 不参与网页语义**：`/v1/images/edits` 的 mask 被忽略并记日志，参考图本身正常生效。
- **`n` 上限 4**：n 张 = n 次相互独立的网页会话并发执行（与参考实现 `ThreadPoolExecutor`
  的多线程语义一致），各次会话独立取 sentinel、独立落 ChatGPT 会话，`index` 不改写 prompt。
  超过 4 返回 400 `invalid_request_error`。上限保守是因为网页端每条会话都是一次真实浏览器回合，
  并发过高会触发账号滥用控制。部分失败时返回已成功的图片，全部失败才报错。
- **风控**：与其他网页逆向方案同级，避免高强度批量调用。

## Native-Image 与网页通道的边界

Native-Image 使用独立的 `/backend-api/codex/images/generations` 和 `/backend-api/codex/images/edits` 路径。
当前本地契约为单张 PNG、无 mask，不支持 partial_images、output_compression、moderation、style 或 input_fidelity。
调度会提前排除不兼容的 Native-Image 账号，保留转发前校验；普通 OAuth 请求仍走 Responses。
上游接受参数并不代表参数实际生效，不能据 HTTP 200 宣称质量、尺寸或格式已按请求执行。
