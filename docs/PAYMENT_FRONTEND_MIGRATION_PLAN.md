# 支付系统前端迁移计划

> 将 sub2apipay (Next.js/React) 的支付功能原生集成到 sub2api (Vue3) 前端中
>
> 分支: `feature/payment` (基于 upstream `Wei-Shaw/sub2api:main`)

---

## 一、迁移概述

### 1.1 现状

当前 sub2api 通过 **iframe 嵌入外部 URL** 的方式提供充值/订阅功能：

- 管理员在 `系统设置 → General tab` 中配置 `purchase_subscription_url`
- 用户侧边栏出现"充值/订阅"菜单项（受 `purchase_subscription_enabled` 控制）
- 点击后在 `PurchaseSubscriptionView.vue` 中以 iframe 加载外部页面
- 外部页面（sub2apipay）通过 URL 参数接收 `user_id`, `token`, `theme`, `lang`

**现有关键文件：**

| 文件 | 作用 |
|------|------|
| `src/components/layout/AppSidebar.vue` | 侧边栏菜单定义（~700行） |
| `src/views/user/PurchaseSubscriptionView.vue` | iframe 嵌入页（~159行��� |
| `src/views/admin/SettingsView.vue` | 管理后台设置页（~2500行） |
| `src/stores/app.ts` | 公共设置存储（含 `purchase_subscription_enabled`） |
| `src/stores/adminSettings.ts` | 管理设置存储 |
| `src/router/index.ts` | 路由定义 |
| `src/i18n/locales/en.ts` / `zh.ts` | 国际化文本 |
| `src/utils/embedded-url.ts` | iframe URL 构建工具 |

### 1.2 目标

将 sub2apipay 的所有前端功能原生实现为 Vue3 组件，集成到 sub2api 项目中：

- **消除 iframe** — 所有支付、订单页面变为原生 Vue 组件
- **消除外部依赖** — 不再需要独立部署 sub2apipay
- **直接调用后端 API** — 替代 HTTP 跨域调用
- **统一认证** — 复用 sub2api 的 JWT 认证，无需 URL 传 token
- **统一 UI** — 使用 sub2api 现有的 TailwindCSS 设计系统和组件库

---

## 二、菜单结构变更

### 2.1 管理员侧边栏 (`adminNavItems`)

**当前菜单（upstream main）：**

```
仪表盘          /admin/dashboard
运维监控        /admin/ops           (ops_monitoring_enabled 控制)
用户管理        /admin/users
分组管理        /admin/groups
渠道管理        /admin/channels
订阅管理        /admin/subscriptions
账号管理        /admin/accounts
公告管理        /admin/announcements
代理管理        /admin/proxies
充值码管理      /admin/redeem
推广码管理      /admin/promo-codes
用量统计        /admin/usage
系统设置        /admin/settings
[自定义菜单]
```

**迁移后菜单：**

```
仪表盘          /admin/dashboard
运维监控        /admin/ops           (ops_monitoring_enabled 控制)
用户管理        /admin/users
分组管理        /admin/groups
渠道管理        /admin/channels
订阅管理        /admin/subscriptions
账号管理        /admin/accounts
公告管理        /admin/announcements
代理管理        /admin/proxies
充值码管理      /admin/redeem
推广码管理      /admin/promo-codes
+ 订单管理      /admin/orders         ← 新增（payment_enabled 控制可见性）
用量统计        /admin/usage
系统设置        /admin/settings
[自定义菜单]
```

**新增"订单管理"菜单及子菜单：**

"订单管理"作为一级菜单项，点击后页面内部提供 tab 导航：

| 子菜单 | 路由 | 说明 |
|--------|------|------|
| 订单概览 | `/admin/orders` | 订单统计仪表盘（日收入、支付方式分布、排行榜） |
| 订单列表 | `/admin/orders/list` | 订单搜索/筛选/详情/退款操作 |
| 渠道管理 | `/admin/orders/channels` | 支付渠道定义（展示名称、倍率、描述） |
| 订阅套餐 | `/admin/orders/plans` | 订阅计划管理（价格、有效期、分组绑定） |

