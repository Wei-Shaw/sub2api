# 支付系统配置指南

Sub2API 内置支付系统，用户可自助充值，无需单独部署支付服务。当前支持两个网关：GPM Pay（越南银行转账，VietQR）和 [NOWPayments](https://nowpayments.io)（加密货币）。

---

## 目录

- [支持的支付方式](#支持的支付方式)
- [快速开始](#快速开始)
- [系统设置](#系统设置)
- [服务商配置](#服务商配置)
- [服务商实例管理](#服务商实例管理)
- [Webhook 配置](#webhook-配置)
- [支付流程](#支付流程)
- [退款](#退款)
- [从 SePay 迁移](#从-sepay-迁移)

---

## 支持的支付方式

| 支付方式 | 服务商键 | 说明 |
|----------|----------|------|
| `gpmpay_bank_transfer` | `gpmpay` | VietQR 银行转账，以 VND 结算 |
| `nowpayments_crypto` | `nowpayments` | NOWPayments 托管的加密货币账单 |

> 第三方支付服务商的安全性、可靠性与合规性请自行评估，本项目不做背书与担保。

---

## 快速开始

1. 进入管理后台 → **设置** → **支付设置**
2. 打开 **启用支付**
3. 配置基础参数（金额范围、订单超时等）
4. 在 **服务商管理** 中至少添加一个 GPM Pay 或 NOWPayments 实例
5. 在服务商后台登记回调地址（见 [Webhook 配置](#webhook-配置)）
6. 用户即可在前台充值

---

## 系统设置

在管理后台 **设置 → 支付设置** 中配置：

### 基础设置

| 设置项 | 说明 | 默认值 |
|--------|------|--------|
| **启用支付** | 支付系统总开关 | 关闭 |
| **商品名前缀** | 支付页展示的前缀 | - |
| **商品名后缀** | 支付页展示的后缀（如「点数」） | - |
| **最小 / 最大金额** | 单笔订单金额范围 | - |
| **每日限额** | 单用户每日累计限额 | - |
| **最大挂起订单数** | 单用户同时未支付的订单数 | - |
| **订单超时** | 未支付订单多少分钟后过期 | 30 |
| **负载均衡策略** | 多实例间的选择策略：`round-robin` 或 `least-amount` | `round-robin` |

### 币种

GPM Pay 固定以 **VND**（越南盾）结算，属于零小数币种：金额 `250000` 表示 ₫250,000，带小数的金额会被拒绝。

NOWPayments 按实例配置的 `currency`（`USD` 或 `VND`，管理端默认 `USD`）给账单定价。用户实际仍用加密货币支付，由 NOWPayments 在收银台按实时汇率换算。

订阅汇率设置（1 USD = X 结算币种）用于把订阅套餐的 USD 定价换算成结算币种。该功能是 opt-in：保持 `0` 时订阅按 price 直付。

### 按 USD 或 VND 充值

账户余额以 USD 计价，而 GPM Pay 只结算 VND。用户可以自己选按哪种币种填金额，切换按钮在快捷金额旁边；网关本身就以 USD 结算时该按钮隐藏。

汇率取自越南外贸银行公开牌价（`portal.vietcombank.com.vn/Usercontrols/TVPortal.TyGia/pXML.aspx`）的**卖出（Sell）**价——我们是把 USD 卖给用户，所以用卖出价。最多每小时抓一次，最近一次成功的结果会落库，重启后不至于完全没有汇率可用。

| 设置项 | 说明 | 默认值 |
|--------|------|--------|
| **汇率加价** | 在牌价之上叠加的百分比 | `0` |
| **汇率缓存最长可用时长** | 牌价接口不可用时，缓存汇率最多还能用多少小时 | `24` |

取整方向是故意不对称的，两边都朝着不利于我们的方向：收取的 VND **向上**取到整盾，记入的 USD **向下**截到分。不会出现「没收到钱却给了余额」。

牌价接口不可用且缓存已超过最长可用时长时，建单直接失败并返回 `EXCHANGE_RATE_STALE`，而不是拿一个不知道多旧的价格定价；完全没有缓存时返回 `EXCHANGE_RATE_UNAVAILABLE`。即使用户按网关币种付款也需要汇率，因为记入的余额是 USD。

`GET /api/v1/payment/exchange-rate?payment_type=<type>` 返回后端实际定价用的汇率，充值表单据此预览，和建单时用的是同一个数。前端预览不作数——后端会根据提交的 `amount` 与 `amount_currency` 重新推算两侧金额。

---

## 服务商配置

实例配置使用 `security.secret_encryption_key` 加密落库，未配置该密钥时无法保存实例。敏感字段管理端接口永不回显；编辑实例时敏感字段留空即保留已存的值。

### GPM Pay

GPM Pay 监控一个越南银行账户，每笔转入都会推送一次签名 webhook。它没有上游订单：VietQR 由 Sub2API 自己生成，转账附言里带订单码，收到转账后按这个码匹配订单。

| 字段 | 敏感 | 必填 | 说明 |
|------|------|------|------|
| `apiToken` | **是** | 是 | GPM Pay API Token，回调丢失时用来查询交易记录 |
| `webhookSecret` | **是** | 是 | GPM Pay HMAC webhook 的签名密钥。由你自己设定，在 GPM Pay 里填同一个值 |
| `bankBin` | 否 | 是 | 收款银行的 6 位 NAPAS BIN（例如 MB 为 `970422`），用于生成 VietQR |
| `accountNumber` | 否 | 是 | GPM Pay 监控的收款账号 |
| `allowSimulated` | 否 | 否 | 设为 `true` 时为 GPM Pay 模拟器发起的转账入账。仅限测试——任何能登录后台的人都能模拟一笔转账。默认 `false` |

实例仍有进行中的订单时，`webhookSecret`、`bankBin`、`accountNumber` 不允许修改——挂起订单的二维码指向的是这个账户，它们的回调也用这个密钥校验。

### NOWPayments

| 字段 | 敏感 | 必填 | 说明 |
|------|------|------|------|
| `apiKey` | **是** | 是 | NOWPayments API Key |
| `ipnSecretKey` | **是** | 是 | NOWPayments 后台生成的 IPN 密钥。NOWPayments 没有可用来复核回调的订单查询接口，签名是唯一的真实性凭据 |
| `env` | 否 | 否 | `production`（默认）或 `sandbox`。沙箱 API Key 在生产地址上无效，反之亦然 |
| `currency` | 否 | 否 | 账单的计价法币：`USD` 或 `VND` |
| `payCurrency` | 否 | 否 | 把所有账单锁定为一种币（例如 `usdttrc20`）。留空则由用户在收银台选择 |

实例仍有进行中的订单时，`apiKey`、`ipnSecretKey`、`env`、`currency` 不允许修改。

---

## 服务商实例管理

在管理后台 **设置 → 支付设置 → 服务商管理** 中添加实例。

- **支持的支付方式** — 该实例对外提供的支付方式（每个网关只有一种）
- **限额** — 按方式配置每日限额与单笔上下限；配在网关键（`gpmpay` 或 `nowpayments`）下的限额对该实例的所有方式生效
- **排序** — 收银台上的展示顺序

同一支付方式可以由多个已启用实例承载，订单按配置的负载均衡策略分摊。

---

## Webhook 配置

管理端的服务商编辑弹窗会显示当前部署的完整地址。

### GPM Pay

```
https://your-domain.com/api/v1/payment/webhook/gpmpay
```

1. 自己定一个 webhook 密钥，先在 Sub2API 的 GPM Pay 实例里保存为 `webhookSecret`，并启用该实例。
2. 在 GPM Pay 后台（**Integrations → Webhooks**）把上面的地址登记为 HMAC webhook，填同一个密钥，并保持 `fireOnSimulated` 关闭。

GPM Pay 登记 webhook 时会先 ping 一次该地址，所以密钥必须先在 Sub2API 里保存好。这次 ping 会被应答并忽略。

**回调凭什么可信。** 每个请求都带 `X-GPMPay-Signature: t=<unix>,v1=<hex>`，其中 `v1` 是用 webhook 密钥对 `"<t>." + rawBody` 计算的 HMAC-SHA256。签名不符，或 `t` 与服务器时钟相差超过 300 秒，直接拒绝。GPM Pay 会推送账户上的每一笔交易，所以转出交易、附言里没有订单码的转账都会被应答并忽略；除非实例设置了 `allowSimulated=true`，模拟转账同样忽略。

订单通过附言里的订单码找回：订单码就是订单的 `out_trade_no` 去掉下划线后转大写（银行常会把附言转大写并去掉标点）。少付、或多付超过一个很小的取整容差的转账不会入账，并以 `PAYMENT_AMOUNT_MISMATCH` 记入该订单的审计日志。

### NOWPayments

```
https://your-domain.com/api/v1/payment/webhook/nowpayments
```

在 NOWPayments 后台（**Settings → Payments → IPN**）把该地址登记为 IPN 回调，并把它生成的 IPN 密钥填入实例的 `ipnSecretKey`。

**回调凭什么可信。** `x-nowpayments-sig` 头必须等于用 IPN 密钥对「键名排序后重新序列化的回调体」计算的 HMAC-SHA512，否则拒绝。只有 `finished` 会让订单入账；`failed`、`expired` 让订单失败；中间态（`waiting`、`confirming`、`sending`、`partially_paid`）只应答，订单继续等待。入账金额取账单的 `price_amount`（配置的法币金额），从不使用链上实付的加密货币数量。

---

## 支付流程

```
用户选择金额与支付方式
       │
       ▼
  创建订单（PENDING）
  ├─ 校验金额范围、挂起订单数、每日限额
  ├─ 负载均衡选择服务商实例
  └─ GPM Pay：     本地生成 VietQR（不产生上游调用），附言带订单码
     NOWPayments： 创建托管账单，拿到账单地址
       │
       ▼
  GPM Pay：     用户在收银台扫码转账
  NOWPayments： 用户跳转到账单地址，用加密货币支付
       │
       ▼
  签名回调到达 → 定位订单并核对金额 → 订单 PAID
       │
       ▼
  自动为用户充值 → 订单 COMPLETED
```

### 订单状态

| 状态 | 说明 |
|------|------|
| `PENDING` | 等待用户支付 |
| `PAID` | 支付已确认，待入账 |
| `COMPLETED` | 入账成功 |
| `EXPIRED` | 超时未支付 |
| `CANCELLED` | 用户取消 |
| `FAILED` | 入账失败，管理员可重试 |

历史订单可能仍带有退款状态（`REFUNDED`、`REFUND_PENDING` 等），那是退款功能移除之前留下的数据：它们能正常展示，但不会再产生。

### 超时与兜底

- 把订单标记为过期之前，后台任务会先查询上游支付状态
- 后台任务同时会复查未过期的挂起订单，回调丢失时无需等到过期即可对上账
- 后台任务每 60 秒执行一次

GPM Pay 的上游查询是用 `apiToken` 在账户的转入交易里搜索订单码。NOWPayments 以回调为准：新订单只记着账单号，支付查询接口查不到它，因此查询结果是「尚未支付」。

---

## 退款

Sub2API 没有退款流程：没有退款端点，管理端没有退款操作，用户也无法申请退款。请在 Sub2API 之外处理退款（给用户转账退回，或通过 NOWPayments 处理），并在管理后台手动调整用户余额。

---

## 从 SePay 迁移

SePay 已被移除。迁移 `248_retire_sepay_provider.sql` 在启动时执行：

- 停用所有 `sepay` 服务商实例（实例保留不删，因为历史订单引用它们）
- 从 `ENABLED_PAYMENT_TYPES` 设置中剔除 `sepay_*`，GPM Pay 与 NOWPayments 的条目不受影响
- 清理前把两者快照到 `payment_provider_instances_backup_248` 和 `settings_payment_backup_248`

历史订单保留 `sepay_*` 支付方式，照常展示。挂起中的 SePay 订单不做处理：网关已无法查询，它们到期后按未支付正常过期。部署前请先检查有没有挂起中的 SePay 订单——升级后才付款的用户不会被自动入账。
