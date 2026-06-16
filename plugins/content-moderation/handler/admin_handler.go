// Package handler exposes the content-moderation admin REST surface. Handlers
// are plain net/http (no gin): the SDK mounts them on its stdlib mux under the
// plugin route prefix. Each handler adapts the HTTP request into a
// service.ModerationService call and writes the project-standard JSON envelope.
package handler

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/plugins/content-moderation/internal/errors"
	"github.com/Wei-Shaw/sub2api/plugins/content-moderation/internal/pagination"
	"github.com/Wei-Shaw/sub2api/plugins/content-moderation/service"
)

// AdminHandler serves the moderation admin endpoints. It depends only on the
// plugin's own ModerationService.
type AdminHandler struct {
	service *service.ModerationService
}

// NewAdminHandler builds the admin handler around the moderation service.
func NewAdminHandler(svc *service.ModerationService) *AdminHandler {
	return &AdminHandler{service: svc}
}

// GetConfig handles GET /config.
func (h *AdminHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.service.GetConfig(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeSuccess(w, cfg)
}

// UpdateConfig handles PUT /config.
func (h *AdminHandler) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	var req service.UpdateContentModerationConfigInput
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, infraerrors.BadRequest("INVALID_REQUEST", "Invalid request: "+err.Error()))
		return
	}
	cfg, err := h.service.UpdateConfig(r.Context(), req)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeSuccess(w, cfg)
}

// TestAPIKeys handles POST /test-api-keys.
func (h *AdminHandler) TestAPIKeys(w http.ResponseWriter, r *http.Request) {
	var req service.TestContentModerationAPIKeysInput
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, infraerrors.BadRequest("INVALID_REQUEST", "Invalid request: "+err.Error()))
		return
	}
	result, err := h.service.TestAPIKeys(r.Context(), req)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeSuccess(w, result)
}

// GetStatus handles GET /status.
func (h *AdminHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	status, err := h.service.GetStatus(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeSuccess(w, status)
}

// ListLogs handles GET /logs.
func (h *AdminHandler) ListLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, pageSize := parsePagination(q)
	filter := service.ContentModerationLogFilter{
		Pagination: pagination.PaginationParams{
			Page:      page,
			PageSize:  pageSize,
			SortOrder: pagination.SortOrderDesc,
		},
		Result:   q.Get("result"),
		Endpoint: q.Get("endpoint"),
		Search:   q.Get("search"),
	}
	if raw := strings.TrimSpace(q.Get("group_id")); raw != "" {
		groupID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || groupID <= 0 {
			writeError(w, r, infraerrors.BadRequest("INVALID_GROUP_ID", "Invalid group_id"))
			return
		}
		filter.GroupID = &groupID
	}
	if raw := strings.TrimSpace(q.Get("from")); raw != "" {
		t, _, err := parseModerationDate(raw)
		if err != nil {
			writeError(w, r, infraerrors.BadRequest("INVALID_FROM", "Invalid from"))
			return
		}
		filter.From = &t
	}
	if raw := strings.TrimSpace(q.Get("to")); raw != "" {
		t, dateOnly, err := parseModerationDate(raw)
		if err != nil {
			writeError(w, r, infraerrors.BadRequest("INVALID_TO", "Invalid to"))
			return
		}
		if dateOnly {
			t = t.Add(24*time.Hour - time.Nanosecond)
		}
		filter.To = &t
	}
	items, pageResult, err := h.service.ListLogs(r.Context(), filter)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writePaginated(w, items, pageResult)
}

// UnbanUser handles POST /unban-user. The target user id is taken from the
// JSON body so the route needs no path parameter.
func (h *AdminHandler) UnbanUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID int64 `json:"user_id"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, infraerrors.BadRequest("INVALID_REQUEST", "Invalid request: "+err.Error()))
		return
	}
	if req.UserID <= 0 {
		writeError(w, r, infraerrors.BadRequest("INVALID_USER_ID", "Invalid user_id"))
		return
	}
	result, err := h.service.UnbanUser(r.Context(), req.UserID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeSuccess(w, result)
}

// DeleteFlaggedHash handles DELETE /flagged-hashes/:hash. The hash is parsed
// from the URL path by the router before dispatch.
func (h *AdminHandler) DeleteFlaggedHash(w http.ResponseWriter, r *http.Request, hash string) {
	hash = strings.TrimSpace(hash)
	if hash == "" {
		writeError(w, r, infraerrors.BadRequest("INVALID_HASH", "Invalid input hash"))
		return
	}
	result, err := h.service.DeleteFlaggedInputHash(r.Context(), hash)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeSuccess(w, result)
}

// ClearFlaggedHashes handles DELETE /flagged-hashes.
func (h *AdminHandler) ClearFlaggedHashes(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.ClearFlaggedInputHashes(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeSuccess(w, result)
}

// parseModerationDate accepts RFC3339 or YYYY-MM-DD. The bool reports a
// date-only value so the caller can extend the upper bound to end-of-day.
func parseModerationDate(raw string) (time.Time, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false, nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, false, nil
	}
	t, err := time.Parse("2006-01-02", raw)
	return t, err == nil, err
}
