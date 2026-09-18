# Dataset Production Capture — Implementation Contract

## Status: Historical implementation contract

The original contract below records the design that was approved before the
capture/runtime-config implementation landed. It is retained for provenance;
the current source code and `DATASET_DEPLOYMENT_STATUS.md` are authoritative for
actual behavior and activation steps.
**Branch:** `feat/dataset-production-capture`
**Base:** `eec13e7cb` (main)
**Approved scope:** 6 actions from audit report

---

## 1. Response Capture Integration (CRITICAL)

### Current State
- **Hook missing:** `service.OpenAIForwardResult` does NOT expose response body
- **Impact:** 0% capture rate (collector/worker/Drive never receive data)
- **Evidence:** `search_files` shows no `CaptureFromOpenAI|CollectorSingleton` in handler/service

### Contract

#### 1.1 Add ResponseBody Field
**File:** `backend/internal/service/openai_gateway.go`

```go
type OpenAIForwardResult struct {
    StatusCode   int
    ResponseBody []byte  // NEW: buffered response for dataset capture
    // ... existing fields
}
```

#### 1.2 Non-Streaming Capture Hook
**Location:** After successful forward, before writing to client

```go
// In ForwardAsChatCompletions (non-streaming path)
if cfg.Dataset.Enabled && result.StatusCode >= 200 && result.StatusCode < 300 {
    dataset.CaptureFromOpenAIRequest(requestBody, result.ResponseBody, result.StatusCode)
}
```

**Requirements:**
- ✅ Fail-open (no error propagation)
- ✅ After response received, before client write
- ✅ Only 2xx responses
- ✅ Pass raw `[]byte` (no JSON parsing in hot path)

#### 1.3 Test Contract
**File:** `backend/internal/service/openai_gateway_dataset_capture_test.go`

```go
func TestForwardAsChatCompletions_CapturesNonStreamingSuccess(t *testing.T)
func TestForwardAsChatCompletions_SkipsCaptureOn4xxAnd5xx(t *testing.T)
func TestForwardAsChatCompletions_SkipsCaptureWhenDisabled(t *testing.T)
```

---

## 2. Streaming Capture (CRITICAL)

### Current State
- **Bypass:** Streaming responses write directly to `c.Writer` without buffering
- **Impact:** Streaming requests (majority of traffic) never captured
- **Root cause:** `transfer.StreamJSON` tees to client only, no dataset hook

### Contract

#### 2.1 Tee Writer for Streaming
**File:** `backend/internal/service/openai_gateway.go`

```go
type capturingWriter struct {
    gin.ResponseWriter
    buf *bytes.Buffer
}

func (w *capturingWriter) Write(data []byte) (int, error) {
    w.buf.Write(data) // Capture
    return w.ResponseWriter.Write(data) // Forward
}
```

#### 2.2 Post-Stream Capture Hook
**Location:** After SSE stream completes

```go
// After stream done
if cfg.Dataset.Enabled && statusCode >= 200 && statusCode < 300 {
    // Reassemble complete response from SSE chunks
    completeResponse := reassembleSSEResponse(capturedWriter.buf.Bytes())
    dataset.CaptureFromOpenAIRequest(requestBody, completeResponse, statusCode)
}
```

**Requirements:**
- ✅ Zero client latency impact (async tee)
- ✅ Reassemble complete response after stream ends
- ✅ Handle partial/cancelled streams gracefully
- ✅ Bounded memory (10MB cap per stream)

#### 2.3 Test Contract
**File:** `backend/internal/service/openai_gateway_streaming_dataset_capture_test.go`

```go
func TestForwardAsChatCompletions_CapturesCompletedStream(t *testing.T)
func TestForwardAsChatCompletions_DropsPartialStreamOnCancel(t *testing.T)
func TestForwardAsChatCompletions_CapsStreamBufferAt10MB(t *testing.T)
```

---

## 3. PII/Credential Redaction (HIGH)

