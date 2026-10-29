// Package conversationlog records bounded, opt-in conversation payloads without
// making database writes part of the model's streaming response path.
package conversationlog

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Config struct {
	Enabled         bool `json:"enabled"`
	RetentionDays   int  `json:"retention_days"`
	MaxBodyBytes    int  `json:"max_body_bytes"`
	RedactSensitive bool `json:"redact_sensitive"`
}

func (c Config) validate() error {
	if c.RetentionDays < 1 || c.RetentionDays > 365 {
		return fmt.Errorf("retention_days must be between 1 and 365")
	}
	if c.MaxBodyBytes < 16384 || c.MaxBodyBytes > 2<<20 {
		return fmt.Errorf("max_body_bytes must be between 16384 and 2097152")
	}
	return nil
}

type Record struct {
	ID                int64           `json:"id"`
	RequestID         string          `json:"request_id"`
	CreatedAt         time.Time       `json:"created_at"`
	UserID            int64           `json:"user_id"`
	APIKeyID          int64           `json:"api_key_id"`
	UserLabel         string          `json:"user_label"`
	KeyName           string          `json:"key_name"`
	Endpoint          string          `json:"endpoint"`
	Model             string          `json:"model"`
	Stream            bool            `json:"stream"`
	StatusCode        int             `json:"status_code"`
	Outcome           string          `json:"outcome"`
	DurationMS        int64           `json:"duration_ms"`
	Prompt            string          `json:"prompt"`
	Answer            string          `json:"answer"`
	RequestBody       string          `json:"request_body,omitempty"`
	ResponseBody      string          `json:"response_body,omitempty"`
	ToolCalls         json.RawMessage `json:"tool_calls,omitempty"`
	RequestTruncated  bool            `json:"request_truncated"`
	ResponseTruncated bool            `json:"response_truncated"`
	RequestBytes      int64           `json:"request_bytes"`
	ResponseBytes     int64           `json:"response_bytes"`
	Redacted          bool            `json:"redacted"`
}

type Recorder struct {
	db      *sql.DB
	config  atomic.Pointer[Config]
	queue   chan Record
	slots   chan struct{}
	mu      sync.RWMutex
	closed  bool
	done    chan struct{}
	dropped atomic.Uint64
	failed  atomic.Uint64
	saved   atomic.Uint64
}

func NewRecorder(db *sql.DB) *Recorder {
	r := &Recorder{db: db, queue: make(chan Record, 32), slots: make(chan struct{}, 32), done: make(chan struct{})}
	cfg := Config{RetentionDays: 7, MaxBodyBytes: 1 << 20, RedactSensitive: true}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.QueryRowContext(ctx, "SELECT enabled,retention_days,max_body_bytes,redact_sensitive FROM conversation_log_config WHERE id=1").Scan(&cfg.Enabled, &cfg.RetentionDays, &cfg.MaxBodyBytes, &cfg.RedactSensitive); err != nil {
		cfg.Enabled = false
		slog.Error("conversation logs: config unavailable, capture disabled", "error", err)
	}
	r.config.Store(&cfg)
	go r.worker()
	return r
}

func (r *Recorder) enqueue(rec Record) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed {
		r.dropped.Add(1)
		return
	}
	select {
	case r.queue <- rec:
	default:
		r.dropped.Add(1)
	}
}

func (r *Recorder) Close() {
	r.mu.Lock()
	if !r.closed {
		r.closed = true
		close(r.queue)
	}
	r.mu.Unlock()
	select {
	case <-r.done:
	case <-time.After(8 * time.Second):
		slog.Warn("conversation logs: shutdown drain timed out")
	}
}

func (r *Recorder) worker() {
	defer close(r.done)
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case rec, ok := <-r.queue:
			if !ok {
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			err := r.insert(ctx, rec)
			cancel()
			if err != nil {
				r.failed.Add(1)
				slog.Error("conversation logs: write failed", "request_id", rec.RequestID, "error", err)
			} else {
				r.saved.Add(1)
			}
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_, err := r.db.ExecContext(ctx, `DELETE FROM conversation_logs WHERE id IN
				(SELECT id FROM conversation_logs WHERE created_at < $1 ORDER BY id LIMIT 1000)`,
				time.Now().Add(-time.Duration(r.config.Load().RetentionDays)*24*time.Hour))
			cancel()
			if err != nil {
				slog.Warn("conversation logs: retention cleanup failed", "error", err)
			}
		}
	}
}

func (r *Recorder) insert(ctx context.Context, v Record) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO conversation_logs
		(request_id,created_at,user_id,api_key_id,user_label,key_name,endpoint,model,stream,status_code,outcome,duration_ms,
		prompt,answer,request_body,response_body,tool_calls,request_truncated,response_truncated,request_bytes,response_bytes,redacted)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)`,
		v.RequestID, v.CreatedAt, v.UserID, v.APIKeyID, v.UserLabel, v.KeyName, v.Endpoint, v.Model, v.Stream,
		v.StatusCode, v.Outcome, v.DurationMS, v.Prompt, v.Answer, v.RequestBody, v.ResponseBody, string(v.ToolCalls),
		v.RequestTruncated, v.ResponseTruncated, v.RequestBytes, v.ResponseBytes, v.Redacted)
	return err
}

const columns = `id,request_id,created_at,user_id,api_key_id,user_label,key_name,endpoint,model,stream,status_code,outcome,duration_ms,
	prompt,answer,request_body,response_body,tool_calls,request_truncated,response_truncated,request_bytes,response_bytes,redacted`

func scanRecord(row interface{ Scan(...any) error }) (Record, error) {
	var v Record
	err := row.Scan(&v.ID, &v.RequestID, &v.CreatedAt, &v.UserID, &v.APIKeyID, &v.UserLabel, &v.KeyName, &v.Endpoint, &v.Model,
		&v.Stream, &v.StatusCode, &v.Outcome, &v.DurationMS, &v.Prompt, &v.Answer, &v.RequestBody, &v.ResponseBody, &v.ToolCalls,
		&v.RequestTruncated, &v.ResponseTruncated, &v.RequestBytes, &v.ResponseBytes, &v.Redacted)
	return v, err
}

type Filters struct {
	UserID                 int64
	Model, Outcome, Search string
	From, Until            *time.Time
}

func where(f Filters) (string, []any) {
	parts, args := []string{"TRUE"}, []any{}
	add := func(expr string, v any) { args = append(args, v); parts = append(parts, fmt.Sprintf(expr, len(args))) }
	if f.UserID > 0 {
		add("user_id=$%d", f.UserID)
	}
	if f.Model != "" {
		add("model=$%d", f.Model)
	}
	if f.Outcome != "" {
		add("outcome=$%d", f.Outcome)
	}
	if f.Search != "" {
		add("(request_id=$%[1]d OR prompt ILIKE '%%'||$%[1]d||'%%' OR answer ILIKE '%%'||$%[1]d||'%%')", f.Search)
	}
	if f.From != nil {
		add("created_at >= $%d", *f.From)
	}
	if f.Until != nil {
		add("created_at < $%d", *f.Until)
	}
	return strings.Join(parts, " AND "), args
}
