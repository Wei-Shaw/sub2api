#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
simulate_recharge_rebate.py —— 本地模拟/管理"邀请返利 + 通用信箱通知"链路。

提供三个子命令：

  simulate（默认，无子命令时等价于它）
      跑通完整链路：注册邀请人 A -> 用 A 的 aff_code 注册被邀请人 B ->
      给 B 插一条 PAID 订单 -> admin retry -> B 到账 + A 返利 + A 收到信箱通知 -> 校验。

  add
      给"现有邀请人"增加一个或多个被邀请人（注册新账号并用邀请人 aff_code 绑定），
      默认给每个新被邀请人充值一笔以触发返利 + 邀请人信箱通知（--no-recharge 可仅建绑定）。
      邀请人可用 --inviter-aff-code / --inviter-email / --inviter-id 指定。

  recharge
      给"现有被邀请人"充值一笔（触发返利 + 通知），不新建账号。按 --invitee-email /
      --invitee-id 指定，或 --inviter-* 配合 --all 给某邀请人名下全部被邀请人充值。

  delete
      删除被邀请人账号（连账号 + 关联数据一起删）。可按 --invitee-email /
      --invitee-id 指定，或 --inviter-* 配合 --all 删除某邀请人名下全部被邀请人。
      删除后自动重算邀请人的 aff_count。破坏性操作，需 --yes 或交互确认。

为什么直接操作 DB 而不用 HTTP 建单：
  POST /payment/orders 会先做支付 provider 校验（selectCreateOrderInstance ->
  loadBalancer.SelectInstance），本地未配置任何 enabled 的 provider 时必然 503
  method_not_configured，且没有 admin/HTTP 接口能绕过。而 fulfillment/retry 阶段
  不校验 provider，因此直接 INSERT status=PAID 的 balance 订单再 retry 即可走完链路。

前置要求：
  - 后端服务已在本地运行（默认 http://127.0.0.1:8080）。simulate/add 需要。
  - 能连上后端的 Postgres（所有子命令都需要，用于插订单 / 解析邀请人 / 删除账号）。
  - 通用信箱已默认开启（灰度开关已移除），无需额外配置即可收到 affiliate_recharge 通知。

用法示例：
  # 完整模拟
  python3 simulate_recharge_rebate.py \
      --admin-email admin@example.com --admin-password 'admin-pass' \
      --db-dsn 'postgres://sub2api:sub2api@127.0.0.1:5432/sub2api?sslmode=disable'

  # 给现有邀请人增加 3 个被邀请人（默认各充值 100 触发返利）
  python3 simulate_recharge_rebate.py add \
      --admin-email admin@example.com --admin-password 'admin-pass' \
      --db-dsn 'postgres://...' \
      --inviter-email inviter@example.com --count 3 --amount 100

  # 删除某邀请人名下全部被邀请人（连账号）
  python3 simulate_recharge_rebate.py delete \
      --db-dsn 'postgres://...' \
      --inviter-email inviter@example.com --all --yes

依赖：仅标准库即可运行；DB 步骤优先用 psycopg（pip install "psycopg[binary]"），
      缺失时回退调用系统 psql（此时用 --db-url 传 psql 连接串）。