### Current State
- **Security risk:** `capture.go` line 42-87 captures raw request/response maps
- **Leak vector:** Authorization headers, API keys, user PII may reach Drive

### Contract

#### 3.1 Redaction Layer
**File:** `backend/internal/dataset/redact.go` (NEW)

```go
package dataset

func RedactSensitiveFields(entry *DatasetEntry) {
    // Request redaction
    if headers, ok := entry.Request["headers"].(map[string]any); ok {
        delete(headers, "Authorization")
        delete(headers, "X-API-Key")
        delete(headers, "Cookie")
    }

    // Response redaction
    if msg, ok := entry.Response["message"].(map[string]any); ok {
        content := msg["content"].(string)
        msg["content"] = redactPII(content)
    }
}

func redactPII(text string) string {
    // Email regex
    text = regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b`).
        ReplaceAllString(text, "[EMAIL_REDACTED]")

    // Phone regex (common formats)
    text = regexp.MustCompile(`\b\d{3}[-.]?\d{3}[-.]?\d{4}\b`).
        ReplaceAllString(text, "[PHONE_REDACTED]")

    // Credit card (simple pattern)
    text = regexp.MustCompile(`\b\d{4}[\s-]?\d{4}[\s-]?\d{4}[\s-]?\d{4}\b`).
        ReplaceAllString(text, "[CARD_REDACTED]")

    return text
}
```

#### 3.2 Integration Point
**File:** `backend/internal/dataset/capture.go` line 89

```go
// Before collector submit
RedactSensitiveFields(&entry)
CollectorSingleton.Collect(&entry)
```

#### 3.3 Test Contract
**File:** `backend/internal/dataset/redact_test.go`

```go
func TestRedactSensitiveFields_RemovesAuthHeaders(t *testing.T)
func TestRedactPII_MasksEmail(t *testing.T)
func TestRedactPII_MasksPhone(t *testing.T)
func TestRedactPII_MasksCreditCard(t *testing.T)
```

---

## 4. Worker Batch Durability (HIGH)

### Current State
- **Data loss risk:** `worker.go` line 77 drains before upload (line 96)
- **Failure mode:** Upload fails → batch already drained → data lost forever

### Contract

#### 4.1 Drain-Upload-Clear Pattern
**File:** `backend/internal/dataset/worker.go` line 76-105

```go
func (w *Worker) flushBatch(ctx context.Context) {
    // Drain WITHOUT removing
    entries := w.collector.Peek(w.batchSize) // NEW: non-destructive read
    if len(entries) == 0 {
        return
    }

    // Size check (existing)
    if totalSize > maxBytes {
        mid := len(entries) / 2
        entries = entries[:mid]
    }

    // Upload with retry
    var fileID string
    var err error
    for attempt := 1; attempt <= 3; attempt++ {
        fileID, err = w.driveClient.UploadBatch(ctx, entries)
        if err == nil {
            break
        }
        log.Printf("[Dataset] Upload attempt %d/3 failed: %v", attempt, err)
        time.Sleep(time.Duration(attempt*2) * time.Second) // Exponential backoff
    }

    if err != nil {
        log.Printf("[Dataset] Failed to upload after 3 attempts: %v", err)
        return // Entries stay in buffer for next tick
    }

    // Clear only after confirmed success
    w.collector.Clear(len(entries)) // NEW: destructive removal

    // Log (existing)
    total, dropped, buffered := w.collector.Stats()
    log.Printf("[Dataset] Batch uploaded: file_id=%s entries=%d total=%d dropped=%d buffered=%d",
        fileID, len(entries), total, dropped, buffered)
}
```

#### 4.2 Collector API Extension
**File:** `backend/internal/dataset/collector.go`

```go
// Peek returns entries without removing (for preview/upload)
func (c *Collector) Peek(limit int) []DatasetEntry {
    // Non-blocking snapshot
}

