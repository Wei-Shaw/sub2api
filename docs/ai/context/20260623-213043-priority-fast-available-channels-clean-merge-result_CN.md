# priority/fast 1.5 倍与可用渠道价格页干净合并记录

## 背景

- 当前工作区存在多组未提交改动，其中包含支付、一次性流量包、用量扣费等其他任务内容。
- 本次只整理并合并用户刚确认的改动：`/available-channels` 价格展示、普通用户侧边栏隐藏 `/monitor`、`priority` / `fast` 从 2 倍改为 1.5 倍。

## 本次提交范围

- 后端新增用户侧只读价格接口 `/api/v1/channels/prices`，从 `ModelPricingResolver` 读取 `gpt-5.4`、`gpt-5.5` 的实际计费价格。
- 前端 `/available-channels` 顶部展示 GPT token 单价、priority/fast 单价，以及当前用户可用分组里的生图 1K/2K/4K 单价。
- 普通用户侧边栏暂不展示 `/monitor`，但保留路由和后台监控能力。
- `BillingService` 将 OpenAI `service_tier=priority` 及客户端别名 `fast` 的扣费倍率统一为 1.5 倍，并让展示价格与实际扣费口径一致。

## 排除范围

- 不提交支付服务、支付订单、一次性流量包、流量包迁移、用量扣费扩展等当前工作区里的其他未提交改动。
- 不回滚这些其他改动，仅保持它们继续留在工作区。

## 验证

- 合并前后需要重新运行后端相关单元测试、前端相关组件测试和前端构建。
