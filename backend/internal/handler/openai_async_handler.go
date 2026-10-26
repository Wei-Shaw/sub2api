package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/openaiasync"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type OpenAIAsyncHandler struct {
	store            *openaiasync.Store
	config           config.OpenAIAsyncConfig
	keys             *service.APIKeyService
	trustForwardedIP bool
	worker           *openaiasync.Worker
	start            sync.Once
	stop             sync.Once
}

func NewOpenAIAsyncHandler(db *sql.DB, cfg *config.Config, keys *service.APIKeyService) *OpenAIAsyncHandler {
	return &OpenAIAsyncHandler{store: openaiasync.NewStore(db, cfg.OpenAIAsync.UserStorageBytes, cfg.OpenAIAsync.MaxActiveBatches), config: cfg.OpenAIAsync, keys: keys, trustForwardedIP: cfg.TrustForwardedIPForAPIKeyACL()}
}

func (h *OpenAIAsyncHandler) Start(router http.Handler) {
	if h == nil || !h.config.Enabled {
		return
	}
	h.start.Do(func() {
		h.worker = openaiasync.NewWorker(h.store, func(ctx context.Context, j *openaiasync.Job, i *openaiasync.Item) openaiasync.Result {
			return h.execute(ctx, router, j, i)
		}, h.config.WorkerConcurrency, h.config.MaxOutputBytes, time.Duration(h.config.RequestTimeoutSeconds)*time.Second)
		h.worker.Start()
	})
}

func (h *OpenAIAsyncHandler) Stop() {
	if h != nil {
		h.stop.Do(func() {
			if h.worker != nil {
				h.worker.Stop()
			}
		})
	}
}

func asyncJSONError(c *gin.Context, status int, message string) {
	c.AbortWithStatusJSON(status, gin.H{"error": gin.H{"type": "invalid_request_error", "message": message}})
}

func (h *OpenAIAsyncHandler) auth(c *gin.Context) (*service.APIKey, bool) {
	if h == nil || !h.config.Enabled {
		asyncJSONError(c, http.StatusNotFound, "Local Files and Batch APIs are disabled")
		return nil, false
	}
	k, ok := middleware.GetAPIKeyFromContext(c)
	if !ok || k == nil {
		asyncJSONError(c, http.StatusUnauthorized, "API key context not found")
		return nil, false
	}
	return k, true
}

func (h *OpenAIAsyncHandler) handleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, openaiasync.ErrNotFound):
		asyncJSONError(c, http.StatusNotFound, "Resource not found")
	case errors.Is(err, openaiasync.ErrQuota):
		asyncJSONError(c, http.StatusTooManyRequests, err.Error())
	default:
		slog.Error("local OpenAI API storage failed", "error", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": gin.H{"type": "api_error", "message": "Local resource storage failed"}})
	}
}

func (h *OpenAIAsyncHandler) Upload(c *gin.Context) {
	k, ok := h.auth(c)
	if !ok {
		return
	}
	if c.Request.MultipartForm != nil {
		defer func() { _ = c.Request.MultipartForm.RemoveAll() }()
	}
	reader, err := c.Request.MultipartReader()
	if err != nil {
		asyncJSONError(c, 400, "Expected multipart/form-data")
		return
	}
	var filename, purpose string
	var body []byte
	ttl := h.config.FileTTLSeconds
	seen := map[string]bool{}
	for {
		part, readErr := reader.NextPart()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			asyncJSONError(c, 400, "Invalid multipart body")
			return
		}
		name := part.FormName()
		if seen[name] {
			_ = part.Close()
			asyncJSONError(c, 400, "Duplicate multipart field")
			return
		}
		seen[name] = true
		limit := int64(1024)
		if name == "file" {
			limit = h.config.MaxFileBytes
		}
		data, readErr := io.ReadAll(io.LimitReader(part, limit+1))
		_ = part.Close()
		if readErr != nil {
			asyncJSONError(c, 400, "Unable to read multipart field")
			return
		}
		if int64(len(data)) > limit {
			asyncJSONError(c, 413, "Multipart field exceeds the local size limit")
			return
		}
		switch name {
		case "file":
			filename = filepath.Base(strings.ReplaceAll(part.FileName(), "\\", "/"))
			body = data
		case "purpose":
			purpose = string(data)
		case "expires_after[anchor]":
			if string(data) != "created_at" {
				asyncJSONError(c, 400, "expires_after anchor must be created_at")
				return
			}
		case "expires_after[seconds]":
			ttl, err = strconv.ParseInt(string(data), 10, 64)
			if err != nil || ttl < 3600 || ttl > 2592000 {
				asyncJSONError(c, 400, "File expiry must be between 3600 and 2592000 seconds")
				return
			}
		default:
			asyncJSONError(c, 400, "Unsupported multipart field")
			return
		}
	}
	if !seen["file"] || filename == "" || filename == "." || len(filename) > 255 || !utf8.ValidString(filename) || strings.ContainsRune(filename, '\x00') {
		asyncJSONError(c, 400, "A file with a valid filename is required")
		return
	}
	if seen["expires_after[anchor]"] != seen["expires_after[seconds]"] {
		asyncJSONError(c, 400, "expires_after requires anchor and seconds")
		return
	}
	switch purpose {
	case "batch":
		if !strings.HasSuffix(strings.ToLower(filename), ".jsonl") {
			asyncJSONError(c, 400, "Batch files must have a .jsonl filename")
			return
		}
	default:
		asyncJSONError(c, 400, "Local file purpose must be batch")
		return
	}
	f, err := h.store.PutFile(c.Request.Context(), k.UserID, filename, purpose, body, time.Duration(ttl)*time.Second)
	if err != nil {
		h.handleError(c, err)
		return
	}
	c.JSON(200, f)
}

