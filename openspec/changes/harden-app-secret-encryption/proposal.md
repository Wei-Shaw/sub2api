## Why

安全审计发现两条互相咬合的问题，合并为一个变更处理。

**C4（Critical）——支付网关密钥以明文落库，且是一次未声明的回归。**
`internal/service/payment_config_providers.go` 的 `encryptConfig` 目前只做 `json.Marshal`，注释声称历史上的 AES-GCM 包装「已被移除」，而 `decryptConfig` 仍然接受旧密文。`internal/payment/crypto.go` 中的 AES-256-GCM 层只以读路径 fallback 的形式存活。但 `ent/schema/payment_provider_instance.go:20` 的 schema 注释仍写着该列存放「加密后的密钥信息」——两边说法冲突，说明这不是一次有意的设计决策，而是一次静默回归。

单行泄露即可暴露：Stripe `secretKey` + `webhookSecret`、EasyPay `pkey`、wxpay `apiV3Key` + `privateKey`、Alipay `privateKey`、Airwallex `apiKey` + `webhookSecret`。拿到这些密钥的攻击者可以为任意订单签发合法的成功回调并给自己充值，从而绕过签名校验层——而审计单独确认过签名校验本身写得正确且是常数时间比较。

**H5（High）——`TOTP_ENCRYPTION_KEY` 是一把伪装过的主密钥，而且每次启动都会重新生成。**
尽管叫这个名字，它是应用级 `SecretEncryptor`（`internal/repository/aes_encryptor.go`）背后唯一的密钥。已确认的调用方包括 `internal/service/` 下的 `backup_service`、`channel_monitor_service`、`image_storage_settings`、`ollama_cloud_usage`、`plugin_manager`、`totp_service`，覆盖备份 S3 secret key、渠道监控上游 API Key、S3 图片存储凭据、Ollama Cloud cookie、插件配置与插件 UI 会话 token。

`deploy/.env.example` 和四个 compose 文件把它留空，而 `config.go` 在为空时**每次进程启动都生成一把新的随机密钥**。于是一个从未启用 2FA 的运维会在每次容器重启后静默失去解密所有已存密文的能力：既不报错，也没有任何提示。`deploy/docker-compose.standalone.yml` 更是压根没有透传这个变量。

## What Changes

- 恢复支付服务商配置的写路径加密：`encryptConfig` 重新调用 `payment.Encrypt`（AES-256-GCM），`internal/payment/crypto.go` 上的 `Deprecated` 标记撤销。
- `decryptConfig` 保留明文分支，但语义从「新格式」降级为「回归窗口内写入的行的读兼容 shim」，注释方向随之调整。
- 新增一次性启动步骤 `internal/repository/secret_encryption_startup.go`：把回归窗口内写成明文的 `payment_provider_instances.config` 行重新加密，使用 PostgreSQL Advisory Lock 串行化多副本，并在 `settings` 表写入持久完成标记。
- 配置键 `totp.encryption_key` 重命名为 `security.secret_encryption_key`；旧键保留为只读别名，两种拼写都必须能通过 viper `AutomaticEnv` 解析。
- 移除空密钥时的自动生成。空密钥不再伪造出一把每次重启都变的随机密钥。
- 新增启动前置检查：密钥为空且数据库中已存在任何依赖 `SecretEncryptor` 的密文或已启用的相关功能时，启动失败并在错误信息中点名要设置的变量。
- 空密钥且没有任何相关功能启用时**继续启动**（见 Decisions），`SecretEncryptor` 降级为一个显式失败的实现，而不是一把随机密钥。
- 五个 deploy 文件全部对齐：`.env.example` 说明新旧变量，四个 compose 文件都实际透传新变量；补上 `docker-compose.standalone.yml` 这一条完全缺失的透传。

## Capabilities

### New Capabilities

- `payment-provider-secret-storage`：定义支付服务商实例配置的静态加密不变式、明文读兼容窗口，以及一次性重加密回填的幂等性、部分执行恢复和多副本互斥语义。
- `app-secret-encryption-key`：定义应用级密钥加密主密钥的配置键、环境变量拼写、旧键别名解析优先级、禁止自动生成，以及空密钥下的启动前置检查与降级行为。

