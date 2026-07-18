#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
simulate_recharge_rebate.py —— 本地模拟"被邀请人充值 -> 邀请人返利 + 通用信箱通知"全链路。

对应"方式二"（本地起服务走 HTTP 全链路），把手动步骤自动化：
  1) 管理员登录，开启返利总开关 + 关闭邮箱验证码（先 GET 再改字段再整体 PUT，避免误重置）。
  2) 注册邀请人 A，取其邀请码 aff_code。
  3) 用 A 的 aff_code 注册被邀请人 B（写入 user_affiliates.inviter_id = A）。
  4) B 创建一笔 balance 充值订单（初始 PENDING）。
  5) 直接改 DB 把订单推进为 PAID（本地无真实支付网关，且没有任何 HTTP 接口能标记已支付）。
  6) 管理员 retry 订单 -> 触发 fulfillment -> 返利入账 + 给 A 发 affiliate_recharge 信箱通知。
  7) 校验：A 返利额度增长、B 余额到账、A 收到 inbox 通知。

前置要求：
  - 后端服务已在本地运行（默认 http://127.0.0.1:8080）。
  - 通用信箱灰度开关为部署期静态配置，需在 backend/config.yaml 设 inbox.v1_enabled: true 并重启，
    否则返利照常入账但不会发信箱通知（脚本会检测并提示）。
  - 能连上后端的 Postgres（用于第 5 步把订单置 PAID）。

用法示例：
  python3 simulate_recharge_rebate.py \
      --base-url http://127.0.0.1:8080 \
      --admin-email admin@example.com --admin-password 'admin-pass' \
      --db-dsn 'postgres://sub2api:sub2api@127.0.0.1:5432/sub2api?sslmode=disable' \
      --amount 100 --rebate-rate 15

  # 若装了 psycopg（pip install "psycopg[binary]"）会优先用它；否则回退到 psql 命令。
  # 也可用 --db-url 传给 psql 的连接串（与 --db-dsn 二选一，psql 形式）。

