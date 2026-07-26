#!/usr/bin/env python3
"""Backfill root-style public account IDs for legacy Sub2API users."""

from __future__ import annotations

import argparse
import os
import secrets
import sys
from dataclasses import dataclass


def generate_root_id() -> str:
    return str(secrets.randbelow(9) + 1) + "".join(str(secrets.randbelow(10)) for _ in range(15))


@dataclass
class Counts:
    scanned: int = 0
    populated: int = 0
    skipped: int = 0
    collisions: int = 0
    invalid_partial: int = 0
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
        candidate = generate_root_id()
        cursor.execute("SELECT 1 FROM users WHERE external_user_id = %s", (candidate,))
        if cursor.fetchone() is None:
            return candidate
        counts.collisions += 1
    raise RuntimeError("public account ID collision retry limit exhausted")


def run(dsn: str, batch_size: int, start_after: int, retries: int, dry_run: bool) -> Counts:
    counts = Counts()
    connection = connect(dsn)
    try:
        cursor = connection.cursor()
        cursor.execute(
            "SELECT COUNT(*) FROM users WHERE (account_id IS NULL) <> (external_user_id IS NULL)"
        )
        counts.invalid_partial = int(cursor.fetchone()[0])
        if counts.invalid_partial:
            raise RuntimeError("partially populated public IDs found; repair them before backfill")
        cursor_id = start_after
        while True:
            cursor.execute(
                """
                SELECT id FROM users
                WHERE id > %s AND account_id IS NULL AND external_user_id IS NULL
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
                cursor.execute(
                    """
                    UPDATE users SET account_id = %s, external_user_id = %s, identity_type = 'root'
                    WHERE id = %s AND account_id IS NULL AND external_user_id IS NULL
                    """,
                    (account_id, account_id, user_id),
                )
                if cursor.rowcount == 1:
                    counts.populated += 1
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
        f"scanned={counts.scanned} populated={counts.populated} skipped={counts.skipped} "
        f"collision_retries={counts.collisions} invalid_partial={counts.invalid_partial} dry_run={args.dry_run}"
        f" last_cursor={counts.last_cursor}"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
