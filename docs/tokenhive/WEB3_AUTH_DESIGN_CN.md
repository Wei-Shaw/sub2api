# Sub2API Web3 EOA 注册与登录设计

> 文档状态：第一版已实现，当前进入部署前回归验证
> 适用版本：第一版 Web3 认证
> 最后更新：2026-08-06

## 1. Overview：总体架构

本方案为 Sub2API 增加基于以太坊地址的 Web3 注册与登录能力。第一版只支持普通 EOA 钱包，通过 SIWE（Sign-In with Ethereum）消息签名证明用户持有对应私钥；支持所有 EVM 兼容网络，不设置网络白名单，同一个地址在不同 EVM 网络上始终对应同一个 Sub2API 用户。

架构采用“独立 Web3 功能岛”而不是深度改造现有认证体系：Web3 身份存入新表 `web3_identities`，一次性签名 Challenge 存入 Redis，验证成功后复用现有用户状态、TOTP、JWT、Refresh Token、注册赠送、邀请码、优惠码和 Affiliate 流程。现有 `users`、`auth_identities`、OAuth、邮箱认证和 Ent Schema 不做结构性修改，以降低将来同步上游代码时的冲突概率。

```text
┌──────────────────────────────────────────────────────────────────┐
│                           Frontend                               │
│                                                                  │
│  /login       ──┐                  ┌── /login/web3              │
│  /register    ──┼─ AuthMethodTabs ─┼── /register/web3           │
│                  │                  │                             │
│                  └── Email          └── Wallet Connect + SIWE    │
└───────────────────────────────┬──────────────────────────────────┘
                                │ HTTPS
                                ▼
┌──────────────────────────────────────────────────────────────────┐
│                        Web3AuthHandler                            │
│                                                                  │
│  Turnstile / Rate Limit / Backend Mode / Request Validation      │
└───────────────────────────────┬──────────────────────────────────┘
                                ▼
┌──────────────────────────────────────────────────────────────────┐
│                        Web3AuthService                            │
│                                                                  │
│  SIWE Message ─ EOA Signature Recovery ─ Address Comparison      │
│         │                                      │                 │
│         ▼                                      ▼                 │
│  Redis Challenge Store                 Web3IdentityRepository    │
│  TTL / Replay Protection               users + web3_identities   │
└───────────────────────────────┬──────────────────────────────────┘
                                ▼
┌──────────────────────────────────────────────────────────────────┐
│                   Existing Authentication                        │
│                                                                  │
│  User Status ─ TOTP 2FA ─ JWT ─ Refresh Token ─ Audit Log        │
└──────────────────────────────────────────────────────────────────┘
```

### 1.1 核心设计决策

| 项目 | 第一版决策 |
| --- | --- |
| 钱包类型 | 仅支持 EOA，不支持 Safe 等 EIP-1271 智能合约钱包 |
| 网络范围 | 不限制网络，支持所有 EVM 兼容网络 |
| 跨链身份 | 同一个地址跨网络视为同一个用户 |
| 邮箱要求 | Web3 注册不要求邮箱，之后可以选择绑定真实邮箱 |
| 钱包数量 | 一个用户最多一个 Web3 钱包，一个钱包最多属于一个用户 |
| Challenge | Redis 保存，默认 5 分钟 TTL，一次性消费 |
| 持久身份 | 新增独立表 `web3_identities` |
| 用户兼容 | 使用内部合成邮箱和随机密码满足 `users` 现有约束 |
| 用户名 | Web3 注册时必填，长度 2 至 100 个 Unicode 字符，不要求全局唯一 |
| TOTP | 复用现有二次验证流程 |
| 前端 Tab | 使用路由驱动的 Tab，避免大幅修改现有登录注册页面 |
| 钱包连接 | 第一版仅使用浏览器注入的 EIP-1193 Provider，不含 WalletConnect 和 EIP-6963 钱包选择器 |
| 个人资料 | 用户接口返回 `web3_address`，Profile 页面只读展示完整地址 |
| SIWE 来源 | 数据库系统设置中的 `frontend_url` 优先，`server.frontend_url` 作为 fallback |
| 上游同步 | 新文件承载主要实现，现有文件只保留少量接线代码 |

### 1.2 设计原则

1. **服务端构造消息**：SIWE 消息必须由后端生成，前端只负责展示和请求钱包签名。
2. **一次签名一次使用**：Challenge 必须有短 TTL，并在验证成功后原子消费。
3. **地址是身份，网络是上下文**：身份唯一键只有 EVM 地址；Chain ID 仅用于本次 SIWE 消息和审计。
4. **不接触私钥**：Sub2API 只接收公开地址、SIWE 消息和签名，不接收助记词、私钥或钱包授权交易。
5. **复用认证出口**：钱包验证成功后继续使用现有用户状态、TOTP、Token 和会话逻辑。
6. **隔离 fork 功能**：避免修改 Ent Schema、统一身份结构和后台设置链路，降低同步上游时的冲突。

## 2. 目标与非目标

### 2.1 第一版目标

