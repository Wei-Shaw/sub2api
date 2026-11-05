//go:build unit

package admin

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
)

func TestAdminFullAccessIdempotencyScopeIsolation(t *testing.T) {
	scope := func(p *adminaccess.Principal) string {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/", nil)
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 9})
		c.Request = c.Request.WithContext(adminaccess.WithPrincipal(context.Background(), p))
		return adminActorScope(c)
	}
	jwt := scope(nil)
	full := scope(&adminaccess.Principal{KeyID: "one", Policy: adminaccess.Policy{FullAccess: true}})
	restricted := scope(&adminaccess.Principal{KeyID: "one", Policy: adminaccess.Policy{}})
	other := scope(&adminaccess.Principal{KeyID: "two", Policy: adminaccess.Policy{FullAccess: true}})
	require.NotEqual(t, jwt, full)
	require.NotEqual(t, full, restricted)
	require.NotEqual(t, full, other)
}
