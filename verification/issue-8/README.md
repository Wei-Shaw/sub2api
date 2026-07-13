# Issue #8 competition and rollout verification

This directory contains the repeatable acceptance assets for issue #8. Raw command output is written to `output/issue-8/`, which is already ignored by Git. Do not add benchmark output or environment credentials to the repository.

## Automated gate

Run the short smoke pass while developing:

```powershell
.\verification\issue-8\run.tests.ps1
.\verification\issue-8\run.ps1 -Mode Quick
```

Run the formal benchmark and contention pass before enabling the feature:

```powershell
.\verification\issue-8\run.ps1 -Mode Full
```

`Full` is a release-evidence mode: its summary fails when the recorded worktree is dirty or contention was skipped. `Quick` intentionally permits a dirty worktree and `-SkipContention` for local iteration. Both modes fail when Docker cleanup reports a failed command or a resource owned by the run remains afterward.

Rebuild the summary from existing raw output without rerunning Docker:

```powershell
.\verification\issue-8\run.ps1 -SummarizeOnly
```

The runner uses `s2a-go-integration:1.26.5`, creates an isolated Redis container, and removes its temporary container and network in a `finally` block. The contention process receives a unique `SUB2API_TEST_RUN_ID`; the integration harness copies it to the `com.sub2api.test.run` label on its Testcontainers containers. PostgreSQL and Redis test data directories use tmpfs, so those image-declared data paths do not create anonymous volumes. Cleanup selects containers and networks only through the matching run label, obtains any remaining owned volume IDs from those containers and the uniquely named runner Redis container, and never removes an unrelated resource merely because it appeared during the run. Every removal command is checked, followed by a second inventory. A command failure or any owned container, network, or volume left behind fails the run and is recorded under `resource_cleanup`.

The formal run executes each benchmark five times for two seconds and then runs the two real-Redis, four-client contention tests. `-DockerCommand` exists only to let `run.tests.ps1` exercise the complete runner with a local fake Docker executable; normal runs should keep the default `docker` value.

The generated files are:

- `output/issue-8/benchmark.txt`: raw Go benchmark output.
- `output/issue-8/contention.txt`: raw multi-instance contention output with `issue8_metric` evidence.
- `output/issue-8/run-status.json`: image, commit, dirty-worktree, exit-code, test run label, cleanup-command, and post-cleanup inventory metadata.
- `output/issue-8/summary.json`: machine-readable gates and medians.
- `output/issue-8/summary.md`: reviewer-facing result table.

## Performance gates

The tracked values in `thresholds.json` are release gates, not optimization targets.

| Path | Gate | Why |
| --- | ---: | --- |
| Flag off, legacy user + account slot lifecycle | median <= 4 ms | Detects regression in the unchanged path. |
| Flag on, standard `Begin -> NextTarget -> Dispatch -> Close` | median <= 4 ms and <= 2x flag-off | Includes the scheduler Redis capacity GET and JSON decode. |
| Flag on, extra lifecycle | median <= 5 ms and <= 1.5x standard | Bounds extra-class bookkeeping and reserve checks. |
| One queue poll | 18-40 ms | Confirms the intended 20 ms polling cadence without runaway polling. |
| 100 ms short-request reference | standard <= 4%, extra <= 5% | Makes fixed admission overhead visible for short HTTP requests. |

Contention passes only when the test process succeeds and the raw log contains all of `capacity_breach=0`, `reserve_breach=0`, and `account_breach=0`.

## Repository-wide formal checks

The issue #8 runner is intentionally narrow. Run the repository gates separately from the repository root after #7 is stable:

