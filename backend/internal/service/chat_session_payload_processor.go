package service

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const (
	ChatSessionPayloadQueueKey       = "chat_session:payload:pending"
	chatSessionPayloadCompression    = "gzip"
	chatSessionMediaInlineMinBytes   = 16 * 1024
	chatSessionMediaReferenceType    = "chat_session_media_ref"
	defaultChatSessionPayloadBaseDir = "./data/chat_session_payloads"
)

type ChatSessionPayloadTask struct {
	SessionKey   string `json:"session_key,omitempty"`
	RequestID    string `json:"request_id,omitempty"`
	UserID       int64  `json:"user_id,omitempty"`
	APIKeyID     int64  `json:"api_key_id,omitempty"`
	Role         string `json:"role,omitempty"`
	Direction    string `json:"direction,omitempty"`
	Kind         string `json:"kind,omitempty"`
	Path         string `json:"path"`
	SHA256       string `json:"sha256,omitempty"`
	Bytes        int64  `json:"bytes,omitempty"`
	StoredBytes  int64  `json:"stored_bytes,omitempty"`
	Compression  string `json:"compression,omitempty"`
	CreatedAtUTC string `json:"created_at_utc,omitempty"`
}

type ChatSessionPayloadProcessor struct {
	db      *sql.DB
	rdb     *redis.Client
	baseDir string
	cancel  context.CancelFunc
	done    chan struct{}
	once    sync.Once
}

func ProvideChatSessionPayloadProcessor(db *sql.DB, rdb *redis.Client, cfg *config.Config) *ChatSessionPayloadProcessor {
	p := NewChatSessionPayloadProcessor(db, rdb, cfg)
	p.Start()
	return p
}

func NewChatSessionPayloadProcessor(db *sql.DB, rdb *redis.Client, cfg *config.Config) *ChatSessionPayloadProcessor {
	baseDir := defaultChatSessionPayloadBaseDir
	if cfg != nil && strings.TrimSpace(cfg.ChatSessionRetention.PayloadDir) != "" {
		baseDir = strings.TrimSpace(cfg.ChatSessionRetention.PayloadDir)
	}
	return &ChatSessionPayloadProcessor{
		db:      db,
		rdb:     rdb,
		baseDir: filepath.Clean(baseDir),
		done:    make(chan struct{}),
	}
}

func (p *ChatSessionPayloadProcessor) Start() {
	if p == nil || p.db == nil || p.rdb == nil {
		return
	}
	p.once.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		p.cancel = cancel
		go p.run(ctx)
	})
}

func (p *ChatSessionPayloadProcessor) Stop() {
	if p == nil || p.cancel == nil {
		return
	}
	p.cancel()
	select {
	case <-p.done:
	case <-time.After(3 * time.Second):
		logger.L().Warn("chat_session.payload_processor_stop_timeout")
	}
}

func (p *ChatSessionPayloadProcessor) run(ctx context.Context) {
	defer close(p.done)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		result, err := p.rdb.BRPop(ctx, 5*time.Second, ChatSessionPayloadQueueKey).Result()
		if err != nil {
			if err != redis.Nil && ctx.Err() == nil {
				logger.L().Warn("chat_session.payload_queue_pop_failed", zap.Error(err))
			}
			continue
		}
		if len(result) < 2 {
			continue
		}
		if err := p.processRawTask(ctx, []byte(result[1])); err != nil {
			logger.L().Warn("chat_session.payload_queue_process_failed", zap.Error(err))
		}
	}
}

func (p *ChatSessionPayloadProcessor) processRawTask(ctx context.Context, raw []byte) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || !json.Valid(raw) {
		return nil
	}
	var task ChatSessionPayloadTask
	if err := json.Unmarshal(raw, &task); err != nil {
		return err
	}
	return p.processTask(ctx, task)
}

