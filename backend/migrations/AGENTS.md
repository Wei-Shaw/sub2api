# MIGRATIONS KNOWLEDGE BASE

## OVERVIEW
`backend/migrations/` contains forward-only SQL migrations executed by the backend’s custom runner.
This directory is high-risk because correctness depends on filename order, checksum immutability, and special handling for `_notx.sql` files.

## STRUCTURE
```text
backend/migrations/
├── NNN_description.sql        # Transactional migrations
├── NNN_description_notx.sql   # Non-transactional concurrent index migrations
└── README.md                  # Full migration workflow and rules
```

## WHERE TO LOOK
| Task | Location | Notes |
|------|----------|-------|
| Migration rules | `README.md` | Naming, immutability, `_notx.sql`, troubleshooting |
| Runtime behavior | `../internal/repository/migrations_runner.go` | Checksums, advisory lock, execution mode |
| Applied migration state | `schema_migrations` table | Stores filename + checksum |

## CONVENTIONS
- Name files as `NNN_description.sql` with zero-padded numeric prefixes.
- Keep migrations forward-only; if you need to revert behavior, add a new migration.
- `_notx.sql` is only for statements that must run outside a transaction, especially `CREATE/DROP INDEX CONCURRENTLY`.
- `_notx.sql` content must be idempotent: use `IF NOT EXISTS` / `IF EXISTS`.
- Files execute in lexicographic order, not by timestamp or folder.

## ANTI-PATTERNS
- Editing a migration that may already have run in any environment.
- Renaming, deleting, or reordering existing migration files.
- Mixing regular DDL/DML or transaction control statements into `_notx.sql` files.
- Assuming goose-style Up/Down parsing exists; the runner executes the file content as-is.

## COMMANDS
```bash
psql -d sub2api -c "SELECT * FROM schema_migrations ORDER BY applied_at DESC;"
git log --oneline -- backend/migrations/NNN_description.sql
```

## NOTES
- The application auto-runs migrations on startup; there is no separate migration binary here.
- Recovery from checksum mismatch is a Git/history problem first, not a SQL-edit-in-place problem.
- If a schema change starts in `backend/ent/schema/`, add the migration here in the same change set.
- Read `README.md` before touching `_notx.sql`; it contains repo-specific execution semantics.