- 在登录页和注册页增加“邮箱 / Web3”Tab，默认仍为邮箱。
- 支持浏览器钱包连接和 SIWE 消息签名。
- 支持任意 EVM Chain ID，不要求用户切换到指定网络。
- 通过 EOA ECDSA 签名恢复验证钱包地址。
- 支持纯钱包账号，不强制填写或绑定邮箱。
- Web3 注册要求用户填写用户名；用户名不要求全局唯一，注册后仍可在个人资料中修改。
- Web3 用户的个人资料页显示独立的钱包身份区块和完整 EVM 地址。
- 保证一个用户最多一个钱包、一个钱包最多一个用户。
- Web3 注册遵守全局注册开关、Turnstile、邀请码、优惠码、Affiliate 和默认额度发放规则。
- Web3 登录复用用户状态检查、Backend Mode、TOTP、JWT 和 Refresh Token。
- 对重放、过期、并发注册、地址大小写和签名篡改提供服务端保护。
- 主要功能通过新增文件实现，减少对上游高频变动文件的修改。

### 2.2 第一版非目标

- 不支持 Safe、账户抽象钱包或其他 EIP-1271 合约钱包。
- 不支持 EIP-6492 counterfactual wallet。
- 不查询钱包余额、NFT、ENS 或链上历史。
- 不要求或发起链上交易，不产生 Gas 费用。
- 不提供钱包更换、解绑或多钱包管理。
- 不提供现有邮箱用户绑定钱包的 UI 和接口。
- 不把 Web3 身份接入现有 `auth_identities` 和个人资料统一身份列表。
- 不增加后台 Web3 开关、钱包搜索和 Web3 专属统计页面。
- 不修改 `users.email`、`users.password_hash` 的非空约束。

## 3. 身份模型

### 3.1 地址规范化

数据库中的地址统一保存为：

```text
0x + 40 位小写十六进制字符
```

例如：

```text
0x0123456789abcdef0123456789abcdef01234567
```

规则：

- 请求地址允许 checksum、全小写或钱包返回的标准格式。
- 后端验证地址格式后转换为 lowercase 作为查询和唯一性依据。
- SIWE 消息和用户界面使用 checksum address 展示。
- 地址比较不得使用普通大小写敏感字符串比较。

### 3.2 跨网络语义

Chain ID 不参与身份唯一性：

```text
Ethereum  0x1234...
BSC       0x1234...
Polygon   0x1234...
Arbitrum  0x1234...
```

上述地址都映射到同一条 `web3_identities` 记录。原因是第一版只支持 EOA，地址控制权由同一私钥证明，当前网络只描述用户签名时的钱包上下文。

Chain ID 仍然必须进入 SIWE 消息，并在后端验证请求与 Challenge 一致。API 使用十进制字符串传输 Chain ID，避免 JavaScript Number 精度问题：

```json
{
  "chain_id": "42161"
}
```

### 3.3 一个用户一个钱包

数据库同时保证两个方向的一对一关系：

```text
user_id UNIQUE / PRIMARY KEY  → 一个用户最多一个钱包
address UNIQUE WHERE active   → 一个钱包最多属于一个活跃用户
```

同一地址可以对应多条历史软删除身份，但任意时刻最多只能有一条 `deleted_at IS NULL` 的活跃身份。

第一版没有钱包替换流程。Web3 是用户唯一登录方式时，钱包或私钥丢失将导致账号无法恢复。前端应明确提示用户绑定真实邮箱或其他独立登录方式以降低风险；TOTP 只是第二验证因素，不能替代钱包作为账号恢复手段。

## 4. 数据库设计

### 4.1 新增表 `web3_identities`

实际实施使用 `194_tokenhive_web3_identities.sql` 创建身份表，并由 `195_web3_identity_soft_delete.sql` 增加身份软删除和活跃地址部分唯一索引。

```sql
CREATE TABLE IF NOT EXISTS web3_identities (
    user_id BIGINT PRIMARY KEY
        REFERENCES users(id)
        ON DELETE CASCADE,

    address VARCHAR(42) NOT NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    deleted_at TIMESTAMPTZ,

    CONSTRAINT web3_identities_address_format_check
        CHECK (address ~ '^0x[0-9a-f]{40}$')
);

CREATE UNIQUE INDEX web3_identities_active_address_key
    ON web3_identities (address)
    WHERE deleted_at IS NULL;
```

第一版不需要独立的自增 `id`：`user_id` 本身就是一对一身份记录的主键。

### 4.2 不修改现有表

第一版不修改：

- `users` 表结构和现有 CHECK Constraint。
- `auth_identities`。
- `pending_auth_sessions`。
- Ent Schema 和 Ent 自动生成文件。

是否为 Web3 用户通过 `web3_identities` 是否存在来判断，而不依赖 `users.signup_source`。这样可以避免上游未来重建 `users_signup_source_check` 时覆盖 fork 中的 provider 枚举。

### 4.3 合成邮箱和密码

由于 `users.email` 和 `users.password_hash` 当前非空，Web3 注册生成内部字段：

```text
email         = web3-<lowercase address without 0x>@web3-connect.invalid
password_hash = 随机 32 字节密码的 bcrypt hash
username      = 用户在 Web3 注册页面填写的用户名
```

