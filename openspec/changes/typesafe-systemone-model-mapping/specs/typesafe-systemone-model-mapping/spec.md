## Purpose

规定 `typesafe` 平台 SystemOne 出站请求的模型名解析方式，使该平台能够对接模型命名与 TypeSafe 不同的 systemone 兼容上游，同时保持入站校验语义不变。

## ADDED Requirements

### Requirement: 出站模型名按账号映射解析
`typesafe` 平台转发 SystemOne 请求时，系统 MUST 在发送前按账号的 `model_mapping` 解析出站模型名，并用解析结果改写 body 的 `model` 字段。入站校验 MUST 保持不变：客户端仍须发送规范模型名，映射只作用于出站方向。

未命中映射、映射结果与原值相同、body 为空、缺少 `model` 字段或 `model` 非字符串时，系统 MUST 原样转发 body，MUST NOT 因映射失败而中断请求。

#### Scenario: 命中映射时改写
- **WHEN** 账号配置 `model_mapping` 为 `{"jev-latest": "jev-1.13-free"}`，客户端发送 `model: jev-latest`
- **THEN** 出站 body 的 `model` MUST 为 `jev-1.13-free`
- **THEN** body 的其余字段 MUST 保持不变

#### Scenario: 未配置映射时逐字节透传
- **WHEN** 账号没有 `model_mapping`
- **THEN** 出站 body MUST 与入站 body 逐字节相同

#### Scenario: 映射结果与原值相同
- **WHEN** 账号的映射把 `jev-latest` 映射为 `jev-latest`
- **THEN** 出站 body MUST 原样转发

#### Scenario: 异常输入不中断请求
- **WHEN** body 为空、缺少 `model` 字段，或账号为 nil
- **THEN** 系统 MUST 原样返回该 body
- **THEN** 系统 MUST NOT 因此报错

### Requirement: 账号连通性测试使用映射后的模型名
`typesafe` 账号的「测试连接」MUST 使用按 `model_mapping` 解析后的模型名构造 SystemOne 请求，并在测试事件中回显实际使用的模型名。未命中映射时 MUST 沿用规范模型名。

#### Scenario: 配置了映射的账号
- **WHEN** 对配置 `{"jev-latest": "jev-1.13-free"}` 的账号执行连通性测试
- **THEN** 测试请求的 `model` MUST 为 `jev-1.13-free`
- **THEN** 测试事件回显的模型名 MUST 为 `jev-1.13-free`

#### Scenario: 未配置映射的账号
- **WHEN** 对未配置 `model_mapping` 的账号执行连通性测试
- **THEN** 测试请求的 `model` MUST 为 `jev-latest`
