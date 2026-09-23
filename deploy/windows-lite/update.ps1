param(
    [int]$Port = 8080,
    [switch]$NoRestart,
    [switch]$ForceDownload,
    [string]$Version,
    [string]$DownloadProxy = $env:SUB2API_DOWNLOAD_PROXY
)

$ErrorActionPreference = "Stop"

$Root = Split-Path -Parent $MyInvocation.MyCommand.Path
$Runtime = Join-Path $Root "runtime"
$Downloads = Join-Path $Runtime "downloads"
$ExtractDir = Join-Path $Downloads "latest"
$Exe = Join-Path $Runtime "sub2api.exe"
$VersionFile = Join-Path $Runtime "sub2api.version.txt"
$AppPid = Join-Path $Runtime "sub2api.pid"
$FrontendPid = Join-Path $Runtime "admin_frontend.pid"
$FrontendServer = Join-Path $Root "admin_frontend_server.py"
$RedisPid = Join-Path $Runtime "mini-redis\mini_redis.pid"
$RedisScript = Join-Path $Runtime "mini-redis\mini_redis.py"
$StartScript = Join-Path $Root "start.ps1"

function Write-Step($Message) {
    Write-Host "[Sub2API Update] $Message" -ForegroundColor Cyan
}

function Assert-Release($Release) {
    if (-not $Release) {
        throw "Unable to determine the latest Sub2API release. GitHub may be rate-limiting this network. Try again later, or run with -Version v0.1.162."
    }

    if ([string]::IsNullOrWhiteSpace($Release.tag_name) -or
        [string]::IsNullOrWhiteSpace($Release.asset_name) -or
        [string]::IsNullOrWhiteSpace($Release.download_url)) {
        throw "Release lookup returned incomplete data. tag='$($Release.tag_name)', asset='$($Release.asset_name)', url='$($Release.download_url)'"
    }
}

function Test-PESignature($Path) {
    if (-not (Test-Path $Path)) { return $false }

    try {
        $bytes = Get-Content $Path -Encoding Byte -TotalCount 2
        return ($bytes.Length -ge 2 -and $bytes[0] -eq 0x4D -and $bytes[1] -eq 0x5A)
    }
    catch {
        return $false
    }
}

function Test-ZipArchive($Path) {
    if (-not (Test-Path $Path)) { return $false }

    try {
        Add-Type -AssemblyName System.IO.Compression.FileSystem -ErrorAction SilentlyContinue
        $archive = [System.IO.Compression.ZipFile]::OpenRead($Path)
        $archive.Dispose()
        return $true
    }
    catch {
        return $false
    }
}

function Find-Python {
    $python = Get-Command python -ErrorAction SilentlyContinue
    if ($python) { return [pscustomobject]@{ File = $python.Source; Args = @() } }

    $python3 = Get-Command python3 -ErrorAction SilentlyContinue
    if ($python3) { return [pscustomobject]@{ File = $python3.Source; Args = @() } }

    $py = Get-Command py -ErrorAction SilentlyContinue
    if ($py) { return [pscustomobject]@{ File = $py.Source; Args = @("-3") } }

    return $null
}

function Get-LatestReleaseViaPython {
    $python = Find-Python
    if (-not $python) {
        throw "Python 3 was not found, so Python fallback release lookup is unavailable."
    }

    $script = @"
import json
import sys
import urllib.request

api = "https://api.github.com/repos/Wei-Shaw/sub2api/releases/latest"
headers = {"User-Agent": "Sub2API-windows-lite-update"}
req = urllib.request.Request(api, headers=headers)
with urllib.request.urlopen(req, timeout=20) as r:
    data = json.load(r)

asset = None
for item in data.get("assets", []):
    if item.get("name", "").endswith("windows_amd64.zip"):
        asset = item
        break

if asset is None:
    raise RuntimeError("No windows_amd64.zip release asset was found.")

print(json.dumps({
    "tag_name": data.get("tag_name") or data.get("name") or "",
    "asset_name": asset["name"],
    "download_url": asset["browser_download_url"],
}, ensure_ascii=False))
"@

    $tempScript = Join-Path $Downloads "get_latest_release.py"
    Set-Content -Path $tempScript -Value $script -Encoding UTF8
    try {
        $args = @($python.Args) + @($tempScript)
        $json = (& $python.File @args 2>&1 | Select-Object -Last 1)
        if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($json)) {
            throw "Python release lookup failed: $json"
        }

        $release = $json | ConvertFrom-Json
        Assert-Release $release
        return $release
    }
    finally {
        Remove-Item $tempScript -Force -ErrorAction SilentlyContinue
    }
}

