//go:build unit

package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type organizationContextRepositoryStub struct {
	service.OrganizationRepository
	requestedUserID int64
	result          *service.OrganizationContext
}

func (s *organizationContextRepositoryStub) GetContextForUser(_ context.Context, userID int64) (*service.OrganizationContext, error) {
	s.requestedUserID = userID
	return s.result, nil
}

func TestRequireOrganizationDerivesScopeFromAuthenticatedSubject(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &organizationContextRepositoryStub{result: &service.OrganizationContext{
		OrganizationID:     7,
		OrganizationStatus: service.OrganizationStatusActive,
		MembershipStatus:   service.MembershipStatusActive,
		Role:               service.OrganizationRoleOwner,
	}}
	handler := NewOrganizationHandler(service.NewOrganizationService(repo, nil, &config.Config{}), nil, nil)

	router := gin.New()
	router.GET("/organization",
		func(c *gin.Context) {
			c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 42})
			c.Next()
		},
		handler.RequireOrganization,
		func(c *gin.Context) {
			resolved, exists := c.Get(OrganizationContextKey)
			require.True(t, exists)
			c.JSON(http.StatusOK, resolved)
		},
	)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/organization?organization_id=999", nil)
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, int64(42), repo.requestedUserID)
	require.Contains(t, response.Body.String(), `"organization_id":7`)
	require.NotContains(t, response.Body.String(), `999`)
}
