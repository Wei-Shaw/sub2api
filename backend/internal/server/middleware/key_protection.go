package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/keyprotection"
	"github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type KeyProtectionSettings interface {
	GetKeyProtectionConfig(context.Context) (keyprotection.Config, error)
}

// KeyProtection runs immediately after authentication and before any body
// reader, audit, routing, retry or asynchronous submission. Its writer sits
// after provider conversion and never supplies restored data back to handlers.
func KeyProtection(settings KeyProtectionSettings, store *keyprotection.RedisStore, maxBody int64) gin.HandlerFunc {
	if maxBody <= 0 {
		maxBody = 16 << 20
	}
	return func(c *gin.Context) {
		if settings == nil {
			c.Next()
			return
		}
		cfg, err := settings.GetKeyProtectionConfig(c.Request.Context())
		if err != nil {
			protectionFailure(c, http.StatusServiceUnavailable, "configuration_unavailable")
			return
		}
		cfg = cfg.Normalized()
		if !cfg.Enabled {
			c.Next()
			return
		}
		key, ok := GetAPIKeyFromContext(c)
		if !ok || key == nil || key.ID <= 0 || key.UserID <= 0 {
			protectionFailure(c, http.StatusUnauthorized, "authenticated_identity_required")
			return
		}
		var groupID int64
		if key.GroupID != nil {
			groupID = *key.GroupID
		}
		if !cfg.Applies(key.UserID, groupID) {
			c.Next()
			return
		}
		c.Set(keyprotection.ProtectedGinKey, true)
		c.Request = c.Request.WithContext(keyprotection.WithProtected(c.Request.Context()))
		protocol, readOnly := protectedProtocol(c)
		if readOnly {
			c.Next()
			return
		}
		if protocol == "" {
			protectionFailure(c, http.StatusBadRequest, "unsupported_transport_use_chat_completions_responses_or_messages_http")
			return
		}
		for _, header := range []string{"Signature", "Signature-Input", "Content-Signature", "X-Signature", "X-Amz-Content-Sha256"} {
			if c.GetHeader(header) != "" {
				protectionFailure(c, http.StatusBadRequest, "signed_body_not_supported")
				return
			}
		}
		contentType := strings.ToLower(c.GetHeader("Content-Type"))
		if contentType != "" && !strings.HasPrefix(contentType, "application/json") {
			protectionFailure(c, http.StatusBadRequest, "json_body_required")
			return
		}
		start := time.Now()
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBody)
		body, err := httputil.ReadRequestBodyWithPrealloc(c.Request)
		if err != nil || int64(len(body)) > maxBody {
			status := http.StatusBadRequest
			var sizeErr *http.MaxBytesError
			if errors.As(err, &sizeErr) || int64(len(body)) > maxBody {
				status = http.StatusRequestEntityTooLarge
			}
			protectionFailure(c, status, "invalid_or_oversized_body")
			return
		}
		// Parse continuation independently; scanner validates full structure and
		// duplicates before any downstream body becomes available.
		var envelope map[string]json.RawMessage
		if json.Unmarshal(body, &envelope) != nil || envelope == nil {
			protectionFailure(c, http.StatusBadRequest, "invalid_json")
			return
		}
		previous := ""
		if v, exists := envelope["previous_response_id"]; exists && string(v) != "null" {
			if json.Unmarshal(v, &previous) != nil || len(previous) > 512 || (previous != "" && protocol != "responses") {
				protectionFailure(c, http.StatusBadRequest, "invalid_continuation")
				return
			}
		}
		session := protectionSession(c, envelope)
		if len(session) > 512 {
			protectionFailure(c, http.StatusBadRequest, "invalid_session")
			return
		}
		scope := keyprotection.Scope{UserID: key.UserID, APIKeyID: key.ID, GroupID: groupID}
		var protected []byte
		var state *keyprotection.State
		persistent := cfg.Mode == "reversible" && (session != "" || previous != "" || protocol == "responses")
		if persistent {
			protected, state, err = store.Protect(c.Request.Context(), scope, session, previous, cfg, body, protocol)
		} else {
			if previous != "" {
				protectionFailure(c, http.StatusBadRequest, "continuation_requires_reversible_mode")
				return
			}
			if cfg.Mode == keyprotection.ModeReversible && store != nil {
				var seed []byte
				seed, err = store.Seed(scope, session)
				if err == nil {
					state, err = keyprotection.NewStateWithSeed(cfg, nil, seed)
				}
			} else {
				state, err = keyprotection.NewState(cfg, nil)
			}
			if err == nil {
				protected, err = state.ProtectJSON(body, protocol)
			}
		}
		if err != nil {
			reason := "input_protection_failed"
			status := http.StatusBadRequest
			if errors.Is(err, keyprotection.ErrExpired) {
				reason = "continuation_mapping_missing_or_expired_resend_full_history"
			}
			if errors.Is(err, keyprotection.ErrStore) {
				reason = "mapping_storage_unavailable"
				status = http.StatusServiceUnavailable
			}
			if errors.Is(err, keyprotection.ErrCapacity) {
				reason = "mapping_capacity_exceeded"
			}
			protectionFailure(c, status, reason)
			return
		}
		// Replace all reread mechanisms, including Gin binding and HTTP GetBody.
		// There is no retained fallback copy for retry or protocol conversion.
		c.Request.Body = httputil.NewPrereadBody(protected)
		c.Request.GetBody = func() (io.ReadCloser, error) { return httputil.NewPrereadBody(protected), nil }
		c.Request.ContentLength = int64(len(protected))
		c.Request.Header.Del("Content-Length")
		c.Request.Header.Del("Content-Encoding")
		c.Request.Header.Set("Accept-Encoding", "identity")
		c.Set(gin.BodyBytesKey, protected)
		// Downstream/reverse-proxy result caches must not store restored bodies.
		c.Header("Cache-Control", "no-store, private")
		logger.FromContext(c.Request.Context()).Info("key protection input scanned", zap.String("mode", cfg.Mode), zap.Any("rule_counts", state.Counts()), zap.Int64("transform_us", time.Since(start).Microseconds()))
		body = nil
		original := c.Writer
		var saveID func(string) error
		if cfg.Mode == "reversible" && protocol == "responses" {
			saveID = func(id string) error { return store.SaveResponse(c.Request.Context(), scope, id, state) }
		}
		writer := keyprotection.NewResponseWriter(original, state, protocol, saveID)
		c.Writer = writer
		defer func() { c.Writer = original }()
		c.Next()
		if c.Request.Context().Err() != nil {
			writer.Abort()
			return
		}
		if err := writer.Finish(); err != nil {
			logger.FromContext(c.Request.Context()).Warn("key protection output failed", zap.String("reason", "invalid_response_or_mapping_storage"))
		}
	}
}