function Get-LatestTagFromRedirect {
    $latestUrl = "https://github.com/Wei-Shaw/sub2api/releases/latest"
    $location = $null

    try {
        $request = [System.Net.WebRequest]::Create($latestUrl)
        $request.Method = "HEAD"
        $request.AllowAutoRedirect = $false
        $request.UserAgent = "Sub2API-windows-lite-update"
        $response = $request.GetResponse()
        $location = $response.Headers["Location"]
        $response.Dispose()
    }
    catch {
        $response = $_.Exception.Response
        if ($response) {
            $location = $response.Headers["Location"]
            $response.Dispose()
        }
        else {
            throw
        }
    }

    if ([string]::IsNullOrWhiteSpace($location) -or $location -notmatch "/releases/tag/([^/?#]+)") {
        throw "Could not resolve the latest release tag from GitHub redirect."
    }

    return [uri]::UnescapeDataString($Matches[1])
}

function New-ReleaseFromTag($Tag) {
    if ([string]::IsNullOrWhiteSpace($Tag)) {
        throw "Release tag is empty."
    }

    $normalized = $Tag.Trim()
    $versionPart = $normalized.TrimStart("v")
    $assetName = "sub2api_${versionPart}_windows_amd64.zip"
    return [pscustomobject]@{
        tag_name = $normalized
        asset_name = $assetName
        download_url = "https://github.com/Wei-Shaw/sub2api/releases/download/$normalized/$assetName"
    }
}

function Get-DownloadUrls($Url) {
    $urls = New-Object System.Collections.Generic.List[string]
    $urls.Add($Url)

    if (-not [string]::IsNullOrWhiteSpace($DownloadProxy)) {
        $proxy = $DownloadProxy.Trim()
        if ($proxy.Contains("{url}")) {
            $urls.Add($proxy.Replace("{url}", $Url))
        }
        else {
            $urls.Add($proxy.TrimEnd("/") + "/" + $Url)
        }
    }

    return $urls | Select-Object -Unique
}

function Get-LatestRelease {
    if (-not [string]::IsNullOrWhiteSpace($Version)) {
        $release = New-ReleaseFromTag $Version
        Assert-Release $release
        return $release
    }

    try {
        $release = Invoke-RestMethod -Uri "https://api.github.com/repos/Wei-Shaw/sub2api/releases/latest" -Headers @{ "User-Agent" = "Sub2API-windows-lite-update" }
        $asset = $release.assets | Where-Object { $_.name -match "windows_amd64\.zip$" } | Select-Object -First 1
        if (-not $asset) {
            throw "No windows_amd64.zip release asset was found."
        }

        return [pscustomobject]@{
            tag_name = if ($release.tag_name) { $release.tag_name } else { $release.name }
            asset_name = $asset.name
            download_url = $asset.browser_download_url
        }
    }
    catch {
        Write-Warning "PowerShell release lookup failed, retrying with Python. Reason: $($_.Exception.Message)"
        try {
            return Get-LatestReleaseViaPython
        }
        catch {
            Write-Warning "Python release lookup failed, retrying with GitHub latest redirect. Reason: $($_.Exception.Message)"
            $tag = Get-LatestTagFromRedirect
            $release = New-ReleaseFromTag $tag
            Assert-Release $release
            return $release
        }
    }
}

function Remove-DownloadedFile($Path) {
    if ([string]::IsNullOrWhiteSpace($Path)) { return }
    if ((Test-Path $Path -PathType Leaf)) {
        Remove-Item $Path -Force -ErrorAction SilentlyContinue
    }
}

