import type { GroupPlatform } from '@/types'

export const CONFIG_SCRIPT_SITE_NAME = 'look2eye'

export type ConfigScriptClient = 'codex' | 'claude' | 'opencode'
export type ConfigScriptOS = 'mac' | 'win'

export interface ConfigScriptInput {
  client: ConfigScriptClient
  os?: ConfigScriptOS
  platform?: GroupPlatform | null
  baseUrl: string
  apiKey: string
  siteName?: string
  allowMessagesDispatch?: boolean
}

interface ConfigFile {
  path: string
  content: string
}

interface ConfigPayload {
  label: string
  files: ConfigFile[]
  env?: Record<string, string>
}

const CODEX_DEFAULT_MODEL = 'gpt-5.5'
const CODEX_DEFAULT_REVIEW_MODEL = 'gpt-5.5'
const CODEX_REASONING_EFFORT = 'xhigh'
const CODEX_CONTEXT_WINDOW = 1000000
const CODEX_AUTO_COMPACT_TOKEN_LIMIT = 900000
const OPENCODE_DEFAULT_MODELS_JSON = `{
  "gpt-5.2": {
    "name": "GPT-5.2",
    "limit": {
      "context": 400000,
      "output": 128000
    },
    "options": {
      "store": false
    },
    "variants": {
      "low": {
        "reasoningEffort": "low"
      },
      "medium": {
        "reasoningEffort": "medium"
      },
      "high": {
        "reasoningEffort": "high"
      },
      "xhigh": {
        "reasoningEffort": "xhigh"
      }
    }
  },
  "gpt-5.5": {
    "name": "GPT-5.5",
    "limit": {
      "context": 1050000,
      "output": 128000
    },
    "options": {
      "store": false
    },
    "variants": {
      "low": {
        "reasoningEffort": "low"
      },
      "medium": {
        "reasoningEffort": "medium"
      },
      "high": {
        "reasoningEffort": "high"
      },
      "xhigh": {
        "reasoningEffort": "xhigh"
      }
    }
  },
  "gpt-5.4": {
    "name": "GPT-5.4",
    "limit": {
      "context": 1050000,
      "output": 128000
    },
    "options": {
      "store": false
    },
    "variants": {
      "low": {
        "reasoningEffort": "low"
      },
      "medium": {
        "reasoningEffort": "medium"
      },
      "high": {
        "reasoningEffort": "high"
      },
      "xhigh": {
        "reasoningEffort": "xhigh"
      }
    }
  },
  "gpt-5.4-mini": {
    "name": "GPT-5.4 Mini",
    "limit": {
      "context": 400000,
      "output": 128000
    },
    "options": {
      "store": false
    },
    "variants": {
      "low": {
        "reasoningEffort": "low"
      },
      "medium": {
        "reasoningEffort": "medium"
      },
      "high": {
        "reasoningEffort": "high"
      },
      "xhigh": {
        "reasoningEffort": "xhigh"
      }
    }
  },
  "gpt-5.3-codex-spark": {
    "name": "GPT-5.3 Codex Spark",
    "limit": {
      "context": 128000,
      "output": 32000
    },
    "options": {
      "store": false
    },
    "variants": {
      "low": {
        "reasoningEffort": "low"
      },
      "medium": {
        "reasoningEffort": "medium"
      },
      "high": {
        "reasoningEffort": "high"
      },
      "xhigh": {
        "reasoningEffort": "xhigh"
      }
    }
  },
  "gpt-5.3-codex": {
    "name": "GPT-5.3 Codex",
    "limit": {
      "context": 400000,
      "output": 128000
    },
    "options": {
      "store": false
    },
    "variants": {
      "low": {
        "reasoningEffort": "low"
      },
      "medium": {
        "reasoningEffort": "medium"
      },
      "high": {
        "reasoningEffort": "high"
      },
      "xhigh": {
        "reasoningEffort": "xhigh"
      }
    }
  },
  "codex-mini-latest": {
    "name": "Codex Mini",
    "limit": {
      "context": 200000,
      "output": 100000
    },
    "options": {
      "store": false
    },
    "variants": {
      "low": {
        "reasoningEffort": "low"
      },
      "medium": {
        "reasoningEffort": "medium"
      },
      "high": {
        "reasoningEffort": "high"
      },
      "xhigh": {
        "reasoningEffort": "xhigh"
      }
    }
  }
}`

const trimTrailingSlash = (value: string) => value.replace(/\/+$/, '')

const stripV1Suffix = (value: string) => trimTrailingSlash(value).replace(/\/v1\/?$/, '')

const ensureSuffix = (value: string, suffix: string) => {
  const trimmed = trimTrailingSlash(value)
  return trimmed.endsWith(suffix) ? trimmed : `${trimmed}${suffix}`
}

const resolveBaseUrl = (baseUrl: string) => trimTrailingSlash(baseUrl || window.location.origin)

const resolveAPIBase = (baseUrl: string) => ensureSuffix(stripV1Suffix(resolveBaseUrl(baseUrl)), '/v1')

const resolveGeminiBase = (baseUrl: string) => ensureSuffix(stripV1Suffix(resolveBaseUrl(baseUrl)), '/v1beta')

const resolveAntigravityBase = (baseUrl: string) =>
  ensureSuffix(`${stripV1Suffix(resolveBaseUrl(baseUrl))}/antigravity`, '/v1')

function shellQuote(value: string): string {
  return `'${value.replace(/'/g, `'\\''`)}'`
}

function powershellSingleQuote(value: string): string {
  return `'${value.replace(/'/g, "''")}'`
}

function json(value: unknown): string {
  return JSON.stringify(value, null, 2)
}

function codexProviderName(siteName: string): string {
  const sanitized = siteName.replace(/[^A-Za-z0-9_-]+/g, '')
  return sanitized || 'Look2eye'
}

function codexConfigTemplate(input: Required<Pick<ConfigScriptInput, 'siteName'>>): string {
  const providerName = codexProviderName(input.siteName)
  return `model_provider = "${providerName}"
model = "${CODEX_DEFAULT_MODEL}"
review_model = "${CODEX_DEFAULT_REVIEW_MODEL}"
model_reasoning_effort = "${CODEX_REASONING_EFFORT}"
model_context_window = ${CODEX_CONTEXT_WINDOW}
model_auto_compact_token_limit = ${CODEX_AUTO_COMPACT_TOKEN_LIMIT}
disable_response_storage = true
network_access = "enabled"
windows_wsl_setup_acknowledged = true

[agents]
max_threads = 20
max_depth = 1

[model_providers.${providerName}]
name = "${providerName}"
base_url = "{{LOOK2EYE_BASE_URL}}"
wire_api = "responses"
supports_websockets = {{LOOK2EYE_ENABLE_WEBSOCKET}}

[features]
responses_websockets_v2 = {{LOOK2EYE_ENABLE_WEBSOCKET}}
goals = true

[windows]
sandbox = "elevated"
`
}

