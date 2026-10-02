package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// modelDowngradeGuardManagerStub 实现 ModelDowngradeGuardManager 的三个能力，
// 每个用例只填自己关心的那一部分。
type modelDowngradeGuardManagerStub struct {
	listResult *service.ModelDowngradeBlockedList
	listErr    error
	listCalls  int

	releaseErr   error
	releaseCalls int
	releaseID    int64
	releaseScope string
	releaseModel string

	applyResult *service.ModelDowngradeManualBlockResult
	applyErr    error
	applyCalls  int
	applyID     int64
	applyModel  string
}

func (s *modelDowngradeGuardManagerStub) ListModelDowngradeBlocked(context.Context) (*service.ModelDowngradeBlockedList, error) {
	s.listCalls++
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.listResult, nil
}

func (s *modelDowngradeGuardManagerStub) ReleaseModelDowngradeBlock(_ context.Context, accountID int64, scope, model string) error {
	s.releaseCalls++
	s.releaseID = accountID
	s.releaseScope = scope
	s.releaseModel = model
	return s.releaseErr
}

func (s *modelDowngradeGuardManagerStub) ApplyModelDowngradeBlockNow(
	_ context.Context, accountID int64, sentModel string,
) (*service.ModelDowngradeManualBlockResult, error) {
	s.applyCalls++
	s.applyID = accountID
	s.applyModel = sentModel
	if s.applyErr != nil {
		return nil, s.applyErr
	}
	return s.applyResult, nil
}

// newModelDowngradeBlockedRouter 注册守卫的三条运行时路由；manager 为 nil 时模拟没装配依赖。
func newModelDowngradeBlockedRouter(manager *modelDowngradeGuardManagerStub) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := &SettingHandler{}
	if manager != nil {
		h.SetModelDowngradeGuardManager(manager)
	}
	router := gin.New()
	router.GET("/api/v1/admin/settings/model-downgrade-guard/blocked", h.GetModelDowngradeGuardBlocked)
	router.DELETE("/api/v1/admin/settings/model-downgrade-guard/blocked/:id", h.ReleaseModelDowngradeGuardBlocked)
	router.POST("/api/v1/admin/settings/model-downgrade-guard/blocked/:id/apply", h.ApplyModelDowngradeGuardBlocked)
	return router
}