### Modified Capabilities

无。仓库当前没有已发布的 OpenSpec capability；`totp.encryption_key` 的既有行为在本变更中作为兼容基线记录，不单独发布为已有 Requirement。

## Decisions

**空密钥且无任何相关功能启用时：继续启动，不失败。**

理由：全新安装在跑完 setup 向导之前，数据库里必然没有任何密文，也没有任何启用的功能。此时强制失败等于要求每个运维在第一次 `docker compose up` 之前先读文档、生成一把他们可能永远用不到的密钥——为一个尚不存在的风险付出确定的上手成本，这正是本变更想避免的那类静默设计债的反面。

安全性由两道闸门保证，而不是由启动失败保证：

1. `SecretEncryptor` 在密钥为空时不再是「一把随机密钥」，而是一个 `Encrypt`/`Decrypt` 都返回明确错误的实现。任何试图写入密钥的路径都会立刻、响亮地失败，并在错误里点名要设置的变量。旧行为是写入成功、重启后数据静默变成垃圾；新行为是当场拒绝。
2. 启动前置检查会在数据库中出现第一份密文或第一个启用的相关功能之后立即开始拦截。也就是说，「继续启动」这个宽松分支只在它确实无害的那一段时间内成立，一旦有东西可丢就自动收紧。

**重加密回填不做成 SQL migration。**

`migrations/*.sql` 由迁移运行器在纯 SQL 上下文中执行，拿不到应用层的加密密钥，因此这一步只能在应用层完成。它被实现为 repository 层的一次性启动步骤，而不是后台 worker：回填必须在任何请求读到这些行之前完成，才能保证「读到的一定是密文或已知的兼容明文」这一不变式。

## Impact

- **后端服务层**：`internal/service/payment_config_providers.go` 写路径恢复加密；无接口签名变化，`internal/testutil` 的 stub 不受影响。
- **后端 payment 包**：`internal/payment/crypto.go` 撤销 `Deprecated` 标记，函数签名与密文格式（`iv:authTag:ciphertext`，各段 base64）保持不变，因此回归窗口之前写入的旧密文继续可读。
- **后端 repository 层**：新增 `secret_encryption_startup.go`；`ent.go` 在 `ensureBootstrapSecrets` 之后新增两次调用；`aes_encryptor.go` 在密钥为空时返回显式失败的实现而不是报错阻断启动。
- **后端配置层**：`SecurityConfig` 新增 `secret_encryption_key`；`TotpConfig.EncryptionKey` 降级为只读别名并在 load 后被镜像为解析结果，因此现有全部 `cfg.Totp.EncryptionKey` / `cfg.Totp.EncryptionKeyConfigured` 读取点无需修改。
- **数据库**：不新增表、不新增列、不新增 migration。完成标记复用 `settings` 表的一个键。`payment_provider_instances.config` 的行内容由明文变为密文。
- **部署**：`deploy/.env.example` 与四个 compose 文件。新增变量 `SECURITY_SECRET_ENCRYPTION_KEY`；`TOTP_ENCRYPTION_KEY` 继续被透传和接受。
- **兼容性**：无外部 API breaking change。已经设置了 `TOTP_ENCRYPTION_KEY` 的运行中安装不需要做任何事。从未设置过的安装在第一次启用相关功能时会得到一条点名变量的启动错误，而不是静默的数据损坏。
- **回滚**：回滚代码后，`decryptConfig` 的旧密文 fallback 仍然存在，因此已被重加密的行依然可读——前提是密钥没有丢。这也是为什么必须先把 H5 的密钥治理修掉，再把 C4 的密文写回去：反过来做会把一批新密文绑定到一把每次重启都变的密钥上。

## Execution References

- `tasks.md`：按文件的实施顺序与验收勾选项。
- `design.md`：重加密回填的幂等性、部分执行、明文/密文判别和多副本互斥的具体设计与被否决的备选方案。
- `specs/payment-provider-secret-storage/spec.md`、`specs/app-secret-encryption-key/spec.md`：Requirement 与 Scenario。
