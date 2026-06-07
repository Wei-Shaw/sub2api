## 1. 后端：领域模型与 service 透传

- [x] 1.1 在 `internal/service/payment_config_recharge_promo.go` 的 `RechargePromo` 结构体加 `Name string \`json:"name,omitempty"\`` 字段，附注释说明用途
- [x] 1.2 修改 `ActivityToPromo(row)` 使其填充 `Name: row.Name`
- [x] 1.3 grep `serializeRechargePromoSetting` / `RechargePromo{` 字面构造点，确认所有路径要么不影响响应、要么也透传 `Name`（旧 system_settings 路径已废弃可忽略，但要确认 marshal 不丢字段）
- [x] 1.4 跑 `go build ./...` 与 `go vet ./...`，确保结构体扩展不破坏既有调用

## 2. 后端：公开端点

- [x] 2.1 在 `internal/handler/` 下新增 `plaza_recharge_promo_handler.go`（或挂到现有 `plaza_handler.go`），实现 `GetPublicRechargePromo(c *gin.Context)`
- [x] 2.2 处理函数：调用 `RechargePromoActivityService.GetCurrent(ctx)` → `ActivityToPromo` → `IsActiveAt(time.Now())` 兜底；任何"无活动 / 过期 / tiers 空 / err"情况返回 `{ "promo": null }`，HTTP 200
- [x] 2.3 定义响应 DTO（`PublicRechargePromo`）：仅包含 `name / valid_from / valid_until / tiers / version`，**不**包含 `enabled / activity_id`
- [x] 2.4 在 `internal/server/routes/plaza.go` 的 `RegisterPlazaRoutes` 中注册 `plaza.GET("/recharge-promo", rateLimiter.LimitWithOptions("plaza-recharge-promo", 60, time.Minute, fail-open), h.Plaza.GetPublicRechargePromo)`
- [x] 2.5 如果 handler 集合 `Handlers.Plaza` 还没暴露这个方法，按现有 `ListModels / ListPlans` 风格补一个方法绑定
- [x] 2.6 单测 `plaza_recharge_promo_handler_test.go`：覆盖 (a) 有活动→返回 promo (b) 无 enabled 行→null (c) 时间窗外→null (d) tiers 空→null (e) 任意 Authorization header 被忽略 (f) 响应 JSON **不包含** `enabled` / `activity_id` 字段
- [x] 2.7 调整 `checkout-info` DTO/序列化路径，让响应里也包含 `name`；扩展或新增金牌测试：assert `recharge_promo.name == 活动表 name`

## 3. 后端：版本一致性测试

- [x] 3.1 写一条集成式断言：同一 admin-config 状态下 `checkout-info.recharge_promo.version` 与 `/api/v1/plaza/recharge-promo` 的 `version` 严格相等
- [x] 3.2 写一条断言：admin 仅修改 `name` 字段保存后，`version` 必须随之变化（确认 hash 输入包含 `name`，否则补进 hash 输入）
- [x] 3.3 跑 `go test ./internal/handler/... ./internal/service/...`

## 4. 前端：API 客户端

- [x] 4.1 在 `frontend/src/api/payment.ts`（或合适的 plaza 客户端）新增 `getPublicRechargePromo(): Promise<{ promo: PublicRechargePromo | null }>`
- [x] 4.2 定义 `PublicRechargePromo` TS 类型，与后端 DTO 一一对应；导出供组件复用
- [x] 4.3 客户端不挂任何鉴权 header（与 `/plaza/plans` 一致）

## 5. 前端：组件实现

