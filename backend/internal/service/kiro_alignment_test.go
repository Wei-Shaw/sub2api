package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kirocooldown"
	"github.com/stretchr/testify/require"
)

type kiroUsageCooldownStore struct {
	state *kirocooldown.State
	err   error
}

func (s *kiroUsageCooldownStore) ReserveRequest(context.Context, string) (time.Duration, error) {
	return 0, nil
}

func (s *kiroUsageCooldownStore) MarkSuccess(context.Context, string) error {
	return nil
}

func (s *kiroUsageCooldownStore) Mark429(context.Context, string) (time.Duration, error) {
	return 0, nil
}

func (s *kiroUsageCooldownStore) MarkSuspended(context.Context, string) (time.Duration, error) {
	return 0, nil
}

func (s *kiroUsageCooldownStore) GetState(context.Context, string) (*kirocooldown.State, error) {
	return s.state, s.err
}

func (s *kiroUsageCooldownStore) ClearEarliestTransientCooldown(context.Context, []string) (bool, error) {
	return false, nil
}

type kiroUsageTokenProviderStub struct {
	getToken     string
	getErr       error
	refreshToken string
	refreshErr   error
	getCalls     int
	refreshCalls int
}

func (s *kiroUsageTokenProviderStub) GetAccessToken(context.Context, *Account) (string, error) {
	s.getCalls++
	return s.getToken, s.getErr
}

func (s *kiroUsageTokenProviderStub) ForceRefreshAccessToken(context.Context, *Account) (string, error) {
	s.refreshCalls++
	return s.refreshToken, s.refreshErr
}

func kiroFloatPtr(v float64) *float64 {
	return &v
}

func kiroUsageTestBody(current float64) string {
	return `{
		"subscriptionInfo": {"subscriptionTitle":"KIRO PRO+"},
		"usageBreakdownList": [{
			"currency":"USD",
			"currentUsageWithPrecision":` + strconv.FormatFloat(current, 'f', -1, 64) + `,
			"usageLimitWithPrecision":2000,
			"resourceType":"CREDIT"
		}]
	}`
}

func setKiroUsageTestEndpoint(t *testing.T, endpoint string) {
	t.Helper()
	prevResolver := resolveKiroRuntimeEndpoint
	resolveKiroRuntimeEndpoint = func(_ string) string { return endpoint }
	t.Cleanup(func() { resolveKiroRuntimeEndpoint = prevResolver })
}

func TestChannel_IsWebSearchEmulationEnabled_Kiro(t *testing.T) {
	c := &Channel{
		FeaturesConfig: map[string]any{
			featureKeyWebSearchEmulation: map[string]any{"kiro": true},
		},
	}

	require.True(t, c.IsWebSearchEmulationEnabled("kiro"))
}

func TestOpenAIGatewayServiceRecordUsage_NormalizesKiroBillingModel(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	userRepo := &openAIRecordUsageUserRepoStub{}
	subRepo := &openAIRecordUsageSubRepoStub{}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, nil)
	svc.billingService = NewBillingService(svc.cfg, &PricingService{
		pricingData: map[string]*LiteLLMModelPricing{
			"claude-sonnet-4-6": {
				InputCostPerToken:  2.5e-6,
				OutputCostPerToken: 10e-6,
			},
		},
	})

	expectedCost, err := svc.billingService.CalculateCost("claude-sonnet-4-6", UsageTokens{
		InputTokens:  20,
		OutputTokens: 10,
	}, 1.1)
	require.NoError(t, err)

	err = svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID:     "resp_kiro_billing_normalized",
			Model:         "claude-sonnet-4-6",
			UpstreamModel: "claude-sonnet-4.6",
			Usage: OpenAIUsage{
				InputTokens:  20,
				OutputTokens: 10,
			},
			Duration: time.Second,
		},
		APIKey:  &APIKey{ID: 10},
		User:    &User{ID: 20},
		Account: &Account{ID: 30, Platform: PlatformKiro},
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, "claude-sonnet-4-6", usageRepo.lastLog.Model)
	require.Equal(t, "claude-sonnet-4-6", usageRepo.lastLog.RequestedModel)
	require.NotNil(t, usageRepo.lastLog.UpstreamModel)
	require.Equal(t, "claude-sonnet-4.6", *usageRepo.lastLog.UpstreamModel)
	require.InDelta(t, expectedCost.ActualCost, usageRepo.lastLog.ActualCost, 1e-12)
	require.InDelta(t, expectedCost.TotalCost, usageRepo.lastLog.TotalCost, 1e-12)
}

