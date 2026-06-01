## Context

项目当前的验证码体系由三个层次组成：

1. **配置层** `captcha_config: map[string]string`，目前主要字段为 `{enabled, site_key, secret_key}`，外加顶层 `captcha_provider ∈ {turnstile, hcaptcha}`。
2. **服务端校验层** `CaptchaVerifier` 接口，方法签名 `VerifyToken(ctx, provider, secretKey, token, remoteIP) (*CaptchaVerifyResponse, error)`；目前由 `captchaHTTPVerifier` 对 Turnstile 与 hCaptcha 的 `siteverify` 端点做 HTTP 调用。
3. **前端层** `CaptchaWidget.vue` dispatcher 根据 provider 渲染 `TurnstileWidget.vue` / `HCaptchaWidget.vue`，二者均"声明式渲染 + `emit('verify', token)`"，4 个登录类视图（Login/Register/ForgotPassword/EmailVerify）以同一种模式消费。

腾讯天御（TencentCaptcha）协议与上述三层均有硬不兼容点：

- 前端凭证是 `ticket + randstr` 两字段；UX 是命令式 `cap.show()` 弹窗触发；存在容灾票据 `trerror_*` 路径。
- 服务端校验是腾讯云 API（`captcha.tencentcloudapi.com`），TC3-HMAC-SHA256 签名，需要 4 个字段：`CaptchaAppId / AppSecretKey / SecretId / SecretKey`。
- 一次校验 ticket 5 分钟有效且不可重用；票据校验失败的错误码区分不出"secret 错"还是"ticket 错"，传统"探活式 secret 校验"不适用。

约束：

- 不引入新的 Go 依赖（不引 `tencentcloud-sdk-go-captcha`）。
- 复用现有 `httpclient.GetClient`（带 SSRF 防护与统一超时）。
- 老 Turnstile / hCaptcha 行为不能回归。
- 升级要支持先后端再前端的灰度发布顺序。
- 现有 `UserIp` 取值方式（`c.ClientIP()`）保持不变，仅文档化其与 `trusted_proxies` 的依赖关系。

## Goals / Non-Goals

**Goals:**

- 在不破坏 Turnstile / hCaptcha 现有行为的前提下，原生支持腾讯天御 provider。
- 把"前端凭证 = 单 token"的隐式假设替换为显式 `captcha_payload` 字段映射，且保留 1 个发布周期的兼容字段。
- 把 `CaptchaVerifier` 接口收敛为对象形式，未来再加新 provider（如 reCAPTCHA、Geetest）成本可控。
- 天御使用其官方推荐的命令式 UX（点击提交 → 弹窗 → 验证 → 真正提交），用户感知与 Turnstile/hCaptcha 一致地"自动出现/被动展示"。
- 容灾票据 `trerror_*` 走严格策略：服务端永远拒绝，前端最多自动重试 1 次。
- `maskCaptchaConfig` 改为按 provider 选择脱敏字段集合，避免日后再加 provider 时反复修改硬编码列表。

**Non-Goals:**

- 不实现天御的 `aidEncrypted`（App ID 加密防盗刷）能力；如需要，未来在 `captcha_config` 中再扩字段。
- 不实现 SDK 化（不引入 `tencentcloud-sdk-go-captcha`）。
- 不实现 trusted_proxies 强校验或启动期阻断；仅在 admin 设置页加非阻断提示。
- 不实现 captcha provider 的灰度切换（A/B）、不实现"天御挂了自动降级 Turnstile/hCaptcha"的兜底。
- 不为 captcha 校验加缓存（票据本身一次性，5 分钟有效，无价值）。
- 不修改现有 captcha 中间件触发场景（哪些 API 需要 captcha 不在本次变更范围）。

## Decisions

### D1：前端 UX = 路 Y（提交时弹窗）