> **可见性规则**: 仅当系统设置中"充值/订阅功能"开启时，"订单管理"菜单才可见。

### 2.2 用户侧边栏 (`userNavItems`)

**当前菜单：**

```
仪表盘          /dashboard
API密钥         /keys
用量统计        /usage
我的订阅        /subscriptions
[Sora]          /sora                (sora_client_enabled 控制)
[充值/订阅]     /purchase            (purchase_subscription_enabled 控制, iframe)
兑换码          /redeem
个人设置        /profile
[自定义菜单]
```

**迁移后菜单：**

```
仪表盘          /dashboard
API密钥         /keys
用量统计        /usage
我的订阅        /subscriptions
[Sora]          /sora                (sora_client_enabled 控制)
- [充值/订阅]   /purchase            ← 替换: 不再是 iframe
+ 充值/订阅     /purchase            ← 原生 Vue 页面（payment_enabled 控制）
+ 我的订单      /orders              ← 新增（payment_enabled 控制）
兑换码          /redeem
个人设置        /profile
[自定义菜单]
```

**变更要点：**

1. `/purchase` 路由保持不变，但组件从 iframe 替换为原生 `PaymentView.vue`
2. 新增 `/orders` 路由 — "我的订单"页面
3. 菜单可见性条件从 `purchase_subscription_enabled` 变更为 `payment_enabled`
4. 移除 `purchase_subscription_url` 设置项

### 2.3 管理员"我的账户"区域 (`personalNavItems`)

与 `userNavItems` 保持一致的变更：增加"我的订单"，替换"充值/订阅"为原生页面。

---

## 三、系统设置变更

### 3.1 Settings Tab 变更

**当前 tabs：**

```
general | security | users | gateway | email | backup | data
```

**迁移后 tabs：**

```
general | security | users | gateway | payment | email | backup | data
```

新增 **`payment`** tab，在 `gateway` 之后、`email` 之前。

### 3.2 `payment` Tab 内容

此 tab 替代原 `general` tab 中的"充值/订阅页面"配置区块。

**配置项设计：**

```
┌─────────────────────────────────────────────────────────┐
│  充值/订阅功能                                            │
│  ─────────────────────────────────────                   │
│  启用充值/订阅        [Toggle]                            │
│  说明: 开启后用户可通过在线支付充值余额或购买订阅。           │
│        同时"订单管理"菜单将对管理员可见。                    │
│                                                          │
│  (以下配置仅在功能启用后显示)                               │
│                                                          │
│  ┌─ 基本设置 ──────────────────────────────────────────┐ │
│  │ 最低充值金额      [___] 元                           │ │
│  │ 最高充值金额      [___] 元                           │ │
│  │ 每日充值限额      [___] 元                           │ │
│  │ 最大待支付订单数  [___]                               │ │
│  │ 订单超时时间      [___] 分钟                          │ │
│  │ 禁用余额支付      [Toggle]                           │ │
│  └──────────────────────────────────────────────────────┘ │
│                                                          │
│  ┌─ 支付服务商 ────────────────────────────────────────┐ │
│  │ 启用的支付方式    [☑ 支付宝] [☑ 微信支付] [☑ Stripe] │ │
│  │                                                      │ │
│  │ 服务商实例管理    [管理实例 →]                         │ │
│  │ (跳转到服务商实例配置子页面)                           │ │
│  └──────────────────────────────────────────────────────┘ │
│                                                          │
│  ┌─ 显示设置 ──────────────────────────────────────────┐ │
│  │ 帮助图片 URL     [___]                               │ │
│  │ 帮助文字         [___]                               │ │
│  └──────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────┘
```

### 3.3 从 `general` Tab 移除的内容

原 `general` tab 中的以下配置区块将移除：

- **"Recharge / Subscription Page"** 整个 card

### 3.4 旧版兼容：自定义页面迁移

对于已使用 `purchase_subscription_url` 配置外部支付页面的用户：

- 数据库迁移脚本检查 `purchase_subscription_url` 是否有值
- 若有值，自动创建一个"自定义菜单页面" (custom_menu_item)，保留原 URL
- 确保旧版用户升级后，原来的外部支付页面仍可通过自定义菜单访问
- `purchase_subscription_enabled` 和 `purchase_subscription_url` 设置项保留但标记 deprecated

