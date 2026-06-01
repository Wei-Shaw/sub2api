# Captcha

## Purpose

Provide a pluggable captcha verification layer that supports multiple providers (Turnstile, hCaptcha, Tencent Captcha) with unified request/response interfaces, seamless frontend widget dispatching, and admin-configurable provider switching.
## Requirements
### Requirement: Multi-Provider Captcha Selection

The system SHALL allow administrators to select a captcha provider from a fixed enumeration and provide provider-specific credentials. Supported providers SHALL be `turnstile`, `hcaptcha`, and `tencent_captcha`. Exactly one provider is active at any time across all captcha-gated endpoints.

#### Scenario: Admin selects Turnstile provider

- **WHEN** an administrator sets `captcha_provider = "turnstile"` and provides `site_key` and `secret_key` in `captcha_config`, with `enabled = "true"`
- **THEN** the system persists the configuration, the public settings endpoint exposes `captcha_provider = "turnstile"` and `captcha_site_key = <site_key>`, and the secret value is never returned in any GET response

#### Scenario: Admin selects hCaptcha provider

- **WHEN** an administrator sets `captcha_provider = "hcaptcha"` and provides `site_key` and `secret_key` in `captcha_config`, with `enabled = "true"`
- **THEN** the system persists the configuration, the public settings endpoint exposes `captcha_provider = "hcaptcha"` and `captcha_site_key = <site_key>`, and the secret value is never returned in any GET response

#### Scenario: Admin selects Tencent Captcha provider

- **WHEN** an administrator sets `captcha_provider = "tencent_captcha"` and provides `captcha_app_id`, `app_secret_key`, `secret_id`, and `secret_key` in `captcha_config`, with `enabled = "true"`
- **THEN** the system persists the configuration, the public settings endpoint exposes `captcha_provider = "tencent_captcha"` and `captcha_site_key = <captcha_app_id>` (the app id is reused into the existing public field), and none of `app_secret_key`, `secret_id`, `secret_key` are returned in any GET response

#### Scenario: Unknown provider rejected

- **WHEN** an administrator submits `captcha_provider` with any value other than the three supported provider names
- **THEN** the request is rejected with a validation error and the existing configuration is unchanged

### Requirement: Captcha Config Sensitive Field Masking

The system SHALL mask provider-specific sensitive fields when returning `captcha_config` through any non-admin or read-back path. The set of masked fields SHALL be selected based on the configured provider. For each masked field the system SHALL emit a parallel `<field>_configured: bool` indicator.

#### Scenario: Masking for Turnstile

- **WHEN** `captcha_provider = "turnstile"` and a settings GET response is produced
- **THEN** the response omits `secret_key` from `captcha_config` and includes `secret_key_configured: true | false`

#### Scenario: Masking for hCaptcha

- **WHEN** `captcha_provider = "hcaptcha"` and a settings GET response is produced
- **THEN** the response omits `secret_key` from `captcha_config` and includes `secret_key_configured: true | false`

#### Scenario: Masking for Tencent Captcha

- **WHEN** `captcha_provider = "tencent_captcha"` and a settings GET response is produced
- **THEN** the response omits all of `app_secret_key`, `secret_id`, `secret_key` from `captcha_config` and includes `app_secret_key_configured`, `secret_id_configured`, `secret_key_configured` booleans

### Requirement: Public Captcha Settings Exposure

The system SHALL expose captcha public settings via an unauthenticated endpoint and via SSR-injected payload so that login/registration views can render captcha without prior authentication. The exposed fields SHALL include `captcha_enabled`, `captcha_provider`, and `captcha_site_key`. The `captcha_site_key` field SHALL contain the provider's public identifier: Turnstile site key, hCaptcha site key, or Tencent Captcha `CaptchaAppId` respectively.

#### Scenario: Public settings response for Tencent Captcha

- **WHEN** an unauthenticated client calls the public settings endpoint while `captcha_provider = "tencent_captcha"` and `captcha_app_id = "200000000"`
- **THEN** the response contains `captcha_provider = "tencent_captcha"`, `captcha_site_key = "200000000"`, `captcha_enabled = true`, and contains none of `app_secret_key`, `secret_id`, `secret_key`

