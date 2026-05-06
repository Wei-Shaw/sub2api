# Gateway Extraction · POC-B · Token Provider 跨进程解耦分析

> 状态：调研报告，不修改任何代码。
> 关联：`docs/plugin-architecture/GATEWAY-EXTRACTION-PROPOSAL.md` §7（风险表）+ §10 Q3。
> 范围：回答"OAuth refresh token 是否能完全在 plugin 进程做？host 留什么 RPC？"；同时覆盖 §7 的"Bedrock SigV4 / Vertex Service Account 凭据泄漏"风险。

---

## 1. 三个 token provider 状态机

三者的核心入口 `GetAccessToken()` 走同一个 4 阶段管道：**cache hit → needs refresh → call OAuthRefreshAPI → set cache TTL**。差异主要落在 `ProviderRefreshPolicy`（`backend/internal/service/refresh_policy.go:26-62`）和"刷新失败后的副作用"。

公共支撑层：

- `OAuthRefreshAPI.RefreshIfNeeded`（`backend/internal/service/oauth_refresh_api.go:75-166`）= 进程内 `sync.Mutex` 按 cacheKey 序列化 + Redis `AcquireRefreshLock` 分布式锁（默认 60 s TTL）+ 锁内 `accountRepo.GetByID` 重读 + executor.Refresh + `persistAccountCredentials`（写 `_token_version` 时间戳）。
- `OAuthRefreshExecutor` 接口（`oauth_refresh_api.go:15-20`）= `TokenRefresher` 超集，加 `CacheKey(account)`。每个平台实现一个 executor 调用上游 OAuth endpoint。
- `account.GetCredentialAsTime("expires_at")` + `xxxTokenRefreshSkew = 3 min` 决定 needsRefresh。
- `CheckTokenVersion` 用 `_token_version` 字段（`oauth_refresh_api.go:148-149` 写入）做"另一路径已写入更新版本"的乐观检测。

### 1.1 ClaudeTokenProvider

文件：`backend/internal/service/claude_token_provider.go:21-167`。

**in-memory 状态**：无（除注入的 `accountRepo`、`tokenCache`、`oauthService`、`refreshAPI`、`executor`、`refreshPolicy`）。

**读 Account 字段**：`Platform`、`Type`（区分 OAuth 与 ServiceAccount=Vertex）、`Credentials.access_token`、`Credentials.expires_at`、`Credentials.refresh_token`（在 executor 里）、`Credentials._token_version`、`Credentials.service_account_json`（Vertex 分支）。

**写 Account 字段**：仅刷新成功后通过 `persistAccountCredentials` 整体替换 `Credentials`（含 `access_token / expires_at / refresh_token / _token_version`）。

**refresh 触发条件**：`expiresAt == nil || time.Until(expiresAt) <= 3min`（`claudeTokenRefreshSkew`）。

**并发控制**：
- 进程内：`OAuthRefreshAPI.localLocks sync.Map[cacheKey]*sync.Mutex`，按 cache key 串行化（`oauth_refresh_api.go:56-64`）。
- 进程间：Redis `AcquireRefreshLock(cacheKey, 60s)`。
- 锁竞争 fallback：`policy.OnLockHeld == ProviderLockHeldWaitForCache` → `time.Sleep(200ms)` 后重读 cache（`claude_token_provider.go:93-104`）。

**错误处理**：`policy.OnRefreshError == ProviderRefreshErrorUseExistingToken`（`refresh_policy.go:32-38`）→ 不向请求路径返回错误，落入 cache 短 TTL（1 min）继续用旧 token，不触发"标记账号 error / auto-pause"。Vertex 子分支走 `getVertexServiceAccountAccessToken`（见 §3.2）。

### 1.2 OpenAITokenProvider

文件：`backend/internal/service/openai_token_provider.go:79-307`。

**in-memory 状态**：`metrics *openAITokenRuntimeMetricsStore`（10 个 `atomic.Int64` 计数器，给 admin 监控 RefreshRequests/Success/Failure/LockContention/LockWaitSamples）。