function buildCodexShellScript(input: Required<Pick<ConfigScriptInput, 'baseUrl' | 'apiKey' | 'siteName'>>): string {
  const siteName = input.siteName
  const baseUrl = resolveBaseUrl(input.baseUrl)
  const template = codexConfigTemplate(input)

  return `#!/usr/bin/env sh
set -eu

look2eye_setup_result() {
  rc=$?
  if [ "$rc" -eq 0 ]; then
    printf '\\n'
    echo "============================================================"
    echo "[LOOK2EYE SETUP SUCCESS] ${siteName} Codex CLI 配置已完成。"
    echo "请重新打开 Codex 或对应客户端，让新配置生效。"
    echo "============================================================"
  else
    printf '\\n' >&2
    echo "============================================================" >&2
    echo "[LOOK2EYE SETUP FAILED] ${siteName} Codex CLI 配置未完成或未完全生效。退出码：$rc" >&2
    echo "请查看上方失败原因；如提示 Codex 未关闭，请手动关闭后重试。" >&2
    echo "============================================================" >&2
  fi
  trap - EXIT
  exit "$rc"
}
trap look2eye_setup_result EXIT

BASE_URL=${shellQuote(baseUrl)}
API_KEY=${shellQuote(input.apiKey)}
ENABLE_WEBSOCKET='false'
CONFIG_TOML_TEMPLATE=${shellQuote(template)}
CODEX_DIR="$HOME/.codex"
CONFIG_PATH="$CODEX_DIR/config.toml"
AUTH_PATH="$CODEX_DIR/auth.json"
BACKUP_DIR="$CODEX_DIR/backups"

mkdir -p "$CODEX_DIR" "$BACKUP_DIR"

stop_codex_processes() {
  if [ "\${LOOK2EYE_SKIP_CODEX_PROCESS_CLOSE:-}" = "1" ]; then
    echo "已跳过结束 Codex 进程。"
    return 0
  fi

  if ! command -v pkill >/dev/null 2>&1; then
    echo "配置已写入，但未找到 pkill，无法自动结束 Codex 进程。请手动关闭 Codex 后重新打开。" >&2
    return 1
  fi

  stopped=0
  failed=0
  for process_name in codex Codex; do
    count=0
    if command -v pgrep >/dev/null 2>&1; then
      count=$(pgrep -x "$process_name" 2>/dev/null | wc -l | tr -d ' ')
    fi
    if pkill -x "$process_name" >/dev/null 2>&1; then
      if [ "\${count:-0}" -gt 0 ]; then
        stopped=$((stopped + count))
      else
        stopped=$((stopped + 1))
      fi
    elif [ "\${count:-0}" -gt 0 ]; then
      failed=1
    fi
  done

  if [ "$stopped" -gt 0 ]; then
    echo "已结束 Codex 进程：$stopped 个。"
  else
    echo "未发现正在运行的 Codex 进程；重新打开 Codex 即可使用新配置。"
  fi
  if [ "$failed" -ne 0 ]; then
    echo "配置已写入，但部分 Codex 进程无法自动关闭。请手动关闭 Codex 后重新打开。" >&2
    return 1
  fi
  return 0
}

new_backup_stamp() {
  base=$(date +"%Y%m%d-%H%M%S")
  stamp="$base"
  counter=2
  while [ -e "$BACKUP_DIR/$(basename "$CONFIG_PATH").bak-$stamp" ] || [ -e "$BACKUP_DIR/$(basename "$AUTH_PATH").bak-$stamp" ]; do
    stamp="$base-$(printf "%02d" "$counter")"
    counter=$((counter + 1))
  done
  printf '%s\\n' "$stamp"
}

PYTHON_BIN=
if command -v python3 >/dev/null 2>&1 && python3 -c 'import sys' >/dev/null 2>&1; then
  PYTHON_BIN=python3
elif command -v python >/dev/null 2>&1 && python -c 'import sys' >/dev/null 2>&1; then
  PYTHON_BIN=python
else
  echo "失败：需要可用的 python3 或 python 才能安全合并或恢复现有 Codex 配置。" >&2
  exit 1
fi

LOOK2EYE_ACTION=\${1:-apply}

case "$LOOK2EYE_ACTION" in
  ""|1|apply|--apply)
    printf "\\n${siteName} Codex CLI 一键配置：默认覆盖当前配置。\\n"
    echo "如需恢复备份，请在终端运行本脚本并追加 restore 参数。"
    ;;
  2|restore|--restore|/restore)
    "$PYTHON_BIN" - "$CONFIG_PATH" "$AUTH_PATH" "$BACKUP_DIR" <<'PY'
import shutil
import sys
from datetime import datetime
from pathlib import Path

config_path = Path(sys.argv[1])
auth_path = Path(sys.argv[2])
backup_dir = Path(sys.argv[3])

def collect(path, kind, sets):
    prefix = path.name + ".bak-"
    for directory in (backup_dir, path.parent):
        if not directory.exists():
            continue
        for candidate in directory.glob(prefix + "*"):
            if not candidate.is_file():
                continue
            stamp = candidate.name[len(prefix):]
            entry = sets.setdefault(stamp, {"timestamp": stamp, "config": None, "auth": None, "mtime": 0.0})
            candidate_mtime = candidate.stat().st_mtime
            if entry[kind] is None or candidate_mtime > entry[kind].stat().st_mtime:
                entry[kind] = candidate
            entry["mtime"] = max(entry["mtime"], candidate_mtime)

sets = {}
collect(config_path, "config", sets)
collect(auth_path, "auth", sets)
items = sorted(sets.values(), key=lambda item: item["mtime"], reverse=True)
if not items:
    raise SystemExit("失败：未找到之前的 Codex config/auth 备份。")

print("")
print("可用备份：")
for index, item in enumerate(items, 1):
    labels = {"config": "配置", "auth": "认证"}
    parts = "+".join(labels[kind] for kind in ("config", "auth") if item[kind] is not None)
    file_time = datetime.fromtimestamp(item["mtime"]).strftime("%Y-%m-%d %H:%M:%S")
    print(f"{index}) {item['timestamp']} [{parts}] 文件时间 {file_time}")

choice = input("请选择备份序号：").strip()
try:
    selected = items[int(choice) - 1]
except (ValueError, IndexError):
    raise SystemExit("失败：备份序号无效。")

if selected["config"] is not None:
    shutil.copy2(selected["config"], config_path)
    print(f"已恢复：{config_path}")
if selected["auth"] is not None:
    shutil.copy2(selected["auth"], auth_path)
    print(f"已恢复：{auth_path}")
print("已完成，之前的 Codex 配置已恢复。")
PY
    stop_codex_processes
    exit 0
    ;;
  *)
    echo "失败：参数无效：$LOOK2EYE_ACTION。直接运行会配置 ${siteName}；恢复备份请使用 restore 参数。" >&2
    exit 1
    ;;
esac

timestamp=$(new_backup_stamp)
if [ -f "$CONFIG_PATH" ]; then
  config_backup="$BACKUP_DIR/$(basename "$CONFIG_PATH").bak-$timestamp"
  cp "$CONFIG_PATH" "$config_backup"
  echo "已备份：$config_backup"
fi
if [ -f "$AUTH_PATH" ]; then
  auth_backup="$BACKUP_DIR/$(basename "$AUTH_PATH").bak-$timestamp"
  cp "$AUTH_PATH" "$auth_backup"
  echo "已备份：$auth_backup"
fi

"$PYTHON_BIN" - "$CONFIG_PATH" "$AUTH_PATH" "$BASE_URL" "$API_KEY" "$ENABLE_WEBSOCKET" "$CONFIG_TOML_TEMPLATE" <<'PY'
import json
import sys
from pathlib import Path

config_path = Path(sys.argv[1])
auth_path = Path(sys.argv[2])
base_url = sys.argv[3].rstrip("/")
api_key = sys.argv[4].strip()
enable_ws = sys.argv[5].lower() == "true"
template = sys.argv[6]

if not api_key:
    raise RuntimeError("缺少 API Key。")
if not base_url:
    raise RuntimeError("缺少 Base URL。")

if auth_path.exists() and auth_path.read_text(encoding="utf-8").strip():
    try:
        json.loads(auth_path.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        raise RuntimeError("auth.json 不是合法 JSON，请手动合并后重试。") from exc

rendered_config = template.replace("{{LOOK2EYE_BASE_URL}}", base_url)
rendered_config = rendered_config.replace("{{LOOK2EYE_ENABLE_WEBSOCKET}}", "true" if enable_ws else "false")
rendered_config = rendered_config.replace("\\r\\n", "\\n").replace("\\r", "\\n").rstrip("\\n") + "\\n"
auth_text = json.dumps({"OPENAI_API_KEY": api_key}, ensure_ascii=False, separators=(",", ":")) + "\\n"

config_path.write_text(rendered_config, encoding="utf-8")
auth_path.write_text(auth_text, encoding="utf-8")

if config_path.read_text(encoding="utf-8") != rendered_config:
    raise RuntimeError("config.toml 写入后回读不一致，请检查磁盘权限或安全软件拦截。")
actual_auth = json.loads(auth_path.read_text(encoding="utf-8"))
if actual_auth.get("OPENAI_API_KEY") != api_key:
    raise RuntimeError("auth.json 写入后 API Key 校验失败。")
PY

echo "回读校验：已确认配置和认证文件写入成功。"
echo "已完成，${siteName} Codex CLI 配置已更新。正在自动关闭 Codex..."
stop_codex_processes
`
}

