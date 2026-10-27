# Sub2API Key Manager

Portable Windows desktop helper for Sub2API users. It validates user-entered API keys and can configure Codex and OpenCode to use a Sub2API endpoint.


## Features

- Reads the Sub2API endpoint from `sub2api_key_manager.env` next to the app.
- Optionally reads a status API token for deployments that protect exact-key validation.
- Validates one OpenAI/Codex key and one Gemini key.
- Shows key state, expiry, charged quota, consumed amount, and remaining balance.
- Infers previously configured keys from Codex/OpenCode config files on startup.
- Configures Codex by writing:
  - `%USERPROFILE%\.codex\config.toml`
  - `%USERPROFILE%\.codex\auth.json`
- Configures OpenCode by writing:
  - `%USERPROFILE%\.config\opencode\opencode.jsonc`
- Backs up existing config files before writing.
- Opens install pages if Codex/OpenCode is not detected.

## Expected Status Endpoint

Create `sub2api_key_manager.env` next to `app.py` while running from source, or next to `Sub2API Key Manager.exe` after building:

```text
SUB2API_ENDPOINT=http://SERVER_IP:18081
STATUS_API_TOKEN=replace_with_status_api_token
```

`STATUS_API_TOKEN` may be left blank if your status service does not require `X-API-Key`. The service is named "key-status-service" and is available in Tools folder as well

The app calls:

```text
GET <endpoint>/api-keys/status?key=<api-key>
```

If the deployment requires a token, the app sends it as:

```text
X-API-Key: <status token>
```

The response should include at least:

```json
{
  "name": "key name",
  "group": { "name": "openai-pool1", "platform": "openai", "status": "active" },
  "status": "active",
  "state": { "usable": true, "expired": false, "deleted": false, "quota_exhausted": false },
  "expires_at": "2026-07-30T20:13:43+08:00",
  "charged_usd": "800.00000000",
  "consumed_usd": "0",
  "remaining_usd": "800.00000000"
}
```

For type checking, group names starting with `openai` are treated as OpenAI/Codex keys and group names starting with `gravity` are treated as Gemini keys.

## Configuration Written

Codex is configured with the env endpoint as the model provider base URL.

OpenCode is configured with:

- OpenAI provider: `<endpoint>/v1`
- Gemini provider: `<endpoint>/antigravity/v1beta`

## Build

From this folder:

```powershell
powershell -ExecutionPolicy Bypass -File .\build.ps1
```

The generated app is written to:

```text
dist\Sub2API Key Manager.exe
```

The build script expects Python at `C:\conda\python.exe`. Change `$Python` in `build.ps1` if needed.

## Local Persistence

API keys are not persisted by this app. Existing keys are inferred from Codex/OpenCode config files only.
