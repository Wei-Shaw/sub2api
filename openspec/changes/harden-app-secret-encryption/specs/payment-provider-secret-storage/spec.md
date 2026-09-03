## ADDED Requirements

### Requirement: 支付服务商配置必须静态加密写入
系统 SHALL 在把 `payment_provider_instances.config` 写入数据库之前，用 AES-256-GCM 加密序列化后的配置。写路径 MUST NOT 产生新的明文行。密文格式 MUST 保持 `iv:authTag:ciphertext`（各段 base64），以保证回归窗口之前写入的旧密文继续可读。

#### Scenario: 保存服务商配置
- **WHEN** 管理员创建或更新一个支付服务商实例
- **THEN** 落库的 `config` 值 MUST NOT 是合法的 JSON 对象
- **THEN** 该值 MUST 能被同一把主密钥解回原始配置

#### Scenario: 敏感字段留空表示不修改
- **WHEN** 管理员提交的配置中某个敏感字段为空字符串
- **THEN** 系统 MUST 先解密既有配置，保留该字段的原值，再整体重新加密写回
- **THEN** 其余字段 MUST 按提交值更新

#### Scenario: 主密钥缺失
- **WHEN** 主密钥未配置
- **THEN** 保存操作 MUST 返回明确错误
- **THEN** 系统 MUST NOT 退回明文写入

### Requirement: 读路径必须同时接受密文与回归窗口明文
系统 SHALL 在读取 `payment_provider_instances.config` 时先尝试解析明文 JSON，失败后再尝试 AES-256-GCM 解密。明文分支 MUST 被记录为回归窗口内写入行的兼容 shim，而不是当前写入格式。两种分支都失败时，系统 MUST 按空配置处理并告警，让管理员可以在后台重新录入。

#### Scenario: 读取回归窗口写入的明文行
- **WHEN** 存储值是合法的 `map[string]string` JSON
- **THEN** 系统 MUST 直接返回该配置
- **THEN** 系统 MUST NOT 报错

#### Scenario: 读取密文行
- **WHEN** 存储值是 `iv:authTag:ciphertext` 格式
- **THEN** 系统 MUST 用主密钥解密并返回配置

#### Scenario: 密钥不匹配的密文
- **WHEN** 存储值是密文但当前主密钥无法解开
- **THEN** 系统 MUST 返回空配置并输出告警
- **THEN** 告警 MUST NOT 包含任何密文或密钥内容

### Requirement: 一次性重加密回填必须幂等
系统 SHALL 在启动阶段把回归窗口内写成明文的行重新加密，并 MUST 在 `settings` 表写入持久完成标记。标记存在时，后续启动 MUST 跳过整个步骤。即使标记被删除，重跑 MUST NOT 产生双重加密。

#### Scenario: 首次启动执行回填
- **WHEN** 完成标记不存在且存在明文行
- **THEN** 系统 MUST 把每个明文行重新加密写回
- **THEN** 系统 MUST 在全部行处理成功之后写入完成标记

#### Scenario: 标记已存在
- **WHEN** 完成标记已存在
- **THEN** 系统 MUST 直接返回
- **THEN** 系统 MUST NOT 扫描任何数据行

#### Scenario: 标记被删除后重跑
- **WHEN** 完成标记被人为删除且所有行已是密文
- **THEN** 系统 MUST 不修改任何行
- **THEN** 系统 MUST 重新写入完成标记

### Requirement: 回填必须能确定性区分明文与密文
系统 SHALL 用「能否 unmarshal 成 `map[string]string`」作为判别依据，MUST NOT 依赖长度、前缀猜测或试解密。判别为明文的行 MUST 被重加密；其余行 MUST 保持原样。

#### Scenario: 明文行
- **WHEN** 存储值能 unmarshal 成非 nil 的 `map[string]string`
- **THEN** 该行 MUST 被判定为明文并重加密

#### Scenario: 密文行
- **WHEN** 存储值不能 unmarshal 成 `map[string]string`
- **THEN** 该行 MUST 被跳过且内容保持逐字节不变

#### Scenario: 空值与 null
- **WHEN** 存储值为空字符串或 `null`
- **THEN** 该行 MUST 被跳过

### Requirement: 回填中途失败必须可安全重试
系统 SHALL 逐行独立更新，MUST NOT 把全部行包在单个事务中。进程在回填中途退出时，MUST NOT 写入完成标记，且已处理与未处理的行 MUST 都能被读路径正常读取。

#### Scenario: 回填执行到一半进程退出
- **WHEN** 部分行已重加密、部分行仍为明文时进程终止
- **THEN** 完成标记 MUST NOT 存在
- **THEN** 两种状态的行 MUST 都能被读路径正常解析
- **THEN** 下一次启动 MUST 只处理剩余的明文行并在成功后写标记

#### Scenario: 回填期间管理员并发修改同一行
- **WHEN** 某一行在被读取之后、写回之前被后台改写
- **THEN** 该行的写回 MUST 失败且影响行数为 0
- **THEN** 系统 MUST 保留管理员的新值，MUST NOT 覆盖

### Requirement: 回填必须在多副本间互斥
系统 SHALL 用 PostgreSQL Advisory Lock 串行化回填，并 MUST 使用与迁移运行器不同的锁 ID。取得锁之后 MUST 重新检查完成标记。

#### Scenario: 两个副本同时启动
- **WHEN** 两个实例同时进入回填步骤
- **THEN** 只有一个实例 MUST 实际执行重加密
- **THEN** 另一个实例 MUST 在取得锁后因标记已存在而零工作返回

#### Scenario: 锁不得与迁移串联
- **WHEN** 一个副本正在执行回填
- **THEN** 另一个副本的数据库迁移 MUST NOT 被该锁阻塞

#### Scenario: 释放锁
- **WHEN** 回填成功、失败或 context 取消
- **THEN** Advisory Lock MUST 被释放
- **THEN** 释放操作 MUST 使用独立超时，不得因数据库异常无限阻塞进程退出

### Requirement: 主密钥缺失时回填必须跳过而非失败
系统 MUST 在主密钥为空时记录告警并跳过回填，且 MUST NOT 写入完成标记，以便配置好密钥后的下一次启动真正执行回填。

#### Scenario: 无密钥启动
- **WHEN** 主密钥为空且存在明文行
- **THEN** 回填 MUST 跳过并输出告警
- **THEN** 完成标记 MUST NOT 被写入
- **THEN** 回填本身 MUST NOT 阻断启动
