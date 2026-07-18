# 邀请返利模拟脚本

`simulate_recharge_rebate.py` 本地模拟"**被邀请人充值 → 邀请人返利 → 邀请人收到通用信箱通知**"的完整链路，用于验证 affiliate + inbox 功能。

## 链路

```
管理员登录 → 开启返利总开关/关邮箱验证码
  → 注册邀请人 A（取 aff_code）
  → 用 aff_code 注册被邀请人 B（写入 inviter_id）
  → B 创建 balance 充值订单（PENDING）
  → 直接改 DB 置订单为 PAID（本地无真实支付网关，且无 HTTP 途径标记已支付）
  → 管理员 retry 订单 → executeFulfillment → 返利入账 + 发 affiliate_recharge 信箱通知
  → 校验：A 返利额度、B 余额、A 的 inbox 通知
```

## 前置条件

1. **后端已在本地运行**（默认 `http://127.0.0.1:8080`）。
2. **信箱灰度开关必须打开**（否则返利照常，但不发通知）——这是**部署期静态配置**，改后需重启：
   ```yaml
   # backend/config.yaml
   inbox:
       v1_enabled: true
   ```
3. **可连后端的 Postgres**（第 5 步把订单置 PAID 需要）。
4. **已有一个管理员账号**（role=admin），且**未开启 TOTP 两步验证**（脚本不支持 2FA 登录）。

> 说明：本地没有内置 mock 支付 provider，webhook 走真实签名校验，也没有任何 admin HTTP 接口能把订单标记为 PAID。因此脚本必须直接改数据库把订单推进为 `PAID`，再用 `admin retry` 触发结算——这是当前代码下最简可行的本地模拟方式。

## 依赖

- 仅需 Python 3 标准库。
- 数据库步骤：优先用 `psycopg`（`pip install "psycopg[binary]"`）；未安装时回退调用系统 `psql`。

## 用法

```bash
python3 script/affiliate/simulate_recharge_rebate.py \
    --base-url http://127.0.0.1:8080 \
    --admin-email admin@example.com \
    --admin-password 'your-admin-password' \
    --db-dsn 'postgres://sub2api:sub2api@127.0.0.1:5432/sub2api?sslmode=disable' \
    --amount 100 \
    --rebate-rate 15
```

未装 psycopg 时用 psql 连接串：

```bash
    --db-url 'postgresql://sub2api:sub2api@127.0.0.1:5432/sub2api?sslmode=disable'
```

## 参数

| 参数 | 默认 | 说明 |
|---|---|---|
| `--base-url` | `http://127.0.0.1:8080` | 后端地址 |
| `--admin-email` / `--admin-password` | 必填 | 管理员账号 |
| `--db-dsn` | — | psycopg 连接串（与 `--db-url` 二选一） |
| `--db-url` | — | psql 连接串 |
| `--amount` | `100` | 充值金额 |
| `--rebate-rate` | `15` | 返利比例(%) |
| `--payment-type` | `alipay` | 占位（本地不真正支付） |
| `--password` | `Passw0rd!` | 新建测试账号密码 |

## 输出

脚本会打印邀请人返利额度（`aff_quota`/`aff_frozen_quota`）、被邀请人余额、以及邀请人收到的 `affiliate_recharge` 信箱通知（seq + payload）。测试账号使用随机后缀邮箱，不会与现有账号冲突（脚本不自动删除，便于复查）。