// Clear removes N entries from front (after successful upload)
func (c *Collector) Clear(count int) {
    for i := 0; i < count; i++ {
        select {
        case <-c.buffer:
        default:
            return
        }
    }
}
```

#### 4.3 Test Contract
**File:** `backend/internal/dataset/worker_durability_test.go`

```go
func TestWorker_RetainsEntriesOnUploadFailure(t *testing.T)
func TestWorker_RetriesUploadWithBackoff(t *testing.T)
func TestWorker_ClearsEntriesOnlyAfterSuccessfulUpload(t *testing.T)
```

---

## 5. Production OAuth Token Provisioning (MEDIUM)

### Current State
- The server never performs interactive OAuth and never reads authorization codes from
  stdin. Missing or invalid token files fail Dataset worker startup without blocking
  unrelated API traffic.
- OAuth credentials can come from a file path or decrypted inline JSON loaded from
  the database.

### Contract

#### 5.1 Token Path Resolution

`backend/internal/dataset/drive_client.go` resolves the token file as follows:

1. If `DATASET_GOOGLE_DRIVE_TOKEN_PATH` is set, use that deployment-only path.
2. For file credentials without an explicit token path, use
   `<credentials>.token` for backward compatibility.
3. For inline JSON credentials, require an explicit
   `DATASET_GOOGLE_DRIVE_TOKEN_PATH`; inline JSON has no stable filesystem path.

The token file must be generated by an approved offline operator procedure,
mounted read-only into the container, and kept outside the database. The server
must not be used to complete OAuth interactively.

#### 5.2 Fail-Open Startup Behavior

`NewDriveClientWithTokenPath` returns an error when credentials or the token file
are missing/invalid. `InitializeDatasetCollection` logs only sanitized state and
leaves the main API available; it does not prompt, panic, or block on stdin.

#### 5.3 Deployment Contract

Before enabling Dataset in the database or runtime configuration:

```bash
# Verify the deployment-only token exists with restrictive permissions.
test -r /path/to/google_drive_credentials.json.token
chmod 600 /path/to/google_drive_credentials.json.token

# Mount credentials and token read-only in the production compose service.
# Set DATASET_GOOGLE_DRIVE_TOKEN_PATH to the in-container token path when
# the database credential is inline JSON.
```

Keep `DATASET_ENABLED=false` until token readability, Drive folder access, and
an approved upload probe have been verified.

#### 5.4 Test Contract

The dataset unit tests cover:

```go
func TestNewDriveClient_RequiresToken(t *testing.T)
func TestNewDriveClient_InlineCredentialsRequireExplicitTokenPath(t *testing.T)
func TestNewDriveClientWithTokenPath_AcceptsInlineCredentials(t *testing.T)
func TestNewDriveClient_WithValidToken(t *testing.T)
```

---

## 6. Runtime Config Reload (MEDIUM)

### Current State
- **Static init:** `integration.go` line 20-24 creates singleton once at server start
- **Admin UI gap:** Settings UI allows credential updates, but worker ignores them until restart

### Contract

#### 6.1 Config Watch + Worker Restart
**File:** `backend/internal/dataset/integration.go`

```go
var (
    CollectorSingleton *Collector
    WorkerSingleton    *Worker
    workerMu           sync.RWMutex
    currentConfig      *config.DatasetConfig
)

// InitializeDatasetCollection starts initial worker + config watcher
func InitializeDatasetCollection(ctx context.Context, cfg *config.Config) func() {
    if cfg == nil || !cfg.Dataset.Enabled {
        log.Println("[Dataset] Collection disabled in config")
        return func() {}
    }

    currentConfig = &cfg.Dataset

    // Start initial worker
    startWorker(ctx, cfg)

    // Watch for config changes (via DB settings)
    stopWatch := watchConfigChanges(ctx, cfg)

    return func() {
        stopWatch()
        stopWorker(context.Background())
    }
}

