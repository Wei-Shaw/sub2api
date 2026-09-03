# 设计：应用级密钥加密加固

## 1. 顺序约束：先修 H5，再修 C4

两个问题必须按固定顺序修，否则第二步会放大第一步的伤害。

`encryptConfig` 恢复加密之后，`payment_provider_instances.config` 的内容将绑定到 `payment.EncryptionKey`，而这把密钥由 `internal/payment/wire.go` 从同一个应用主密钥派生。如果主密钥仍然是「空则每次启动随机生成」，那么：

- 第一次启动生成密钥 K1，把支付配置加密成 C1；
- 重启后生成 K2，`decryptConfig` 无法解开 C1，按现有逻辑当作空配置吞掉；
- 管理员重新填一遍配置，得到 C2，绑定 K2；
- 再重启，循环。

也就是说，单独修 C4 会把「明文泄露」换成「支付配置每次重启就丢」。因此本变更把 H5 的密钥治理放在前面：主密钥要么显式配置，要么不存在（此时 `SecretEncryptor` 显式失败，`ProvideEncryptionKey` 已经返回 nil，`encryptConfig` 也就不会产生任何绑定到临时密钥的密文）。

## 2. 配置键解析

### 2.1 键与环境变量拼写

viper 的 `AutomaticEnv` 配合 `SetEnvKeyReplacer(strings.NewReplacer(".", "_"))`，把点分键大写后转成环境变量名。本仓库没有设置 `SetEnvPrefix`，所以映射是一一对应的：

| 配置键 | 环境变量 | 状态 |
| --- | --- | --- |
| `security.secret_encryption_key` | `SECURITY_SECRET_ENCRYPTION_KEY` | 新的规范名 |
| `totp.encryption_key` | `TOTP_ENCRYPTION_KEY` | 旧名，保留为只读别名 |

关键点：`AutomaticEnv` **不会**把一个键引入 `AllKeys()`，它只能覆盖已经在 `AllKeys()` 里的键。`AllKeys()` 是 `SetDefault` 键、配置文件键和显式 `BindEnv` 键的并集，而 `viper_bind_struct` 逃生舱在本仓库被编译掉了（`-tags embed`）。所以新键必须在 `setDefaults()` 里注册一条 `viper.SetDefault("security.secret_encryption_key", "")`，否则纯环境变量驱动的部署会把 `SECURITY_SECRET_ENCRYPTION_KEY` 读进来然后静默丢弃——这正是 `image_storage` 凭据丢失的同一个 bug。`internal/config/env_reachability_test.go` 的 `TestConfigKeysAreEnvReachable` 会守护这条不变式。

旧键 `totp.encryption_key` 的 `SetDefault` 保持原样，不删除。

### 2.2 解析优先级

在 `load()` 中，两个键都 unmarshal 完成后按以下顺序归一：

1. 两者都为空 → 结果为空，`Configured=false`。
2. 只有新键非空 → 用新键。
3. 只有旧键非空 → 用旧键，并 warn 一次，提示迁移到新变量名。
4. 两者都非空且相等 → 用该值，不 warn（同时设置两个变量是常见的滚动迁移做法）。
5. 两者都非空且不等 → 新键胜出，warn 一次点名冲突。规范名优先于兼容别名，否则运维无法通过设置新变量来覆盖一个遗留的旧变量。

### 2.3 为什么保留 `cfg.Totp.EncryptionKey` 作为镜像

全仓库有约十处读取 `cfg.Totp.EncryptionKey` 或 `cfg.Totp.EncryptionKeyConfigured`，分布在 `internal/repository`、`internal/payment`、`internal/service`、`internal/securityaudit`。把它们全部改名会跨越多个当前有其它 agent 在编辑的文件，并且会让本变更的 diff 被机械改名淹没。

因此 `load()` 在归一之后把解析结果**写回** `cfg.Totp.EncryptionKey` 和 `cfg.Totp.EncryptionKeyConfigured`。所有既有读取点继续工作且语义正确（它们读到的就是最终生效的主密钥）。`TotpConfig.EncryptionKey` 的文档注释改为明确标注这是兼容别名，规范位置是 `Security.SecretEncryptionKey`。后续把调用点迁到新字段是独立的机械改名，不属于本变更。