function buildCodexPowerShellPayload(input: Required<Pick<ConfigScriptInput, 'baseUrl' | 'apiKey' | 'siteName'>>): string {
  const siteName = input.siteName
  const baseUrl = resolveBaseUrl(input.baseUrl)
  const template = codexConfigTemplate(input)

  return `$ErrorActionPreference = "Stop"

$BaseUrl = ${powershellSingleQuote(baseUrl)}
$ApiKey = ${powershellSingleQuote(input.apiKey)}
$EnableWebSocket = $false
$ConfigTomlTemplate = ${powershellSingleQuote(template)}
$SiteName = ${powershellSingleQuote(siteName)}

function Stop-CodexProcesses {
  if ($env:LOOK2EYE_SKIP_CODEX_PROCESS_CLOSE -eq "1") {
    Write-Host "已跳过结束 Codex 进程。"
    return
  }

  $processes = @(Get-Process -ErrorAction SilentlyContinue | Where-Object {
    $_.ProcessName -ieq "codex" -and $_.Id -ne $PID
  })
  if ($processes.Count -eq 0) {
    Write-Host "未发现正在运行的 Codex 进程；重新打开 Codex 即可使用新配置。"
    return
  }

  $stopped = 0
  foreach ($process in $processes) {
    Stop-Process -Id $process.Id -Force -ErrorAction Stop
    $stopped += 1
  }
  Write-Host ("已结束 Codex 进程：{0} 个。" -f $stopped)
}

function New-BackupStamp {
  param([string[]]$Paths, [string]$BackupDir)
  $base = Get-Date -Format "yyyyMMdd-HHmmss"
  $stamp = $base
  $counter = 2
  while ($true) {
    $exists = $false
    foreach ($path in $Paths) {
      $name = Split-Path -Leaf $path
      if (Test-Path -LiteralPath (Join-Path $BackupDir "$name.bak-$stamp")) {
        $exists = $true
        break
      }
    }
    if (-not $exists) { return $stamp }
    $stamp = "{0}-{1:D2}" -f $base, $counter
    $counter++
  }
}

function Backup-File {
  param([string]$Path, [string]$BackupDir, [string]$Timestamp)
  if (Test-Path -LiteralPath $Path) {
    New-Item -ItemType Directory -Force -Path $BackupDir | Out-Null
    $backup = Join-Path $BackupDir "$((Split-Path -Leaf $Path)).bak-$Timestamp"
    Copy-Item -LiteralPath $Path -Destination $backup -Force
    Write-Host "已备份：$backup"
  }
}

function Get-CodexBackupSets {
  param([string]$ConfigPath, [string]$AuthPath, [string]$BackupDir)
  $sets = @{}
  foreach ($item in @(@{ Path = $ConfigPath; Kind = "Config" }, @{ Path = $AuthPath; Kind = "Auth" })) {
    $name = Split-Path -Leaf $item.Path
    foreach ($dir in @($BackupDir, (Split-Path -Parent $item.Path))) {
      if (-not (Test-Path -LiteralPath $dir)) { continue }
      foreach ($file in Get-ChildItem -LiteralPath $dir -File -Filter "$name.bak-*") {
        $stamp = $file.Name.Substring(("$name.bak-").Length)
        if (-not $sets.ContainsKey($stamp)) {
          $sets[$stamp] = [ordered]@{ Timestamp = $stamp; Config = $null; Auth = $null; LastWriteTime = $file.LastWriteTime }
        }
        $entry = $sets[$stamp]
        $entry[$item.Kind] = $file
        if ($file.LastWriteTime -gt $entry.LastWriteTime) { $entry.LastWriteTime = $file.LastWriteTime }
      }
    }
  }
  return @($sets.Values | ForEach-Object { [pscustomobject]$_ } | Sort-Object LastWriteTime -Descending)
}

function Restore-CodexBackup {
  param([string]$ConfigPath, [string]$AuthPath, [string]$BackupDir)
  $items = Get-CodexBackupSets $ConfigPath $AuthPath $BackupDir
  if ($items.Count -eq 0) { throw "未找到之前的 Codex config/auth 备份。" }
  Write-Host ""
  Write-Host "可用备份："
  for ($i = 0; $i -lt $items.Count; $i++) {
    $parts = @()
    if ($null -ne $items[$i].Config) { $parts += "配置" }
    if ($null -ne $items[$i].Auth) { $parts += "认证" }
    Write-Host ("{0}) {1} [{2}] 文件时间 {3}" -f ($i + 1), $items[$i].Timestamp, ($parts -join "+"), $items[$i].LastWriteTime.ToString("yyyy-MM-dd HH:mm:ss"))
  }
  $choice = Read-Host "请选择备份序号"
  $index = 0
  if (-not [int]::TryParse($choice.Trim(), [ref]$index) -or $index -lt 1 -or $index -gt $items.Count) {
    throw "备份序号无效。"
  }
  $selected = $items[$index - 1]
  if ($null -ne $selected.Config) {
    Copy-Item -LiteralPath $selected.Config.FullName -Destination $ConfigPath -Force
    Write-Host "已恢复：$ConfigPath"
  }
  if ($null -ne $selected.Auth) {
    Copy-Item -LiteralPath $selected.Auth.FullName -Destination $AuthPath -Force
    Write-Host "已恢复：$AuthPath"
  }
}

function Resolve-SetupAction {
  param([string[]]$Arguments)
  if ($null -eq $Arguments -or $Arguments.Count -eq 0) { return "apply" }
  switch ($Arguments[0].Trim().ToLowerInvariant()) {
    "" { return "apply" }
    "1" { return "apply" }
    "apply" { return "apply" }
    "--apply" { return "apply" }
    "2" { return "restore" }
    "restore" { return "restore" }
    "--restore" { return "restore" }
    "/restore" { return "restore" }
    default { throw "参数无效：$($Arguments[0])。直接运行会配置 $SiteName；恢复备份请使用 restore 参数。" }
  }
}

try {
  if ([string]::IsNullOrWhiteSpace($env:USERPROFILE)) { throw "USERPROFILE 为空。" }
  $codexDir = Join-Path $env:USERPROFILE ".codex"
  $configPath = Join-Path $codexDir "config.toml"
  $authPath = Join-Path $codexDir "auth.json"
  $backupDir = Join-Path $codexDir "backups"

  if ((Resolve-SetupAction -Arguments $args) -eq "restore") {
    Restore-CodexBackup $configPath $authPath $backupDir
    Stop-CodexProcesses
    exit 0
  }

  if ([string]::IsNullOrWhiteSpace($ApiKey)) { throw "缺少 API Key。" }
  if ([string]::IsNullOrWhiteSpace($BaseUrl)) { throw "缺少 Base URL。" }

  New-Item -ItemType Directory -Force -Path $codexDir | Out-Null
  New-Item -ItemType Directory -Force -Path $backupDir | Out-Null
  if (Test-Path -LiteralPath $authPath) {
    $rawAuth = [System.IO.File]::ReadAllText($authPath)
    if (-not [string]::IsNullOrWhiteSpace($rawAuth)) {
      $null = $rawAuth | ConvertFrom-Json -ErrorAction Stop
    }
  }

  $newConfig = $ConfigTomlTemplate.Replace("{{LOOK2EYE_BASE_URL}}", $BaseUrl.Trim().TrimEnd("/"))
  $newConfig = $newConfig.Replace("{{LOOK2EYE_ENABLE_WEBSOCKET}}", $EnableWebSocket.ToString().ToLowerInvariant())
  $newConfig = $newConfig.TrimEnd([char[]]@([char]13, [char]10)) + [Environment]::NewLine
  $newAuth = ([ordered]@{ OPENAI_API_KEY = $ApiKey.Trim() } | ConvertTo-Json -Compress) + [Environment]::NewLine

  $timestamp = New-BackupStamp -Paths @($configPath, $authPath) -BackupDir $backupDir
  Backup-File $configPath $backupDir $timestamp
  Backup-File $authPath $backupDir $timestamp

  $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
  [System.IO.File]::WriteAllText($configPath, $newConfig, $utf8NoBom)
  [System.IO.File]::WriteAllText($authPath, $newAuth, $utf8NoBom)

  if ([System.IO.File]::ReadAllText($configPath) -ne $newConfig) { throw "config.toml 写入后回读不一致。" }
  $actualAuth = [System.IO.File]::ReadAllText($authPath) | ConvertFrom-Json -ErrorAction Stop
  if ($actualAuth.OPENAI_API_KEY -ne $ApiKey.Trim()) { throw "auth.json 写入后 API Key 校验失败。" }

  Write-Host "回读校验：已确认配置和认证文件写入成功。"
  Write-Host "已完成，$SiteName Codex CLI 配置已更新。正在自动关闭 Codex..."
  Stop-CodexProcesses
  exit 0
} catch {
  Write-Host "失败：$($_.Exception.Message)" -ForegroundColor Red
  exit 1
}
`
}