示例：

```text
web3-0123456789abcdef0123456789abcdef01234567@web3-connect.invalid
```

要求：

- 合成邮箱域名必须加入保留邮箱判断。
- 合成邮箱不得用于邮箱密码登录、密码找回或邮件通知。
- 用户接口应隐藏合成邮箱，表现为 `email: ""` 和“未绑定邮箱”。
- 管理员接口可以保留内部合成邮箱，便于排查和搜索。
- 用户后续绑定真实邮箱时，按首次邮箱绑定处理，不要求输入随机旧密码。

### 4.4 注册事务

用户和钱包身份必须在同一 SQL Transaction 中创建：

```text
BEGIN
  INSERT users
  INSERT web3_identities
COMMIT
```

如果两个请求并发注册同一地址，数据库唯一约束保证只有一个成功，失败事务中的用户记录一并回滚，不产生孤立用户。

注册 Repository 应只插入必要的 `users` 字段，其余字段使用数据库默认值，降低上游新增字段时的维护成本。

### 4.5 删除语义

- 用户软删除：在同一事务中设置 `users.deleted_at` 和对应 `web3_identities.deleted_at`。
- 地址复用：已软删除身份不参与地址唯一性约束，同一钱包可以注册新用户。
- 历史保留：旧身份记录继续保留，通过 `user_id` 和 `deleted_at` 与新身份区分。
- 用户物理删除：`ON DELETE CASCADE` 删除钱包身份。

查询、登录和注册查重只读取 `deleted_at IS NULL` 的活跃身份。若未来增加用户恢复功能，不应自动恢复旧钱包身份，因为该地址可能已经归属新的活跃用户。

## 5. Redis Challenge 设计

### 5.1 为什么使用 Redis

SIWE Challenge 是短期、一次性的认证会话，不需要持久化到数据库。Sub2API 已经使用 Redis 保存 Passkey Session 和其他认证缓存，因此 Web3 Challenge 复用相同模式可以减少数据库迁移和代码侵入。

### 5.2 Redis Key

```text
web3:challenge:<sha256(challenge_token)>
```

服务端不持久化前端持有的原始 Challenge Token；Redis Key 只使用其 SHA-256 哈希。

### 5.3 Redis Value

```json
{
  "intent": "login",
  "address": "0x0123456789abcdef0123456789abcdef01234567",
  "checksum_address": "0x0123...",
  "chain_id": "42161",
  "nonce": "a8f3c19d74e64245b37a2bd24012fb59",
  "message": "example.com wants you to sign in with your Ethereum account...",
  "message_digest": "64-character lowercase SHA-256 hex",
  "browser_session_hash": "...",
  "failed_attempts": 0,
  "created_at": "2026-08-05T00:00:00Z",
  "expires_at": "2026-08-05T00:05:00Z"
}
```

当前实现使用 32 字节随机 Challenge Token 和 16 字节随机 Nonce，均以小写十六进制字符串传输。

默认 TTL：

```text
5 分钟
```

### 5.4 原子消费

签名验证成功后使用 Redis Lua 或 WATCH/MULTI 实现比较并删除：

1. 读取 Challenge。
2. 在应用层解析和验证签名。
3. 调用原子脚本比较 `message_digest` 并删除 Key。
4. 只有删除成功的请求可以继续登录或注册。

两个相同 Verify 请求并发执行时，签名可能都通过，但只有一个能原子消费 Challenge，另一个返回 `WEB3_CHALLENGE_CONSUMED`。

签名失败时不立即消费 Challenge，但增加 `failed_attempts`；达到 3 次后删除 Challenge，防止高频尝试。

## 6. SIWE 消息与 EOA 验证

### 6.1 消息字段

后端根据 EIP-4361 构造 SIWE 消息，至少包含：

- Domain。
- Ethereum Address。
- Statement。
- URI。
- Version `1`。
- Chain ID。
- Nonce。
- Issued At。
- Expiration Time。
- Request ID 或内部 Challenge 标识。

登录 Statement 示例：

```text
Sign in to Sub2API. This request does not trigger a blockchain transaction or cost gas.
```

注册 Statement 示例：

```text
Register a Sub2API account with this wallet. This request does not trigger a blockchain transaction or cost gas.
```

### 6.2 Domain 和 URI

- Domain 和 URI 必须来自可信服务端配置。
- 不直接信任请求的 `Host` Header。
- 实际读取顺序为数据库系统设置 `frontend_url` 优先，配置文件 `server.frontend_url` fallback；如果后台保存过非空值，它会覆盖 `config.yaml`。
- Domain 使用 URL 的 `host[:port]`，URI 使用去除 query、fragment 和末尾 `/` 后的完整 Frontend URL。
- 如果 Frontend URL 缺失或不是合法的 HTTP/HTTPS URL，Challenge 接口 fail-close。
- 配置错误时返回 `WEB3_FRONTEND_URL_NOT_CONFIGURED`，前端显示“Web3 认证配置不完整”。
- 生产和测试环境使用不同 Domain 时，签名不能跨环境复用。

### 6.3 EOA 验证步骤

