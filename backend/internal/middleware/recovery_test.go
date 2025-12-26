//go:build unit

package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/infrastructure/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRecovery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name         string
		handler      gin.HandlerFunc
		wantHTTPCode int
		wantBody     response.Response
REDACTED{
		{
			name: "panic_returns_standard_json_500",
			handler: func(c *gin.Context) {
				panic("boom")
		REDACTED,
			wantHTTPCode: http.StatusInternalServerError,
			wantBody: response.Response{
				Code:    http.StatusInternalServerError,
				Message: infraerrors.UnknownMessage,
		REDACTED,
	REDACTED,
		{
			name: "no_panic_passthrough",
			handler: func(c *gin.Context) {
				response.Success(c, gin.H{"ok": trueREDACTED)
		REDACTED,
			wantHTTPCode: http.StatusOK,
			wantBody: response.Response{
				Code:    0,
				Message: "success",
				Data:    map[string]any{"ok": trueREDACTED,
		REDACTED,
	REDACTED,
		{
			name: "panic_after_write_does_not_override_body",
			handler: func(c *gin.Context) {
				response.Success(c, gin.H{"ok": trueREDACTED)
				panic("boom")
		REDACTED,
			wantHTTPCode: http.StatusOK,
			wantBody: response.Response{
				Code:    0,
				Message: "success",
				Data:    map[string]any{"ok": trueREDACTED,
		REDACTED,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			r.Use(Recovery())
			r.GET("/t", tt.handler)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/t", nil)
			r.ServeHTTP(w, req)

			require.Equal(t, tt.wantHTTPCode, w.Code)

			var got response.Response
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
			require.Equal(t, tt.wantBody, got)
	REDACTED)
REDACTED
REDACTED