依赖：仅标准库即可运行；DB 步骤优先用 psycopg，缺失时回退调用系统 psql。
"""
from __future__ import annotations

import argparse
import json
import shutil
import subprocess
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid


# --------------------------------------------------------------------------- #
# 通用 HTTP 封装（标准库，避免额外依赖）
# --------------------------------------------------------------------------- #
class APIError(RuntimeError):
    pass


class Client:
    """极简 sub2api HTTP 客户端：自动带 Bearer token、解包 {code,message,data} 信封。"""

    def __init__(self, base_url: str, timeout: float = 20.0):
        self.base = base_url.rstrip("/")
        self.timeout = timeout
        self.token: str | None = None

    def _request(self, method: str, path: str, *, body: dict | None = None,
                 params: dict | None = None, auth: bool = True,
                 admin_api_key: str | None = None) -> dict:
        url = f"{self.base}/api/v1{path}"
        if params:
            url += "?" + urllib.parse.urlencode(params)
        data = None
        headers = {"Accept": "application/json"}
        if body is not None:
            data = json.dumps(body).encode("utf-8")
            headers["Content-Type"] = "application/json"
        if admin_api_key:
            headers["x-api-key"] = admin_api_key
        elif auth and self.token:
            headers["Authorization"] = f"Bearer {self.token}"

        req = urllib.request.Request(url, data=data, headers=headers, method=method)
        try:
            with urllib.request.urlopen(req, timeout=self.timeout) as resp:
                raw = resp.read().decode("utf-8")
        except urllib.error.HTTPError as e:
            detail = e.read().decode("utf-8", "replace")
            raise APIError(f"{method} {path} -> HTTP {e.code}: {detail}") from None
        except urllib.error.URLError as e:
            raise APIError(f"{method} {path} -> 连接失败: {e.reason} (后端是否已启动 {self.base}?)") from None

        try:
            payload = json.loads(raw) if raw else {}
        except json.JSONDecodeError:
            raise APIError(f"{method} {path} -> 非 JSON 响应: {raw[:200]}") from None

        # 统一信封 {code,message,data}
        if isinstance(payload, dict) and "code" in payload:
            if payload.get("code") not in (0, None):
                raise APIError(f"{method} {path} -> 业务错误 code={payload.get('code')}: "
                               f"{payload.get('message')}")
            return payload.get("data", {}) or {}
        return payload

    def get(self, path, **kw):
        return self._request("GET", path, **kw)

    def post(self, path, **kw):
        return self._request("POST", path, **kw)

    def put(self, path, **kw):
        return self._request("PUT", path, **kw)


# --------------------------------------------------------------------------- #
# 业务步骤
# --------------------------------------------------------------------------- #
def login(cli: Client, email: str, password: str) -> dict:
    data = cli.post("/auth/login", body={"email": email, "password": password}, auth=False)
    if data.get("requires_2fa"):
        raise APIError(f"账号 {email} 开启了 TOTP 两步验证，脚本不支持；请用未开启 2FA 的账号。")
    token = data.get("access_token")
    if not token:
        raise APIError(f"登录 {email} 未返回 access_token: {data}")
    cli.token = token
    return data


def register(base_url: str, email: str, password: str, aff_code: str | None = None) -> dict:
    """注册返回 AuthResponse（含 access_token + user）。用独立 Client 避免污染 token。"""
    c = Client(base_url)
    body = {"email": email, "password": password}
    if aff_code:
        body["aff_code"] = aff_code
    data = c.post("/auth/register", body=body, auth=False)
    if not data.get("access_token"):
        raise APIError(f"注册 {email} 未返回 access_token（是否开启了邮箱验证码？）: {data}")
    c.token = data["access_token"]
    return {"client": c, "auth": data}


def ensure_affiliate_settings(admin: Client, rebate_rate: float) -> None:
    """先 GET 完整设置，改 affiliate/邮箱验证码相关字段后整体 PUT（避免非指针字段被重置）。"""
    settings = admin.get("/admin/settings")
    if not isinstance(settings, dict):
        raise APIError(f"读取 /admin/settings 异常: {settings}")

    # 覆盖关键字段（保留其余原值一并回写）
    settings["affiliate_enabled"] = True
    settings["affiliate_rebate_rate"] = rebate_rate
    settings["affiliate_rebate_freeze_hours"] = 0
    settings["affiliate_rebate_duration_days"] = 0
    settings["affiliate_rebate_per_invitee_cap"] = 0
    # 关闭注册障碍，便于脚本注册测试账号
    settings["email_verify_enabled"] = False
    settings["registration_enabled"] = True

    admin.put("/admin/settings", body=settings)


def get_aff_code(cli: Client) -> str:
    data = cli.get("/user/aff")
    code = data.get("aff_code")
    if not code:
        raise APIError(f"未取到 aff_code: {data}")
    return code


def create_balance_order(cli: Client, amount: float, payment_type: str) -> dict:
    data = cli.post("/payment/orders", body={
        "amount": amount,
        "payment_type": payment_type,
        "order_type": "balance",
    })
    if not data.get("order_id"):
        raise APIError(f"创建订单未返回 order_id: {data}")
    return data


# --------------------------------------------------------------------------- #
# DB：把订单推进为 PAID（本地无真实网关，且无 HTTP 途径标记已支付）
# --------------------------------------------------------------------------- #
def mark_order_paid_in_db(order_id: int, amount: float, dsn: str | None, psql_url: str | None) -> None:
    sql = (
        "UPDATE payment_orders "
        "SET status = 'PAID', paid_at = now(), "
        f"    pay_amount = CASE WHEN pay_amount IS NULL OR pay_amount = 0 THEN {amount} ELSE pay_amount END "
        f"WHERE id = {int(order_id)} AND status = 'PENDING';"
    )
    # 优先 psycopg
    if dsn:
        try:
            import psycopg  # type: ignore

            with psycopg.connect(dsn) as conn:  # noqa: SIM117
                with conn.cursor() as cur:
                    cur.execute(sql)
                    affected = cur.rowcount
                conn.commit()
            if affected < 1:
                raise APIError(f"订单 {order_id} 未被更新（可能不是 PENDING 状态）")
            print(f"  [DB] 订单 {order_id} 已置 PAID（psycopg, 影响 {affected} 行）")
            return
        except ImportError:
            print("  [DB] 未安装 psycopg，回退到 psql 命令 ...")
        except Exception as e:  # noqa: BLE001
            raise APIError(f"psycopg 更新订单失败: {e}") from None

    # 回退 psql
    conn_str = psql_url or dsn
    if not conn_str:
        raise APIError("需要数据库连接：请提供 --db-dsn（psycopg）或 --db-url（psql）")
    psql = shutil.which("psql")
    if not psql:
        raise APIError("未找到 psql，且未安装 psycopg。请 `pip install \"psycopg[binary]\"` 或安装 psql。")
    proc = subprocess.run(  # noqa: S603
        [psql, conn_str, "-v", "ON_ERROR_STOP=1", "-c", sql],
        capture_output=True, text=True,
    )
    if proc.returncode != 0:
        raise APIError(f"psql 更新订单失败: {proc.stderr.strip() or proc.stdout.strip()}")
    if "UPDATE 0" in proc.stdout:
        raise APIError(f"订单 {order_id} 未被更新（可能不是 PENDING 状态）: {proc.stdout.strip()}")
    print(f"  [DB] 订单 {order_id} 已置 PAID（psql）: {proc.stdout.strip()}")


# --------------------------------------------------------------------------- #
# 校验
# --------------------------------------------------------------------------- #
def fetch_inbox_recharge(inviter_cli: Client) -> list[dict]:
    """从 catchup 拉取 affiliate_recharge namespace 的消息。"""
    data = inviter_cli.get("/inbox/catchup", params={"since": 0})
    msgs = data.get("messages", []) or []
    return [m for m in msgs if m.get("namespace") == "affiliate_recharge"]


def main() -> int:
    ap = argparse.ArgumentParser(description="模拟充值返利 + 信箱通知全链路")
    ap.add_argument("--base-url", default="http://127.0.0.1:8080", help="后端服务地址")
    ap.add_argument("--admin-email", required=True, help="管理员邮箱")
    ap.add_argument("--admin-password", required=True, help="管理员密码")
    ap.add_argument("--db-dsn", default=None,
                    help="Postgres DSN（psycopg 用），例：postgres://user:pass@host:5432/db?sslmode=disable")
    ap.add_argument("--db-url", default=None, help="psql 连接串（未装 psycopg 时用）")
    ap.add_argument("--amount", type=float, default=100.0, help="充值金额")
    ap.add_argument("--rebate-rate", type=float, default=15.0, help="返利比例(%%)")
    ap.add_argument("--payment-type", default="alipay", help="支付方式（仅占位，本地不真正支付）")
    ap.add_argument("--password", default="Passw0rd!", help="新建测试账号的密码")
    ap.add_argument("--keep", action="store_true", help="保留测试账号（默认也不删除，仅打印）")
    args = ap.parse_args()

    if not args.db_dsn and not args.db_url:
        print("错误：需要数据库连接以把订单置为 PAID（本地无真实支付网关）。"
              "请提供 --db-dsn 或 --db-url。", file=sys.stderr)
        return 2

    suffix = uuid.uuid4().hex[:8]
    inviter_email = f"aff_inviter_{suffix}@example.com"
    invitee_email = f"aff_invitee_{suffix}@example.com"

    admin = Client(args.base_url)

    print(f"[1/8] 管理员登录 {args.admin_email} ...")
    login(admin, args.admin_email, args.admin_password)

    print(f"[2/8] 开启返利总开关 + 关闭邮箱验证码（rebate_rate={args.rebate_rate}%）...")
    ensure_affiliate_settings(admin, args.rebate_rate)

    print(f"[3/8] 注册邀请人 A: {inviter_email} ...")
    a = register(args.base_url, inviter_email, args.password)
    inviter_cli: Client = a["client"]
    inviter_id = a["auth"]["user"]["id"]
    aff_code = get_aff_code(inviter_cli)
    print(f"      A user_id={inviter_id}, aff_code={aff_code}")

    print(f"[4/8] 用 A 的 aff_code 注册被邀请人 B: {invitee_email} ...")
    b = register(args.base_url, invitee_email, args.password, aff_code=aff_code)
    invitee_cli: Client = b["client"]
    invitee_id = b["auth"]["user"]["id"]
    print(f"      B user_id={invitee_id}")

    print(f"[5/8] B 创建 balance 充值订单 amount={args.amount} ...")
    order = create_balance_order(invitee_cli, args.amount, args.payment_type)
    order_id = order["order_id"]
    print(f"      order_id={order_id}, status={order.get('status')}")

    print("[6/8] 直接改 DB 把订单置为 PAID（本地无真实支付网关）...")
    mark_order_paid_in_db(order_id, args.amount, args.db_dsn, args.db_url)

    print("[7/8] 管理员 retry 订单，触发到账 + 返利结算 + 信箱通知 ...")
    admin.post(f"/admin/payment/orders/{order_id}/retry")
    time.sleep(1.0)  # 给异步/事务提交一点时间

    print("[8/8] 校验结果 ...")
    aff = inviter_cli.get("/user/aff")
    quota = aff.get("aff_quota", 0)
    frozen = aff.get("aff_frozen_quota", 0)
    me_b = invitee_cli.get("/auth/me")
    b_balance = (me_b.get("user") or me_b).get("balance")

    recharge_msgs = fetch_inbox_recharge(inviter_cli)

    print("\n================ 结果 ================")
    print(f"邀请人 A(user_id={inviter_id}) 返利: aff_quota={quota}, aff_frozen_quota={frozen}")
    print(f"被邀请人 B(user_id={invitee_id}) 余额: balance={b_balance}")
    if recharge_msgs:
        print(f"A 收到 affiliate_recharge 信箱通知 {len(recharge_msgs)} 条:")
        for m in recharge_msgs:
            print(f"  - seq={m.get('seq')} payload={m.get('payload')}")
    else:
        print("A 未收到 affiliate_recharge 信箱通知。")
        print("  可能原因：config.Inbox.V1Enabled 未开启（需在 backend/config.yaml 设 inbox.v1_enabled: true 并重启），")
        print("  或本次充值未产生返利（比例/有效期/上限），或 catchup 首次懒初始化时序问题。")
    expected = round(args.amount * args.rebate_rate / 100, 8)
    print(f"预期返利 ≈ {expected}（比例 {args.rebate_rate}% × 金额 {args.amount}）")
    print("=====================================")
    print(f"\n测试账号（{'保留' if args.keep else '未自动清理'}）：\n  A: {inviter_email}\n  B: {invitee_email}\n  密码: {args.password}")
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except APIError as e:
        print(f"\n[失败] {e}", file=sys.stderr)
        sys.exit(1)
    except KeyboardInterrupt:
        sys.exit(130)
