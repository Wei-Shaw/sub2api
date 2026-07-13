[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$runner = Join-Path $PSScriptRoot "run.ps1"
$powershell = Join-Path $PSHOME "pwsh.exe"
$tempRoot = [System.IO.Path]::GetFullPath([System.IO.Path]::GetTempPath())
$testRoot = [System.IO.Path]::GetFullPath(
    (Join-Path $tempRoot ("s2a-issue8-run-tests-{0}" -f [guid]::NewGuid().ToString("N")))
)
if (-not $testRoot.StartsWith($tempRoot, [System.StringComparison]::OrdinalIgnoreCase)) {
    throw "test directory escaped the system temp root: $testRoot"
}

function Write-PassingFixture {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Directory,

        [Parameter(Mandatory = $true)]
        [ValidateSet("Quick", "Full")]
        [string]$Mode,

        [Parameter(Mandatory = $true)]
        [bool]$GitDirty,

        [Parameter(Mandatory = $true)]
        [bool]$ContentionSkipped,

        [bool]$ResourceCleanupPassed = $true
    )

    New-Item -ItemType Directory -Force -Path $Directory | Out-Null
    @(
        "BenchmarkIssue8GatewayAdmissionLifecycle/flag_off_legacy_short-8 100 1000000 ns/op",
        "BenchmarkIssue8GatewayAdmissionLifecycle/flag_on_standard_short-8 100 1500000 ns/op",
        "BenchmarkIssue8GatewayAdmissionLifecycle/flag_on_extra_short-8 100 1800000 ns/op",
        "BenchmarkIssue8GatewayAdmissionLifecycle/flag_on_poll_once-8 100 20000000 ns/op"
    ) | Set-Content -LiteralPath (Join-Path $Directory "benchmark.txt") -Encoding utf8
    "capacity_breach=0 reserve_breach=0 account_breach=0" |
        Set-Content -LiteralPath (Join-Path $Directory "contention.txt") -Encoding utf8

    [ordered]@{
        generated_at_utc = [DateTimeOffset]::UtcNow.ToString("o")
        mode = $Mode
        go_image = "test-go-image"
        redis_image = "test-redis-image"
        git_commit = "test-commit"
        git_dirty = $GitDirty
        benchmark_exit_code = 0
        contention_exit_code = 0
        contention_skipped = $ContentionSkipped
        work_error = ""
        resource_cleanup = [ordered]@{
            passed = $ResourceCleanupPassed
            run_id = "test-run"
            failures = if ($ResourceCleanupPassed) { @() } else { @("test cleanup failure") }
            remaining = [ordered]@{
                runner_containers = @()
                runner_networks = @()
                testcontainers = @()
                test_networks = @()
                anonymous_volumes = @()
            }
        }
    } | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath (Join-Path $Directory "run-status.json") -Encoding utf8
}

