package pluginsdk

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
)

// Header names mirrored from backend/internal/plugin/router_middleware.go.
// They are documented in plugin-sdk/README.md "Request Headers Injected by
// Host"; plugins should not parse them by hand—use RequestMetadata instead.
const (
	HeaderPluginUserID    = "X-Plugin-User-ID"
	HeaderPluginUserRole  = "X-Plugin-User-Role"
	HeaderPluginName      = "X-Plugin-Name"
	HeaderPluginRequestID = "X-Plugin-Request-ID"
	HeaderPluginAPIKeyID  = "X-Plugin-API-Key-ID"
	HeaderPluginClientIP  = "X-Plugin-Client-IP"
	HeaderTraceparent     = "traceparent"
)

// RequestMeta is the parsed view of the X-Plugin-* / traceparent headers the
// host injects into every proxied request. Zero values mean "absent or
// unparseable"; callers can rely on UserID==0 to denote anonymous.
type RequestMeta struct {
	// TraceID is the 32 hex char trace-id from a valid W3C traceparent;
	// empty if the header was missing or malformed.
	TraceID string
	// SpanID is the 16 hex char parent-id; empty when TraceID is empty.
	SpanID string
	// RequestID is the host-generated UUID propagated to plugins; empty if
	// the host did not set the header (older host versions).
	RequestID string
	// PluginName as declared in manifest.
	PluginName string
	// UserID is the authenticated user id; 0 means anonymous (no JWT/APIKey).
	UserID int64
	// UserRole is the authenticated user role string; "" when anonymous.
	UserRole string
	// APIKeyID is the api_keys.id for APIKey-authenticated requests; 0
	// otherwise. Plugins MUST NOT receive the raw key string.
	APIKeyID int64
	// ClientIP is the gin.Context.ClientIP() resolution from the host.
	ClientIP string
}

// RequestMetadata extracts the X-Plugin-* / traceparent headers from r. It
// never panics on missing or malformed values; callers can always rely on the
// zero value of RequestMeta.
func RequestMetadata(r *http.Request) RequestMeta {
	if r == nil || r.Header == nil {
		return RequestMeta{}
	}
	m := RequestMeta{
		RequestID:  r.Header.Get(HeaderPluginRequestID),
		PluginName: r.Header.Get(HeaderPluginName),
		UserRole:   r.Header.Get(HeaderPluginUserRole),
		ClientIP:   r.Header.Get(HeaderPluginClientIP),
	}
	if v := r.Header.Get(HeaderPluginUserID); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			m.UserID = id
		}
	}
	if v := r.Header.Get(HeaderPluginAPIKeyID); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			m.APIKeyID = id
		}
	}
	if tp := r.Header.Get(HeaderTraceparent); tp != "" {
		m.TraceID, m.SpanID = parseTraceparent(tp)
	}
	return m
}

// LoggerWithRequest returns a logger that automatically tags every record
// with trace_id / request_id / user_id / api_key_id, matching the host zap
// convention. Empty fields are omitted so anonymous requests don't pollute
// the log surface.
//
// Plugins typically derive a per-request logger as the first thing in their
// handler:
//
//	logger := pluginsdk.LoggerWithRequest(slog.Default(), pluginsdk.RequestMetadata(r))
//	logger.Info("processing request")
func LoggerWithRequest(base *slog.Logger, meta RequestMeta) *slog.Logger {
	if base == nil {
		base = slog.Default()
	}
	attrs := make([]any, 0, 8)
	if meta.TraceID != "" {
		attrs = append(attrs, "trace_id", meta.TraceID)
	}
	if meta.RequestID != "" {
		attrs = append(attrs, "request_id", meta.RequestID)
	}
	if meta.UserID != 0 {
		attrs = append(attrs, "user_id", meta.UserID)
	}
	if meta.APIKeyID != 0 {
		attrs = append(attrs, "api_key_id", meta.APIKeyID)
	}
	if len(attrs) == 0 {
		return base
	}
	return base.With(attrs...)
}

// parseTraceparent extracts trace-id and parent-id from a W3C traceparent
// header. Returns ("", "") if tp is malformed; the host already validates
// before injection so this is mostly defensive against future format
// extensions.
func parseTraceparent(tp string) (traceID, spanID string) {
	parts := strings.Split(tp, "-")
	if len(parts) != 4 {
		return "", ""
	}
	if parts[0] != "00" {
		return "", ""
	}
	if len(parts[1]) != 32 || len(parts[2]) != 16 {
		return "", ""
	}
	return parts[1], parts[2]
}
