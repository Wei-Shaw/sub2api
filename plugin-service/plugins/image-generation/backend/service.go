package backend

import (
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/plugin-service/internal/host/httpx"
	"github.com/Wei-Shaw/sub2api/plugin-service/internal/model"
	"github.com/Wei-Shaw/sub2api/plugin-service/internal/repository"
	"github.com/Wei-Shaw/sub2api/plugin-service/internal/service"
)

func parseHistoryQuery(values url.Values) model.HistoryQuery {
	return model.HistoryQuery{
		Page:     parsePositiveInt(values.Get("page"), 1),
		PageSize: parsePositiveInt(values.Get("page_size"), 20),
	}
}

func parseCursorQuery(values url.Values, key string) (model.CursorQuery, error) {
	query := model.CursorQuery{Limit: parsePositiveInt(values.Get("limit"), 20)}
	if query.Limit > 100 {
		query.Limit = 100
	}
	raw := strings.TrimSpace(values.Get(key))
	if raw == "" {
		return query, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return query, errors.New("invalid cursor")
	}
	parts := strings.SplitN(string(decoded), "|", 2)
	if len(parts) != 2 {
		return query, errors.New("invalid cursor")
	}
	query.BeforeTime, err = time.Parse(time.RFC3339Nano, parts[0])
	if err != nil || parts[1] == "" {
		return query, errors.New("invalid cursor")
	}
	query.BeforeID = parts[1]
	return query, nil
}

func encodeCursor(value time.Time, id string) string {
	if value.IsZero() || id == "" {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString([]byte(value.UTC().Format(time.RFC3339Nano) + "|" + id))
}

func parsePositiveInt(raw string, fallback int) int {
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, repository.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "history record not found")
	case errors.Is(err, service.ErrHistoryForbidden):
		httpx.WriteError(w, http.StatusForbidden, "history record is not accessible")
	default:
		httpx.WriteError(w, http.StatusConflict, err.Error())
	}
}

func sanitizeHistoryRecords(records []model.HistoryRecord) []model.HistoryRecord {
	sanitized := make([]model.HistoryRecord, 0, len(records))
	for _, record := range records {
		current := sanitizeHistoryRecord(&record)
		sanitized = append(sanitized, *current)
	}
	return sanitized
}

func sanitizeHistoryRecord(record *model.HistoryRecord) *model.HistoryRecord {
	if record == nil {
		return nil
	}

	safe := *record
	safe.Request = sanitizeRequestPayload(record.Request)
	return &safe
}

func sanitizeRequestPayload(request map[string]any) map[string]any {
	if request == nil {
		return nil
	}

	safe := make(map[string]any, len(request))
	for key, value := range request {
		if key == "provider_api_key" {
			continue
		}
		safe[key] = value
	}
	return safe
}
