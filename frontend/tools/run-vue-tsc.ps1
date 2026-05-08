$ErrorActionPreference = 'Stop'

$projectRoot = Split-Path -Parent $PSScriptRoot
$wslPath = & wsl.exe bash -lc "wslpath -a '$projectRoot'" 2>$null

if ($LASTEXITCODE -eq 0 -and $wslPath) {
  $wslPath = $wslPath.Trim()
  & wsl.exe bash -lc "cd '$wslPath' && ./node_modules/.bin/vue-tsc --noEmit"
  exit $LASTEXITCODE
}

throw 'WSL is required for frontend typecheck on this machine, but it is not available.'