func (p *ChatSessionPayloadProcessor) processTask(ctx context.Context, task ChatSessionPayloadTask) error {
	if p == nil || p.db == nil {
		return nil
	}
	path := strings.TrimSpace(task.Path)
	if path == "" {
		return nil
	}
	sha := strings.TrimSpace(task.SHA256)
	resultPayload, processErr := p.rewritePayloadMedia(ctx, task, path, sha)
	status := "processed"
	var processedError any
	if processErr != nil {
		status = "failed"
		processedError = processErr.Error()
	}
	startedAt := time.Now()
	result, err := p.db.ExecContext(ctx, `
		UPDATE chat_messages
		SET processed_status = $3,
			processed_at = NOW(),
			processed_error = $4::text,
			content_sha256 = COALESCE(NULLIF($5::text, ''), content_sha256),
			content_bytes = COALESCE($6::bigint, content_bytes),
			content_stored_bytes = COALESCE($7::bigint, content_stored_bytes)
		WHERE content_path = $1
			AND ($2::text = '' OR content_sha256 = $2::text)
			AND (processed_status IS NULL OR processed_status <> 'processed')
	`, path, sha, status, processedError, resultPayload.SHA256, nullableInt64Value(resultPayload.Bytes), nullableInt64Value(resultPayload.StoredBytes))
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if processErr != nil {
		logger.L().Warn("chat_session.payload_queue_processed_failed",
			zap.String("path", path),
			zap.Error(processErr),
			zap.Int64("rows", rows),
			zap.Duration("duration", time.Since(startedAt)),
		)
		return nil
	}
	logger.L().Debug("chat_session.payload_queue_processed",
		zap.String("path", path),
		zap.Int64("rows", rows),
		zap.Duration("duration", time.Since(startedAt)),
	)
	return nil
}

type chatSessionPayloadProcessResult struct {
	SHA256      string
	Bytes       int64
	StoredBytes int64
}

type chatSessionMediaRef struct {
	Type          string `json:"type"`
	Storage       string `json:"storage"`
	Path          string `json:"path"`
	SHA256        string `json:"sha256"`
	Bytes         int64  `json:"bytes"`
	MimeType      string `json:"mime_type,omitempty"`
	Encoding      string `json:"encoding"`
	OriginalBytes int64  `json:"original_bytes,omitempty"`
}

func (p *ChatSessionPayloadProcessor) rewritePayloadMedia(ctx context.Context, task ChatSessionPayloadTask, relPath string, expectedSHA string) (chatSessionPayloadProcessResult, error) {
	fullPath, err := p.safePath(relPath)
	if err != nil {
		return chatSessionPayloadProcessResult{}, err
	}
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return chatSessionPayloadProcessResult{}, err
	}
	if strings.TrimSpace(task.Compression) == chatSessionPayloadCompression || strings.HasSuffix(fullPath, ".gz") {
		data, err = gunzipChatSessionPayload(data)
		if err != nil {
			return chatSessionPayloadProcessResult{}, err
		}
	}
	if expectedSHA != "" {
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != expectedSHA {
			return chatSessionPayloadProcessResult{}, errors.New("chat session payload sha256 mismatch")
		}
	}
	var decoded any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		return chatSessionPayloadProcessResult{}, err
	}
	stats := chatSessionMediaRewriteStats{}
	rewritten := p.rewriteMediaValue(ctx, decoded, payloadDateDir(task, relPath), &stats)
	if stats.Count == 0 {
		return chatSessionPayloadProcessResult{}, nil
	}
	rewrittenJSON, err := json.Marshal(rewritten)
	if err != nil {
		return chatSessionPayloadProcessResult{}, err
	}
	compressed, err := gzipChatSessionPayload(rewrittenJSON)
	if err != nil {
		return chatSessionPayloadProcessResult{}, err
	}
	if err := writeFileAtomic(fullPath, compressed, 0640); err != nil {
		return chatSessionPayloadProcessResult{}, err
	}
	sum := sha256.Sum256(rewrittenJSON)
	return chatSessionPayloadProcessResult{
		SHA256:      hex.EncodeToString(sum[:]),
		Bytes:       int64(len(rewrittenJSON)),
		StoredBytes: int64(len(compressed)),
	}, nil
}

type chatSessionMediaRewriteStats struct {
	Count int
	Bytes int64
}

func (p *ChatSessionPayloadProcessor) rewriteMediaValue(ctx context.Context, value any, dateDir string, stats *chatSessionMediaRewriteStats) any {
	select {
	case <-ctx.Done():
		return value
	default:
	}
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			v[key] = p.rewriteMediaValue(ctx, child, dateDir, stats)
		}
		return v
	case []any:
		for i, child := range v {
			v[i] = p.rewriteMediaValue(ctx, child, dateDir, stats)
		}
		return v
	case string:
		ref, ok := p.externalizeMediaString(v, dateDir)
		if !ok {
			return v
		}
		stats.Count++
		stats.Bytes += ref.Bytes
		return ref
	default:
		return value
	}
}

