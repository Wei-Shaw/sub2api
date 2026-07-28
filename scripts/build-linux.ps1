#Requires -Version 5.1
<#
.SYNOPSIS
  在 Windows 本机交叉编译 Sub2API 的 Linux x86_64 (amd64) 二进制（含嵌入前端）。

.DESCRIPTION
  1. 在 frontend/ 执行 pnpm install + pnpm build（产物到 backend/internal/web/dist）
  2. 使用 GOOS=linux GOARCH=amd64 CGO_ENABLED=0 与 -tags embed 编译
  3. 输出默认：backend/sub2api-linux-amd64

  服务器架构须为 x86_64（uname -m 显示 x86_64）。

.PARAMETER SkipFrontend
  跳过前端构建（仅当 dist 已是最新且你明确知道在做什么时使用）。

.PARAMETER SkipPnpmInstall
  跳过 pnpm install，直接 pnpm build（依赖未变时可加快速度）。

.PARAMETER Output
  输出文件路径（相对仓库根或绝对路径）。默认 backend/sub2api-linux-amd64。

.PARAMETER Version
  写入二进制的版本号（-X main.Version）。默认：custom-yyyyMMdd-HHmm。

.EXAMPLE
  .\scripts\build-linux.ps1

.EXAMPLE
  .\scripts\build-linux.ps1 -SkipPnpmInstall

.EXAMPLE
  .\scripts\build-linux.ps1 -Version "1.2.3-custom" -Output "dist\sub2api"