- **What**: `TencentCaptchaWidget` 在 `onMounted` 时只完成 SDK 加载与实例化（`new TencentCaptcha(appId, callback, opts)`），不立即调用 `cap.show()`。`CaptchaWidget` dispatcher 暴露一个统一的 `execute(): Promise<Payload | null>` 方法，由表单提交按钮回调中 `await captchaRef.value.execute()` 主动触发；天御 widget 内部以此触发 `cap.show()` 并把 callback 转成 promise resolve。
- **Why**:
  - 这是天御官方推荐 UX；用户拿到"无感验证"的红利。
  - 嵌入式（`type:'embed'`）虽然 UI 与现有 widget 一致，但放弃了天御的可疑度评分优势，且嵌入容器固定 300×230，与现有窄边登录卡片排版冲突。
  - 命令式 `execute()` API 同样适用于 Turnstile/hCaptcha——它们的 `execute()` 实现为"如果已有 token 直接 resolve；否则等用户先完成 widget 再 resolve"，对应现有 UI 不变。
- **Alternatives considered**:
  - 路 X：mounted 时直接 `cap.show()`——一进登录页就弹窗，体验割裂，否决。
  - 路 Z：嵌入式 `type:'embed'`——上面已说明缺点，否决。

### D2：API 协议 = 选项 B（统一 `captcha_payload`）

- **What**: 所有 captcha-gated DTO（`LoginRequest` / `RegisterRequest` / `SendVerificationEmailRequest` / `ForgotPasswordRequest` 等）新增字段 `captcha_payload: map[string]string`。后端读取顺序：`captcha_payload` 优先；若为空再回退兼容字段 `captcha_token` / `turnstile_token`。前端只发新字段（旧前端遇新后端兼容；新前端遇旧后端会因后端识别不出而验证失败——属于发布顺序约束）。
- **Why**:
  - D1 选了 Y 之后，前端 submit 流程已经被改造为 `await captchaRef.execute()` 返回 payload 对象，B 几乎是顺手落地；选 A 反而要再加一个 `captcha_extra` 平级字段，长期维护成本更高。
  - 未来再加 provider（如 Geetest 的 `lot_number / captcha_output / pass_token / gen_time`）不需要再扩协议。
  - `map[string]string` 与后端 `captcha_config` 的存储形态对称。
- **兼容窗口**: 1 个 release 周期。下一个 change 中移除 `captcha_token` / `turnstile_token` 兼容读取，写明 `**BREAKING**`。
- **Alternatives considered**:
  - 选项 A：增量 `captcha_extra: map`——保留双协议导致前端要维护两套提交逻辑，长期负担更大。

### D3：服务端 verifier 接口 = V2（请求对象）

- **What**: 把 `CaptchaVerifier` 改为：

  ```go
  type VerifyRequest struct {
      Provider string                       // "turnstile" | "hcaptcha" | "tencent_captcha"
      Config   map[string]string            // 完整 captcha_config，由 verifier 自取
      Payload  map[string]string            // {"token": "..."} 或 {"ticket": "...", "randstr": "..."}
      RemoteIP string                       // c.ClientIP() 直传
  }
  type VerifyResult struct {
      Success      bool
      ErrorCode    string                   // 归一化的错误码（详见 D6）
      ProviderMsg  string                   // 原始 provider 错误，用于日志
      EvilLevel    int                      // 仅 tencent_captcha 提供，其它 0
      RawResponse  map[string]any           // 仅在 debug 模式下落日志
  }
  type CaptchaVerifier interface {
      Verify(ctx context.Context, req VerifyRequest) (*VerifyResult, error)
      ValidateProviderConfig(ctx context.Context, provider string, config map[string]string) error
  }
  ```

  老的 `VerifyToken / ValidateProviderSecretKey` 旧签名在 service 层包一层，3 个月废弃窗口后删除。

