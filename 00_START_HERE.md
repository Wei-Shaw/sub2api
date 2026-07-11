# Sub2API 当前入口

更新时间：2026-07-11
当前状态：**内部 mock 可演示 / 工程通过 / 非生产 READY**

## 当前目标

把交付后修复与遗漏源文件收敛为可审查的本地 commits，并保持：

- QCanvas `POST /v1/video/tasks` → poll → `succeeded + result_url` 的 mock 路径。
- Windows + Docker Desktop 的本地一键试跑文档路径。
- 不调用真实 Provider、不触碰生产数据或真实支付。

## 阅读顺序

1. [当前现实](02_CURRENT_REALITY_STATUS.md)
2. [当前目标](docs/goals/03_CURRENT_GOAL.md)
3. [产品不变量](PRODUCT_INVARIANTS.md)
4. [架构护栏](ARCHITECTURE_GUARDRAILS.md)
5. [质量门禁](CODE_QUALITY_GATE.md)
6. [视频网关契约](docs/api/video-gateway-contract.md)
7. [最新审查包](docs/reviews/LATEST_REVIEW_PACKAGE.html)
8. [Cleanup Merge 审查](docs/superpowers/codex-handoff/deliverables/2026-07-11-GROK-CLEANUP-MERGE-review.md)

## 决策边界

mock 通过只证明内部试运行链路，不证明真实 Provider、真实计费、生产部署或公网访问可用。任何真实 Provider、生产数据、密钥读取、部署或外发都需要单独授权。