func TestAccountUsageService_GetUsage_KiroMapsCredits(t *testing.T) {
	account := Account{
		ID:       701,
		Platform: PlatformKiro,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "kiro-access-token",
			"provider":     "Github",
			"auth_method":  "social",
			"profile_arn":  "arn:aws:codewhisperer:us-east-1:123456789012:profile/SOCIAL",
		},
	}
	repo := &stubOpenAIAccountRepo{accounts: []Account{account}}
	svc := NewAccountUsageService(repo, nil, nil, nil, nil, NewUsageCache(), nil, nil)

	resetAt := time.Now().Add(10 * 24 * time.Hour).Unix()
	bonusExpiry := time.Now().Add(7 * 24 * time.Hour).Unix()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/getUsageLimits", r.URL.Path)
		require.Equal(t, "arn:aws:codewhisperer:us-east-1:123456789012:profile/SOCIAL", r.URL.Query().Get("profileArn"))
		require.Equal(t, kiroUsageOrigin, r.URL.Query().Get("origin"))
		require.Equal(t, kiroUsageResourceType, r.URL.Query().Get("resourceType"))
		require.Equal(t, "Bearer kiro-access-token", r.Header.Get("Authorization"))
		require.Equal(t, "*/*", r.Header.Get("Accept"))
		require.True(t, strings.Contains(r.Header.Get("User-Agent"), "KiroIDE-"))
		require.True(t, strings.Contains(r.Header.Get("X-Amz-User-Agent"), "KiroIDE-"))
		require.Equal(t, "vibe", r.Header.Get("x-amzn-kiro-agent-mode"))
		require.Equal(t, "true", r.Header.Get("x-amzn-codewhisperer-optout"))
		require.NotEmpty(t, r.Header.Get("Amz-Sdk-Invocation-Id"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"nextDateReset": ` + strconv.FormatInt(resetAt, 10) + `,
			"overageConfiguration": {"overageStatus":"ENABLED"},
			"subscriptionInfo": {"subscriptionTitle":"KIRO PRO+","type":"Q_DEVELOPER_STANDALONE_PRO_PLUS"},
			"usageBreakdownList": [{
				"currency":"USD",
				"currentOveragesWithPrecision":2,
				"currentUsageWithPrecision":125,
				"freeTrialInfo":{
					"currentUsageWithPrecision":25,
					"freeTrialExpiry":` + strconv.FormatInt(bonusExpiry, 10) + `,
					"freeTrialStatus":"ACTIVE",
					"usageLimitWithPrecision":500
				},
				"nextDateReset": ` + strconv.FormatInt(resetAt, 10) + `,
				"overageCharges":0.08,
				"resourceType":"CREDIT",
				"usageLimitWithPrecision":2000
			}]
		}`))
	}))
	defer server.Close()

	prevResolver := resolveKiroRuntimeEndpoint
	resolveKiroRuntimeEndpoint = func(_ string) string { return server.URL }
	defer func() { resolveKiroRuntimeEndpoint = prevResolver }()

	usage, err := svc.GetUsage(context.Background(), account.ID)
	require.NoError(t, err)
	require.NotNil(t, usage)
	require.Equal(t, "active", usage.Source)
	require.Equal(t, "KIRO PRO+", usage.KiroSubscriptionName)
	require.Equal(t, "Q_DEVELOPER_STANDALONE_PRO_PLUS", usage.KiroSubscriptionType)
	require.True(t, usage.KiroOveragesEnabled)
	require.NotNil(t, usage.KiroCredit)
	require.Equal(t, 125.0, usage.KiroCredit.CurrentUsage)
	require.Equal(t, 2000.0, usage.KiroCredit.UsageLimit)
	require.InDelta(t, 6.25, usage.KiroCredit.PercentageUsed, 0.001)
	require.NotNil(t, usage.KiroBonus)
	require.Equal(t, 25.0, usage.KiroBonus.CurrentUsage)
	require.Equal(t, 500.0, usage.KiroBonus.UsageLimit)
	require.NotNil(t, usage.KiroOverage)
	require.Equal(t, "$", usage.KiroOverage.CurrencySymbol)
	require.Equal(t, 2.0, usage.KiroOverage.CurrentOverages)
	require.Equal(t, 0.08, usage.KiroOverage.OverageCharges)
	require.NotNil(t, usage.KiroResetAt)
	require.Equal(t, kiroQuotaStateOverageActive, usage.KiroQuotaState)
	require.Equal(t, "overages_enabled", usage.KiroQuotaReason)
	require.NotNil(t, usage.KiroQuotaResetAt)
}

func TestAccountUsageService_GetUsage_KiroUsesTokenProviderAccessToken(t *testing.T) {
	account := Account{
		ID:       710,
		Platform: PlatformKiro,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "stale-access-token",
			"provider":     "Github",
			"auth_method":  "social",
		},
	}
	repo := &stubOpenAIAccountRepo{accounts: []Account{account}}
	provider := &kiroUsageTokenProviderStub{getToken: "provider-access-token"}
	svc := NewAccountUsageService(repo, nil, nil, nil, nil, NewUsageCache(), nil, nil).
		SetKiroTokenProvider(provider)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer provider-access-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(kiroUsageTestBody(15)))
	}))
	defer server.Close()
	setKiroUsageTestEndpoint(t, server.URL)

	usage, err := svc.GetUsage(context.Background(), account.ID)
	require.NoError(t, err)
	require.NotNil(t, usage)
	require.NotNil(t, usage.KiroCredit)
	require.Equal(t, 15.0, usage.KiroCredit.CurrentUsage)
	require.Equal(t, 1, provider.getCalls)
	require.Equal(t, 0, provider.refreshCalls)
}

func TestAccountUsageService_GetUsage_KiroRefreshesAndRetriesOn401(t *testing.T) {
	account := Account{
		ID:       711,
		Platform: PlatformKiro,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "expired-access-token",
			"provider":     "Github",
			"auth_method":  "social",
		},
	}
	repo := &stubOpenAIAccountRepo{accounts: []Account{account}}
	provider := &kiroUsageTokenProviderStub{
		getToken:     "expired-access-token",
		refreshToken: "refreshed-access-token",
	}
	svc := NewAccountUsageService(repo, nil, nil, nil, nil, NewUsageCache(), nil, nil).
		SetKiroTokenProvider(provider)

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			require.Equal(t, "Bearer expired-access-token", r.Header.Get("Authorization"))
			http.Error(w, `{"message":"Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		require.Equal(t, "Bearer refreshed-access-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(kiroUsageTestBody(17)))
	}))
	defer server.Close()
	setKiroUsageTestEndpoint(t, server.URL)

	usage, err := svc.GetUsage(context.Background(), account.ID)
	require.NoError(t, err)
	require.NotNil(t, usage)
	require.NotNil(t, usage.KiroCredit)
	require.Equal(t, 17.0, usage.KiroCredit.CurrentUsage)
	require.Equal(t, 2, attempts)
	require.Equal(t, 1, provider.getCalls)
	require.Equal(t, 1, provider.refreshCalls)
}

