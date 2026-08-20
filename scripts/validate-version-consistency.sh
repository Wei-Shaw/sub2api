#!/bin/sh
set -eu

root_dir=${PROJECT_ROOT:-/app}
fork_version=$(tr -d '\r\n' < "$root_dir/FORK_VERSION")
base_version=$(tr -d '\r\n' < "$root_dir/backend/cmd/server/VERSION")

case "$fork_version" in
  *-custom|*-custom.*) ;;
  *)
    echo "FORK_VERSION must be a custom build version, got: $fork_version" >&2
    exit 1
    ;;
esac

fork_base=${fork_version%%-*}
if [ "$fork_base" != "$base_version" ]; then
  echo "version mismatch: FORK_VERSION=$fork_version but backend/cmd/server/VERSION=$base_version" >&2
  echo "update both version sources before building a custom image" >&2
  exit 1
fi

printf '%s\n' "$fork_version"
