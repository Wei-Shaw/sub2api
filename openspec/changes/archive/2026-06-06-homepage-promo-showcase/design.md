## Context

`HomeView.vue` 是匿名可见的着陆页（路由 `requiresAuth: false`），目前的底部段是"Supported Providers"——5 个 logo 卡 + i18n 文案，纯展示，没有转化路径。这个段在 `feat/pricing` 分支上几乎是 dead weight：真正能驱动注册/充值的两个东西（套餐 cards 和充值赠送活动）都藏在登录后的 `PaymentView` 里，而新归档的 `recharge-bonus` capability 把活动信息也只暴露在 `GET /api/payment/checkout-info`（authenticated）。

同时项目里已经有完备的"匿名可访问"端点基础设施：
- `RegisterPlazaRoutes`（`backend/internal/server/routes/plaza.go`）挂在 `/api/v1/plaza/*` 下，配 Redis 限流（60 req/min/IP，fail-open）。
- `RechargePromoActivityService.GetCurrent(ctx)` 已经能从 `recharge_promo_activities` 表取出当前生效活动，`ActivityToPromo` 将其转换为 `RechargePromo`（service 层结构体）。
- `RechargePromo` 的 `Version` token 已稳定（hash-based），可作为 frontend 缓存 key。

约束：
- 匿名访客也要看到活动（用户已确认）→ 必须有公开端点。
- 活动卡和套餐卡**叠加**展示；红点不在首页出现。
- 活动 `name` 字段（schema 里 `MaxLen(120)`，default "默认活动"）要透传到前端做运营文案位。
- 首页只展示 **3 个**精选套餐 + "查看全部 →" 链接到 `/plaza/plans`。
- 不影响现有 admin / `PaymentView` / fulfillment / refund 流程。

## Goals / Non-Goals

**Goals:**

- 用一个新的 `HomeShowcaseSection` 替换 `HomeView` 末端的 Providers 段，做到"有活动就顶 banner，永远展示套餐预览"。
- 新增 `GET /api/v1/plaza/recharge-promo` 公开端点；和现有 `/api/v1/plaza/{models,plans}` 同风格、同限流、同 fail-open 策略。
- 把 `name` 字段从 ent schema → service struct → API DTO 一路透传，不偷懒到前端拼字符串。
- 套餐卡片复用 `PlanPlazaCards.vue`（已包含 `payment_enabled` 守卫和 Buy CTA 的登录跳转），避免双份样式。
- 客户端首屏对新端点的失败"silent skip"（活动加载失败不阻塞套餐展示，反过来也是）。

**Non-Goals:**

- 不改 admin 充值赠送活动管理 UI / API。
- 不改 `PaymentView` 红点、breakdown 行、fulfillment / refund 计算。
- 不在首页显示红点（用户明确不要）。
- 不在 `HomeView` 上做 SSR / 预渲染（保持现有 SPA 行为）。
- 不在首页直接列出活动的"立即充值"CTA 与套餐 CTA 的差异提示——活动 banner 自身的 CTA 走 `/login?redirect=/purchase` 或 `/purchase`，套餐卡片继续用现有 `Buy now` 行为。
- 不增加任何 server-side 缓存层；端点现读 `RechargePromoActivityService.GetCurrent`，请求量靠 Redis 边界限流兜底（与 `/plaza/plans` 一致）。

## Decisions

### D1. 公开端点路径：`/api/v1/plaza/recharge-promo`，不是 `/api/public/recharge-promo`

**选择**：挂在 `RegisterPlazaRoutes` 下面，和 `models` / `plans` 同组、同限流、同 prefix。

**理由**：
- 语义一致：`/plaza/*` 已经是"给匿名访客的展示读模型"，新端点天然属于这个集合。
- 复用 Redis 限流 middleware，无需重新设计 fail-open 策略。
- 与 `pricing-plaza` capability 的归口一致——后续如果再加"匿名公告"等公开展示，也走同一组。

**不选 `/api/public/recharge-promo`**：会引入第二个公开命名空间，且不享 `plaza` 组的限流/fail-open 配置；权益不够。

**不选注入到 `cachedPublicSettings`**：活动数据是结构化的（tiers 数组 + 时间窗 + 文案），塞进 settings 会让 settings DTO 膨胀，并且活动变更需等下次注入刷新；新端点更干净。

### D2. DTO 形态：和 `checkout-info.recharge_promo` 字段一致 + 多带 `name`