func protectionFailure(c *gin.Context, status int, reason string) {
	logger.FromContext(c.Request.Context()).Warn("key protection request rejected", zap.String("reason", reason))
	message := "Automatic key protection failed: " + reason + ". No unprotected request was forwarded."
	if strings.Contains(c.FullPath(), "/messages") {
		c.AbortWithStatusJSON(status, gin.H{"type": "error", "error": gin.H{"type": "key_protection_error", "message": message}})
	} else {
		c.AbortWithStatusJSON(status, gin.H{"error": gin.H{"type": "key_protection_error", "code": reason, "message": message}})
	}
}

func protectedProtocol(c *gin.Context) (string, bool) {
	path := c.Request.URL.Path
	if c.Request.Method == http.MethodGet && c.GetHeader("Upgrade") == "" {
		if strings.HasSuffix(path, "/models") || strings.Contains(c.FullPath(), "/models/:model") || strings.HasSuffix(path, "/usage") || strings.HasSuffix(path, "/sub2api/billing") {
			return "", true
		}
	}
	if c.Request.Method != http.MethodPost || c.GetHeader("Upgrade") != "" {
		return "", false
	}
	switch path {
	case "/v1/chat/completions", "/chat/completions":
		return "chat", false
	case "/v1/responses", "/responses", "/backend-api/codex/responses":
		return "responses", false
	case "/v1/messages", "/antigravity/v1/messages":
		return "messages", false
	}
	return "", false
}

var protectionClaudeSession = regexp.MustCompile(`_session_([a-fA-F0-9-]{16,64})$`)

func protectionSession(c *gin.Context, root map[string]json.RawMessage) string {
	for _, header := range []string{"X-Sub2API-Session-ID", "X-Claude-Code-Session-ID", "session-id", "session_id", "conversation_id", "X-Session-Affinity", "X-Session-Id", "X-OpenCode-Session", "X-Conversation-ID"} {
		if value := strings.TrimSpace(c.GetHeader(header)); value != "" {
			return value
		}
	}
	var metadata map[string]json.RawMessage
	_ = json.Unmarshal(root["metadata"], &metadata)
	var userID string
	_ = json.Unmarshal(metadata["user_id"], &userID)
	if match := protectionClaudeSession.FindStringSubmatch(userID); len(match) == 2 {
		return match[1]
	}
	var embedded struct {
		SessionID string `json:"session_id"`
	}
	_ = json.Unmarshal([]byte(userID), &embedded)
	return embedded.SessionID
}
