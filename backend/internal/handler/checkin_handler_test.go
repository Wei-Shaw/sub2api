package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type checkInHandlerRepo struct {
	service.CheckInRepository
	status *service.CheckInStatus
	result *service.CheckInResult
}

func (r *checkInHandlerRepo) GetUserStatus(context.Context, int64, time.Time, int) (*service.CheckInStatus, error) {
	return r.status, nil
}

func (r *checkInHandlerRepo) CheckIn(context.Context, int64, time.Time, service.CheckInRewardPicker) (*service.CheckInResult, error) {
	return r.result, nil
}

func TestCheckInHandlerHidesRewardRules(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, checked := range []bool{false, true} {
		for _, mode := range []string{service.CheckInModeStandard, service.CheckInModeReduced, service.CheckInModeCampaign} {
			t.Run(mode+"/checked="+strconv.FormatBool(checked), func(t *testing.T) {
				record := service.CheckInRecord{ID: 1, Date: "2026-09-25", Reward: 10, Mode: mode, CycleRewardAfter: 40}
				repo := &checkInHandlerRepo{
					status: &service.CheckInStatus{
						Config: service.CheckInConfig{
							Enabled: true, StandardMin: 3, StandardMax: 10,
							ReducedThreshold: 30, ReducedMin: 1, ReducedMax: 3, CampaignReward: 10,
						},
						CheckedToday: checked, TotalReward: 40, CycleReward: 40,
						CampaignEligible: mode == service.CheckInModeCampaign,
						RecentCheckIns:   []service.CheckInRecord{record},
					},
					result: &service.CheckInResult{Record: record, AlreadyChecked: checked, NewBalance: 50},
				}
				if checked {
					repo.status.TodayReward = record.Reward
				}
				h := NewCheckInHandler(service.NewCheckInService(repo, nil, nil))
				for _, method := range []string{http.MethodGet, http.MethodPost} {
					w := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(w)
					c.Request = httptest.NewRequest(method, "/api/v1/user/checkin", nil)
					c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7})
					if method == http.MethodGet {
						h.GetStatus(c)
					} else {
						h.CheckIn(c)
					}
					require.Equal(t, http.StatusOK, w.Code)
					var body struct {
						Data map[string]any `json:"data"`
					}
					require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
					for _, field := range []string{
						"standard_min", "standard_max", "reduced_threshold", "reduced_min", "reduced_max",
						"campaign_start", "campaign_end", "campaign_reward", "campaign_eligible",
						"next_reward_mode", "next_reward_min", "next_reward_max", "cycle_reward", "cycle_reward_after", "mode",
					} {
						require.NotContains(t, w.Body.String(), `"`+field+`"`, method+" exposes "+field)
					}
					if method == http.MethodGet {
						require.Equal(t, map[string]any{"enabled": true}, body.Data["config"])
						require.Equal(t, checked, body.Data["checked_today"])
						require.Equal(t, repo.status.TodayReward, body.Data["today_reward"])
						history, ok := body.Data["recent_checkins"].([]any)
						require.True(t, ok, "recent_checkins should be an array")
						require.NotEmpty(t, history)
						historyRecord, ok := history[0].(map[string]any)
						require.True(t, ok, "recent_checkins[0] should be an object")
						require.Equal(t, record.Reward, historyRecord["reward"])
					} else {
						checkInRecord, ok := body.Data["record"].(map[string]any)
						require.True(t, ok, "record should be an object")
						require.Equal(t, record.Reward, checkInRecord["reward"])
						require.Equal(t, checked, body.Data["already_checked"])
						require.Equal(t, float64(50), body.Data["new_balance"])
					}
				}
				// 用户响应过滤不能改变管理端复用的完整奖励配置。
				raw, err := json.Marshal(repo.status.Config)
				require.NoError(t, err)
				require.Contains(t, string(raw), `"standard_max":10`)
				require.Contains(t, string(raw), `"campaign_reward":10`)
			})
		}
	}
}
