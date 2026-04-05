# 支付系统实施路线图

> 基于前端计划 + 后端计划的完整实施路线图
>
> 分支: `feature/payment` @ `touwaeriol/sub2api`

---

## Phase 概览

```
Phase 1: 数据库 & Schema        ← 无依赖，最先开始
Phase 2: Go 后端核心             ← 依赖 Phase 1
Phase 3: Go 后端 API            ← 依赖 Phase 2
Phase 4: 前端基础设施            ← 可与 Phase 2 并行
Phase 5: 前端用户页面            ← 依赖 Phase 3 + 4
Phase 6: 前端管理页面            ← 依赖 Phase 3 + 4
```

---

## Phase 1: 数据库 & Schema

### 任务列表

| # | 任务 | 文件 | 说明 |
|---|------|------|------|
| 1.1 | PaymentOrder schema | ent/schema/payment_order.go | 订单表，12种状态 |
| 1.2 | PaymentAuditLog schema | ent/schema/payment_audit_log.go | 审计日志 |
| 1.3 | PaymentChannel schema | ent/schema/payment_channel.go | 展示渠道 |
| 1.4 | SubscriptionPlan schema | ent/schema/subscription_plan.go | 订阅套餐 |
| 1.5 | PaymentProviderInstance schema | ent/schema/payment_provider_instance.go | 服务商实例 |
| 1.6 | SQL migration: orders | migrations/090_payment_orders.sql | |
| 1.7 | SQL migration: audit_logs | migrations/091_payment_audit_logs.sql | |
| 1.8 | SQL migration: channels | migrations/092_payment_channels.sql | |
| 1.9 | SQL migration: plans | migrations/093_subscription_plans.sql | |
| 1.10 | SQL migration: instances | migrations/094_payment_provider_instances.sql | |
| 1.11 | Ent generate | `go generate ./ent` | 生成 CRUD 代码 |

**验收**: `go build ./...` 通过，Ent 生成代码无报错

---

## Phase 2: Go 后端核心

### 2A: 支付基础设施（可并行）

| # | 任务 | 文件 | 说明 |
|---|------|------|------|
| 2.1 | 类型 & 接口定义 | internal/payment/types.go | Provider 接口, 请求/响应类型 |
| 2.2 | Provider 注册表 | internal/payment/registry.go | 动态注册/查找 |
| 2.3 | 手续费计算 | internal/payment/fee.go | shopspring/decimal, ROUND_UP |
| 2.4 | AES 加解密 | internal/payment/crypto.go | 实例配置加密 |
| 2.5 | 负载均衡 | internal/payment/load_balancer.go | round-robin / least-amount |
| 2.6 | Provider 工厂 | internal/payment/provider_factory.go | 根据实例配置创建 Provider |

### 2B: Provider 实现（可并行）

| # | 任务 | 文件 | SDK |
|---|------|------|-----|
| 2.7 | EasyPay | internal/payment/provider/easypay.go | net/http + crypto/md5 |
| 2.8 | Alipay | internal/payment/provider/alipay.go | smartwalle/alipay/v3 |
| 2.9 | Wxpay | internal/payment/provider/wxpay.go | wechatpay-apiv3/wechatpay-go |
| 2.10 | Stripe | internal/payment/provider/stripe.go | stripe/stripe-go/v82 |

### 2C: Service 层（依赖 2A + 2B）

| # | 任务 | 文件 | 说明 |
|---|------|------|------|
| 2.11 | 配置服务 | internal/service/payment_config_service.go | 系统配置+实例+渠道+套餐 CRUD |
| 2.12 | 订单服务-创建 | internal/service/payment_service.go | 创建订单、手续费、限额校验 |
| 2.13 | 订单服务-支付确认 | 同上 | Webhook 处理、状态流转 |
| 2.14 | 订单服务-履约 | 同上 | 余额充值(RedeemService)、订阅激活 |
| 2.15 | 订单服务-退款 | 同上 | prepare/execute/rollback 三阶段 |
| 2.16 | 订单服务-超时 | 同上 | 定时过期 + 关单 |
| 2.17 | 订单服务-统计 | 同上 | Dashboard 聚合查询 |
| 2.18 | Wire DI | internal/payment/wire.go | Provider Set |

**验收**: 单元测试通过，`go build ./...` 通过

### 新增 Go 依赖

```bash
go get github.com/stripe/stripe-go/v82
go get github.com/smartwalle/alipay/v3
go get github.com/wechatpay-apiv3/wechatpay-go
go get github.com/shopspring/decimal
```

