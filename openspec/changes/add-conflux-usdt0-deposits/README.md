# add-conflux-usdt0-deposits

为每个用户分配可跨已配置 Conflux eSpace 网络复用的 HD 钱包充值地址，在每个网络监听其唯一配置的 USD 稳定币 ERC20 合约，并在交易达到 `finalized` 后按 `1 Token = 1 USD` 幂等增加 Web3 子余额。

> **核心资产假设**
>
> - 当前 Web3 充值只支持经过产品和运维确认的**美元稳定币**，并直接按 `1 Token = 1 USD` 计入内部 `usdt` Web3 余额；系统没有价格预言机、实时汇率或稳定币脱锚处理。
> - 每个启用网络默认且经过验证的配置只包含**一种充值 Token**。`assets` 使用映射结构是为了保留配置 schema 的扩展能力，不表示“同一网络多 Token”已经获得产品支持。
> - 非美元稳定币、波动资产，或同一网络的第二种充值 Token，不得仅通过增加配置直接上线；必须先补充计价与余额隔离设计、Spec、实现和验收测试。

本变更只覆盖充值闭环，不实现自动归集、自动退款、同一网络多 Token、实时汇率或用户自助找回误充值资产。

实施前先阅读：

1. `proposal.md`：范围、能力和影响。
2. `design.md`：数据模型、状态机、扫描器、HD 钱包和 API 决策。
3. `implementation-guide.md`：建议目录、依赖方向和落地顺序。
4. `commit-plan.md`：按依赖顺序拆分的 50 个小步提交及阶段门禁。
5. `tasks.md`：实施任务清单。
6. `verification.md`：验收矩阵与上线门禁。
7. `source-baseline.md`：官方链参数与目标仓库现状基线。
