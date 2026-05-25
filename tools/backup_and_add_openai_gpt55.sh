#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CONFIG_FILE="${CONFIG_FILE:-${ROOT_DIR}/backend/config.yaml}"
BACKUP_DIR="${BACKUP_DIR:-${ROOT_DIR}/backups/db}"
TIMESTAMP="$(date +%Y%m%d-%H%M%S)"

REMOTE_HOST="${REMOTE_HOST:-}"
REMOTE_PORT="${REMOTE_PORT:-22}"
REMOTE_USER="${REMOTE_USER:-}"
REMOTE_PASS="${REMOTE_PASS:-}"
REMOTE_APP_DIR="${REMOTE_APP_DIR:-/opt/sub2api}"
REMOTE_CONFIG_FILE="${REMOTE_CONFIG_FILE:-${REMOTE_APP_DIR}/config.yaml}"

MODE="${1:-}"

log() {
  printf '[%s] %s\n' "$(date '+%F %T')" "$*"
}

usage() {
  cat <<'EOF'
Usage:
  ./tools/backup_and_add_openai_gpt55.sh --local
  ./tools/backup_and_add_openai_gpt55.sh --remote-prod

Behavior:
  --local
    Backup the local database referenced by backend/config.yaml, then add
    gpt-5.5 to every persisted OpenAI account model_mapping that lacks it.

  --remote-prod
    Connect to the remote host, create a database dump there, copy that dump
    back to the local backups/db directory, then update the remote database in
    place. Requires REMOTE_HOST, REMOTE_USER, and REMOTE_PASS in the environment.
EOF
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "Missing required command: $1" >&2
    exit 1
  }
}

yaml_db_value_from_file() {
  local file="$1"
  local target="$2"
  awk -v target="${target}" '
    /^database:[[:space:]]*$/ { in_db=1; next }
    in_db && /^[^[:space:]]/ { exit }
    in_db {
      key = $1
      sub(/:.*/, "", key)
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", key)
      if (key != target) {
        next
      }

      value = $0
      sub(/^[^:]*:[[:space:]]*/, "", value)
      sub(/[[:space:]]+#.*/, "", value)
      gsub(/^"|"$/, "", value)
      gsub(/^'\''|'\''$/, "", value)
      print value
      exit
    }
  ' "${file}"
}

read -r -d '' UPDATE_SQL <<'SQL' || true
WITH target_accounts AS (
  SELECT id, name
  FROM accounts
  WHERE deleted_at IS NULL
    AND platform = 'openai'
    AND jsonb_typeof(credentials) = 'object'
    AND jsonb_typeof(credentials->'model_mapping') = 'object'
    AND NOT (credentials->'model_mapping' ? 'gpt-5.5')
),
updated AS (
  UPDATE accounts AS a
  SET credentials = jsonb_set(
        a.credentials,
        '{model_mapping,gpt-5.5}',
        to_jsonb('gpt-5.5'::text),
        true
      ),
      updated_at = NOW()
  FROM target_accounts t
  WHERE a.id = t.id
  RETURNING a.id, a.name
)
SELECT COUNT(*) AS updated_count FROM updated;
SQL

run_local() {
  require_cmd pg_dump
  require_cmd psql

  if [[ ! -f "${CONFIG_FILE}" ]]; then
    echo "Config file not found: ${CONFIG_FILE}" >&2
    exit 1
  fi

  local db_host db_port db_user db_password db_name db_sslmode backup_file updated_count
  db_host="${DB_HOST:-$(yaml_db_value_from_file "${CONFIG_FILE}" host)}"
  db_port="${DB_PORT:-$(yaml_db_value_from_file "${CONFIG_FILE}" port)}"
  db_user="${DB_USER:-$(yaml_db_value_from_file "${CONFIG_FILE}" user)}"
  db_password="${DB_PASSWORD:-$(yaml_db_value_from_file "${CONFIG_FILE}" password)}"
  db_name="${DB_NAME:-$(yaml_db_value_from_file "${CONFIG_FILE}" dbname)}"
  db_sslmode="${DB_SSLMODE:-$(yaml_db_value_from_file "${CONFIG_FILE}" sslmode)}"

  db_host="${db_host:-127.0.0.1}"
  db_port="${db_port:-5432}"
  db_sslmode="${db_sslmode:-disable}"

  if [[ -z "${db_user}" || -z "${db_name}" ]]; then
    echo "Incomplete database config in ${CONFIG_FILE}" >&2
    exit 1
  fi

  mkdir -p "${BACKUP_DIR}"
  backup_file="${BACKUP_DIR}/sub2api-before-openai-gpt55-local-${TIMESTAMP}.dump"

  log "Backing up local database to ${backup_file}"
  env PGPASSWORD="${db_password}" PGSSLMODE="${db_sslmode}" \
    pg_dump --format=custom --no-owner --no-privileges \
    -h "${db_host}" -p "${db_port}" -U "${db_user}" -d "${db_name}" \
    -f "${backup_file}"

  log "Applying local database update"
  updated_count="$(
    env PGPASSWORD="${db_password}" PGSSLMODE="${db_sslmode}" \
      psql -h "${db_host}" -p "${db_port}" -U "${db_user}" -d "${db_name}" \
      -v ON_ERROR_STOP=1 -t -A -c "${UPDATE_SQL}"
  )"
  updated_count="$(printf '%s' "${updated_count}" | tr -d '[:space:]')"

  log "Local backup complete: ${backup_file}"
  log "Local update complete: ${updated_count:-0} account(s) updated"
}