func TestAccountUsageService_GetUsage_KiroRefreshesAndRetriesOn403TokenError(t *testing.T) {
	account := Account{
		ID:       712,
		Platform: PlatformKiro,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "expired-access-token",
			"provider":     "Github",
			"auth_method":  "social",
		},
	}
	repo := &stubOpenAIAccountRepo{accounts: []Account{account}}
	provider := &kiroUsageTokenProviderStub{
		getToken:     "expired-access-token",
		refreshToken: "refreshed-access-token",
	}
	svc := NewAccountUsageService(repo, nil, nil, nil, nil, NewUsageCache(), nil, nil).
		SetKiroTokenProvider(provider)

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			require.Equal(t, "Bearer expired-access-token", r.Header.Get("Authorization"))
			http.Error(w, `{"message":"token expired"}`, http.StatusForbidden)
			return
		}
		require.Equal(t, "Bearer refreshed-access-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(kiroUsageTestBody(19)))
	}))
	defer server.Close()
	setKiroUsageTestEndpoint(t, server.URL)

	usage, err := svc.GetUsage(context.Background(), account.ID)
	require.NoError(t, err)
	require.NotNil(t, usage)
	require.NotNil(t, usage.KiroCredit)
	require.Equal(t, 19.0, usage.KiroCredit.CurrentUsage)
	require.Equal(t, 2, attempts)
	require.Equal(t, 1, provider.getCalls)
	require.Equal(t, 1, provider.refreshCalls)
}

