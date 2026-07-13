[CmdletBinding()]
param(
    [ValidateSet("Quick", "Full")]
    [string]$Mode = "Quick",

    [string]$GoImage = "s2a-go-integration:1.26.5",

    [string]$RedisImage = "redis:8.4-alpine",

    [string]$DockerCommand = "docker",

    [string]$OutputDirectory = "",

    [switch]$SkipContention,

    [switch]$SummarizeOnly
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
$backendRoot = Join-Path $repoRoot "backend"
$thresholdPath = Join-Path $PSScriptRoot "thresholds.json"
if ([string]::IsNullOrWhiteSpace($OutputDirectory)) {
    $OutputDirectory = Join-Path $repoRoot "output\issue-8"
}
$OutputDirectory = [System.IO.Path]::GetFullPath($OutputDirectory)
$benchmarkPath = Join-Path $OutputDirectory "benchmark.txt"
$contentionPath = Join-Path $OutputDirectory "contention.txt"
$statusPath = Join-Path $OutputDirectory "run-status.json"
$summaryJSONPath = Join-Path $OutputDirectory "summary.json"
$summaryMarkdownPath = Join-Path $OutputDirectory "summary.md"

New-Item -ItemType Directory -Force -Path $OutputDirectory | Out-Null
$thresholds = Get-Content -Raw -LiteralPath $thresholdPath | ConvertFrom-Json
$modeThresholds = if ($Mode -eq "Quick") { $thresholds.quick } else { $thresholds.full }

function Invoke-CapturedDocker {
    param(
        [Parameter(Mandatory = $true)]
        [string[]]$Arguments,

        [Parameter(Mandatory = $true)]
        [string]$OutputPath
    )

    $lines = @(& $DockerCommand @Arguments 2>&1)
    $exitCode = $LASTEXITCODE
    $lines | Set-Content -LiteralPath $OutputPath -Encoding utf8
    foreach ($line in $lines) {
        Write-Host $line
    }
    return [int]$exitCode
}

function Invoke-DockerResult {
    param(
        [Parameter(Mandatory = $true)]
        [string[]]$Arguments
    )

    try {
        $lines = @(& $DockerCommand @Arguments 2>&1)
        $exitCode = [int]$LASTEXITCODE
    } catch {
        $lines = @($_.Exception.Message)
        $exitCode = -1
    }
    return [pscustomobject]@{
        exit_code = $exitCode
        lines = @($lines | ForEach-Object { "$_" })
    }
}

function Get-OptionalValue {
    param(
        [Parameter(Mandatory = $true)]
        [object]$Object,

        [Parameter(Mandatory = $true)]
        [string]$Name
    )

    $property = $Object.PSObject.Properties[$Name]
    if ($null -eq $property) {
        return $null
    }
    return $property.Value
}

function Get-Median {
    param([double[]]$Values)

    $sorted = @($Values | Sort-Object)
    if ($sorted.Count -eq 0) {
        throw "cannot calculate a median without samples"
    }
    $middle = [int][math]::Floor($sorted.Count / 2)
    if ($sorted.Count % 2 -eq 1) {
        return [double]$sorted[$middle]
    }
    return ([double]$sorted[$middle - 1] + [double]$sorted[$middle]) / 2
}

function Get-BenchmarkSamples {
    param([string]$Path)

    $samples = @{}
    if (-not (Test-Path -LiteralPath $Path)) {
        return $samples
    }
    $pattern = '^BenchmarkIssue8GatewayAdmissionLifecycle/(?<name>\S+)-\d+\s+\d+\s+(?<ns>[\d.]+)\s+ns/op'
    foreach ($line in Get-Content -LiteralPath $Path) {
        $match = [regex]::Match($line, $pattern)
        if (-not $match.Success) {
            continue
        }
        $name = $match.Groups["name"].Value
        $value = [double]::Parse(
            $match.Groups["ns"].Value,
            [System.Globalization.CultureInfo]::InvariantCulture
        )
        if (-not $samples.ContainsKey($name)) {
            $samples[$name] = @()
        }
        $samples[$name] = @($samples[$name]) + $value
    }
    return $samples
}

function Remove-RunDockerResources {
    param(
        [Parameter(Mandatory = $true)]
        [string]$RunnerContainerName,

        [Parameter(Mandatory = $true)]
        [string]$RunnerNetworkName,

        [Parameter(Mandatory = $true)]
        [string]$TestRunID,

        [bool]$RunnerContainerCreated = $false,

        [bool]$RunnerNetworkCreated = $false
    )

    $failures = [System.Collections.Generic.List[string]]::new()

    function Get-CleanupIds {
        param(
            [string[]]$Arguments,
            [string]$Description
        )

        $result = Invoke-DockerResult -Arguments $Arguments
        if ([int]$result.exit_code -ne 0) {
            $failures.Add("$Description failed with exit code $($result.exit_code): $(@($result.lines) -join '; ')") | Out-Null
            return @()
        }
        return @($result.lines | ForEach-Object { "$($_)".Trim() } | Where-Object {
            -not [string]::IsNullOrWhiteSpace($_)
        })
    }

    function Invoke-CleanupRemoval {
        param(
            [string[]]$Arguments,
            [string]$Description
        )

        $result = Invoke-DockerResult -Arguments $Arguments
        if ([int]$result.exit_code -ne 0) {
            $failures.Add("$Description failed with exit code $($result.exit_code): $(@($result.lines) -join '; ')") | Out-Null
        }
    }

    $runLabel = "label=com.sub2api.test.run=$TestRunID"
    $testcontainers = @(Get-CleanupIds `
        -Arguments @("ps", "-aq", "--filter", $runLabel) `
        -Description "Testcontainers container inventory")
    $testNetworks = @(Get-CleanupIds `
        -Arguments @("network", "ls", "-q", "--filter", $runLabel) `
        -Description "Testcontainers network inventory")
    $runVolumes = @(Get-CleanupIds `
        -Arguments @("volume", "ls", "-q", "--filter", $runLabel) `
        -Description "Testcontainers volume inventory")
    $currentAnonymousVolumes = @(Get-CleanupIds `
        -Arguments @("volume", "ls", "-q", "--filter", "label=com.docker.volume.anonymous") `
        -Description "anonymous volume inventory")

    $volumeOwners = @($testcontainers)
    if ($RunnerContainerCreated) {
        $volumeOwners += $RunnerContainerName
    }
    $mountedVolumes = @()
    if ($volumeOwners.Count -gt 0) {
        $mountResult = Invoke-DockerResult -Arguments (
            @("inspect", "--format", '{{range .Mounts}}{{if eq .Type "volume"}}{{println .Name}}{{end}}{{end}}') +
            $volumeOwners
        )
        if ([int]$mountResult.exit_code -ne 0) {
            $failures.Add("owned volume inventory failed with exit code $($mountResult.exit_code): $(@($mountResult.lines) -join '; ')") | Out-Null
        } else {
            $mountedVolumes = @($mountResult.lines | ForEach-Object { "$($_)".Trim() } | Where-Object {
                -not [string]::IsNullOrWhiteSpace($_)
            })
        }
    }
    $ownedAnonymousVolumes = @($mountedVolumes | Where-Object {
        $_ -in $currentAnonymousVolumes
    })
    $trackedVolumes = @(($runVolumes + $ownedAnonymousVolumes) | Sort-Object -Unique)

    if ($RunnerContainerCreated) {
        Invoke-CleanupRemoval `
            -Arguments @("rm", "-f", $RunnerContainerName) `
            -Description "runner container '$RunnerContainerName' removal"
    }
    if ($RunnerNetworkCreated) {
        Invoke-CleanupRemoval `
            -Arguments @("network", "rm", $RunnerNetworkName) `
            -Description "runner network '$RunnerNetworkName' removal"
    }
    if ($testcontainers.Count -gt 0) {
        Invoke-CleanupRemoval `
            -Arguments (@("rm", "-fv") + $testcontainers) `
            -Description "Testcontainers container removal [$($testcontainers -join ', ')]"
    }
    if ($testNetworks.Count -gt 0) {
        Invoke-CleanupRemoval `
            -Arguments (@("network", "rm") + $testNetworks) `
            -Description "Testcontainers network removal [$($testNetworks -join ', ')]"
    }
    $volumesAfterContainerRemoval = @(Get-CleanupIds `
        -Arguments @("volume", "ls", "-q") `
        -Description "owned volume removal inventory")
    $volumesToRemove = @($trackedVolumes | Where-Object {
        $_ -in $volumesAfterContainerRemoval
    })
    if ($volumesToRemove.Count -gt 0) {
        Invoke-CleanupRemoval `
            -Arguments (@("volume", "rm") + $volumesToRemove) `
            -Description "owned volume removal [$($volumesToRemove -join ', ')]"
    }

    $remainingRunnerContainers = if ($RunnerContainerCreated) {
        @(Get-CleanupIds `
            -Arguments @("ps", "-aq", "--filter", "name=^/$RunnerContainerName$") `
            -Description "runner container verification")
    } else { @() }
    $remainingRunnerNetworks = if ($RunnerNetworkCreated) {
        @(Get-CleanupIds `
            -Arguments @("network", "ls", "-q", "--filter", "name=^$RunnerNetworkName$") `
            -Description "runner network verification")
    } else { @() }
    $remainingTestcontainers = @(Get-CleanupIds `
        -Arguments @("ps", "-aq", "--filter", $runLabel) `
        -Description "Testcontainers container verification")
    $remainingTestNetworks = @(Get-CleanupIds `
        -Arguments @("network", "ls", "-q", "--filter", $runLabel) `
        -Description "Testcontainers network verification")
    $remainingAnonymousVolumes = @(
        @(Get-CleanupIds `
            -Arguments @("volume", "ls", "-q") `
            -Description "owned volume verification") |
            Where-Object { $_ -in $trackedVolumes }
    )

    $remaining = [ordered]@{
        runner_containers = $remainingRunnerContainers
        runner_networks = $remainingRunnerNetworks
        testcontainers = $remainingTestcontainers
        test_networks = $remainingTestNetworks
        anonymous_volumes = $remainingAnonymousVolumes
    }
    foreach ($property in $remaining.GetEnumerator()) {
        if (@($property.Value).Count -gt 0) {
            $failures.Add("$($property.Key) remain after cleanup: $(@($property.Value) -join ', ')") | Out-Null
        }
    }

    return [pscustomobject]@{
        passed = $failures.Count -eq 0
        run_id = $TestRunID
        failures = @($failures)
        remaining = $remaining
    }
}

if (-not $SummarizeOnly) {
    $suffix = "{0}-{1}" -f $PID, ([DateTimeOffset]::UtcNow.ToUnixTimeSeconds())
    $networkName = "s2a-issue8-$suffix"
    $redisName = "s2a-issue8-redis-$suffix"
    $testRunID = [guid]::NewGuid().ToString("N")
    $benchmarkExitCode = 1
    $contentionExitCode = if ($SkipContention) { 0 } else { 1 }
    $redisCreated = $false
    $networkCreated = $false
    $resourceCleanup = $null
    $workError = ""

    try {
        foreach ($path in @($benchmarkPath, $contentionPath, $statusPath, $summaryJSONPath, $summaryMarkdownPath)) {
            Remove-Item -LiteralPath $path -Force -ErrorAction SilentlyContinue
        }

        & $DockerCommand image inspect $GoImage *> $null
        if ($LASTEXITCODE -ne 0) {
            throw "Go image is unavailable: $GoImage"
        }
        & $DockerCommand image inspect $RedisImage *> $null
        if ($LASTEXITCODE -ne 0) {
            throw "Redis image is unavailable: $RedisImage"
        }

        & $DockerCommand network create $networkName *> $null
        if ($LASTEXITCODE -ne 0) {
            throw "failed to create Docker network $networkName"
        }
        $networkCreated = $true
        & $DockerCommand run -d --rm --name $redisName --network $networkName $RedisImage *> $null
        if ($LASTEXITCODE -ne 0) {
            throw "failed to start Redis container $redisName"
        }
        $redisCreated = $true

        $redisReady = $false
        for ($attempt = 0; $attempt -lt 50; $attempt++) {
            $ping = & $DockerCommand exec $redisName redis-cli ping 2>$null
            if ($LASTEXITCODE -eq 0 -and "$ping" -eq "PONG") {
                $redisReady = $true
                break
            }
            Start-Sleep -Milliseconds 200
        }
        if (-not $redisReady) {
            throw "Redis did not become ready"
        }

        $benchmarkArgs = @(
            "run", "--rm",
            "--network", $networkName,
            "-e", "TEST_REDIS_URL=redis://${redisName}:6379/0",
            "--mount", "type=bind,source=$backendRoot,target=/workspace",
            "-w", "/workspace",
            $GoImage,
            "go", "test", "./internal/repository",
            "-run", "^$",
            "-bench", "^BenchmarkIssue8GatewayAdmissionLifecycle$",
            "-benchmem",
            "-benchtime=$($modeThresholds.benchmark_time)",
            "-count=$($modeThresholds.benchmark_count)"
        )
        $benchmarkExitCode = Invoke-CapturedDocker -Arguments $benchmarkArgs -OutputPath $benchmarkPath

        if (-not $SkipContention) {
            $contentionArgs = @(
                "run", "--rm",
                "-e", "CI=true",
                "-e", "TESTCONTAINERS_RYUK_DISABLED=true",
                "-e", "SUB2API_TEST_RUN_ID=$testRunID",
                "-v", "/var/run/docker.sock:/var/run/docker.sock",
                "--mount", "type=bind,source=$backendRoot,target=/workspace",
                "-w", "/workspace",
                $GoImage,
                "go", "test", "-tags=integration", "./internal/repository",
                "-run", "^TestIssue8GatewayAdmission.*ZeroBreakthrough",
                "-count=1", "-v"
            )
            $contentionExitCode = Invoke-CapturedDocker -Arguments $contentionArgs -OutputPath $contentionPath
        } else {
            "contention run skipped" | Set-Content -LiteralPath $contentionPath -Encoding utf8
        }
    } catch {
        $workError = $_.Exception.Message
    } finally {
        $resourceCleanup = Remove-RunDockerResources `
            -RunnerContainerName $redisName `
            -RunnerNetworkName $networkName `
            -TestRunID $testRunID `
            -RunnerContainerCreated $redisCreated `
            -RunnerNetworkCreated $networkCreated
    }

    $gitCommit = (& git -C $repoRoot rev-parse HEAD 2>$null)
    $gitDirtyText = ((& git -C $repoRoot status --porcelain 2>$null) -join "`n")
    $runStatus = [ordered]@{
        generated_at_utc = [DateTimeOffset]::UtcNow.ToString("o")
        mode = $Mode
        go_image = $GoImage
        redis_image = $RedisImage
        git_commit = "$gitCommit".Trim()
        git_dirty = -not [string]::IsNullOrWhiteSpace($gitDirtyText)
        benchmark_exit_code = $benchmarkExitCode
        contention_exit_code = $contentionExitCode
        contention_skipped = [bool]$SkipContention
        work_error = $workError
        resource_cleanup = $resourceCleanup
    }
    $runStatus | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath $statusPath -Encoding utf8
}

if (-not (Test-Path -LiteralPath $statusPath)) {
    throw "run status is missing: $statusPath"
}
$runStatus = Get-Content -Raw -LiteralPath $statusPath | ConvertFrom-Json
$samples = Get-BenchmarkSamples -Path $benchmarkPath
$medians = @{}
foreach ($name in $samples.Keys) {
    $medians[$name] = Get-Median -Values @($samples[$name])
}

$benchmarkGates = @()
foreach ($property in $thresholds.benchmarks.PSObject.Properties) {
    $name = $property.Name
    $rule = $property.Value
    $passed = $true
    $reasons = @()
    $median = $null
    $ratio = $null
    $shortRequestPercent = $null

    if (-not $medians.ContainsKey($name)) {
        $passed = $false
        $reasons += "missing benchmark samples"
    } else {
        $median = [double]$medians[$name]
        $minimum = Get-OptionalValue -Object $rule -Name "min_median_ns_per_op"
        $maximum = Get-OptionalValue -Object $rule -Name "max_median_ns_per_op"
        if ($null -ne $minimum -and $median -lt [double]$minimum) {
            $passed = $false
            $reasons += "median is below minimum"
        }
        if ($null -ne $maximum -and $median -gt [double]$maximum) {
            $passed = $false
            $reasons += "median exceeds maximum"
        }

        $ratioBase = Get-OptionalValue -Object $rule -Name "max_ratio_to"
        $maxRatio = Get-OptionalValue -Object $rule -Name "max_ratio"
        if ($null -ne $ratioBase -and $null -ne $maxRatio) {
            if (-not $medians.ContainsKey([string]$ratioBase)) {
                $passed = $false
                $reasons += "ratio baseline is missing"
            } else {
                $ratio = $median / [double]$medians[[string]$ratioBase]
                if ($ratio -gt [double]$maxRatio) {
                    $passed = $false
                    $reasons += "ratio exceeds maximum"
                }
            }
        }

        $maxShortPercent = Get-OptionalValue -Object $rule -Name "max_short_request_overhead_percent"
        if ($null -ne $maxShortPercent) {
            $shortRequestPercent = $median / ([double]$thresholds.reference_short_request_ms * 1e6) * 100
            if ($shortRequestPercent -gt [double]$maxShortPercent) {
                $passed = $false
                $reasons += "short-request overhead exceeds maximum"
            }
        }
    }

    $benchmarkGates += [pscustomobject]@{
        name = $name
        samples = if ($samples.ContainsKey($name)) { @($samples[$name]).Count } else { 0 }
        median_ns_per_op = $median
        median_ms_per_op = if ($null -ne $median) { $median / 1e6 } else { $null }
        ratio = $ratio
        short_request_overhead_percent = $shortRequestPercent
        passed = $passed
        reason = $reasons -join "; "
    }
}

$contentionText = if (Test-Path -LiteralPath $contentionPath) {
    Get-Content -Raw -LiteralPath $contentionPath
} else {
    ""
}
$missingEvidence = @()
if (-not [bool]$runStatus.contention_skipped) {
    foreach ($evidence in $thresholds.contention.required_evidence) {
        if (-not $contentionText.Contains([string]$evidence)) {
            $missingEvidence += [string]$evidence
        }
    }
}
$benchmarkPassed = ([int]$runStatus.benchmark_exit_code -eq 0) -and -not ($benchmarkGates.passed -contains $false)
$formalRun = "$($runStatus.mode)" -eq "Full"
$contentionPassed = if ([bool]$runStatus.contention_skipped) {
    -not $formalRun
} else {
    ([int]$runStatus.contention_exit_code -eq 0) -and $missingEvidence.Count -eq 0
}
$repositoryStatePassed = -not ($formalRun -and [bool]$runStatus.git_dirty)
$workErrorValue = Get-OptionalValue -Object $runStatus -Name "work_error"
$workError = if ($null -eq $workErrorValue) { "" } else { "$workErrorValue" }
$workPassed = [string]::IsNullOrWhiteSpace($workError)
$resourceCleanup = Get-OptionalValue -Object $runStatus -Name "resource_cleanup"
$resourceCleanupPassedValue = if ($null -eq $resourceCleanup) {
    $null
} else {
    Get-OptionalValue -Object $resourceCleanup -Name "passed"
}
$resourceCleanupPassed = $null -ne $resourceCleanupPassedValue -and [bool]$resourceCleanupPassedValue
$resourceCleanupRunID = if ($null -eq $resourceCleanup) {
    "not recorded"
} else {
    $value = Get-OptionalValue -Object $resourceCleanup -Name "run_id"
    if ([string]::IsNullOrWhiteSpace("$value")) { "not recorded" } else { "$value" }
}
$resourceCleanupFailures = @(
    if ($null -eq $resourceCleanup) {
        "resource cleanup evidence is missing"
    } else {
        $property = $resourceCleanup.PSObject.Properties["failures"]
        if ($null -eq $property) {
            "resource cleanup failure details are missing"
        } else {
            @($property.Value)
        }
    }
)
$overallPassed = $workPassed -and $benchmarkPassed -and $contentionPassed -and $repositoryStatePassed -and $resourceCleanupPassed

$summary = [ordered]@{
    generated_at_utc = [DateTimeOffset]::UtcNow.ToString("o")
    mode = $runStatus.mode
    git_commit = $runStatus.git_commit
    git_dirty = $runStatus.git_dirty
    work_passed = $workPassed
    work_error = $workError
    repository_state_passed = $repositoryStatePassed
    benchmark_passed = $benchmarkPassed
    benchmark_gates = $benchmarkGates
    contention_skipped = $runStatus.contention_skipped
    contention_passed = $contentionPassed
    contention_missing_evidence = $missingEvidence
    resource_cleanup_passed = $resourceCleanupPassed
    resource_cleanup = $resourceCleanup
    overall_passed = $overallPassed
    raw_outputs = [ordered]@{
        benchmark = $benchmarkPath
        contention = $contentionPath
        status = $statusPath
    }
}
$summary | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $summaryJSONPath -Encoding utf8

$markdown = @(
    "# Issue 8 acceptance summary",
    "",
    "- Overall: **$(if ($overallPassed) { 'PASS' } else { 'FAIL' })**",
    "- Mode: $($runStatus.mode)",
    "- Commit: ``$($runStatus.git_commit)``",
    "- Dirty worktree: $($runStatus.git_dirty)",
    "- Runner work: **$(if ($workPassed) { 'PASS' } else { 'FAIL' })**",
    "- Runner failure: $(if ($workPassed) { 'none' } else { $workError })",
    "- Repository state: **$(if ($repositoryStatePassed) { 'PASS' } else { 'FAIL: Full mode requires a clean worktree' })**",
    "- Docker resource cleanup: **$(if ($resourceCleanupPassed) { 'PASS' } else { 'FAIL' })**",
    "- Docker cleanup run: ``$resourceCleanupRunID``",
    "- Docker cleanup failures: $(if ($resourceCleanupFailures.Count -eq 0) { 'none' } else { $resourceCleanupFailures -join '; ' })",
    "",
    "| Benchmark | Median ms/op | Ratio | 100ms request overhead | Gate |",
    "| --- | ---: | ---: | ---: | --- |"
)
foreach ($gate in $benchmarkGates) {
    $medianText = if ($null -eq $gate.median_ms_per_op) { "n/a" } else { "{0:N3}" -f $gate.median_ms_per_op }
    $ratioText = if ($null -eq $gate.ratio) { "n/a" } else { "{0:N2}x" -f $gate.ratio }
    $overheadText = if ($null -eq $gate.short_request_overhead_percent) { "n/a" } else { "{0:N2}%" -f $gate.short_request_overhead_percent }
    $gateText = if ($gate.passed) { "PASS" } else { "FAIL: $($gate.reason)" }
    $markdown += "| $($gate.name) | $medianText | $ratioText | $overheadText | $gateText |"
}
$markdown += @(
    "",
    "- Contention: **$(if ([bool]$runStatus.contention_skipped -and $formalRun) { 'FAIL: skipped in Full mode' } elseif ([bool]$runStatus.contention_skipped) { 'SKIPPED' } elseif ($contentionPassed) { 'PASS' } else { 'FAIL' })**",
    "- Missing contention evidence: $(if ($missingEvidence.Count -eq 0) { 'none' } else { $missingEvidence -join ', ' })",
    "",
    "Raw output remains under ``output/issue-8`` and is intentionally ignored by Git."
)
$markdown | Set-Content -LiteralPath $summaryMarkdownPath -Encoding utf8

Write-Host "Summary: $summaryMarkdownPath"
if (-not $overallPassed) {
    exit 1
}
