#!/usr/bin/env python3
"""Purge an organization (company account) and all of its related data.

This turns the owner and every member back into ordinary personal accounts.

Organization identity is NOT a flag on the users table; a user is treated as a
company account purely because rows exist in organizations / organization_
memberships and related tables. Deleting those rows restores personal status.

All organization foreign keys are RESTRICT (Postgres default), so rows must be
removed bottom-up. The billing snapshot columns added in migration 189
(usage_logs / balance_ledger / async_media_tasks / batch_image_jobs) have no
foreign key, so their organization association is cleared with UPDATE to avoid
leaving dangling ids in historical records.

The tool defaults to a dry-run. Pass --execute to actually delete. Deletion runs
inside a single transaction and is atomic.

It does NOT refund the upgrade fee and does NOT reclaim member balances; member
balances stay on their own accounts. Handle refunds separately if required.

Usage:

    purge_organization.py --account-id <id> [--execute]
    purge_organization.py --owner-email <email> [--execute]

The database DSN is read from --dsn or the DATABASE_URL environment variable.
"""

from __future__ import annotations

import argparse
import os
import sys
from dataclasses import dataclass, field


@dataclass
class Organization:
    id: int
    account_id: str
    name: str
    status: str
    owner_user_id: int


@dataclass
class Member:
    user_id: int
    role: str
    status: str


@dataclass
class PurgeStep:
    """One table cleanup.

    count_query returns how many rows exec_query would affect; both take the
    same named parameters.
    """

    label: str
    count_query: str
    exec_query: str
    params: dict = field(default_factory=dict)


def connect(dsn: str):
    try:
        import psycopg  # type: ignore

        return psycopg.connect(dsn)
    except ImportError:
        try:
            import psycopg2  # type: ignore

            return psycopg2.connect(dsn)
        except ImportError as exc:
            raise RuntimeError("install psycopg or psycopg2 to run this script") from exc


def list_organizations(cursor) -> None:
    cursor.execute(
        """
        SELECT o.id, o.account_id, o.status, o.owner_user_id,
               COALESCE(u.email, '(none)') AS owner_email, o.name
        FROM organizations o
        LEFT JOIN users u ON u.id = o.owner_user_id
        ORDER BY o.id
        """
    )
    rows = cursor.fetchall()
    if not rows:
        print("no organizations found in this database")
        return
    print(f"found {len(rows)} organization(s):")
    for r in rows:
        print(
            f"  id={r[0]} account_id={r[1]} status={r[2]} "
            f'owner_user_id={r[3]} owner_email={r[4]} name="{r[5]}"'
        )


def resolve_organization(cursor, account_id: str, owner_email: str) -> Organization:
    if account_id:
        cursor.execute(
            "SELECT id, account_id, name, status, owner_user_id "
            "FROM organizations WHERE account_id = %(account_id)s",
            {"account_id": account_id},
        )
    else:
        cursor.execute(
            """
            SELECT o.id, o.account_id, o.name, o.status, o.owner_user_id
            FROM organizations o
            JOIN users u ON u.id = o.owner_user_id
            WHERE lower(u.email) = lower(%(owner_email)s)
            """,
            {"owner_email": owner_email},
        )
    row = cursor.fetchone()
    if row is None:
        raise RuntimeError(
            "no organization found for the given selector "
            "(note: --account-id is the owner's 16-digit public account_id, "
            "not the numeric primary key; run with --list to see valid selectors)"
        )
    return Organization(
        id=int(row[0]),
        account_id=row[1],
        name=row[2],
        status=row[3],
        owner_user_id=int(row[4]),
    )


def load_members(cursor, org_id: int) -> list[Member]:
    cursor.execute(
        "SELECT user_id, role, status FROM organization_memberships "
        "WHERE organization_id = %(org_id)s ORDER BY role DESC, user_id",
        {"org_id": org_id},
    )
    return [Member(user_id=int(r[0]), role=r[1], status=r[2]) for r in cursor.fetchall()]