```text
读取 Redis Challenge
  ↓
校验未过期、未消费、浏览器会话一致
  ↓
校验服务端保存消息的 SHA-256 Digest 未被篡改
  ↓
按照 Ethereum personal_sign / EIP-191 规则计算消息哈希
  ↓
使用签名恢复公钥和地址
  ↓
恢复地址 lowercase 后与 Challenge address 比较
  ↓
原子消费 Challenge
```

第一版不调用 RPC，也不查询地址是否有合约代码。Safe 等合约钱包的签名通常不能通过 EOA 地址恢复验证，因此后端统一返回 `WEB3_SIGNATURE_INVALID`。由于没有链上 RPC，后端不能可靠区分“智能合约钱包签名”和“普通无效签名”；前端应在连接和签名区域固定标注“当前仅支持普通 EOA 钱包”。

### 6.4 实际后端依赖

实际实施使用 `github.com/decred/dcrd/dcrec/secp256k1/v4` 完成公钥恢复，并使用 `golang.org/x/crypto/sha3` 完成 Keccak-256 和 EIP-191 哈希。该选择避免引入完整 `go-ethereum` 的大量传递依赖；地址 checksum、personal-sign 哈希和签名恢复均有独立单元测试覆盖。后续如果引入完整 SIWE 解析库，应保持现有 Challenge、Domain、URI、Nonce、时间和浏览器会话校验语义不变。

## 7. 后端接口设计

### 7.1 路由

第一版使用四个固定用途的接口，不允许客户端在 Verify 时改变 intent：

```text
POST /api/v1/auth/web3/login/challenge
POST /api/v1/auth/web3/login/verify

POST /api/v1/auth/web3/register/challenge
POST /api/v1/auth/web3/register/verify
```

固定路由比通用 `intent` 字段更容易进行限流、审计和权限判断。

### 7.2 创建登录 Challenge

请求：

```json
{
  "address": "0xAbC...",
  "chain_id": "42161",
  "turnstile_token": "..."
}
```

响应：

```json
{
  "challenge_token": "...",
  "message": "example.com wants you to sign in...",
  "expires_at": "2026-08-04T00:05:00Z"
}
```

创建登录 Challenge 时不检查地址是否已注册，避免匿名请求枚举钱包账号。只有在用户完成有效签名、证明拥有该地址后，Verify 接口才查询身份并返回“钱包尚未注册”。

### 7.3 验证登录签名

请求：

```json
{
  "challenge_token": "...",
  "signature": "0x..."
}
```

成功时返回现有 `AuthResponse`；启用 TOTP 时返回现有 `requires_2fa` 响应，并继续使用 `/api/v1/auth/login/2fa`。

### 7.4 创建注册 Challenge

请求与登录 Challenge 相同，但后端额外检查：

- 全局注册开关。
- Backend Mode。
- Turnstile。

注册 Challenge 创建时同样不公开地址是否已注册。地址重复检查在有效签名之后执行，并由数据库唯一约束提供最终并发保护。

### 7.5 验证注册签名

请求：

```json
{
  "challenge_token": "...",
  "signature": "0x...",
  "username": "Alice",
  "invitation_code": "",
  "promo_code": "",
  "aff_code": ""
}
```

服务端按以下顺序执行：

1. 重新检查全局注册开关。
2. 校验并规范化用户名；用户名不合法时不消费 Challenge。
3. 校验 Challenge 和签名。
4. 校验邀请码。
5. 确认地址仍未被注册。
6. 生成合成邮箱、随机密码和初始用户字段。
7. 事务创建 `users` 和 `web3_identities`。
8. 初始化默认额度、默认订阅和平台配额。
9. 应用优惠码。
10. 初始化 Affiliate 并绑定邀请人。
11. 返回现有 Token Pair。

签名验证必须发生在邀请码消耗和用户创建之前。

### 7.6 实际错误码

| 错误码 | 含义 |
| --- | --- |
| `WEB3_ADDRESS_INVALID` | 地址格式无效 |
| `WEB3_CHAIN_ID_INVALID` | Chain ID 不是合法十进制正整数 |
| `WEB3_CHALLENGE_NOT_FOUND` | Challenge 不存在 |
| `WEB3_CHALLENGE_EXPIRED` | Challenge 已过期 |
| `WEB3_CHALLENGE_CONSUMED` | Challenge 已被消费 |
| `WEB3_CHALLENGE_SESSION_MISMATCH` | Challenge 与浏览器会话不匹配 |
| `WEB3_CHALLENGE_INTENT_MISMATCH` | 登录和注册 Challenge 被交叉使用；固定前端路由下通常不会触发 |
| `WEB3_SIGNATURE_INVALID` | 签名无效或恢复地址不一致 |
| `WEB3_IDENTITY_NOT_FOUND` | 钱包尚未注册 |
| `WEB3_IDENTITY_EXISTS` | 钱包已经注册 |
| `WEB3_USERNAME_REQUIRED` | 用户名为空或只包含空白字符 |
| `WEB3_USERNAME_TOO_SHORT` | 用户名少于 2 个字符 |
| `WEB3_USERNAME_TOO_LONG` | 用户名超过 100 个字符 |
| `WEB3_USERNAME_INVALID` | 用户名包含控制字符 |
| `WEB3_FRONTEND_URL_NOT_CONFIGURED` | Frontend URL 缺失或不合法 |
| `REGISTRATION_DISABLED` | 当前关闭注册；复用现有注册错误码 |