function Assert-RunnerExitCode {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Name,

        [Parameter(Mandatory = $true)]
        [string]$OutputDirectory,

        [Parameter(Mandatory = $true)]
        [int]$ExpectedExitCode
    )

    $output = @(& $powershell -NoProfile -NonInteractive -File $runner -SummarizeOnly -OutputDirectory $OutputDirectory 2>&1)
    $actualExitCode = $LASTEXITCODE
    if ($actualExitCode -ne $ExpectedExitCode) {
        throw "$Name expected exit code $ExpectedExitCode but received $actualExitCode.`n$($output -join "`n")"
    }
    Write-Host "PASS: $Name"
}

function Write-FakeDockerCommand {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )

    $implementationPath = [System.IO.Path]::ChangeExtension($Path, ".impl.ps1")
    @'
Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$dockerArgs = @($args)
$commandLine = $dockerArgs -join " "
$stateDirectory = $env:S2A_ISSUE8_FAKE_DOCKER_STATE
$scenario = $env:S2A_ISSUE8_FAKE_DOCKER_SCENARIO
$workCompletedPath = Join-Path $stateDirectory "work-completed"
$containerRemovedPath = Join-Path $stateDirectory "container-removed"
$networkRemovedPath = Join-Path $stateDirectory "network-removed"
$volumeRemovedPath = Join-Path $stateDirectory "volume-removed"
$dockerArgs -join "`t" | Add-Content -LiteralPath (Join-Path $stateDirectory "calls.log") -Encoding utf8

function Complete-FakeCommand {
    param([int]$ExitCode = 0)
    exit $ExitCode
}

if ($dockerArgs.Count -ge 2 -and $dockerArgs[0] -eq "image" -and $dockerArgs[1] -eq "inspect") {
    Complete-FakeCommand
}

if ($dockerArgs.Count -ge 1 -and $dockerArgs[0] -eq "ps") {
    if ($commandLine.Contains("com.sub2api.test.run") -and (Test-Path -LiteralPath $workCompletedPath)) {
        if (-not (Test-Path -LiteralPath $containerRemovedPath)) {
            "tc-owned"
        }
    } elseif ($commandLine.Contains("org.testcontainers=true") -and (Test-Path -LiteralPath $workCompletedPath)) {
        "tc-owned"
        if ($scenario -eq "success-with-unrelated") {
            "tc-unrelated"
        }
    }
    Complete-FakeCommand
}

if ($dockerArgs.Count -ge 2 -and $dockerArgs[0] -eq "network" -and $dockerArgs[1] -eq "ls") {
    if ($commandLine.Contains("com.sub2api.test.run") -and (Test-Path -LiteralPath $workCompletedPath)) {
        if (-not (Test-Path -LiteralPath $networkRemovedPath)) {
            "net-owned"
        }
    } elseif ($commandLine.Contains("org.testcontainers=true") -and (Test-Path -LiteralPath $workCompletedPath)) {
        "net-owned"
        if ($scenario -eq "success-with-unrelated") {
            "net-unrelated"
        }
    }
    Complete-FakeCommand
}

if ($dockerArgs.Count -ge 2 -and $dockerArgs[0] -eq "volume" -and $dockerArgs[1] -eq "ls") {
    if (-not $commandLine.Contains("com.sub2api.test.run") -and (Test-Path -LiteralPath $workCompletedPath)) {
        if (-not (Test-Path -LiteralPath $volumeRemovedPath)) {
            "vol-owned"
        }
        if ($scenario -eq "success-with-unrelated") {
            "vol-unrelated"
        }
    }
    Complete-FakeCommand
}

if ($dockerArgs.Count -ge 2 -and $dockerArgs[0] -eq "network" -and $dockerArgs[1] -eq "create") {
    "runner-network-id"
    Complete-FakeCommand
}

if ($dockerArgs.Count -ge 1 -and $dockerArgs[0] -eq "run") {
    if ($dockerArgs -contains "-d") {
        if ($scenario -eq "setup-failure") {
            "simulated Redis startup failure"
            Complete-FakeCommand -ExitCode 31
        }
        "runner-redis-id"
        Complete-FakeCommand
    }
    if ($dockerArgs -contains "-bench") {
        "BenchmarkIssue8GatewayAdmissionLifecycle/flag_off_legacy_short-8 100 1000000 ns/op"
        "BenchmarkIssue8GatewayAdmissionLifecycle/flag_on_standard_short-8 100 1500000 ns/op"
        "BenchmarkIssue8GatewayAdmissionLifecycle/flag_on_extra_short-8 100 1800000 ns/op"
        "BenchmarkIssue8GatewayAdmissionLifecycle/flag_on_poll_once-8 100 20000000 ns/op"
        Complete-FakeCommand
    }
    if ($dockerArgs -contains "-tags=integration") {
        New-Item -ItemType File -Force -Path $workCompletedPath | Out-Null
        "capacity_breach=0 reserve_breach=0 account_breach=0"
        Complete-FakeCommand
    }
}

if ($dockerArgs.Count -ge 1 -and $dockerArgs[0] -eq "exec") {
    "PONG"
    Complete-FakeCommand
}

if ($dockerArgs.Count -ge 1 -and $dockerArgs[0] -eq "inspect") {
    if ($dockerArgs -contains "tc-owned") {
        "vol-owned"
    }
    Complete-FakeCommand
}

if ($dockerArgs.Count -ge 1 -and $dockerArgs[0] -eq "rm") {
    if ($dockerArgs -contains "tc-owned") {
        if ($scenario -eq "command-failure") {
            "simulated container removal failure"
            Complete-FakeCommand -ExitCode 23
        }
        if ($scenario -ne "residue-all") {
            New-Item -ItemType File -Force -Path $containerRemovedPath | Out-Null
        }
    }
    Complete-FakeCommand
}

if ($dockerArgs.Count -ge 2 -and $dockerArgs[0] -eq "network" -and $dockerArgs[1] -eq "rm") {
    if ($dockerArgs -contains "net-owned" -and $scenario -ne "residue-all") {
        New-Item -ItemType File -Force -Path $networkRemovedPath | Out-Null
    }
    Complete-FakeCommand
}

if ($dockerArgs.Count -ge 2 -and $dockerArgs[0] -eq "volume" -and $dockerArgs[1] -eq "rm") {
    if ($dockerArgs -contains "vol-owned" -and $scenario -ne "residue-all") {
        New-Item -ItemType File -Force -Path $volumeRemovedPath | Out-Null
    }
    Complete-FakeCommand
}

Complete-FakeCommand
'@ | Set-Content -LiteralPath $implementationPath -Encoding utf8

    @(
        "@echo off",
        "`"$powershell`" -NoProfile -NonInteractive -File `"$implementationPath`" %*",
        "exit /b %ERRORLEVEL%"
    ) | Set-Content -LiteralPath $Path -Encoding ascii
}