function buildCodexBatchScript(input: Required<Pick<ConfigScriptInput, 'baseUrl' | 'apiKey' | 'siteName'>>): string {
  const encoded = toBase64UTF8(buildCodexPowerShellPayload(input))
  return `@echo off
chcp 65001 >nul
setlocal
set "LOOK2EYE_CODEX_EXIT=1"
powershell.exe -NoProfile -ExecutionPolicy Bypass -Command "$script = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('${encoded}')); $path = Join-Path $env:TEMP 'look2eye-codex-config.ps1'; Set-Content -Encoding UTF8 -Path $path -Value $script; & $path %*"
set "LOOK2EYE_CODEX_EXIT=%ERRORLEVEL%"
echo.
if "%LOOK2EYE_CODEX_EXIT%"=="0" (
  echo ============================================================
  echo [LOOK2EYE SETUP SUCCESS] ${input.siteName} Codex CLI config/restore completed.
  echo Reopen Codex or the target client to load the new config.
  echo ============================================================
) else (
  echo ============================================================
  echo [LOOK2EYE SETUP FAILED] ${input.siteName} Codex CLI config/restore did not complete. Exit code: %LOOK2EYE_CODEX_EXIT%
  echo Check the error above. If Codex is still open, close it manually and retry.
  echo ============================================================
)
pause
endlocal & exit /b %LOOK2EYE_CODEX_EXIT%
`
}

function resolveClaudeBase(_input: Required<Pick<ConfigScriptInput, 'baseUrl'>> & Pick<ConfigScriptInput, 'platform'>): string {
  return resolveBaseUrl(_input.baseUrl)
}

function buildClaudeShellScript(input: Required<Pick<ConfigScriptInput, 'baseUrl' | 'apiKey' | 'siteName'>> & Pick<ConfigScriptInput, 'platform'>): string {
  const siteName = input.siteName
  const baseUrl = resolveClaudeBase(input)

  return `#!/usr/bin/env sh
set -eu

look2eye_setup_result() {
  rc=$?
  if [ "$rc" -eq 0 ]; then
    printf '\\n'
    echo "============================================================"
    echo "[LOOK2EYE SETUP SUCCESS] ${siteName} Claude Code 配置已完成。"
    echo "请重新打开 Claude Code 或对应客户端，让新配置生效。"
    echo "============================================================"
  else
    printf '\\n' >&2
    echo "============================================================" >&2
    echo "[LOOK2EYE SETUP FAILED] ${siteName} Claude Code 配置未完成或未完全生效。退出码：$rc" >&2
    echo "请查看上方失败原因；如 settings.json 无法自动合并，请手动处理后重试。" >&2
    echo "============================================================" >&2
  fi
  trap - EXIT
  exit "$rc"
}
trap look2eye_setup_result EXIT

BASE_URL=${shellQuote(baseUrl)}
API_KEY=${shellQuote(input.apiKey)}
CLAUDE_DIR="$HOME/.claude"
SETTINGS_PATH="$CLAUDE_DIR/settings.json"
BACKUP_DIR="$CLAUDE_DIR/backups"

PYTHON_BIN=
if command -v python3 >/dev/null 2>&1 && python3 -c 'import sys' >/dev/null 2>&1; then
  PYTHON_BIN=python3
elif command -v python >/dev/null 2>&1 && python -c 'import sys' >/dev/null 2>&1; then
  PYTHON_BIN=python
else
  echo "Failed: a working python3 or python is required to merge existing Claude Code config safely." >&2
  exit 1
fi

mkdir -p "$CLAUDE_DIR" "$BACKUP_DIR"

unique_backup_path() {
  path=$1
  backup_dir=$2
  timestamp=$3
  name=$(basename "$path")
  backup="$backup_dir/$name.bak-$timestamp"
  counter=2
  while [ -e "$backup" ]; do
    backup="$backup_dir/$name.bak-$timestamp-$(printf "%02d" "$counter")"
    counter=$((counter + 1))
  done
  printf '%s\\n' "$backup"
}

timestamp=$(date +"%Y%m%d-%H%M%S")
if [ -f "$SETTINGS_PATH" ]; then
  backup_path=$(unique_backup_path "$SETTINGS_PATH" "$BACKUP_DIR" "$timestamp")
  cp "$SETTINGS_PATH" "$backup_path"
  echo "Backup: $backup_path"
fi

"$PYTHON_BIN" - "$SETTINGS_PATH" "$BASE_URL" "$API_KEY" <<'PY'
import json
import sys
from pathlib import Path

settings_path = Path(sys.argv[1])
base_url = sys.argv[2].rstrip("/")
api_key = sys.argv[3]

if not api_key:
    raise RuntimeError("Missing API key.")
if not base_url:
    raise RuntimeError("Missing base URL.")

settings = {}
if settings_path.exists() and settings_path.read_text(encoding="utf-8").strip():
    try:
        settings = json.loads(settings_path.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        raise RuntimeError("settings.json is not valid JSON. Please merge it manually.") from exc
    if not isinstance(settings, dict):
        raise RuntimeError("settings.json must contain a JSON object.")

env = settings.get("env")
if not isinstance(env, dict):
    env = {}
env.update({
    "ANTHROPIC_BASE_URL": base_url,
    "ANTHROPIC_AUTH_TOKEN": api_key,
    "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
    "CLAUDE_CODE_ATTRIBUTION_HEADER": "0",
})
settings["env"] = env

settings_path.write_text(json.dumps(settings, indent=2, ensure_ascii=False) + "\\n", encoding="utf-8")
PY

echo "Done. ${siteName} Claude Code config has been updated."
`
}

