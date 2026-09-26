#!/usr/bin/env bash

set -Eeuo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
script="${root_dir}/deploy/aws-ssm-deploy.sh"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "${tmp_dir}"' EXIT

# shellcheck source=../aws-ssm-deploy.sh
source "${script}"

target_sha="0123456789abcdef0123456789abcdef01234567"
target_image="ghcr.io/domenlee/sub2api:${target_sha}"

validate_target "${target_sha}" "${target_image}"
if validate_target "not-a-sha" "${target_image}"; then
  echo "invalid commit SHA was accepted" >&2
  exit 1
fi
if validate_target "${target_sha}" "ghcr.io/example/sub2api:${target_sha}"; then
  echo "unexpected image repository was accepted" >&2
  exit 1
fi

cat > "${tmp_dir}/source.yml" <<'YAML'
services:
  postgres:
    image: "postgres:18-alpine"
  sub2api:
    image: "ghcr.io/domenlee/sub2api:old"
YAML

render_override "${tmp_dir}/source.yml" "${tmp_dir}/rendered.yml" "${target_image}"
grep -Fxq '    image: "postgres:18-alpine"' "${tmp_dir}/rendered.yml"
grep -Fxq "    image: \"${target_image}\"" "${tmp_dir}/rendered.yml"

printf 'services:\n  sub2api:\n    restart: always\n' > "${tmp_dir}/missing-image.yml"
if render_override "${tmp_dir}/missing-image.yml" "${tmp_dir}/invalid.yml" "${target_image}"; then
  echo "override without an image was accepted" >&2
  exit 1
fi

echo "aws ssm deployment script tests passed"
