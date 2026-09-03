## ADDED Requirements

### Requirement: 应用主密钥必须有一个与用途相符的规范配置键
系统 SHALL 使用 `security.secret_encryption_key` 作为应用级 `SecretEncryptor` 主密钥的规范配置键。该键 MUST 通过环境变量 `SECURITY_SECRET_ENCRYPTION_KEY` 可达。密钥格式 MUST 为 64 位十六进制字符（32 字节 AES-256）。

#### Scenario: 通过环境变量配置
- **WHEN** 部署只设置了 `SECURITY_SECRET_ENCRYPTION_KEY` 而没有配置文件
- **THEN** 系统 MUST 使用该值作为主密钥
- **THEN** 该值 MUST NOT 被静默丢弃

#### Scenario: 通过配置文件配置
- **WHEN** `config.yaml` 中设置了 `security.secret_encryption_key`
- **THEN** 系统 MUST 使用该值作为主密钥

#### Scenario: 非法密钥
- **WHEN** 主密钥非空但不是 64 位十六进制
- **THEN** 系统 MUST 启动失败并给出明确错误
- **THEN** 系统 MUST NOT 退化为空密钥或随机密钥

### Requirement: 旧配置键必须作为只读别名继续生效
系统 SHALL 继续接受 `totp.encryption_key` / `TOTP_ENCRYPTION_KEY`，使已在运行的安装无需改动即可升级。两种拼写 MUST 都能解析到同一个生效值。

#### Scenario: 只设置了旧变量
- **WHEN** 部署只设置了 `TOTP_ENCRYPTION_KEY`
- **THEN** 系统 MUST 使用该值作为主密钥
- **THEN** 系统 MUST 输出一次迁移提示告警

#### Scenario: 两个变量都设置且相同
- **WHEN** `SECURITY_SECRET_ENCRYPTION_KEY` 与 `TOTP_ENCRYPTION_KEY` 值相同
- **THEN** 系统 MUST 使用该值
- **THEN** 系统 MUST NOT 输出冲突告警

#### Scenario: 两个变量都设置且不同
- **WHEN** 两个变量值不同
- **THEN** 规范键 `SECURITY_SECRET_ENCRYPTION_KEY` MUST 胜出
- **THEN** 系统 MUST 输出一次冲突告警
- **THEN** 告警 MUST NOT 包含任何一个密钥的取值

### Requirement: 系统禁止自动生成应用主密钥
系统 MUST NOT 在主密钥为空时生成随机密钥。空密钥 MUST 保持为空，并 MUST 被标记为「未配置」。

#### Scenario: 空密钥启动
- **WHEN** 两个变量都未设置
- **THEN** 生效密钥 MUST 为空
- **THEN** 系统 MUST NOT 生成任何随机密钥
- **THEN** 「密钥已配置」标记 MUST 为 false

#### Scenario: 重启后的密钥稳定性
- **WHEN** 同一份配置连续启动两次
- **THEN** 两次的生效密钥 MUST 相同

### Requirement: 空密钥下的加密器必须显式失败
系统 SHALL 在主密钥为空时提供一个 `Encrypt` 与 `Decrypt` 均返回错误的 `SecretEncryptor` 实现。该实现 MUST NOT 返回 nil，MUST NOT panic，且错误文本 MUST 点名需要设置的环境变量。

#### Scenario: 无密钥时尝试写入密文
- **WHEN** 某个功能在主密钥为空时调用 Encrypt
- **THEN** 调用 MUST 返回错误
- **THEN** 错误文本 MUST 包含 `SECURITY_SECRET_ENCRYPTION_KEY`
- **THEN** 系统 MUST NOT 写入任何可被后续启动误判为有效的数据

#### Scenario: 无密钥时构造加密器
- **WHEN** 主密钥为空
- **THEN** 加密器构造 MUST 成功返回一个非 nil 实现
- **THEN** 进程 MUST NOT 因此启动失败

### Requirement: 已有密文或已启用相关功能时空密钥必须阻断启动
系统 SHALL 在主密钥为空时检查数据库中是否已存在依赖 `SecretEncryptor` 的密文或已启用的相关功能。命中任一信号时，系统 MUST 启动失败，错误信息 MUST 点名命中的信号与需要设置的环境变量。检查范围 MUST 至少覆盖备份 S3、渠道监控、图片存储、插件、Ollama Cloud、提示词审计节点凭据和 TOTP。

#### Scenario: 已存在备份 S3 凭据
- **WHEN** 主密钥为空且 `settings` 中存在 `backup_s3_config`
- **THEN** 启动 MUST 失败
- **THEN** 错误信息 MUST 同时给出新旧两种环境变量拼写

#### Scenario: 已有用户启用 2FA
- **WHEN** 主密钥为空且存在 `totp_enabled = true` 的用户
- **THEN** 启动 MUST 失败

#### Scenario: 已存在渠道监控密文
- **WHEN** 主密钥为空且 `channel_monitors` 中存在非空 `api_key`
- **THEN** 启动 MUST 失败

#### Scenario: 密钥已配置
- **WHEN** 主密钥非空
- **THEN** 系统 MUST 跳过全部探测查询
- **THEN** 启动 MUST 不受这些信号影响

### Requirement: 空密钥且无相关功能时必须允许启动
系统 SHALL 在主密钥为空且所有探测信号均未命中时继续启动，并 MUST 输出一次说明性告警。全新安装 MUST NOT 因为一个尚未被使用的密钥而被阻断。

#### Scenario: 全新安装首次启动
- **WHEN** 主密钥为空且数据库中没有任何密文或已启用的相关功能
- **THEN** 启动 MUST 成功
- **THEN** 系统 MUST 输出一次告警，说明哪些功能在配置密钥前不可用

#### Scenario: 探测所依赖的表尚不存在
- **WHEN** 某张探测目标表在当前 schema 中不存在
- **THEN** 该信号 MUST 被视为未命中
- **THEN** 探测失败本身 MUST NOT 阻断启动

### Requirement: 部署样例与全部 compose 文件必须实际透传主密钥
系统的部署资产 SHALL 保证运维设置的变量真正到达容器。`deploy/.env.example` MUST 同时记录规范变量与兼容别名；`deploy/` 下的每个 compose 文件 MUST 在其服务的 `environment` 中透传规范变量。

#### Scenario: 标准 compose 部署
- **WHEN** 运维在 `.env` 中设置 `SECURITY_SECRET_ENCRYPTION_KEY` 并使用 `docker-compose.yml`
- **THEN** 容器内 MUST 能读到该变量

#### Scenario: standalone compose 部署
- **WHEN** 运维在 `.env` 中设置 `SECURITY_SECRET_ENCRYPTION_KEY` 并使用 `docker-compose.standalone.yml`
- **THEN** 容器内 MUST 能读到该变量

#### Scenario: 仅设置了旧变量的存量部署
- **WHEN** 运维的 `.env` 中只有 `TOTP_ENCRYPTION_KEY`
- **THEN** 每个 compose 文件 MUST 继续透传该变量
- **THEN** 升级 MUST NOT 要求运维修改 `.env`