---

## 四、路由变更

### 4.1 新增路由

| 路由 | 组件 | 权限 | 说明 |
|------|------|------|------|
| `/purchase` | `PaymentView.vue` | requiresAuth | 替换原 iframe 页面 |
| `/orders` | `UserOrdersView.vue` | requiresAuth | 用户订单列表 |
| `/admin/orders` | `AdminOrdersView.vue` | requiresAdmin | 订单概览（默认 tab） |
| `/admin/orders/list` | 同上组件内 tab | requiresAdmin | 订单列表 |
| `/admin/orders/channels` | 同上组件内 tab | requiresAdmin | 渠道管理 |
| `/admin/orders/plans` | 同上组件内 tab | requiresAdmin | 订阅套餐管理 |
| `/admin/orders/providers` | `AdminProvidersView.vue` | requiresAdmin | 服务商实例管理 |

### 4.2 路由 meta

所有支付相关路由需要 meta 字段：

```typescript
meta: {
  requiresAuth: true,
  requiresPayment: true,  // 新增: 需要 payment_enabled
}
```

路由守卫检查：若 `requiresPayment && !payment_enabled`，重定向到 dashboard。

---

## 五、新增 Vue 组件清单

### 5.1 用户侧页面

#### 5.1.1 `PaymentView.vue` — 充值/订阅主页面

**替换**: `PurchaseSubscriptionView.vue`（iframe）

**来源**: sub2apipay `PaymentForm.tsx` + `PurchaseFlow.tsx` + `TopUpModal.tsx`

**功能**:
- 金额输入（自定义 + 快捷按钮）
- 支付方式选择（动态展示）
- 手续费实时计算
- 余额支付开关
- 订阅套餐选购（卡片 + 确认弹窗）
- 创建订单后跳转支付流程

**子组件**: `PaymentMethodSelector.vue`, `AmountInput.vue`, `FeeDisplay.vue`, `SubscriptionPlanCard.vue`, `SubscriptionConfirmModal.vue`

**API**: `GET /api/v1/payment/config`, `GET /api/v1/payment/plans`, `POST /api/v1/payment/orders`

#### 5.1.2 `PaymentQRCodeView.vue` — 二维码支付页

**来源**: sub2apipay `PaymentQRCode.tsx`

**功能**: 二维码显示、倒计时、轮询状态（3s）、支付成功自动跳转

**子组件**: `QRCodeDisplay.vue`, `CountdownTimer.vue`

**API**: `GET /api/v1/payment/orders/:id`

#### 5.1.3 `StripePaymentView.vue` — Stripe 支付页

**来源**: sub2apipay `stripe-popup/page.tsx`

**功能**: Stripe Payment Element 集成，使用 `clientSecret` 完成支付

**依赖**: `@stripe/stripe-js`

#### 5.1.4 `PaymentResultView.vue` — 支付结果页

**功能**: 成功/失败状态展示、订单摘要、跳转按钮

#### 5.1.5 `UserOrdersView.vue` — 我的订单

**来源**: sub2apipay `OrderTable.tsx` + `MobileOrderList.tsx` + `OrderFilterBar.tsx`

**功能**: 订单列表（分页、状态筛选、日期范围）、取消/退款操作

**子组件**: `OrderStatusBadge.vue`, `OrderFilterBar.vue`

**API**: `GET /api/v1/payment/orders/my`, `POST .../cancel`, `POST .../refund-request`

### 5.2 管理员侧页面

#### 5.2.1 `AdminOrdersView.vue` — 订单管理（tab 容器）

页面内 tab 导航：订单概览 | 订单列表 | 渠道管理 | 订阅套餐

**订单概览 tab**:
- 统计卡片（今日/总收入、订单数、均价）
- 每日收入趋势图（Chart.js）
- 支付方式分布饼图
- 充值排行榜

**子组件**: `OrderStatsCards.vue`, `DailyRevenueChart.vue`, `PaymentMethodChart.vue`, `TopUsersLeaderboard.vue`