**读/写 Account 字段**：与 Claude 相同。额外用 `account.GetOpenAIAccessToken()` 在 stale 路径读 access_token。

**refresh 触发条件**：`time.Until(expiresAt) <= 3min`。

**并发控制**：与 Claude 相同 + 锁竞争退避算法（`waitForTokenAfterLockRace`，5 次抓 cache，20→120 ms 指数退避 + ±20% jitter，`openai_token_provider.go:258-307`）。

**错误处理**：与 Claude 相同（`OnRefreshError == UseExistingToken`，FailureTTL=1 min），但每个分支会更新 metrics。

### 1.3 AntigravityTokenProvider

文件：`backend/internal/service/antigravity_token_provider.go:27-227`。

**in-memory 状态**：`backfillCooldown sync.Map[accountID]time.Time`（5 min 冷却，避免 project_id backfill 风暴）；可选 `tempUnschedCache TempUnschedCache`（Redis 临时不可调度缓存）。

**读 Account 字段**：同 Claude；额外读 `Credentials.api_key`（Upstream 类型直接返回）、`Credentials.project_id`（缺失时背调）。

**写 Account 字段**：刷新写整 Credentials；project_id backfill 单独写 `Credentials.project_id`（`antigravity_token_provider.go:139-148`）；refresh 失败时调 `accountRepo.SetTempUnschedulable(account.ID, until=now+10min, reason)` + `tempUnschedCache.SetTempUnsched`（`antigravity_token_provider.go:193-227`）。这是三家里**唯一**会自动改账号调度位的 provider。

**refresh 触发条件**：`time.Until(expiresAt) <= 3min`，且对请求路径用 `context.WithTimeout(ctx, 8 s)`（`antigravityRequestRefreshTimeout`，`antigravity_token_provider.go:101-104`）防止代理超时阻塞。

**并发控制**：同 Claude，但策略是 `OnLockHeld == ProviderLockHeldUseExistingToken`（`refresh_policy.go:56-62`）→ 锁被持时不等 cache，直接用 stale token 走完请求。

**错误处理**：`OnRefreshError == ProviderRefreshErrorReturn` + 必走 `markTempUnschedulable`（双写 DB + Redis），让调度器立刻跳过该账号；后台 `TokenRefreshService` 下个周期继续重试。

---

## 2. Account.Credentials 写回路径

**关键事实：Account.Credentials 在 DB 里以 datatypes.JSONMap (UTF-8 JSON) 明文存储，没有应用层加密**（`backend/internal/repository/account_repo.go:341-441`，全程只调 `SetCredentials(normalizeJSONMap(...))`）。`backend/internal/repository/aes_encryptor.go` 提供的 `AESEncryptor` 仅用于 TOTP 与微信支付凭据等极少数字段，不覆盖账号 OAuth credentials。

**唯一应用级加密在 plugin SDK 侧**（`backend/internal/plugin/secret_encryption_server.go:1-246`），用 HKDF 派生 per-plugin 32B 密钥 + AES-GCM，但只用于 plugin 自己的 Settings/Secret，**与账号 credentials 无关**。

写回路径（统一入口 `service/account_credentials_persistence.go:9-19`）：

1. Token provider 调 `persistAccountCredentials(ctx, repo, account, newCreds)`：先 clone，再优先 `repo.UpdateCredentials(id, creds)`（仅写 credentials 列），失败回退 `repo.Update(account)`（全字段）。
2. 进入 `accountRepository.UpdateCredentials`（`backend/internal/repository/account_repo.go:432-441`）：
   - `client.Account.UpdateOneID(id).SetCredentials(normalizeJSONMap(creds)).Save(ctx)`
   - `r.syncSchedulerAccountSnapshot(ctx, id)` 主动刷新调度器缓存（避免 outbox worker 延迟）。
3. **没有 audit log**：repo 层只 `enqueueSchedulerOutbox` 走调度事件（仅在 `Update` 全字段路径，`UpdateCredentials` 路径连 outbox 都不写，假定调度位字段未变）。
4. 写完后下次 `accountRepo.GetByID` 即返回新 credentials。