func startWorker(ctx context.Context, cfg *config.Config) {
    workerMu.Lock()
    defer workerMu.Unlock()

    // Stop existing worker
    if WorkerSingleton != nil {
        shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
        defer cancel()
        WorkerSingleton.Stop(shutdownCtx)
    }

    // Create Drive client with new credentials
    driveClient, err := NewDriveClient(ctx, cfg.Dataset.GoogleDriveCredentials, cfg.Dataset.GoogleDriveFolderID)
    if err != nil {
        log.Printf("[Dataset] WARN: Failed to create Drive client: %v", err)
        return
    }

    // Reuse collector (preserves buffered entries)
    if CollectorSingleton == nil {
        CollectorSingleton = NewCollector(cfg.Dataset.BufferMaxItems)
    }

    // Create new worker
    WorkerSingleton = NewWorker(CollectorSingleton, driveClient,
        cfg.Dataset.BatchSize, cfg.Dataset.BatchMaxMB, cfg.Dataset.BatchIntervalSec)
    WorkerSingleton.Start()

    log.Println("[Dataset] Worker (re)started with new config")
}

func watchConfigChanges(ctx context.Context, cfg *config.Config) func() {
    ticker := time.NewTicker(30 * time.Second)
    stopCh := make(chan struct{})

    go func() {
        for {
            select {
            case <-ticker.C:
                // Reload from DB (via SettingService)
                newCfg := loadDatasetConfigFromDB(ctx)
                if newCfg != nil && configChanged(currentConfig, newCfg) {
                    log.Println("[Dataset] Config changed, restarting worker")
                    cfg.Dataset = *newCfg
                    currentConfig = newCfg
                    startWorker(ctx, cfg)
                }
            case <-stopCh:
                ticker.Stop()
                return
            }
        }
    }()

    return func() { close(stopCh) }
}

func configChanged(old, new *config.DatasetConfig) bool {
    return old.GoogleDriveCredentials != new.GoogleDriveCredentials ||
           old.GoogleDriveFolderID != new.GoogleDriveFolderID ||
           old.BatchSize != new.BatchSize
}
```

#### 6.2 DB Integration
**File:** `backend/internal/service/dataset_settings.go`

```go
// LoadDatasetConfig loads current dataset config from encrypted DB settings
func (s *SettingService) LoadDatasetConfig(ctx context.Context) (*config.DatasetConfig, error) {
    // Fetch from settings table (existing Admin Settings implementation)
    // Decrypt credentials
    // Return config struct
}
```

#### 6.3 Test Contract
**File:** `backend/internal/dataset/integration_reload_test.go`

```go
func TestConfigWatch_RestartsWorkerOnCredentialChange(t *testing.T)
func TestConfigWatch_RestartsWorkerOnFolderIDChange(t *testing.T)
func TestConfigWatch_PreservesCollectorBufferAcrossRestart(t *testing.T)
```

---

## Summary: Implementation Order

### Phase 1: Core Infrastructure (Actions 3, 4)
1. ✅ Redaction layer (`redact.go` + tests)
2. ✅ Worker durability (`worker.go` + `collector.go` extensions + tests)

### Phase 2: Capture Integration (Actions 1, 2)
3. ✅ Non-streaming capture (`openai_gateway.go` + tests)
4. ✅ Streaming capture (tee writer + reassembly + tests)

### Phase 3: Production Readiness (Actions 5, 6)
5. ✅ OAuth setup script + Drive client fallback + tests
6. ✅ Config watch + worker restart + DB integration + tests

### Verification Checklist
- [ ] `make test` passes (all new + existing tests)
- [ ] `make lint` passes (golangci-lint clean)
- [ ] `go vet ./...` passes
- [ ] Manual capture test: send request → verify JSONL in Drive
- [ ] Manual streaming test: SSE request → verify complete response captured
- [ ] Manual redaction test: send email/phone → verify redacted in JSONL
- [ ] Manual failure test: simulate Drive outage → verify entries retained
- [ ] Manual reload test: update credentials in Admin Settings → verify worker restarts

---

**Next Action:** Implement Phase 1 (redaction + durability) with TDD approach.