错误响应继续使用 Sub2API 现有错误结构。前端已为常见用户可见的 Web3 错误提供中英文映射；`WEB3_CHALLENGE_INTENT_MISMATCH` 属于固定路由之间的防御性校验，正常页面流程不会触发。

## 8. 后端模块设计

### 8.1 新增文件

```text
backend/internal/handler/web3_auth_handler.go
backend/internal/service/web3_auth.go
backend/internal/repository/web3_identity_repo.go
backend/internal/repository/web3_challenge_store.go
backend/migrations/194_tokenhive_web3_identities.sql
backend/migrations/195_web3_identity_soft_delete.sql
```

测试：

```text
backend/internal/service/web3_auth_test.go
backend/internal/repository/web3_identity_repo_test.go
backend/internal/repository/web3_identity_soft_delete_integration_test.go
backend/internal/repository/web3_challenge_store_test.go
backend/migrations/web3_identity_soft_delete_migration_test.go
backend/internal/handler/dto/web3_email_mapper_test.go
backend/internal/handler/auth_current_user_test.go
backend/internal/handler/user_handler_test.go
```

最后两个 Handler 测试文件为现有测试文件，本功能只在其中追加 Profile Web3 地址相关用例。

### 8.2 Service Interface

```go
type Web3AuthService struct {
    identityRepository Web3IdentityRepository
    challengeStore     Web3ChallengeStore
    authService        *AuthService
}

func (s *Web3AuthService) CreateLoginChallenge(
    ctx context.Context,
    input Web3ChallengeInput,
) (*Web3ChallengeResult, error)

func (s *Web3AuthService) VerifyLogin(
    ctx context.Context,
    input Web3VerifyInput,
) (*Web3LoginResult, error)

func (s *Web3AuthService) CreateRegistrationChallenge(
    ctx context.Context,
    input Web3ChallengeInput,
) (*Web3ChallengeResult, error)

func (s *Web3AuthService) VerifyRegistration(
    ctx context.Context,
    input Web3RegistrationVerifyInput,
) (*User, error)
```

### 8.3 Repository Interface

```go
type Web3IdentityRepository interface {
    GetUserIDByAddress(ctx context.Context, address string) (int64, error)
    GetAddressByUserID(ctx context.Context, userID int64) (string, bool, error)
    ExistsByAddress(ctx context.Context, address string) (bool, error)
    CreateUserWithIdentity(
        ctx context.Context,
        input Web3UserCreateInput,
    ) (int64, error)
}
```

### 8.4 Challenge Store Interface

```go
type Web3ChallengeStore interface {
    Save(
        ctx context.Context,
        tokenHash string,
        challenge Web3Challenge,
        ttl time.Duration,
    ) error

    Get(ctx context.Context, tokenHash string) (*Web3Challenge, error)

    Consume(
        ctx context.Context,
        tokenHash string,
        expectedDigest string,
    ) (bool, error)

    RecordFailure(ctx context.Context, tokenHash string) error
}
```

### 8.5 复用现有 AuthService

Web3 Service 放在现有 `service` package 中，通过组合 `AuthService` 复用：

- Turnstile。
- 注册开关。
- 默认注册额度和订阅计划。
- Promo。
- Affiliate。
- 用户登录时间记录。
- Token Pair。

不调用现有邮箱 `RegisterWithVerification`，因为该流程绑定邮箱策略、邮箱验证码和 Email AuthIdentity。Web3 在新文件中编排注册，但复用同一底层模块。

### 8.6 TOTP 复用

钱包签名是第一验证因素，TOTP 是第二验证因素：

```text
Wallet Signature
  ↓
TOTP Enabled?
  ├─ No  → Token Pair
  └─ Yes → Existing temporary login session → /auth/login/2fa
```

Web3 登录返回 TOTP Challenge 时，现有 `user_email_masked` 字段可以暂时返回缩短的钱包地址，例如 `0x1234...abcd`，避免暴露内部合成邮箱。

## 9. 后端现有文件影响

主要实现放在新文件，现有文件只进行薄接入：

| 文件 | 实际修改 |
| --- | --- |
| `backend/go.mod` | 增加 SIWE/Ethereum 签名依赖 |
| `backend/go.sum` | 依赖校验更新 |
| `backend/internal/handler/handler.go` | 增加 `Web3Auth` Handler 字段 |
| `backend/internal/handler/wire.go` | 增加 Handler Provider |
| `backend/internal/service/wire.go` | 增加 Service Provider |
| `backend/internal/repository/wire.go` | 增加 Repository 和 Challenge Store Provider |
| `backend/cmd/server/wire_gen.go` | Wire 自动生成更新 |
| `backend/internal/server/routes/auth.go` | 增加四个 Web3 路由和限流 |
| `backend/internal/server/middleware/audit_log.go` | 登录/注册审计映射，Verify Body 不入审计 |
| `backend/internal/server/middleware/backend_mode_guard.go` | 允许路由进入后再执行用户角色检查 |
| `backend/internal/service/auth_service.go` | 保留邮箱判断增加 Web3 合成邮箱后缀 |
| `backend/internal/handler/auth_handler.go` | `/auth/me` 注入 Web3 地址查询 |
| `backend/internal/handler/user_handler.go` | Profile 响应增加 `web3_address` |
| `backend/internal/handler/dto/mappers.go` | 用户接口隐藏合成邮箱，管理员接口保留合成邮箱 |