---

## Phase 3: Go 后端 API

### 3A: Handler 层

| # | 任务 | 文件 | 端点数 |
|---|------|------|--------|
| 3.1 | 用户 Handler | internal/handler/payment_handler.go | 9 |
| 3.2 | Webhook Handler | internal/handler/payment_webhook_handler.go | 4 |
| 3.3 | 管理 Handler | internal/handler/admin/payment_handler.go | 18 |

### 3B: 路由 & 集成

| # | 任务 | 文件 | 说明 |
|---|------|------|------|
| 3.4 | 路由注册 | internal/server/routes/payment.go | 新路由文件 |
| 3.5 | Router 集成 | internal/server/router.go | registerRoutes 中调用 |
| 3.6 | Handlers 结构体 | internal/handler/handlers.go | 添加 Payment 字段 |
| 3.7 | Wire 更新 | cmd/server/wire.go | 注入支付依赖 |
| 3.8 | Settings API 扩展 | internal/handler/admin/setting_handler.go | payment_enabled 等配置 |

**验收**: API 可调通（curl/Postman），Webhook 端点可接收请求

---

## Phase 4: 前端基础设施

可与 Phase 2 并行开始。

| # | 任务 | 文件 | 说明 |
|---|------|------|------|
| 4.1 | TypeScript 类型 | frontend/src/types/payment.ts | Order, Plan, Channel 等类型 |
| 4.2 | 用户 API 模块 | frontend/src/api/payment.ts | 9 个 API 方法 |
| 4.3 | 管理 API 模块 | frontend/src/api/admin/payment.ts | 18 个 API 方法 |
| 4.4 | API 导出 | frontend/src/api/index.ts | 导出新模块 |
| 4.5 | Payment Store | frontend/src/stores/payment.ts | 配置/订单/套餐状态 |
| 4.6 | App Store 修改 | frontend/src/stores/app.ts | 新增 payment_enabled |
| 4.7 | Admin Store 修改 | frontend/src/stores/adminSettings.ts | 新增 paymentEnabled |
| 4.8 | Router 新增路由 | frontend/src/router/index.ts | 7 个新路由 + meta |
| 4.9 | 路由守卫 | frontend/src/router/index.ts | requiresPayment 检查 |
| 4.10 | Sidebar 修改 | frontend/src/components/layout/AppSidebar.vue | 新增菜单项 |
| 4.11 | i18n 英文 | frontend/src/i18n/locales/en.ts | payment 命名空间 |
| 4.12 | i18n 中文 | frontend/src/i18n/locales/zh.ts | payment 命名空间 |

**新增前端依赖**:
```bash
cd frontend && pnpm add @stripe/stripe-js
```

**验收**: `pnpm typecheck` 通过，菜单可见性正确

---

## Phase 5: 前端用户页面

依赖 Phase 3 (API) + Phase 4 (基础设施)。

| # | 任务 | 文件 | 说明 |
|---|------|------|------|
| 5.1 | 支付方式选择器 | components/payment/PaymentMethodSelector.vue | 共享组件 |
| 5.2 | 金额输入 | components/payment/AmountInput.vue | 快捷按钮+自定义 |
| 5.3 | 手续费展示 | components/payment/FeeDisplay.vue | 实时计算 |
| 5.4 | 套餐卡片 | components/payment/SubscriptionPlanCard.vue | |
| 5.5 | 套餐确认弹窗 | components/payment/SubscriptionConfirmModal.vue | |
| 5.6 | **充值/订阅主页** | views/user/PaymentView.vue | 替换 iframe，核心页面 |
| 5.7 | QR码展示 | components/payment/QRCodeDisplay.vue | qrcode 库 |
| 5.8 | 倒计时 | components/payment/CountdownTimer.vue | |
| 5.9 | **二维码支付页** | views/user/PaymentQRCodeView.vue | 轮询+倒计时 |
| 5.10 | **Stripe 支付页** | views/user/StripePaymentView.vue | Payment Element |
| 5.11 | **支付结果页** | views/user/PaymentResultView.vue | 成功/失败展示 |
| 5.12 | 订单状态标签 | components/payment/OrderStatusBadge.vue | 彩色标签 |
| 5.13 | 订单筛选栏 | components/payment/OrderFilterBar.vue | |
| 5.14 | **我的订单页** | views/user/UserOrdersView.vue | 列表+筛选+操作 |
| 5.15 | 删除旧文件 | PurchaseSubscriptionView.vue, embedded-url.ts | |

