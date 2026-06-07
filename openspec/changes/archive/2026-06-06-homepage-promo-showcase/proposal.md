## Why

首页（`HomeView`）当前底部展示的是"Supported Providers"图标墙——既无转化路径，也不能反映已经上线的"价格广场 + 充值赠送活动"等真正驱动用户决策的功能。我们刚把 `recharge-bonus-promo` 改完并归档，但匿名访客在首页完全感知不到"现在有活动"，只能在登录并进入 `/purchase` 后才看见。换句话说，最高曝光的页面浪费在了静态品牌展示上，最强转化信号却藏在登录墙后。

这次改版把首页底部"展示型 providers"换成"转化型 promo + plans"——有活动时把活动横幅顶上去，下面继续放 3 张精选套餐卡片；没活动时只放套餐卡片。匿名访客也能看见，点 CTA 走"未登录 → /login → /purchase 或 /plaza/plans"的现成路径。

## What Changes

- **删除** `HomeView` 里 "Supported Providers" 区块（含 5 个 logo 卡 + i18n 文案）。
- **新增** `HomeShowcaseSection` 组件，渲染逻辑：
  - 当存在生效的 `recharge_promo` 时，在区域顶部叠加 `HomePromoBanner`（标题用活动 `name` 字段、副标题列阶梯、到期时间）。
  - **始终**渲染套餐卡片网格（最多 3 张），下方 "查看全部套餐 →" 链接跳 `/plaza/plans`。
  - 套餐为空 / `payment_enabled = false` 时整段不渲染。
- **新增** 后端公开端点 `GET /api/v1/plaza/recharge-promo`（匿名可访问）——返回当前生效活动的展示用字段：`name`、`valid_from`、`valid_until`、`tiers`、`version`。无生效活动时返回 `{ promo: null }`。
- **修改** `pricing-plaza` capability 中的 `Homepage Promotes Plaza` 需求——把旧的 "View model pricing" 二级 CTA + `PricingTeaser` 替换为新的 `HomeShowcaseSection`（活动+套餐）契约。Hero 区主 CTA 保留；顶部导航的 plaza 链接保留。
- **修改** `recharge-bonus` capability，新增"公开匿名展示端点"需求——`GET /api/v1/plaza/recharge-promo` 的语义、字段和无活动时的返回形态。
- **复用** 既有 `PlanPlazaCards` 组件（带数量限制 prop）展示套餐卡，避免重复实现卡片样式与 Buy CTA 行为。
- **首页不展示红点**——`useRechargePromoDot` 不在首页调用；红点只在 `PaymentView`/sidebar 上督促。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `pricing-plaza`：`Homepage Promotes Plaza` 需求重写——把旧的 "View model pricing" CTA + `PricingTeaser` 行为替换为新的 promo + plans 展示契约，删除"Supported Providers"暗含的展示职责。
- `recharge-bonus`：新增 `Public Recharge Promo Endpoint` 需求——定义匿名可访问的 `/api/v1/plaza/recharge-promo`，列出返回字段（含 `name`）、无活动时返回形态、与既有 authenticated `checkout-info.recharge_promo` 的字段一致性。

## Impact

- **后端**：
  - 新建 `internal/handler/public/recharge_promo_handler.go`（或落到 `plaza_handler.go` 旁边）；
  - 新增路由 `GET /api/v1/plaza/recharge-promo` 挂在 `public` / `plaza` 组下；
  - `RechargePromo` 领域结构体增加 `Name string` 字段，并在 `ActivityToPromo` / `serializeRechargePromoSetting` 里透传；
  - `checkout-info` 端点也顺带返回 `name`（向后兼容，老前端忽略即可）。
- **前端**：
  - `HomeView.vue` 删除 Providers 段，挂入 `HomeShowcaseSection.vue`；
  - 新组件 `HomeShowcaseSection.vue` / `HomePromoBanner.vue`；
  - 复用 `PlanPlazaCards.vue`，新增 `maxItems` / `viewAllHref` prop（或在父层裁剪 + 自渲染"查看全部"链接）；
  - `paymentApi`（或 plaza 客户端）新增 `getPublicRechargePromo()`；
  - i18n：`zh-CN` / `en-US` 新增 `home.showcase.*`、`home.promo.*`、`home.plans.view_all` 等键，删除 `home.providers.*`；
  - 删除 `home.cta_view_pricing` 相关用法（如不再使用则同步清理 i18n 键）。
- **不影响**：admin 活动管理 UI、`PaymentView` 红点、refund/fulfillment 计算、其他公开端点（`/api/v1/plaza/models`、`/api/v1/plaza/plans`）。
- **测试**：
  - 后端 handler 单测覆盖"有活动 / 无活动 / 过期 / 未启用"四种返回；
  - 前端组件测试：promo 存在/不存在两种渲染分支、点击 "查看全部" 跳转、`payment_enabled = false` 时整段不渲染。