function Assert-FullCleanupFailure {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Name,

        [Parameter(Mandatory = $true)]
        [string]$Scenario,

        [Parameter(Mandatory = $true)]
        [string]$ExpectedFailurePattern,

        [hashtable]$ExpectedRemaining = @{}
    )

    $scenarioRoot = Join-Path $testRoot $Scenario
    $stateDirectory = Join-Path $scenarioRoot "docker-state"
    $outputDirectory = Join-Path $scenarioRoot "output"
    $fakeDocker = Join-Path $scenarioRoot "docker.cmd"
    New-Item -ItemType Directory -Force -Path $stateDirectory | Out-Null
    New-Item -ItemType Directory -Force -Path $outputDirectory | Out-Null
    "STALE PASS SUMMARY" | Set-Content -LiteralPath (Join-Path $outputDirectory "summary.md") -Encoding utf8
    "STALE BENCHMARK" | Set-Content -LiteralPath (Join-Path $outputDirectory "benchmark.txt") -Encoding utf8
    Write-FakeDockerCommand -Path $fakeDocker

    $previousState = $env:S2A_ISSUE8_FAKE_DOCKER_STATE
    $previousScenario = $env:S2A_ISSUE8_FAKE_DOCKER_SCENARIO
    try {
        $env:S2A_ISSUE8_FAKE_DOCKER_STATE = $stateDirectory
        $env:S2A_ISSUE8_FAKE_DOCKER_SCENARIO = $Scenario
        $output = @(& $powershell `
            -NoProfile `
            -NonInteractive `
            -File $runner `
            -Mode Full `
            -GoImage "fake-go-image" `
            -RedisImage "fake-redis-image" `
            -DockerCommand $fakeDocker `
            -OutputDirectory $outputDirectory 2>&1)
        $actualExitCode = $LASTEXITCODE
    } finally {
        $env:S2A_ISSUE8_FAKE_DOCKER_STATE = $previousState
        $env:S2A_ISSUE8_FAKE_DOCKER_SCENARIO = $previousScenario
    }

    if ($actualExitCode -ne 1) {
        throw "$Name expected exit code 1 but received $actualExitCode.`n$($output -join "`n")"
    }
    $statusPath = Join-Path $outputDirectory "run-status.json"
    if (-not (Test-Path -LiteralPath $statusPath)) {
        throw "$Name did not write cleanup evidence.`n$($output -join "`n")"
    }
    $status = Get-Content -Raw -LiteralPath $statusPath | ConvertFrom-Json
    if ($null -eq $status.resource_cleanup -or [bool]$status.resource_cleanup.passed) {
        throw "$Name did not record a failed cleanup.`n$($output -join "`n")"
    }
    $failureText = @($status.resource_cleanup.failures) -join "`n"
    if ($failureText -notmatch $ExpectedFailurePattern) {
        throw "$Name did not record the expected failure '$ExpectedFailurePattern'.`n$failureText"
    }
    $summaryPath = Join-Path $outputDirectory "summary.md"
    $summaryText = if (Test-Path -LiteralPath $summaryPath) {
        Get-Content -Raw -LiteralPath $summaryPath
    } else {
        ""
    }
    if ($summaryText -notmatch $ExpectedFailurePattern) {
        throw "$Name did not expose the cleanup failure in summary.md.`n$summaryText"
    }
    foreach ($entry in $ExpectedRemaining.GetEnumerator()) {
        $property = $status.resource_cleanup.remaining.PSObject.Properties[$entry.Key]
        $actual = if ($null -eq $property) { @() } else { @($property.Value) }
        if ($entry.Value -notin $actual) {
            throw "$Name did not record remaining $($entry.Key) '$($entry.Value)'. Actual: $($actual -join ', ')"
        }
    }
    Write-Host "PASS: $Name"
}

