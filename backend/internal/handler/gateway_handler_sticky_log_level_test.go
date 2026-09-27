//go:build unit

package handler

import (
	"bytes"
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Sticky-session diagnostics must stay at debug: the default log.level=info
// would otherwise emit several entries per /v1/messages request.
func TestGatewayHandlerMessages_StickyDiagnosticsLogAtDebug(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logSink, restore := captureHandlerStructuredLog(t)
	defer restore()

	groupID := int64(2101)
	accountID := int64(1101)
	group := &service.Group{ID: groupID, Hydrated: true, Platform: service.PlatformAnthropic, Status: service.StatusActive}
	account := &service.Account{
		ID:       accountID,
		Name:     "ag-sticky",
		Platform: service.PlatformAntigravity,
		Type:     service.AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":              "tok_xxx",
			"intercept_warmup_requests": true,
		},
		Extra:         map[string]any{"mixed_scheduling": true},
		Concurrency:   1,
		Priority:      1,
		Status:        service.StatusActive,
		Schedulable:   true,
		AccountGroups: []service.AccountGroup{{AccountID: accountID, GroupID: groupID}},
	}
	h, cleanup := newTestGatewayHandler(t, group, []*service.Account{account})
	defer cleanup()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":256,"metadata":{"user_id":"not-a-session-id"},"messages":[{"role":"user","content":[{"type":"text","text":"Warmup"}]}]}`)
	req := httptest.NewRequest("POST", "/v1/messages", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req.WithContext(context.WithValue(req.Context(), ctxkey.Group, group))
	apiKey := &service.APIKey{
		ID:      3101,
		UserID:  4101,
		GroupID: &groupID,
		Status:  service.StatusActive,
		User:    &service.User{ID: 4101, Concurrency: 10, Balance: 100},
		Group:   group,
	}
	c.Set(string(middleware.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: apiKey.UserID, Concurrency: 10})

	h.Messages(c)
	require.Equal(t, 200, rec.Code)

	logSink.mu.Lock()
	defer logSink.mu.Unlock()
	var infoSticky []string
	debugSticky := 0
	for _, ev := range logSink.events {
		if !strings.HasPrefix(ev.Message, "sticky.") {
			continue
		}
		switch ev.Level {
		case "debug":
			debugSticky++
		default:
			infoSticky = append(infoSticky, ev.Level+" "+ev.Message)
		}
	}
	require.Empty(t, infoSticky, "sticky diagnostics above debug level")
	require.NotZero(t, debugSticky, "sticky diagnostics should still be available at debug level")
}