func TestAccountUsageService_GetUsage_KiroDoesNotRefreshOrdinary403(t *testing.T) {
	account := Account{
		ID:       713,
		Platform: PlatformKiro,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "valid-access-token",
			"provider":     "Github",
			"auth_method":  "social",
		},
	}
	repo := &stubOpenAIAccountRepo{accounts: []Account{account}}
	provider := &kiroUsageTokenProviderStub{
		getToken:     "valid-access-token",
		refreshToken: "unused-access-token",
	}
	svc := NewAccountUsageService(repo, nil, nil, nil, nil, NewUsageCache(), nil, nil).
		SetKiroTokenProvider(provider)

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		require.Equal(t, "Bearer valid-access-token", r.Header.Get("Authorization"))
		http.Error(w, `{"message":"usage forbidden"}`, http.StatusForbidden)
	}))
	defer server.Close()
	setKiroUsageTestEndpoint(t, server.URL)

	usage, err := svc.GetUsage(context.Background(), account.ID)
	require.NoError(t, err)
	require.NotNil(t, usage)
	require.Equal(t, errorCodeForbidden, usage.ErrorCode)
	require.Contains(t, usage.Error, "status 403")
	require.Equal(t, 1, attempts)
	require.Equal(t, 1, provider.getCalls)
	require.Equal(t, 0, provider.refreshCalls)
}

func TestAccountUsageService_GetUsage_KiroActiveUsesCachedSnapshotWithinTTL(t *testing.T) {
	account := Account{
		ID:       702,
		Platform: PlatformKiro,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "kiro-access-token",
			"provider":     "Github",
			"auth_method":  "social",
		},
	}
	repo := &stubOpenAIAccountRepo{accounts: []Account{account}}
	svc := NewAccountUsageService(repo, nil, nil, nil, nil, NewUsageCache(), nil, nil)

	successServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"subscriptionInfo": {"subscriptionTitle":"KIRO PRO+"},
			"usageBreakdownList": [{
				"currency":"USD",
				"currentUsageWithPrecision":300,
				"usageLimitWithPrecision":2000,
				"resourceType":"CREDIT"
			}]
		}`))
	}))
	defer successServer.Close()

	prevResolver := resolveKiroRuntimeEndpoint
	resolveKiroRuntimeEndpoint = func(_ string) string { return successServer.URL }
	defer func() { resolveKiroRuntimeEndpoint = prevResolver }()

	firstUsage, err := svc.GetUsage(context.Background(), account.ID)
	require.NoError(t, err)
	require.NotNil(t, firstUsage)
	require.NotNil(t, firstUsage.KiroCredit)
	require.Equal(t, 300.0, firstUsage.KiroCredit.CurrentUsage)

	failingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"temporary failure"}`, http.StatusInternalServerError)
	}))
	defer failingServer.Close()
	resolveKiroRuntimeEndpoint = func(_ string) string { return failingServer.URL }

	usage, err := svc.GetUsage(context.Background(), account.ID)
	require.NoError(t, err)
	require.NotNil(t, usage)
	require.NotNil(t, usage.KiroCredit)
	require.Equal(t, 300.0, usage.KiroCredit.CurrentUsage)
	require.Empty(t, usage.Error)
	require.Empty(t, usage.ErrorCode)
}