function Assert-RunnerCleanupSuccess {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Name
    )

    $scenario = "success-with-unrelated"
    $scenarioRoot = Join-Path $testRoot $scenario
    $stateDirectory = Join-Path $scenarioRoot "docker-state"
    $outputDirectory = Join-Path $scenarioRoot "output"
    $fakeDocker = Join-Path $scenarioRoot "docker.cmd"
    New-Item -ItemType Directory -Force -Path $stateDirectory | Out-Null
    New-Item -ItemType Directory -Force -Path $outputDirectory | Out-Null
    "STALE PASS SUMMARY" | Set-Content -LiteralPath (Join-Path $outputDirectory "summary.md") -Encoding utf8
    "STALE BENCHMARK" | Set-Content -LiteralPath (Join-Path $outputDirectory "benchmark.txt") -Encoding utf8
    Write-FakeDockerCommand -Path $fakeDocker

    $previousState = $env:S2A_ISSUE8_FAKE_DOCKER_STATE
    $previousScenario = $env:S2A_ISSUE8_FAKE_DOCKER_SCENARIO
    try {
        $env:S2A_ISSUE8_FAKE_DOCKER_STATE = $stateDirectory
        $env:S2A_ISSUE8_FAKE_DOCKER_SCENARIO = $scenario
        $output = @(& $powershell `
            -NoProfile `
            -NonInteractive `
            -File $runner `
            -Mode Quick `
            -GoImage "fake-go-image" `
            -RedisImage "fake-redis-image" `
            -DockerCommand $fakeDocker `
            -OutputDirectory $outputDirectory 2>&1)
        $actualExitCode = $LASTEXITCODE
    } finally {
        $env:S2A_ISSUE8_FAKE_DOCKER_STATE = $previousState
        $env:S2A_ISSUE8_FAKE_DOCKER_SCENARIO = $previousScenario
    }

    if ($actualExitCode -ne 0) {
        throw "$Name expected exit code 0 but received $actualExitCode.`n$($output -join "`n")"
    }
    $status = Get-Content -Raw -LiteralPath (Join-Path $outputDirectory "run-status.json") | ConvertFrom-Json
    if (-not [bool]$status.resource_cleanup.passed) {
        throw "$Name recorded a failed cleanup: $(@($status.resource_cleanup.failures) -join '; ')"
    }
    $summaryText = Get-Content -Raw -LiteralPath (Join-Path $outputDirectory "summary.md")
    if (-not $summaryText.Contains("- Docker cleanup failures: none")) {
        throw "$Name did not report an empty cleanup failure list as none.`n$summaryText"
    }
    $calls = Get-Content -Raw -LiteralPath (Join-Path $stateDirectory "calls.log")
    $runID = "$($status.resource_cleanup.run_id)"
    if (-not $calls.Contains("SUB2API_TEST_RUN_ID=$runID")) {
        throw "$Name did not inject its test run ID into the contention run."
    }
    if (-not $calls.Contains("label=com.sub2api.test.run=$runID")) {
        throw "$Name did not use its custom run label during cleanup."
    }
    $removalLines = @($calls -split "`r?`n" | Where-Object {
        $_ -match '(^|\t)(rm|network\trm|volume\trm)(\t|$)'
    })
    if (($removalLines -join "`n").Contains("unrelated")) {
        throw "$Name attempted to remove an unrelated resource.`n$($removalLines -join "`n")"
    }
    Write-Host "PASS: $Name"
}

function Assert-RunnerSetupFailureWritesEvidence {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Name
    )

    $scenario = "setup-failure"
    $scenarioRoot = Join-Path $testRoot $scenario
    $stateDirectory = Join-Path $scenarioRoot "docker-state"
    $outputDirectory = Join-Path $scenarioRoot "output"
    $fakeDocker = Join-Path $scenarioRoot "docker.cmd"
    New-Item -ItemType Directory -Force -Path $stateDirectory | Out-Null
    New-Item -ItemType Directory -Force -Path $outputDirectory | Out-Null
    "STALE PASS SUMMARY" | Set-Content -LiteralPath (Join-Path $outputDirectory "summary.md") -Encoding utf8
    "STALE BENCHMARK" | Set-Content -LiteralPath (Join-Path $outputDirectory "benchmark.txt") -Encoding utf8
    Write-FakeDockerCommand -Path $fakeDocker

    $previousState = $env:S2A_ISSUE8_FAKE_DOCKER_STATE
    $previousScenario = $env:S2A_ISSUE8_FAKE_DOCKER_SCENARIO
    try {
        $env:S2A_ISSUE8_FAKE_DOCKER_STATE = $stateDirectory
        $env:S2A_ISSUE8_FAKE_DOCKER_SCENARIO = $scenario
        $output = @(& $powershell `
            -NoProfile `
            -NonInteractive `
            -File $runner `
            -Mode Full `
            -GoImage "fake-go-image" `
            -RedisImage "fake-redis-image" `
            -DockerCommand $fakeDocker `
            -OutputDirectory $outputDirectory 2>&1)
        $actualExitCode = $LASTEXITCODE
    } finally {
        $env:S2A_ISSUE8_FAKE_DOCKER_STATE = $previousState
        $env:S2A_ISSUE8_FAKE_DOCKER_SCENARIO = $previousScenario
    }

    if ($actualExitCode -ne 1) {
        throw "$Name expected exit code 1 but received $actualExitCode.`n$($output -join "`n")"
    }
    $statusPath = Join-Path $outputDirectory "run-status.json"
    if (-not (Test-Path -LiteralPath $statusPath)) {
        throw "$Name did not write run-status.json.`n$($output -join "`n")"
    }
    $status = Get-Content -Raw -LiteralPath $statusPath | ConvertFrom-Json
    if ("$($status.work_error)" -notmatch "failed to start Redis container") {
        throw "$Name did not record the setup failure: $($status.work_error)"
    }
    if ($null -eq $status.resource_cleanup -or -not [bool]$status.resource_cleanup.passed) {
        throw "$Name did not record successful post-failure cleanup."
    }
    $summaryText = Get-Content -Raw -LiteralPath (Join-Path $outputDirectory "summary.md")
    if ($summaryText -notmatch "Runner work: \*\*FAIL\*\*" -or $summaryText -notmatch "failed to start Redis container") {
        throw "$Name did not expose setup failure evidence in summary.md.`n$summaryText"
    }
    if ($summaryText.Contains("STALE PASS SUMMARY")) {
        throw "$Name left stale PASS evidence in summary.md."
    }
    $calls = Get-Content -Raw -LiteralPath (Join-Path $stateDirectory "calls.log")
    if ($calls -notmatch "network\trm\ts2a-issue8-") {
        throw "$Name did not remove the network created before setup failed.`n$calls"
    }
    Write-Host "PASS: $Name"
}

