package conversationlog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

// RegisterAdmin must be called only on an administrator-authenticated group.
// Content capture is independent of the optional risk-control feature flag.
func (r *Recorder) RegisterAdmin(admin *gin.RouterGroup) {
	group := admin.Group("/conversation-logs")
	group.Use(func(c *gin.Context) { c.Header("Cache-Control", "no-store"); c.Next() })
	group.GET("/config", r.getConfig)
	group.PUT("/config", r.updateConfig)
	group.GET("", r.list)
	group.GET("/:id", r.detail)
	group.DELETE("/:id", r.remove)
}

func (r *Recorder) getConfig(c *gin.Context) {
	response.Success(c, gin.H{"config": r.config.Load(), "dropped_records": r.dropped.Load(), "failed_writes": r.failed.Load(),
		"saved_records": r.saved.Load(), "queue_depth": len(r.queue), "active_captures": len(r.slots),
		"supported_protocols": []string{"chat_completions_http", "responses_http", "anthropic_messages_http"}, "websocket_capture": false})
}

func (r *Recorder) updateConfig(c *gin.Context) {
	// Older clients omit redact_sensitive; preserve its current value.
	cfg := *r.config.Load()
	if err := c.ShouldBindJSON(&cfg); err != nil {
		response.Error(c, 400, "Invalid JSON config")
		return
	}
	if err := cfg.validate(); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	if _, err := r.db.ExecContext(ctx, "UPDATE conversation_log_config SET enabled=$1,retention_days=$2,max_body_bytes=$3,redact_sensitive=$4 WHERE id=1", cfg.Enabled, cfg.RetentionDays, cfg.MaxBodyBytes, cfg.RedactSensitive); err != nil {
		response.Error(c, 500, "Unable to save conversation log config")
		return
	}
	r.config.Store(&cfg)
	response.Success(c, cfg)
}

func parseFilters(c *gin.Context) (Filters, error) {
	f := Filters{Model: strings.TrimSpace(c.Query("model")), Outcome: c.Query("outcome"), Search: strings.TrimSpace(c.Query("q"))}
	if len(f.Model) > 256 || len(f.Search) > 256 {
		return f, fmt.Errorf("model and search must be at most 256 bytes")
	}
	if s := c.Query("user_id"); s != "" {
		id, err := strconv.ParseInt(s, 10, 64)
		if err != nil || id <= 0 {
			return f, fmt.Errorf("invalid user_id")
		}
		f.UserID = id
	}
	if f.Outcome != "" {
		switch f.Outcome {
		case "completed", "failed", "interrupted", "incomplete", "capture_truncated":
		default:
			return f, fmt.Errorf("invalid outcome")
		}
	}
	for _, item := range []struct {
		key string
		dst **time.Time
	}{{"from", &f.From}, {"until", &f.Until}} {
		if s := c.Query(item.key); s != "" {
			v, err := time.Parse(time.RFC3339, s)
			if err != nil {
				return f, fmt.Errorf("%s must be RFC3339", item.key)
			}
			*item.dst = &v
		}
	}
	return f, nil
}

func pageNumber(c *gin.Context, key string, defaultValue, max int) int {
	n, err := strconv.Atoi(c.Query(key))
	if err != nil || n < 1 {
		return defaultValue
	}
	if n > max {
		return max
	}
	return n
}

func (r *Recorder) list(c *gin.Context) {
	f, err := parseFilters(c)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	page, size := pageNumber(c, "page", 1, 1000000), pageNumber(c, "page_size", 20, 100)
	condition, args := where(f)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	var total int64
	if err = r.db.QueryRowContext(ctx, "SELECT count(*) FROM conversation_logs WHERE "+condition, args...).Scan(&total); err != nil {
		response.Error(c, 500, "Unable to list conversation logs")
		return
	}
	// Return bounded binary previews: SQL_ASCII databases count bytes in left(),
	// which can split a UTF-8 character and fail PostgreSQL's wire encoding check.
	// bytea travels safely even when its final code point is partial. Trim it in Go.
	listColumns := strings.NewReplacer("request_body", "''::text", "response_body", "''::text", "tool_calls", "'[]'::jsonb",
		"prompt,answer", "substring(convert_to(prompt,'UTF8') from 1 for 960),substring(convert_to(answer,'UTF8') from 1 for 960)").Replace(columns)
	query := "SELECT " + listColumns + " FROM conversation_logs WHERE " + condition + fmt.Sprintf(" ORDER BY created_at DESC,id DESC LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	args = append(args, size, (page-1)*size)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		response.Error(c, 500, "Unable to list conversation logs")
		return
	}
	defer rows.Close()
	items := []Record{}
	for rows.Next() {
		v, err := scanRecord(rows)
		if err != nil {
			response.Error(c, 500, "Unable to read conversation logs")
			return
		}
		v.Prompt = previewText(v.Prompt)
		v.Answer = previewText(v.Answer)
		items = append(items, v)
	}
	if rows.Err() != nil {
		response.Error(c, 500, "Unable to read conversation logs")
		return
	}
	response.Success(c, gin.H{"items": items, "total": total, "page": page, "page_size": size})
}

func previewText(value string) string {
	value = strings.ToValidUTF8(value, "")
	count := 0
	for offset := range value {
		if count == 240 {
			return value[:offset]
		}
		count++
	}
	return value
}

func recordID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.Error(c, 400, "Invalid record ID")
		return 0, false
	}
	return id, true
}

func (r *Recorder) detail(c *gin.Context) {
	id, ok := recordID(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	v, err := scanRecord(r.db.QueryRowContext(ctx, "SELECT "+columns+" FROM conversation_logs WHERE id=$1", id))
	if errors.Is(err, sql.ErrNoRows) {
		response.Error(c, http.StatusNotFound, "Conversation log not found")
		return
	}
	if err != nil {
		response.Error(c, 500, "Unable to read conversation log")
		return
	}
	response.Success(c, v)
}

func (r *Recorder) remove(c *gin.Context) {
	id, ok := recordID(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	result, err := r.db.ExecContext(ctx, "DELETE FROM conversation_logs WHERE id=$1", id)
	if err != nil {
		response.Error(c, 500, "Unable to delete conversation log")
		return
	}
	n, err := result.RowsAffected()
	if err != nil {
		response.Error(c, 500, "Unable to confirm deletion")
		return
	}
	if n == 0 {
		response.Error(c, 404, "Conversation log not found")
		return
	}
	response.Success(c, gin.H{"deleted": id})
}