**API**: `GET /api/v1/admin/payment/dashboard`

#### 5.2.2 订单列表 tab

**来源**: sub2apipay `admin/orders/page.tsx` + `admin/OrderTable.tsx` + `admin/OrderDetail.tsx` + `admin/RefundDialog.tsx`

**功能**: 全量订单搜索/筛选、详情面板、管理操作（取消/重试/退款）

**子组件**: `AdminOrderTable.vue`, `AdminOrderDetail.vue`, `AdminRefundDialog.vue`, `AdminOrderSearchBar.vue`

**API**: `GET/POST .../admin/payment/orders`, `.../cancel`, `.../retry`, `.../refund`

#### 5.2.3 渠道管理 tab

**来源**: sub2apipay `admin/channels/page.tsx` + `ChannelCard.tsx`

**功能**: 渠道 CRUD（名称、关联 Group、费率、描述、启/禁用）

**API**: `GET/POST/PUT/DELETE /api/v1/admin/payment/channels`

#### 5.2.4 订阅套餐 tab

**来源**: sub2apipay `admin/subscriptions/page.tsx`

**功能**: 套餐 CRUD（名称、价格、有效期、分组绑定、上架状态）

**API**: `GET/POST/PUT/DELETE /api/v1/admin/payment/plans`

#### 5.2.5 `AdminProvidersView.vue` — 服务商实例管理

**来源**: sub2apipay `admin/payment-config/page.tsx`（1479 行，最大单组件）

**功能**: 服务商实例 CRUD，动态配置表单（EasyPay/Alipay/Wxpay/Stripe），限额配置，退款开关

**路由**: `/admin/orders/providers`（从系统设置 payment tab "管理实例"按钮跳转）

**子组件**: `ProviderInstanceForm.vue`, `ProviderInstanceCard.vue`

**API**: `GET/POST/PUT/DELETE /api/v1/admin/payment/providers`

### 5.3 共享组件

| 组件 | 用途 |
|------|------|
| `PaymentMethodSelector.vue` | 支付方式选择 |
| `AmountInput.vue` | 金额输入（含快捷按钮） |
| `FeeDisplay.vue` | 手续费展示 |
| `QRCodeDisplay.vue` | 二维码渲染 |
| `CountdownTimer.vue` | 倒计时 |
| `OrderStatusBadge.vue` | 订单状态标签 |
| `OrderFilterBar.vue` | 订单筛选栏 |
| `SubscriptionPlanCard.vue` | 套餐卡片 |
| `SubscriptionConfirmModal.vue` | 套餐确认弹窗 |

---

## 六、Store 变更

### 6.1 `appStore` 变更

**publicSettings 字段变更**:
```typescript
// deprecated:
purchase_subscription_enabled: boolean
purchase_subscription_url: string

// 新增:
payment_enabled: boolean  // 充值/订阅功能总开关
```

### 6.2 新增 `paymentStore`

```typescript
// src/stores/payment.ts
defineStore('payment', () => {
  const config = ref<PaymentConfig | null>(null)
  const currentOrder = ref<Order | null>(null)
  const plans = ref<SubscriptionPlan[]>([])
  
  async function fetchConfig(): Promise<PaymentConfig>
  async function fetchPlans(): Promise<SubscriptionPlan[]>
  async function createOrder(params): Promise<Order>
  async function pollOrderStatus(orderId: string): Promise<OrderStatus>
})
```

### 6.3 `adminSettingsStore` 变更

新增: `paymentEnabled` — 控制管理侧"订单管理"菜单可见性。

---

## 七、API 模块新增

### 7.1 用户 API (`src/api/payment.ts`)

```typescript
export const paymentAPI = {
  getConfig(): Promise<PaymentConfig>
  getPlans(): Promise<SubscriptionPlan[]>
  createOrder(params): Promise<CreateOrderResult>
  getOrder(orderId): Promise<Order>
  getMyOrders(params): Promise<PaginatedResult<Order>>
  cancelOrder(orderId): Promise<void>
  requestRefund(orderId, reason): Promise<void>
  getLimits(): Promise<MethodLimits>
}
```