```powershell
$backend = (Resolve-Path .\backend).Path
$image = "s2a-go-integration:1.26.5"
$testRunID = [guid]::NewGuid().ToString("N")

docker run --rm --mount "type=bind,source=$backend,target=/workspace" -w /workspace $image go test -tags=unit ./...
docker run --rm -e CI=true -e TESTCONTAINERS_RYUK_DISABLED=true -e SUB2API_TEST_RUN_ID=$testRunID -v /var/run/docker.sock:/var/run/docker.sock --mount "type=bind,source=$backend,target=/workspace" -w /workspace $image go test -tags=integration ./...
docker run --rm --mount "type=bind,source=$backend,target=/workspace" -w /workspace $image go vet ./...
docker run --rm --mount "type=bind,source=$backend,target=/workspace" -w /workspace $image go generate ./cmd/server
git diff --exit-code -- backend/cmd/server/wire_gen.go

$raceImage = "s2a-issue8-race:go1.26.5"
docker build --file .\verification\issue-8\Dockerfile.race --tag $raceImage .
docker run --rm -e CI=true -e TESTCONTAINERS_RYUK_DISABLED=true -e SUB2API_TEST_RUN_ID=$testRunID -v /var/run/docker.sock:/var/run/docker.sock --mount "type=bind,source=$backend,target=/workspace" -w /workspace $raceImage go test -race -tags=integration ./internal/repository -run '^TestIssue8GatewayAdmission' -count=1
```

Snapshot Docker container IDs and running/health states, networks, and anonymous volumes before these repository-wide commands. After the final command, remove only resources carrying this run's `com.sub2api.test.run=$testRunID` label or anonymous volumes proven to be mounted by those containers. Verify all run-labeled resources and new test-owned anonymous volumes are gone, and confirm every pre-existing running business container is still running and healthy. Do not use an unscoped Docker prune.

The race image uses the Debian Go toolchain for CGO/GCC support and copies the Docker client plus its musl loader from the project integration image so Testcontainers can use the mounted host socket.

Use the installed frontend binaries directly; do not change the package-manager configuration:

```powershell
Set-Location .\frontend
.\node_modules\.bin\vue-tsc.cmd --noEmit
.\node_modules\.bin\eslint.cmd . --ext .vue,.js,.jsx,.cjs,.mjs,.ts,.tsx,.cts,.mts
.\node_modules\.bin\vitest.cmd run
.\node_modules\.bin\vue-tsc.cmd -b
.\node_modules\.bin\vite.cmd build
```

Do not recommend production enablement unless the formal issue #8 summary and every repository-wide gate pass.
After running these commands on the clean code-and-assets commit, replace the `NOT RUN` entries in `repository-gates.md` with the actual result and raw-output location. The resulting evidence-only commit may follow the verified commit; do not carry forward results from an earlier code/assets commit or a dirty worktree.

## Staged enablement rehearsal

Use a staging environment with at least two application instances and production-like Redis latency. Set these environment variables in the operator shell:

```powershell
$base = $env:SUB2API_BASE_URL.TrimEnd('/')
$headers = @{ "x-api-key" = $env:SUB2API_ADMIN_API_KEY }
$testUserID = [int64]$env:SUB2API_TEST_USER_ID

function Get-AdminSettingsData {
    $response = Invoke-RestMethod -Headers $headers -Uri "$base/api/v1/admin/settings"
    if ($response.code -ne 0 -or $null -eq $response.data) {
        throw "GET /admin/settings did not return a successful data envelope"
    }
    return $response.data
}

function Put-AdminSettingsData {
    param([Parameter(Mandatory = $true)][object]$Settings)

    $body = $Settings | ConvertTo-Json -Depth 50
    $response = Invoke-RestMethod -Method Put -Headers $headers -ContentType 'application/json' -Body $body -Uri "$base/api/v1/admin/settings"
    if ($response.code -ne 0 -or $null -eq $response.data) {
        throw "PUT /admin/settings did not return a successful data envelope"
    }
    return $response.data
}
```

`PUT /admin/settings` is not generally partial-safe. Several newer fields preserve omission, but many legacy scalar fields bind omitted JSON properties to zero values and are then persisted. Every settings change below therefore starts from the current complete `data` object and sends the complete object back.

### 1. Capture the disabled baseline

Deploy the schema and application while the feature remains disabled. Record the commit, instance count, request volume, error rate, timeout rate, queue depth, and billing count for at least 15 minutes.

```powershell
$settingsBefore = Get-AdminSettingsData
$settingsBefore | ConvertTo-Json -Depth 20 | Set-Content output/issue-8/settings-before.json
Invoke-RestMethod -Headers $headers -Uri "$base/api/v1/admin/ops/concurrency" | ConvertTo-Json -Depth 20 | Set-Content output/issue-8/concurrency-before.json
Invoke-RestMethod -Headers $headers -Uri "$base/api/v1/admin/ops/user-concurrency" | ConvertTo-Json -Depth 20 | Set-Content output/issue-8/user-concurrency-before.json
```

