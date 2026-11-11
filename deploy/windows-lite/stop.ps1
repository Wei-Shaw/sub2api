$ErrorActionPreference = "SilentlyContinue"

$Root = Split-Path -Parent $MyInvocation.MyCommand.Path
$Runtime = Join-Path $Root "runtime"
$AppPid = Join-Path $Runtime "sub2api.pid"
$FrontendPid = Join-Path $Runtime "admin_frontend.pid"
$FrontendServer = Join-Path $Runtime "admin_frontend_server.py"
$Exe = Join-Path $Runtime "sub2api.exe"
$RedisPid = Join-Path $Runtime "mini-redis\mini_redis.pid"
$RedisScript = Join-Path $Runtime "mini-redis\mini_redis.py"

Write-Host "Stopping Sub2API lightweight runtime..."

if (Test-Path $AppPid) {
    $pidValue = Get-Content $AppPid | Select-Object -First 1
    if ($pidValue) {
        Stop-Process -Id ([int]$pidValue) -Force
    }
    Remove-Item $AppPid -Force
}

Get-Process -Name "sub2api" -ErrorAction SilentlyContinue | Stop-Process -Force

Get-Process -Name "sub2api" -ErrorAction SilentlyContinue |
    Where-Object { -not $_.Path -or $_.Path -eq $Exe } |
    Stop-Process -Force

if (Test-Path $FrontendPid) {
    $pidValue = Get-Content $FrontendPid | Select-Object -First 1
    if ($pidValue) {
        Stop-Process -Id ([int]$pidValue) -Force
    }
    Remove-Item $FrontendPid -Force
}

if (Test-Path $RedisPid) {
    $pidValue = Get-Content $RedisPid | Select-Object -First 1
    if ($pidValue) {
        Stop-Process -Id ([int]$pidValue) -Force
    }
    Remove-Item $RedisPid -Force
}

Get-CimInstance Win32_Process -Filter "Name = 'python.exe' OR Name = 'pythonw.exe'" -ErrorAction SilentlyContinue |
    Where-Object { $_.CommandLine -and ($_.CommandLine -like "*mini_redis.py*" -or $_.CommandLine -like "*$RedisScript*" -or $_.CommandLine -like "*admin_frontend_server.py*" -or $_.CommandLine -like "*$FrontendServer*") } |
    ForEach-Object { Stop-Process -Id $_.ProcessId -Force }

Write-Host "Stopped."