ssh_expect() {
  local remote_cmd="$1"
  EXPECT_HOST="${REMOTE_HOST}" \
  EXPECT_PORT="${REMOTE_PORT}" \
  EXPECT_USER="${REMOTE_USER}" \
  EXPECT_PASS="${REMOTE_PASS}" \
  EXPECT_CMD="${remote_cmd}" \
  expect <<'EOF'
set timeout -1
set host $env(EXPECT_HOST)
set port $env(EXPECT_PORT)
set user $env(EXPECT_USER)
set pass $env(EXPECT_PASS)
set cmd  $env(EXPECT_CMD)
spawn ssh -p $port -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null $user@$host $cmd
expect {
  -re "(?i)password:" {
    send -- "$pass\r"
    exp_continue
  }
  eof {
    catch wait result
    set code [lindex $result 3]
    exit $code
  }
}
EOF
}

scp_expect_remote_to_local() {
  local remote_path="$1"
  local local_path="$2"
  EXPECT_HOST="${REMOTE_HOST}" \
  EXPECT_PORT="${REMOTE_PORT}" \
  EXPECT_USER="${REMOTE_USER}" \
  EXPECT_PASS="${REMOTE_PASS}" \
  EXPECT_REMOTE_PATH="${remote_path}" \
  EXPECT_LOCAL_PATH="${local_path}" \
  expect <<'EOF'
set timeout -1
set host   $env(EXPECT_HOST)
set port   $env(EXPECT_PORT)
set user   $env(EXPECT_USER)
set pass   $env(EXPECT_PASS)
set source $env(EXPECT_REMOTE_PATH)
set target $env(EXPECT_LOCAL_PATH)
spawn scp -P $port -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null $user@$host:$source $target
expect {
  -re "(?i)password:" {
    send -- "$pass\r"
    exp_continue
  }
  eof {
    catch wait result
    set code [lindex $result 3]
    exit $code
  }
}
EOF
}

scp_expect_local_to_remote() {
  local local_path="$1"
  local remote_path="$2"
  EXPECT_HOST="${REMOTE_HOST}" \
  EXPECT_PORT="${REMOTE_PORT}" \
  EXPECT_USER="${REMOTE_USER}" \
  EXPECT_PASS="${REMOTE_PASS}" \
  EXPECT_LOCAL_PATH="${local_path}" \
  EXPECT_REMOTE_PATH="${remote_path}" \
  expect <<'EOF'
set timeout -1
set host   $env(EXPECT_HOST)
set port   $env(EXPECT_PORT)
set user   $env(EXPECT_USER)
set pass   $env(EXPECT_PASS)
set source $env(EXPECT_LOCAL_PATH)
set target $env(EXPECT_REMOTE_PATH)
spawn scp -P $port -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null $source $user@$host:$target
expect {
  -re "(?i)password:" {
    send -- "$pass\r"
    exp_continue
  }
  eof {
    catch wait result
    set code [lindex $result 3]
    exit $code
  }
}
EOF
}

