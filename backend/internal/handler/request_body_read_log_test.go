//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRequestBodyReadFailureLogsWithoutPayload(t *testing.T) {
	log, logs := newObservedLogger(t)
	req, err := http.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader("secret-payload-marker"))
	require.NoError(t, err)
	req.Header.Set("Content-Encoding", "gzip")
	req.ContentLength = 21

	status, message := requestBodyReadFailure(log, req, errors.New(`decode Content-Encoding "gzip": unexpected EOF secret-payload-marker`))

	require.Equal(t, http.StatusBadRequest, status)
	require.Equal(t, "Failed to read request body", message)
	entries := logs.All()
	require.Len(t, entries, 1)
	require.Equal(t, "read request body failed", entries[0].Message)
	fields := entries[0].ContextMap()
	require.Equal(t, "decode_content_encoding", fields["error_kind"])
	require.Equal(t, "gzip", fields["content_encoding"])
	require.EqualValues(t, 21, fields["content_length"])
	require.NotContains(t, entries[0].Message+fmt.Sprint(fields), "secret-payload-marker")
}

func TestRequestBodyReadFailureKeepsBodyLimitMessageWithoutLogging(t *testing.T) {
	log, logs := newObservedLogger(t)
	status, message := requestBodyReadFailure(log, nil, &http.MaxBytesError{Limit: 2 << 20})
	require.Equal(t, http.StatusRequestEntityTooLarge, status)
	require.Equal(t, buildBodyTooLargeMessage(2<<20), message)
	require.Empty(t, logs.All())
}

// The Anthropic /v1/messages handler reads the full body before it knows the
// model or account. A failed read must report why instead of a generic 400.
func TestGatewayMessagesClassifiesRequestBodyReadFailures(t *testing.T) {
	gin.SetMode(gin.TestMode)
	deadlineErr := &net.OpError{Op: "read", Net: "tcp", Err: os.ErrDeadlineExceeded}

	tests := []struct {
		name        string
		readErr     error
		cancelCtx   bool
		wantStatus  int
		wantMessage string
	}{
		{"idle timeout", deadlineErr, false, http.StatusRequestTimeout, "Timed out reading request body"},
		{"client disconnect", io.ErrUnexpectedEOF, true, 499, "Client closed the connection before the request body was received"},
		{"truncated body", io.ErrUnexpectedEOF, false, http.StatusBadRequest, "Failed to read request body"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			body := io.MultiReader(strings.NewReader(`{"model":"claude-sonnet-4-5","messages":[`), iotest.ErrReader(tc.readErr))
			req := httptest.NewRequest(http.MethodPost, "/v1/messages", body)
			req.ContentLength = 1 << 20
			if tc.cancelCtx {
				ctx, cancel := context.WithCancel(req.Context())
				cancel()
				req = req.WithContext(ctx)
			}
			c.Request = req
			groupID := int64(1)
			c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{ID: 1, UserID: 1, GroupID: &groupID})
			c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1, Concurrency: 1})

			(&GatewayHandler{}).Messages(c)

			require.Equal(t, tc.wantStatus, rec.Code)
			var resp struct {
				Type  string `json:"type"`
				Error struct {
					Type    string `json:"type"`
					Message string `json:"message"`
				} `json:"error"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			require.Equal(t, "error", resp.Type)
			require.Equal(t, "invalid_request_error", resp.Error.Type)
			require.Equal(t, tc.wantMessage, resp.Error.Message)
		})
	}
}

// Google-format errors carry no type field; body read failures must still be
// attributed to the client request rather than the platform.
func TestGeminiRequestBodyReadTimeoutClassifiedAsClient(t *testing.T) {
	entry := recordGoogleFormatOpsError(t, func(c *gin.Context) {
		c.Request.Body = io.NopCloser(iotest.ErrReader(&net.OpError{Op: "read", Net: "tcp", Err: os.ErrDeadlineExceeded}))
		readGeminiCachedContentBody(c)
	})
	require.Equal(t, http.StatusRequestTimeout, entry.StatusCode)
	require.Equal(t, "invalid_request_error", entry.ErrorType)
	require.Equal(t, "request", entry.ErrorPhase)
	require.Equal(t, "client", entry.ErrorOwner)
	require.Equal(t, "P3", entry.Severity)
}
