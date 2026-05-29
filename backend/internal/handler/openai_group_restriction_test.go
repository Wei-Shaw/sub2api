package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIGroupCodexOfficialRestriction_FallbackUsesConfiguredOpenAIGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupID := int64(10)
	fallbackID := int64(20)
	apiKey := &service.APIKey{
		ID:      1,
		GroupID: &groupID,
		Group: &service.Group{
			ID:                groupID,
			Platform:          service.PlatformOpenAI,
			CodexOfficialOnly: true,
			FallbackGroupID:   &fallbackID,
		},
	}
	fallbackGroup := &service.Group{
		ID:       fallbackID,
		Platform: service.PlatformOpenAI,
		Status:   service.StatusActive,
	}
	handler := &OpenAIGatewayHandler{
		apiKeyService: service.NewAPIKeyService(
			nil,
			nil,
			&openAIGroupRestrictionGroupRepoStub{
				groups: map[int64]*service.Group{
					fallbackID: fallbackGroup,
				},
			},
			nil,
			nil,
			nil,
			nil,
		),
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "curl/8.7.1")

	effectiveAPIKey, rejected := handler.applyOpenAIGroupCodexOfficialRestriction(c, apiKey, false)

	require.False(t, rejected)
	require.NotNil(t, effectiveAPIKey)
	require.NotSame(t, apiKey, effectiveAPIKey)
	require.Equal(t, fallbackID, *effectiveAPIKey.GroupID)
	require.Same(t, fallbackGroup, effectiveAPIKey.Group)
	require.Equal(t, http.StatusOK, rec.Code)
}

type openAIGroupRestrictionGroupRepoStub struct {
	groups map[int64]*service.Group
}

func (s *openAIGroupRestrictionGroupRepoStub) Create(context.Context, *service.Group) error {
	panic("unexpected Create call")
}

func (s *openAIGroupRestrictionGroupRepoStub) GetByID(ctx context.Context, id int64) (*service.Group, error) {
	return s.GetByIDLite(ctx, id)
}

func (s *openAIGroupRestrictionGroupRepoStub) GetByIDLite(_ context.Context, id int64) (*service.Group, error) {
	group, ok := s.groups[id]
	if !ok {
		return nil, service.ErrGroupNotFound
	}
	return group, nil
}

func (s *openAIGroupRestrictionGroupRepoStub) Update(context.Context, *service.Group) error {
	panic("unexpected Update call")
}

func (s *openAIGroupRestrictionGroupRepoStub) Delete(context.Context, int64) error {
	panic("unexpected Delete call")
}

func (s *openAIGroupRestrictionGroupRepoStub) DeleteCascade(context.Context, int64) ([]int64, error) {
	panic("unexpected DeleteCascade call")
}

func (s *openAIGroupRestrictionGroupRepoStub) List(context.Context, pagination.PaginationParams) ([]service.Group, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}

func (s *openAIGroupRestrictionGroupRepoStub) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string, *bool) ([]service.Group, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters call")
}

func (s *openAIGroupRestrictionGroupRepoStub) ListActive(context.Context) ([]service.Group, error) {
	panic("unexpected ListActive call")
}

func (s *openAIGroupRestrictionGroupRepoStub) ListActiveByPlatform(context.Context, string) ([]service.Group, error) {
	panic("unexpected ListActiveByPlatform call")
}

func (s *openAIGroupRestrictionGroupRepoStub) ExistsByName(context.Context, string) (bool, error) {
	panic("unexpected ExistsByName call")
}

func (s *openAIGroupRestrictionGroupRepoStub) GetAccountCount(context.Context, int64) (int64, int64, error) {
	panic("unexpected GetAccountCount call")
}

func (s *openAIGroupRestrictionGroupRepoStub) DeleteAccountGroupsByGroupID(context.Context, int64) (int64, error) {
	panic("unexpected DeleteAccountGroupsByGroupID call")
}

func (s *openAIGroupRestrictionGroupRepoStub) GetAccountIDsByGroupIDs(context.Context, []int64) ([]int64, error) {
	panic("unexpected GetAccountIDsByGroupIDs call")
}

func (s *openAIGroupRestrictionGroupRepoStub) BindAccountsToGroup(context.Context, int64, []int64) error {
	panic("unexpected BindAccountsToGroup call")
}

func (s *openAIGroupRestrictionGroupRepoStub) UpdateSortOrders(context.Context, []service.GroupSortOrderUpdate) error {
	panic("unexpected UpdateSortOrders call")
}