function Download-Asset($Release) {
    Assert-Release $Release
    New-Item -ItemType Directory -Force -Path $Downloads | Out-Null

    $zip = Join-Path $Downloads $Release.asset_name
    $partialZip = "$zip.part"
    if ((Test-Path $zip -PathType Container)) {
        throw "Refusing to use a directory as the download file: $zip"
    }

    if ((Test-Path $zip) -and -not $ForceDownload) {
        if (Test-ZipArchive $zip) {
            Write-Step "Using existing package: $zip"
            return $zip
        }

        Write-Warning "Existing package is incomplete or invalid. Downloading again: $zip"
        Remove-DownloadedFile $zip
    }
    Remove-DownloadedFile $partialZip

    $downloadErrors = New-Object System.Collections.Generic.List[string]
    foreach ($downloadUrl in (Get-DownloadUrls $Release.download_url)) {
        Write-Step "Downloading latest package: $($Release.asset_name)"
        if ($downloadUrl -ne $Release.download_url) {
            Write-Step "Using download proxy: $DownloadProxy"
        }

        $curl = Get-Command curl.exe -ErrorAction SilentlyContinue
        if ($curl) {
            Remove-DownloadedFile $partialZip
            Write-Step "Downloading with curl.exe..."
            & $curl.Source -L --fail --retry 2 --retry-delay 2 --connect-timeout 10 --max-time 180 -A "Sub2API-windows-lite-update" -o $partialZip $downloadUrl
            if ($LASTEXITCODE -eq 0 -and (Test-ZipArchive $partialZip)) {
                Move-Item -Force $partialZip $zip
                return $zip
            }

            $downloadErrors.Add("curl.exe exit code: $LASTEXITCODE")
            Remove-DownloadedFile $partialZip
            Write-Warning "curl.exe download failed or produced an invalid zip."
        }

        try {
            Remove-DownloadedFile $partialZip
            Write-Step "Retrying download with PowerShell..."
            Invoke-WebRequest -Uri $downloadUrl -OutFile $partialZip -Headers @{ "User-Agent" = "Sub2API-windows-lite-update" } -TimeoutSec 180
            if (Test-ZipArchive $partialZip) {
                Move-Item -Force $partialZip $zip
                return $zip
            }

            Remove-DownloadedFile $partialZip
            throw "PowerShell download produced an invalid zip."
        }
        catch {
            $downloadErrors.Add("PowerShell: $($_.Exception.Message)")
            Write-Warning "PowerShell download failed. Reason: $($_.Exception.Message)"
            Remove-DownloadedFile $partialZip

            $python = Find-Python
            if (-not $python) { continue }

            Write-Step "Retrying download with Python..."
            $script = @"
import sys
import urllib.request

url = sys.argv[1]
out = sys.argv[2]
req = urllib.request.Request(url, headers={"User-Agent": "Sub2API-windows-lite-update"})
with urllib.request.urlopen(req, timeout=180) as r, open(out, "wb") as f:
    while True:
        chunk = r.read(1024 * 1024)
        if not chunk:
            break
        f.write(chunk)
print(out)
"@
            $tempScript = Join-Path $Downloads "download_asset.py"
            Set-Content -Path $tempScript -Value $script -Encoding UTF8
            try {
                Remove-DownloadedFile $partialZip
                $args = @($python.Args) + @($tempScript, $downloadUrl, $partialZip)
                & $python.File @args | Out-Null
                if (Test-ZipArchive $partialZip) {
                    Move-Item -Force $partialZip $zip
                    return $zip
                }
                $downloadErrors.Add("Python download produced an invalid zip")
                Remove-DownloadedFile $partialZip
            }
            catch {
                $downloadErrors.Add("Python: $($_.Exception.Message)")
                Remove-DownloadedFile $partialZip
            }
            finally {
                Remove-Item $tempScript -Force -ErrorAction SilentlyContinue
            }
        }
    }

    throw "Failed to download $($Release.asset_name). Errors: $($downloadErrors -join ' | ')"
}

function Expand-And-FindBinary($ZipPath) {
    if (Test-Path $ExtractDir) {
        Remove-Item -Recurse -Force $ExtractDir
    }

    New-Item -ItemType Directory -Force -Path $ExtractDir | Out-Null
    Expand-Archive -Path $ZipPath -DestinationPath $ExtractDir -Force

    $found = Get-ChildItem -Path $ExtractDir -Recurse -Filter "sub2api.exe" |
        Where-Object { Test-PESignature $_.FullName } |
        Select-Object -First 1

    if (-not $found) {
        throw "sub2api.exe was not found in the release package."
    }

    return $found.FullName
}

