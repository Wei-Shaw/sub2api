package handler

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIResponses_RejectsInvalidUTF8BeforeRouting(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, suffix := range []string{"", `,"stream":false`, `,"stream":true`} {
		for _, invalid := range [][]byte{{0xbb, 0xd8, 0xb8, 0xb4}, {0xe4, 0xbd}, {0xff}} {
			t.Run(fmt.Sprintf("%s/%x", suffix, invalid), func(t *testing.T) {
				body := append([]byte(`{"input":"`), invalid...)
				body = append(body, []byte(`"`+suffix+`}`)...)
				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
				groupID := int64(2)
				c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{ID: 101, GroupID: &groupID, User: &service.User{ID: 1}})
				c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1, Concurrency: 1})
				h := newOpenAIHandlerForPreviousResponseIDValidation(t, nil)
				h.Responses(c)
				require.Equal(t, http.StatusBadRequest, w.Code)
				require.Contains(t, w.Body.String(), "valid UTF-8 JSON")
				require.Contains(t, w.Body.String(), "invalid_request_error")
			})
		}
	}
}

func TestOpenAIResponses_ValidUnicodeReachesFieldValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{"input":"回复 ok 🌍"}`))
	groupID := int64(2)
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{ID: 101, GroupID: &groupID, User: &service.User{ID: 1}})
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1, Concurrency: 1})
	h := newOpenAIHandlerForPreviousResponseIDValidation(t, nil)
	h.Responses(c)
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "model is required")
	require.NotContains(t, w.Body.String(), "UTF-8")
}
