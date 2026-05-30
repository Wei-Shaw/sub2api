package handler

import (
	"encoding/json"
	"io"
	"log/slog"
	"math"
	"net/http"

	infraerrors "github.com/Wei-Shaw/sub2api/plugins/content-moderation/internal/errors"
	"github.com/Wei-Shaw/sub2api/plugins/content-moderation/internal/pagination"
)

// maxRequestBodyBytes bounds admin request bodies so a malformed or hostile
// payload cannot exhaust memory (CLAUDE.md: HTTP bodies must use a limit).
const maxRequestBodyBytes = 1 << 20 // 1 MiB

// envelope is the project-standard JSON response shape. It mirrors the core's
// internal/pkg/response.Response so the admin frontend sees an identical
// contract whether a route is served by the core or this plugin.
type envelope struct {
	Code     int               `json:"code"`
	Message  string            `json:"message"`
	Reason   string            `json:"reason,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
	Data     any               `json:"data,omitempty"`
}

// paginatedData matches the frontend's expected list payload.
type paginatedData struct {
	Items    any   `json:"items"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
	Pages    int   `json:"pages"`
}

// writeSuccess writes a 200 envelope carrying data.
func writeSuccess(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, envelope{Code: 0, Message: "success", Data: data})
}

// writePaginated writes a 200 envelope wrapping a page of items. A nil page
// result degrades to an empty first page rather than panicking.
func writePaginated(w http.ResponseWriter, items any, page *pagination.PaginationResult) {
	if page == nil {
		writeSuccess(w, paginatedData{Items: items, Total: 0, Page: 1, PageSize: 20, Pages: 1})
		return
	}
	pages := page.Pages
	if pages < 1 {
		pages = 1
	}
	writeSuccess(w, paginatedData{
		Items:    items,
		Total:    page.Total,
		Page:     page.Page,
		PageSize: page.PageSize,
		Pages:    pages,
	})
}

// writeError maps any error onto the structured envelope using the shared
// classifier. 5xx causes are logged with the request path; the response body
// only ever exposes the sanitized message/reason/metadata.
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	statusCode, st := infraerrors.ToHTTP(err)
	if statusCode >= http.StatusInternalServerError && r != nil {
		slog.Error("content-moderation admin handler error",
			"method", r.Method,
			"path", r.URL.Path,
			"error", err.Error(),
		)
	}
	writeJSON(w, statusCode, envelope{
		Code:     int(st.Code),
		Message:  st.Message,
		Reason:   st.Reason,
		Metadata: st.Metadata,
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// decodeJSON reads a bounded request body into dst. An empty body decodes to
// the zero value rather than an error so endpoints with all-optional fields
// (e.g. PUT /config) accept an empty PUT.
func decodeJSON(r *http.Request, dst any) error {
	if r.Body == nil {
		return nil
	}
	limited := io.LimitReader(r.Body, maxRequestBodyBytes)
	dec := json.NewDecoder(limited)
	if err := dec.Decode(dst); err != nil {
		if err == io.EOF {
			return nil
		}
		return err
	}
	return nil
}

// parsePagination reads page / page_size (or limit) query params, clamping to
// the same bounds the core uses.
func parsePagination(q interface{ Get(string) string }) (page, pageSize int) {
	page = 1
	pageSize = 20
	if v := parsePositiveInt(q.Get("page")); v > 0 {
		page = v
	}
	if v := parsePositiveInt(q.Get("page_size")); v > 0 && v <= 1000 {
		pageSize = v
	} else if v := parsePositiveInt(q.Get("limit")); v > 0 && v <= 1000 {
		pageSize = v
	}
	return page, pageSize
}

func parsePositiveInt(s string) int {
	if s == "" {
		return 0
	}
	var result int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		result = result*10 + int(c-'0')
		if result > math.MaxInt32 {
			return math.MaxInt32
		}
	}
	return result
}
