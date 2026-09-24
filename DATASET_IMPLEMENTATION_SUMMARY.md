# Dataset Collection Infrastructure

## Status: INFRASTRUCTURE READY, INTEGRATION PENDING

Infrastructure code is complete and tested:
- ✅ `internal/dataset/` package (models, collector, worker, drive_client, integration)
- ✅ Config loaded from `internal/config/config.go` (`DatasetConfig` struct)
- ✅ Lifecycle hooks in `cmd/server/main.go` (InitializeDatasetCollection + graceful shutdown)
- ✅ Google Drive OAuth credentials placed in production: `/home/ubuntu/apps/sub2api/deploy/credentials/google_drive_credentials.json`
- ✅ Build passes, dependencies resolved

## Integration Gap

**Handler hook NOT implemented** — capturing request/response requires deeper service-layer changes:
- `service.OpenAIForwardResult` does not expose raw response body
- Streaming responses are written directly to `c.Writer` without buffering
- Non-streaming responses also bypass structured capture

**Required for activation:**
1. Add `ResponseBody []byte` field to `service.OpenAIForwardResult`
2. Buffer non-streaming responses in `ForwardAsChatCompletions` before writing to client
3. Hook `dataset.CaptureFromOpenAIRequest(requestBody, responseBody, statusCode)` after successful forward
4. Test with real OAuth flow (first run triggers browser auth)

## Production Deployment (when ready)

### 1. Add to `/home/ubuntu/apps/sub2api/deploy/.env`:
```bash
# Dataset Collection
DATASET_ENABLED=false  # Set true after testing OAuth flow
DATASET_GOOGLE_DRIVE_CREDENTIALS=/app/backend/credentials/google_drive_credentials.json
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

### 3. First-time OAuth setup:
```bash
# SSH into production server
docker exec -it sub2api-backend-1 /bin/sh
# Server will print OAuth URL, open in browser, paste auth code back
# Token saved to /app/backend/credentials/google_drive_credentials.json.token
```

### 4. Set `DATASET_ENABLED=true` and restart

## Architecture

```
Request → Handler (NOT HOOKED YET)
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
- `backend/internal/dataset/*.go` — core infrastructure (6 files)
- Production credentials already deployed

## Next Steps (for activation)
1. Refactor `service.OpenAIForwardResult` to expose response body
2. Hook capture call in handler success path
3. Test OAuth flow on staging/local
4. Enable in production with `DATASET_ENABLED=true`
