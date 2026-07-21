# 邀请返利模拟 / 管理脚本

`simulate_recharge_rebate.py` 用于本地验证与管理"**邀请返利 + 通用信箱通知**"链路，提供四个子命令：

| 子命令 | 作用 |
|---|---|
| `simulate`（默认） | 跑通完整链路：造邀请人 A + 被邀请人 B + 充值 → A 返利 + A 收到信箱通知 → 校验 |
| `add` | 给**现有邀请人**增加一个或多个被邀请人，默认立即充值触发返利（`--no-recharge` 仅建绑定） |
| `recharge` | 给**现有被邀请人**充值一笔（触发返利 + 通知），**不新建账号** |
| `delete` | 删除被邀请人账号（**连账号 + 关联数据一起删**），并重算邀请人 `aff_count` |

> 无子命令时等价于 `simulate`（向后兼容旧调用方式）。

## simulate 链路

```
管理员登录 → 开启返利总开关/关邮箱验证码
  → 注册邀请人 A（取 aff_code）
  → 用 aff_code 注册被邀请人 B（写入 inviter_id）
  → 直接向 DB 插入一条 status=PAID 的 balance 订单给 B
  → 管理员 retry 订单 → executeFulfillment → 余额入账 + 返利结算 + 发 affiliate_recharge 信箱通知
  → 校验：A 返利额度、B 余额、A 的 inbox 通知
```

> 为什么直接插 DB 而不用 `POST /payment/orders`：创建订单接口会**先做支付 provider 校验**
> （`selectCreateOrderInstance` → 负载均衡选实例），本地未配置任何 enabled 的支付 provider
> 时必然返回 `503 method_not_configured`，且没有任何 admin/HTTP 接口能绕过。而
> `fulfillment`/`retry` 阶段**不校验 provider**（`RetryFulfillment` 只看 `paid_at` + `status`），
> 所以直接插一条 PAID 的 balance 订单再 retry，即可走完整结算链路。订单的 `payment_type` 只是占位。

## 前置条件

1. **后端已在本地运行**（默认 `http://127.0.0.1:8080`）——`simulate` / `add` / `recharge` 需要。
2. **可连后端的 Postgres**——所有子命令都需要（插订单 / 解析邀请人 / 删除账号）。
3. **已有管理员账号**（role=admin）且**未开启 TOTP 两步验证**（脚本不支持 2FA 登录）——`simulate` / `add` / `recharge` 需要。

> 通用信箱已**默认开启**（灰度开关已移除），无需额外配置即可收到 `affiliate_recharge` 通知。

## 依赖

- 仅需 Python 3 标准库。
- 数据库步骤优先用 `psycopg`（`pip install "psycopg[binary]"`，配合 `--db-dsn`）；未安装时回退调用系统 `psql`（配合 `--db-url`）。

## 用法

### 完整模拟

```bash
python3 script/affiliate/simulate_recharge_rebate.py \
    --admin-email admin@example.com \
    --admin-password 'your-admin-password' \
    --db-dsn 'postgres://sub2api:sub2api@127.0.0.1:5432/sub2api?sslmode=disable' \
    --amount 100 --rebate-rate 15
```

### 给现有邀请人增加被邀请人

```bash
# 通过邀请码指定邀请人，增加 3 个被邀请人（默认各充值 100 触发返利）
python3 script/affiliate/simulate_recharge_rebate.py add \
    --admin-email admin@example.com --admin-password 'pass' \
    --db-dsn 'postgres://...' \
    --inviter-aff-code ABCD2345EFGH \
    --count 3 --amount 100

# 也可用邮箱或 user_id 指定邀请人（三选一）
    --inviter-email inviter@example.com
    --inviter-id 42
```

> 默认会给每个新被邀请人充值一笔以触发返利 + 邀请人信箱通知；加 `--no-recharge` 可仅创建绑定关系（不充值、不产生返利）。

### 给现有被邀请人充值（不新建账号）