func TestAccountUsageService_GetUsage_KiroBuilderIDWithoutProfileArnOmitsProfileArn(t *testing.T) {
	account := Account{
		ID:       703,
		Platform: PlatformKiro,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "kiro-access-token",
			"provider":     "BuilderId",
			"auth_method":  "idc",
			"region":       "us-east-1",
		},
	}
	repo := &stubOpenAIAccountRepo{accounts: []Account{account}}
	svc := NewAccountUsageService(repo, nil, nil, nil, nil, NewUsageCache(), nil, nil)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/getUsageLimits", r.URL.Path)
		require.Empty(t, r.URL.Query().Get("profileArn"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"subscriptionInfo": {"subscriptionTitle":"KIRO PRO+"},
			"usageBreakdownList": [{
				"currency":"USD",
				"currentUsageWithPrecision":42,
				"usageLimitWithPrecision":2000,
				"resourceType":"CREDIT"
			}]
		}`))
	}))
	defer server.Close()

	prevResolver := resolveKiroRuntimeEndpoint
	resolveKiroRuntimeEndpoint = func(_ string) string { return server.URL }
	defer func() { resolveKiroRuntimeEndpoint = prevResolver }()

	usage, err := svc.GetUsage(context.Background(), account.ID)
	require.NoError(t, err)
	require.NotNil(t, usage)
	require.NotNil(t, usage.KiroCredit)
	require.Equal(t, 42.0, usage.KiroCredit.CurrentUsage)
}

func TestAccountUsageService_GetUsage_KiroEnterpriseUsesCredentialProfileArn(t *testing.T) {
	account := Account{
		ID:       707,
		Platform: PlatformKiro,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "kiro-access-token",
			"provider":     "AWS",
			"auth_method":  "idc",
			"region":       "us-east-1",
			"start_url":    "https://d-example.awsapps.com/start",
			"profile_arn":  "arn:aws:codewhisperer:us-east-1:123456789012:profile/REALENTERPRISE",
		},
	}
	repo := &stubOpenAIAccountRepo{accounts: []Account{account}}
	svc := NewAccountUsageService(repo, nil, nil, nil, nil, NewUsageCache(), nil, nil)

	const resolvedProfileArn = "arn:aws:codewhisperer:us-east-1:123456789012:profile/REALENTERPRISE"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/getUsageLimits", r.URL.Path)
		require.Equal(t, resolvedProfileArn, r.URL.Query().Get("profileArn"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"subscriptionInfo": {"subscriptionTitle":"KIRO PRO+"},
			"usageBreakdownList": [{
				"currency":"USD",
				"currentUsageWithPrecision":64,
				"usageLimitWithPrecision":2000,
				"resourceType":"CREDIT"
			}]
		}`))
	}))
	defer server.Close()

	prevResolver := resolveKiroRuntimeEndpoint
	resolveKiroRuntimeEndpoint = func(_ string) string { return server.URL }
	defer func() { resolveKiroRuntimeEndpoint = prevResolver }()

	usage, err := svc.GetUsage(context.Background(), account.ID)
	require.NoError(t, err)
	require.NotNil(t, usage)
	require.NotNil(t, usage.KiroCredit)
	require.Equal(t, 64.0, usage.KiroCredit.CurrentUsage)
}

func TestAccountUsageService_GetUsage_KiroUsesAPIRegionForUsageRequest(t *testing.T) {
	account := Account{
		ID:       709,
		Platform: PlatformKiro,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "kiro-access-token",
			"provider":     "AWS",
			"auth_method":  "idc",
			"api_region":   "eu-west-1",
			"region":       "ap-northeast-2",
			"start_url":    "https://d-example.awsapps.com/start",
			"profile_arn":  "arn:aws:codewhisperer:eu-west-1:123456789012:profile/REALAPIREGION",
		},
	}
	repo := &stubOpenAIAccountRepo{accounts: []Account{account}}
	svc := NewAccountUsageService(repo, nil, nil, nil, nil, NewUsageCache(), nil, nil)

	const resolvedProfileArn = "arn:aws:codewhisperer:eu-west-1:123456789012:profile/REALAPIREGION"
	gotRegions := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/getUsageLimits", r.URL.Path)
		require.Equal(t, resolvedProfileArn, r.URL.Query().Get("profileArn"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"subscriptionInfo": {"subscriptionTitle":"KIRO PRO+"},
			"usageBreakdownList": [{
				"currency":"USD",
				"currentUsageWithPrecision":11,
				"usageLimitWithPrecision":2000,
				"resourceType":"CREDIT"
			}]
		}`))
	}))
	defer server.Close()

	prevResolver := resolveKiroRuntimeEndpoint
	resolveKiroRuntimeEndpoint = func(region string) string {
		gotRegions = append(gotRegions, region)
		return server.URL
	}
	defer func() { resolveKiroRuntimeEndpoint = prevResolver }()

	usage, err := svc.GetUsage(context.Background(), account.ID)
	require.NoError(t, err)
	require.NotNil(t, usage)
	require.Equal(t, []string{"eu-west-1"}, gotRegions)
}

func TestAccountUsageService_GetUsage_KiroOmitsProfileArnAndUsesDefaultRegionWithoutAPIRegionOrProfileArn(t *testing.T) {
	account := Account{
		ID:       710,
		Platform: PlatformKiro,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "kiro-access-token",
			"provider":     "AWS",
			"auth_method":  "idc",
			"region":       "ap-northeast-2",
			"start_url":    "https://d-example.awsapps.com/start",
		},
	}
	repo := &stubOpenAIAccountRepo{accounts: []Account{account}}
	svc := NewAccountUsageService(repo, nil, nil, nil, nil, NewUsageCache(), nil, nil)

	gotRegions := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/getUsageLimits", r.URL.Path)
		require.Empty(t, r.URL.Query().Get("profileArn"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"subscriptionInfo": {"subscriptionTitle":"KIRO PRO+"},
			"usageBreakdownList": [{
				"currency":"USD",
				"currentUsageWithPrecision":7,
				"usageLimitWithPrecision":2000,
				"resourceType":"CREDIT"
			}]
		}`))
	}))
	defer server.Close()

	prevResolver := resolveKiroRuntimeEndpoint
	resolveKiroRuntimeEndpoint = func(region string) string {
		gotRegions = append(gotRegions, region)
		return server.URL
	}
	defer func() { resolveKiroRuntimeEndpoint = prevResolver }()

	usage, err := svc.GetUsage(context.Background(), account.ID)
	require.NoError(t, err)
	require.NotNil(t, usage)
	require.Equal(t, []string{kiroDefaultRegion}, gotRegions)
}