明确不修改：

- `backend/ent/schema/user.go`。
- `backend/ent/schema/auth_identity.go`。
- Ent 自动生成文件。
- `backend/internal/repository/user_repo.go`。
- `backend/internal/service/user_service.go`。
- OAuth 和 Pending Auth Session。
- JWT Claims 和 Refresh Token 数据结构。
- 后台设置和 Public Settings DTO。

## 10. 前端设计

### 10.1 路由驱动 Tab

为了避免在大型 `LoginView.vue` 和 `RegisterView.vue` 中加入大量钱包状态，Tab 采用路由驱动：

```text
/login          → 邮箱登录
/login/web3     → Web3 登录
/register       → 邮箱注册
/register/web3  → Web3 注册
```

四个页面都显示统一的：

```text
[ 邮箱 ] [ Web3 ]
```

现有邮箱页面只需插入一个 `AuthMethodTabs`，不重构原有表单。

### 10.2 新增文件

```text
frontend/src/components/auth/AuthMethodTabs.vue
frontend/src/components/auth/Web3WalletPanel.vue
frontend/src/views/auth/Web3LoginView.vue
frontend/src/views/auth/Web3RegisterView.vue
frontend/src/api/web3Auth.ts
frontend/src/composables/useWeb3Wallet.ts
frontend/src/utils/web3Username.ts
frontend/src/types/ethereum.d.ts
frontend/src/components/user/profile/ProfileWeb3IdentityCard.vue
```

测试：

```text
frontend/src/composables/useWeb3Wallet.spec.ts
frontend/src/utils/web3Username.spec.ts
frontend/src/views/user/__tests__/ProfileView.spec.ts
```

### 10.3 钱包连接状态

```text
未连接
  → [连接钱包]

已连接
  → 显示 0x1234...abcd
  → 显示当前网络名称或 Chain 123456
  → [签名并登录] / [签名并注册]

等待钱包确认
  → 请在钱包中确认签名

验证中
  → 正在验证钱包签名

完成
  → 保存 Token，跳转系统首页
```

不显示“网络不支持”或“请切换网络”。如果用户在 Challenge 创建后切换账号或网络，前端必须废弃当前 Challenge 并重新创建。

### 10.4 注册页字段

Web3 注册页继续展示当前启用的：

- 用户名，必填，长度 2 至 100 个字符。
- 邀请码。
- 优惠码。
- Affiliate Code。
- 登录/注册协议。
- Turnstile。

不展示：

- 邮箱。
- 密码。
- 邮箱验证码。
- 邮箱后缀规则。

### 10.5 Token 接入

Web3 API 返回现有 `AuthResponse`，前端复用 `authStore.setToken()` 和现有 Refresh Token LocalStorage 约定，不在 Auth Store 中加入钱包连接状态。钱包地址、Chain ID、连接状态和签名状态由 `useWeb3Wallet` 管理；Challenge Token 只保存在 `Web3WalletPanel` 页面状态中，并在请求结束或账号、网络变化时失效。

### 10.6 当前钱包接入与后续扩展

第一版直接使用 `window.ethereum` 提供的浏览器注入 EIP-1193 Provider，通过 `eth_requestAccounts`、`eth_chainId` 和 `personal_sign` 完成连接与签名，不新增 Wagmi、viem 或 WalletConnect 依赖。

因此第一版在同时安装多个注入钱包时不提供钱包选择器，也不支持移动端二维码连接。后续如果增加 EIP-6963 或 WalletConnect，应继续把差异封装在 `useWeb3Wallet` 或新的连接器层内，避免修改页面认证流程。

### 10.7 个人资料页展示

`GET /api/v1/auth/me` 和 `GET /api/v1/user/profile` 在用户存在 `web3_identities` 记录时返回：

```json
{
  "web3_address": "0x0123456789abcdef0123456789abcdef01234567"
}
```

前端仅在 `web3_address` 非空时显示独立的 Web3 身份区块，并展示完整钱包地址。该区块只用于查看当前登录身份，不提供解绑、更换或新增钱包操作，也不将 Web3 身份并入现有统一身份绑定列表。

## 11. 安全设计

### 11.1 重放保护

- Nonce 使用密码学安全随机数，至少 128 位熵。
- Challenge 默认 5 分钟过期。
- Token 只保存哈希。
- 验证成功后原子消费。
- Domain、URI、Address、Chain ID、Nonce 和时间字段全部服务端核对。

### 11.2 会话绑定

Challenge 已绑定匿名浏览器 Session Cookie 的哈希：