#### Scenario: Backward-compatible legacy fields

- **WHEN** a client reads the public settings response and falls back to legacy fields `turnstile_site_key` / `turnstile_enabled` (older frontend versions)
- **THEN** the response still populates those legacy fields with the same values for the active provider so existing clients keep working during the compatibility window

### Requirement: Unified Captcha Payload Protocol

The system SHALL accept captcha credentials from clients via a single structured field `captcha_payload: map<string, string>` on all captcha-gated request DTOs (Login, Register, SendVerificationEmail, ForgotPassword, and any future captcha-gated endpoint). Provider-specific keys inside the map SHALL be:

- `turnstile`: `{ "token": "<turnstile-response>" }`
- `hcaptcha`: `{ "token": "<hcaptcha-response>" }`
- `tencent_captcha`: `{ "ticket": "<ticket>", "randstr": "<randstr>" }`

The system SHALL also continue accepting the legacy single-field forms `captcha_token` and `turnstile_token` for one release cycle; when both are present, `captcha_payload` takes precedence.

#### Scenario: New client submits structured payload

- **WHEN** a client submits a captcha-gated request with `captcha_payload = { "ticket": "abc", "randstr": "xyz" }` while `captcha_provider = "tencent_captcha"`
- **THEN** the server uses the structured payload for verification and ignores any legacy `captcha_token` field even if present

#### Scenario: Legacy client submits flat token

- **WHEN** an older client submits a captcha-gated request with `captcha_token = "tok"` and no `captcha_payload`, while `captcha_provider = "turnstile"`
- **THEN** the server normalizes the input to `{ "token": "tok" }` and proceeds with verification

#### Scenario: Both fields present

- **WHEN** a client submits both `captcha_payload = { "token": "new" }` and `captcha_token = "old"`
- **THEN** the server uses `captcha_payload` and ignores `captcha_token`

### Requirement: Captcha Verifier Interface (Request-Object Form)

The system SHALL expose a captcha verifier interface that accepts a request object containing `Provider`, `Config` (the full `captcha_config` map), `Payload` (the structured client payload), and `RemoteIP`, and returns a result object containing `Success`, normalized `ErrorCode`, `ProviderMsg`, and provider-specific extras. The interface SHALL also expose a `ValidateProviderConfig` method for admin-side pre-flight checks.

#### Scenario: Verifier dispatches by provider

- **WHEN** the verifier receives a `VerifyRequest` with `Provider = "tencent_captcha"`
- **THEN** the verifier reads `captcha_app_id`, `app_secret_key`, `secret_id`, `secret_key` from `Config`, reads `ticket`, `randstr` from `Payload`, and dispatches to the Tencent Captcha verification path

#### Scenario: Missing required payload key

- **WHEN** the verifier receives `Provider = "tencent_captcha"` but `Payload` is missing either `ticket` or `randstr`
- **THEN** the verifier returns `Success = false` with `ErrorCode = "captcha.invalid"` and does not perform an outbound HTTP call

#### Scenario: ValidateProviderConfig for Tencent Captcha

- **WHEN** an administrator saves captcha settings with `captcha_provider = "tencent_captcha"` and `ValidateProviderConfig` is invoked
- **THEN** the method returns `nil` (success) without performing any network call, and the system surfaces an advisory message indicating that Tencent Captcha credentials cannot be pre-validated and must be exercised by a real verification

#### Scenario: ValidateProviderConfig for Turnstile / hCaptcha

- **WHEN** an administrator saves captcha settings with `captcha_provider = "turnstile"` or `"hcaptcha"`
- **THEN** the method performs an HTTP probe with a deliberately invalid token against the provider's siteverify endpoint and returns success if the response indicates "invalid token" (not "invalid secret"), or returns an error if the response indicates "invalid secret"

### Requirement: Tencent Captcha Server-Side Verification

The system SHALL verify Tencent Captcha tickets by invoking the `DescribeCaptchaResult` action on `https://captcha.tencentcloudapi.com/` using TC3-HMAC-SHA256 request signing. The request SHALL include `CaptchaType = 9`, `Ticket`, `Randstr`, `UserIp`, `CaptchaAppId`, `AppSecretKey`, and (in the optional `BusinessId`/`SceneId` parameters only when configured) business identifiers. The verification SHALL succeed if and only if the response field `CaptchaCode == 1`.

