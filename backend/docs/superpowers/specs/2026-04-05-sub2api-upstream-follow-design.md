# sub2api 上游跟进设计

## 背景

当前本地 `main` 已相对 `origin/main` 分叉：

- 本地 `HEAD = 883c9355`
- 远端 `origin/main = f585a15e`
- `merge-base = b384570d`

远端并不是零散一两条修复，而是沿着 `channel / billing / usage / admin UI` 一整批推进，包含：

- 渠道管理系统（`channel_service.go`、`channel_repo*.go`、`ChannelsView.vue` 等）
- 新的计费/定价解析能力（`model_pricing_resolver.go`、`pricing_service.go` 等）
- 多条 migration（`081` 到 `089`）
- usage/billing/UI 的一系列调整

与此同时，本地近 23 个提交也已经在同一条带上落下了大量定制设计，包括：

1. OpenAI `active/exhausted` 路由语义与 `-Sys` 机制
2. usage/billing 的“优先账号倍率 + priority 单价来源 + 用户可见价格因子”口径
3. `sub2api-openai` / OpenCode 推荐配置独立 provider 语义

因此，这次“跟进上游”不能按简单 merge 处理，否则极易把刚落下的本地设计覆盖掉。

## 目标

本次的目标不是直接合并所有上游改动，而是：

1. 在**本地设计优先**的前提下，对上游变更做结构化吸收设计；
2. 将上游改动分批次梳理成可吸收、需人工移植、应暂缓三类；
3. 为后续真正的代码跟进制定一份以本地语义为基线的移植计划。

## 非目标

本次明确不做：

1. 不直接执行 `git merge origin/main`
2. 不承诺“一次性跟完上游全部改动”
3. 不为了追上游而回退本地已确认的设计语义

## 本地优先语义基线

在后续任何上游吸收动作中，以下语义必须优先保留：

1. OpenAI `active/exhausted` 路由规则，以及 `-Sys` 相关 continuation 与目标组判定语义
2. usage/billing 的当前口径：
   - 优先账号倍率
   - `priority` 单价来源
   - 用户侧可见的价格因子、价格来源、单价说明
3. `sub2api-openai` / OpenCode 推荐配置的独立 provider 语义

这三块是本次上游跟进的“冻结边界”，上游变更只能在不破坏它们的前提下被吸收。

## 结构化吸收策略

### Batch A：低耦合增量

优先吸收那些与本地冻结语义弱耦合、结构独立的上游能力，例如：

- 新增但不直接触碰本地 billing/routing 主语义的后端 service/repository 文件
- 独立 admin API / handler
- 独立 migration
- 新图表或辅助前端组件

这类内容可以接近机械吸收，但仍要过一轮编译/测试验证。

### Batch B：中耦合接线层

吸收那些会触碰 wiring、DTO、routes、API 接口层，但还没有直接改动本地核心语义的变更，例如：

- handler/router 接线
- DTO / API 参数扩展
- admin/user 页面接线层补丁

这类变更不能盲目整块复制，但通常可以在保持本地语义的前提下合并。

### Batch C：高风险热点

以下文件/区域视为高风险热点，必须逐文件做语义对照后再决定是否移植：

- `backend/internal/service/billing_service.go`
- `backend/internal/service/gateway_service.go`
- `backend/internal/service/openai_gateway_service.go`
- `backend/internal/repository/usage_log_repo.go`
- `backend/internal/handler/dto/types.go`
- `backend/internal/handler/dto/mappers.go`
- `backend/ent/schema/usage_log.go`
- `frontend/src/views/admin/UsageView.vue`
- `frontend/src/components/admin/usage/*`
- `frontend/src/views/user/UsageView.vue`
- `frontend/src/types/index.ts`
- `frontend/src/i18n/locales/{zh,en}.ts`

这些正是本地与上游都集中改过的区域。即使 git 文本层能合上，也极易出现“语义表面兼容、实际互相覆盖”的问题。

## 第一阶段交付物

第一阶段不直接写代码，而是先产出一份**结构化吸收清单**，至少包含：

1. Batch A / B / C 的具体文件清单
2. 每一批里哪些上游提交/文件可以机械吸收
3. 哪些必须人工移植
4. 哪些当前建议暂缓
5. 对 Batch C 每个热点文件说明：
   - 本地语义必须保留什么
   - 上游新增了什么
   - 主要冲突点在哪里

只有这份清单经你确认后，才进入真正的移植实现计划。

## 风险与控制

### 风险 1：文本冲突不大，但语义冲突很大

最危险的不是 merge 冲突，而是 merge 成功以后把本地 billing/routing 语义静默改掉。

控制方式：

1. 先冻结本地语义边界
2. 将高风险热点全部升格为 Batch C
3. 对 Batch C 先做人读对照，再谈移植

### 风险 2：channel 体系与本地 usage/billing 改动互相覆盖

上游的 `channel` 体系已经进入模型定价、限制、usage 展示链路；本地也刚好在同一条 usage/billing 带上做了深改。

控制方式：

1. 不直接把 `channel` 相关的 `billing/usage` 逻辑整块并入
2. 先把 `channel` 独立能力归到 Batch A/B
3. 与 usage/billing 交叉区域列入 Batch C

### 风险 3：迁移顺序错误导致目标反复变动

如果边吸收边决定目标，很容易让“本地优先”变成“每碰到冲突再临时拍板”。

控制方式：

1. 先完成结构化吸收清单
2. 你确认清单后，再生成实施计划
3. 后续按批次执行，而不是跳着修

## 结论

这次“跟进上游”应该按如下顺序推进：

1. 不做直接 merge
2. 先冻结本地设计语义
3. 先产出 Batch A/B/C 结构化吸收清单
4. 只有在清单确认后，再进入真正的移植计划

这样可以在尽可能跟进上游的同时，把本地这几天已经确认过的 routing / billing / opencode 设计稳定保住。