- Challenge 创建时记录 `browser_session_hash`。
- Verify 时要求同一浏览器 Session。
- Cookie 名为 `web3_auth_browser_session`，Path 为 `/api/v1/auth/web3`，有效期 10 分钟。
- Cookie 使用 `HttpOnly`。`web3_auth.browser_cookie_same_site` 默认为 `lax`；当前端与 API 位于不同站点时配置为 `none`，并自动强制设置 `Secure`，因此该模式要求 HTTPS。跨站部署还必须在 `cors.allowed_origins` 中配置精确前端 Origin，并保持 `cors.allow_credentials: true`。

即使 Challenge Token 被日志或浏览器扩展读取，也不能在另一浏览器直接消费。

### 11.3 限流

当前路由阈值：

```text
登录 Challenge：20 次 / IP / 分钟
登录 Verify：20 次 / IP / 分钟
注册 Challenge：5 次 / IP / 分钟
注册 Verify：5 次 / IP / 分钟
```

四个路由均使用现有 Rate Limiter 的 fail-close 策略。第一版没有额外实现“同一地址”维度的独立限流。

### 11.4 日志和审计

当前实现：

- 四个 Web3 Challenge/Verify 路由的请求体都标记为 Body Omitted。
- 登录和注册 Verify 分别映射到现有登录、注册审计动作。
- 验证成功后设置 `auth_method = web3` 和用户审计 Actor。

不得记录：

- 原始 Challenge Token。
- 完整签名请求体。
- 原始 SIWE 消息中的可复用会话凭证。
- 钱包连接器内部状态。

审计中间件应把 Verify 路由标记为 Body Omitted。

### 11.5 用户提示

当前页面在签名前固定显示：

- 只签署链下消息，不会发起链上交易或产生 Gas 费用。
- 第一版仅支持 EOA，不支持 Safe 等智能合约钱包。
- 注册无需邮箱；第一版中如果钱包丢失，账号将无法恢复。

系统流程只调用钱包 Provider 的连接和消息签名方法，不提供任何输入私钥或助记词的界面。

## 12. 并发与一致性

### 12.1 并发登录

多个有效签名请求并发验证同一个 Challenge 时，Redis 原子消费保证只有一个请求签发 Token。

### 12.2 并发注册

两个不同 Challenge 同时注册同一个地址时，数据库活跃地址部分唯一索引 `UNIQUE(address) WHERE deleted_at IS NULL` 是最终一致性保护。失败请求转换为 `WEB3_IDENTITY_EXISTS`。

### 12.3 注册后初始化失败

`users + web3_identities` 创建必须原子。默认订阅、Promo、Affiliate 等后置初始化沿用当前注册流程的错误处理策略：

- 核心身份创建失败：整体注册失败。
- 非核心赠送或异步初始化失败：记录日志并按现有注册策略决定是否继续。
- 不为降低冲突而改变现有赠送和 Affiliate 的一致性语义。

## 13. Backend Mode、注册开关和 Turnstile

### 13.1 Backend Mode

- Web3 注册在 Backend Mode 下禁止。
- Web3 登录 Challenge 可以进入认证路由。
- Verify 找到用户后执行与现有登录相同的角色检查。
- 非管理员用户在 Backend Mode 下不能完成 Web3 登录。

### 13.2 注册开关

- Challenge 创建时检查一次。
- Verify 和创建用户前再次检查，避免 Challenge 创建后管理员关闭注册。

### 13.3 Turnstile

- Turnstile 在 Challenge 创建接口验证。
- Verify 不重复消费一次性 Turnstile Token。
- 登录和注册分别使用现有认证入口的安全策略。

## 14. 测试状态

### 14.1 当前已覆盖

- 地址规范化、EIP-55 checksum、Chain ID 和用户名校验。
- SIWE 消息字段与 EOA `personal_sign` 签名恢复。
- 用户名校验失败时不消费 Challenge。
- 浏览器 Session 不一致时拒绝 Verify。
- 数据库系统设置 `frontend_url` 优先于配置文件 fallback。
- 合成邮箱生成、用户接口隐藏以及管理员接口保留。
- Redis Challenge 原子消费一次和三次签名失败后删除。
- Repository 按用户 ID 查询钱包地址。
- `/auth/me`、Profile 接口和 Profile 页面展示 `web3_address`。
- 前端注入钱包连接、Chain ID 归一化、UTF-8 消息签名和钱包缺失提示。
- 前端用户名规范化和校验。

### 14.2 部署前仍需回归

- PostgreSQL 上执行迁移，并验证唯一约束、事务回滚和物理删除级联。
- 完整登录、注册、重复注册、禁用用户和注册关闭流程。
- Turnstile、Backend Mode、TOTP 和 Token/Refresh Token 端到端流程。
- 邀请码、Promo、Affiliate、默认额度和订阅初始化。
- Challenge 过期、错误签名、重复 Verify 和并发消费。
- 邮箱、OAuth、Passkey 与 Web3 页面和认证流程的回归。
- 多种浏览器注入钱包的实际签名兼容性。

## 15. 上游同步与冲突控制

### 15.1 高冲突文件规避

第一版仍避免修改：