func TestAccountUsageService_GetUsage_KiroIncludesRuntimeCooldownState(t *testing.T) {
	account := Account{
		ID:       704,
		Platform: PlatformKiro,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "kiro-access-token",
			"provider":     "Github",
			"auth_method":  "social",
		},
	}
	repo := &stubOpenAIAccountRepo{accounts: []Account{account}}
	svc := NewAccountUsageService(repo, nil, nil, nil, nil, NewUsageCache(), nil, nil).
		SetKiroCooldownStore(&kiroUsageCooldownStore{
			state: &kirocooldown.State{
				Active:        true,
				Reason:        kirocooldown.CooldownReason429,
				CooldownUntil: time.Now().Add(90 * time.Second),
				Remaining:     90 * time.Second,
			},
		})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"subscriptionInfo": {"subscriptionTitle":"KIRO PRO+"},
			"usageBreakdownList": [{
				"currency":"USD",
				"currentUsageWithPrecision":42,
				"usageLimitWithPrecision":2000,
				"resourceType":"CREDIT"
			}]
		}`))
	}))
	defer server.Close()

	prevResolver := resolveKiroRuntimeEndpoint
	resolveKiroRuntimeEndpoint = func(_ string) string { return server.URL }
	defer func() { resolveKiroRuntimeEndpoint = prevResolver }()

	usage, err := svc.GetUsage(context.Background(), account.ID)
	require.NoError(t, err)
	require.Equal(t, "cooldown", usage.KiroRuntimeState)
	require.Equal(t, kirocooldown.CooldownReason429, usage.KiroRuntimeReason)
	require.NotNil(t, usage.KiroRuntimeResetAt)
}

func TestBuildKiroDegradedUsage_ClassifiesProfileError(t *testing.T) {
	info := buildKiroDegradedUsage(&kiroUsageHTTPError{
		StatusCode: http.StatusBadRequest,
		Body:       `{"message":"profileArn is required for this request."}`,
	})

	require.Equal(t, errorCodeForbidden, info.ErrorCode)
	require.False(t, info.NeedsReauth)
}

func TestBuildKiroDegradedUsage_ClassifiesOverageExhausted(t *testing.T) {
	info := buildKiroDegradedUsage(&kiroUsageHTTPError{
		StatusCode: http.StatusTooManyRequests,
		Body:       `{"message":"overage exhausted for this billing window"}`,
	})

	require.Equal(t, errorCodeNetworkError, info.ErrorCode)
	require.Equal(t, kiroQuotaStateOverageExhausted, info.KiroQuotaState)
	require.Contains(t, info.KiroQuotaReason, "overage exhausted")
}

func TestAccountUsageService_GetUsage_KiroCachesErrorSnapshotWhenRefreshFailsWithoutPriorSuccess(t *testing.T) {
	account := Account{
		ID:       708,
		Platform: PlatformKiro,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "kiro-access-token",
			"provider":     "Github",
			"auth_method":  "social",
		},
	}
	repo := &stubOpenAIAccountRepo{accounts: []Account{account}}
	svc := NewAccountUsageService(repo, nil, nil, nil, nil, NewUsageCache(), nil, nil)

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		http.Error(w, `{"message":"FEATURE_NOT_SUPPORTED","reason":"FEATURE_NOT_SUPPORTED"}`, http.StatusForbidden)
	}))
	defer server.Close()

	prevResolver := resolveKiroRuntimeEndpoint
	resolveKiroRuntimeEndpoint = func(_ string) string { return server.URL }
	defer func() { resolveKiroRuntimeEndpoint = prevResolver }()

	firstUsage, err := svc.GetUsage(context.Background(), account.ID)
	require.NoError(t, err)
	require.NotNil(t, firstUsage)
	require.Equal(t, errorCodeForbidden, firstUsage.ErrorCode)

	secondUsage, err := svc.GetUsage(context.Background(), account.ID)
	require.NoError(t, err)
	require.NotNil(t, secondUsage)
	require.Equal(t, errorCodeForbidden, secondUsage.ErrorCode)
	require.Equal(t, 1, requestCount)
}

func TestMapKiroUsageToInfo_CreditsExhaustedWithoutOverages(t *testing.T) {
	info := mapKiroUsageToInfo(&kiroUsageLimitsResponse{
		NextDateReset: "2099-03-13T12:00:00Z",
		OverageConfiguration: kiroOverageConfiguration{
			OverageStatus: "DISABLED",
		},
		UsageBreakdownList: []kiroUsageBreakdown{
			{
				ResourceType:                 "CREDIT",
				CurrentUsageWithPrecision:    kiroFloatPtr(2000),
				UsageLimitWithPrecision:      kiroFloatPtr(2000),
				CurrentOveragesWithPrecision: kiroFloatPtr(0),
			},
		},
	})

	require.Equal(t, kiroQuotaStateCreditsExhausted, info.KiroQuotaState)
	require.Equal(t, "credits_exhausted", info.KiroQuotaReason)
	require.NotNil(t, info.KiroQuotaResetAt)
}

func TestAccountUsageService_EnrichAccountWithKiroRuntimeState(t *testing.T) {
	svc := NewAccountUsageService(nil, nil, nil, nil, nil, NewUsageCache(), nil, nil).
		SetKiroCooldownStore(&kiroUsageCooldownStore{
			state: &kirocooldown.State{
				Active:        true,
				Reason:        kirocooldown.CooldownReason429,
				CooldownUntil: time.Now().Add(2 * time.Minute),
				Remaining:     2 * time.Minute,
			},
		})

	account := &Account{
		ID:          705,
		Platform:    PlatformKiro,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "kiro-access-token"},
	}

	svc.EnrichAccountWithKiroRuntimeState(context.Background(), account)
	require.Equal(t, "cooldown", account.KiroRuntimeState)
	require.Equal(t, kirocooldown.CooldownReason429, account.KiroRuntimeReason)
	require.NotNil(t, account.KiroRuntimeResetAt)
}

func TestAccountUsageService_EnrichAccountWithKiroRuntimeStateIncludesCachedQuotaState(t *testing.T) {
	account := Account{
		ID:       706,
		Platform: PlatformKiro,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "kiro-access-token",
			"provider":     "Github",
			"auth_method":  "social",
		},
	}
	repo := &stubOpenAIAccountRepo{accounts: []Account{account}}
	svc := NewAccountUsageService(repo, nil, nil, nil, nil, NewUsageCache(), nil, nil)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"nextDateReset":"2099-03-13T12:00:00Z",
			"overageConfiguration":{"overageStatus":"ENABLED"},
			"subscriptionInfo": {"subscriptionTitle":"KIRO PRO+"},
			"usageBreakdownList": [{
				"currency":"USD",
				"currentUsageWithPrecision":2000,
				"currentOveragesWithPrecision":4,
				"overageCharges":0.2,
				"usageLimitWithPrecision":2000,
				"resourceType":"CREDIT"
			}]
		}`))
	}))
	defer server.Close()

	prevResolver := resolveKiroRuntimeEndpoint
	resolveKiroRuntimeEndpoint = func(_ string) string { return server.URL }
	defer func() { resolveKiroRuntimeEndpoint = prevResolver }()

	_, err := svc.GetUsage(context.Background(), account.ID)
	require.NoError(t, err)

	target := &Account{
		ID:          account.ID,
		Platform:    PlatformKiro,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "kiro-access-token"},
	}
	svc.EnrichAccountWithKiroRuntimeState(context.Background(), target)

	require.Equal(t, kiroQuotaStateOverageActive, target.KiroQuotaState)
	require.Equal(t, "overages_enabled", target.KiroQuotaReason)
	require.NotNil(t, target.KiroQuotaResetAt)
}