## 3. 移除自动生成之后的空密钥处理

原逻辑：空 → `generateJWTSecret(32)` → 得到一把随机密钥 → `EncryptionKeyConfigured=false`。

新逻辑：空 → 保持为空 → `Configured=false`。

这会撞到 `NewAESEncryptor`：它对空字符串做 `hex.DecodeString` 得到零长度，随后 `len(key) != 32` 返回 error，而它是 Wire provider，返回 error 会直接让整个进程起不来。全新安装因此会被卡死在一个它还不需要的密钥上。

解决方案是让 `NewAESEncryptor` 在密钥为空时返回 `disabledSecretEncryptor`——一个 `Encrypt` 和 `Decrypt` 都返回固定错误的实现，错误文本点名 `SECURITY_SECRET_ENCRYPTION_KEY`。

被否决的备选方案：

- **返回 nil encryptor**：调用方全部要加 nil 检查，且忘记检查的地方会 panic 而不是报错。
- **保留自动生成但持久化到 `security_secrets` 表**（和 JWT secret 一样）：这确实能修掉「每次重启都变」，而且诱人。但它把一把无人知晓的主密钥藏进数据库里——数据库备份从此包含解开自身全部密文的密钥，破坏了「密钥与密文分离」这个加密静态数据的基本前提。JWT secret 可以这么做是因为它不加密任何静态数据；主密钥不行。
- **密钥为空时直接让所有相关服务不可用**：等价于当前效果，但要改的服务文件多得多，收益相同。

## 4. 启动前置检查

`ensureSecretEncryptionKeyUsable(ctx, db, cfg)` 在密钥非空时立刻返回 nil；为空时才查库。

判定「已经有东西可丢」使用一组显式信号，每条都直接对应一个已确认的 `SecretEncryptor` 调用点：

| 信号 | 查询 | 对应调用点 |
| --- | --- | --- |
| 备份 S3 凭据 | `settings` 存在键 `backup_s3_config` | `backup_service.go` |
| 图片存储凭据 | `settings` 存在键 `image_storage_config` | `image_storage_settings.go` |
| Ollama Cloud 会话 | `settings` 存在键 `ollama_cloud_usage_settings` | `ollama_cloud_usage.go` |
| 提示词审计节点 token | `settings` 存在键 `prompt_audit_config` | `securityaudit/prompt_config_store.go` |
| 渠道监控开关 | `settings` 中 `channel_monitor_enabled` = `true` | `channel_monitor_service.go` |
| 插件管理开关 | `settings` 中 `plugin_management_enabled` = `true` | `plugin_manager.go` |
| TOTP 开关 | `settings` 中 `totp_enabled` = `true` | `totp_service.go` |
| 渠道监控已存密文 | `channel_monitors` 存在 `api_key <> ''` 的行 | `channel_monitor_service.go` |
| 插件已存密文 | `sub2api_plugin_installations` 存在 `config_encrypted <> ''` 的行 | `plugin_manager.go` |
| 用户已启用 2FA | `users` 存在 `totp_enabled = true` 的行 | `totp_service.go` |

设计取舍：

- 每条查询都用 `EXISTS`，只在密钥为空这条冷路径上执行，成本可忽略。
- 表可能不存在（旧库尚未跑到对应 migration，或未来表被重命名）。前置检查对「表不存在」一律按「该信号为假」处理并继续，绝不因为一次探测失败就阻断启动。前置检查的职责是**防止静默数据损坏**，不是给自己发明新的启动失败模式。
- 命中任意一条即返回错误，错误文本包含命中的信号名和要设置的变量名（新旧两种拼写都列出，因为运维手上可能是旧文档）。

## 5. 一次性重加密回填

### 5.1 明文与密文的判别

这一步必须是判定，不能是猜测。两种格式在结构上互斥：

- 明文行是 `json.Marshal(map[string]string)` 的输出，永远以 `{` 开头，且 `json.Unmarshal` 到 `map[string]string` 一定成功。
- 密文行是 `fmt.Sprintf("%s:%s:%s", b64, b64, b64)`，永远以 base64 字符开头，绝不可能是合法的 JSON 对象。