```jsonc
// GET /api/v1/plaza/recharge-promo
{
  "promo": {
    "name":        "618 充值返现",            // ← 新增（同时也会回填到 checkout-info）
    "valid_from":  "2026-06-01T00:00:00Z",  // 可空
    "valid_until": "2026-06-18T23:59:59Z",  // 可空
    "tiers": [
      {"min_amount": 100,  "bonus_rate": 0.03},
      {"min_amount": 500,  "bonus_rate": 0.05},
      {"min_amount": 1000, "bonus_rate": 0.08}
    ],
    "version": "v17b9c3e2"
  }
}
```

无生效活动时返回 `{ "promo": null }`（不是 200 + omit 字段；前端判定更稳）。

**关键差异 vs checkout-info**：
- 不返回 `enabled` 字段——上线即"有"，没上线即 `null`，前端对"未启用 / 过期 / 时间窗外"统一视作 null。
- 不返回 `activity_id`——那是后端审计字段，对匿名前端无意义。
- 增加 `name`——展示标题。`checkout-info` 也会顺带补这个字段（向后兼容：旧前端忽略多余 key）。

### D3. `RechargePromo` 结构体加 `Name`，schema 不动

后端：
- `internal/service/payment_config_recharge_promo.go` 的 `RechargePromo` 结构体加 `Name string \`json:"name,omitempty"\``。
- `ActivityToPromo(row)` 把 `row.Name` 写入 `Name`。
- `serializeRechargePromoSetting` / 历史快照等转换路径同步透传（如果还在用，目前 spec 已经迁移到活动表，旧路径基本死代码——确认一下不写就不写）。
- `checkout-info` 输出顺带带上 `name`（一处 marshal 改）。

ent schema 不动：`name` 字段已经是 `MaxLen(120).NotEmpty().Default("默认活动")`，不需要 migration。

### D4. 前端组件结构

```
HomeView.vue
├─ HeroSection                    （保留）
├─ FeatureTags                    （保留）
├─ FeaturesGrid                   （保留）
├─ HomeShowcaseSection.vue        ← 新增；替代原 ProvidersSection
│   ├─ HomePromoBanner.vue        ← 仅 promo 存在时渲染
│   │   └─ name + 阶梯列表 + valid_until 倒计时（可选）+ 主 CTA
│   └─ PlanPlazaCards
│       :max-items="3"
│       + "查看全部套餐 →"  router-link to="/plaza/plans"
└─ Footer                         （保留）
```

**为什么不让 `PlanPlazaCards` 自己内置 `maxItems` 限流**：
- `PlanPlazaCards` 当前组件还在 plaza 全量场景（`PlazaPlansView`）使用，限流是"首页特化"。
- 折中：要么在 `PlanPlazaCards` 上加可选 prop `:max-items`（不传 = 全量，传了 = slice），要么在 `HomeShowcaseSection` 里取数据后自行 slice 再传给一个无状态版本。
- 选 prop 方案——`PlanPlazaCards` 已经做了内部数据加载，让它直接吐前 N 张更省一次 round-trip。`viewAllHref` 也加成 prop，不传则不渲染"查看全部"链接，保持 `PlazaPlansView` 不变。

### D5. 活动数据加载时机与失败语义

- `HomeShowcaseSection` 在 `onMounted` 调 `paymentApi.getPublicRechargePromo()`；
- 加载中：不渲染 banner（套餐卡正常加载，互不阻塞）；
- 失败 / 网络错误：silent skip，console.warn 即可，不显示错误 toast（首页是 marketing 页面，不能因为 promo 端点抖动就显示错误）；
- 成功 + `promo === null`：不渲染 banner；
- 成功 + `promo` 存在：渲染 banner。

套餐数据走 `PlanPlazaCards` 内部已有的 `/api/v1/plaza/plans` 加载逻辑，不动。

### D6. 活动 banner 的 CTA 行为

- 主按钮 "立即充值" / "Recharge now"（i18n key `home.promo.cta_recharge`）。
- 已登录：`router.push('/purchase')`。
- 匿名：`router.push('/login?redirect=/purchase')`。
- 复用 `useAuthRedirect.gotoOrLogin('/purchase')`（已有 helper）。

### D7. i18n 键命名

新增：
- `home.showcase.title` / `home.showcase.subtitle`（节标题，如"现在可用 / Get Started Now"）
- `home.promo.cta_recharge` — banner 主按钮
- `home.promo.tier_label`（"满 {min} 立返 {rate}%"）— 阶梯单行模板
- `home.promo.expires_at`（"活动至 {date}"）
- `home.plans.view_all`（"查看全部套餐 →"）

