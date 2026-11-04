//go:build unit

package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// groupScopedSchedulerCache 按桶的分组只返回该分组的成员账号，并记录被查询过的分组。
type groupScopedSchedulerCache struct {
	*fakeSchedulerCache
	mu       sync.Mutex
	groupIDs []int64
}

func (c *groupScopedSchedulerCache) GetSnapshot(_ context.Context, bucket service.SchedulerBucket) ([]*service.Account, bool, error) {
	c.mu.Lock()
	c.groupIDs = append(c.groupIDs, bucket.GroupID)
	c.mu.Unlock()
	var members []*service.Account
	for _, account := range c.accounts {
		for _, ag := range account.AccountGroups {
			if ag.GroupID == bucket.GroupID {
				members = append(members, account)
				break
			}
		}
	}
	return members, true, nil
}

func (c *groupScopedSchedulerCache) queriedGroupIDs() []int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]int64(nil), c.groupIDs...)
}

type groupMapRepo struct {
	*fakeGroupRepo
	groups map[int64]*service.Group
}

func (r *groupMapRepo) GetByID(_ context.Context, id int64) (*service.Group, error) {
	if group, ok := r.groups[id]; ok {
		return group, nil
	}
	return nil, service.ErrGroupNotFound
}

func (r *groupMapRepo) GetByIDLite(ctx context.Context, id int64) (*service.Group, error) {
	return r.GetByID(ctx, id)
}

func TestGatewayOpenAICompatibleHandlersClaudeCodeOnlyFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const (
		primaryGroupID    = int64(9300)
		fallbackGroupID   = int64(9301)
		primaryAccountID  = int64(9310)
		fallbackAccountID = int64(9311)
	)

	// 账号不带 api_key：转发在取令牌时失败，请求不会触达上游，断言只看选号结果。
	newAccount := func(id, groupID int64) *service.Account {
		return &service.Account{
			ID: id, Platform: service.PlatformAnthropic, Type: service.AccountTypeAPIKey,
			Status: service.StatusActive, Schedulable: true, Concurrency: 1,
			AccountGroups: []service.AccountGroup{{AccountID: id, GroupID: groupID}},
		}
	}

	endpoints := []struct {
		name string
		path string
		body string
		call func(*GatewayHandler, *gin.Context)
	}{
		{
			name: "responses", path: "/v1/responses",
			body: `{"model":"claude-sonnet-4-5","input":"hello","stream":false}`,
			call: (*GatewayHandler).Responses,
		},
		{
			name: "chat completions", path: "/v1/chat/completions",
			body: `{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"hello"}],"stream":false}`,
			call: (*GatewayHandler).ChatCompletions,
		},
	}

	for _, ep := range endpoints {
		for _, tc := range []struct {
			name        string
			hasFallback bool
		}{
			{name: "with fallback group", hasFallback: true},
			{name: "without fallback group", hasFallback: false},
		} {
			t.Run(ep.name+"/"+tc.name, func(t *testing.T) {
				primary := &service.Group{
					ID: primaryGroupID, Hydrated: true, Platform: service.PlatformAnthropic,
					Status: service.StatusActive, ClaudeCodeOnly: true,
				}
				if tc.hasFallback {
					fallbackID := fallbackGroupID
					primary.FallbackGroupID = &fallbackID
				}
				fallback := &service.Group{
					ID: fallbackGroupID, Hydrated: true, Platform: service.PlatformAnthropic,
					Status: service.StatusActive,
				}

				schedulerCache := &groupScopedSchedulerCache{fakeSchedulerCache: &fakeSchedulerCache{accounts: []*service.Account{
					newAccount(primaryAccountID, primaryGroupID),
					newAccount(fallbackAccountID, fallbackGroupID),
				}}}
				gatewayService := service.NewGatewayService(
					nil, &groupMapRepo{fakeGroupRepo: &fakeGroupRepo{}, groups: map[int64]*service.Group{
						primaryGroupID:  primary,
						fallbackGroupID: fallback,
					}}, nil, nil, nil, nil, nil, nil, nil,
					service.NewSchedulerSnapshotService(schedulerCache, nil, nil, nil, nil),
					nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
				)
				cfg := &config.Config{RunMode: config.RunModeSimple}
				billingCacheService := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
				t.Cleanup(billingCacheService.Stop)
				h := &GatewayHandler{
					gatewayService:      gatewayService,
					billingCacheService: billingCacheService,
					concurrencyHelper:   NewConcurrencyHelper(service.NewConcurrencyService(&fakeConcurrencyCache{}), SSEPingFormatClaude, 0),
					maxAccountSwitches:  1,
					cfg:                 cfg,
				}

				primaryGroupIDRef := primaryGroupID
				apiKey := &service.APIKey{
					ID: 9320, UserID: 9330, GroupID: &primaryGroupIDRef, Group: primary, Status: service.StatusActive,
					User: &service.User{ID: 9330, Concurrency: 10, Balance: 100},
				}
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				ctx := context.WithValue(context.Background(), ctxkey.Group, primary)
				req := httptest.NewRequest(http.MethodPost, ep.path, bytes.NewBufferString(ep.body)).WithContext(ctx)
				req.Header.Set("Content-Type", "application/json")
				c.Request = req
				c.Set(string(middleware.ContextKeyAPIKey), apiKey)
				c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: apiKey.UserID, Concurrency: 10})

				ep.call(h, c)

				selected, reachedSelection := c.Get(opsAccountIDKey)
				if !tc.hasFallback {
					require.Equal(t, http.StatusForbidden, recorder.Code)
					require.Contains(t, recorder.Body.String(), "This group is restricted to Claude Code clients")
					require.False(t, reachedSelection, "a Claude Code only group without fallback must be rejected before account selection")
					require.Empty(t, schedulerCache.queriedGroupIDs())
					return
				}
				require.NotContains(t, recorder.Body.String(), "restricted to Claude Code clients")
				require.True(t, reachedSelection, "a Claude Code only group with fallback must reach account selection")
				require.Equal(t, fallbackAccountID, selected)
				queried := schedulerCache.queriedGroupIDs()
				require.Contains(t, queried, fallbackGroupID)
				require.NotContains(t, queried, primaryGroupID)
			})
		}
	}
}
