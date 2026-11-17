package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type grokMediaEligibilityAdminServiceStub struct {
	*stubAdminService
	account *service.Account
}

func (s *grokMediaEligibilityAdminServiceStub) GetAccount(_ context.Context, _ int64) (*service.Account, error) {
	return s.account, nil
}

func (s *grokMediaEligibilityAdminServiceStub) UpdateAccountExtra(_ context.Context, _ int64, updates map[string]any) error {
	if s.account.Extra == nil {
		s.account.Extra = map[string]any{}
	}
	for key, value := range updates {
		if value == nil {
			delete(s.account.Extra, key)
		} else {
			s.account.Extra[key] = value
		}
	}
	return nil
}

func newGrokMediaEligibilityRouter(account *service.Account) *gin.Engine {
	stub := &grokMediaEligibilityAdminServiceStub{
		stubAdminService: newStubAdminService(),
		account:          account,
	}
	h := NewAccountHandler(stub, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.GET("/accounts/:id/grok-media-eligibility", h.GetGrokMediaEligibility)
	router.PUT("/accounts/:id/grok-media-eligibility", h.UpdateGrokMediaEligibility)
	return router
}

func TestGrokMediaEligibilityHandlerGetReturnsModeAndReason(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newGrokMediaEligibilityRouter(&service.Account{
		ID:       12,
		Platform: service.PlatformGrok,
		Type:     service.AccountTypeOAuth,
		Extra: map[string]any{
			"grok_billing_snapshot": map[string]any{"status_code": 200, "partial": true},
			"unrelated_setting":     "preserve",
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/accounts/12/grok-media-eligibility", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var envelope struct {
		Data grokMediaEligibilityResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	require.Equal(t, int64(12), envelope.Data.AccountID)
	require.Equal(t, grokMediaEligibilityModeAuto, envelope.Data.Mode)
	require.Equal(t, "billing_inconclusive", envelope.Data.Reason)
}

func TestGrokMediaEligibilityHandlerUpdateModesPreservesExtra(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := &service.Account{
		ID:       12,
		Platform: service.PlatformGrok,
		Type:     service.AccountTypeOAuth,
		Extra:    map[string]any{"unrelated_setting": "preserve"},
	}
	router := newGrokMediaEligibilityRouter(account)

	tests := []struct {
		mode         string
		wantMode     string
		wantExtra    any
		wantEligible bool
		wantReason   string
	}{
		{mode: "enabled", wantMode: grokMediaEligibilityModeEnabled, wantExtra: true, wantEligible: true, wantReason: "override_enabled"},
		{mode: "disabled", wantMode: grokMediaEligibilityModeDisabled, wantExtra: false, wantEligible: false, wantReason: "override_disabled"},
		{mode: "auto", wantMode: grokMediaEligibilityModeAuto, wantExtra: nil, wantEligible: false, wantReason: "billing_unobserved"},
	}

	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			body, err := json.Marshal(map[string]string{"mode": tt.mode})
			require.NoError(t, err)
			req := httptest.NewRequest(http.MethodPut, "/accounts/12/grok-media-eligibility", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			var envelope struct {
				Data grokMediaEligibilityResponse `json:"data"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
			require.Equal(t, tt.wantMode, envelope.Data.Mode)
			require.Equal(t, tt.wantEligible, envelope.Data.Eligible)
			require.Equal(t, tt.wantReason, envelope.Data.Reason)
			require.Equal(t, "preserve", account.Extra["unrelated_setting"])
			if tt.wantExtra == nil {
				_, exists := account.Extra[service.GrokMediaEligibleExtraKey]
				require.False(t, exists)
			} else {
				require.Equal(t, tt.wantExtra, account.Extra[service.GrokMediaEligibleExtraKey])
			}
		})
	}
}

func TestGrokMediaEligibilityHandlerRejectsUnsupportedAndInvalidModes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tt := range []struct {
		name    string
		account *service.Account
		body    string
	}{
		{name: "unsupported account", account: &service.Account{ID: 1, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth}, body: `{"mode":"enabled"}`},
		{name: "invalid mode", account: &service.Account{ID: 1, Platform: service.PlatformGrok, Type: service.AccountTypeOAuth}, body: `{"mode":"force"}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			router := newGrokMediaEligibilityRouter(tt.account)
			req := httptest.NewRequest(http.MethodPut, "/accounts/1/grok-media-eligibility", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			require.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}
