//go:build unit

package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type fastCountTokensSettings struct {
	service.SettingRepository
	policy string
}

func (r *fastCountTokensSettings) GetValue(_ context.Context, key string) (string, error) {
	if key == service.SettingKeyBetaPolicySettings {
		return r.policy, nil
	}
	return "", service.ErrSettingNotFound
}

func (r *fastCountTokensSettings) Get(ctx context.Context, key string) (*service.Setting, error) {
	value, err := r.GetValue(ctx, key)
	return &service.Setting{Key: key, Value: value}, err
}

func (r *fastCountTokensSettings) GetMultiple(context.Context, []string) (map[string]string, error) {
	return map[string]string{}, nil
}

type fastCountTokensSessions struct {
	service.SessionLimitCache
	released  int
	accountID int64
	session   string
}

func (s *fastCountTokensSessions) RegisterSession(context.Context, int64, string, int, time.Duration) (bool, error) {
	return true, nil
}

func (s *fastCountTokensSessions) UnregisterSession(_ context.Context, accountID int64, session string) error {
	s.accountID, s.session = accountID, session
	s.released++
	return nil
}

type fastCountTokensTransport struct {
	service.HTTPUpstream
	calls int
}

func (u *fastCountTokensTransport) DoWithTLS(*http.Request, string, int64, int, *tlsfingerprint.Profile) (*http.Response, error) {
	u.calls++
	return nil, errors.New("synthetic transport failure")
}

func TestAnthropicFastCountTokensPolicyDeniedHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, body, policy string
		denied             bool
	}{
		{"block", `{"model":"claude-opus-5","speed":"fast","messages":[{"role":"user","content":"hello"}]}`, `{"rules":[{"beta_token":"fast-mode-2026-02-01","action":"block","scope":"all","error_message":"custom admin policy"}]}`, true},
		{"duplicate speed", `{"model":"claude-opus-5","speed":"standard","speed":"fast","messages":[{"role":"user","content":"hello"}]}`, `{"rules":[]}`, true},
		{"real transport error still logged", `{"model":"claude-opus-5","messages":[{"role":"user","content":"hello"}]}`, `{"rules":[]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			groupID := int64(77)
			group := &service.Group{ID: groupID, Hydrated: true, Platform: service.PlatformAnthropic, Status: service.StatusActive}
			account := &service.Account{
				ID: 88, Platform: service.PlatformAnthropic, Type: service.AccountTypeOAuth,
				Status: service.StatusActive, Schedulable: true, Concurrency: 1,
				Credentials:   map[string]any{"access_token": "synthetic-token"},
				Extra:         map[string]any{"max_sessions": 1},
				AccountGroups: []service.AccountGroup{{AccountID: 88, GroupID: groupID}},
			}
			cfg := &config.Config{RunMode: config.RunModeSimple}
			settings := service.NewSettingService(&fastCountTokensSettings{policy: tc.policy}, cfg)
			sessions := &fastCountTokensSessions{}
			upstream := &fastCountTokensTransport{}
			snapshot := service.NewSchedulerSnapshotService(&fakeSchedulerCache{accounts: []*service.Account{account}}, nil, nil, nil, nil)
			gateway := service.NewGatewayService(
				nil, &fakeGroupRepo{group: group}, nil, nil, nil, nil, nil, nil, cfg, snapshot,
				nil, nil, nil, nil, nil, upstream, nil, nil, sessions, nil, nil, settings,
				nil, nil, nil, nil, nil, nil,
			)
			billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
			t.Cleanup(billing.Stop)
			h := &GatewayHandler{gatewayService: gateway, billingCacheService: billing, cfg: cfg}
			core, logs := observer.New(zap.DebugLevel)
			ctx := logger.IntoContext(context.WithValue(context.Background(), ctxkey.Group, group), zap.New(core))
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", strings.NewReader(tc.body)).WithContext(ctx)
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{ID: 99, UserID: 100, GroupID: &groupID, Group: group, User: &service.User{ID: 100}})
			c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 100, Concurrency: 1})

			h.CountTokens(c)

			require.Equal(t, account.ID, sessions.accountID)
			require.NotEmpty(t, sessions.session)
			require.Equal(t, 1, sessions.released, "local rejection must still release the selected session")
			errorLogs := logs.FilterMessage("gateway.count_tokens_forward_failed")
			if tc.denied {
				require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
				require.Contains(t, recorder.Body.String(), "invalid_request_error")
				require.Zero(t, upstream.calls)
				require.True(t, service.HasOpsClientBusinessLimited(c))
				require.Equal(t, service.OpsClientBusinessLimitedReasonLocalPolicyDenied, service.OpsClientBusinessLimitedReason(c))
				require.Zero(t, errorLogs.Len())
				_, limited, owner, source := classifyOpsErrorLog(c, "invalid_request_error", "custom admin policy", "", recorder.Code)
				require.True(t, limited)
				require.Equal(t, "client", owner)
				require.Equal(t, "client_request", source)
			} else {
				require.Equal(t, http.StatusBadGateway, recorder.Code)
				require.Equal(t, 1, upstream.calls)
				require.False(t, service.HasOpsClientBusinessLimited(c))
				require.Equal(t, 1, errorLogs.Len())
				require.Equal(t, zap.ErrorLevel, errorLogs.All()[0].Level)
			}
		})
	}
}