删除（如果别处不再使用）：
- `home.providers.*` 整组（按现有键扫一下确认没人引用再删）

### D8. 测试策略

后端：
- `recharge_promo_public_handler_test.go`：覆盖 `有活动 / 无活动 / 过期 / disabled / 限流命中` 五种场景；
- 验证 `name` 字段被正确序列化、`enabled / activity_id` 字段不出现在响应里；
- `checkout-info` 的金牌测试加一条 assert：响应里包含 `recharge_promo.name`（向后兼容验证）。

前端：
- `HomeShowcaseSection.spec.ts`：mock `getPublicRechargePromo`，三态（loading / promo-present / promo-null）渲染分支；
- `HomePromoBanner.spec.ts`：name 渲染、tier 列表、CTA 跳转登录态分流；
- `PlanPlazaCards.spec.ts`：扩展现有 spec，新增 `:max-items="3"` + `viewAllHref` 时只渲染前 3 张 + "查看全部" 链接；
- `HomeView.spec.ts`（如果有）更新——assert Providers 段已被移除。

## Risks / Trade-offs

- **[首页 N+1 请求]** → 首屏现在多调用一次 `/plaza/recharge-promo`。Mitigation：和 `/plaza/plans` 并行发起、有 Redis 限流兜底；活动响应体小（< 1 KB），开销可忽略。
- **[活动 banner 文案被运营写崩 (XSS)]** → `name` 字段是用户可控的字符串，模板渲染必须走 `{{ name }}` 而非 `v-html`。Mitigation：组件里禁用 `v-html`，单测里加一条 "name 含 `<script>` 时不被解析" 的断言。
- **[过期活动残留]** → `RechargePromoActivityService.GetCurrent` 已包含时间窗过滤，但仍要确认 `IsActiveAt(time.Now())` 在 handler 层兜底（避免 service 层语义飘移）。Mitigation：handler 拿到 row 后再调一次 `promo.IsActiveAt(now)`，否则返回 `null`。
- **[`payment_enabled = false` 时仍展示 banner]** → 不合理（充值入口禁用了，活动还在喊"立即充值"）。Mitigation：`HomeShowcaseSection` 顶层守卫 `appStore.cachedPublicSettings.payment_enabled === false` 时整段不渲染（连 banner 一起隐藏）。
- **[新端点暴露 `name` 引发信息泄露]** → 风险低；`name` 是运营文案，本来就是要给用户看的。Mitigation：admin 写 name 时已有 `MaxLen(120) + NotEmpty` 约束，无需额外。
- **[i18n 旧键删除影响其他页面]** → 删 `home.providers.*` 前 grep 一遍仓库，确认无引用。Mitigation：tasks 里加一步 "verify no usage" 后再删。

## Migration Plan

1. **后端先行**（无破坏性）：
   - 加 `Name` 字段 + handler + 路由 + 测试；
   - `checkout-info` 顺带返回 `name`（向后兼容，旧前端忽略）；
   - 部署后老前端继续工作，新端点开始返回数据。
2. **前端跟进**：
   - 添加 `HomeShowcaseSection` + `HomePromoBanner` + `PlanPlazaCards` prop 扩展；
   - 删除 `HomeView` 里的 Providers 段 & i18n 旧键；
   - 部署后首页换面。
3. **回滚**：
   - 后端：路由可单独 disable（注释掉 `RegisterPlazaRoutes` 里那一行），不影响其他公开端点；
   - 前端：保留 git tag，必要时 revert `HomeView` 一个 commit 即可恢复 Providers 段（不过没必要，新版只多不少）。

## Open Questions

- 活动 banner 的"倒计时"要不要做？（"距活动结束 2 天 13 小时"）
  - 倾向：做"剩余 X 天"的弱提示，不要做实时秒级倒计时（避免前端定时器 + 页面性能负担）；只在 `valid_until` 存在时显示。如果运营不给 `valid_until`（"长期活动"），就只显示 `name + 阶梯`。
- 套餐 "查看全部" 是不是要带"+N"角标提示还有几张？
  - 倾向：不做，超过 3 张时纯文案 "查看全部套餐 →" 就够；要装饰性数字感觉过度设计。如果产品强烈要，再开 follow-up。
- `valid_from` 在未来时该怎么处理？现在 `IsActiveAt` 直接返回 `false`，匿名访客看不到"即将开始"。要不要做一个"即将开始"的灰色 banner？
  - 倾向：v1 不做。"未生效"和"已结束"对外都是 `promo: null`。