def build_steps(org_id: int) -> list[PurgeStep]:
    """Return the cleanup steps in strict bottom-up dependency order so that
    RESTRICT foreign keys are never violated."""
    one = {"org_id": org_id}
    # financial ledger may reference upgrade applications of this org via
    # application_id while its own organization_id is still NULL (e.g. the
    # upgrade_reserve phase). Clear those too, and do it before deleting the
    # applications they point at.
    ledger_where = (
        "organization_id = %(org_id)s\n"
        "        OR application_id IN "
        "(SELECT id FROM company_upgrade_applications WHERE organization_id = %(org_id)s)"
    )

    return [
        # 1. Clear billing snapshot associations (no FK; keep the historical rows).
        PurgeStep(
            "usage_logs (clear org association)",
            "SELECT COUNT(*) FROM usage_logs WHERE organization_id = %(org_id)s",
            "UPDATE usage_logs SET organization_id = NULL, payer_user_id = NULL, "
            "balance_source = NULL, authz_generation = NULL WHERE organization_id = %(org_id)s",
            one,
        ),
        PurgeStep(
            "balance_ledger (clear org association)",
            "SELECT COUNT(*) FROM balance_ledger WHERE organization_id = %(org_id)s",
            "UPDATE balance_ledger SET organization_id = NULL, payer_user_id = NULL, "
            "balance_source = NULL, authz_generation = NULL WHERE organization_id = %(org_id)s",
            one,
        ),
        PurgeStep(
            "async_media_tasks (clear org association)",
            "SELECT COUNT(*) FROM async_media_tasks WHERE organization_id = %(org_id)s",
            "UPDATE async_media_tasks SET organization_id = NULL, payer_user_id = NULL, "
            "balance_source = NULL, authz_generation = NULL WHERE organization_id = %(org_id)s",
            one,
        ),
        PurgeStep(
            "batch_image_jobs (clear org association)",
            "SELECT COUNT(*) FROM batch_image_jobs WHERE organization_id = %(org_id)s",
            "UPDATE batch_image_jobs SET organization_id = NULL, payer_user_id = NULL, "
            "balance_source = NULL, authz_generation = NULL WHERE organization_id = %(org_id)s",
            one,
        ),
        # 2. Policy attachments (reference organizations + memberships).
        PurgeStep(
            "member_policy_attachments",
            "SELECT COUNT(*) FROM member_policy_attachments WHERE organization_id = %(org_id)s",
            "DELETE FROM member_policy_attachments WHERE organization_id = %(org_id)s",
            one,
        ),
        # 3. Financial ledger (references organizations + applications).
        PurgeStep(
            "organization_financial_ledger",
            "SELECT COUNT(*) FROM organization_financial_ledger WHERE " + ledger_where,
            "DELETE FROM organization_financial_ledger WHERE " + ledger_where,
            one,
        ),
        # 4. Audit events.
        PurgeStep(
            "organization_audit_events",
            "SELECT COUNT(*) FROM organization_audit_events WHERE organization_id = %(org_id)s",
            "DELETE FROM organization_audit_events WHERE organization_id = %(org_id)s",
            one,
        ),
        # 5. Name change requests.
        PurgeStep(
            "organization_name_change_requests",
            "SELECT COUNT(*) FROM organization_name_change_requests WHERE organization_id = %(org_id)s",
            "DELETE FROM organization_name_change_requests WHERE organization_id = %(org_id)s",
            one,
        ),
        # 6. Upgrade applications tied to this organization.
        PurgeStep(
            "company_upgrade_applications",
            "SELECT COUNT(*) FROM company_upgrade_applications WHERE organization_id = %(org_id)s",
            "DELETE FROM company_upgrade_applications WHERE organization_id = %(org_id)s",
            one,
        ),
        # 7. Memberships.
        PurgeStep(
            "organization_memberships",
            "SELECT COUNT(*) FROM organization_memberships WHERE organization_id = %(org_id)s",
            "DELETE FROM organization_memberships WHERE organization_id = %(org_id)s",
            one,
        ),
        # 8. The organization row itself.
        PurgeStep(
            "organizations",
            "SELECT COUNT(*) FROM organizations WHERE id = %(org_id)s",
            "DELETE FROM organizations WHERE id = %(org_id)s",
            one,
        ),
    ]


def run_dry_run(cursor, steps: list[PurgeStep]) -> None:
    print("mode=dry-run (no changes applied; re-run with --execute to delete)")
    total = 0
    for step in steps:
        cursor.execute(step.count_query, step.params)
        n = int(cursor.fetchone()[0])
        print(f"  {step.label:<40} rows={n}")
        total += n
    print(f"total affected rows (would be)={total}")


def run_purge(connection, cursor, steps: list[PurgeStep]) -> None:
    try:
        for step in steps:
            cursor.execute(step.exec_query, step.params)
            print(f"  {step.label:<40} affected={cursor.rowcount}")
        connection.commit()
    except Exception:
        connection.rollback()
        raise


def run(dsn: str, account_id: str, owner_email: str, execute: bool, list_only: bool) -> int:
    connection = connect(dsn)
    try:
        cursor = connection.cursor()
        if list_only:
            list_organizations(cursor)
            connection.rollback()
            return 0
        org = resolve_organization(cursor, account_id, owner_email)
        members = load_members(cursor, org.id)

        print(
            f"organization id={org.id} account_id={org.account_id} "
            f'name="{org.name}" status={org.status} owner_user_id={org.owner_user_id}'
        )
        print(f"members={len(members)}")
        for m in members:
            print(f"  member user_id={m.user_id} role={m.role} status={m.status}")

        steps = build_steps(org.id)

        if not execute:
            run_dry_run(cursor, steps)
            connection.rollback()
            return 0

        print("mode=execute")
        run_purge(connection, cursor, steps)
        print(
            f"done: organization id={org.id} (account_id={org.account_id}) purged; "
            f"{len(members)} member(s) restored to personal accounts"
        )
        return 0
    finally:
        connection.close()


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--account-id", default="", help="organization account_id to purge")
    parser.add_argument(
        "--owner-email", default="", help="owner user email (alternative to --account-id)"
    )
    parser.add_argument(
        "--dsn",
        default=os.environ.get("DATABASE_URL", ""),
        help="Postgres DSN (defaults to DATABASE_URL)",
    )
    parser.add_argument(
        "--execute", action="store_true", help="perform deletion (default is dry-run)"
    )
    parser.add_argument(
        "--list",
        dest="list_only",
        action="store_true",
        help="list all organizations with their selectors, then exit",
    )
    args = parser.parse_args()

    if not args.dsn:
        parser.error("a DSN is required (pass --dsn or set DATABASE_URL)")
    if not args.list_only and bool(args.account_id) == bool(args.owner_email):
        parser.error("exactly one of --account-id or --owner-email is required")

    try:
        return run(args.dsn, args.account_id, args.owner_email, args.execute, args.list_only)
    except Exception as exc:
        print(f"purge failed (transaction rolled back): {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