### 7.2 管理 API (`src/api/admin/payment.ts`)

```typescript
export const adminPaymentAPI = {
  // 仪表盘
  getDashboard(days?): Promise<DashboardStats>
  // 订单
  getOrders(params): Promise<PaginatedResult<Order>>
  getOrder(orderId): Promise<OrderDetail>
  cancelOrder(orderId): Promise<void>
  retryRecharge(orderId): Promise<void>
  refundOrder(orderId, params): Promise<RefundResult>
  // 渠道
  getChannels(): Promise<Channel[]>
  createChannel(data): Promise<Channel>
  updateChannel(id, data): Promise<Channel>
  deleteChannel(id): Promise<void>
  // 订阅套餐
  getPlans(): Promise<SubscriptionPlan[]>
  createPlan(data): Promise<SubscriptionPlan>
  updatePlan(id, data): Promise<SubscriptionPlan>
  deletePlan(id): Promise<void>
  // 服务商实例
  getProviders(): Promise<ProviderInstance[]>
  createProvider(data): Promise<ProviderInstance>
  updateProvider(id, data): Promise<ProviderInstance>
  deleteProvider(id): Promise<void>
}
```

---

## 八、i18n 变更

### 8.1 新增 `payment` 命名空间

```typescript
payment: {
  // 充值页面
  title: '充值/订阅',  // 'Recharge / Subscription'
  amountLabel: '充值金额',
  quickAmounts: '快捷金额',
  customAmount: '自定义金额',
  paymentMethod: '支付方式',
  fee: '手续费',
  actualPay: '实付金额',
  createOrder: '确认支付',
  
  methods: {
    alipay: '支付宝',
    wxpay: '微信支付',
    stripe: 'Stripe',
    alipay_direct: '支付宝（直连）',
    wxpay_direct: '微信支付（直连）',
  },
  
  status: {
    pending: '待支付',
    paid: '已支付',
    recharging: '充值中',
    completed: '已完成',
    expired: '已过期',
    cancelled: '已取消',
    failed: '失败',
    refund_requested: '退款申请中',
    refunding: '退款中',
    refunded: '已退款',
    partially_refunded: '部分退款',
    refund_failed: '退款失败',
  },
  
  qr: {
    scanToPay: '请扫码支付',
    expiresIn: '剩余支付时间',
    expired: '订单已过期',
  },
  
  orders: {
    title: '我的订单',
    empty: '暂无订单',
    orderId: '订单号',
    amount: '金额',
    payAmount: '实付',
    status: '状态',
    paymentMethod: '支付方式',
    createdAt: '创建时间',
    cancel: '取消订单',
    requestRefund: '申请退款',
  },
  
  result: {
    success: '支付成功',
    failed: '支付失败',
    backToRecharge: '返回充值',
    viewOrders: '查看订单',
  },
}
```

### 8.2 修改的 i18n 键

```typescript
nav: {
  myOrders: '我的订单',           // 'My Orders'
  orderManagement: '订单管理',     // 'Order Management'
}

admin.settings.tabs: {
  payment: '充值/订阅',            // 'Recharge / Subscription'
}

admin.settings.payment: {
  title: '充值/订阅功能',
  description: '配置在线支付充值和订阅购买功能',
  enabled: '启用充值/订阅',
  enabledHint: '开启后用户可通过在线支付充值余额或购买订阅',
  minAmount: '最低充值金额',
  maxAmount: '最高充值金额',
  dailyLimit: '每日充值限额',
  maxPendingOrders: '最大待支付订单数',
  orderTimeout: '订单超时时间（分钟）',
  balancePaymentDisabled: '禁用余额支付',
  enabledPaymentTypes: '启用的支付方式',
  manageProviders: '管理服务商实例',
  helpImageUrl: '帮助图片 URL',
  helpText: '帮助文字',
}
```

### 8.3 Deprecated i18n 键

保留但不再使用：`admin.settings.purchase.*`, `purchase.*`

---

## 九、文件变更清单

### 9.1 修改的现有文件