**验收**: 用户可完成充值流程（创建订单→扫码/Stripe→支付成功→余额增加）

---

## Phase 6: 前端管理页面

可与 Phase 5 并行。

| # | 任务 | 文件 | 说明 |
|---|------|------|------|
| 6.1 | 统计卡片 | components/admin/payment/OrderStatsCards.vue | |
| 6.2 | 日收入图表 | components/admin/payment/DailyRevenueChart.vue | Chart.js |
| 6.3 | 支付分布图 | components/admin/payment/PaymentMethodChart.vue | |
| 6.4 | 排行榜 | components/admin/payment/TopUsersLeaderboard.vue | |
| 6.5 | **订单管理页(tab容器)** | views/admin/orders/AdminOrdersView.vue | 4个tab |
| 6.6 | 管理订单表格 | components/admin/payment/AdminOrderTable.vue | DataTable |
| 6.7 | 订单详情 | components/admin/payment/AdminOrderDetail.vue | |
| 6.8 | 退款对话框 | components/admin/payment/AdminRefundDialog.vue | |
| 6.9 | 订单搜索栏 | components/admin/payment/AdminOrderSearchBar.vue | |
| 6.10 | **订单列表tab** | 融入 AdminOrdersView | |
| 6.11 | **渠道管理tab** | views/admin/orders/AdminOrderChannelsView.vue | |
| 6.12 | **订阅套餐tab** | views/admin/orders/AdminOrderPlansView.vue | |
| 6.13 | 服务商实例表单 | components/admin/payment/ProviderInstanceForm.vue | |
| 6.14 | 服务商实例卡片 | components/admin/payment/ProviderInstanceCard.vue | |
| 6.15 | **服务商管理页** | views/admin/orders/AdminProvidersView.vue | |
| 6.16 | **Settings payment tab** | 修改 SettingsView.vue | 新增tab+移除旧配置 |
| 6.17 | 旧版兼容迁移 | Go migration code | purchase_url→自定义菜单 |

**验收**: 管理员可完成全流程（配置支付→管理订单→处理退款）

---

## 依赖关系图

```
Phase 1 (DB)
    │
    ▼
Phase 2 (Go Core) ──────┐
    │                    │
    ▼                    │
Phase 3 (Go API) ◄──────┤
    │                    │
    ▼                    ▼
Phase 5 (User UI) ◄── Phase 4 (FE Infra)
                         │
Phase 6 (Admin UI) ◄────┘

可并行:
- Phase 2A + 2B (基础设施 + Provider 实现)
- Phase 4 可与 Phase 2 同时开始
- Phase 5 + Phase 6 可并行
```

---

## 测试策略

### 后端单元测试

| 测试文件 | 覆盖 |
|---------|------|
| internal/payment/fee_test.go | 手续费计算精度 |
| internal/payment/registry_test.go | 注册/查找 |
| internal/payment/load_balancer_test.go | 实例选择策略 |
| internal/payment/crypto_test.go | 加解密正确性 |
| internal/payment/provider/easypay_test.go | 签名验证 |
| internal/service/payment_service_test.go | 订单状态流转 |
| internal/service/payment_config_service_test.go | 配置 CRUD |
| internal/handler/payment_handler_test.go | API 请求/响应 |

### 后端集成测试

| 测试 | 覆盖 |
|------|------|
| 订单创建→支付→履约 | 完整流程 |
| 退款三阶段 | prepare→execute→rollback |
| Webhook 验签 | 各 Provider 签名验证 |

### 前端组件测试

| 测试 | 覆盖 |
|------|------|
| PaymentMethodSelector | 选择/禁用/限额展示 |
| AmountInput | 金额验证/精度 |
| OrderStatusBadge | 状态→颜色映射 |
| PaymentView | 创建订单流程 |

---

## 迁移兼容

### 滚动部署 3 阶段

1. **部署后端** — 新 API 端点上线，但 `payment_enabled=false`，不影响现有功能
2. **部署前端** — 新页面/组件上线，但菜单隐藏（因为 `payment_enabled=false`）
3. **管理员开启** — 在设置中启用 `payment_enabled`，配置支付服务商，功能生效

### purchase_subscription_url 兼容

Go 迁移代码在启动时检查：
```go
if url := settingService.Get("purchase_subscription_url"); url != "" {
    // 自动创建 custom_menu_item 保留旧 URL
    // 标记 purchase_subscription_url 为 deprecated
}
```

### 数据库迁移顺序

迁移 090-094 仅创建新表，不修改现有表，零风险。