**对 plugin 的影响**：
- credentials 进出 host 进程都是明文 JSON（无对称加密 wrap），所以"host 加密 / plugin 解密"这条边界在账号侧目前**不存在**。未来要做"plugin 不持有原 refresh_token"必须**新增**机制（见 §6）。
- `_token_version` 是事实上的 CAS 字段（毫秒时间戳）；`OAuthRefreshAPI` 用它做并发恢复（`oauth_refresh_api.go:175-193`）。跨进程后 plugin 必须遵守同一 invariant：**写回 credentials 必须带 host 已知的 last version 做比较**。

---

## 3. Bedrock / Vertex 特殊性

### 3.1 Bedrock（Anthropic 子类，`Type=AccountTypeBedrock`）

文件：`backend/internal/service/bedrock_signer.go:1-67`。

- **凭据形式**：long-lived AWS access key（`aws_access_key_id` / `aws_secret_access_key`），可选 `aws_session_token`，加 `aws_region`。也可走 `auth_mode=apikey`（`account.go:940-942`）即 Bedrock API Key（短 token）。
- **签名时机**：每个上游请求都 `signer.SignHTTP(ctx, creds, req, sha256(body), "bedrock", region, time.Now())`（SigV4，**无中间 access token**，无缓存）。`BedrockSigner` 是无状态对象，构造一次复用即可。
- **没有 OAuth refresh 概念**：credentials 是 IAM 静态密钥，过期由 AWS 控制台轮换驱动，host 不主动刷新。
- **跨进程影响**：要么 host 把明文 access key + secret 通过 GetAccount RPC 发给 plugin（同 OAuth refresh_token 同等敏感度），要么 host 暴露 `SignBedrockRequest(method, url, headers, bodyHash) → signed_headers` RPC，每请求一次往返。考虑 SigV4 计算成本极低（µs 级），**第二种把长寿密钥锁在 host 进程的方案显著更好**。

### 3.2 Vertex Service Account（Anthropic & Gemini 共用，`Type=AccountTypeServiceAccount`）

文件：`backend/internal/service/vertex_service_account.go:1-249`。

- **凭据形式**：Google Service Account JSON（含 `private_key` PEM），存在 `Credentials.service_account_json`（字符串或嵌套 map）。
- **token 流程**：`getVertexServiceAccountAccessToken` 用 RS256 JWT bearer flow 换 1h access_token，scope=`https://www.googleapis.com/auth/cloud-platform`。结果缓存到 `tokenCache`（cacheKey=`vertex:service_account:<sha256(client_email+kid)前 8B>`），cache miss 时尝试 `AcquireRefreshLock(30s)`，竞争时 `time.Sleep(200ms)` 重读。
- **不写 Account.Credentials**：Vertex JWT 换 token 不消耗、不轮换 service-account key；token 只进缓存，不回写 DB。所以 Vertex 不存在"OAuth refresh race 写回 DB"问题。
- **跨进程影响**：private key（PEM 格式 RSA 私钥）一旦交给 plugin，泄漏成本极高。建议同 Bedrock：host 暴露 `MintVertexAccessToken(account_id, scope) → access_token + expires_at` RPC，plugin 只拿短期（≤1h）access_token。

### 3.3 对比总结

| 凭据类型 | 短期 / 长期 | 跨进程透传到 plugin？ | host 必须做的事 |
|---|---|---|---|
| Anthropic / OpenAI / Antigravity OAuth `access_token` | 短期 (<1h) | 可以透传 | refresh + DB 写回 + 加锁 |
| Anthropic / OpenAI / Antigravity OAuth `refresh_token` | 长期（数月） | **不应透传**到 plugin 进程 | 自己持有 + 暴露 refresh RPC |
| Bedrock IAM access key + secret | 长期（IAM 轮换） | **不应透传** | 暴露 `SignBedrockRequest` RPC |
| Vertex Service Account JSON (含 private key) | 长期 | **不应透传** | 暴露 `MintVertexAccessToken` RPC |
| Vertex / OAuth 派生的 access_token | 短期 | 可以透传 | 已在 host 缓存，按需返回 |
| Antigravity Upstream `api_key` | 长期 | 可以透传（本就是给 plugin 用） | 透明返回 |