- [x] 5.1 新建 `frontend/src/components/home/HomePromoBanner.vue`：props `{ promo: PublicRechargePromo }`；渲染 `name`、tier 列表（i18n `home.promo.tier_label`）、`valid_until` 存在时显示 "活动至 {date}"；含 "Recharge now" 按钮，调用 `useAuthRedirect.gotoOrLogin('/purchase')`；**禁用 `v-html`**
- [x] 5.2 在 `PlanPlazaCards.vue` 增加可选 prop `maxItems?: number`（不传 = 全量）和 `viewAllHref?: string`（传则在卡片下方渲染 "查看全部套餐 →" 链接）；保持 `PlazaPlansView.vue` 不传新 prop 的现状不变
- [x] 5.3 新建 `frontend/src/components/home/HomeShowcaseSection.vue`：
  - 顶层守卫：`appStore.cachedPublicSettings.payment_enabled === false` 时返回空
  - `onMounted` 调 `getPublicRechargePromo()`；失败 silent skip（仅 console.warn）
  - 渲染：`promo` 存在时先渲染 `HomePromoBanner`；恒渲染 `<PlanPlazaCards :max-items="3" :view-all-href="'/plaza/plans'" />`
  - 不调用 `useRechargePromoDot`，不渲染任何红点
- [x] 5.4 修改 `frontend/src/views/HomeView.vue`：
  - 删除 Supported Providers 整段（template + 相关 i18n 引用 + 相关 ScopedSlot/CSS）
  - 删除 hero 区域的 "View model pricing" 二级 CTA（保留主 "Get Started"）
  - 删除（如有的）`PricingTeaser` 组件引用
  - 在 Features Grid 之后挂 `<HomeShowcaseSection />`

## 6. 前端：i18n

- [x] 6.1 `frontend/src/locales/zh-CN.ts` 新增：`home.showcase.title` / `home.showcase.subtitle`、`home.promo.cta_recharge`（"立即充值"）、`home.promo.tier_label`（"满 ¥{min} 加赠 {rate}%"）、`home.promo.expires_at`（"活动至 {date}"）、`home.plans.view_all`（"查看全部套餐 →"）
- [x] 6.2 `frontend/src/locales/en-US.ts` 同步加英文版本
- [x] 6.3 grep 仓库中 `home.providers.` / `home.cta_view_pricing` / `home.pricing_teaser` 使用情况，确认仅 `HomeView` 引用后从两份 locale 文件中删除对应键

## 7. 前端：测试

- [x] 7.1 `HomeShowcaseSection.spec.ts`：mock `getPublicRechargePromo`，覆盖 (a) `payment_enabled=false` 整段不渲染 (b) `promo=null` 只渲染 plan cards + "View all" (c) `promo` 存在时 banner+cards 都渲染 (d) API 失败时只渲染 plan cards 且无 toast
- [x] 7.2 `HomePromoBanner.spec.ts`：覆盖 name 渲染、tier 列表、`valid_until` 存在/不存在时 expires_at 行的显示、CTA 在登录态/匿名态的跳转目标、`name` 含 `<script>` 时被作为文本渲染（XSS 防护）
- [x] 7.3 扩展 `PlanPlazaCards.spec.ts`：`maxItems=3` 时只渲染前 3 张；传 `viewAllHref` 时渲染 "查看全部" 链接；不传时维持现有行为
- [x] 7.4 如果存在 `HomeView.spec.ts`，断言 Providers 段已不在 DOM；否则新增最小化 smoke test
- [x] 7.5 跑 `pnpm test` / `npm test` 确认全绿（本任务相关 31 个用例 100% 通过；仓库其余 14 个既有失败与 chart 渲染、注册流程相关，与本变更无关）

## 8. 联调与文档

- [ ] 8.1 本地启动后端 + 前端，验证三种状态：(a) 无活动 (b) 有活动且未过期 (c) 活动 `valid_until` 已过 → 全部表现符合 spec
- [ ] 8.2 验证 `payment_enabled=false` 时整段消失（在 admin 切换设置后刷新首页）
- [ ] 8.3 验证 admin 改 `name` 后首页 banner 标题更新（清浏览器缓存）
- [ ] 8.4 验证匿名访客点击 "Recharge now" → `/login?redirect=/purchase`，登录后落到 `/purchase`
- [ ] 8.5 跑 `openspec validate homepage-promo-showcase --strict` 确认 spec 通过
- [ ] 8.6 准备 PR 描述：链接 proposal、列出删除的 Providers 段截图前后对比、说明端点限流策略

## 9. 归档

- [ ] 9.1 PR 合并、上线后，按 `openspec-archive-change` 流程归档 `homepage-promo-showcase`，把两个 spec delta 落入主 spec