func parsePage(c *gin.Context, max int) (int, bool) {
	limit := 20
	if raw, ok := c.GetQuery("limit"); ok {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > max {
			asyncJSONError(c, 400, fmt.Sprintf("limit must be between 1 and %d", max))
			return 0, false
		}
		limit = n
	}
	return limit, true
}

func listPayload[T any](data []T, limit int, id func(T) string) gin.H {
	hasMore := len(data) > limit
	if hasMore {
		data = data[:limit]
	}
	var first, last any
	if len(data) > 0 {
		first = id(data[0])
		last = id(data[len(data)-1])
	}
	return gin.H{"object": "list", "data": data, "first_id": first, "last_id": last, "has_more": hasMore}
}

func (h *OpenAIAsyncHandler) ListFiles(c *gin.Context) {
	k, ok := h.auth(c)
	if !ok {
		return
	}
	limit, ok := parsePage(c, 10000)
	if !ok {
		return
	}
	order := c.DefaultQuery("order", "desc")
	if order != "asc" && order != "desc" {
		asyncJSONError(c, 400, "order must be asc or desc")
		return
	}
	files, err := h.store.ListFiles(c.Request.Context(), k.UserID, c.Query("purpose"), c.Query("after"), order == "asc", limit+1)
	if err != nil {
		h.handleError(c, err)
		return
	}
	c.JSON(200, listPayload(files, limit, func(f openaiasync.File) string { return f.ID }))
}