function buildClaudePowerShellPayload(input: Required<Pick<ConfigScriptInput, 'baseUrl' | 'apiKey' | 'siteName'>> & Pick<ConfigScriptInput, 'platform'>): string {
  const siteName = input.siteName
  const baseUrl = resolveClaudeBase(input)

  return `$ErrorActionPreference = "Stop"

$BaseUrl = ${powershellSingleQuote(baseUrl)}
$ApiKey = ${powershellSingleQuote(input.apiKey)}
$SiteName = ${powershellSingleQuote(siteName)}

function ConvertTo-OrderedMap {
  param([object]$Value)
  $map = [ordered]@{}
  if ($null -eq $Value) { return $map }
  if ($Value -is [System.Collections.IDictionary]) {
    foreach ($key in $Value.Keys) { $map[[string]$key] = $Value[$key] }
    return $map
  }
  foreach ($prop in $Value.PSObject.Properties) {
    $map[$prop.Name] = $prop.Value
  }
  return $map
}

function Backup-File {
  param(
    [string]$Path,
    [string]$BackupDir,
    [string]$Timestamp
  )
  if (Test-Path -LiteralPath $Path) {
    New-Item -ItemType Directory -Force -Path $BackupDir | Out-Null
    $backup = Get-UniqueBackupPath $Path $BackupDir $Timestamp
    Copy-Item -LiteralPath $Path -Destination $backup
    Write-Host "Backup: $backup"
  }
}

function Get-UniqueBackupPath {
  param(
    [string]$Path,
    [string]$BackupDir,
    [string]$Timestamp
  )
  $name = Split-Path -Leaf $Path
  $backup = Join-Path $BackupDir "$name.bak-$Timestamp"
  $counter = 2
  while (Test-Path -LiteralPath $backup) {
    $backup = Join-Path $BackupDir ("{0}.bak-{1}-{2:D2}" -f $name, $Timestamp, $counter)
    $counter++
  }
  return $backup
}

function Write-Utf8NoBom {
  param(
    [string]$Path,
    [string]$Text
  )
  $encoding = New-Object System.Text.UTF8Encoding($false)
  [System.IO.File]::WriteAllText($Path, $Text, $encoding)
}

try {
  if ([string]::IsNullOrWhiteSpace($ApiKey)) { throw "Missing API key." }
  if ([string]::IsNullOrWhiteSpace($BaseUrl)) { throw "Missing base URL." }

  $targetHome = $env:USERPROFILE
  if ([string]::IsNullOrWhiteSpace($targetHome)) { throw "USERPROFILE is empty." }

  $claudeDir = Join-Path $targetHome ".claude"
  $settingsPath = Join-Path $claudeDir "settings.json"
  $backupDir = Join-Path $claudeDir "backups"
  $settings = [ordered]@{}

  if (Test-Path -LiteralPath $settingsPath) {
    $raw = [System.IO.File]::ReadAllText($settingsPath)
    if (-not [string]::IsNullOrWhiteSpace($raw)) {
      try {
        $settings = ConvertTo-OrderedMap ($raw | ConvertFrom-Json)
      } catch {
        throw "settings.json is not valid JSON. Please merge it manually."
      }
    }
  }

  $envMap = [ordered]@{}
  if ($settings.Contains("env")) {
    $envMap = ConvertTo-OrderedMap $settings["env"]
  }
  $envMap["ANTHROPIC_BASE_URL"] = $BaseUrl.Trim().TrimEnd("/")
  $envMap["ANTHROPIC_AUTH_TOKEN"] = $ApiKey
  $envMap["CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC"] = "1"
  $envMap["CLAUDE_CODE_ATTRIBUTION_HEADER"] = "0"
  $settings["env"] = $envMap

  Write-Host "Applying $SiteName Claude Code config..."
  Write-Host "Settings: $settingsPath"
  Write-Host "Base:     $($BaseUrl.Trim().TrimEnd('/'))"

  New-Item -ItemType Directory -Force -Path $claudeDir | Out-Null
  New-Item -ItemType Directory -Force -Path $backupDir | Out-Null
  $timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
  Backup-File $settingsPath $backupDir $timestamp
  Write-Utf8NoBom $settingsPath (($settings | ConvertTo-Json -Depth 50) + [Environment]::NewLine)

  Write-Host "Done. $SiteName Claude Code config has been updated."
  exit 0
} catch {
  Write-Host "Failed: $($_.Exception.Message)" -ForegroundColor Red
  exit 1
}
`
}

function buildClaudeBatchScript(input: Required<Pick<ConfigScriptInput, 'baseUrl' | 'apiKey' | 'siteName'>> & Pick<ConfigScriptInput, 'platform'>): string {
  const psScript = buildClaudePowerShellPayload(input)
  return `@echo off
setlocal
set "LOOK2EYE_SETUP_SCRIPT_PATH=%~f0"
set "LOOK2EYE_SETUP_PAYLOAD="
set "LOOK2EYE_SETUP_EXIT=1"
set "LOOK2EYE_SETUP_LAUNCHED_FROM_EXPLORER=0"
set "LOOK2EYE_SETUP_MARKER=__LOOK2EYE_CLAUDE_CODE_PS1__"
for /f "usebackq delims=" %%I in (\`powershell.exe -NoProfile -Command "$p=[System.IO.Path]::ChangeExtension([System.IO.Path]::GetTempFileName(), '.ps1'); Write-Output $p"\`) do set "LOOK2EYE_SETUP_PAYLOAD=%%I"
for /f "usebackq delims=" %%I in (\`powershell.exe -NoProfile -Command "$ErrorActionPreference='Stop'; try { $ps = Get-CimInstance Win32_Process -Filter ('ProcessId={0}' -f $PID); $cmd = if ($null -ne $ps) { Get-CimInstance Win32_Process -Filter ('ProcessId={0}' -f $ps.ParentProcessId) } else { $null }; $parent = if ($null -ne $cmd) { Get-CimInstance Win32_Process -Filter ('ProcessId={0}' -f $cmd.ParentProcessId) } else { $null }; if ($null -ne $parent -and $parent.Name -ieq 'explorer.exe') { '1' } else { '0' } } catch { '0' }"\`) do set "LOOK2EYE_SETUP_LAUNCHED_FROM_EXPLORER=%%I"
if not defined LOOK2EYE_SETUP_PAYLOAD (
  set "LOOK2EYE_SETUP_EXIT=1"
  goto :look2eye_setup_done
)
powershell.exe -NoProfile -ExecutionPolicy Bypass -Command "$p=$env:LOOK2EYE_SETUP_SCRIPT_PATH; $out=$env:LOOK2EYE_SETUP_PAYLOAD; $marker=$env:LOOK2EYE_SETUP_MARKER; $text=[System.IO.File]::ReadAllText($p); $idx=$text.LastIndexOf($marker); if($idx -lt 0){ Write-Error 'PowerShell payload marker not found.'; exit 1 }; $code=$text.Substring($idx + $marker.Length).TrimStart(); $enc=New-Object System.Text.UTF8Encoding($true); [System.IO.File]::WriteAllText($out, $code, $enc)"
if errorlevel 1 (
  set "LOOK2EYE_SETUP_EXIT=1"
  goto :look2eye_setup_done
)
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%LOOK2EYE_SETUP_PAYLOAD%"
set "LOOK2EYE_SETUP_EXIT=%ERRORLEVEL%"
if exist "%LOOK2EYE_SETUP_PAYLOAD%" del /f /q "%LOOK2EYE_SETUP_PAYLOAD%" >nul 2>nul
:look2eye_setup_done
echo.
if "%LOOK2EYE_SETUP_EXIT%"=="0" (
  echo ============================================================
  echo [LOOK2EYE SETUP SUCCESS] ${input.siteName} Claude Code config completed.
  echo Reopen Claude Code or the target client to load the new config.
  echo ============================================================
) else (
  echo ============================================================
  echo [LOOK2EYE SETUP FAILED] ${input.siteName} Claude Code config did not complete. Exit code: %LOOK2EYE_SETUP_EXIT%
  echo Check the error above. If settings.json cannot be merged automatically, handle it manually and retry.
  echo ============================================================
)
if "%LOOK2EYE_SETUP_LAUNCHED_FROM_EXPLORER%"=="1" (
  echo.
  echo 按任意键关闭窗口...
  pause >nul
)
endlocal & exit /b %LOOK2EYE_SETUP_EXIT%

__LOOK2EYE_CLAUDE_CODE_PS1__
${psScript}`
}

