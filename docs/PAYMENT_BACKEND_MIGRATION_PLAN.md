# 支付系统 Go 后端实现计划

> 将 sub2apipay (TypeScript/Prisma) 后端逻辑迁移到 sub2api (Go/Ent)
>
> 分支: `feature/payment` (基于 upstream `Wei-Shaw/sub2api:main`)

---

## 一、新增 Ent Schema (5 个)

| Schema | 表名 | 说明 |
|--------|------|------|
| PaymentOrder | payment_orders | 充值/订阅订单 |
| PaymentAuditLog | payment_audit_logs | 订单审计日志 |
| PaymentChannel | payment_channels | 展示渠道 |
| SubscriptionPlan | subscription_plans | 订阅套餐 |
| PaymentProviderInstance | payment_provider_instances | 支付服务商实例 |

所有表使用 `payment_` 前缀避免与现有表冲突。

### PaymentOrder 字段

- user_id, user_email, user_name, user_notes
- amount(充值额), pay_amount(实付), fee_rate, recharge_code
- payment_type, payment_trade_no, pay_url, qr_code, qr_code_img
- order_type(balance/subscription), plan_id, subscription_group_id, subscription_days
- provider_instance_id
- status (PENDING/PAID/RECHARGING/COMPLETED/EXPIRED/CANCELLED/FAILED/REFUND_REQUESTED/REFUNDING/PARTIALLY_REFUNDED/REFUNDED/REFUND_FAILED)
- refund_amount, refund_reason, refund_at, force_refund, refund_requested_at, refund_request_reason, refund_requested_by
- expires_at, paid_at, completed_at, failed_at, failed_reason
- client_ip, src_host, src_url
- created_at, updated_at
- 索引: user_id, status, expires_at, created_at, paid_at, (payment_type,paid_at), order_type

### PaymentAuditLog 字段

- order_id, action, detail, operator, created_at

### PaymentChannel 字段

- group_id(关联sub2api Group), name, platform, rate_multiplier, description, models(JSON), features(JSON), sort_order, enabled, created_at, updated_at

### SubscriptionPlan 字段

- group_id, name, description, price, original_price, validity_days, validity_unit(day/week/month), features(JSON), product_name, for_sale, sort_order, created_at, updated_at

### PaymentProviderInstance 字段

- provider_key(easypay/alipay/wxpay/stripe), name, config(AES加密JSON), supported_types, enabled, sort_order, limits(JSON), refund_enabled, created_at, updated_at

---

## 二、Go 接口定义

### 2.1 Provider 接口

```go
// internal/payment/types.go
type Provider interface {
    Name() string
    ProviderKey() string
    SupportedTypes() []PaymentType
    CreatePayment(ctx context.Context, req CreatePaymentRequest) (*CreatePaymentResponse, error)
    QueryOrder(ctx context.Context, tradeNo string) (*QueryOrderResponse, error)
    VerifyNotification(ctx context.Context, body []byte, headers map[string]string) (*PaymentNotification, error)
    Refund(ctx context.Context, req RefundRequest) (*RefundResponse, error)
}

type CancelableProvider interface {
    Provider
    CancelPayment(ctx context.Context, tradeNo string) error
}
```

### 2.2 Registry

```go
// internal/payment/registry.go
type Registry struct {
    providers map[string]Provider       // providerKey -> Provider
    typeMap   map[PaymentType]string    // paymentType -> providerKey
}

func (r *Registry) Register(p Provider)
func (r *Registry) GetProvider(paymentType PaymentType) (Provider, error)
func (r *Registry) GetProviderKey(paymentType PaymentType) string
func (r *Registry) SupportedTypes() []PaymentType
```

### 2.3 LoadBalancer

```go
// internal/payment/load_balancer.go
type LoadBalancer interface {
    SelectInstance(ctx context.Context, providerKey string, paymentType PaymentType) (*InstanceSelection, error)
}
```

---

## 三、Service 层

### 3.1 PaymentService — 订单全生命周期

```go
// 直接注入现有 service，不走 HTTP
type PaymentService struct {
    db              *ent.Client
    registry        *payment.Registry
    loadBalancer    payment.LoadBalancer
    redeemService   *RedeemService        // 余额充值直接调用
    subscriptionSvc *SubscriptionService  // 订阅激活直接调用
    adminService    AdminService          // 余额扣减
    configService   *PaymentConfigService
}

// 订单
CreateOrder / GetOrder / GetUserOrders / CancelOrder / AdminCancelOrder
// 支付确认 & 履约
HandlePaymentNotification / ExecuteBalanceFulfillment / ExecuteSubscriptionFulfillment / RetryFulfillment
// 退款三阶段
RequestRefund / PrepareRefund / ExecuteRefund / RollbackRefund
// 超时
ExpireTimedOutOrders
// 统计
GetDashboardStats
```

### 3.2 PaymentConfigService — 配置 & CRUD

```go
type PaymentConfigService struct {
    db             *ent.Client
    settingService *SettingService  // 复用现有 Setting 表
    encryptionKey  []byte
}

// 系统配置
IsPaymentEnabled / GetPaymentConfig / UpdatePaymentConfig
// 服务商实例 CRUD
ListProviderInstances / CreateProviderInstance / UpdateProviderInstance / DeleteProviderInstance
// 渠道 CRUD
ListChannels / CreateChannel / UpdateChannel / DeleteChannel
// 套餐 CRUD
ListPlans / CreatePlan / UpdatePlan / DeletePlan
// 手续费 & 限额
GetMethodLimits / CalculatePayAmount
```

