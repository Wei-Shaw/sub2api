#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
backend_dir=$(CDPATH= cd -- "${script_dir}/.." && pwd)
run_id=${CI_JOB_ID:-$$}
network="sub2api-valkey-test-${run_id}"
tester="sub2api-valkey-test-runner-${run_id}"
image=${SUB2API_TEST_VALKEY_IMAGE:-valkey/valkey:8.1-alpine}
go_image=${SUB2API_TEST_GO_IMAGE:-golang:1.26.5-alpine}

cleanup() {
  docker rm -f "${tester}" >/dev/null 2>&1 || true
  index=0
  while [ "${index}" -lt 6 ]; do
    docker rm -f "sub2api-valkey-test-${run_id}-${index}" >/dev/null 2>&1 || true
    index=$((index + 1))
  done
  docker network rm "${network}" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

docker network create "${network}" >/dev/null

addresses=""
index=0
while [ "${index}" -lt 6 ]; do
  node="sub2api-valkey-test-${run_id}-${index}"
  docker run -d --name "${node}" --network "${network}" "${image}" \
    valkey-server \
    --cluster-enabled yes \
    --cluster-config-file nodes.conf \
    --cluster-node-timeout 5000 \
    --appendonly no \
    --protected-mode no >/dev/null
  if [ -n "${addresses}" ]; then
    addresses="${addresses},"
  fi
  addresses="${addresses}${node}:6379"
  index=$((index + 1))
done

first_node="sub2api-valkey-test-${run_id}-0"
attempt=0
until docker run --rm --network "${network}" "${image}" valkey-cli -h "${first_node}" ping >/dev/null 2>&1; do
  attempt=$((attempt + 1))
  if [ "${attempt}" -ge 30 ]; then
    echo "Valkey nodes did not become ready" >&2
    exit 1
  fi
  sleep 1
done

old_ifs=${IFS}
IFS=,
set -- ${addresses}
IFS=${old_ifs}
docker run --rm --network "${network}" "${image}" \
  valkey-cli --cluster create "$@" --cluster-replicas 1 --cluster-yes >/dev/null

docker create --name "${tester}" --network "${network}" --workdir /src \
  --env "SUB2API_TEST_VALKEY_CLUSTER_ADDRESSES=${addresses}" \
  --env SUB2API_TEST_VALKEY_CLUSTER_DESTRUCTIVE=1 \
  "${go_image}" go test -v ./internal/clustercompat >/dev/null
docker cp "${backend_dir}/." "${tester}:/src"
docker start --attach "${tester}"
