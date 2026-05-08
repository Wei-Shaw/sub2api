#!/bin/bash
#
# deploy/fix-migration-checksums.sh
# Repair schema_migrations.checksum rows when upstream rewrote historical
# migration files (against the "migrations are immutable" principle).
#
# Usage:
#   sudo bash deploy/fix-migration-checksums.sh            # dry-run (report only)
#   sudo bash deploy/fix-migration-checksums.sh --apply    # write the UPDATEs
#
# When to use this
#   After `install-custom.sh upgrade` fails with container logs containing
#   "migration <file> checksum mismatch (db=... file=...)".
#
# How it works
#   1. Reads /opt/sub2api/backend/migrations/*.sql and sha256-hashes each file.
#      (install-custom.sh's upgrade flow runs `git pull` first, so these files
#      are already the same content the new image's embedded FS has.)
#   2. Reads schema_migrations rows from the running PostgreSQL container.
#   3. Reports every row whose on-disk file hash differs from the DB record.
#   4. In --apply mode, issues `UPDATE schema_migrations SET checksum = ...`
#      for each mismatched row.
#
# Safety
#   - Default dry-run prints diffs but does NOT touch the DB.
#   - Orphan migrations (DB rows with no matching file) are reported, never
#     deleted.
#   - New migrations (files with no DB row) are IGNORED — the backend will
#     apply them itself on next startup.
#
# Known caveat
#   This tool is ONLY safe when upstream merely re-formatted/split a historical
#   migration (so the already-applied DDL remains correct). If upstream changed
#   the DDL semantics in a way your DB has not caught up to, updating the
#   checksum will mask a real schema drift. Always read the git diff of the
#   mismatched file before running with --apply.
#

set -euo pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

print_info()    { echo -e "${BLUE}[INFO]${NC} $*"; }
print_success() { echo -e "${GREEN}[ OK ]${NC} $*"; }
print_warning() { echo -e "${YELLOW}[WARN]${NC} $*"; }
print_error()   { echo -e "${RED}[ERR ]${NC} $*" >&2; }

# -----------------------------------------------------------------------------
# Configuration (matches install-custom.sh)
# -----------------------------------------------------------------------------
INSTALL_DIR="/opt/sub2api"
MIGRATIONS_DIR="$INSTALL_DIR/backend/migrations"
ENV_FILE="$INSTALL_DIR/deploy/.env"
POSTGRES_CONTAINER="sub2api-postgres"

APPLY=0
if [ "${1:-}" = "--apply" ]; then
    APPLY=1
fi

# -----------------------------------------------------------------------------
# Pre-flight
# -----------------------------------------------------------------------------
if [ "$(id -u)" -ne 0 ]; then
    print_error "Please run as root (use sudo)"
    exit 1
fi

for cmd in docker sha256sum awk; do
    if ! command -v "$cmd" >/dev/null 2>&1; then
        print_error "Required tool not found: $cmd"
        exit 1
    fi
done

if [ ! -d "$MIGRATIONS_DIR" ]; then
    print_error "Migrations directory not found: $MIGRATIONS_DIR"
    print_error "Make sure install-custom.sh has run and the repo is checked out at $INSTALL_DIR."
    exit 1
fi

if [ ! -f "$ENV_FILE" ]; then
    print_error ".env not found: $ENV_FILE"
    exit 1
fi

if ! docker ps --format '{{.Names}}' | grep -q "^${POSTGRES_CONTAINER}$"; then
    print_error "Postgres container '$POSTGRES_CONTAINER' not running."
    print_error "Start it first: docker compose -p sub2api up -d postgres"
    exit 1
fi

# Read DB credentials from .env
POSTGRES_USER=$(grep '^POSTGRES_USER=' "$ENV_FILE" | cut -d= -f2 | tr -d '"')
POSTGRES_DB=$(grep '^POSTGRES_DB=' "$ENV_FILE" | cut -d= -f2 | tr -d '"')
POSTGRES_USER=${POSTGRES_USER:-sub2api}
POSTGRES_DB=${POSTGRES_DB:-sub2api}

print_info "Using container=$POSTGRES_CONTAINER user=$POSTGRES_USER db=$POSTGRES_DB"
print_info "Scanning migrations in: $MIGRATIONS_DIR"
if [ $APPLY -eq 1 ]; then
    print_warning "Running with --apply: mismatches WILL be written to the database."
else
    print_info "Dry-run mode (default). Add --apply to write changes."
fi
echo ""

