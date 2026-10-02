# TypeSafe（System One）平台

`upstream/main` 已原生支持 TypeSafe 平台，本 fork 不再保留独立的平台骨架：平台常量、`POST /v1/systemone`
透传、账号创建与测试连接、配额与 composite 路由约束、计费兜底价、内容审核与 Prompt 审计提取、后台调度
快照、余额在途预留与 mandatory usage 记账都由上游实现。本分支只保留一个模型映射兼容补丁。

## 上游原生实现（不要重复声明）

- **平台常量**：`domain.PlatformTypeSafe` / `service.PlatformTypeSafe`，ent 构建期白名单、
  `AllowedQuotaPlatforms`、前端平台目录与配额 / composite 约束迁移均由上游提供。
- **端点**：`POST /v1/systemone`，挂在通用网关 handler / service 上（`handler/gateway_systemone.go`、
  `service/gateway_systemone.go`），请求体 `{model,state,questions}` 除模型映射外原样透传，非 OpenAI 协议、
  无流式。上游另有 handler 级门禁 `rejectSystemOneOnlyPlatform`，阻止 typesafe 流量进入其它协议链路。
- **校验**：`pkg/typesafe.ValidateSystemOneRequest`——`model` 必须字面等于 `jev-latest`；拒绝重复键、
  大小写 / Unicode 折叠变体键、`stream:true` 与非法 question 结构。
- **计费**：内置兜底价 `jev-latest`（仅输入计费，$0.042 / 1M tokens），`ForwardResult.Model` 固定
  `jev-latest`；上游响应回显的型号（如 `jev-1.13.0`）记录在 `UpstreamResponseModel`。
- **审计与调度**：内容审核 / Prompt 审计按 `typesafe_systemone` 协议提取 `state`/`questions` 文本，
  调度与渠道 restrict 走通用网关路径，无需本 fork 额外接线。

## 本 fork 的模型映射兼容补丁

上游把 `model` 钉死为 `jev-latest`，且渠道 / 账号 `model_mapping` 不写回请求体，因此旧实现依赖的别名
（例如 `jev-judge-v1`）会稳定 400。补丁只放开「先解析映射」这一步，不放宽任何校验：

1. `pkg/typesafe.ValidateSystemOneRequestForMapping`：与严格校验逐条相同的结构与键名卫生检查，但不校验
   模型名取值。
2. `service.SystemOneRoutingModel`：给出渠道映射后实际用于调度与上游请求的模型名（命中映射用映射结果，
   否则沿用客户端请求名）。
3. `service.ResolveSystemOneUpstreamBody`：先取分组渠道映射（`ResolveChannelMappingAndRestrict`），再叠加
   账号级 `model_mapping`（`Account.GetMappedModel`），把最终模型名写入请求体。
4. 改写后的请求体重跑 `ValidateSystemOneRequest`：实际发往上游的模型名必须仍是 `jev-latest`，否则按上游
   口径返回 400 `model must be jev-latest`，未知模型不会被发往上游。

重复键 / 大小写变体、question 结构、`stream` 等校验完全沿用上游，不做普遍放宽。映射同样不绕过账号型号
白名单与渠道 restrict：调度仍按渠道映射后的模型名做账号支持与 restrict 判定，账号 `model_mapping` 就是
白名单本身——未命中映射、无法把模型落到 `jev-latest` 的账号不会让请求通过。

## 生产信息

- 旧实现的私有迁移 `240_typesafe_platform.sql` 已在生产应用；切到上游后由上游迁移
  `241_add_typesafe_platform.sql` 承接。
- 前置条件：`241` 的两个 CHECK 列表缺 `ollama_cloud`，会收窄 ollama_cloud 的配额与 composite 路由约束
  （有存量行时启动期迁移中止）。该补丁属于 `priv-infra` 叠加层，本分支不改迁移。
