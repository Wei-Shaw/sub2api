# Dataset Collection Infrastructure

## Status: CAPTURE IMPLEMENTED, RUNTIME ACTIVATION PENDING

Infrastructure code is complete and tested:
- ✅ `internal/dataset/` package (models, collector, worker, drive_client, integration)
- ✅ Config loaded from `internal/config/config.go` (`DatasetConfig` struct)
- ✅ Lifecycle hooks in `cmd/server/main.go` (InitializeDatasetCollection + graceful shutdown)
- ✅ Google Drive OAuth credentials placed in production: `/home/ubuntu/apps/sub2api/deploy/credentials/google_drive_credentials.json`
- ✅ Build passes, dependencies resolved

## Capture and runtime configuration

The production capture path is implemented at the handler boundary for both
non-streaming and completed streaming responses. It is bounded, fail-open, and
redacts sensitive data before collection.

Startup hydrates Dataset enablement and worker settings from the DB before the
collector starts. Google OAuth credentials may be a path or decrypted inline
JSON. Inline JSON requires the deployment-only
`DATASET_GOOGLE_DRIVE_TOKEN_PATH`; file credentials retain the
`<credentials>.token` fallback. The server does not prompt for OAuth input.

## Production Deployment (when ready)

### 1. Add to `/home/ubuntu/apps/sub2api/deploy/.env`:
```bash
# Dataset Collection
DATASET_ENABLED=false  # Keep false until token and upload probe are verified
DATASET_GOOGLE_DRIVE_CREDENTIALS=/app/backend/credentials/google_drive_credentials.json
DATASET_GOOGLE_DRIVE_TOKEN_PATH=/app/backend/credentials/google_drive_credentials.json.token
DATASET_GOOGLE_DRIVE_FOLDER_ID=  # Create folder in Drive, paste ID here
DATASET_BATCH_SIZE=100
DATASET_BATCH_MAX_MB=10
DATASET_BATCH_INTERVAL_SEC=300
DATASET_BUFFER_MAX_ITEMS=1000
```

### 2. Update `docker-compose.prod.yml`:
```yaml
services:
  backend:
    volumes:
      - ./data:/app/data
      - ./credentials:/app/backend/credentials:ro  # Add this line
```

### 3. Offline OAuth setup:
```bash
# Provision the OAuth token through an approved offline operator procedure.
# Mount the generated token read-only at DATASET_GOOGLE_DRIVE_TOKEN_PATH.
# Missing tokens disable/degrade Dataset startup instead of blocking the server.
```

### 4. Set `DATASET_ENABLED=true` and restart

## Architecture

```
Request → Handler (bounded capture hook)
            ↓
       Collector (bounded buffer, fail-open)
            ↓
       Worker (background, batch upload every 5 min)
            ↓
       Google Drive (JSONL files)
```

## Files Modified
- `backend/internal/config/config.go` — DatasetConfig struct
- `backend/cmd/server/main.go` — lifecycle integration
- `backend/cmd/server/wire.go` — cleanup hook
- `backend/go.mod`, `backend/go.sum` — google drive deps
- `.gitignore` — ignore credentials
- `.env.dataset.example` — template

## Files Created
- `backend/internal/dataset/*.go` — core infrastructure, capture, and runtime-config code
- Production credentials already deployed

## Next Steps (for activation)
1. Deploy the exact CI-green image containing the capture/runtime-config changes.
2. Verify the token mount and Drive folder access without logging secrets.
3. Enable Dataset only through the approved runtime path.
4. Run an approved probe and verify the resulting Drive file ID/content metadata.
