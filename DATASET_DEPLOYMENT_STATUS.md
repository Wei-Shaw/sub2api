# Dataset Collection Deployment Summary

## ✅ Original Infrastructure Baseline

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

**Handler Integration:** ✅ Implemented on `feat/dataset-production-capture`
- Non-streaming and completed streaming responses are captured at the handler boundary.
- Partial streams, upstream errors, and client disconnects are excluded.
- Capture is bounded and fail-open; sensitive fields are redacted before buffering/upload.

**Runtime Activation Requirements:**
1. Deploy the image containing the capture/runtime-config changes after CI passes.
2. Mount the offline OAuth token file and set `DATASET_GOOGLE_DRIVE_TOKEN_PATH` when the DB credential is inline JSON.
3. Keep `DATASET_ENABLED=false` until the token and folder are verified.
4. Run an approved production probe and verify the uploaded JSONL file by Drive file ID.

## 🚀 Deployment Steps (When Runtime Activation Is Approved)

### 1. Build and verify the approved exact SHA
```bash
# Check CI status for the implementation SHA
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

### 3. Offline OAuth Setup (before DATASET_ENABLED=true)
```bash
# Provision the OAuth token through an approved offline operator procedure.
# Mount the resulting token read-only at the configured token path.
# The server fails fast/degrades if the token is missing; it never prompts on stdin.
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

**Dataset Collection:** Disabled in the live runtime until the token/probe gate is approved
**Reason:** Production OAuth token and upload verification are not yet complete
**Merge Safety:** Capture code is fail-open and does not affect existing traffic when disabled
**Next Action:** Verify the deployment token mount, run CI on the implementation branch, then perform the approved runtime probe

## Architecture

```
┌─────────────────────────────────────────────────┐
│  Request → Handler (bounded capture hook)       │
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

**Status:** Capture/runtime code implemented; waiting for CI, token-mount, and approved upload-probe gates.
