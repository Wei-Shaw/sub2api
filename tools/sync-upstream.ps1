param(
    [string]$TargetBranch = "custom/main",
    [string]$UpstreamRemote = "upstream",
    [string]$UpstreamBranch = "main",
    [string]$OriginRemote = "origin",
    [switch]$Push,
    [switch]$AllowDirty,
    [switch]$DryRun
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

if ($PSVersionTable.PSVersion.Major -ge 7) {
    $PSNativeCommandUseErrorActionPreference = $false
}

function Invoke-Git {
    param(
        [Parameter(Mandatory = $true)]
        [string[]]$Args,
        [switch]$AllowFailure
    )

    $output = & git @Args 2>&1
    $exitCode = $LASTEXITCODE

    if (-not $AllowFailure -and $exitCode -ne 0) {
        $rendered = ($output | Out-String).Trim()
        if ($rendered) {
            throw "git $($Args -join ' ') failed:`n$rendered"
        }

        throw "git $($Args -join ' ') failed with exit code $exitCode."
    }

    return [pscustomobject]@{
        Output   = @($output)
        ExitCode = $exitCode
    }
}

function Write-Section {
    param([string]$Message)
    Write-Host ""
    Write-Host "==> $Message" -ForegroundColor Cyan
}

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
Push-Location $repoRoot

try {
    Write-Section "Repository"
    $topLevel = (Invoke-Git -Args @("rev-parse", "--show-toplevel")).Output[0].Trim()
    $currentBranch = (Invoke-Git -Args @("branch", "--show-current")).Output[0].Trim()
    Write-Host "Repo root: $topLevel"
    Write-Host "Current branch: $currentBranch"
    Write-Host "Target branch: $TargetBranch"
    Write-Host "Upstream ref: $UpstreamRemote/$UpstreamBranch"

    if (-not $AllowDirty) {
        $statusLines = (Invoke-Git -Args @("status", "--porcelain")).Output
        $hasChanges = ($statusLines | Measure-Object).Count -gt 0
        if ($hasChanges) {
            throw "Working tree is not clean. Commit or stash changes first, or rerun with -AllowDirty."
        }
    }

    Write-Section "Fetching remotes"
    Invoke-Git -Args @("fetch", $UpstreamRemote) | Out-Null
    Write-Host "Fetched $UpstreamRemote"

    $upstreamRef = "$UpstreamRemote/$UpstreamBranch"

    Invoke-Git -Args @("rev-parse", "--verify", $TargetBranch) | Out-Null
    Invoke-Git -Args @("rev-parse", "--verify", $upstreamRef) | Out-Null

    if ($currentBranch -ne $TargetBranch) {
        Write-Section "Checking out target branch"
        Invoke-Git -Args @("checkout", $TargetBranch) | Out-Null
        $currentBranch = $TargetBranch
        Write-Host "Switched to $TargetBranch"
    }

    Write-Section "Comparing branches"
    $countLine = (Invoke-Git -Args @("rev-list", "--left-right", "--count", "$TargetBranch...$upstreamRef")).Output[0].Trim()
    $parts = $countLine -split "\s+"
    if ($parts.Count -lt 2) {
        throw "Unexpected rev-list output: $countLine"
    }

    $targetAhead = [int]$parts[0]
    $upstreamAhead = [int]$parts[1]
    Write-Host "$TargetBranch ahead: $targetAhead"
    Write-Host "$upstreamRef ahead: $upstreamAhead"

    if ($upstreamAhead -gt 0) {
        Write-Host ""
        Write-Host "Upstream commits waiting to merge:" -ForegroundColor Yellow
        $pending = Invoke-Git -Args @("log", "--oneline", "--decorate", "$TargetBranch..$upstreamRef")
        $pending.Output | ForEach-Object { Write-Host $_ }
    } else {
        Write-Host "No new upstream commits to merge." -ForegroundColor Green
    }

    if ($DryRun) {
        Write-Section "Dry run"
        Write-Host "No changes were made."
        return
    }

    if ($upstreamAhead -eq 0) {
        Write-Section "Result"
        Write-Host "$TargetBranch is already up to date with $upstreamRef." -ForegroundColor Green
    } else {
        Write-Section "Merging upstream"
        $merge = Invoke-Git -Args @("merge", $upstreamRef) -AllowFailure
        if ($merge.ExitCode -ne 0) {
            $merge.Output | ForEach-Object { Write-Host $_ }
            $conflicts = (Invoke-Git -Args @("diff", "--name-only", "--diff-filter=U") -AllowFailure).Output
            if (($conflicts | Measure-Object).Count -gt 0) {
                Write-Host ""
                Write-Host "Merge conflicts detected:" -ForegroundColor Yellow
                $conflicts | ForEach-Object { Write-Host $_ }
                throw "Resolve conflicts, then run 'git add <files>' and 'git commit' to finish the merge."
            }

            throw "Merge failed. Review the git output above."
        }

        $merge.Output | ForEach-Object { Write-Host $_ }
        Write-Host "$TargetBranch now includes $upstreamRef." -ForegroundColor Green
    }

    if ($Push) {
        Write-Section "Pushing target branch"
        Invoke-Git -Args @("push", $OriginRemote, $TargetBranch) | Out-Null
        Write-Host "Pushed $TargetBranch to $OriginRemote." -ForegroundColor Green
    } else {
        Write-Host ""
        Write-Host "Tip: run 'git push $OriginRemote $TargetBranch' after checking the result." -ForegroundColor DarkGray
    }
}
finally {
    Pop-Location
}
