# User Usage

本 feature 持有 `/usage` 的本人请求记录、统计、导出、错误列表与详情，并向用户仪表盘提供用量数据接口。

- [data/datasources/usageDatasource.ts](data/datasources/usageDatasource.ts)：本人列表、统计、错误、dashboard 快照及 Key 已结算/待结算查询。
- [presentation/pages/UsagePage.vue](presentation/pages/UsagePage.vue)：筛选、分页、查询取消、导出和详情编排。
- [presentation/widgets](presentation/widgets/)：用户错误请求及详情。
- 共享用量组件由 [admin-usage](../admin-usage/README.md) 的具体公开出口持有；用户视图必须使用用户 audience，不渲染账号、上游端点或管理员成本字段。

前端只展示服务端返回的本人用量；API Key 过滤不改变用户边界。已结算金额、待结算请求与错误记录不能合并成同一成功计费口径。账务链路见 [关键请求链路](../../../../docs/REQUEST_LIFECYCLES.md)。

从 `frontend/` 执行 `pnpm exec vitest run src/features/usage src/features/dashboard-user src/features/admin-usage`；字段与费用口径变化需同步后端 usage DTO/handler 和 billing 回归。

`presentation/usageExport.ts` 持有每页 1000 条的导出请求、每页最多三次的 429 等待重试和取消；页面固定开始时的筛选、排序、页数及文件名，离页终止当前导出。Retry-After 由统一网络客户端读取，等待限制为 1–60 秒。