| 文件 | 变更 |
|------|------|
| `src/components/layout/AppSidebar.vue` | 新增菜单项，条件改为 `payment_enabled` |
| `src/router/index.ts` | 新增路由，修改 `/purchase` 组件 |
| `src/views/admin/SettingsView.vue` | 新增 `payment` tab，移除旧 purchase 配置 |
| `src/stores/app.ts` | publicSettings 新增 `payment_enabled` |
| `src/stores/adminSettings.ts` | 新增 `paymentEnabled` |
| `src/stores/index.ts` | 导出新 store |
| `src/i18n/locales/en.ts` | 新增 payment 命名空间 |
| `src/i18n/locales/zh.ts` | 新增 payment 命名空间 |
| `src/api/index.ts` | 导出新 API 模块 |
| `src/types/index.ts` | 新增 payment 类型 |

### 9.2 新增文件

**视图 (10 个)**:
- `src/views/user/PaymentView.vue`
- `src/views/user/PaymentQRCodeView.vue`
- `src/views/user/StripePaymentView.vue`
- `src/views/user/PaymentResultView.vue`
- `src/views/user/UserOrdersView.vue`
- `src/views/admin/orders/AdminOrdersView.vue`
- `src/views/admin/orders/AdminOrderListView.vue`
- `src/views/admin/orders/AdminOrderChannelsView.vue`
- `src/views/admin/orders/AdminOrderPlansView.vue`
- `src/views/admin/orders/AdminProvidersView.vue`

**组件 (19 个)**:
- `src/components/payment/PaymentMethodSelector.vue`
- `src/components/payment/AmountInput.vue`
- `src/components/payment/FeeDisplay.vue`
- `src/components/payment/QRCodeDisplay.vue`
- `src/components/payment/CountdownTimer.vue`
- `src/components/payment/OrderStatusBadge.vue`
- `src/components/payment/OrderFilterBar.vue`
- `src/components/payment/SubscriptionPlanCard.vue`
- `src/components/payment/SubscriptionConfirmModal.vue`
- `src/components/admin/payment/OrderStatsCards.vue`
- `src/components/admin/payment/DailyRevenueChart.vue`
- `src/components/admin/payment/PaymentMethodChart.vue`
- `src/components/admin/payment/TopUsersLeaderboard.vue`
- `src/components/admin/payment/AdminOrderTable.vue`
- `src/components/admin/payment/AdminOrderDetail.vue`
- `src/components/admin/payment/AdminRefundDialog.vue`
- `src/components/admin/payment/AdminOrderSearchBar.vue`
- `src/components/admin/payment/ProviderInstanceForm.vue`
- `src/components/admin/payment/ProviderInstanceCard.vue`

**API / Store / Types (4 个)**:
- `src/api/payment.ts`
- `src/api/admin/payment.ts`
- `src/stores/payment.ts`
- `src/types/payment.ts`

### 9.3 删除的文件

| 文件 | 说明 |
|------|------|
| `src/views/user/PurchaseSubscriptionView.vue` | 替换为 `PaymentView.vue` |
| `src/utils/embedded-url.ts` | 不再需要 iframe URL 构建 |

### 9.4 新增依赖

| 包 | 用途 |
|----|------|
| `@stripe/stripe-js` | Stripe Payment Element |
| `qrcode` | 二维码渲染（已有此依赖） |

---

## 十、后端 Setting 表新增键（前端消费）

| Key | 类型 | 默认值 | 说明 |
|-----|------|--------|------|
| `payment_enabled` | bool | `false` | 总开关 |
| `payment_min_amount` | number | `1` | 最低充值额 |
| `payment_max_amount` | number | `10000` | 最高充值额 |
| `payment_daily_limit` | number | `50000` | 日限额 |
| `payment_max_pending_orders` | number | `3` | 最大待支付数 |
| `payment_order_timeout_minutes` | number | `30` | 订单超时 |
| `payment_balance_disabled` | bool | `false` | 禁用余额支付 |
| `payment_enabled_types` | string | `""` | 支付方式（逗号分隔） |
| `payment_help_image_url` | string | `""` | 帮助图片 |
| `payment_help_text` | string | `""` | 帮助文字 |

---

## 十一、实施顺序