func (p *ChatSessionPayloadProcessor) externalizeMediaString(value string, dateDir string) (chatSessionMediaRef, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return chatSessionMediaRef{}, false
	}
	mimeType := ""
	encoded := value
	if strings.HasPrefix(value, "data:") {
		header, body, ok := strings.Cut(value, ",")
		if !ok || !strings.Contains(header, ";base64") {
			return chatSessionMediaRef{}, false
		}
		mimeType = strings.TrimPrefix(strings.TrimSuffix(header, ";base64"), "data:")
		encoded = body
	} else if len(value) < chatSessionMediaInlineMinBytes {
		return chatSessionMediaRef{}, false
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(encoded)
	}
	if err != nil || len(decoded) < chatSessionMediaInlineMinBytes {
		return chatSessionMediaRef{}, false
	}
	if mimeType == "" && !looksLikeSupportedImage(decoded) {
		return chatSessionMediaRef{}, false
	}
	sum := sha256.Sum256(decoded)
	sumHex := hex.EncodeToString(sum[:])
	ext := mediaExtension(mimeType, decoded)
	mediaRelPath := filepath.ToSlash(filepath.Join(dateDir, "media", sumHex+ext))
	fullPath, err := p.safePath(mediaRelPath)
	if err != nil {
		return chatSessionMediaRef{}, false
	}
	if err := os.MkdirAll(filepath.Dir(fullPath), 0750); err != nil {
		return chatSessionMediaRef{}, false
	}
	if _, err := os.Stat(fullPath); errors.Is(err, os.ErrNotExist) {
		if err := writeFileAtomic(fullPath, decoded, 0640); err != nil {
			return chatSessionMediaRef{}, false
		}
	}
	return chatSessionMediaRef{
		Type:          chatSessionMediaReferenceType,
		Storage:       "file",
		Path:          mediaRelPath,
		SHA256:        sumHex,
		Bytes:         int64(len(decoded)),
		MimeType:      mimeType,
		Encoding:      "base64",
		OriginalBytes: int64(len(value)),
	}, true
}

func payloadDateDir(task ChatSessionPayloadTask, relPath string) string {
	if task.CreatedAtUTC != "" {
		if ts, err := time.Parse(time.RFC3339Nano, task.CreatedAtUTC); err == nil {
			return ts.UTC().Format("2006-01-02")
		}
	}
	first, _, ok := strings.Cut(filepath.ToSlash(strings.TrimSpace(relPath)), "/")
	if ok {
		if _, err := time.Parse("2006-01-02", first); err == nil {
			return first
		}
	}
	return time.Now().UTC().Format("2006-01-02")
}

func mediaExtension(mimeType string, data []byte) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/png":
		return ".png"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	}
	if exts, err := mime.ExtensionsByType(mimeType); err == nil && len(exts) > 0 {
		return exts[0]
	}
	switch {
	case bytes.HasPrefix(data, []byte{0x89, 'P', 'N', 'G'}):
		return ".png"
	case bytes.HasPrefix(data, []byte{0xff, 0xd8, 0xff}):
		return ".jpg"
	case bytes.HasPrefix(data, []byte("GIF87a")), bytes.HasPrefix(data, []byte("GIF89a")):
		return ".gif"
	case len(data) > 12 && bytes.Equal(data[0:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		return ".webp"
	default:
		return ".bin"
	}
}

func looksLikeSupportedImage(data []byte) bool {
	return mediaExtension("", data) != ".bin"
}

func (p *ChatSessionPayloadProcessor) safePath(relPath string) (string, error) {
	if p == nil || strings.TrimSpace(p.baseDir) == "" {
		return "", errors.New("chat session payload processor not configured")
	}
	cleanRel := filepath.Clean(filepath.FromSlash(strings.TrimSpace(relPath)))
	if cleanRel == "." || filepath.IsAbs(cleanRel) || strings.HasPrefix(cleanRel, ".."+string(filepath.Separator)) || cleanRel == ".." {
		return "", errors.New("invalid chat session payload path")
	}
	baseAbs, err := filepath.Abs(p.baseDir)
	if err != nil {
		return "", err
	}
	fullPath := filepath.Join(baseAbs, cleanRel)
	fullAbs, err := filepath.Abs(fullPath)
	if err != nil {
		return "", err
	}
	if fullAbs != baseAbs && !strings.HasPrefix(fullAbs, baseAbs+string(filepath.Separator)) {
		return "", errors.New("chat session payload path escapes base dir")
	}
	return fullAbs, nil
}

func gzipChatSessionPayload(raw []byte) ([]byte, error) {
	var buf bytes.Buffer
	writer, err := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
	if err != nil {
		return nil, err
	}
	if _, err := writer.Write(raw); err != nil {
		_ = writer.Close()
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func gunzipChatSessionPayload(raw []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func nullableInt64Value(value int64) any {
	if value <= 0 {
		return nil
	}
	return value
}
