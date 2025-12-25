//go:build unit

package response

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/infrastructure/errors"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestErrorWithDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		statusCode int
		message    string
		reason     string
		metadata   map[string]string
		want       Response
	}{
		{
			name:       "plain_error",
			statusCode: http.StatusBadRequest,
			message:    "invalid request",
			want: Response{
				Code:    http.StatusBadRequest,
				Message: "invalid request",
			},
		},
		{
			name:       "structured_error",
			statusCode: http.StatusForbidden,
			message:    "no access",
			reason:     "FORBIDDEN",
			metadata:   map[string]string{"k": "v"},
			want: Response{
				Code:     http.StatusForbidden,
				Message:  "no access",
				Reason:   "FORBIDDEN",
				Metadata: map[string]string{"k": "v"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			ErrorWithDetails(c, tt.statusCode, tt.message, tt.reason, tt.metadata)

			require.Equal(t, tt.statusCode, w.Code)

			var got Response
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
			require.Equal(t, tt.want, got)
		})
	}
}

func TestErrorFrom(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name         string
		err          error
		wantWritten  bool
		wantHTTPCode int
		wantBody     Response
	}{
		{
			name:        "nil_error",
			err:         nil,
			wantWritten: false,
		},
		{
			name:         "application_error",
			err:          infraerrors.Forbidden("FORBIDDEN", "no access").WithMetadata(map[string]string{"scope": "admin"}),
			wantWritten:  true,
			wantHTTPCode: http.StatusForbidden,
			wantBody: Response{
				Code:     http.StatusForbidden,
				Message:  "no access",
				Reason:   "FORBIDDEN",
				Metadata: map[string]string{"scope": "admin"},
			},
		},
		{
			name:         "bad_request_error",
			err:          infraerrors.BadRequest("INVALID_REQUEST", "invalid request"),
			wantWritten:  true,
			wantHTTPCode: http.StatusBadRequest,
			wantBody: Response{
				Code:    http.StatusBadRequest,
				Message: "invalid request",
				Reason:  "INVALID_REQUEST",
			},
		},
		{
			name:         "unauthorized_error",
			err:          infraerrors.Unauthorized("UNAUTHORIZED", "unauthorized"),
			wantWritten:  true,
			wantHTTPCode: http.StatusUnauthorized,
			wantBody: Response{
				Code:    http.StatusUnauthorized,
				Message: "unauthorized",
				Reason:  "UNAUTHORIZED",
			},
		},
		{
			name:         "not_found_error",
			err:          infraerrors.NotFound("NOT_FOUND", "not found"),
			wantWritten:  true,
			wantHTTPCode: http.StatusNotFound,
			wantBody: Response{
				Code:    http.StatusNotFound,
				Message: "not found",
				Reason:  "NOT_FOUND",
			},
		},
		{
			name:         "conflict_error",
			err:          infraerrors.Conflict("CONFLICT", "conflict"),
			wantWritten:  true,
			wantHTTPCode: http.StatusConflict,
			wantBody: Response{
				Code:    http.StatusConflict,
				Message: "conflict",
				Reason:  "CONFLICT",
			},
		},
		{
			name:         "unknown_error_defaults_to_500",
			err:          errors.New("boom"),
			wantWritten:  true,
			wantHTTPCode: http.StatusInternalServerError,
			wantBody: Response{
				Code:    http.StatusInternalServerError,
				Message: infraerrors.UnknownMessage,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			written := ErrorFrom(c, tt.err)
			require.Equal(t, tt.wantWritten, written)

			if !tt.wantWritten {
				require.Equal(t, 200, w.Code)
				require.Empty(t, w.Body.String())
				return
			}

			require.Equal(t, tt.wantHTTPCode, w.Code)
			var got Response
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
			require.Equal(t, tt.wantBody, got)
		})
	}
}