---

## 4. 推荐的 HostService RPC 集

现有 `HostService`（`plugin-sdk/proto/sdk.proto:1014-1052`）目前只覆盖支付/订阅/计费，**完全没有账号相关 RPC**；账号路径要从零增。下表的 RPC 全部新增：

| RPC | 是否必须 | 调用频率 | capability gate | 备注 |
|---|---|---|---|---|
| `GetAccount(account_id, fields)` | 必须 | hot path（每请求 1 次或缓存命中跳过） | `accounts.read` | 默认**不**返回 refresh_token / service_account_json / aws_secret；按 plugin 类型白名单返回字段 |
| `GetAccountAccessToken(account_id, refresh_window)` | 必须，**OAuth 三家与 Vertex 必走此** | hot path（与请求 1:1） | `accounts.read_token` | 替代 plugin 自己跑 token provider；host 端跑现有 `*TokenProvider.GetAccessToken` 并返回 (token, expires_at, account_snapshot)；refresh / 锁 / DB 写回全在 host |
| `SignBedrockRequest(account_id, method, url, headers, payload_hash)` | 必须（Bedrock 唯一选择） | hot path | `accounts.sign_aws` | 仅 Bedrock 插件需要；host 端调 `BedrockSigner.SignRequest`，返回签名后 headers |
| `MintVertexAccessToken(account_id, scope)` | 必须（替代 plugin 持 PEM） | hot path（受 host cache 保护） | `accounts.read_token` | 与 `GetAccountAccessToken` 可合并：host 看 type 分发到 OAuth / Vertex / 等 |
| `MarkAccountTempUnschedulable(account_id, until_unix, reason)` | 必须 | cold path（refresh 失败时） | `accounts.update_status` | 替代 antigravity provider 的 `accountRepo.SetTempUnschedulable + tempUnschedCache.SetTempUnsched` 双写 |
| `UpdateAccountCredentialField(account_id, field, value, expected_token_version)` | 可选 | cold path（project_id backfill） | `accounts.update_credentials` | 仅给"低敏感、可由 plugin 计算的"字段（如 antigravity project_id）；CAS by `_token_version`；高敏感字段（refresh_token）不开放 |
| `AcquireRefreshLock(account_id, ttl)` / `ReleaseRefreshLock(account_id)` | **不需要**单独暴露 | — | — | 已被 `GetAccountAccessToken` 内化（host 自己持锁）；plugin 不应直接操作分布式锁 |
| `MarkAccountError(account_id, code, message)` | 必须 | cold path | `accounts.update_status` | 用于 plugin 自己侦测到 401/invalid_grant 后通知 host |

**核心结论**：把 token 刷新整体留在 host 后，plugin **不需要** `AcquireRefreshLock` / `UpdateAccountCredentials`（refresh_token 写回）/ `RecordTokenVersion` 这些低层 RPC。它需要的只是 `GetAccountAccessToken`（合并 OAuth 与 Vertex）+ `SignBedrockRequest`（Bedrock 专用）+ `MarkAccountTempUnschedulable` / `MarkAccountError`（异常通报）四件套。

---

## 5. 并发 / 一致性风险分析

### 5.1 现有 host 内的保护

- 进程内：`OAuthRefreshAPI.localLocks sync.Map[cacheKey]*sync.Mutex`（`oauth_refresh_api.go:56-64`），保证同进程同 cacheKey 串行刷新。
- 进程间：`tokenCache.AcquireRefreshLock(cacheKey, 60s)`（基于 Redis SetNX，`oauth_refresh_api.go:88-105`）。
- 数据层：`_token_version`（`oauth_refresh_api.go:148-149`）+ `tryRecoverFromRefreshRace`（`oauth_refresh_api.go:175-193`）：当 executor 返回 `invalid_grant` 时，重读 DB 比较 refresh_token 是否已变；若变则视为另一个 worker 已成功刷新，本次失败转成功。