### 3.3 关键差异 vs sub2apipay

| 维度 | sub2apipay (TS) | sub2api (Go) |
|------|-----------------|-------------|
| 余额充值 | HTTP 调 sub2api API | 直接调 RedeemService |
| 订阅激活 | HTTP 调 sub2api API | 直接调 SubscriptionService |
| 用户验证 | HTTP 调 /api/v1/auth/me | JWT middleware 已处理 |
| Decimal | Prisma.Decimal | shopspring/decimal |
| 加密 | Node.js crypto | Go crypto/aes + cipher |
| 配置存储 | SystemConfig 表 | 复用现有 Setting 表 |

---

## 四、Handler 层 & API 端点

### 4.1 用户端 (9 个)

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/v1/payment/config | 支付配置 |
| GET | /api/v1/payment/plans | 套餐列表 |
| GET | /api/v1/payment/channels | 渠道列表 |
| GET | /api/v1/payment/limits | 支付方式限额 |
| POST | /api/v1/payment/orders | 创建订单 |
| GET | /api/v1/payment/orders/my | 用户订单列表 |
| GET | /api/v1/payment/orders/:id | 订单详情 |
| POST | /api/v1/payment/orders/:id/cancel | 取消订单 |
| POST | /api/v1/payment/orders/:id/refund-request | 申请退款 |

### 4.2 Webhook (4 个，无需认证)

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | /api/v1/payment/webhook/easypay | EasyPay 回调 |
| POST | /api/v1/payment/webhook/alipay | 支付宝回调 |
| POST | /api/v1/payment/webhook/wxpay | 微信支付回调 |
| POST | /api/v1/payment/webhook/stripe | Stripe Webhook |

### 4.3 管理端 (18 个)

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /admin/payment/dashboard | 仪表盘 |
| GET | /admin/payment/orders | 订单列表 |
| GET | /admin/payment/orders/:id | 订单详情 |
| POST | /admin/payment/orders/:id/cancel | 取消订单 |
| POST | /admin/payment/orders/:id/retry | 重试充值 |
| POST | /admin/payment/orders/:id/refund | 处理退款 |
| GET/POST | /admin/payment/channels | 渠道列表/创建 |
| PUT/DELETE | /admin/payment/channels/:id | 更新/删除 |
| GET/POST | /admin/payment/plans | 套餐列表/创建 |
| PUT/DELETE | /admin/payment/plans/:id | 更新/删除 |
| GET/POST | /admin/payment/providers | 实例列表/创建 |
| PUT/DELETE | /admin/payment/providers/:id | 更新/删除 |

---

## 五、Provider 实现

### 目录结构

```
internal/payment/
├── types.go, registry.go, load_balancer.go, fee.go, crypto.go, wire.go
├── provider_factory.go
└── provider/
    ├── easypay.go    # 纯 HTTP + MD5 (无需 SDK)
    ├── alipay.go     # smartwalle/alipay/v3 SDK
    ├── wxpay.go      # wechatpay-apiv3/wechatpay-go 官方 SDK
    └── stripe.go     # stripe/stripe-go 官方 SDK
```

### Go 依赖

| 包 | 用途 |
|----|------|
| github.com/stripe/stripe-go/v82 | Stripe 官方 |
| github.com/wechatpay-apiv3/wechatpay-go | 微信支付官方 |
| github.com/smartwalle/alipay/v3 | 支付宝 |
| github.com/shopspring/decimal | 精确十进制 |

---

## 六、数据库迁移 (5 个 SQL 文件)

```
migrations/090_payment_orders.sql
migrations/091_payment_audit_logs.sql
migrations/092_payment_channels.sql
migrations/093_subscription_plans.sql
migrations/094_payment_provider_instances.sql
```

---

## 七、Wire DI

```go
// internal/payment/wire.go
var ProviderSet = wire.NewSet(
    NewRegistry, NewLoadBalancer,
    NewPaymentConfigService, NewPaymentService,
    NewPaymentHandler, NewPaymentAdminHandler, NewPaymentWebhookHandler,
)
```

---

## 八、路由注册

```go
// internal/server/routes/payment.go
func RegisterPaymentRoutes(v1 *gin.RouterGroup, h *handler.Handlers, jwtAuth, adminAuth middleware)
```

在 router.go 的 registerRoutes() 中新增:
```go
routes.RegisterPaymentRoutes(v1, h, jwtAuth, adminAuth)
```

---

## 九、新增文件清单 (~25 个 Go + 5 个 SQL)

**payment 核心 (8)**:
- internal/payment/types.go, registry.go, load_balancer.go
- internal/payment/fee.go, crypto.go, wire.go, provider_factory.go

**provider 实现 (4)**:
- internal/payment/provider/easypay.go, alipay.go, wxpay.go, stripe.go

**service (2)**:
- internal/service/payment_service.go, payment_config_service.go

**handler (3)**:
- internal/handler/payment_handler.go, payment_webhook_handler.go
- internal/handler/admin/payment_handler.go

**routes (1)**:
- internal/server/routes/payment.go

**schema (5)**:
- ent/schema/payment_order.go, payment_audit_log.go, payment_channel.go
- ent/schema/subscription_plan.go, payment_provider_instance.go

**migration (5)**:
- migrations/090-094

**Ent generate**: 运行 `go generate ./ent` 生成 CRUD 代码