func TestSettingHandler_GetModelDowngradeGuardBlocked(t *testing.T) {
	triggeredAt := time.Date(2026, 9, 18, 2, 0, 0, 0, time.UTC)
	until := time.Date(2026, 9, 19, 2, 0, 0, 0, time.UTC)
	lister := &modelDowngradeGuardManagerStub{
		listResult: &service.ModelDowngradeBlockedList{
			Items: []service.ModelDowngradeBlockedAccount{
				{
					AccountID:            101,
					AccountName:          "openai-pool-1",
					Scope:                service.ModelDowngradeBlockedScopeAccount,
					SentModel:            "gpt-6-astra",
					ResponseModel:        "gpt-5.6-luna",
					TriggerCount:         5,
					TriggerThreshold:     5,
					TriggerWindowMinutes: 30,
					TriggeredAt:          triggeredAt,
					Until:                until,
				},
				{
					AccountID:     102,
					AccountName:   "openai-pool-2",
					Scope:         service.ModelDowngradeBlockedScopeModel,
					Model:         "gpt-6-astra",
					SentModel:     "gpt-6-astra",
					ResponseModel: "gpt-5.6-luna",
					Until:         until,
				},
			},
			Summary: service.ModelDowngradeBlockedSummary{
				Blocked:         2,
				TotalActive:     20,
				MaxBlockedRatio: 0.3,
			},
		},
	}

	router := newModelDowngradeBlockedRouter(lister)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings/model-downgrade-guard/blocked", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, 1, lister.listCalls)

	var body struct {
		Data struct {
			Items []struct {
				AccountID            int64  `json:"account_id"`
				AccountName          string `json:"account_name"`
				Scope                string `json:"scope"`
				Model                string `json:"model"`
				SentModel            string `json:"sent_model"`
				ResponseModel        string `json:"response_model"`
				TriggerCount         int64  `json:"trigger_count"`
				TriggerThreshold     int    `json:"trigger_threshold"`
				TriggerWindowMinutes int    `json:"trigger_window_minutes"`
				TriggeredAt          string `json:"triggered_at"`
				Until                string `json:"until"`
			} `json:"items"`
			Summary struct {
				Blocked         int64   `json:"blocked"`
				TotalActive     int64   `json:"total_active"`
				MaxBlockedRatio float64 `json:"max_blocked_ratio"`
			} `json:"summary"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	require.Len(t, body.Data.Items, 2)
	require.Equal(t, int64(101), body.Data.Items[0].AccountID)
	require.Equal(t, "account", body.Data.Items[0].Scope)
	require.Equal(t, "", body.Data.Items[0].Model)
	require.Equal(t, "gpt-6-astra", body.Data.Items[0].SentModel)
	require.Equal(t, "gpt-5.6-luna", body.Data.Items[0].ResponseModel)
	require.Equal(t, int64(5), body.Data.Items[0].TriggerCount)
	require.Equal(t, 30, body.Data.Items[0].TriggerWindowMinutes)
	require.Equal(t, triggeredAt.Format(time.RFC3339), body.Data.Items[0].TriggeredAt)
	require.Equal(t, until.Format(time.RFC3339), body.Data.Items[0].Until)

	require.Equal(t, "model", body.Data.Items[1].Scope)
	require.Equal(t, "gpt-6-astra", body.Data.Items[1].Model)
	// TriggeredAt 为零值时不输出，前端按缺省展示。
	require.Equal(t, "", body.Data.Items[1].TriggeredAt)

	require.Equal(t, int64(2), body.Data.Summary.Blocked)
	require.Equal(t, int64(20), body.Data.Summary.TotalActive)
	require.InDelta(t, 0.3, body.Data.Summary.MaxBlockedRatio, 1e-9)
}

func TestSettingHandler_GetModelDowngradeGuardBlocked_EmptyItemsIsArray(t *testing.T) {
	lister := &modelDowngradeGuardManagerStub{
		listResult: &service.ModelDowngradeBlockedList{Items: nil},
	}
	router := newModelDowngradeBlockedRouter(lister)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings/model-downgrade-guard/blocked", nil))

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"items":[]`)
}

func TestSettingHandler_GetModelDowngradeGuardBlocked_Errors(t *testing.T) {
	t.Run("lister not configured", func(t *testing.T) {
		router := newModelDowngradeBlockedRouter(nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings/model-downgrade-guard/blocked", nil))
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("lister failure", func(t *testing.T) {
		router := newModelDowngradeBlockedRouter(&modelDowngradeGuardManagerStub{listErr: errors.New("boom")})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings/model-downgrade-guard/blocked", nil))
		require.GreaterOrEqual(t, w.Code, http.StatusBadRequest)
	})
}

func newModelDowngradeReleaseRouter(releaser *modelDowngradeGuardManagerStub) *gin.Engine {
	return newModelDowngradeBlockedRouter(releaser)
}

func releaseModelDowngradeBlocked(router *gin.Engine, target string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, target, nil))
	return w
}

func TestSettingHandler_ReleaseModelDowngradeGuardBlocked(t *testing.T) {
	t.Run("account scope defaults when scope is omitted", func(t *testing.T) {
		releaser := &modelDowngradeGuardManagerStub{}
		w := releaseModelDowngradeBlocked(
			newModelDowngradeReleaseRouter(releaser),
			"/api/v1/admin/settings/model-downgrade-guard/blocked/101",
		)

		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, 1, releaser.releaseCalls)
		require.Equal(t, int64(101), releaser.releaseID)
		require.Equal(t, service.ModelDowngradeBlockedScopeAccount, releaser.releaseScope)
		require.Equal(t, "", releaser.releaseModel)
	})

	t.Run("model scope forwards the model", func(t *testing.T) {
		releaser := &modelDowngradeGuardManagerStub{}
		w := releaseModelDowngradeBlocked(
			newModelDowngradeReleaseRouter(releaser),
			"/api/v1/admin/settings/model-downgrade-guard/blocked/102?scope=model&model=gpt-6-astra",
		)

		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, service.ModelDowngradeBlockedScopeModel, releaser.releaseScope)
		require.Equal(t, "gpt-6-astra", releaser.releaseModel)
		require.Contains(t, w.Body.String(), `"model":"gpt-6-astra"`)
	})

	t.Run("not found maps to 404", func(t *testing.T) {
		releaser := &modelDowngradeGuardManagerStub{releaseErr: service.ErrModelDowngradeBlockNotFound}
		w := releaseModelDowngradeBlocked(
			newModelDowngradeReleaseRouter(releaser),
			"/api/v1/admin/settings/model-downgrade-guard/blocked/101",
		)

		require.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("invalid scope maps to 400", func(t *testing.T) {
		releaser := &modelDowngradeGuardManagerStub{releaseErr: service.ErrModelDowngradeBlockScopeInvalid}
		w := releaseModelDowngradeBlocked(
			newModelDowngradeReleaseRouter(releaser),
			"/api/v1/admin/settings/model-downgrade-guard/blocked/101?scope=model",
		)

		require.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("invalid account id is rejected before reaching the service", func(t *testing.T) {
		releaser := &modelDowngradeGuardManagerStub{}
		router := newModelDowngradeReleaseRouter(releaser)

		for _, target := range []string{
			"/api/v1/admin/settings/model-downgrade-guard/blocked/abc",
			"/api/v1/admin/settings/model-downgrade-guard/blocked/0",
			"/api/v1/admin/settings/model-downgrade-guard/blocked/-3",
		} {
			w := releaseModelDowngradeBlocked(router, target)
			require.Equal(t, http.StatusBadRequest, w.Code, target)
		}
		require.Zero(t, releaser.releaseCalls)
	})

	t.Run("releaser not configured", func(t *testing.T) {
		w := releaseModelDowngradeBlocked(
			newModelDowngradeReleaseRouter(nil),
			"/api/v1/admin/settings/model-downgrade-guard/blocked/101",
		)
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

// ==================== 观察记录 ====================

// 观察行和真实下线行同表返回，status / cause / 比例上限的分子分母都要透出去，
// 前端才能画出不同的状态徽标。
func TestSettingHandler_GetModelDowngradeGuardBlocked_ObservedRows(t *testing.T) {
	observedAt := time.Date(2026, 9, 18, 2, 0, 0, 0, time.UTC)
	expiresAt := time.Date(2026, 9, 19, 2, 0, 0, 0, time.UTC)
	lister := &modelDowngradeGuardManagerStub{
		listResult: &service.ModelDowngradeBlockedList{
			Items: []service.ModelDowngradeBlockedAccount{
				{
					AccountID:            103,
					AccountName:          "openai-pool-3",
					Scope:                service.ModelDowngradeBlockedScopeObserved,
					Status:               service.ModelDowngradeBlockedStatusObserved,
					Cause:                service.ModelDowngradeObservedCauseDryRun,
					Model:                "gpt-6-astra",
					SentModel:            "gpt-6-astra",
					ResponseModel:        "gpt-5.6-luna",
					TriggerCount:         5,
					TriggerThreshold:     5,
					TriggerWindowMinutes: 30,
					TriggeredAt:          observedAt,
					Until:                expiresAt,
				},
				{
					AccountID:       104,
					AccountName:     "openai-pool-4",
					Scope:           service.ModelDowngradeBlockedScopeObserved,
					Status:          service.ModelDowngradeBlockedStatusRatioCapped,
					Cause:           service.ModelDowngradeObservedCauseRatioCap,
					Model:           "gpt-6-sol",
					SentModel:       "gpt-6-sol",
					ResponseModel:   "gpt-5.6-luna",
					TriggeredAt:     observedAt,
					Until:           expiresAt,
					Blocked:         3,
					Total:           10,
					MaxBlockedRatio: 0.3,
				},
			},
			Summary: service.ModelDowngradeBlockedSummary{Blocked: 1, Observed: 2, TotalActive: 20, MaxBlockedRatio: 0.3},
		},
	}

	router := newModelDowngradeBlockedRouter(lister)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings/model-downgrade-guard/blocked", nil))

	require.Equal(t, http.StatusOK, w.Code)
	var body struct {
		Data struct {
			Items []struct {
				AccountID       int64   `json:"account_id"`
				Scope           string  `json:"scope"`
				Status          string  `json:"status"`
				Cause           string  `json:"cause"`
				Model           string  `json:"model"`
				Blocked         int64   `json:"blocked"`
				Total           int64   `json:"total"`
				MaxBlockedRatio float64 `json:"max_blocked_ratio"`
			} `json:"items"`
			Summary struct {
				Blocked  int64 `json:"blocked"`
				Observed int64 `json:"observed"`
			} `json:"summary"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	require.Len(t, body.Data.Items, 2)
	require.Equal(t, "observed", body.Data.Items[0].Scope)
	require.Equal(t, "observed", body.Data.Items[0].Status)
	require.Equal(t, "dry_run", body.Data.Items[0].Cause)
	require.Equal(t, "gpt-6-astra", body.Data.Items[0].Model)
	// 观察模式没有分子分母，omitempty 之后前端拿到 0。
	require.Zero(t, body.Data.Items[0].Blocked)

	require.Equal(t, "ratio_capped", body.Data.Items[1].Status)
	require.Equal(t, "ratio_cap", body.Data.Items[1].Cause)
	require.Equal(t, int64(3), body.Data.Items[1].Blocked)
	require.Equal(t, int64(10), body.Data.Items[1].Total)
	require.InDelta(t, 0.3, body.Data.Items[1].MaxBlockedRatio, 1e-9)

	require.Equal(t, int64(1), body.Data.Summary.Blocked)
	require.Equal(t, int64(2), body.Data.Summary.Observed)
}

// 不带 Status 的行默认按真实受限处理，前端不会拿到空状态。
func TestSettingHandler_GetModelDowngradeGuardBlocked_DefaultsStatusToBlocked(t *testing.T) {
	lister := &modelDowngradeGuardManagerStub{
		listResult: &service.ModelDowngradeBlockedList{
			Items: []service.ModelDowngradeBlockedAccount{
				{AccountID: 101, Scope: service.ModelDowngradeBlockedScopeAccount, Until: time.Now()},
			},
		},
	}
	router := newModelDowngradeBlockedRouter(lister)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings/model-downgrade-guard/blocked", nil))

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"status":"blocked"`)
}

func TestSettingHandler_ReleaseModelDowngradeGuardBlocked_ObservedScope(t *testing.T) {
	releaser := &modelDowngradeGuardManagerStub{}
	w := releaseModelDowngradeBlocked(
		newModelDowngradeReleaseRouter(releaser),
		"/api/v1/admin/settings/model-downgrade-guard/blocked/103?scope=observed&model=gpt-6-astra",
	)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, service.ModelDowngradeBlockedScopeObserved, releaser.releaseScope)
	require.Equal(t, "gpt-6-astra", releaser.releaseModel)
}

// ==================== 立即下线（观察记录转正） ====================

func newModelDowngradeApplyRouter(applier *modelDowngradeGuardManagerStub) *gin.Engine {
	return newModelDowngradeBlockedRouter(applier)
}

func applyModelDowngradeBlocked(router *gin.Engine, target string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, target, nil))
	return w
}

func TestSettingHandler_ApplyModelDowngradeGuardBlocked(t *testing.T) {
	until := time.Date(2026, 9, 19, 2, 0, 0, 0, time.UTC)

	t.Run("applies and echoes the resolved scope", func(t *testing.T) {
		applier := &modelDowngradeGuardManagerStub{
			applyResult: &service.ModelDowngradeManualBlockResult{
				AccountID: 103,
				Scope:     service.ModelDowngradeBlockedScopeAccount,
				Until:     until,
			},
		}
		w := applyModelDowngradeBlocked(
			newModelDowngradeApplyRouter(applier),
			"/api/v1/admin/settings/model-downgrade-guard/blocked/103/apply?model=gpt-6-astra",
		)

		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, 1, applier.applyCalls)
		require.Equal(t, int64(103), applier.applyID)
		require.Equal(t, "gpt-6-astra", applier.applyModel)
		require.Contains(t, w.Body.String(), `"scope":"account"`)
		require.Contains(t, w.Body.String(), until.Format(time.RFC3339))
	})

	t.Run("ratio cap maps to 409 with the numerator and denominator", func(t *testing.T) {
		applier := &modelDowngradeGuardManagerStub{
			applyErr: service.ErrModelDowngradeBlockRatioCapped.WithMetadata(map[string]string{
				"blocked":           "3",
				"total":             "10",
				"max_blocked_ratio": "0.3",
			}),
		}
		w := applyModelDowngradeBlocked(
			newModelDowngradeApplyRouter(applier),
			"/api/v1/admin/settings/model-downgrade-guard/blocked/103/apply?model=gpt-6-astra",
		)

		require.Equal(t, http.StatusConflict, w.Code)
		require.Contains(t, w.Body.String(), "MODEL_DOWNGRADE_BLOCK_RATIO_CAPPED")
		require.Contains(t, w.Body.String(), `"blocked":"3"`)
		require.Contains(t, w.Body.String(), `"total":"10"`)
	})

	t.Run("already blocked maps to 409 with the scope", func(t *testing.T) {
		applier := &modelDowngradeGuardManagerStub{
			applyErr: service.ErrModelDowngradeBlockAlreadyActive.WithMetadata(map[string]string{
				"scope": service.ModelDowngradeBlockedScopeAccount,
			}),
		}
		w := applyModelDowngradeBlocked(
			newModelDowngradeApplyRouter(applier),
			"/api/v1/admin/settings/model-downgrade-guard/blocked/103/apply?model=gpt-6-astra",
		)

		require.Equal(t, http.StatusConflict, w.Code)
		require.Contains(t, w.Body.String(), "MODEL_DOWNGRADE_BLOCK_ALREADY_ACTIVE")
		require.Contains(t, w.Body.String(), `"scope":"account"`)
		// 前端对这类 409 直接显示后端 message。
		require.Contains(t, w.Body.String(), "account is already blocked by the model downgrade guard")
	})

	t.Run("missing model maps to 400", func(t *testing.T) {
		applier := &modelDowngradeGuardManagerStub{}
		for _, target := range []string{
			"/api/v1/admin/settings/model-downgrade-guard/blocked/103/apply",
			"/api/v1/admin/settings/model-downgrade-guard/blocked/103/apply?model=%20%20",
		} {
			w := applyModelDowngradeBlocked(newModelDowngradeApplyRouter(applier), target)
			require.Equal(t, http.StatusBadRequest, w.Code, target)
		}
		require.Zero(t, applier.applyCalls)
	})

	t.Run("invalid account id maps to 400", func(t *testing.T) {
		applier := &modelDowngradeGuardManagerStub{}
		w := applyModelDowngradeBlocked(
			newModelDowngradeApplyRouter(applier),
			"/api/v1/admin/settings/model-downgrade-guard/blocked/0/apply?model=gpt-6-astra",
		)
		require.Equal(t, http.StatusBadRequest, w.Code)
		require.Zero(t, applier.applyCalls)
	})

	t.Run("account not found maps to 404", func(t *testing.T) {
		applier := &modelDowngradeGuardManagerStub{applyErr: service.ErrAccountNotFound}
		w := applyModelDowngradeBlocked(
			newModelDowngradeApplyRouter(applier),
			"/api/v1/admin/settings/model-downgrade-guard/blocked/103/apply?model=gpt-6-astra",
		)
		require.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("applier not configured", func(t *testing.T) {
		w := applyModelDowngradeBlocked(
			newModelDowngradeApplyRouter(nil),
			"/api/v1/admin/settings/model-downgrade-guard/blocked/103/apply?model=gpt-6-astra",
		)
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

// ==================== 配置读写 ====================

// modelDowngradeGuardSettingRepoStub 在通用设置 stub 之上放开单键写入：
// 守卫配置走 SettingRepository.Set，而通用 stub 的 Set 是 panic。
type modelDowngradeGuardSettingRepoStub struct {
	*settingHandlerRepoStub
}

func (s *modelDowngradeGuardSettingRepoStub) Set(_ context.Context, key, value string) error {
	if s.values == nil {
		s.values = map[string]string{}
	}
	s.values[key] = value
	return nil
}

func newModelDowngradeGuardSettingsHandler(stored string) (*SettingHandler, *modelDowngradeGuardSettingRepoStub) {
	gin.SetMode(gin.TestMode)
	values := map[string]string{}
	if stored != "" {
		values[service.SettingKeyModelDowngradeGuardSettings] = stored
	}
	repo := &modelDowngradeGuardSettingRepoStub{settingHandlerRepoStub: &settingHandlerRepoStub{values: values}}
	svc := service.NewSettingService(repo, &config.Config{})
	return NewSettingHandler(svc, nil, nil, nil, nil, nil, nil), repo
}

func doModelDowngradeGuardSettingsRequest(h *SettingHandler, method, body string) *httptest.ResponseRecorder {
	router := gin.New()
	router.GET("/api/v1/admin/settings/model-downgrade-guard", h.GetModelDowngradeGuardSettings)
	router.PUT("/api/v1/admin/settings/model-downgrade-guard", h.UpdateModelDowngradeGuardSettings)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, "/api/v1/admin/settings/model-downgrade-guard", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	return w
}

type modelDowngradeGuardSettingsBody struct {
	Data struct {
		Enabled bool   `json:"enabled"`
		Action  string `json:"action"`
		Pairs   []struct {
			SentModel     string `json:"sent_model"`
			ResponseModel string `json:"response_model"`
		} `json:"pairs"`
		ThresholdCount         int     `json:"threshold_count"`
		ThresholdWindowMinutes int     `json:"threshold_window_minutes"`
		BlockHours             int     `json:"block_hours"`
		MaxBlockedRatio        float64 `json:"max_blocked_ratio"`
	} `json:"data"`
}

func TestSettingHandler_GetModelDowngradeGuardSettings_Defaults(t *testing.T) {
	h, _ := newModelDowngradeGuardSettingsHandler("")

	w := doModelDowngradeGuardSettingsRequest(h, http.MethodGet, "")

	require.Equal(t, http.StatusOK, w.Code)
	var body modelDowngradeGuardSettingsBody
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.False(t, body.Data.Enabled, "守卫默认关闭")
	require.Equal(t, service.ModelDowngradeGuardActionModelBlock, body.Data.Action)
	require.Len(t, body.Data.Pairs, 1)
	require.Equal(t, 5, body.Data.ThresholdCount)
	require.Equal(t, 30, body.Data.ThresholdWindowMinutes)
	require.Equal(t, 24, body.Data.BlockHours)
	require.InDelta(t, 0.3, body.Data.MaxBlockedRatio, 1e-9)
}

func TestSettingHandler_UpdateModelDowngradeGuardSettings(t *testing.T) {
	t.Run("persists normalized settings and echoes them back", func(t *testing.T) {
		h, repo := newModelDowngradeGuardSettingsHandler("")

		w := doModelDowngradeGuardSettingsRequest(h, http.MethodPut, `{
			"enabled": true,
			"action": "temp_unsched",
			"pairs": [
				{"sent_model": " gpt-6-astra ", "response_model": " gpt-5.6-luna "},
				{"sent_model": "", "response_model": "ignored"}
			],
			"threshold_count": 8,
			"threshold_window_minutes": 15,
			"block_hours": 6,
			"max_blocked_ratio": 0.2
		}`)

		require.Equal(t, http.StatusOK, w.Code)
		var body modelDowngradeGuardSettingsBody
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		require.True(t, body.Data.Enabled)
		require.Equal(t, service.ModelDowngradeGuardActionTempUnsched, body.Data.Action)
		require.Len(t, body.Data.Pairs, 1)
		require.Equal(t, "gpt-6-astra", body.Data.Pairs[0].SentModel)
		require.Equal(t, "gpt-5.6-luna", body.Data.Pairs[0].ResponseModel)
		require.Equal(t, 8, body.Data.ThresholdCount)
		require.Equal(t, 15, body.Data.ThresholdWindowMinutes)
		require.Equal(t, 6, body.Data.BlockHours)
		require.InDelta(t, 0.2, body.Data.MaxBlockedRatio, 1e-9)

		var stored service.ModelDowngradeGuardSettings
		require.NoError(t, json.Unmarshal([]byte(repo.values[service.SettingKeyModelDowngradeGuardSettings]), &stored))
		require.Equal(t, []service.ModelDowngradePair{{SentModel: "gpt-6-astra", ResponseModel: "gpt-5.6-luna"}}, stored.Pairs)
	})

	t.Run("rejects invalid settings without persisting", func(t *testing.T) {
		h, repo := newModelDowngradeGuardSettingsHandler("")

		w := doModelDowngradeGuardSettingsRequest(h, http.MethodPut, `{
			"enabled": true,
			"action": "model_block",
			"pairs": [],
			"threshold_count": 5,
			"threshold_window_minutes": 30,
			"block_hours": 24,
			"max_blocked_ratio": 0.3
		}`)

		require.Equal(t, http.StatusBadRequest, w.Code)
		require.Contains(t, w.Body.String(), "at least one downgrade pair is required")
		_, written := repo.values[service.SettingKeyModelDowngradeGuardSettings]
		require.False(t, written)
	})
}