Required baseline evidence:

- `extra_concurrency_enabled` is `false` on every instance.
- Existing users have `extra_concurrency=0` after migration.
- Existing routes retain their prior concurrency errors, waiting behavior, and billing behavior.
- No request that fails before upstream dispatch creates usage billing.

### 2. Configure policy while still disabled

Set a conservative global reserve and one test user's extra concurrency. Read the settings immediately before the update so concurrent administrative changes are not overwritten by a stale snapshot.

```powershell
$disabledPolicy = Get-AdminSettingsData
$disabledPolicy.extra_concurrency_enabled = $false
$disabledPolicy.extra_concurrency_wait_timeout_seconds = 30
$disabledPolicy.extra_concurrency_reserve_percent = 25
$disabledPolicy.extra_concurrency_min_reserved_slots = 1
$disabledPolicy.extra_concurrency_platform_reserves = [pscustomobject]@{}
$disabledPolicy = Put-AdminSettingsData -Settings $disabledPolicy

$testUserPolicy = @{ extra_concurrency = 1 } | ConvertTo-Json
Invoke-RestMethod -Method Put -Headers $headers -ContentType 'application/json' -Body $testUserPolicy -Uri "$base/api/v1/admin/users/$testUserID"
```

The user update is partial-safe for this payload. `UpdateUserRequest.extra_concurrency` is a pointer, and the service changes it only when the field is present; omitted user fields remain unchanged. This guarantee is specific to `PUT /admin/users/:id` and must not be inferred for the settings endpoint.

Verify the test user still follows the standard-only path before the global flag is enabled.

### 3. Enable only the canary entitlement

Because every non-canary existing user still has `extra_concurrency=0`, enabling the global flag exposes the new path only to explicitly configured users.

```powershell
$enable = Get-AdminSettingsData
$enable.extra_concurrency_enabled = $true
$enable = Put-AdminSettingsData -Settings $enable
```

Run a 15-minute single-user canary, then a 30-minute small cohort, then a 60-minute 10% cohort. At each stage, capture the two ops endpoints and the application error and billing views.

Advance only when all gates hold:

- No platform capacity or reserved-slot breach.
- No account concurrency breach.
- Standard requests retain priority and FIFO ordering.
- Error rate and timeout rate are each no more than 0.5 percentage points above the disabled baseline.
- p95 request latency is no more than 5% above the disabled baseline for the same traffic mix.
- Pre-upstream timeout, cancellation, disconnect, retry-selection failure, and final billing recheck rejection create no billing record.
- Dynamic settings reach every instance within one second.

## Emergency rollback rehearsal

Before broader enablement, start one delayed upstream request that has already dispatched and one extra request still waiting. Then disable only the global flag:

```powershell
$disable = Get-AdminSettingsData
$disable.extra_concurrency_enabled = $false
$rollbackStarted = Get-Date
$disable = Put-AdminSettingsData -Settings $disable
```

Do not roll back migration 174, do not clear user `extra_concurrency`, and do not terminate already dispatched requests.

Rollback passes only when:

- Every instance reports the flag disabled within one second.
- The already dispatched request completes naturally and is billed at most once.
- The undispatched extra waiter enters the standard queue with its original arrival order and deadline.
- New requests use the legacy standard-concurrency path.
- Platform and account usage remain within capacity during the transition.
- The operator can confirm the disabled state and stable error rate within two minutes of `$rollbackStarted`.

## Manual result record

Create `output/issue-8/rollout-rehearsal.md` from this table. Keep it with the raw output, not in Git.

| Stage | Start/end UTC | Instances | Requests | Error delta | p95 delta | Capacity breaches | Billing anomalies | Decision |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| Disabled baseline | | | | | | | | |
| Single test user | | | | | | | | |
| Small cohort | | | | | | | | |
| 10% cohort | | | | | | | | |
| Emergency rollback | | | | | | | | |

The final issue comment should link the commit and summarize `summary.md`, repository-wide checks, the staged table, and the rollback rehearsal. Do not publish secrets or raw request bodies.
