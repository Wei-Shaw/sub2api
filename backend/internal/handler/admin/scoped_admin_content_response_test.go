//go:build unit

package admin

import (
	"context"
	"encoding/json"
	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
	"time"
)

type scopedContentPromoRepo struct{ service.PromoCodeRepository }

func (scopedContentPromoRepo) ListUsagesByPromoCode(context.Context, int64, pagination.PaginationParams) ([]service.PromoCodeUsage, *pagination.PaginationResult, error) {
	return []service.PromoCodeUsage{{ID: 1, PromoCodeID: 2, UserID: 3, BonusAmount: 4, User: &service.User{ID: 3, Email: "private@example.com", Balance: 999, FrozenBalance: 888, TotalRecharged: 777}}}, &pagination.PaginationResult{Total: 1}, nil
}

type scopedContentUsers struct{ service.UserRepository }

func (scopedContentUsers) ListWithFilters(context.Context, pagination.PaginationParams, service.UserListFilters) ([]service.User, *pagination.PaginationResult, error) {
	return []service.User{{ID: 3, Email: "private@example.com", Balance: 999}}, &pagination.PaginationResult{Total: 1}, nil
}

type scopedContentSubs struct {
	service.UserSubscriptionRepository
}

func (scopedContentSubs) ListActiveByUserID(context.Context, int64) ([]service.UserSubscription, error) {
	return nil, nil
}

type scopedContentReads struct {
	service.AnnouncementReadRepository
}

func (scopedContentReads) GetReadMapByUsers(context.Context, int64, []int64) (map[int64]time.Time, error) {
	return map[int64]time.Time{3: time.Unix(100, 0).UTC()}, nil
}
func TestScopedAdminContentHandlersResponseBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	promo := NewPromoHandler(service.NewPromoService(scopedContentPromoRepo{}, nil, nil, nil, nil))
	announcement := NewAnnouncementHandler(service.NewAnnouncementService(&announcementRepoCapture{}, scopedContentReads{}, scopedContentUsers{}, scopedContentSubs{}))
	for _, endpoint := range []struct {
		name, path, request string
		handler             gin.HandlerFunc
	}{{"优惠码", "/promo-codes/:id/usages", "/promo-codes/2/usages", promo.GetUsages}, {"公告", "/announcements/:id/read-status", "/announcements/2/read-status", announcement.ListReadStatus}} {
		for _, scoped := range []bool{false, true} {
			t.Run(endpoint.name+map[bool]string{true: "受限", false: "普通"}[scoped], func(t *testing.T) {
				r := gin.New()
				r.Use(func(c *gin.Context) {
					if scoped {
						c.Request = c.Request.WithContext(adminaccess.WithPrincipal(c.Request.Context(), &adminaccess.Principal{Policy: adminaccess.Policy{GlobalPermissions: []string{"promo.read", "announcements.read"}}}))
					}
					c.Next()
				})
				r.GET(endpoint.path, endpoint.handler)
				w := httptest.NewRecorder()
				r.ServeHTTP(w, httptest.NewRequest("GET", endpoint.request, nil))
				require.Equal(t, 200, w.Code)
				var body struct {
					Data struct {
						Items []map[string]json.RawMessage `json:"items"`
					} `json:"data"`
				}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
				require.Len(t, body.Data.Items, 1)
				item := body.Data.Items[0]
				if scoped {
					require.NotContains(t, w.Body.String(), "private@example.com")
					require.NotContains(t, w.Body.String(), "999")
					require.NotContains(t, item, "user")
					require.NotContains(t, item, "balance")
					require.Contains(t, item, "user_id")
					if endpoint.name == "优惠码" {
						require.Contains(t, item, "bonus_amount")
						require.Contains(t, item, "used_at")
					} else {
						require.Contains(t, item, "eligible")
						require.Contains(t, item, "read_at")
					}
				} else {
					require.Contains(t, w.Body.String(), "private@example.com")
					require.Contains(t, w.Body.String(), "999")
				}
			})
		}
	}
}
