# Dataset Collection Deployment Summary

## ✅ Infrastructure Deployed

**Commit:** `e88c7264f` — feat(dataset): add dataset collection infrastructure
**Pushed:** 2026-09-13 11:35 +07

### Code Changes (15 files, +888 lines)
- ✅ `backend/internal/dataset/` package complete (models, collector, worker, drive_client, integration)
- ✅ Config structure in `internal/config/config.go`
- ✅ Lifecycle hooks in `cmd/server/main.go` and `cmd/server/wire.go`
- ✅ Dependencies added to `go.mod`: `golang.org/x/oauth2`, `google.golang.org/api`
- ✅ `.gitignore` updated to exclude credentials

### Production Configuration
- ✅ Credentials deployed: `/home/ubuntu/apps/sub2api/deploy/credentials/google_drive_credentials.json` (mode 600)
  - Project: `dataset-508503`
  - Client ID: `755436741376-***`
- ✅ Production `.env` updated with `DATASET_*` config (disabled by default)
- ✅ `docker-compose.prod.yml` mounts credentials folder as read-only

## 📋 Integration Status

**Handler Integration:** ⚠️ NOT IMPLEMENTED
Reason: `service.OpenAIForwardResult` does not expose raw response body.
Streaming and non-streaming responses bypass structured capture.

**Required for Activation:**
1. Add `ResponseBody []byte` field to `service.OpenAIForwardResult`
2. Buffer responses in `ForwardAsChatCompletions` before client write
3. Hook `dataset.CaptureFromOpenAIRequest()` in success path
4. Test OAuth flow (browser auth on first run)

## 🚀 Deployment Steps (When Handler Integration Complete)

### 1. Wait for CI to build image with commit `e88c7264f`
```bash
# Check CI status
gh run list --limit 5 --branch main
```

### 2. Deploy new image
```bash
cd /home/ubuntu/apps/sub2api/deploy
# Update IMAGE_TAG in .env to new SHA
docker-compose -f docker-compose.prod.yml pull
docker-compose -f docker-compose.prod.yml up -d sub2api
docker logs -f sub2api
```

### 3. First-time OAuth Setup (when DATASET_ENABLED=true)
```bash
docker exec -it sub2api /bin/sh
# Server prints OAuth URL → open in browser → paste code back
# Token saved to /app/backend/credentials/google_drive_credentials.json.token
```

### 4. Create Google Drive Folder
1. Go to https://drive.google.com
2. Create folder "sub2api-datasets"
3. Copy folder ID from URL: `https://drive.google.com/drive/folders/{FOLDER_ID}`
4. Add to `/home/ubuntu/apps/sub2api/deploy/.env`: `DATASET_GOOGLE_DRIVE_FOLDER_ID={FOLDER_ID}`

### 5. Enable Collection
```bash
# Edit .env
DATASET_ENABLED=true

# Restart
docker-compose -f docker-compose.prod.yml restart sub2api
```

## 📊 Current Production State

**Dataset Collection:** Disabled (`DATASET_ENABLED=false`)
**Reason:** Handler integration not implemented
**Merge Safety:** ✅ Code merged, infrastructure ready, no impact on existing traffic
**Next Action:** Implement service layer response capture when ready to activate

## Architecture

```
┌─────────────────────────────────────────────────┐
│  Request → Handler (NOT HOOKED)                 │
│             ↓                                    │
│        Collector (bounded 1000-item buffer)     │
│             ↓ (fail-open, non-blocking)         │
│        Worker (batch every 5min)                │
│             ↓                                    │
│        Google Drive (JSONL upload)              │
└─────────────────────────────────────────────────┘
```

**Files:**
- Implementation: `DATASET_IMPLEMENTATION_SUMMARY.md`
- Original plan: `DATASET_COLLECTION_PLAN.md`
- Example config: `.env.dataset.example`

**Status:** Infrastructure complete, waiting for handler integration to activate.