function Stop-Runtime {
    Write-Step "Stopping current runtime..."

    if (Test-Path $AppPid) {
        $pidValue = Get-Content $AppPid -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($pidValue) {
            Stop-Process -Id ([int]$pidValue) -Force -ErrorAction SilentlyContinue
        }
        Remove-Item $AppPid -Force -ErrorAction SilentlyContinue
    }

    Get-Process -Name "sub2api" -ErrorAction SilentlyContinue |
        Where-Object { -not $_.Path -or $_.Path -eq $Exe } |
        Stop-Process -Force -ErrorAction SilentlyContinue

    if (Test-Path $FrontendPid) {
        $pidValue = Get-Content $FrontendPid -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($pidValue) {
            Stop-Process -Id ([int]$pidValue) -Force -ErrorAction SilentlyContinue
        }
        Remove-Item $FrontendPid -Force -ErrorAction SilentlyContinue
    }

    if (Test-Path $RedisPid) {
        $pidValue = Get-Content $RedisPid -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($pidValue) {
            Stop-Process -Id ([int]$pidValue) -Force -ErrorAction SilentlyContinue
        }
        Remove-Item $RedisPid -Force -ErrorAction SilentlyContinue
    }

    Get-CimInstance Win32_Process -Filter "Name = 'python.exe' OR Name = 'pythonw.exe'" -ErrorAction SilentlyContinue |
        Where-Object {
            $_.CommandLine -and (
                $_.CommandLine -like "*mini_redis.py*" -or
                $_.CommandLine -like "*$RedisScript*" -or
                $_.CommandLine -like "*admin_frontend_server.py*" -or
                $_.CommandLine -like "*$FrontendServer*"
            )
        } |
        ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }
}

function Install-Binary($Source, $Release) {
    if (-not (Test-PESignature $Source)) {
        throw "Downloaded binary is not a valid Windows executable: $Source"
    }

    if ((Test-Path $Exe) -and (Test-PESignature $Exe)) {
        $timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
        Copy-Item -Force $Exe "$Exe.backup"
        Copy-Item -Force $Exe "$Exe.$timestamp"
    }

    Copy-Item -Force $Source $Exe
    Copy-Item -Force $Source "$Exe.backup"
    Set-Content -Path $VersionFile -Value $Release.tag_name -Encoding UTF8
    Write-Step "Installed version: $($Release.tag_name)"

    $timestampBackups = Get-ChildItem -Path $Runtime -File -Filter "sub2api.exe.*" -ErrorAction SilentlyContinue |
        Where-Object { $_.Name -match '^sub2api\.exe\.\d{8}-\d{6}$' } |
        Sort-Object LastWriteTime -Descending
    if ($timestampBackups.Count -gt 3) {
        $timestampBackups | Select-Object -Skip 3 | ForEach-Object {
            Remove-Item -LiteralPath $_.FullName -Force -ErrorAction SilentlyContinue
        }
        Write-Step "Pruned old timestamped backups (kept latest 3)."
    }

    $currentZip = Join-Path $Downloads $Release.asset_name
    $zipPackages = Get-ChildItem -Path $Downloads -File -Filter "sub2api_*_windows_amd64.zip" -ErrorAction SilentlyContinue |
        Sort-Object LastWriteTime -Descending
    if ($zipPackages.Count -gt 2) {
        $zipPackages | Select-Object -Skip 2 | ForEach-Object {
            if ($_.FullName -ne $currentZip) {
                Remove-Item -LiteralPath $_.FullName -Force -ErrorAction SilentlyContinue
            }
        }
        Write-Step "Pruned old download packages (kept latest 2)."
    }
}

New-Item -ItemType Directory -Force -Path $Runtime, $Downloads | Out-Null

$release = Get-LatestRelease
Write-Step "Latest release: $($release.tag_name)"

$zipPath = Download-Asset $release
$newBinary = Expand-And-FindBinary $zipPath

Stop-Runtime
Install-Binary $newBinary $release

if (-not $NoRestart) {
    if (-not (Test-Path $StartScript)) {
        throw "Updated successfully, but start.ps1 was not found: $StartScript"
    }

    Write-Step "Restarting service..."
    & powershell -NoProfile -ExecutionPolicy Bypass -File $StartScript -Port $Port
}
else {
    Write-Step "Updated successfully. Restart skipped by -NoRestart."
}
