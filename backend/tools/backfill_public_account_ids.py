#!/usr/bin/env python3
"""Backfill unique 16-digit account_id values for legacy users.

IAM users historically reused the organization owner's account_id. This script
assigns fresh identifiers only to shared IAM rows and users missing an account_id.
It intentionally refuses duplicate root accounts because those require manual
review of organization and billing ownership.
"""

from __future__ import annotations

import argparse
import os
import secrets
import sys
from dataclasses import dataclass


def generate_account_id() -> str:
    return str(secrets.randbelow(9) + 1) + "".join(str(secrets.randbelow(10)) for _ in range(15))


@dataclass
class Counts:
    scanned: int = 0
    reassigned: int = 0
    skipped: int = 0
    collisions: int = 0
    duplicate_root_accounts: int = 0
    last_cursor: int = 0


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


def next_unique_id(cursor, retries: int, counts: Counts) -> str:
    for _ in range(retries):
        candidate = generate_account_id()
        cursor.execute("SELECT 1 FROM users WHERE account_id = %s", (candidate,))
        if cursor.fetchone() is None:
            return candidate
        counts.collisions += 1
    raise RuntimeError("account_id collision retry limit exhausted")


def run(dsn: str, batch_size: int, start_after: int, retries: int, dry_run: bool) -> Counts:
    counts = Counts()
    connection = connect(dsn)
    try:
        cursor = connection.cursor()
        cursor.execute(
            """
            SELECT COUNT(*) FROM (
                SELECT account_id
                FROM users
                WHERE account_id IS NOT NULL AND identity_type = 'root'
                GROUP BY account_id
                HAVING COUNT(*) > 1
            ) duplicate_roots
            """
        )
        counts.duplicate_root_accounts = int(cursor.fetchone()[0])
        if counts.duplicate_root_accounts:
            raise RuntimeError("duplicate root account_id values found; resolve them before backfill")

        cursor_id = start_after
        while True:
            cursor.execute(
                """
                SELECT id FROM users
                WHERE id > %s
                  AND (
                      account_id IS NULL
                      OR (
                          identity_type = 'iam'
                          AND EXISTS (
                              SELECT 1 FROM users other
                              WHERE other.account_id = users.account_id
                                AND other.id <> users.id
                          )
                      )
                  )
                ORDER BY id LIMIT %s
                FOR UPDATE SKIP LOCKED
                """,
                (cursor_id, batch_size),
            )
            rows = cursor.fetchall()
            if not rows:
                connection.rollback()
                break
            for (user_id,) in rows:
                counts.scanned += 1
                cursor_id = int(user_id)
                counts.last_cursor = cursor_id
                if dry_run:
                    counts.skipped += 1
                    continue
                account_id = next_unique_id(cursor, retries, counts)
                cursor.execute("UPDATE users SET account_id = %s WHERE id = %s", (account_id, user_id))
                if cursor.rowcount == 1:
                    counts.reassigned += 1
                else:
                    counts.skipped += 1
            if dry_run:
                connection.rollback()
            else:
                connection.commit()
        return counts
    finally:
        connection.close()


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--dsn", default=os.environ.get("DATABASE_URL", ""))
    parser.add_argument("--batch-size", type=int, default=500)
    parser.add_argument("--start-after", type=int, default=0)
    parser.add_argument("--collision-retries", type=int, default=20)
    parser.add_argument("--dry-run", action="store_true")
    args = parser.parse_args()
    if not args.dsn or args.batch_size < 1 or args.collision_retries < 1:
        parser.error("a DSN and positive batch/retry values are required")
    try:
        counts = run(args.dsn, args.batch_size, args.start_after, args.collision_retries, args.dry_run)
    except Exception as exc:
        print(f"backfill failed: {exc}", file=sys.stderr)
        return 1
    print(
        f"scanned={counts.scanned} reassigned={counts.reassigned} skipped={counts.skipped} "
        f"collision_retries={counts.collisions} duplicate_root_accounts={counts.duplicate_root_accounts} "
        f"dry_run={args.dry_run} last_cursor={counts.last_cursor}"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
