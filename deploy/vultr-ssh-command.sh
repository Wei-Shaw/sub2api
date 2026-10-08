#!/usr/bin/env bash

set -Eeuo pipefail

# Installed as a forced SSH command. Never evaluate SSH_ORIGINAL_COMMAND.
command="${SSH_ORIGINAL_COMMAND:-}"
if [[ ! "${command}" =~ ^deploy\ ([0-9a-f]{40})$ ]]; then
  echo "Only 'deploy <40-character commit SHA>' is allowed." >&2
  exit 64
fi

target_sha="${BASH_REMATCH[1]}"
exec sudo -n /usr/local/sbin/sub2api-vultr-deploy \
  "${target_sha}" "ghcr.io/domenlee/sub2api:${target_sha}"