#### Scenario: Successful verification

- **WHEN** the Tencent Captcha API responds with `Response.CaptchaCode = 1`
- **THEN** the verifier returns `Success = true` and `EvilLevel` populated from the response (defaulting to `0` if absent)

#### Scenario: CaptchaCode indicates ticket failure

- **WHEN** the Tencent Captcha API responds with `Response.CaptchaCode != 1` and the code does not map to a configuration-class error
- **THEN** the verifier returns `Success = false` with `ErrorCode = "captcha.invalid"` and surfaces the raw `CaptchaMsg` in `ProviderMsg`

#### Scenario: CaptchaCode indicates configuration error

- **WHEN** the Tencent Captcha API responds with `Response.CaptchaCode ∈ {6, 7, 15}` (decrypt failure / signature failure / app-secret mismatch)
- **THEN** the verifier returns `Success = false` with `ErrorCode = "captcha.config"`

#### Scenario: CaptchaCode indicates expired ticket

- **WHEN** the Tencent Captcha API responds with `Response.CaptchaCode = 9`
- **THEN** the verifier returns `Success = false` with `ErrorCode = "captcha.timeout"`

#### Scenario: CaptchaCode indicates duplicate ticket use

- **WHEN** the Tencent Captcha API responds with `Response.CaptchaCode = 10`
- **THEN** the verifier returns `Success = false` with `ErrorCode = "captcha.duplicate"`

#### Scenario: HTTP transport error

- **WHEN** the outbound HTTP call to `captcha.tencentcloudapi.com` fails (DNS, TLS, timeout, non-2xx)
- **THEN** the verifier returns `Success = false` with `ErrorCode = "captcha.network"` and surfaces the underlying error string in `ProviderMsg`

#### Scenario: HTTP client reuse

- **WHEN** the verifier performs any outbound HTTP call to the Tencent Captcha API
- **THEN** the call goes through the project-shared HTTP client (with SSRF protection and unified timeout) rather than a verifier-local `http.Client`

### Requirement: Tencent Captcha Fallback Ticket Rejection

The system SHALL reject any Tencent Captcha ticket whose value begins with the literal prefix `trerror_`. Rejection SHALL occur **before** any outbound HTTP call to the Tencent Captcha API.

#### Scenario: Fallback ticket detected

- **WHEN** the verifier receives `Payload["ticket"] = "trerror_1001"` (any string starting with `trerror_`)
- **THEN** the verifier returns `Success = false` with `ErrorCode = "captcha.tencent.fallback_ticket"`, performs no outbound HTTP call, and writes a WARN-level audit log entry with the truncated/anonymized remote IP

#### Scenario: Normal ticket starts with similar but non-matching prefix

- **WHEN** the verifier receives `Payload["ticket"] = "trerr_xxx"` or `"t_xxx"` (does not match `trerror_` literally)
- **THEN** the verifier proceeds to the normal verification path

### Requirement: Frontend Imperative Captcha Trigger API

The captcha widget dispatcher (`CaptchaWidget`) SHALL expose a method `execute(): Promise<Payload | null>` to the parent form. Calling `execute()` SHALL resolve with the structured `captcha_payload` to send to the server, or `null` if the user cancelled the verification. Each provider-specific widget (`TurnstileWidget`, `HCaptchaWidget`, `TencentCaptchaWidget`) SHALL implement this method consistently.

#### Scenario: Tencent widget execute() triggers popup

- **WHEN** a parent form calls `await captchaRef.execute()` while `captcha_provider = "tencent_captcha"`
- **THEN** the widget invokes `cap.show()` on its `TencentCaptcha` instance; on a successful callback (`ret = 0`) the promise resolves to `{ ticket, randstr }`; on a user-cancelled callback (`ret = 2`) the promise resolves to `null`

#### Scenario: Turnstile widget execute() returns cached token