# -----------------------------------------------------------------------------
# Collect DB checksums
# -----------------------------------------------------------------------------
# Output format: filename<TAB>db_checksum (one row per line)
DB_ROWS=$(docker exec -e PGPASSWORD="$(grep '^POSTGRES_PASSWORD=' "$ENV_FILE" | cut -d= -f2 | tr -d '"')" \
    "$POSTGRES_CONTAINER" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -At -F $'\t' \
    -c "SELECT filename, checksum FROM schema_migrations ORDER BY filename;")

if [ -z "$DB_ROWS" ]; then
    print_error "schema_migrations table is empty or unreadable."
    exit 1
fi

# -----------------------------------------------------------------------------
# Compare against files on disk
# -----------------------------------------------------------------------------
MISMATCHED=()
ORPHANS=()
MATCHED_COUNT=0

while IFS=$'\t' read -r filename db_checksum; do
    [ -z "$filename" ] && continue
    file_path="$MIGRATIONS_DIR/$filename"
    if [ ! -f "$file_path" ]; then
        ORPHANS+=("$filename")
        continue
    fi
    file_checksum=$(sha256sum "$file_path" | awk '{print $1}')
    if [ "$db_checksum" = "$file_checksum" ]; then
        MATCHED_COUNT=$((MATCHED_COUNT + 1))
    else
        MISMATCHED+=("${filename}|${db_checksum}|${file_checksum}")
    fi
done <<< "$DB_ROWS"

# -----------------------------------------------------------------------------
# Report
# -----------------------------------------------------------------------------
print_info "Summary:"
printf "  matched:    %d\n" "$MATCHED_COUNT"
printf "  mismatched: %d\n" "${#MISMATCHED[@]}"
printf "  orphans:    %d (DB rows with no matching file, left untouched)\n" "${#ORPHANS[@]}"
echo ""

if [ ${#ORPHANS[@]} -gt 0 ]; then
    print_warning "Orphan migrations (DB has the row but the file is gone):"
    for f in "${ORPHANS[@]}"; do
        echo "  - $f"
    done
    echo ""
fi

if [ ${#MISMATCHED[@]} -eq 0 ]; then
    print_success "No checksum mismatches. schema_migrations is in sync with the files."
    exit 0
fi

print_warning "Mismatched migrations:"
for entry in "${MISMATCHED[@]}"; do
    IFS='|' read -r filename db_cs file_cs <<< "$entry"
    echo ""
    echo "  file: $filename"
    echo "    db checksum:   ${db_cs:0:16}..."
    echo "    file checksum: ${file_cs:0:16}..."
    # Show the most recent commit that touched this migration file (may hint at
    # whether upstream just re-formatted or actually changed DDL).
    if [ -d "$INSTALL_DIR/.git" ]; then
        last_commit=$(git -C "$INSTALL_DIR" log -1 --pretty=format:'%h %s' -- "backend/migrations/$filename" 2>/dev/null || echo "")
        if [ -n "$last_commit" ]; then
            echo "    last commit:   $last_commit"
        fi
    fi
done
echo ""

if [ $APPLY -eq 0 ]; then
    print_info "To write these checksums to the database, re-run with --apply:"
    print_info "  sudo bash $0 --apply"
    print_info "Before doing that, inspect each file's git history:"
    print_info "  cd $INSTALL_DIR && git log -p -- backend/migrations/<file>"
    exit 0
fi

# -----------------------------------------------------------------------------
# Apply
# -----------------------------------------------------------------------------
print_info "Applying ${#MISMATCHED[@]} checksum updates..."
for entry in "${MISMATCHED[@]}"; do
    IFS='|' read -r filename db_cs file_cs <<< "$entry"
    # Use parameterized-ish approach: docker exec reads stdin, no shell interpolation of filename
    updated=$(docker exec -i -e PGPASSWORD="$(grep '^POSTGRES_PASSWORD=' "$ENV_FILE" | cut -d= -f2 | tr -d '"')" \
        "$POSTGRES_CONTAINER" \
        psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -At \
        -c "UPDATE schema_migrations SET checksum = '$file_cs' WHERE filename = '$filename' AND checksum = '$db_cs';" 2>&1 | tail -1)
    if [ "$updated" = "UPDATE 1" ]; then
        echo "  ✓ $filename"
    else
        print_warning "  unexpected result for $filename: $updated"
    fi
done

echo ""
print_success "Done. Retry the upgrade:"
echo "  curl -sSL https://raw.githubusercontent.com/qiangweihewu/sub2api/i18n-seo/deploy/install-custom.sh \\"
echo "    | sudo bash -s -- upgrade"
