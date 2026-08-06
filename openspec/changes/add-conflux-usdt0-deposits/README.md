# add-conflux-usdt0-deposits

为每个用户分配 Conflux eSpace HD 钱包充值地址，监听指定 USDT0 ERC20 合约，在交易达到 `finalized` 后按 `1 USDT0 = 1 USD balance` 幂等增加平台余额。

本变更只覆盖首期充值闭环，不实现自动归集、自动退款、多网络、多 Token、实时汇率或用户自助找回误充值资产。

实施前先阅读：

1. `proposal.md`：范围、能力和影响。
2. `design.md`：数据模型、状态机、扫描器、HD 钱包和 API 决策。
3. `implementation-guide.md`：建议目录、依赖方向和落地顺序。
4. `commit-plan.md`：按依赖顺序拆分的 50 个小步提交及阶段门禁。
5. `tasks.md`：实施任务清单。
6. `verification.md`：验收矩阵与上线门禁。
7. `source-baseline.md`：官方链参数与目标仓库现状基线。
