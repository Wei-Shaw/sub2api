# Review Round 1 — 编译 + 测试

## 检查结果

| 检查项 | 状态 | 详情 |
|---|---|---|
| go build ./... | ✅ | 全量编译通过，无错误 |
| gateway unit tests | ✅ | 7 passed, 0 failed (registry_test.go) |
| plugin-sdk unit tests | ✅ | 全部通过 — sdk: 73 passed, decimalx: 8 passed, driver: 11 passed, streamutil: 12 passed |
| handler tests (regression) | ❌ | 2 failures: 1 branch-introduced, 1 pre-existing |
| golangci-lint | ❌ | 3 gofmt issues in gateway package |
| gofmt | ❌ | 4 files need formatting |

## 发现的问题

### 问题 1: TestWeChatPaymentOAuthCallbackUsesExplicitPaymentResumeSigningKeyWhenMixedKeysConfigured 失败
- 文件：`backend/internal/handler/auth_wechat_oauth_test.go:467`
- 严重度：**blocker**
- 描述：本分支引入的回归。测试设置 `PAYMENT_RESUME_SIGNING_KEY` 为 `"explicit-payment-resume-signing-key"`（35 字节），但 `auth_wechat_payment_compat.go:201` 的 `parsePaymentResumeSigningKey()` 现在严格校验长度必须为 64 hex 字符或 32 raw 字节。35 字节不满足任何条件，导致解析失败、resume token 未签发、测试断言 `require.NoError` 失败。修复方案：将测试中的 `explicitSigningKey` 改为 32 字节或 64 hex 字符的合法值。

### 问题 2: TestAccountHandler_AccountIDsRequired 失败（预已存在）
- 文件：`backend/internal/handler/admin/account_handler_errors_test.go:75`
- 严重度：**warning（预已存在，非本分支引入）**
- 描述：期望 `reason` 为 `"INVALID_REQUEST_BODY"`，实际为空字符串。该测试文件及对应源文件在本分支无任何改动，属于 `release/custom-0.1.121` 已有的失败。

### 问题 3: gofmt 格式不一致
- 文件：
  - `backend/internal/gateway/anthropic_provider.go:23` — 多余空格对齐 `{ return "anthropic" }`
  - `backend/internal/gateway/antigravity_provider.go:25` — 同上
  - `backend/internal/gateway/openai_provider.go:23` — 同上
  - `backend/internal/gateway/result.go` — 格式不一致
- 严重度：**warning**
- 描述：golangci-lint 报告 3 个 gofmt 问题，`gofmt -l` 报告 4 个文件。运行 `gofmt -w` 即可修复。

## 结论

整体状态：**FAIL**

有 1 个本分支引入的 blocker（wechat payment resume 测试因签名密钥长度校验变严格而失败），需要修复后方可合并。另有 3-4 个 gofmt 格式问题需要清理。预已存在的 admin handler 测试失败不阻塞本分支。