因此判别规则是：`json.Unmarshal([]byte(stored), &map[string]string{})` 成功 ⇒ 明文，需要重加密；失败 ⇒ 不动。

`stored == ""` 跳过。`stored == "null"` 会 unmarshal 成功但得到 nil map，同样跳过——没有任何秘密可保护，重加密一个空值只是给回滚增加噪音。

这条规则的漏判方向是安全的：一行被误判为密文只会被留在原地（下一次仍会被检查），不会被错误地二次加密。

### 5.2 幂等性

三层叠加：

1. **持久完成标记**。`settings` 表中的键 `payment_provider_config_encryption_backfill_v1`，值为完成时间与处理行数的 JSON。标记存在 ⇒ 整个步骤直接返回，一次查询即结束。
2. **判别本身即幂等**。即使标记被人为删除，重跑也只会挑出仍然是明文的行；已经是密文的行不匹配判别规则，会被跳过。所以重跑是安全的，不会产生双重加密。
3. **写入带条件守卫**。每行的更新是 `UPDATE payment_provider_instances SET config = $1 WHERE id = $2 AND config = $3`，其中 `$3` 是读到的原始明文。如果管理员在扫描和写回之间通过后台改了这一行，`RowsAffected` 为 0，回填放弃这一行而不是覆盖管理员的新值。这一行的新值已经是密文（因为 `encryptConfig` 已经修好），所以放弃它是正确的。

### 5.3 部分执行

回填**不是**一个大事务。逐行独立更新，理由是：一次事务包住全部行会在实例很多时持有长事务，而这一步跑在启动路径上、拿着 advisory lock，长事务会把其它副本的启动一起拖住。

进程在中途死掉的后果因此是：一部分行已经是密文，一部分行还是明文，标记**没有**写入（标记只在整趟成功走完之后写）。下一次启动重新走一遍，只处理剩下的明文行，然后写标记。中间态对读路径完全无害——`decryptConfig` 同时接受密文和明文，两种行都能正常读。

这正是「标记只在全部成功后写」这个选择的意义：宁可重跑一次廉价的扫描，也不要让一个半成品状态被标记成已完成。

### 5.4 多副本互斥

需要锁。两个副本同时启动、同时扫描到同一批明文行时，两者会各自用不同的随机 nonce 加密出不同的密文，然后互相覆盖。结果不会损坏数据（两份密文都能解开），但会产生一次无意义的写竞争，并且第二个 writer 的条件守卫会因为第一个已经改掉了 `config` 而全部落空——回填会被误判为「没有明文行」，然后写下完成标记，而实际上另一副本可能才处理了一半。这是真实的正确性问题。

因此使用 PostgreSQL Advisory Lock，模式照抄 `internal/repository/migrations_runner.go`：`pg_try_advisory_lock` 轮询直到拿到锁或 ctx 超时，`defer` 中用独立超时的 context 释放。

但**不复用** `migrationsAdvisoryLockID`，改用一个独立的常量 ID。理由：

- 复用会把回填和迁移串在同一把锁上。副本 A 在做回填时，副本 B 连**迁移**都启动不了——两件不相干的事被耦合成一条队列。
- 迁移运行器在 `InitEnt` 中先跑完并已经释放了锁，所以此处复用不会自死锁；但那是靠调用顺序侥幸成立的，一旦顺序变动就会变成启动期死锁。独立 ID 让这个不变式不依赖调用顺序。

拿到锁之后**重新检查一次完成标记**（双重检查）。第二个副本在锁上等待期间，第一个副本可能已经跑完并写下标记；重新检查让第二个副本零工作返回。

### 5.5 密钥缺失时的行为

密钥为空时，回填记录一条 warn 并直接返回，**不写标记**。这样一旦运维配好密钥再启动，回填就会真正执行。此处不返回错误：判断「密钥缺失是否应该阻断启动」是前置检查的职责，回填不重复这个决策，也不为同一件事发明第二种失败方式。

## 6. 未决问题

无。所有取舍已在上文记录。
