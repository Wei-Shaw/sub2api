# Dataset Collection Implementation Plan

## Mục tiêu
Chiết xuất dataset từ sub2api để training AI sau này, lưu trữ bên ngoài server (Google Drive), không ảnh hưởng đến logic hiện tại.

## Kiến trúc

```
sub2api Request/Response
        │
        ├─ Hook bất đồng bộ (fail-open)
        │  └─ Capture: request body + response + metadata
        │     └─ Push to in-memory buffer (bounded queue)
        │
        └─ Background Worker (goroutine)
           ├─ Batch when: 100 conversations OR 10 MB OR 5 minutes
           ├─ Upload to Google Drive
           └─ Clear local buffer

Google Drive (Google One 5TB)
```

## Điểm capture

### 1. Chat Completions (OpenAI-compatible)
- **File**: `internal/handler/gateway_handler_chat_completions.go`
- **Method**: `ChatCompletions(c *gin.Context)`
- **Capture**: request body + response (streaming/non-streaming)

### 2. OpenAI Gateway
- **File**: `internal/handler/openai_gateway_handler.go`
- **Method**: `HandleOpenAI*` methods
- **Capture**: request body + response

### 3. Responses API (Anthropic)
- **File**: `internal/handler/gateway_handler_responses.go`
- **Capture**: request body + response

## Cấu trúc dataset

```json
{
  "conversation_id": "uuid",
  "timestamp": "2026-09-13T10:30:00Z",
  "user_id": 123,
  "api_key_id": 456,
  "platform": "openai",
  "endpoint": "/v1/chat/completions",
  "request": {
    "model": "gpt-4",
    "messages": [...],
    "temperature": 0.7,
    ...
  },
  "response": {
    "id": "chatcmpl-...",
    "choices": [...],
    ...
  },
  "metadata": {
    "latency_ms": 1234,
    "tokens": {
      "prompt": 100,
      "completion": 50,
      "total": 150
    }
  }
}
```

## Implementation checklist

### Phase 1: Config & Credentials
- [x] Di chuyển OAuth credentials vào `backend/credentials/`
- [ ] Thêm config vào `.env`:
  - `DATASET_COLLECTION_ENABLED=true`
  - `DATASET_GOOGLE_DRIVE_CREDENTIALS=/path/to/credentials.json`
  - `DATASET_GOOGLE_DRIVE_FOLDER_ID=<folder_id>`
  - `DATASET_BATCH_SIZE=100`
  - `DATASET_BATCH_MAX_MB=10`
  - `DATASET_BATCH_INTERVAL_SEC=300`
- [ ] Load config trong `internal/config/config.go`

### Phase 2: Dataset Collector Core
- [ ] `internal/dataset/collector.go`:
  - In-memory buffer (thread-safe)
  - Bounded queue (max 1000 items)
  - Fail-open pattern (log error, không crash)
- [ ] `internal/dataset/models.go`:
  - `DatasetEntry` struct
  - Serialization helpers

### Phase 3: Google Drive Client
- [ ] `internal/dataset/drive_client.go`:
  - OAuth flow với credentials JSON
  - Upload file to Google Drive
  - Create/append to folder
  - Error handling + retry

### Phase 4: Background Worker
- [ ] `internal/dataset/worker.go`:
  - Goroutine chạy background
  - Batch logic (size/time trigger)
  - Upload batch to Drive
  - Cleanup buffer

### Phase 5: Hook Integration
- [ ] Hook vào `ChatCompletions`:
  - Capture request body
  - Capture response (streaming + non-streaming)
  - Push to collector (non-blocking)
- [ ] Hook vào `OpenAIGatewayHandler`
- [ ] Hook vào Responses API

### Phase 6: Testing
- [ ] Unit tests: collector, buffer, batch logic
- [ ] Integration test: upload to Drive
- [ ] Focused backend tests
- [ ] Build verification

### Phase 7: Documentation
- [ ] README: setup guide
- [ ] ENV variables documentation
- [ ] Google Drive folder setup guide

## Fail-safe guarantees

1. **Fail-open**: Nếu collector crash, request vẫn xử lý bình thường
2. **Non-blocking**: Hook chỉ push vào channel, không chờ upload
3. **Bounded memory**: Buffer giới hạn 1000 items, drop old nếu full
4. **No PII leak**: Redact sensitive fields trước khi lưu
5. **Config-gated**: Chỉ chạy khi `DATASET_COLLECTION_ENABLED=true`

## Security considerations

- Credentials file: `chmod 600`, không commit vào git
- PII redaction: user_id hash, api_key redact
- Google Drive folder: private, chỉ service account truy cập
- Log không chứa credentials

## Deployment notes

- Feature flag: `DATASET_COLLECTION_ENABLED` default `false`
- Production: enable sau khi verify staging
- Monitoring: log batch upload status, error rate

## Next steps

1. Implement Phase 1 (config)
2. Implement Phase 2 (collector core)
3. Implement Phase 3 (Drive client)
4. Test upload flow
5. Hook integration
6. Full testing