### Phase 1: 基础设施
1. TypeScript 类型定义 (`types/payment.ts`)
2. API 模块 (`api/payment.ts`, `api/admin/payment.ts`)
3. Store (`stores/payment.ts`)
4. 修改 `stores/app.ts` + `stores/adminSettings.ts`
5. 修改 `router/index.ts` — 新增路由
6. 修改 `AppSidebar.vue` — 新增菜单项
7. 新增 i18n 文本

### Phase 2: 用户侧页面
1. `PaymentView.vue` + 子组件
2. `PaymentQRCodeView.vue` + QR 组件
3. `StripePaymentView.vue`
4. `PaymentResultView.vue`
5. `UserOrdersView.vue` + 订单组件

### Phase 3: 管理侧页面
1. `AdminOrdersView.vue`（tab 容器 + 订单概览）
2. 订单列表 tab + 表格/详情/退款
3. 渠道管理 tab
4. 订阅套餐 tab
5. `AdminProvidersView.vue`（最复杂）

### Phase 4: 系统设置集成
1. `SettingsView.vue` — 新增 `payment` tab
2. 移除 `general` tab 旧 purchase 配置
3. 旧版兼容迁移逻辑

---

## 附录 A: sub2apipay → sub2api 组件映射

| sub2apipay (React) | 行数 | sub2api (Vue3) | 说明 |
|---------------------|------|----------------|------|
| `PaymentForm.tsx` | ~400 | `PaymentView.vue` + 子组件 | 拆分为细粒度组件 |
| `PaymentQRCode.tsx` | ~300 | `PaymentQRCodeView.vue` | |
| `PurchaseFlow.tsx` | ~350 | 融入 `PaymentView.vue` | |
| `SubscriptionConfirm.tsx` | ~200 | `SubscriptionConfirmModal.vue` | |
| `SubscriptionPlanCard.tsx` | ~150 | `SubscriptionPlanCard.vue` | |
| `TopUpModal.tsx` | ~300 | 融入 `PaymentView.vue` | |
| `OrderTable.tsx` | ~250 | 复用 sub2api DataTable | |
| `MobileOrderList.tsx` | ~200 | 响应式设计融入 View | |
| `OrderFilterBar.tsx` | ~150 | `OrderFilterBar.vue` | |
| `OrderStatus.tsx` | ~100 | `OrderStatusBadge.vue` | |
| `admin/DashboardStats.tsx` | ~150 | `OrderStatsCards.vue` | |
| `admin/DailyChart.tsx` | ~200 | `DailyRevenueChart.vue` | |
| `admin/PaymentMethodChart.tsx` | ~150 | `PaymentMethodChart.vue` | |
| `admin/Leaderboard.tsx` | ~150 | `TopUsersLeaderboard.vue` | |
| `admin/OrderTable.tsx` | ~300 | `AdminOrderTable.vue` | |
| `admin/OrderDetail.tsx` | ~250 | `AdminOrderDetail.vue` | |
| `admin/RefundDialog.tsx` | ~200 | `AdminRefundDialog.vue` | |
| `admin/payment-config (page.tsx)` | ~1479 | 拆分为 View + Form + Card | |

## 附录 B: 关键设计决策

### B.1 为什么不保留 iframe

| 维度 | iframe | 原生集成 |
|------|--------|---------|
| 用户体验 | 两套 UI，主题同步延迟 | 统一 UI |
| 认证 | URL 传 token，泄露风险 | JWT 自动携带 |
| 部署 | 需独立部署 + 域名 | 单一部署 |
| 维护 | 两个项目两套技术栈 | 一个项目 |
| 性能 | iframe 加载开销 | 无额外开销 |
| CSP | X-Frame-Options 兼容问题 | 无此问题 |

### B.2 "订单管理"作为一级菜单

- 订单是高频管理操作，需快速访问
- 包含 4 个子功能，信息密度高
- 遵循 sub2api 现有模式：业务操作独立菜单

### B.3 `payment_enabled` 总开关

- 所有支付菜单和路由受此开关控制
- 关闭时：侧边栏不显示、路由守卫重定向、后端 API 同步校验
- 零配置时不影响不使用支付功能的用户
