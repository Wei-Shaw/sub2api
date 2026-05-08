param(
  [Parameter(ValueFromRemainingArguments = $true)]
  [string[]]$ArgsFromCaller
)

$ErrorActionPreference = 'Stop'

$projectRoot = Split-Path -Parent $PSScriptRoot
$wslPath = & wsl.exe bash -lc "wslpath -a '$projectRoot'" 2>$null

if ($LASTEXITCODE -eq 0 -and $wslPath) {
  $wslPath = $wslPath.Trim()
  $escapedArgs = @()
  foreach ($arg in $ArgsFromCaller) {
    $escapedArgs += "'" + ($arg -replace "'", "'\''") + "'"
  }
  $argSuffix = if ($escapedArgs.Count -gt 0) { ' ' + ($escapedArgs -join ' ') } else { '' }
  & wsl.exe bash -lc "cd '$wslPath' && ./node_modules/.bin/vitest run$argSuffix"
  exit $LASTEXITCODE
}

throw 'WSL is required for frontend vitest on this machine, but it is not available.'