- **Why**:
  - 天御的 4 个密钥字段塞不进 `secretKey string`，硬塞会让函数签名说谎。
  - `Config map[string]string` 直接对齐存储形态，verifier 内部自取所需字段，加 provider 时只动 verifier 自身。
  - `ValidateProviderConfig` 替代原 `ValidateProviderSecretKey`：Turnstile/hCaptcha 实现照旧（构造已知错的 token 做探活）；天御实现返回 `nil` 并附 warning（admin 端展示"无法预校验，请实测"）。
- **Alternatives considered**:
  - V1：在旧签名上塞 `extra map`——`secretKey` 字段名继续骗人，可读性差，长期变成两套并存。

### D4：TC3-HMAC-SHA256 签名 = 手写

- **What**: 在 `backend/internal/service/captcha_tencent.go`（新增）中手写 TC3 签名，按腾讯云官方文档实现以下 5 段：
  1. 拼接 `CanonicalRequest`（HTTPMethod / CanonicalURI / CanonicalQueryString / CanonicalHeaders / SignedHeaders / HashedRequestPayload）
  2. 拼接 `StringToSign`（`TC3-HMAC-SHA256\n<RequestTimestamp>\n<CredentialScope>\n<HashedCanonicalRequest>`）
  3. 派生 `SecretSigning`（`HMAC-SHA256(HMAC-SHA256(HMAC-SHA256("TC3"+SecretKey, Date), "captcha"), "tc3_request")`）
  4. 计算 `Signature = HMAC-SHA256(SecretSigning, StringToSign)`
  5. 组 `Authorization` header
  通过 `httpclient.GetClient()` POST `https://captcha.tencentcloudapi.com/`，超时 6s，重试 0 次（票据 5 分钟内失败重试无意义）。
- **Why**:
  - 不污染 `go.mod`；保留 SSRF 防护与统一超时配置。
  - TC3 签名是固定模板（约 80 行），代码评审一次即可后续无变化。
  - 项目暂无其他腾讯云能力规划，引入 SDK 主体不划算。
- **Alternatives considered**:
  - 引入 `tencentcloud-sdk-go-captcha`：SDK 内部自带 `http.Client`，绕过 `httpclient.GetClient`，失去 SSRF 防护边界，否决。

### D5：天御容灾票据 `trerror_*` = 严格模式（a）

- **What**:
  - **服务端**: `tencentCaptchaHTTPVerifier.Verify` 在调用云 API 之前先检查 `Payload["ticket"]` 是否以 `trerror_` 开头；若是，**不调用云 API**，直接返回 `VerifyResult{Success: false, ErrorCode: "captcha.tencent.fallback_ticket"}`。同时写一条 `WARN` 日志 `captcha.tencent.fallback_ticket_rejected` 含脱敏后的 RemoteIP，用于审计天御故障率。
  - **前端**: `TencentCaptchaWidget` callback 中识别 `ticket` 以 `trerror_` 开头时，**不** emit 该 payload，而是 emit `error` 事件，并在 widget state 里记一次"fallback 次数"。表单 submit 处理收到该错误时：
    - 第 1 次：清空 captcha 状态、提示"验证码服务异常，请重试"、自动再次 `await captchaRef.execute()`（用户会再次看到弹窗）。
    - 第 2 次及以后：不再自动重试，直接弹普通错误 toast、按钮恢复可点。
  - **不放行**: 即便天御侧大面积故障，本系统宁可登录链路受影响也不放风险通过。design 中显式拒绝（b）放行方案。
- **Why**:
  - 攻击者可随意伪造 `trerror_xxx` 字符串，"信任前端容灾"等于裸奔。
  - 1 次自动重试覆盖偶发网络抖动；2 次都 fallback 大概率是 SDK 加载持续失败或天御真挂了，此时不应该把"故障"伪装成"通过"。
- **Risks/Mitigation**: 见"Risks / Trade-offs"。

### D6：错误码归一化