function buildOpenCodeShellScript(input: Required<Pick<ConfigScriptInput, 'baseUrl' | 'apiKey' | 'siteName'>>): string {
  const siteName = input.siteName
  const baseUrl = resolveBaseUrl(input.baseUrl)

  return `#!/usr/bin/env sh
set -eu

look2eye_setup_result() {
  rc=$?
  if [ "$rc" -eq 0 ]; then
    printf '\\n'
    echo "============================================================"
    echo "[LOOK2EYE SETUP SUCCESS] ${siteName} OpenCode 配置已完成。"
    echo "请重新打开 OpenCode 或对应客户端，让新配置生效。"
    echo "============================================================"
  else
    printf '\\n' >&2
    echo "============================================================" >&2
    echo "[LOOK2EYE SETUP FAILED] ${siteName} OpenCode 配置未完成或未完全生效。退出码：$rc" >&2
    echo "请查看上方失败原因；如 opencode.json 无法自动合并，请手动处理后重试。" >&2
    echo "============================================================" >&2
  fi
  trap - EXIT
  exit "$rc"
}
trap look2eye_setup_result EXIT

BASE_URL=${shellQuote(baseUrl)}
API_KEY=${shellQuote(input.apiKey)}
OPENCODE_DIR="$HOME/.config/opencode"
CONFIG_PATH="$OPENCODE_DIR/opencode.json"
BACKUP_DIR="$OPENCODE_DIR/backups"

PYTHON_BIN=
if command -v python3 >/dev/null 2>&1 && python3 -c 'import sys' >/dev/null 2>&1; then
  PYTHON_BIN=python3
elif command -v python >/dev/null 2>&1 && python -c 'import sys' >/dev/null 2>&1; then
  PYTHON_BIN=python
else
  echo "Failed: a working python3 or python is required to merge existing OpenCode config safely." >&2
  exit 1
fi

mkdir -p "$OPENCODE_DIR" "$BACKUP_DIR"

unique_backup_path() {
  path=$1
  backup_dir=$2
  timestamp=$3
  name=$(basename "$path")
  backup="$backup_dir/$name.bak-$timestamp"
  counter=2
  while [ -e "$backup" ]; do
    backup="$backup_dir/$name.bak-$timestamp-$(printf "%02d" "$counter")"
    counter=$((counter + 1))
  done
  printf '%s\\n' "$backup"
}

timestamp=$(date +"%Y%m%d-%H%M%S")
if [ -f "$CONFIG_PATH" ]; then
  backup_path=$(unique_backup_path "$CONFIG_PATH" "$BACKUP_DIR" "$timestamp")
  cp "$CONFIG_PATH" "$backup_path"
  echo "Backup: $backup_path"
fi

"$PYTHON_BIN" - "$CONFIG_PATH" "$BASE_URL" "$API_KEY" <<'PY'
import json
import sys
from pathlib import Path

config_path = Path(sys.argv[1])
base_url = sys.argv[2].rstrip("/")
api_key = sys.argv[3]
default_models = json.loads(r'''${OPENCODE_DEFAULT_MODELS_JSON}''')

if not api_key:
    raise RuntimeError("Missing API key.")
if not base_url:
    raise RuntimeError("Missing base URL.")

config = {}
if config_path.exists() and config_path.read_text(encoding="utf-8").strip():
    try:
        config = json.loads(config_path.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        raise RuntimeError("opencode.json is not valid JSON. Please merge it manually.") from exc
    if not isinstance(config, dict):
        raise RuntimeError("opencode.json must contain a JSON object.")

provider = config.get("provider")
if not isinstance(provider, dict):
    provider = {}
openai = provider.get("openai")
if not isinstance(openai, dict):
    openai = {}
options = openai.get("options")
if not isinstance(options, dict):
    options = {}
options["baseURL"] = base_url
options["apiKey"] = api_key
openai["options"] = options

models = openai.get("models")
if not isinstance(models, dict):
    models = {}
for model_name, model_config in default_models.items():
    existing_model = models.get(model_name)
    if isinstance(existing_model, dict):
        existing_model["variants"] = model_config.get("variants", {})
        models[model_name] = existing_model
    else:
        models[model_name] = model_config
openai["models"] = models
provider["openai"] = openai
config["provider"] = provider
config.setdefault("$schema", "https://opencode.ai/config.json")

agent = config.get("agent")
if not isinstance(agent, dict):
    agent = {}
for section_name in ("build", "plan"):
    section = agent.get(section_name)
    if not isinstance(section, dict):
        section = {}
    opts = section.get("options")
    if not isinstance(opts, dict):
        opts = {}
    opts["store"] = False
    section["options"] = opts
    agent[section_name] = section
config["agent"] = agent

config_path.write_text(json.dumps(config, indent=2, ensure_ascii=False) + "\\n", encoding="utf-8")
PY

echo "Done. ${siteName} OpenCode config has been updated."
`
}

