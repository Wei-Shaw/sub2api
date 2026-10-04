## 1. 出站模型映射

- [x] 1.1 新增 `applySystemOneModelMapping(account, body) []byte`：按 `account.ResolveMappedModel` 改写 body 的 `model`
- [x] 1.2 未命中映射、映射结果与原值相同、空 body、缺 `model`、nil 账号时原样返回
- [x] 1.3 `ForwardSystemOne` 在 `NewSystemOneRequest` 之前调用该函数
- [x] 1.4 入站校验器 `ValidateSystemOneRequest` 保持不变（客户端仍须发 `jev-latest`）

## 2. 账号连通性测试

- [x] 2.1 `testTypeSafeAccountConnection` 用 `account.ResolveMappedModel` 解析测试模型名
- [x] 2.2 测试事件 `test_start` 回显实际使用的模型名

## 3. 测试

- [x] 3.1 `TestForwardSystemOneAppliesModelMapping`：入站 `jev-latest` → 出站 `jev-1.13-free`，其余字段不变
- [x] 3.2 `TestForwardSystemOneWithoutMappingKeepsBodyVerbatim`：无映射时出站 body 逐字节不变（回归护栏）
- [x] 3.3 `TestApplySystemOneModelMapping` 表驱动：命中映射 / 未命中 / 结果相同 / 空 body / 缺 model / nil 账号
- [x] 3.4 既有 systemone 测试全部保持通过

## 4. 验证

- [x] 4.1 `go build ./...` 通过
- [x] 4.2 `go test -tags=unit ./internal/service/ -run 'SystemOne|TypeSafe'` 全部通过
- [x] 4.3 全量 `go test -tags=unit ./...`
- [x] 4.4 真实上游验证：`opencode.ai/zen/v1/systemone` 对 `jev-1.13-free` 返回 200、对 `jev-latest` 返回 401（改动依据）
- [x] 4.5 记录验证边界：未做真实账号端到端联调（需部署侧账号与 key），仅由单元测试覆盖出站 body 形状