func (h *OpenAIAsyncHandler) GetFile(c *gin.Context) {
	k, ok := h.auth(c)
	if !ok {
		return
	}
	f, err := h.store.FileMetadata(c.Request.Context(), k.UserID, c.Param("file_id"))
	if err != nil {
		h.handleError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(200, f)
}

func (h *OpenAIAsyncHandler) FileContent(c *gin.Context) {
	k, ok := h.auth(c)
	if !ok {
		return
	}
	f, body, err := h.store.GetFile(c.Request.Context(), k.UserID, c.Param("file_id"))
	if err != nil {
		h.handleError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": f.Filename}))
	c.Data(200, "application/octet-stream", body)
}

func (h *OpenAIAsyncHandler) DeleteFile(c *gin.Context) {
	k, ok := h.auth(c)
	if !ok {
		return
	}
	id := c.Param("file_id")
	if err := h.store.DeleteFile(c.Request.Context(), k.UserID, id); err != nil {
		h.handleError(c, err)
		return
	}
	c.JSON(200, gin.H{"id": id, "object": "file", "deleted": true})
}

func (h *OpenAIAsyncHandler) CreateBatch(c *gin.Context) {
	k, ok := h.auth(c)
	if !ok {
		return
	}
	if k.GroupID == nil {
		asyncJSONError(c, 400, "A group must be assigned before submitting a batch")
		return
	}
	var req struct {
		InputFileID        string            `json:"input_file_id"`
		Endpoint           string            `json:"endpoint"`
		CompletionWindow   string            `json:"completion_window"`
		Metadata           map[string]string `json:"metadata"`
		OutputExpiresAfter *struct {
			Anchor  string `json:"anchor"`
			Seconds int64  `json:"seconds"`
		} `json:"output_expires_after"`
	}
	dec := json.NewDecoder(io.LimitReader(c.Request.Body, 65537))
	dec.DisallowUnknownFields()
	if dec.Decode(&req) != nil || dec.Decode(new(any)) != io.EOF {
		asyncJSONError(c, 400, "Invalid batch request")
		return
	}
	if req.CompletionWindow != "24h" || !openaiasync.ValidEndpoint(req.Endpoint) {
		asyncJSONError(c, 400, "Use completion_window=24h and a supported local batch endpoint")
		return
	}
	if len(req.Metadata) > 16 {
		asyncJSONError(c, 400, "metadata supports at most 16 entries")
		return
	}
	for key, value := range req.Metadata {
		if utf8.RuneCountInString(key) > 64 || utf8.RuneCountInString(value) > 512 || strings.ContainsRune(key, '\x00') || strings.ContainsRune(value, '\x00') {
			asyncJSONError(c, 400, "metadata keys and values exceed the length limit")
			return
		}
	}
	ttl := h.config.FileTTLSeconds
	if req.OutputExpiresAfter != nil {
		ttl = req.OutputExpiresAfter.Seconds
		if req.OutputExpiresAfter.Anchor != "created_at" || ttl < 3600 || ttl > 2592000 {
			asyncJSONError(c, 400, "Invalid output file expiration policy")
			return
		}
	}
	f, body, err := h.store.GetFile(c.Request.Context(), k.UserID, req.InputFileID)
	if err != nil {
		h.handleError(c, err)
		return
	}
	if f.Purpose != "batch" {
		asyncJSONError(c, 400, "Input file must have purpose=batch")
		return
	}
	items, err := openaiasync.ParseInput(body, req.Endpoint, h.config.MaxBatchRequests, h.config.MaxLineBytes)
	if err != nil {
		asyncJSONError(c, 400, err.Error())
		return
	}
	now := time.Now().Unix()
	j := &openaiasync.Job{Batch: openaiasync.Batch{ID: openaiasync.NewID("batch_"), Object: "batch", Endpoint: req.Endpoint, InputFileID: req.InputFileID, CompletionWindow: "24h", Status: "validating", CreatedAt: now, ExpiresAt: now + 86400, RequestCounts: openaiasync.Counts{Total: len(items)}, Metadata: req.Metadata}, UserID: k.UserID, APIKeyID: k.ID, GroupID: *k.GroupID, RemoteIP: ip.GetSecurityClientIP(c, h.trustForwardedIP), OutputTTL: time.Duration(ttl) * time.Second}
	if err = h.store.CreateBatch(c.Request.Context(), j, items); err != nil {
		h.handleError(c, err)
		return
	}
	c.JSON(200, j.Batch)
}

func (h *OpenAIAsyncHandler) GetBatch(c *gin.Context) {
	k, ok := h.auth(c)
	if !ok {
		return
	}
	b, err := h.store.GetBatch(c.Request.Context(), k.UserID, c.Param("batch_id"))
	if err != nil {
		h.handleError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(200, b)
}

func (h *OpenAIAsyncHandler) ListBatches(c *gin.Context) {
	k, ok := h.auth(c)
	if !ok {
		return
	}
	limit, ok := parsePage(c, 100)
	if !ok {
		return
	}
	batches, err := h.store.ListBatches(c.Request.Context(), k.UserID, c.Query("after"), limit+1)
	if err != nil {
		h.handleError(c, err)
		return
	}
	c.JSON(200, listPayload(batches, limit, func(b openaiasync.Batch) string { return b.ID }))
}

func (h *OpenAIAsyncHandler) CancelBatch(c *gin.Context) {
	k, ok := h.auth(c)
	if !ok {
		return
	}
	b, err := h.store.Cancel(c.Request.Context(), k.UserID, c.Param("batch_id"))
	if err != nil {
		h.handleError(c, err)
		return
	}
	c.JSON(200, b)
}

// Bounded response capture is used only for non-streaming batch requests.
type batchResponseWriter struct {
	header   http.Header
	body     bytes.Buffer
	status   int
	limit    int64
	overflow bool
}

func (w *batchResponseWriter) Header() http.Header { return w.header }
func (w *batchResponseWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}
func (w *batchResponseWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = 200
	}
	if int64(w.body.Len())+int64(len(p)) > w.limit {
		w.overflow = true
		return len(p), nil
	}
	if w.overflow {
		return len(p), nil
	}
	return w.body.Write(p)
}

func (h *OpenAIAsyncHandler) execute(ctx context.Context, router http.Handler, j *openaiasync.Job, i *openaiasync.Item) openaiasync.Result {
	k, err := h.keys.GetByID(ctx, j.APIKeyID)
	if err != nil || k == nil || k.UserID != j.UserID || k.GroupID == nil || *k.GroupID != j.GroupID {
		return openaiasync.Failure(i.CustomID, "batch_authorization_changed", "The submitting API key or its group is no longer available")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, j.Endpoint, bytes.NewReader(i.Body))
	if err != nil {
		return openaiasync.Failure(i.CustomID, "invalid_request", "Cannot construct batch request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+k.Key)
	req.Header.Set("X-Client-Request-Id", j.ID+":"+strconv.Itoa(i.Ordinal))
	req.RemoteAddr = net.JoinHostPort(j.RemoteIP, "0")
	writer := &batchResponseWriter{header: make(http.Header), limit: h.config.MaxResponseBytes}
	router.ServeHTTP(writer, req)
	if writer.overflow {
		return openaiasync.Failure(i.CustomID, "response_too_large", "Response exceeded the local per-request size limit")
	}
	if ctx.Err() != nil {
		return openaiasync.Failure(i.CustomID, "request_timeout", "Execution timed out; the upstream outcome may be unknown. This request was not retried.")
	}
	if !json.Valid(writer.body.Bytes()) {
		return openaiasync.Failure(i.CustomID, "invalid_response", "Gateway returned an invalid JSON response")
	}
	status := writer.status
	if status == 0 {
		status = 200
	}
	return openaiasync.Result{ID: openaiasync.NewID("batch_req_"), CustomID: i.CustomID, Response: &openaiasync.Response{StatusCode: status, RequestID: writer.header.Get("X-Request-Id"), Body: append(json.RawMessage(nil), writer.body.Bytes()...)}}
}
