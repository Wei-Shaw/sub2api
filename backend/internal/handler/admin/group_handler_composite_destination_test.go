package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type compositeDestinationErrorAdminStub struct {
	service.AdminService
	calls int
}

func (s *compositeDestinationErrorAdminStub) CreateCompositeRoute(_ context.Context, _ int64, _ service.CompositeRouteInput) (*service.CompositeModelRoute, error) {
	s.calls++
	return nil, infraerrors.BadRequest("INVALID_COMPOSITE_ROUTE_DESTINATION", "invalid composite route destination")
}
func (s *compositeDestinationErrorAdminStub) UpdateCompositeRoute(ctx context.Context, groupID, _ int64, input service.CompositeRouteInput) (*service.CompositeModelRoute, error) {
	return s.CreateCompositeRoute(ctx, groupID, input)
}

func TestCompositeRouteDestinationValidationReturnsHTTP400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, method := range []string{http.MethodPost, http.MethodPut} {
		for _, fields := range []string{
			`"target_group_id":20,"target_account_id":31`,
			`"target_group_id":999999`,
			`"target_account_id":999999`,
		} {
			t.Run(method+fields, func(t *testing.T) {
				svc := &compositeDestinationErrorAdminStub{}
				h := NewGroupHandler(svc, nil, nil)
				router := gin.New()
				router.POST("/groups/:id/composite-routes", h.CreateCompositeRoute)
				router.PUT("/groups/:id/composite-routes/:route_id", h.UpdateCompositeRoute)
				path := "/groups/7/composite-routes"
				if method == http.MethodPut {
					path += "/10"
				}
				request := httptest.NewRequest(method, path, strings.NewReader(`{"public_model":"alias","target_platform":"openai",`+fields+`}`))
				request.Header.Set("Content-Type", "application/json")
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
				require.Contains(t, response.Body.String(), "INVALID_COMPOSITE_ROUTE_DESTINATION")
				require.Equal(t, 1, svc.calls, "request must reach service validation rather than fail binding")
			})
		}
	}
}