function buildOpenCodePowerShellPayload(input: Required<Pick<ConfigScriptInput, 'baseUrl' | 'apiKey' | 'siteName'>>): string {
  const siteName = input.siteName
  const baseUrl = resolveBaseUrl(input.baseUrl)

  return `$ErrorActionPreference = "Stop"

$BaseUrl = ${powershellSingleQuote(baseUrl)}
$ApiKey = ${powershellSingleQuote(input.apiKey)}
$SiteName = ${powershellSingleQuote(siteName)}
$ModelsJson = @'
${OPENCODE_DEFAULT_MODELS_JSON}
'@

function ConvertTo-OrderedMap {
  param([object]$Value)
  $map = [ordered]@{}
  if ($null -eq $Value) { return $map }
  if ($Value -is [System.Collections.IDictionary]) {
    foreach ($key in $Value.Keys) { $map[[string]$key] = $Value[$key] }
    return $map
  }
  foreach ($prop in $Value.PSObject.Properties) {
    $map[$prop.Name] = $prop.Value
  }
  return $map
}

function Backup-File {
  param(
    [string]$Path,
    [string]$BackupDir,
    [string]$Timestamp
  )
  if (Test-Path -LiteralPath $Path) {
    New-Item -ItemType Directory -Force -Path $BackupDir | Out-Null
    $backup = Get-UniqueBackupPath $Path $BackupDir $Timestamp
    Copy-Item -LiteralPath $Path -Destination $backup
    Write-Host "Backup: $backup"
  }
}

function Get-UniqueBackupPath {
  param(
    [string]$Path,
    [string]$BackupDir,
    [string]$Timestamp
  )
  $name = Split-Path -Leaf $Path
  $backup = Join-Path $BackupDir "$name.bak-$Timestamp"
  $counter = 2
  while (Test-Path -LiteralPath $backup) {
    $backup = Join-Path $BackupDir ("{0}.bak-{1}-{2:D2}" -f $name, $Timestamp, $counter)
    $counter++
  }
  return $backup
}

function Write-Utf8NoBom {
  param(
    [string]$Path,
    [string]$Text
  )
  $encoding = New-Object System.Text.UTF8Encoding($false)
  [System.IO.File]::WriteAllText($Path, $Text, $encoding)
}

function Ensure-AgentStoreFalse {
  param([object]$Config)
  $agent = [ordered]@{}
  if ($Config.Contains("agent")) {
    $agent = ConvertTo-OrderedMap $Config["agent"]
  }
  foreach ($sectionName in @("build", "plan")) {
    $section = [ordered]@{}
    if ($agent.Contains($sectionName)) {
      $section = ConvertTo-OrderedMap $agent[$sectionName]
    }
    $options = [ordered]@{}
    if ($section.Contains("options")) {
      $options = ConvertTo-OrderedMap $section["options"]
    }
    $options["store"] = $false
    $section["options"] = $options
    $agent[$sectionName] = $section
  }
  $Config["agent"] = $agent
}

try {
  if ([string]::IsNullOrWhiteSpace($ApiKey)) { throw "Missing API key." }
  if ([string]::IsNullOrWhiteSpace($BaseUrl)) { throw "Missing base URL." }

  $targetHome = $env:USERPROFILE
  if ([string]::IsNullOrWhiteSpace($targetHome)) { throw "USERPROFILE is empty." }

  $openCodeDir = Join-Path (Join-Path $targetHome ".config") "opencode"
  $configPath = Join-Path $openCodeDir "opencode.json"
  $backupDir = Join-Path $openCodeDir "backups"
  $config = [ordered]@{}

  if (Test-Path -LiteralPath $configPath) {
    $raw = [System.IO.File]::ReadAllText($configPath)
    if (-not [string]::IsNullOrWhiteSpace($raw)) {
      try {
        $config = ConvertTo-OrderedMap ($raw | ConvertFrom-Json)
      } catch {
        throw "opencode.json is not valid JSON. Please merge it manually."
      }
    }
  }

  $provider = [ordered]@{}
  if ($config.Contains("provider")) {
    $provider = ConvertTo-OrderedMap $config["provider"]
  }
  $openai = [ordered]@{}
  if ($provider.Contains("openai")) {
    $openai = ConvertTo-OrderedMap $provider["openai"]
  }
  $options = [ordered]@{}
  if ($openai.Contains("options")) {
    $options = ConvertTo-OrderedMap $openai["options"]
  }
  $options["baseURL"] = $BaseUrl.Trim().TrimEnd("/")
  $options["apiKey"] = $ApiKey
  $openai["options"] = $options

  $models = [ordered]@{}
  if ($openai.Contains("models")) {
    $models = ConvertTo-OrderedMap $openai["models"]
  }
  $defaultModels = ConvertTo-OrderedMap ($ModelsJson | ConvertFrom-Json)
  foreach ($modelName in $defaultModels.Keys) {
    if (-not $models.Contains($modelName)) {
      $models[$modelName] = $defaultModels[$modelName]
    } else {
      $model = ConvertTo-OrderedMap $models[$modelName]
      $defaultModel = ConvertTo-OrderedMap $defaultModels[$modelName]
      if ($defaultModel.Contains("variants")) {
        $model["variants"] = ConvertTo-OrderedMap $defaultModel["variants"]
      }
      $models[$modelName] = $model
    }
  }
  $openai["models"] = $models
  $provider["openai"] = $openai
  $config["provider"] = $provider
  if (-not $config.Contains('$schema')) {
    $config['$schema'] = "https://opencode.ai/config.json"
  }
  Ensure-AgentStoreFalse $config

  Write-Host "Applying $SiteName OpenCode config..."
  Write-Host "Config: $configPath"
  Write-Host "Base:   $($BaseUrl.Trim().TrimEnd('/'))"

  New-Item -ItemType Directory -Force -Path $openCodeDir | Out-Null
  New-Item -ItemType Directory -Force -Path $backupDir | Out-Null
  $timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
  Backup-File $configPath $backupDir $timestamp
  Write-Utf8NoBom $configPath (($config | ConvertTo-Json -Depth 80) + [Environment]::NewLine)

  Write-Host "Done. $SiteName OpenCode config has been updated."
  exit 0
} catch {
  Write-Host "Failed: $($_.Exception.Message)" -ForegroundColor Red
  exit 1
}
`
}

function buildOpenCodeBatchScript(input: Required<Pick<ConfigScriptInput, 'baseUrl' | 'apiKey' | 'siteName'>>): string {
  const psScript = buildOpenCodePowerShellPayload(input)
  return `@echo off
setlocal
set "LOOK2EYE_SETUP_SCRIPT_PATH=%~f0"
set "LOOK2EYE_SETUP_PAYLOAD="
set "LOOK2EYE_SETUP_EXIT=1"
set "LOOK2EYE_SETUP_LAUNCHED_FROM_EXPLORER=0"
set "LOOK2EYE_SETUP_MARKER=__LOOK2EYE_OPENCODE_PS1__"
for /f "usebackq delims=" %%I in (\`powershell.exe -NoProfile -Command "$p=[System.IO.Path]::ChangeExtension([System.IO.Path]::GetTempFileName(), '.ps1'); Write-Output $p"\`) do set "LOOK2EYE_SETUP_PAYLOAD=%%I"
for /f "usebackq delims=" %%I in (\`powershell.exe -NoProfile -Command "$ErrorActionPreference='Stop'; try { $ps = Get-CimInstance Win32_Process -Filter ('ProcessId={0}' -f $PID); $cmd = if ($null -ne $ps) { Get-CimInstance Win32_Process -Filter ('ProcessId={0}' -f $ps.ParentProcessId) } else { $null }; $parent = if ($null -ne $cmd) { Get-CimInstance Win32_Process -Filter ('ProcessId={0}' -f $cmd.ParentProcessId) } else { $null }; if ($null -ne $parent -and $parent.Name -ieq 'explorer.exe') { '1' } else { '0' } } catch { '0' }"\`) do set "LOOK2EYE_SETUP_LAUNCHED_FROM_EXPLORER=%%I"
if not defined LOOK2EYE_SETUP_PAYLOAD (
  set "LOOK2EYE_SETUP_EXIT=1"
  goto :look2eye_setup_done
)
powershell.exe -NoProfile -ExecutionPolicy Bypass -Command "$p=$env:LOOK2EYE_SETUP_SCRIPT_PATH; $out=$env:LOOK2EYE_SETUP_PAYLOAD; $marker=$env:LOOK2EYE_SETUP_MARKER; $text=[System.IO.File]::ReadAllText($p); $idx=$text.LastIndexOf($marker); if($idx -lt 0){ Write-Error 'PowerShell payload marker not found.'; exit 1 }; $code=$text.Substring($idx + $marker.Length).TrimStart(); $enc=New-Object System.Text.UTF8Encoding($true); [System.IO.File]::WriteAllText($out, $code, $enc)"
if errorlevel 1 (
  set "LOOK2EYE_SETUP_EXIT=1"
  goto :look2eye_setup_done
)
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%LOOK2EYE_SETUP_PAYLOAD%"
set "LOOK2EYE_SETUP_EXIT=%ERRORLEVEL%"
if exist "%LOOK2EYE_SETUP_PAYLOAD%" del /f /q "%LOOK2EYE_SETUP_PAYLOAD%" >nul 2>nul
:look2eye_setup_done
echo.
if "%LOOK2EYE_SETUP_EXIT%"=="0" (
  echo ============================================================
  echo [LOOK2EYE SETUP SUCCESS] ${input.siteName} OpenCode config completed.
  echo Reopen OpenCode or the target client to load the new config.
  echo ============================================================
) else (
  echo ============================================================
  echo [LOOK2EYE SETUP FAILED] ${input.siteName} OpenCode config did not complete. Exit code: %LOOK2EYE_SETUP_EXIT%
  echo Check the error above. If opencode.json cannot be merged automatically, handle it manually and retry.
  echo ============================================================
)
if "%LOOK2EYE_SETUP_LAUNCHED_FROM_EXPLORER%"=="1" (
  echo.
  echo 按任意键关闭窗口...
  pause >nul
)
endlocal & exit /b %LOOK2EYE_SETUP_EXIT%

__LOOK2EYE_OPENCODE_PS1__
${psScript}`
}

function buildClaudePayload(input: Required<Pick<ConfigScriptInput, 'baseUrl' | 'apiKey' | 'siteName'>> & Pick<ConfigScriptInput, 'platform'>): ConfigPayload {
  const baseUrl = resolveClaudeBase(input)
  const env = {
    ANTHROPIC_BASE_URL: baseUrl,
    ANTHROPIC_AUTH_TOKEN: input.apiKey,
    CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: '1',
    CLAUDE_CODE_ATTRIBUTION_HEADER: '0'
  }
  const envContent = `export ANTHROPIC_BASE_URL=${shellQuote(baseUrl)}
export ANTHROPIC_AUTH_TOKEN=${shellQuote(input.apiKey)}
export CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1
export CLAUDE_CODE_ATTRIBUTION_HEADER=0`
  const settingsJSON = json({
    env
  })

  return {
    label: 'Claude Code',
    env,
    files: [
      { path: '.look2eye/claude-code.env', content: `# ${input.siteName} Claude Code environment\n${envContent}\n` },
      { path: '.claude/settings.json', content: settingsJSON }
    ]
  }
}

