# 验证：SystemOne 出站模型映射（2026-10-04）

## 问题

`typesafe` 平台的 SystemOne 出站模型名被写死为 `jev-latest`，账号的 `model_mapping` 完全不生效。

实测（真实上游，`Authorization: Bearer public`）：

| 打到 `https://opencode.ai/zen/v1/systemone` | 结果 |
| --- | --- |
| `model: jev-latest` | **401** `{"type":"error","error":{"type":"ModelError","message":"Model jev-latest is not supported"}}` |
| `model: jev-1.13-free` | **200 OK**，返回 `{"model":"jev-1.13-free","answers":{...},"cost":"0"}` |

生产环境日志佐证（4 次账号测试全部失败于同一原因）：

```
service/account_test_service_typesafe.go:85
Account test error: API returned 401:
{"type":"error","error":{"type":"ModelError","message":"Model jev-latest is not supported"}}
```

## 写死点（改动前）

| # | 位置 | 行为 |
| --- | --- | --- |
| 1 | `account_test_service_typesafe.go` | 测试 payload 的 `model` 写死 `typesafe.JevLatestModel` |
| 2 | `pkg/typesafe/systemone.go:41` | 入站校验器要求 `model == "jev-latest"` |
| 3 | `service/gateway_systemone.go` | `ForwardSystemOne` 把 body 原样透传，**`model_mapping` 零引用** |

## 改动

| 位置 | 改动 |
| --- | --- |
| `gateway_systemone.go` | 新增 `applySystemOneModelMapping`，在 `NewSystemOneRequest` 前调用 |
| `account_test_service_typesafe.go` | 测试模型名改用 `account.ResolveMappedModel` 解析，并在 `test_start` 事件回显 |
| `pkg/typesafe/systemone.go` | **不改**（入站语义保持：客户端仍须发规范模型名） |

## 自动验证

- `go build ./...`：通过
- `go test -tags=unit ./internal/service/ -run 'SystemOne|TypeSafe'`：全部通过，含新增的 3 个测试
  - `TestForwardSystemOneAppliesModelMapping`：入站 `jev-latest` → 出站 `jev-1.13-free`，`state` 与 `questions` 保持不变
  - `TestForwardSystemOneWithoutMappingKeepsBodyVerbatim`：无映射时出站 body 逐字节相同
  - `TestApplySystemOneModelMapping`：命中映射 / 未命中 / 映射结果相同 / 空 body / 缺 `model` / nil 账号
- `gofmt -l`：无输出

## 验证边界

1. **未做真实账号端到端联调。** 单元测试用 httptest 模拟上游，断言出站 body 的 `model` 已被改写；未用真实 `typesafe` 账号打真实上游（需部署侧账号与 key）。
2. **只覆盖 `typesafe` 平台。** 其他平台的模型映射路径未改动。
3. **入站仍要求 `jev-latest`。** 客户端不能直接发送映射后的模型名——若发送 `jev-1.13-free`，入站校验会以 `model must be jev-latest` 拒绝。这是刻意的：映射是出站方向的改写，不是入站语义的放宽。