try {
    $fullDirty = Join-Path $testRoot "full-dirty"
    Write-PassingFixture -Directory $fullDirty -Mode Full -GitDirty $true -ContentionSkipped $false
    Assert-RunnerExitCode -Name "Full rejects a dirty worktree" -OutputDirectory $fullDirty -ExpectedExitCode 1

    $fullSkipped = Join-Path $testRoot "full-skipped"
    Write-PassingFixture -Directory $fullSkipped -Mode Full -GitDirty $false -ContentionSkipped $true
    Assert-RunnerExitCode -Name "Full rejects skipped contention" -OutputDirectory $fullSkipped -ExpectedExitCode 1

    $fullCleanupFailed = Join-Path $testRoot "full-cleanup-failed"
    Write-PassingFixture `
        -Directory $fullCleanupFailed `
        -Mode Full `
        -GitDirty $false `
        -ContentionSkipped $false `
        -ResourceCleanupPassed $false
    Assert-RunnerExitCode -Name "Full rejects failed resource cleanup" -OutputDirectory $fullCleanupFailed -ExpectedExitCode 1

    Assert-FullCleanupFailure `
        -Name "Full records a failed Docker cleanup command" `
        -Scenario "command-failure" `
        -ExpectedFailurePattern "container.*tc-owned.*exit code 23"

    Assert-FullCleanupFailure `
        -Name "Full rejects Docker resources that remain after cleanup" `
        -Scenario "residue-all" `
        -ExpectedFailurePattern "remain after cleanup" `
        -ExpectedRemaining @{
            testcontainers = "tc-owned"
            test_networks = "net-owned"
            anonymous_volumes = "vol-owned"
        }

    Assert-RunnerSetupFailureWritesEvidence -Name "Runner records setup failures and still verifies cleanup"

    Assert-RunnerCleanupSuccess -Name "Runner scopes cleanup to its own Docker resources"

    $quickDirtySkipped = Join-Path $testRoot "quick-dirty-skipped"
    Write-PassingFixture -Directory $quickDirtySkipped -Mode Quick -GitDirty $true -ContentionSkipped $true
    Assert-RunnerExitCode -Name "Quick allows dirty worktrees and skipped contention" -OutputDirectory $quickDirtySkipped -ExpectedExitCode 0
} finally {
    Remove-Item -LiteralPath $testRoot -Recurse -Force -ErrorAction SilentlyContinue
}