- **What**: `VerifyResult.ErrorCode` 使用统一命名空间，便于前端与日志关联：
  - `captcha.invalid` —— 通用失败（Turnstile `invalid-input-response`、hCaptcha `invalid-input-response`、天御 `CaptchaCode != 1` 且非下列特殊码）
  - `captcha.config` —— 服务端配置错误（Turnstile `invalid-input-secret` / hCaptcha 同名 / 天御 `CaptchaCode in {6,7,15}`）
  - `captcha.timeout` —— 票据过期（Turnstile `timeout-or-duplicate`、天御 `CaptchaCode = 9`）
  - `captcha.duplicate` —— 票据重用（同上 turnstile 合并、天御 `CaptchaCode = 10`）
  - `captcha.tencent.fallback_ticket` —— **新增**，专用于 trerror_ 拒绝
  - `captcha.network` —— HTTP/超时错误（保留原行为）
- **Why**: handler 层据此决定返回给客户端的错误文案；admin 调试时能从单一字段快速判定问题类别。前端基于 `captcha.tencent.fallback_ticket` 实现 D5 的自动重试逻辑。

### D7：`maskCaptchaConfig` 改为 provider-aware

- **What**: 移除当前 `delete(masked, "secret_key")` 的硬编码，改成查表：

  ```go
  var captchaSecretFields = map[string][]string{
      "turnstile":       {"secret_key"},
      "hcaptcha":        {"secret_key"},
      "tencent_captcha": {"app_secret_key", "secret_id", "secret_key"},
  }
  ```

  脱敏策略与现有"删除字段"一致（GET 返回时不出现）。`*_configured: bool` 字段同步扩展到天御 3 个字段。

- **Why**: 天御有 3 个敏感字段而非 1 个；硬编码不可维护。

### D8：UserIp 不变 + 注释

- **What**: `tencentCaptchaHTTPVerifier` 在组装 `UserIp` 参数时使用入参 `req.RemoteIP`（即上游 `c.ClientIP()`）。在该字段赋值处加注释：

  ```go
  // UserIp 直接使用 gin 的 ClientIP（已按 trusted_proxies 规则解析）。
  // 注意：未配置 trusted_proxies 时该值可能是内网/网关 IP，会降低
  // 天御风控的评分准确性，但不影响功能（评分不是通过性的硬门槛）。
  ```

- **Why**: 用户明确决策"不强制 trusted_proxies、不做私网回退兜底"。在代码处留注释是最低成本的可发现性手段。

### D9：迁移与回滚

- **Migration**: **无需新增 migration**。仓库现状是 `captcha_provider` 存储在通用 `settings(key, value)` 表中，没有 ENUM 类型也没有 CHECK 约束（参见 `backend/migrations/142_captcha_provider.sql` 仅做 `INSERT ... ON CONFLICT DO NOTHING` 种子写入）。新增 `'tencent_captcha'` 取值仅在应用层 `normalizeCaptchaProvider` 中放开即可。仓库同时遵循 `backend/migrations/README.md` L47 的"forward-only migration"约定（不在同文件写 down SQL，不存在 `make migrate-down` 通道），不应为 no-op 凭空增加 migration 文件污染 schema 历史。
- **Rollback**: 因为没有 schema 变更，回滚等同于在 admin 后台把 `captcha_provider` 切回 `turnstile` 或 `hcaptcha`；如果发现"已启用天御 + 后端代码已回滚"的窗口期，由于旧后端的 `normalizeCaptchaProvider` 不识别 `tencent_captcha`，会归一为默认 provider（实测以现行实现为准），不会写脏数据也不会出现约束冲突。
- **Release order**:
  1. **后端先行**: 同时支持新旧 payload 字段；`normalizeCaptchaProvider` 接受 `tencent_captcha`；mask/PublicSettings 等 provider-aware 改造落地。此时前端旧版仍工作。
  2. **前端跟进**: 切到 `captcha_payload`；admin 设置页放开天御 provider 选项。
  3. **下一个 release**: 移除后端兼容字段读取（届时另起 OpenSpec change 标 BREAKING）。

