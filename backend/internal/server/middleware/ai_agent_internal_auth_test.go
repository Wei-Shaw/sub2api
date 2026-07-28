package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func TestAdminAuthAcceptsSignedLoopbackAIAgentDelegation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	internalAuth, err := service.NewAgentInternalAuth()
	if err != nil {
		t.Fatalf("NewAgentInternalAuth() error = %v", err)
	}
	token, err := internalAuth.Sign(service.AgentInternalIdentity{
		UserID: 42, Concurrency: 7, Email: "admin@example.com", SessionID: "session-1",
	}, http.MethodGet, "/admin/test")
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	router := gin.New()
	router.GET("/admin/test", gin.HandlerFunc(NewAdminAuthMiddleware(nil, nil, nil, nil, internalAuth)), func(c *gin.Context) {
		subject, ok := GetAuthSubjectFromContext(c)
		if !ok {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.JSON(http.StatusOK, gin.H{"user_id": subject.UserID, "session_id": c.GetString(ContextKeySessionID)})
	})

	request := httptest.NewRequest(http.MethodGet, "/admin/test", nil)
	request.RemoteAddr = "127.0.0.1:4321"
	request.Header.Set(service.AgentInternalAuthHeader, token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if got := response.Body.String(); got != `{"session_id":"session-1","user_id":42}` {
		t.Fatalf("body = %s", got)
	}
}
