## 1. 密钥治理（必须先于第 3 节完成）

- [x] 1.1 在 `SecurityConfig` 新增 `SecretEncryptionKey`（`mapstructure:"secret_encryption_key"`）与 `SecretEncryptionKeyConfigured`（`mapstructure:"-"`）
- [x] 1.2 在 `setDefaults()` 注册 `viper.SetDefault("security.secret_encryption_key", "")`，保留既有的 `totp.encryption_key` 默认值
- [x] 1.3 在 `load()` 中实现新旧键归一：新键优先、旧键回退、冲突告警、只用旧键时输出迁移提示
- [x] 1.4 删除空密钥时的 `generateJWTSecret(32)` 自动生成分支
- [x] 1.5 把归一结果镜像回 `cfg.Totp.EncryptionKey` / `cfg.Totp.EncryptionKeyConfigured`，保证约十处既有读取点无需修改
- [x] 1.6 把 `TotpConfig.EncryptionKey` 的注释改为「兼容别名，规范位置见 Security.SecretEncryptionKey」
- [x] 1.7 让 `NewAESEncryptor` 在密钥为空时返回显式失败的 `disabledSecretEncryptor` 而不是 error，避免全新安装被卡住
- [x] 1.8 `disabledSecretEncryptor` 的错误文本点名 `SECURITY_SECRET_ENCRYPTION_KEY`
- [x] 1.9 确认 `TestConfigKeysAreEnvReachable` 通过，新键在 `AllKeys()` 中可达

## 2. 启动前置检查

- [x] 2.1 新建 `internal/repository/secret_encryption_startup.go`
- [x] 2.2 实现 `ensureSecretEncryptionKeyUsable`：密钥非空时零查询返回
- [x] 2.3 实现 settings 键探测：`backup_s3_config`、`image_storage_config`、`ollama_cloud_usage_settings`、`prompt_audit_config`
- [x] 2.4 实现 settings 开关探测：`channel_monitor_enabled`、`plugin_management_enabled`、`totp_enabled`
- [x] 2.5 实现数据行探测：`channel_monitors.api_key`、`sub2api_plugin_installations.config_encrypted`、`users.totp_enabled`
- [x] 2.6 表不存在时按信号未命中处理，探测失败不得阻断启动
- [x] 2.7 命中时返回的错误文本包含命中信号名与新旧两种环境变量拼写
- [x] 2.8 全部未命中时输出一次说明性告警并继续启动

## 3. 恢复支付配置加密

- [x] 3.1 `encryptConfig` 恢复调用 `payment.Encrypt`
- [x] 3.2 密钥缺失或长度非法时返回明确错误，不得退回明文写入
- [x] 3.3 撤销 `internal/payment/crypto.go` 上 `Encrypt` / `Decrypt` 的 `Deprecated` 标记，改写为描述当前方向的文档
- [x] 3.4 `decryptConfig` 保留明文分支，注释改为「回归窗口写入行的读兼容 shim」，删除方向写反的 TODO
- [x] 3.5 删除 `decryptConfig` 中已不再需要的 `//nolint:staticcheck` 抑制

## 4. 一次性重加密回填

- [x] 4.1 在 `secret_encryption_startup.go` 中实现 `reencryptPaymentProviderConfigs`
- [x] 4.2 完成标记：`settings` 键 `payment_provider_config_encryption_backfill_v1`，值含完成时间与处理行数
- [x] 4.3 取锁前先查标记，命中即零工作返回
- [x] 4.4 使用独立的 Advisory Lock ID，不复用 `migrationsAdvisoryLockID`
- [x] 4.5 取得锁后重新检查标记（双重检查）
- [x] 4.6 `defer` 中用独立超时的 context 释放锁
- [x] 4.7 明文判别只用 `json.Unmarshal` 到 `map[string]string` 是否成功，跳过空串与 nil map
- [x] 4.8 逐行独立更新，不使用包住全部行的大事务
- [x] 4.9 写回带条件守卫 `WHERE id = $2 AND config = $3`，影响行数为 0 时放弃该行
- [x] 4.10 只在整趟成功后写标记
- [x] 4.11 密钥为空时告警跳过、不写标记、不阻断启动
- [x] 4.12 在 `InitEnt` 中于 `ensureBootstrapSecrets` 之后接线前置检查与回填

## 5. 部署资产

- [x] 5.1 `deploy/.env.example` 新增 `SECURITY_SECRET_ENCRYPTION_KEY`，保留 `TOTP_ENCRYPTION_KEY` 并标注为兼容别名
- [x] 5.2 `.env.example` 的说明改写：不再宣称「留空会生成随机密钥」，改为说明留空的真实后果
- [x] 5.3 `docker-compose.yml` 透传新变量
- [x] 5.4 `docker-compose.local.yml` 透传新变量
- [x] 5.5 `docker-compose.dev.yml` 透传新变量
- [x] 5.6 `docker-compose.standalone.yml` 补上完全缺失的透传（新旧变量都要有）
- [x] 5.7 逐文件确认变量确实出现在服务的 `environment` 块内，而不只是写在 `.env.example` 里

## 6. 测试

- [x] 6.1 `encryptConfig` 输出不是合法 JSON 且可被 `decryptConfig` 解回
- [x] 6.2 `decryptConfig` 对密文、回归窗口明文、密钥不匹配密文三种输入的行为
- [x] 6.3 密钥缺失时 `encryptConfig` 返回错误
- [x] 6.4 敏感字段留空时的 merge 语义在加密恢复后仍然正确
- [x] 6.5 配置键归一的五种组合（都空 / 只新 / 只旧 / 相同 / 冲突）
- [x] 6.6 空密钥不再自动生成，且两次 load 结果一致
- [x] 6.7 新键在 viper `AllKeys()` 中可达
- [x] 6.8 `disabledSecretEncryptor` 的 Encrypt/Decrypt 均返回含变量名的错误
- [x] 6.9 明文/密文判别函数的表驱动测试，覆盖空串、`null`、明文、密文、垃圾数据
- [x] 6.10 回填在密钥为空时跳过且不写标记