## Risks / Trade-offs

- **天御侧大面积故障 → 严格 trerror_ 致登录可用性受损** → 接受。已知风险，design 中显式 non-goal 不实现自动降级。如运营观察到长时间故障，可临时把 admin 设置改回 turnstile/hcaptcha provider 紧急切换；评估期内此切换由人工执行。
- **前端老版本部署到新后端期间，新加 provider 选项无法在 UI 选择** → 用户控制 admin 后台前端版本（与后端同步发布），影响窗口可控。
- **TC3 签名手写代码错误 → 所有验证请求 401** → 上线前以"真 AppId/SecretId/SecretKey + 已知 ticket"打通至少 1 次端到端；并新增单测对照腾讯云签名示例向量。
- **`Verify` 接口重构波及面 → 测试覆盖** → Turnstile/hCaptcha 既有单测改为新签名后保留逻辑，新增 tencent verifier 的独立单测；CaptchaService 层加一个并行的 table-driven 测试覆盖三 provider。
- **trusted_proxies 未配 → UserIp 不准 → 天御评分降级** → 接受。代码注释 + admin 提示已落地；功能不阻断。
- **浏览器加载 `TCaptcha.js` 失败（CDN/网络）→ 用户卡死** → D5 的 1 次自动重试覆盖偶发；持续失败时弹"验证码加载失败，请刷新页面或稍后重试"，与现有 Turnstile 加载失败行为对齐。
- **`captcha_payload` 与老字段并存期，开发者误读** → handler 层封装单一 helper `extractCaptchaPayload(req)`，禁止业务代码直接读 DTO 字段；通过 lint/code-review 控制。

## Migration Plan

1. **后端发版**：`normalizeCaptchaProvider` 接受 `tencent_captcha`；新 verifier 接口 + tencent verifier 实现 + `captcha_payload` 双协议读取 + maskCaptchaConfig 改造。CI 必须包含新签字单测与 tencent verifier 单测。**无 DB migration**（参见 D9）。
2. **前端发版**：dispatcher `execute()` API + TencentCaptchaWidget + 4 个 view submit 流程改造 + admin 设置页天御表单。
3. **灰度**：选 1 个 staging 环境填入天御真实密钥，跑完整登录/注册/找回密码链路，含人工模拟"提交后断网"验证 trerror_ 严格策略生效。
4. **生产启用**：admin 后台切换 provider 到 tencent_captcha。
5. **下一个 release（非本变更范围）**：移除后端 `captcha_token` / `turnstile_token` 兼容读取，配套 BREAKING 标记。

**Rollback**:

- 灰度/生产阶段发现问题：admin 后台切回 turnstile/hcaptcha 即可（无需回滚代码）。
- 后端发版后发现问题：直接回滚到上一 tag；因无 schema 变更，无需任何 DB 操作。前端因兼容字段保留，可独立回滚。

## Open Questions

- **Q1**: `captcha_payload` 在 `SendVerificationEmailRequest` 等"前置发邮件类"接口里是否同样接入？目前现状是这些接口已经在 captcha 体系内（与 Login/Register/Forgot 相同），按一致性原则一并改造。如有遗漏接口需在 tasks.md 中显式扫描列出。
- **Q2**: 天御 `CaptchaType` 字段取值默认 9（Web 端），是否需要做成可配置（移动端/小程序客户端复用本服务时切换）？本期 default 写死为 `9`；如未来出现多端共用，再扩 `captcha_config.tencent_captcha_type` 字段。
- **Q3**: trerror_ 拒绝时是否要在 HTTP 响应中向客户端透出"专属错误码"以便前端区分？design 中假定**会**通过统一错误响应 `code: "captcha.tencent.fallback_ticket"` 透出（D6）。如最终响应结构不允许带 code 字段，前端只能基于 message 文案匹配，可读性下降——以实际 ResponseEnvelope 实现为准。
