#!/bin/sh
set -eu

label=${1:-}
case "$label" in
	''|*[!a-zA-Z0-9_-]*) echo "usage: sh diagnostics/gateway-performance-20261104/run-benchmarks.sh LABEL" >&2; exit 2 ;;
esac
evidence_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_dir=$(CDPATH= cd -- "$evidence_dir/../.." && pwd)
output="$evidence_dir/$label.txt"
if [ -e "$output" ]; then
	echo "refusing to overwrite existing evidence: $output" >&2
	exit 1
fi

docker run --rm --network=none --cpus=2 --memory=3g \
	-v "$repo_dir:/src:ro" \
	-v sub2api-gomod:/go/pkg/mod \
	-v sub2api-gocache:/root/.cache/go-build \
	-w /src/backend -e CGO_ENABLED=0 -e GOMAXPROCS=2 \
	-e GOTOOLCHAIN=local -e GOPROXY=off golang:1.26.10-alpine \
	go test ./internal/application/service ./internal/shared/apicompat \
	-run '^TestAnthropicStreamFragmentSnapshotsSurviveFollowingBlocks$' \
	-bench 'Benchmark(OpenAIResponses_LargeInputRawPatch|OpenAIWSHeaderPreparation|AnthropicFragmentAccumulation|ResponsesStreamEventWire)$' \
	-benchmem -benchtime=300ms -count=3 -timeout=3m > "$output" 2>&1
echo "saved $output"