#>
[CmdletBinding()]
param(
    [switch]$SkipFrontend,
    [switch]$SkipPnpmInstall,
    [string]$Output = "",
    [string]$Version = ""
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

function Write-Step {
    param([string]$Message)
    Write-Host ""
    Write-Host "==> $Message" -ForegroundColor Cyan
}

function Write-Ok {
    param([string]$Message)
    Write-Host "    $Message" -ForegroundColor Green
}

function Write-Err {
    param([string]$Message)
    Write-Host "ERROR: $Message" -ForegroundColor Red
}

function Test-CommandExists {
    param([string]$Name)
    return [bool](Get-Command $Name -ErrorAction SilentlyContinue)
}

function Get-RepoRoot {
    # 脚本位于 <repo>/scripts/build-linux.ps1
    $scriptDir = $PSScriptRoot
    if (-not $scriptDir) {
        $scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
    }
    $root = Resolve-Path (Join-Path $scriptDir "..")
    $frontendPkg = Join-Path $root "frontend\package.json"
    $backendMain = Join-Path $root "backend\cmd\server"
    if (-not (Test-Path $frontendPkg)) {
        throw "未找到 frontend/package.json，请确认脚本位于仓库的 scripts/ 目录下。当前根目录: $root"
    }
    if (-not (Test-Path $backendMain)) {
        throw "未找到 backend/cmd/server，请确认仓库结构完整。当前根目录: $root"
    }
    return $root.Path
}

function Restore-GoEnv {
    param(
        [AllowNull()][string]$PrevGoos,
        [AllowNull()][string]$PrevGoarch,
        [AllowNull()][string]$PrevCgo
    )
    if ($null -eq $PrevGoos -or $PrevGoos -eq "") {
        Remove-Item Env:GOOS -ErrorAction SilentlyContinue
    } else {
        $env:GOOS = $PrevGoos
    }
    if ($null -eq $PrevGoarch -or $PrevGoarch -eq "") {
        Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
    } else {
        $env:GOARCH = $PrevGoarch
    }
    if ($null -eq $PrevCgo -or $PrevCgo -eq "") {
        Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue
    } else {
        $env:CGO_ENABLED = $PrevCgo
    }
}

# --- main ---

$repoRoot = Get-RepoRoot
$frontendDir = Join-Path $repoRoot "frontend"
$backendDir = Join-Path $repoRoot "backend"
$distIndex = Join-Path $backendDir "internal\web\dist\index.html"

if ([string]::IsNullOrWhiteSpace($Output)) {
    $Output = Join-Path $backendDir "sub2api-linux-amd64"
} elseif (-not [System.IO.Path]::IsPathRooted($Output)) {
    $Output = Join-Path $repoRoot $Output
}
$Output = [System.IO.Path]::GetFullPath($Output)

if ([string]::IsNullOrWhiteSpace($Version)) {
    $Version = "custom-" + (Get-Date -Format "yyyyMMdd-HHmm")
}

$commit = "local"
if (Test-CommandExists "git") {
    try {
        Push-Location $repoRoot
        $gitSha = (git rev-parse --short HEAD 2>$null)
        if ($LASTEXITCODE -eq 0 -and $gitSha) {
            $commit = $gitSha.Trim()
        }
    } catch {
        # ignore
    } finally {
        Pop-Location
    }
}

$buildDate = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")

Write-Host "Sub2API Linux x86_64 (amd64) 构建" -ForegroundColor Yellow
Write-Host "  仓库根目录 : $repoRoot"
Write-Host "  目标平台   : linux/amd64 (x86_64)"
Write-Host "  版本       : $Version"
Write-Host "  Commit     : $commit"
Write-Host "  输出文件   : $Output"

# 依赖检查
Write-Step "检查本机工具"
if (-not (Test-CommandExists "go")) {
    Write-Err "未找到 go。请安装 Go 并确保在 PATH 中。"
    exit 1
}
$goVersion = (go version)
Write-Ok $goVersion

if (-not $SkipFrontend) {
    if (-not (Test-CommandExists "pnpm")) {
        Write-Err "未找到 pnpm。请执行: npm install -g pnpm"
        exit 1
    }
    Write-Ok "pnpm: $((pnpm --version))"
}

if ($SkipFrontend) {
    Write-Step "跳过前端构建 (-SkipFrontend)"
    if (-not (Test-Path $distIndex)) {
        Write-Err "未找到 $distIndex。跳过前端时必须已有完整 dist，请先不带 -SkipFrontend 构建一次。"
        exit 1
    }
    Write-Ok "已存在 dist/index.html"
} else {
    Write-Step "构建前端 (pnpm → backend/internal/web/dist)"
    Push-Location $frontendDir
    try {
        if (-not $SkipPnpmInstall) {
            Write-Host "    pnpm install ..."
            pnpm install
            if ($LASTEXITCODE -ne 0) { throw "pnpm install 失败 (exit $LASTEXITCODE)" }
        } else {
            Write-Ok "跳过 pnpm install (-SkipPnpmInstall)"
        }
        Write-Host "    pnpm build ..."
        pnpm build
        if ($LASTEXITCODE -ne 0) { throw "pnpm build 失败 (exit $LASTEXITCODE)" }
    } finally {
        Pop-Location
    }
    if (-not (Test-Path $distIndex)) {
        Write-Err "前端构建后仍未找到 dist/index.html: $distIndex"
        exit 1
    }
    Write-Ok "前端构建完成"
}

# 交叉编译：保存并恢复环境变量，避免影响本机后续开发
$prevGoos = $env:GOOS
$prevGoarch = $env:GOARCH
$prevCgo = $env:CGO_ENABLED

Write-Step "交叉编译 Go (linux/amd64, -tags embed)"
$outDir = Split-Path -Parent $Output
if ($outDir -and -not (Test-Path $outDir)) {
    New-Item -ItemType Directory -Path $outDir -Force | Out-Null
}

$ldflags = "-s -w -X main.Version=$Version -X main.Commit=$commit -X main.Date=$buildDate -X main.BuildType=release"

Push-Location $backendDir
try {
    $env:CGO_ENABLED = "0"
    $env:GOOS = "linux"
    $env:GOARCH = "amd64"

    Write-Host "    go build -tags embed -o $Output ./cmd/server"
    go build -tags embed -ldflags $ldflags -trimpath -o $Output ./cmd/server
    if ($LASTEXITCODE -ne 0) {
        throw "go build 失败 (exit $LASTEXITCODE)"
    }
} finally {
    Pop-Location
    Restore-GoEnv -PrevGoos $prevGoos -PrevGoarch $prevGoarch -PrevCgo $prevCgo
}

if (-not (Test-Path $Output)) {
    Write-Err "构建结束但未找到输出文件: $Output"
    exit 1
}

$item = Get-Item $Output
$sizeMB = [math]::Round($item.Length / 1MB, 2)

Write-Step "构建成功"
Write-Ok "文件: $($item.FullName)"
Write-Ok "大小: $sizeMB MB ($($item.Length) bytes)"
Write-Ok "时间: $($item.LastWriteTime)"
if ($item.Length -lt 5MB) {
    Write-Host "警告: 文件小于 5MB，可能未正确嵌入前端，请确认 pnpm build 与 -tags embed。" -ForegroundColor Yellow
}

Write-Host ""
Write-Host "下一步（示例）:" -ForegroundColor Yellow
Write-Host "  scp `"$($item.FullName)`" root@你的服务器IP:/opt/sub2api/sub2api.new"
Write-Host "  # 服务器上: stop → 替换二进制 → chmod +x → start"
Write-Host "  # 详见 docs/BUILD_LINUX_AMD64_CN.md"
Write-Host ""