run_remote_prod() {
  require_cmd expect
  require_cmd scp

  if [[ -z "${REMOTE_HOST}" ]]; then
    echo "REMOTE_HOST is required for --remote-prod" >&2
    exit 1
  fi
  if [[ -z "${REMOTE_USER}" ]]; then
    echo "REMOTE_USER is required for --remote-prod" >&2
    exit 1
  fi
  if [[ -z "${REMOTE_PASS}" ]]; then
    echo "REMOTE_PASS is required for --remote-prod" >&2
    exit 1
  fi

  mkdir -p "${BACKUP_DIR}"

  local remote_dump_path local_dump_path remote_helper_path local_helper_path updated_count
  remote_dump_path="/tmp/sub2api-before-openai-gpt55-remote-${TIMESTAMP}.dump"
  local_dump_path="${BACKUP_DIR}/sub2api-before-openai-gpt55-remote-${TIMESTAMP}.dump"
  remote_helper_path="/tmp/sub2api-openai-gpt55-helper-${TIMESTAMP}.sh"
  local_helper_path="${BACKUP_DIR}/.sub2api-openai-gpt55-helper-${TIMESTAMP}.sh"

  cat > "${local_helper_path}" <<EOF
#!/usr/bin/env bash
set -euo pipefail

CONFIG_FILE="\${1:?missing config file}"
MODE="\${2:?missing mode}"
PAYLOAD_FILE="\${3:-}"
DUMP_FILE="\${4:-}"

yaml_db_value() {
  local target="\$1"
  awk -v target="\$target" '
    /^database:[[:space:]]*$/ { in_db=1; next }
    in_db && /^[^[:space:]]/ { exit }
    in_db {
      key = \$1
      sub(/:.*/, "", key)
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", key)
      if (key != target) next
      value = \$0
      sub(/^[^:]*:[[:space:]]*/, "", value)
      sub(/[[:space:]]+#.*/, "", value)
      gsub(/^"|"$/, "", value)
      gsub(/^'\''|'\''$/, "", value)
      print value
      exit
    }
  ' "\${CONFIG_FILE}"
}

db_host="\${DB_HOST:-\$(yaml_db_value host)}"
db_port="\${DB_PORT:-\$(yaml_db_value port)}"
db_user="\${DB_USER:-\$(yaml_db_value user)}"
db_password="\${DB_PASSWORD:-\$(yaml_db_value password)}"
db_name="\${DB_NAME:-\$(yaml_db_value dbname)}"
db_sslmode="\${DB_SSLMODE:-\$(yaml_db_value sslmode)}"
db_host="\${db_host:-127.0.0.1}"
db_port="\${db_port:-5432}"
db_sslmode="\${db_sslmode:-disable}"

case "\${MODE}" in
  dump)
    env PGPASSWORD="\${db_password}" PGSSLMODE="\${db_sslmode}" \
      pg_dump --format=custom --no-owner --no-privileges \
      -h "\${db_host}" -p "\${db_port}" -U "\${db_user}" -d "\${db_name}" \
      -f "\${DUMP_FILE}"
    ;;
  update)
    env PGPASSWORD="\${db_password}" PGSSLMODE="\${db_sslmode}" \
      psql -h "\${db_host}" -p "\${db_port}" -U "\${db_user}" -d "\${db_name}" \
      -v ON_ERROR_STOP=1 -t -A -f "\${PAYLOAD_FILE}"
    ;;
  *)
    echo "unknown mode: \${MODE}" >&2
    exit 1
    ;;
esac
EOF

  chmod +x "${local_helper_path}"

  log "Backing up production database on ${REMOTE_HOST} to ${remote_dump_path}"
  scp_expect_local_to_remote "${local_helper_path}" "${remote_helper_path}"
  ssh_expect "bash '${remote_helper_path}' '${REMOTE_CONFIG_FILE}' dump '' '${remote_dump_path}'"

  log "Copying production backup to local file ${local_dump_path}"
  scp_expect_remote_to_local "${remote_dump_path}" "${local_dump_path}"

  cat > "${local_helper_path}.sql" <<EOF
${UPDATE_SQL}
EOF
  scp_expect_local_to_remote "${local_helper_path}.sql" "/tmp/sub2api-openai-gpt55-update-${TIMESTAMP}.sql"

  log "Applying production database update on ${REMOTE_HOST}"
  updated_count="$(ssh_expect "bash '${remote_helper_path}' '${REMOTE_CONFIG_FILE}' update '/tmp/sub2api-openai-gpt55-update-${TIMESTAMP}.sql' ''")"
  updated_count="$(printf '%s' "${updated_count}" | tr -d '[:space:]')"

  ssh_expect "rm -f '${remote_helper_path}' '/tmp/sub2api-openai-gpt55-update-${TIMESTAMP}.sql'"
  rm -f "${local_helper_path}" "${local_helper_path}.sql"

  log "Production backup complete: ${local_dump_path}"
  log "Production update complete: ${updated_count:-0} account(s) updated"
}

case "${MODE}" in
  --local)
    run_local
    ;;
  --remote-prod)
    run_remote_prod
    ;;
  *)
    usage >&2
    exit 1
    ;;
esac