```bash
# 按邮箱/ID 给现有被邀请人充值（可重复指定多个）
python3 script/affiliate/simulate_recharge_rebate.py recharge \
    --admin-email admin@example.com --admin-password 'pass' \
    --db-dsn 'postgres://...' \
    --invitee-email aff_invitee_xxx@example.com --amount 100

# 给某邀请人名下全部被邀请人各充值一笔
python3 script/affiliate/simulate_recharge_rebate.py recharge \
    --admin-email admin@example.com --admin-password 'pass' \
    --db-dsn 'postgres://...' \
    --inviter-aff-code ABCD2345EFGH --all --amount 100
```

### 删除被邀请人（连账号）

```bash
# 按邮箱删除（可重复 --invitee-email 指定多个）
python3 script/affiliate/simulate_recharge_rebate.py delete \
    --db-dsn 'postgres://...' \
    --invitee-email aff_invitee_xxx@example.com \
    --invitee-email aff_invitee_yyy@example.com

# 按 user_id 删除
    --invitee-id 101 --invitee-id 102

# 删除某邀请人名下的全部被邀请人
python3 script/affiliate/simulate_recharge_rebate.py delete \
    --db-dsn 'postgres://...' \
    --inviter-email inviter@example.com --all --yes
```

> 删除为破坏性操作：默认会列出目标账号并要求输入 `yes` 确认；`--yes` 可跳过交互。
> 删除范围：`payment_orders` / `balance_ledger` / `payment_audit_logs`（无外键，手动清理）
> + `users`（触发 `user_affiliates` / `user_affiliate_ledger` 等外键的 CASCADE / SET NULL），
> 最后重算受影响邀请人的 `aff_count`（整体在一个事务内完成）。

未装 psycopg 时改用 psql 连接串：

```bash
    --db-url 'postgresql://sub2api:sub2api@127.0.0.1:5432/sub2api?sslmode=disable'
```

## 参数速查

公共（`simulate` / `add` / `recharge`）：`--base-url` `--admin-email` `--admin-password`；
数据库（全部子命令）：`--db-dsn`（psycopg）/ `--db-url`（psql，二选一）。

| 子命令 | 参数 | 默认 | 说明 |
|---|---|---|---|
| simulate | `--amount` / `--rebate-rate` / `--payment-type` / `--password` / `--keep` | 100 / 15 / alipay / Passw0rd! / — | 充值金额、返利比例、占位支付方式、新账号密码、保留提示 |
| add | `--inviter-aff-code` / `--inviter-email` / `--inviter-id` | 必填其一 | 指定现有邀请人 |
| add | `--count` / `--amount` / `--rebate-rate` / `--no-recharge` | 1 / 100 / 15 / off | 新增数量、充值金额与比例；`--no-recharge` 时仅建绑定不充值（默认充值） |
| delete | `--invitee-email` / `--invitee-id` | — | 要删除的被邀请人（可多次指定） |
| delete | `--inviter-*` + `--all` / `--yes` | — | 删除某邀请人名下全部被邀请人 / 跳过确认 |
| recharge | `--invitee-email` / `--invitee-id` / `--inviter-*` + `--all` | — | 指定现有被邀请人（或某邀请人名下全部），只充值不新建账号 |
| recharge | `--amount` / `--rebate-rate` / `--payment-type` | 100 / 15 / alipay | 充值金额、返利比例、占位支付方式 |

## 输出

- `simulate`：打印邀请人返利额度（`aff_quota`/`aff_frozen_quota`）、被邀请人余额、邀请人收到的 `affiliate_recharge` 通知（seq + payload）。
- `add`：打印新增被邀请人的 `user_id` / 邮箱，以及（默认充值时）预期返利。
- `recharge`：打印已充值的被邀请人 `user_id` / 邮箱与每笔预期返利。
- `delete`：打印被删账号清单与删除后邀请人的 `aff_count`。

测试账号使用随机后缀邮箱（`aff_inviter_*` / `aff_invitee_*`），不与现有账号冲突。