- 大型 `auth_service.go` 注册和登录主流程；只在保留邮箱判断中增加 Web3 合成邮箱后缀。
- 大型 `user_service.go` 身份摘要。
- `settings_view.go` 和后台设置链路。
- Ent Schema 和 Ent 生成文件。
- 现有登录注册表单主体。

### 15.2 薄接点

不可避免的现有文件改动限制为：

- Wire Provider 增加 Web3 Service、Repository、Challenge Store 和 Handler 接线。
- Handler 容器增加一个字段。
- Auth Route 增加四个路由。
- Audit 和 Backend Mode 增加固定路由映射。
- 登录注册页面增加一个 Tab 组件。
- Router 增加两个 Web3 页面路由。
- `AuthHandler` 和 `UserHandler` 注入 Web3 Repository，为当前用户与 Profile 响应补充 `web3_address`。
- `frontend/src/types/index.ts` 的 `User` 增加一个可选 `web3_address` 字段。

### 15.3 路由驱动页面

不直接把 Web3 表单嵌进 `LoginView.vue` 和 `RegisterView.vue` 的复杂条件分支中。上游修改邮箱注册、OAuth、Passkey、协议或表单校验时，Web3 页面不会产生大段冲突。

### 15.4 数据迁移独立

`web3_identities` 是纯新增表，不修改 `auth_identities` Provider Check Constraint。上游未来扩展统一身份体系时，可以通过一次迁移将：

```text
web3_identities
  → auth_identities(provider_type = web3)
```

导入上游模型，再删除功能岛。

## 16. 实施阶段

### Phase 1：后端身份基础（已完成）

- 新增 `web3_identities` 迁移及身份软删除迁移。
- 实现地址规范化和 Repository。
- 实现 Redis Challenge Store。
- 实现 SIWE 消息生成与 EOA 验证。
- 添加登录/注册接口。
- 接入 TOTP、Token、Turnstile、审计和 Backend Mode。

### Phase 2：前端登录注册（已完成）

- 使用浏览器 EIP-1193 注入 Provider 实现钱包连接，不新增前端钱包依赖。
- 实现 `AuthMethodTabs`。
- 实现 Web3 登录页和注册页。
- 接入协议、邀请码、优惠码、Affiliate 和 Turnstile。
- 添加中英文提示和错误映射。

### Phase 3：验证与上线（进行中）

- 已完成签名、地址、用户名、Challenge Store、合成邮箱隐藏、Profile 地址和前端钱包 RPC 的针对性测试。
- 在目标 PostgreSQL 环境执行数据库迁移，并准备迁移前备份与人工回退方案；当前迁移框架不使用独立 down migration。
- 验证邮箱、OAuth、Passkey 和 Web3 登录互不影响。
- 小范围启用并观察登录失败率、Challenge 消费失败和重复注册冲突。

### 后续可选阶段

- 现有用户绑定钱包。
- 钱包解绑和双签名更换流程。
- 管理后台钱包地址搜索。
- Web3 注册来源统计。
- EIP-1271 智能合约钱包。
- EIP-6492 账户抽象钱包。
- 将 Web3 身份迁入上游统一 `auth_identities` 模型。

## 17. 验收标准

第一版满足以下条件后可以验收：

1. 用户可以在任意 EVM 网络连接普通 EOA 钱包。
2. 用户无需邮箱即可注册 Sub2API 账号。
3. Web3 注册必须填写合法用户名，注册后可沿用现有 Profile 流程修改用户名。
4. 同一地址在不同网络登录到同一用户。
5. 一个用户不能拥有两个 Web3 钱包。
6. 一个钱包不能注册到两个用户。
7. 页面明确标注第一版不支持 Safe 等智能合约钱包；相关签名验证失败时返回统一的签名无效错误。
8. Challenge 过期、重放和并发消费得到服务端阻止。
9. 开启 TOTP 的用户必须完成二次验证。
10. Web3 注册遵守注册开关、Turnstile、邀请码、Promo 和 Affiliate 规则。
11. 用户接口不暴露合成邮箱，并在 Profile 页面只读展示 `web3_address`。
12. Frontend URL 缺失或不合法时 Challenge fail-close，并返回明确错误码。
13. 不修改现有 Ent Schema 和统一身份表。
14. 邮箱、OAuth、Passkey、Refresh Token 和 Backend Mode 回归测试通过。

## 18. 已知限制

- 钱包丢失且未绑定其他登录方式时无法恢复账号。
- 第一版不支持智能合约钱包。
- 现有邮箱用户无法在第一版中绑定钱包。
- Web3 用户不会进入现有统一身份绑定列表。
- 现有基于 `users.signup_source` 的统计不能直接识别 Web3 来源，需要通过 `web3_identities` 单独统计。
- 管理员第一版主要通过合成邮箱或用户名识别 Web3 用户，没有专用钱包搜索字段。

## 19. 参考标准

- EIP-191：Signed Data Standard。
- EIP-55：Mixed-case checksum address encoding。
- EIP-4361：Sign-In with Ethereum。
- EIP-1271：智能合约签名验证，第一版不实现。
- EIP-6492：Counterfactual contract wallet 签名，第一版不实现。