"""
from __future__ import annotations

import argparse
import json
import secrets
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
# DB 访问层：统一屏蔽 psycopg / psql 两种后端。
#
# 上层一律用 %s 占位 + 参数列表书写 SQL：
#   - psycopg 路径：原生参数化执行；
#   - psql 路径：把参数渲染成字面量后拼进 SQL（测试数据可信，做基本转义）。
# --------------------------------------------------------------------------- #
class DB:
    def __init__(self, dsn: str | None, psql_url: str | None):
        self.dsn = dsn
        self.psql_url = psql_url
        self._psycopg = None
        if dsn:
            try:
                import psycopg  # type: ignore
                self._psycopg = psycopg
            except ImportError:
                self._psycopg = None
        if not self.usable:
            raise APIError("需要数据库连接：请提供 --db-dsn（psycopg）或 --db-url（psql）")

    @property
    def use_psycopg(self) -> bool:
        return self._psycopg is not None and bool(self.dsn)

    @property
    def usable(self) -> bool:
        return self.use_psycopg or bool(self.psql_url or self.dsn)

    # --- 字面量渲染（仅 psql 路径使用）--- #
    @staticmethod
    def _lit(v) -> str:
        if v is None:
            return "NULL"
        if isinstance(v, bool):
            return "TRUE" if v else "FALSE"
        if isinstance(v, int):
            return str(int(v))
        if isinstance(v, float):
            return repr(float(v))
        if isinstance(v, (list, tuple)):
            return "ARRAY[" + ",".join(DB._lit(x) for x in v) + "]"
        return "'" + str(v).replace("'", "''") + "'"

    @classmethod
    def _render(cls, sql: str, params) -> str:
        params = list(params or [])
        parts = sql.split("%s")
        if len(parts) - 1 != len(params):
            raise APIError(f"SQL 占位符数量与参数不匹配: {sql!r} params={params!r}")
        out = []
        for i, seg in enumerate(parts):
            out.append(seg)
            if i < len(params):
                out.append(cls._lit(params[i]))
        return "".join(out)

    def _psql_bin(self) -> str:
        psql = shutil.which("psql")
        if not psql:
            raise APIError('未找到 psql，且未安装 psycopg。请 `pip install "psycopg[binary]"` 或安装 psql。')
        return psql

    def _psql_conn(self) -> str:
        conn = self.psql_url or self.dsn
        if not conn:
            raise APIError("psql 路径需要 --db-url 或 --db-dsn 连接串。")
        return conn

    # --- 查询：返回 list[tuple[str|Any, ...]] --- #
    def query(self, sql: str, params=()) -> list:
        if self.use_psycopg:
            with self._psycopg.connect(self.dsn) as conn:  # noqa: SIM117
                with conn.cursor() as cur:
                    cur.execute(sql, list(params) or None)
                    return list(cur.fetchall())
        sep = "\x1f"
        rendered = self._render(sql, params)
        proc = subprocess.run(  # noqa: S603
            [self._psql_bin(), self._psql_conn(), "-t", "-A", "-F", sep,
             "-v", "ON_ERROR_STOP=1", "-c", rendered],
            capture_output=True, text=True,
        )
        if proc.returncode != 0:
            raise APIError(f"psql 查询失败: {proc.stderr.strip() or proc.stdout.strip()}")
        rows = []
        for line in proc.stdout.splitlines():
            if line == "":
                continue
            rows.append(tuple(line.split(sep)))
        return rows

    # --- 事务：一组语句要么全成功要么回滚 --- #
    def transaction(self, statements: list[tuple[str, list]]) -> None:
        if self.use_psycopg:
            with self._psycopg.connect(self.dsn) as conn:  # noqa: SIM117
                with conn.cursor() as cur:
                    for sql, params in statements:
                        cur.execute(sql, list(params) or None)
                conn.commit()
            return
        # psql：拼成单个 BEGIN;...;COMMIT; 一次执行；ON_ERROR_STOP 保证出错回滚。
        body = ";\n".join(self._render(sql, params) for sql, params in statements)
        script = f"BEGIN;\n{body};\nCOMMIT;\n"
        proc = subprocess.run(  # noqa: S603
            [self._psql_bin(), self._psql_conn(), "-v", "ON_ERROR_STOP=1", "-c", script],
            capture_output=True, text=True,
        )
        if proc.returncode != 0:
            raise APIError(f"psql 事务失败（已回滚）: {proc.stderr.strip() or proc.stdout.strip()}")


# --------------------------------------------------------------------------- #
# 邀请码：与后端 repository/affiliate_repo.go 保持一致（12 位，去混淆字符集）。
# --------------------------------------------------------------------------- #
_AFF_CHARSET = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"


def gen_aff_code() -> str:
    return "".join(secrets.choice(_AFF_CHARSET) for _ in range(12))


# --------------------------------------------------------------------------- #
# 业务步骤（HTTP）
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


def fetch_inbox_recharge(inviter_cli: Client) -> list[dict]:
    """从 catchup 拉取 affiliate_recharge namespace 的消息。"""
    data = inviter_cli.get("/inbox/catchup", params={"since": 0})
    msgs = data.get("messages", []) or []
    return [m for m in msgs if m.get("namespace") == "affiliate_recharge"]


# --------------------------------------------------------------------------- #
# 业务步骤（DB）
# --------------------------------------------------------------------------- #
def insert_paid_balance_order(
    db: DB, *, user_id: int, user_email: str, user_name: str, amount: float,
    recharge_code: str, payment_type: str,
) -> int:
    """插入一条 PAID balance 订单，返回新订单 id。列清单依据 payment_orders 的 NOT NULL 约束。"""
    cols = (
        "user_id, user_email, user_name, amount, pay_amount, recharge_code, "
        "payment_type, payment_trade_no, order_type, status, client_ip, src_host, "
        "expires_at, paid_at, created_at, updated_at"
    )
    sql = (
        f"INSERT INTO payment_orders ({cols}) VALUES "
        "(%s, %s, %s, %s, %s, %s, %s, '', 'balance', 'PAID', '', '', "
        "now() + interval '30 minutes', now(), now(), now()) RETURNING id"
    )
    rows = db.query(sql, [user_id, user_email, user_name, amount, amount,
                          recharge_code, payment_type])
    if not rows:
        raise APIError("插入 PAID 订单未返回 id")
    new_id = int(rows[0][0])
    print(f"  [DB] 已插入 PAID 订单 id={new_id}")
    return new_id


def resolve_inviter(db: DB, *, aff_code: str | None, email: str | None,
                    user_id: int | None) -> tuple[int, str]:
    """把 aff_code / email / user_id 解析为 (inviter_id, aff_code)。

    若目标用户尚无 user_affiliates 行（还没生成 aff_code），会即时插入一行并生成随机码。
    """
    if aff_code:
        rows = db.query(
            "SELECT user_id, aff_code FROM user_affiliates WHERE aff_code = %s",
            [aff_code.strip().upper()])
        if not rows:
            raise APIError(f"未找到 aff_code={aff_code} 对应的邀请人。")
        return int(rows[0][0]), str(rows[0][1])

    if user_id is not None:
        uid = int(user_id)
        rows = db.query("SELECT id FROM users WHERE id = %s", [uid])
        if not rows:
            raise APIError(f"未找到 user_id={uid} 的用户。")
    else:
        rows = db.query("SELECT id FROM users WHERE email = %s", [email])
        if not rows:
            raise APIError(f"未找到邮箱 {email} 的用户。")
        uid = int(rows[0][0])

    rows = db.query("SELECT aff_code FROM user_affiliates WHERE user_id = %s", [uid])
    if rows and rows[0][0]:
        return uid, str(rows[0][0])

    # 懒生成：与后端 ensureUserAffiliate 等价（随机码 + ON CONFLICT DO NOTHING）
    for _ in range(12):
        code = gen_aff_code()
        try:
            db.transaction([(
                "INSERT INTO user_affiliates (user_id, aff_code, created_at, updated_at) "
                "VALUES (%s, %s, NOW(), NOW()) ON CONFLICT (user_id) DO NOTHING",
                [uid, code],
            )])
        except APIError:
            continue
        rows = db.query("SELECT aff_code FROM user_affiliates WHERE user_id = %s", [uid])
        if rows and rows[0][0]:
            return uid, str(rows[0][0])
    raise APIError(f"为 user_id={uid} 生成 aff_code 失败。")


def resolve_invitee_ids(db: DB, *, emails: list[str], ids: list[int],
                        inviter_id: int | None, take_all: bool) -> list[int]:
    """把 --invitee-email / --invitee-id / (--inviter-* + --all) 汇总为去重后的 user_id 列表。"""
    result: set[int] = set()
    if ids:
        result.update(int(i) for i in ids)
    if emails:
        rows = db.query("SELECT id, email FROM users WHERE email = ANY(%s)", [list(emails)])
        found = {str(r[1]) for r in rows}
        for r in rows:
            result.add(int(r[0]))
        missing = [e for e in emails if e not in found]
        if missing:
            raise APIError(f"以下邮箱未找到对应用户: {', '.join(missing)}")
    if take_all:
        if inviter_id is None:
            raise APIError("--all 需要配合 --inviter-aff-code/--inviter-email/--inviter-id。")
        rows = db.query(
            "SELECT user_id FROM user_affiliates WHERE inviter_id = %s", [inviter_id])
        for r in rows:
            result.add(int(r[0]))
    return sorted(result)


def delete_users_cascade(db: DB, ids: list[int]) -> None:
    """连账号一起删：清理无外键的孤儿表（payment_orders/balance_ledger/payment_audit_logs），
    再删 users 让 DB 级联清理 user_affiliates / user_affiliate_ledger 等；最后重算受影响邀请人的
    aff_count。整体在一个事务中完成。"""
    if not ids:
        return
    stmts: list[tuple[str, list]] = [
        # payment_audit_logs.order_id 是 varchar，存的是订单 id 的字符串形式
        ("DELETE FROM payment_audit_logs WHERE order_id IN "
         "(SELECT id::text FROM payment_orders WHERE user_id = ANY(%s))", [ids]),
        ("DELETE FROM payment_orders WHERE user_id = ANY(%s)", [ids]),
        ("DELETE FROM balance_ledger WHERE user_id = ANY(%s)", [ids]),
        # 删用户前，把"受影响且不在删除集合内"的邀请人暂存，删后重算其 aff_count
        ("CREATE TEMP TABLE _affected_inv ON COMMIT DROP AS "
         "SELECT DISTINCT inviter_id FROM user_affiliates "
         "WHERE user_id = ANY(%s) AND inviter_id IS NOT NULL "
         "AND NOT (inviter_id = ANY(%s))", [ids, ids]),
        # 级联删除：user_affiliates / user_affiliate_ledger 等外键均为 CASCADE / SET NULL
        ("DELETE FROM users WHERE id = ANY(%s)", [ids]),
        ("UPDATE user_affiliates ua SET "
         "aff_count = (SELECT COUNT(*) FROM user_affiliates x WHERE x.inviter_id = ua.user_id), "
         "updated_at = NOW() "
         "WHERE user_id IN (SELECT inviter_id FROM _affected_inv)", []),
    ]
    db.transaction(stmts)


# --------------------------------------------------------------------------- #
# 子命令：simulate
# --------------------------------------------------------------------------- #
def cmd_simulate(args) -> int:
    db = DB(args.db_dsn, args.db_url)
    suffix = uuid.uuid4().hex[:8]
    inviter_email = f"aff_inviter_{suffix}@example.com"
    invitee_email = f"aff_invitee_{suffix}@example.com"

    admin = Client(args.base_url)

    print(f"[1/7] 管理员登录 {args.admin_email} ...")
    login(admin, args.admin_email, args.admin_password)

    print(f"[2/7] 开启返利总开关 + 关闭邮箱验证码（rebate_rate={args.rebate_rate}%）...")
    ensure_affiliate_settings(admin, args.rebate_rate)

    print(f"[3/7] 注册邀请人 A: {inviter_email} ...")
    a = register(args.base_url, inviter_email, args.password)
    inviter_cli: Client = a["client"]
    inviter_id = a["auth"]["user"]["id"]
    aff_code = get_aff_code(inviter_cli)
    print(f"      A user_id={inviter_id}, aff_code={aff_code}")

    print(f"[4/7] 用 A 的 aff_code 注册被邀请人 B: {invitee_email} ...")
    b = register(args.base_url, invitee_email, args.password, aff_code=aff_code)
    invitee_cli: Client = b["client"]
    invitee_user = b["auth"].get("user") or {}
    invitee_id = invitee_user["id"]
    invitee_name = invitee_user.get("name") or invitee_user.get("username") or invitee_email
    print(f"      B user_id={invitee_id}")

    print(f"[5/7] 直接插入一条 PAID balance 订单 amount={args.amount}（绕过 provider 校验）...")
    recharge_code = f"SIM-{suffix.upper()}"
    order_id = insert_paid_balance_order(
        db, user_id=invitee_id, user_email=invitee_email, user_name=invitee_name,
        amount=args.amount, recharge_code=recharge_code, payment_type=args.payment_type,
    )

    print("[6/7] 管理员 retry 订单，触发到账 + 返利结算 + 信箱通知 ...")
    admin.post(f"/admin/payment/orders/{order_id}/retry")
    time.sleep(1.0)  # 给异步/事务提交一点时间

    print("[7/7] 校验结果 ...")
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
        print("  可能原因：本次充值未产生返利（比例/有效期/上限），或 catchup 首次懒初始化时序问题。")
    expected = round(args.amount * args.rebate_rate / 100, 8)
    print(f"预期返利 ≈ {expected}（比例 {args.rebate_rate}% × 金额 {args.amount}）")
    print("=====================================")
    print(f"\n测试账号（{'保留' if args.keep else '未自动清理'}）：\n  A: {inviter_email}\n  B: {invitee_email}\n  密码: {args.password}")
    return 0


# --------------------------------------------------------------------------- #
# 子命令：add（给现有邀请人增加被邀请人）
# --------------------------------------------------------------------------- #
def cmd_add(args) -> int:
    if args.count < 1:
        raise APIError("--count 必须 >= 1")
    db = DB(args.db_dsn, args.db_url)

    admin = Client(args.base_url)
    print(f"[1/4] 管理员登录 {args.admin_email} ...")
    login(admin, args.admin_email, args.admin_password)

    print("[2/4] 确保返利开启 + 关闭邮箱验证码 ...")
    ensure_affiliate_settings(admin, args.rebate_rate)

    print("[3/4] 解析目标邀请人 ...")
    inviter_id, aff_code = resolve_inviter(
        db, aff_code=args.inviter_aff_code, email=args.inviter_email,
        user_id=args.inviter_id)
    print(f"      邀请人 user_id={inviter_id}, aff_code={aff_code}")

    # 新增被邀请人默认执行充值动作（充值才会产生返利 + 触发邀请人信箱通知，
    # 是本脚本的核心测试目的）；--no-recharge 时仅创建绑定关系不充值。
    recharge = not args.no_recharge
    print(f"[4/4] 注册 {args.count} 个被邀请人"
          f"{'（并各充值一笔触发返利）' if recharge else '（仅绑定，不充值）'} ...")
    created: list[tuple[int, str]] = []
    for i in range(args.count):
        suffix = uuid.uuid4().hex[:8]
        email = f"aff_invitee_{suffix}@example.com"
        b = register(args.base_url, email, args.password, aff_code=aff_code)
        invitee_user = b["auth"].get("user") or {}
        iid = invitee_user["id"]
        name = invitee_user.get("name") or invitee_user.get("username") or email
        created.append((iid, email))
        print(f"  + 被邀请人 #{i + 1}: user_id={iid}, {email}")

        if recharge:
            recharge_code = f"ADD-{suffix.upper()}"
            order_id = insert_paid_balance_order(
                db, user_id=iid, user_email=email, user_name=name,
                amount=args.amount, recharge_code=recharge_code,
                payment_type=args.payment_type)
            admin.post(f"/admin/payment/orders/{order_id}/retry")

    if recharge:
        time.sleep(1.0)

    print("\n================ 结果 ================")
    print(f"邀请人 user_id={inviter_id} aff_code={aff_code} 新增被邀请人 {len(created)} 个:")
    for iid, email in created:
        print(f"  - user_id={iid} {email}")
    if recharge:
        print(f"每个被邀请人已充值 {args.amount}，预期各产生返利 ≈ "
              f"{round(args.amount * args.rebate_rate / 100, 8)}")
    print(f"新账号密码: {args.password}")
    print("=====================================")
    return 0


# --------------------------------------------------------------------------- #
# 子命令：delete（删除被邀请人，连账号）
# --------------------------------------------------------------------------- #
def cmd_delete(args) -> int:
    db = DB(args.db_dsn, args.db_url)

    inviter_id = None
    if args.inviter_aff_code or args.inviter_email or args.inviter_id is not None:
        inviter_id, aff_code = resolve_inviter(
            db, aff_code=args.inviter_aff_code, email=args.inviter_email,
            user_id=args.inviter_id)
        print(f"目标邀请人 user_id={inviter_id}, aff_code={aff_code}")

    ids = resolve_invitee_ids(
        db, emails=args.invitee_email or [], ids=args.invitee_id or [],
        inviter_id=inviter_id, take_all=args.all)
    if not ids:
        raise APIError("未指定要删除的被邀请人：请用 --invitee-email/--invitee-id，"
                       "或 --inviter-* 配合 --all。")

    # 展示将删除的账号
    rows = db.query("SELECT id, COALESCE(email,''), COALESCE(username,'') "
                    "FROM users WHERE id = ANY(%s) ORDER BY id", [ids])
    if not rows:
        raise APIError(f"给定的 user_id 均不存在: {ids}")
    print(f"\n即将删除以下 {len(rows)} 个账号（连同 user_affiliates / ledger / 订单等关联数据）:")
    for r in rows:
        print(f"  - user_id={r[0]} email={r[1]} username={r[2]}")
    existing_ids = [int(r[0]) for r in rows]

    if not args.yes:
        ans = input("\n确认删除？输入 yes 继续：").strip().lower()
        if ans != "yes":
            print("已取消。")
            return 1

    delete_users_cascade(db, existing_ids)
    print(f"\n已删除 {len(existing_ids)} 个账号及其关联数据。")
    if inviter_id is not None and inviter_id not in existing_ids:
        rows = db.query("SELECT aff_count FROM user_affiliates WHERE user_id = %s", [inviter_id])
        if rows:
            print(f"邀请人 user_id={inviter_id} 现有 aff_count={rows[0][0]}")
    return 0


# --------------------------------------------------------------------------- #
# 子命令：recharge（给现有被邀请人充值，不新建账号）
# --------------------------------------------------------------------------- #
def cmd_recharge(args) -> int:
    db = DB(args.db_dsn, args.db_url)

    admin = Client(args.base_url)
    print(f"[1/3] 管理员登录 {args.admin_email} ...")
    login(admin, args.admin_email, args.admin_password)

    print("[2/3] 确保返利开启 ...")
    ensure_affiliate_settings(admin, args.rebate_rate)

    inviter_id = None
    if args.inviter_aff_code or args.inviter_email or args.inviter_id is not None:
        inviter_id, _ = resolve_inviter(
            db, aff_code=args.inviter_aff_code, email=args.inviter_email,
            user_id=args.inviter_id)

    ids = resolve_invitee_ids(
        db, emails=args.invitee_email or [], ids=args.invitee_id or [],
        inviter_id=inviter_id, take_all=args.all)
    if not ids:
        raise APIError("未指定要充值的被邀请人：请用 --invitee-email/--invitee-id，"
                       "或 --inviter-* 配合 --all。")

    # 复用已存在账号（不新建）：从 DB 取其 email/username 用于订单快照字段。
    rows = db.query("SELECT id, COALESCE(email,''), COALESCE(username,'') "
                    "FROM users WHERE id = ANY(%s) ORDER BY id", [ids])
    if not rows:
        raise APIError(f"给定的被邀请人 user_id 均不存在: {ids}")

    print(f"[3/3] 给 {len(rows)} 个现有被邀请人各充值 {args.amount}（不新建账号）...")
    charged: list[tuple[int, str]] = []
    for r in rows:
        iid, email, name = int(r[0]), r[1], (r[2] or r[1])
        suffix = uuid.uuid4().hex[:8]
        recharge_code = f"RCG-{suffix.upper()}"
        order_id = insert_paid_balance_order(
            db, user_id=iid, user_email=email, user_name=name,
            amount=args.amount, recharge_code=recharge_code,
            payment_type=args.payment_type)
        admin.post(f"/admin/payment/orders/{order_id}/retry")
        charged.append((iid, email))
    time.sleep(1.0)

    print("\n================ 结果 ================")
    print(f"已为 {len(charged)} 个被邀请人各充值 {args.amount}:")
    for iid, email in charged:
        print(f"  - user_id={iid} {email}")
    print(f"每笔预期产生返利 ≈ {round(args.amount * args.rebate_rate / 100, 8)}"
          f"（比例 {args.rebate_rate}%）")
    print("=====================================")
    return 0


# --------------------------------------------------------------------------- #
# 参数解析（子命令 + 向后兼容默认 simulate）
# --------------------------------------------------------------------------- #
def build_parser() -> argparse.ArgumentParser:
    db_parent = argparse.ArgumentParser(add_help=False)
    db_parent.add_argument("--db-dsn", default=None,
                           help="Postgres DSN（psycopg 用），例：postgres://user:pass@host:5432/db?sslmode=disable")
    db_parent.add_argument("--db-url", default=None, help="psql 连接串（未装 psycopg 时用）")

    http_parent = argparse.ArgumentParser(add_help=False)
    http_parent.add_argument("--base-url", default="http://127.0.0.1:8080", help="后端服务地址")
    http_parent.add_argument("--admin-email", required=True, help="管理员邮箱")
    http_parent.add_argument("--admin-password", required=True, help="管理员密码")

    ap = argparse.ArgumentParser(description="模拟/管理邀请返利 + 信箱通知")
    sub = ap.add_subparsers(dest="command")

    # simulate
    p_sim = sub.add_parser("simulate", parents=[http_parent, db_parent],
                           help="跑通完整充值返利 + 通知链路（默认）")
    p_sim.add_argument("--amount", type=float, default=100.0, help="充值金额")
    p_sim.add_argument("--rebate-rate", type=float, default=15.0, help="返利比例(%%)")
    p_sim.add_argument("--payment-type", default="alipay", help="订单 payment_type 占位值")
    p_sim.add_argument("--password", default="Passw0rd!", help="新建测试账号的密码")
    p_sim.add_argument("--keep", action="store_true", help="保留测试账号（仅提示，不自动删除）")
    p_sim.set_defaults(func=cmd_simulate)

    # add
    p_add = sub.add_parser("add", parents=[http_parent, db_parent],
                           help="给现有邀请人增加被邀请人")
    g_add = p_add.add_mutually_exclusive_group(required=True)
    g_add.add_argument("--inviter-aff-code", help="邀请人 aff_code")
    g_add.add_argument("--inviter-email", help="邀请人邮箱")
    g_add.add_argument("--inviter-id", type=int, help="邀请人 user_id")
    p_add.add_argument("--count", type=int, default=1, help="新增被邀请人数量")
    p_add.add_argument("--no-recharge", action="store_true",
                       help="仅创建绑定关系，不给新被邀请人充值（默认会充值以触发返利）")
    p_add.add_argument("--amount", type=float, default=100.0, help="每个被邀请人的充值金额")
    p_add.add_argument("--rebate-rate", type=float, default=15.0, help="返利比例(%%)")
    p_add.add_argument("--payment-type", default="alipay", help="订单 payment_type 占位值")
    p_add.add_argument("--password", default="Passw0rd!", help="新建账号密码")
    p_add.set_defaults(func=cmd_add)

    # delete
    p_del = sub.add_parser("delete", parents=[db_parent],
                           help="删除被邀请人（连账号 + 关联数据）")
    p_del.add_argument("--invitee-email", action="append",
                       help="要删除的被邀请人邮箱（可重复指定多个）")
    p_del.add_argument("--invitee-id", action="append", type=int,
                       help="要删除的被邀请人 user_id（可重复指定多个）")
    p_del.add_argument("--inviter-aff-code", help="邀请人 aff_code（配合 --all 或用于校验/重算）")
    p_del.add_argument("--inviter-email", help="邀请人邮箱")
    p_del.add_argument("--inviter-id", type=int, help="邀请人 user_id")
    p_del.add_argument("--all", action="store_true",
                       help="删除指定邀请人名下的全部被邀请人")
    p_del.add_argument("--yes", action="store_true", help="跳过交互确认（危险操作）")
    p_del.set_defaults(func=cmd_delete)

    # recharge
    p_rcg = sub.add_parser("recharge", parents=[http_parent, db_parent],
                           help="给现有被邀请人充值（不新建账号）")
    p_rcg.add_argument("--invitee-email", action="append",
                       help="要充值的被邀请人邮箱（可重复指定多个）")
    p_rcg.add_argument("--invitee-id", action="append", type=int,
                       help="要充值的被邀请人 user_id（可重复指定多个）")
    p_rcg.add_argument("--inviter-aff-code", help="邀请人 aff_code（配合 --all）")
    p_rcg.add_argument("--inviter-email", help="邀请人邮箱（配合 --all）")
    p_rcg.add_argument("--inviter-id", type=int, help="邀请人 user_id（配合 --all）")
    p_rcg.add_argument("--all", action="store_true",
                       help="给指定邀请人名下的全部被邀请人充值")
    p_rcg.add_argument("--amount", type=float, default=100.0, help="每个被邀请人的充值金额")
    p_rcg.add_argument("--rebate-rate", type=float, default=15.0, help="返利比例(%%)")
    p_rcg.add_argument("--payment-type", default="alipay", help="订单 payment_type 占位值")
    p_rcg.set_defaults(func=cmd_recharge)

    return ap


def main() -> int:
    argv = sys.argv[1:]
    # 向后兼容：未显式给子命令时默认 simulate（旧调用首个参数是 --xxx 选项）
    if not argv or argv[0].startswith("-") or argv[0] not in {"simulate", "add", "delete", "recharge"}:
        argv = ["simulate"] + argv

    ap = build_parser()
    args = ap.parse_args(argv)
    return args.func(args)


if __name__ == "__main__":
    try:
        sys.exit(main())
    except APIError as e:
        print(f"\n[失败] {e}", file=sys.stderr)
        sys.exit(1)
    except KeyboardInterrupt:
        sys.exit(130)