- **WHEN** a parent form calls `await captchaRef.execute()` while `captcha_provider = "turnstile"` and the user has already completed the inline widget
- **THEN** the promise resolves to `{ token: "<cached-token>" }` without re-rendering the widget

#### Scenario: Turnstile widget execute() before completion

- **WHEN** a parent form calls `await captchaRef.execute()` while `captcha_provider = "turnstile"` and the user has not yet completed the widget
- **THEN** the promise stays pending until the user completes the widget (resolves with the new token) or the widget errors (resolves to `null`)

### Requirement: Frontend Submit-Time Captcha Flow

All captcha-gated forms (Login, Register, ForgotPassword, EmailVerify) SHALL intercept the user's submit action, call `await captchaRef.execute()` first, and proceed with the actual API request only after a non-null payload is obtained. While `execute()` is pending, the submit button SHALL be disabled.

#### Scenario: Successful flow

- **WHEN** the user clicks the submit button on the Register form with captcha enabled
- **THEN** the form first awaits `captchaRef.execute()`; upon resolution with a non-null payload, the form sends the register request with `captcha_payload` set to the resolved value

#### Scenario: User cancels captcha popup

- **WHEN** `captchaRef.execute()` resolves with `null` (user cancelled)
- **THEN** the form does not send the register request, re-enables the submit button, and displays no error toast

### Requirement: Frontend Fallback Ticket Retry Policy

Frontend captcha-gated forms SHALL implement strict retry policy for Tencent Captcha fallback tickets: when the server response indicates `captcha.tencent.fallback_ticket`, the form SHALL automatically clear captcha state and re-invoke `execute()` exactly once. A second consecutive `captcha.tencent.fallback_ticket` SHALL surface a normal error toast and SHALL NOT trigger another automatic retry.

#### Scenario: First fallback triggers auto-retry

- **WHEN** the server returns `ErrorCode = "captcha.tencent.fallback_ticket"` on the first submit attempt
- **THEN** the form clears the captcha widget state, awaits `captchaRef.execute()` again, and re-submits with the new payload, without showing an error toast to the user

#### Scenario: Second fallback shows error

- **WHEN** the server returns `ErrorCode = "captcha.tencent.fallback_ticket"` on the auto-retried submission
- **THEN** the form displays an error toast (e.g., "验证码服务异常，请稍后重试"), re-enables the submit button, and does not retry automatically

#### Scenario: Frontend-side fallback ticket detection

- **WHEN** the Tencent Captcha SDK invokes its callback with `ticket` starting with `trerror_`
- **THEN** the widget does not emit a `verify` event with that payload; it emits an internal `fallback` event so the form can apply the same retry-once policy without round-tripping to the server

### Requirement: Tencent Captcha JS SDK Loading

The frontend SHALL dynamically load the Tencent Captcha JS SDK from `https://turing.captcha.qcloud.com/TCaptcha.js` on demand, no earlier than when a captcha widget component is mounted with `provider = "tencent_captcha"`. The SDK SHALL NOT be loaded for users on Turnstile or hCaptcha providers.

#### Scenario: SDK loaded once per page

- **WHEN** the user navigates to a page that mounts `TencentCaptchaWidget` for the first time in the session
- **THEN** the SDK script is injected into the document head; subsequent mounts within the same page do not inject the script again

#### Scenario: SDK load failure

- **WHEN** the SDK script fails to load (network error, CSP block, etc.) within 10 seconds
- **THEN** the widget surfaces an error state; calling `execute()` immediately resolves to `null` and the form treats this as a captcha failure

### Requirement: UserIp Source Stability

The system SHALL pass the user's IP address to the Tencent Captcha API using the value returned by the Gin framework's `Context.ClientIP()` method (which honors the configured `trusted_proxies` chain). The system SHALL NOT apply additional fallback logic (such as scanning headers for the first non-RFC1918 address) and SHALL NOT block requests on missing or private IPs.

#### Scenario: Private IP forwarded

- **WHEN** `c.ClientIP()` returns a private/loopback address because `trusted_proxies` is not configured
- **THEN** that address is still passed as `UserIp` to the Tencent Captcha API; verification proceeds and may receive a lower-quality risk score, but does not fail on that basis

