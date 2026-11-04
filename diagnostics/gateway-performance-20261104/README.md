# Gateway Docker performance evidence — 2026-11-04

Base revision: `af1bcc3fce7aafcee6a908a407977e46455e52e3`. The baseline contains the new benchmark fixtures and regression test but uses the original production code. The optimized run uses the same fixtures, container limits and flags.

- Platform: Linux arm64, Go 1.26.10, `CGO_ENABLED=0`, `GOMAXPROCS=2`.
- Image: `golang:1.26.10-alpine`, local image ID `sha256:9ce231b28998b32e116927c61aa5c406183061d4f3f2d61b292c88e7f6bdf7ca`.
- Limits: 2 CPU, 3 GiB RAM, network disabled, source mounted read-only.
- Duration: 300 ms per benchmark, 3 repetitions; compare medians.
- Module/build caches: `sub2api-gomod` and `sub2api-gocache`, populated before the offline run.

From the repository root, run:

```sh
sh diagnostics/gateway-performance-20261104/run-benchmarks.sh rerun
```

The script refuses to replace existing logs. It requires the image and dependency caches to be available locally; an empty cache fails with `GOPROXY=off`.

`baseline.txt` and `optimized.txt` hold benchmark tool output. Captured logs normalize leading indentation and trailing whitespace for Git checks; measured values are unchanged. `comparison.json` contains the per-benchmark medians and percentage changes. The timed runs are sequential; no regression or build jobs were run alongside them.

| Fixture | Baseline → optimized median | Baseline → optimized allocation |
| --- | --- | --- |
| 16 MiB raw set patch | 11.127 → 7.598 ms | 33,571,232 → 16,786,713 B/op; 9 → 24 allocs/op |
| 256 KiB WS preparation | 1.169 → 0.695 ms | 1,219,257 → 333,386 B/op |
| 256 KiB fragmented text | 5.155 → 0.140 ms | 34,567,588 → 1,239,761 B/op |
| 256 KiB fragmented tool arguments | 5.300 → 0.177 ms | 34,566,563 → 1,238,755 B/op |
| 256 KiB fragmented unsigned thinking | 5.740 → 0.182 ms | 34,566,600 → 1,238,784 B/op |
| output_text.delta wire | 1414 → 795.8 ns | 1360 → 624 B/op; 21 → 4 allocs/op |

`focused-rerun.txt` records the passing targeted compatibility tests. `focused.txt` retains the first run: its encoding-error assertion initially used a NaN value, which sjson formats without returning an error. The fixture was corrected to an unsupported channel value before validation. `build.txt` records the successful backend build with toolchain metadata, and `mcp-sdk.txt` records the official SDK OAuth interoperability check. Full unit and fuzz results are recorded separately.

The first full unit/fuzz attempts used a 3 GiB container while other validation containers were running; large-package compilers were killed. The full unit run also exposed a harness requirement: `ent/schema` dynamically creates `.entc`, so it cannot execute from a read-only source directory. Those attempt logs are `unit.txt` and `fuzz.txt`. Subsequent validation copies the backend to `/tmp/backend` inside a disposable container, keeps the host checkout read-only, and runs one compilation at a time with `GOFLAGS=-p=1`, `GOMEMLIMIT=1400MiB`, `GOMAXPROCS=2` and a 4 GiB container limit. Frontend/docs/deploy references use read-only symlinks to the mounted checkout.

From the temporary backend directory in that container, the checks are:

```sh
go test -tags=unit ./... -count=1 -timeout=10m
go test ./internal/application/service -run '^$' -fuzz '^FuzzOpenAIRequestSetPatches$' -fuzztime=20s -parallel=2 -timeout=4m
go test -tags=mcp_go_client_oauth ./internal/transport/http/server/routes -run TestMCPOAuthOfficialGoSDKInteroperability -count=1 -timeout=3m
go build -trimpath -ldflags='-s -w' -o /tmp/sub2api ./cmd/server
```

The memory/compilation settings above apply to validation, not to the benchmark numbers. No production runtime/configuration tuning was included.

Final validation:

- `unit-rerun.txt`: PASS, 73 packages with tests; application/service 225.712 s, handler 60.766 s.
- `fuzz-rerun.txt`: PASS, 20 seconds, 35,003 executions, 84 additional interesting inputs, no semantic mismatch.
- `mcp-sdk.txt`: PASS, official Go SDK OAuth interoperability.
- `build.txt`: PASS, pure Go Linux arm64 release compilation (`CGO_ENABLED=0`).
- `git diff --check`, Go formatting, shell syntax and new/updated document link checks: PASS.
- Existing baseline gate failures persist: missing PR169 documentation link in `docs/README.md`, `payment_fulfillment_test.go` at 1201 lines and `pricing_service.go` at 1218 lines. No unrelated baseline fixes were included.

The WS benchmark exercises the actual forwarding/header preparation code using an in-memory upstream connection. It includes request context setup and response handling, excludes network and the final real socket JSON encoder, and must not be reported as end-to-end gateway latency. Fragment benchmarks accumulate 256 × 1 KiB fragments. Raw patch benchmarks use generated Responses histories from 4 to 32 MiB; the byte count is the full request size, although only two small fields are modified. SSE wire benchmarks encode one event including `MarshalJSON`.

For the recommendations, implementation boundaries and Rust follow-up criteria, see [the optimization record](../../docs/GATEWAY_PERFORMANCE_OPTIMIZATION_CN.md).