未发现 `singleflight.Group`；现有方案是"local mutex + redis lock + DB CAS"三层。

### 5.2 多 plugin instance 部署场景

如果 plugin 自己跑 token provider，多 instance 同时检测到过期会在 plugin 进程内各起一次：plugin 进程内的 `sync.Mutex` 不够用，必须依赖 Redis 分布式锁。问题：

1. **Redis 命名空间被 plugin 污染**：plugin 直接操作 host 的 OAuth lock key 违反 §7 风险表"sticky session redis 命名空间被 plugin 污染"原则。
2. **DB 写权限下放**：plugin 必须能 `UPDATE accounts SET credentials = ?, _token_version = ?`，任何一个 plugin 漏洞 = 全 host 账号库被改。
3. **审计与监控割裂**：refresh metrics（OpenAI 那 10 个 atomic 计数）必须在 plugin 端复制；host 看不到全量。
4. **invalid_grant 竞争恢复 broken**：恢复逻辑依赖"另一 worker 已写新 refresh_token"。如果 host 进程也在跑（双写期）就会和 plugin 抢，refresh_token 真的会被消耗一次又被另一进程读到旧值，竞争窗口被放大。

### 5.3 推荐方案

**把 token provider 整段保留在 host 进程**，plugin 通过 `GetAccountAccessToken` RPC 拿短期 access_token。理由：

- host 已有的 mutex + redis lock + `_token_version` 三层保护**零修改**继续生效。
- plugin 进程崩溃只会影响在途请求，不会破坏 token 状态机。
- Redis 锁命名空间 / DB 写权限 / 审计指标全部留在 host。
- RPC overhead：cache hit 路径只是一次 gRPC（µs 级）+ 一次 host 内部 map 查询；cache miss 路径反正本来就要做网络 IO，多一跳 gRPC 可忽略。

不推荐的备选（都明显更差）：
- **plugin 自跑 + Redis 分布式锁**：3 + 4 + 5 全踩。
- **host 提供 `AcquireRefreshLock` RPC + plugin 自己刷新**：把 host 的进程内 mutex 优势丢掉了；锁释放路径还要兜 plugin crash。
- **行级 SELECT FOR UPDATE**：跨 connection 持锁，PG 慢，且与现有 ent ORM 不兼容。

### 5.4 双 instance 同时拥有 lock 的边界

仍要预防场景：两个 host instance（不是 plugin）部署到同一 Redis 时也会竞争。这部分**已经**由现有 `OAuthRefreshAPI` 处理：lock 失败 → `LockHeld=true` → 调用方按 policy 等 cache 或用旧 token。不需要因 plugin 化而调整。

---

## 6. 加密边界

### 6.1 host 持有的密钥

- `cfg.Totp.EncryptionKey`（32B hex，`backend/internal/config/config.go:1112-1115`）：TOTP / 部分微信支付字段加密 + plugin SDK `SecretEncryptionServer`（`backend/cmd/server/wire.go:95` 把同一个 key 塞给 `out.SecretEncryptionMasterKeyHex`）。**Account.Credentials 不使用此密钥**。
- 没有第二把密钥；Account.Credentials 是明文 JSON。

### 6.2 跨进程时的方案

**plugin 不需要拿到原 refresh_token**：因为 token provider 留 host，plugin 只调 `GetAccountAccessToken` 拿短期 access_token，refresh_token / aws_secret / service-account-private-key 都不出 host 进程。

**短期凭据在 plugin 进程的处理**：
- access_token（≤1h）走 gRPC 明文消息；gRPC 通道现已在 reverse channel 上跑（见 `secret_encryption_server.go:1-30` 注释的 threat model），plugin 通过 `x-sub2api-plugin` header 鉴权。
- plugin 进程内仅在请求生命周期内保留 access_token，不写日志、不持久化。
- plugin 崩溃：access_token 在 plugin 进程地址空间随进程消失即可；host 端 cache TTL 自动过期。**禁止**plugin 落盘任何 token；如果 plugin 需要本地 cache，必须只缓存 (account_id, expires_at)，每次过期前重调 RPC。
- plugin OnExit 钩子可选地清空内存 cache，但 Go runtime 没有可靠的进程退出前内存擦除原语，关键防线是"不落盘"。

