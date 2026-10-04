## Why

`typesafe` 平台的 SystemOne 路径把出站模型名写死为 `jev-latest`，且**完全不使用账号的 `model_mapping`**。这带来两个问题：

1. **无法对接非 TypeSafe 的 systemone 上游。** OpenCode Zen 也提供 systemone 协议端点（`/zen/v1/systemone`），但它只认自己的模型名。实测对照：

   | 打到 `opencode.ai/zen/v1/systemone` | 结果 |
   | --- | --- |
   | `model: jev-latest` | **401** `Model jev-latest is not supported` |
   | `model: jev-1.13-free` | **200 OK** |

   因此把 `typesafe` 账号的 `base_url` 指向 OpenCode Zen 时，无论怎么配 `model_mapping` 都不会生效，请求稳定 401。

2. **账号「测试连接」永远失败。** 测试路径同样写死 `jev-latest`，即使上游配置正确也无法通过自检。

写死点共三处：账号测试的 payload、网关入站校验器、以及出站转发（body 原样透传）。

## What Changes

- 出站转发（`ForwardSystemOne`）在发送前按账号 `model_mapping` 改写 body 的 `model` 字段。
- 账号「测试连接」（`testTypeSafeAccountConnection`）改用映射后的模型名，并在测试事件里回显实际使用的模型。
- 入站校验器（`ValidateSystemOneRequest`）**保持不变**：客户端仍必须发送规范模型名 `jev-latest`，映射只作用于出站方向。

未命中映射、映射结果与原值相同、或 body 形状异常时，一律原样返回，不中断请求。

## Capabilities

### New Capabilities
- `typesafe-systemone-model-mapping`：SystemOne 出站模型名的账号级映射，使 `typesafe` 平台能够对接模型命名不同的 systemone 上游。

### Modified Capabilities
<!-- openspec/specs 目前为空，没有既有能力需要修改。 -->

## Impact

- **后端**：`internal/service/gateway_systemone.go`（新增映射辅助函数并在转发前调用）、`internal/service/account_test_service_typesafe.go`（测试 payload 使用映射后的模型名）。
- **数据库**：无。
- **管理端 API**：无新增字段；复用既有的 `credentials.model_mapping`。
- **上游影响**：仅影响 `typesafe` 平台且配置了 `model_mapping` 的账号；未配置映射的账号出站 body 逐字节不变。
- **非目标**：不改动入站校验语义，不为 systemone 增加流式支持，不改动 `typesafe` 之外的平台。
