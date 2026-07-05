$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$outDir = Join-Path $root "native-dist"
$compiler = Join-Path $env:WINDIR "Microsoft.NET\Framework64\v4.0.30319\csc.exe"

if (-not (Test-Path -LiteralPath $compiler)) {
  $compiler = Join-Path $env:WINDIR "Microsoft.NET\Framework\v4.0.30319\csc.exe"
}

if (-not (Test-Path -LiteralPath $compiler)) {
  throw "C# compiler not found."
}

New-Item -ItemType Directory -Force -Path $outDir | Out-Null

& $compiler /nologo /target:winexe /optimize+ /platform:anycpu /out:"$outDir\Sub2ApiImageGenerator.exe" /reference:System.dll /reference:System.Drawing.dll /reference:System.Windows.Forms.dll "$PSScriptRoot\Sub2ApiImageGenerator.cs"

Get-Item -LiteralPath "$outDir\Sub2ApiImageGenerator.exe"