### 6.3 高敏感字段隔离

`GetAccount` RPC 必须有字段白名单，按 plugin manifest 声明的 capability 决定返回哪些 credentials 字段：
- 默认：返回 platform / type / 调度位字段；**不返回** `refresh_token` / `service_account_json` / `aws_secret_access_key` / `aws_session_token`。
- 仅当 plugin 声明 `accounts.read_raw_credentials`（建议**不**对网关 plugin 开放）才返回原始凭据。
- 网关 plugin 的正确路径是调 `GetAccountAccessToken` / `SignBedrockRequest`，不直接读原始 credentials。

---

## 7. 工程难度评级

| 平台 | OAuth refresh 跨进程难度 | 关键挑战 |
|---|---|---|
| anthropic (OAuth / Setup Token) | **低** | host 暴露 `GetAccountAccessToken` RPC 直接复用现有 `ClaudeTokenProvider`，policy 不变；plugin 端零状态 |
| openai | **低** | 同 anthropic；额外把 metrics snapshot 也通过 admin RPC 暴露（不阻塞拆分） |
| antigravity | **中** | 多了 `markTempUnschedulable`（双写 DB + Redis）+ `project_id` backfill 两条副作用路径，需要 `MarkAccountTempUnschedulable` + `UpdateAccountCredentialField` 两个新 RPC；project_id backfill 也可整体留 host |
| bedrock (anthropic 子类) | **中** | 必须新增 `SignBedrockRequest` RPC（per-request gRPC 往返）；好处是长寿 IAM 密钥永远不离 host；坏处是签名延迟从 µs 升到亚 ms 级 gRPC，但相对上游 LLM 调用本身忽略不计 |
| vertex (gemini 子类 + anthropic vertex 子类) | **中** | 同 Bedrock，`MintVertexAccessToken` 即可；Vertex 还要在 host 端保留 `vertex:service_account:<fp>` cache，避免每次 plugin 调用都跑 JWT 签名 |

**整体结论**：**低 - 中**。把 token provider 整体留在 host、新增 4 个 host RPC（`GetAccountAccessToken` / `SignBedrockRequest` / `MarkAccountTempUnschedulable` / `MarkAccountError`），就可以让 gateway plugin 完全不持有任何长期凭据。这套方案不会破坏现有的 mutex + redis lock + `_token_version` 三层并发保护，也不需要把 Redis 锁命名空间下放到 plugin。

唯一需要跑 PoC 验证的工程问题：**Bedrock SigV4 跨进程 gRPC 是否引入肉眼可见的请求延迟**——理论上 SigV4 本身只占几十 µs，远低于 P99 SSE 流式延迟（POC-A 关注的本体），但需要在双写期实测确认。

---

## 引用一览

- `backend/internal/service/claude_token_provider.go:21-167`
- `backend/internal/service/openai_token_provider.go:79-307`
- `backend/internal/service/antigravity_token_provider.go:27-227`
- `backend/internal/service/oauth_refresh_api.go:15-225`
- `backend/internal/service/refresh_policy.go:26-62`
- `backend/internal/service/account_credentials_persistence.go:9-30`
- `backend/internal/service/vertex_service_account.go:147-249`
- `backend/internal/service/bedrock_signer.go:16-67`
- `backend/internal/service/account.go:19-67`
- `backend/internal/repository/account_repo.go:341-441`
- `backend/internal/repository/aes_encryptor.go:17-95`
- `backend/internal/plugin/secret_encryption_server.go:1-246`
- `backend/internal/config/config.go:1112-1115`
- `backend/cmd/server/wire.go:95`
- `plugin-sdk/proto/sdk.proto:1014-1052`
- `docs/plugin-architecture/GATEWAY-EXTRACTION-PROPOSAL.md:316-352`
