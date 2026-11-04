#!/bin/sh
set -eu

probe_mode=${1:-probes}
probe_label=${2:-rerun}
case "$probe_label" in
	''|*[!a-zA-Z0-9_-]*) echo "invalid label" >&2; exit 2 ;;
esac
case "$probe_mode" in
	probes)
		probe_filter='^BenchmarkRound2'
		probe_tests='^TestRound2EnvelopeProbeFixtures$'
		probe_time=200ms
		probe_count=3
		;;
	controls)
		probe_filter='Benchmark(Round2IntegritySnapshot|DeriveOpenAIContentSessionSeedLargeBody|OpenAIUsageExtract|OpenAIResponses_LargeInputMeta|OpenAIWSTokenEventClassification|ResponsesCustomInputStream|StripOpenAIResponsesInputNamespaces)$'
		probe_tests='^$'
		probe_time=150ms
		probe_count=3
		;;
	verify)
		probe_filter='^$'
		probe_tests='^(TestRound2EnvelopeProbeFixtures|TestParseOpenAIWSEventEnvelope|TestOpenAIImageOutputCounter|TestExtractOpenAIUsageFromJSONBytes|TestResToAnthFuncArgs|TestResponsesClientToolStreamRestorer|TestChatCompletionsCustomInput|TestStream_)'
		probe_time=150ms
		probe_count=1
		;;
	admission)
		probe_filter='^Benchmark(OpenAIWSJSONValidation|OpenAIIntegritySnapshotAdmission)$'
		probe_tests='^$'
		probe_time=300ms
		probe_count=3
		;;
	*) echo "usage: sh run-probes.sh [probes|controls|verify|admission] [LABEL]" >&2; exit 2 ;;
esac

probe_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
probe_repo=$(CDPATH= cd -- "$probe_dir/../.." && pwd)
probe_output="$probe_dir/$probe_mode-$probe_label.txt"
if [ -e "$probe_output" ]; then
	echo "refusing to overwrite $probe_output" >&2
	exit 1
fi

docker run --rm --network=none --cpus=2 --memory=4g \
	-v "$probe_repo:/src:ro" \
	-v sub2api-gomod:/go/pkg/mod \
	-v sub2api-gocache:/root/.cache/go-build \
	-w /src/backend -e CGO_ENABLED=0 -e GOMAXPROCS=2 \
	-e GOMEMLIMIT=1400MiB -e GOFLAGS=-p=1 \
	-e GOTOOLCHAIN=local -e GOPROXY=off golang:1.26.10-alpine \
	go test -overlay=/src/diagnostics/gateway-performance-round2-20261104/overlay.json \
	./internal/application/service ./internal/shared/apicompat \
	-run "$probe_tests" -bench "$probe_filter" \
	-benchmem -benchtime="$probe_time" -count="$probe_count" -timeout=4m \
	> "$probe_output" 2>&1
echo "saved $probe_output"
