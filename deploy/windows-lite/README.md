# Windows Lite Pack (non-Docker)

Optional local launcher for Windows users who want to run the official `windows_amd64` release **without Docker**.

This is **not** a replacement for the recommended Docker / Linux systemd deployment paths. It is a lightweight helper for:

- downloading the latest GitHub release binary
- starting a local PostgreSQL-backed instance
- optionally using a tiny Python Redis-compatible helper for local-only runs
- updating / stopping the process safely

## Requirements

- Windows PowerShell
- Python 3 (for the optional mini Redis helper and optional admin frontend static server)
- PostgreSQL listening locally (default `127.0.0.1:5432`)

## Files

| File | Purpose |
|------|---------|
| `start.ps1` | Download/start Sub2API, mini Redis helper, optional frontend proxy |
| `stop.ps1` | Stop local processes started by this pack |
| `update.ps1` | Update `sub2api.exe` from GitHub releases and restart |
| `update-sub2api.cmd` | Double-click entry for update |
| `admin_frontend_server.py` | Optional static admin UI + API reverse proxy |
| `mini-redis/mini_redis.py` | Optional local Redis-compatible helper (**local/dev only**) |
| `tray/` | Optional Windows tray app source |

## Quick start

From this directory:

```powershell
# Required: your local PostgreSQL password
$env:SUB2API_DB_PASSWORD = "your-postgres-password"

# Optional overrides
# $env:SUB2API_DB_USER = "postgres"
# $env:SUB2API_DB_NAME = "sub2api"
# $env:SUB2API_ADMIN_EMAIL = "admin@example.com"
# $env:SUB2API_ADMIN_PASSWORD = "choose-a-strong-password"
# $env:SUB2API_DOWNLOAD_PROXY = "https://ghproxy.example/{url}"

powershell -NoProfile -ExecutionPolicy Bypass -File .\start.ps1
```

Then open:

```text
http://localhost:8080
```

Stop:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\stop.ps1
```

Update:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\update.ps1
```

## Security notes

- No database password, JWT secret, TOTP key, or admin password is hard-coded in this pack.
- On first start, local secrets are generated and stored in `runtime/data/local.secrets.env`.
- **Do not commit** `runtime/`, logs, downloaded zips, or `local.secrets.env`.
- `mini-redis` is a local convenience helper only. Prefer a real Redis for anything beyond personal local testing.

## Optional tray app

```powershell
cd tray
dotnet build -c Release
```

Run the resulting executable from the `windows-lite` directory context (scripts are resolved one level above the tray binary output when published next to these scripts).

## Scope / non-goals

- Does not modify Sub2API backend business logic
- Does not ship `sub2api.exe` in git
- Does not claim production parity with Docker Compose
- Intended as an optional community-friendly Windows local path
