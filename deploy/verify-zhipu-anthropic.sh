#!/usr/bin/env bash

set -euo pipefail
umask 077

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CLIENT_ENV="${CLIENT_ENV:-${SCRIPT_DIR}/client.env}"

if [[ ! -f "${CLIENT_ENV}" ]]; then
  echo "missing client environment file: ${CLIENT_ENV}" >&2
  exit 1
fi

set -a
# shellcheck disable=SC1090
source "${CLIENT_ENV}"
set +a

: "${ANTHROPIC_BASE_URL:?ANTHROPIC_BASE_URL is required}"
: "${ANTHROPIC_AUTH_TOKEN:?ANTHROPIC_AUTH_TOKEN is required}"

for command_name in curl jq; do
  if ! command -v "${command_name}" >/dev/null 2>&1; then
    echo "missing required command: ${command_name}" >&2
    exit 1
  fi
done

auth_curl() {
  curl --config <(printf 'header = "Authorization: Bearer %s"\n' "${ANTHROPIC_AUTH_TOKEN}") "$@"
}

echo '[1/5] health'
curl -fsS --connect-timeout 5 --max-time 15 "${ANTHROPIC_BASE_URL}/health" |
  jq -e '{status} | select(.status == "ok")'

echo '[2/5] models'
models_response="$(auth_curl -fsS --connect-timeout 5 --max-time 30 "${ANTHROPIC_BASE_URL}/v1/models")"
printf '%s' "${models_response}" |
  jq -e '{models:[.data[].id]} |
    select((.models | index("glm-4.7")) != null) |
    select((.models | index("glm-5.2")) != null) |
    select((.models | index("glm-5.2[1m]")) != null)'

echo '[3/5] non-streaming messages'
nonstream_payload="$(jq -nc '{
  model:"glm-4.7",
  max_tokens:16,
  stream:false,
  messages:[{role:"user",content:"Reply with exactly OK."}]
}')"
nonstream_response="$(auth_curl -fsS --connect-timeout 10 --max-time 180 \
  -H 'anthropic-version: 2023-06-01' \
  -H 'Content-Type: application/json' \
  --data-binary "${nonstream_payload}" \
  "${ANTHROPIC_BASE_URL}/v1/messages")"
printf '%s' "${nonstream_response}" |
  jq -e '{type,model,stop_reason,usage,content_types:[.content[].type]} |
    select(.type == "message") |
    select((.content_types | length) > 0)'

echo '[4/5] streaming messages'
stream_payload="$(jq -nc '{
  model:"glm-5.2[1m]",
  max_tokens:16,
  stream:true,
  messages:[{role:"user",content:"Reply with exactly OK."}]
}')"
stream_response="$(auth_curl -sS -N --connect-timeout 10 --max-time 180 \
  -H 'anthropic-version: 2023-06-01' \
  -H 'Content-Type: application/json' \
  --data-binary "${stream_payload}" \
  "${ANTHROPIC_BASE_URL}/v1/messages")"
stream_events="$(printf '%s\n' "${stream_response}" |
  sed -n 's/^data:[[:space:]]*//p' |
  tr -d '\r' |
  grep -v '^\[DONE\]$' |
  jq -sc '.')"
printf '%s' "${stream_events}" |
  jq -e '{
    event_types:([.[].type] | unique),
    terminal_event:any(.[]; .type == "message_stop")
  } |
  select((.event_types | index("message_start")) != null) |
  select((.event_types | index("content_block_delta")) != null) |
  select(.terminal_event == true)'

echo '[5/5] count_tokens'
count_payload="$(jq -nc '{
  model:"glm-5.2[1m]",
  messages:[{role:"user",content:"Count this short request."}]
}')"
count_response="$(auth_curl -fsS --connect-timeout 10 --max-time 120 \
  -H 'anthropic-version: 2023-06-01' \
  -H 'Content-Type: application/json' \
  --data-binary "${count_payload}" \
  "${ANTHROPIC_BASE_URL}/v1/messages/count_tokens")"
printf '%s' "${count_response}" |
  jq -e '{input_tokens} | select(.input_tokens >= 0)'

echo 'all API checks passed'