function buildOpenCodePayload(input: Required<Pick<ConfigScriptInput, 'baseUrl' | 'apiKey' | 'siteName'>> & Pick<ConfigScriptInput, 'platform'>): ConfigPayload {
  const platform = input.platform || 'anthropic'
  const providerID = platform === 'antigravity' ? 'antigravity-claude' : platform
  const baseURL = platform === 'gemini'
    ? resolveGeminiBase(input.baseUrl)
    : platform === 'antigravity'
      ? resolveAntigravityBase(input.baseUrl)
      : resolveAPIBase(input.baseUrl)
  const model = platform === 'gemini'
    ? 'gemini-2.0-flash'
    : platform === 'openai'
      ? 'gpt-5.5'
      : 'claude-opus-4-6-thinking'

  const config = {
    $schema: 'https://opencode.ai/config.json',
    provider: {
      [providerID]: {
        npm: platform === 'gemini' ? '@ai-sdk/google' : platform === 'openai' ? '@ai-sdk/openai' : '@ai-sdk/anthropic',
        name: input.siteName,
        options: {
          baseURL,
          apiKey: input.apiKey
        },
        models: {
          [model]: {
            name: model
          }
        }
      }
    }
  }

  return {
    label: 'OpenCode',
    files: [
      { path: '.config/opencode/opencode.json', content: json(config) }
    ]
  }
}

function buildPayload(input: ConfigScriptInput): ConfigPayload {
  const base = {
    baseUrl: input.baseUrl,
    apiKey: input.apiKey,
    siteName: input.siteName || CONFIG_SCRIPT_SITE_NAME
  }

  switch (input.client) {
    case 'codex':
      throw new Error('Codex CLI uses a dedicated setup script generator.')
    case 'claude':
      return buildClaudePayload({ ...base, platform: input.platform })
    case 'opencode':
      return buildOpenCodePayload({ ...base, platform: input.platform })
  }
}

function buildShellScript(payload: ConfigPayload, siteName: string): string {
  const writeFileCommands = payload.files.map((file) => {
    const target = `$HOME/${file.path}`
    const dir = file.path.split('/').slice(0, -1).join('/')
    return `mkdir -p "$HOME/${dir}"
cat > "${target}" <<'LOOK2EYE_CONFIG_EOF'
${file.content}
LOOK2EYE_CONFIG_EOF`
  }).join('\n\n')

  const profileCommands = payload.files.some((file) => file.path === '.look2eye/claude-code.env')
    ? `
PROFILE="$HOME/.zshrc"
if [ -n "\${BASH_VERSION:-}" ]; then
  PROFILE="$HOME/.bashrc"
fi
touch "$PROFILE"
SOURCE_LINE='. "$HOME/.look2eye/claude-code.env"'
if ! grep -qxF "$SOURCE_LINE" "$PROFILE"; then
  printf '\\n# ${siteName} Claude Code\\n%s\\n' "$SOURCE_LINE" >> "$PROFILE"
fi`
    : ''

  return `#!/usr/bin/env sh
set -eu

echo "Installing ${siteName} ${payload.label} configuration..."

${writeFileCommands}${profileCommands}

echo "Done. Restart your terminal or source your shell profile if environment variables were updated."
`
}

function buildPowerShellScript(payload: ConfigPayload, siteName: string): string {
  const writeFileCommands = payload.files.map((file) => {
    const windowsPath = file.path.replace(/\//g, '\\')
    return `$target = Join-Path $env:USERPROFILE ${JSON.stringify(windowsPath)}
New-Item -ItemType Directory -Force -Path (Split-Path $target) | Out-Null
@'
${file.content}
'@ | Set-Content -Encoding UTF8 -Path $target`
  }).join('\n\n')

  const envCommands = payload.env
    ? Object.entries(payload.env)
      .map(([key, value]) => `[Environment]::SetEnvironmentVariable(${JSON.stringify(key)}, ${JSON.stringify(value)}, 'User')`)
      .join('\n')
    : ''

  return `$ErrorActionPreference = 'Stop'
Write-Host "Installing ${siteName} ${payload.label} configuration..."

${writeFileCommands}${envCommands ? `\n\n${envCommands}` : ''}

Write-Host "Done. Restart your terminal for environment variable changes to take effect."
`
}

function toBase64UTF8(value: string): string {
  const bytes = new TextEncoder().encode(value)
  let binary = ''
  bytes.forEach((byte) => {
    binary += String.fromCharCode(byte)
  })
  return btoa(binary)
}

function buildBatchScript(payload: ConfigPayload, siteName: string): string {
  const psScript = buildPowerShellScript(payload, siteName)
  const encoded = toBase64UTF8(psScript)
  return `@echo off
setlocal
echo Installing ${siteName} ${payload.label} configuration...
powershell -NoProfile -ExecutionPolicy Bypass -Command "$script = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('${encoded}')); $path = Join-Path $env:TEMP 'look2eye-config.ps1'; Set-Content -Encoding UTF8 -Path $path -Value $script; & $path"
if errorlevel 1 (
  echo Installation failed.
  pause
  exit /b 1
)
echo Done.
pause
`
}

export function getConfigScriptOS(): ConfigScriptOS {
  const nav = navigator as Navigator & { userAgentData?: { platform?: string } }
  const platform = nav.userAgentData?.platform || navigator.platform || ''
  return /win/i.test(platform) ? 'win' : 'mac'
}

export function buildAPIKeyConfigScript(input: ConfigScriptInput): { filename: string; content: string; os: ConfigScriptOS } {
  const os = input.os || getConfigScriptOS()
  const siteName = input.siteName || CONFIG_SCRIPT_SITE_NAME
  const filenameClient = input.client === 'claude' ? 'claude-code' : input.client
  const extension = os === 'win' ? 'bat' : 'sh'
  const content = input.client === 'codex'
    ? os === 'win'
      ? buildCodexBatchScript({ baseUrl: input.baseUrl, apiKey: input.apiKey, siteName })
      : buildCodexShellScript({ baseUrl: input.baseUrl, apiKey: input.apiKey, siteName })
    : input.client === 'claude'
      ? os === 'win'
        ? buildClaudeBatchScript({ baseUrl: input.baseUrl, apiKey: input.apiKey, siteName, platform: input.platform })
        : buildClaudeShellScript({ baseUrl: input.baseUrl, apiKey: input.apiKey, siteName, platform: input.platform })
    : input.client === 'opencode'
      ? os === 'win'
        ? buildOpenCodeBatchScript({ baseUrl: input.baseUrl, apiKey: input.apiKey, siteName })
        : buildOpenCodeShellScript({ baseUrl: input.baseUrl, apiKey: input.apiKey, siteName })
    : os === 'win'
      ? buildBatchScript(buildPayload({ ...input, siteName }), siteName)
      : buildShellScript(buildPayload({ ...input, siteName }), siteName)

  return {
    filename: `${siteName}-${filenameClient}-config.${extension}`,
    content,
    os
  }
}

export function isConfigScriptClientAvailable(input: Pick<ConfigScriptInput, 'client' | 'platform' | 'allowMessagesDispatch'>): boolean {
  return !!input.platform
}

export function downloadAPIKeyConfigScript(input: ConfigScriptInput): void {
  const script = buildAPIKeyConfigScript(input)
  const mime = script.os === 'win' ? 'application/x-bat' : 'text/x-shellscript'
  const blob = new Blob([script.content], { type: `${mime};charset=utf-8` })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = script.filename
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
  URL.revokeObjectURL(url)
}
