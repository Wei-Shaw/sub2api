//go:build unit

package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// errSettingRepo: a SettingRepository that always returns errors on read
// ---------------------------------------------------------------------------

type errSettingRepo struct {
	mockSettingRepo // embed the existing mock from backup_service_test.go
	readErr         error
}

func (r *errSettingRepo) GetValue(_ context.Context, _ string) (string, error) {
	return "", r.readErr
}

func (r *errSettingRepo) Get(_ context.Context, _ string) (*Setting, error) {
	return nil, r.readErr
}

// ---------------------------------------------------------------------------
// overloadAccountRepoStub: records SetOverloaded calls
// ---------------------------------------------------------------------------

type overloadAccountRepoStub struct {
	mockAccountRepoForGemini
	overloadCalls   int
	lastOverloadID  int64
	lastOverloadEnd time.Time
}

func (r *overloadAccountRepoStub) SetOverloaded(_ context.Context, id int64, until time.Time) error {
	r.overloadCalls++
	r.lastOverloadID = id
	r.lastOverloadEnd = until
	return nil
}

// ===========================================================================
// SettingService: GetOverloadCooldownSettings
// ===========================================================================

func TestGetOverloadCooldownSettings_DefaultsWhenNotSet(t *testing.T) {
	repo := newMockSettingRepo()
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetOverloadCooldownSettings(context.Background())
	require.NoError(t, err)
	require.True(t, settings.Enabled)
	require.Equal(t, 10, settings.CooldownMinutes)
	require.Zero(t, settings.OAuthRetryCount)
}

func TestGetOverloadCooldownSettings_ReadsFromDB(t *testing.T) {
	repo := newMockSettingRepo()
	data, _ := json.Marshal(OverloadCooldownSettings{Enabled: false, CooldownMinutes: 30, OAuthRetryCount: 2})
	repo.data[SettingKeyOverloadCooldownSettings] = string(data)
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetOverloadCooldownSettings(context.Background())
	require.NoError(t, err)
	require.False(t, settings.Enabled)
	require.Equal(t, 30, settings.CooldownMinutes)
	require.Equal(t, 2, settings.OAuthRetryCount)
}

func TestGetOverloadCooldownSettings_ClampsMinValue(t *testing.T) {
	repo := newMockSettingRepo()
	data, _ := json.Marshal(OverloadCooldownSettings{Enabled: true, CooldownMinutes: 0, OAuthRetryCount: -1})
	repo.data[SettingKeyOverloadCooldownSettings] = string(data)
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetOverloadCooldownSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, settings.CooldownMinutes)
	require.Zero(t, settings.OAuthRetryCount)
}

func TestGetOverloadCooldownSettings_ClampsMaxValue(t *testing.T) {
	repo := newMockSettingRepo()
	data, _ := json.Marshal(OverloadCooldownSettings{Enabled: true, CooldownMinutes: 999, OAuthRetryCount: 999})
	repo.data[SettingKeyOverloadCooldownSettings] = string(data)
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetOverloadCooldownSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, 120, settings.CooldownMinutes)
	require.Equal(t, maxOAuth529RetryCount, settings.OAuthRetryCount)
}

func TestGetOverloadCooldownSettings_InvalidJSON_ReturnsDefaults(t *testing.T) {
	repo := newMockSettingRepo()
	repo.data[SettingKeyOverloadCooldownSettings] = "not-json"
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetOverloadCooldownSettings(context.Background())
	require.NoError(t, err)
	require.True(t, settings.Enabled)
	require.Equal(t, 10, settings.CooldownMinutes)
}

func TestGetOverloadCooldownSettings_EmptyValue_ReturnsDefaults(t *testing.T) {
	repo := newMockSettingRepo()
	repo.data[SettingKeyOverloadCooldownSettings] = ""
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetOverloadCooldownSettings(context.Background())
	require.NoError(t, err)
	require.True(t, settings.Enabled)
	require.Equal(t, 10, settings.CooldownMinutes)
}

// ===========================================================================
// SettingService: SetOverloadCooldownSettings
// ===========================================================================

func TestSetOverloadCooldownSettings_Success(t *testing.T) {
	repo := newMockSettingRepo()
	svc := NewSettingService(repo, &config.Config{})

	err := svc.SetOverloadCooldownSettings(context.Background(), &OverloadCooldownSettings{
		Enabled:         false,
		CooldownMinutes: 25,
		OAuthRetryCount: 2,
	})
	require.NoError(t, err)

	// Verify round-trip
	settings, err := svc.GetOverloadCooldownSettings(context.Background())
	require.NoError(t, err)
	require.False(t, settings.Enabled)
	require.Equal(t, 25, settings.CooldownMinutes)
	require.Equal(t, 2, settings.OAuthRetryCount)
}

func TestSetOverloadCooldownSettings_RejectsNil(t *testing.T) {
	svc := NewSettingService(newMockSettingRepo(), &config.Config{})
	err := svc.SetOverloadCooldownSettings(context.Background(), nil)
	require.Error(t, err)
}

func TestSetOverloadCooldownSettings_EnabledRejectsOutOfRange(t *testing.T) {
	svc := NewSettingService(newMockSettingRepo(), &config.Config{})

	for _, minutes := range []int{0, -1, 121, 999} {
		err := svc.SetOverloadCooldownSettings(context.Background(), &OverloadCooldownSettings{
			Enabled: true, CooldownMinutes: minutes,
		})
		require.Error(t, err, "should reject enabled=true + cooldown_minutes=%d", minutes)
		require.Contains(t, err.Error(), "cooldown_minutes must be between 1-120")
	}
}

func TestSetOverloadCooldownSettings_RejectsOAuthRetryCountOutOfRange(t *testing.T) {
	svc := NewSettingService(newMockSettingRepo(), &config.Config{})

	for _, retryCount := range []int{-1, maxOAuth529RetryCount + 1, 999} {
		err := svc.SetOverloadCooldownSettings(context.Background(), &OverloadCooldownSettings{
			Enabled: true, CooldownMinutes: 10, OAuthRetryCount: retryCount,
		})
		require.Error(t, err, "should reject oauth_retry_count=%d", retryCount)
		require.Contains(t, err.Error(), "oauth_retry_count must be between 0-2")
	}
}

func TestSetOverloadCooldownSettings_DisabledNormalizesOutOfRange(t *testing.T) {
	repo := newMockSettingRepo()
	svc := NewSettingService(repo, &config.Config{})

	// enabled=false + cooldown_minutes=0 应该保存成功，值被归一化为10
	err := svc.SetOverloadCooldownSettings(context.Background(), &OverloadCooldownSettings{
		Enabled: false, CooldownMinutes: 0,
	})
	require.NoError(t, err, "disabled with invalid minutes should NOT be rejected")

	// 验证持久化后读回来的值
	settings, err := svc.GetOverloadCooldownSettings(context.Background())
	require.NoError(t, err)
	require.False(t, settings.Enabled)
	require.Equal(t, 10, settings.CooldownMinutes, "should be normalized to default")
}

func TestSetOverloadCooldownSettings_AcceptsBoundaries(t *testing.T) {
	svc := NewSettingService(newMockSettingRepo(), &config.Config{})

	for _, minutes := range []int{1, 60, 120} {
		err := svc.SetOverloadCooldownSettings(context.Background(), &OverloadCooldownSettings{
			Enabled: true, CooldownMinutes: minutes,
		})
		require.NoError(t, err, "should accept cooldown_minutes=%d", minutes)
	}
}

// ===========================================================================
// RateLimitService: handle529 behaviour
// ===========================================================================

func TestHandle529_EnabledFromDB_PausesAccount(t *testing.T) {
	accountRepo := &overloadAccountRepoStub{}
	settingRepo := newMockSettingRepo()
	data, _ := json.Marshal(OverloadCooldownSettings{Enabled: true, CooldownMinutes: 15})
	settingRepo.data[SettingKeyOverloadCooldownSettings] = string(data)

	settingSvc := NewSettingService(settingRepo, &config.Config{})
	svc := NewRateLimitService(accountRepo, nil, &config.Config{}, nil, nil)
	svc.SetSettingService(settingSvc)

	account := &Account{ID: 42, Platform: PlatformAnthropic, Type: AccountTypeOAuth}
	before := time.Now()
	svc.handle529(context.Background(), account)

	require.Equal(t, 1, accountRepo.overloadCalls)
	require.Equal(t, int64(42), accountRepo.lastOverloadID)
	require.WithinDuration(t, before.Add(15*time.Minute), accountRepo.lastOverloadEnd, 2*time.Second)
}

func TestHandle529_DisabledFromDB_SkipsAccount(t *testing.T) {
	accountRepo := &overloadAccountRepoStub{}
	settingRepo := newMockSettingRepo()
	data, _ := json.Marshal(OverloadCooldownSettings{Enabled: false, CooldownMinutes: 15})
	settingRepo.data[SettingKeyOverloadCooldownSettings] = string(data)

	settingSvc := NewSettingService(settingRepo, &config.Config{})
	svc := NewRateLimitService(accountRepo, nil, &config.Config{}, nil, nil)
	svc.SetSettingService(settingSvc)

	account := &Account{ID: 42, Platform: PlatformAnthropic, Type: AccountTypeOAuth}
	svc.handle529(context.Background(), account)

	require.Equal(t, 0, accountRepo.overloadCalls, "should NOT pause when disabled")
}

func TestHandle529_NilSettingService_FallsBackToConfig(t *testing.T) {
	accountRepo := &overloadAccountRepoStub{}
	cfg := &config.Config{}
	cfg.RateLimit.OverloadCooldownMinutes = 20
	svc := NewRateLimitService(accountRepo, nil, cfg, nil, nil)
	// NOT calling SetSettingService — remains nil

	account := &Account{ID: 77, Platform: PlatformAnthropic, Type: AccountTypeOAuth}
	before := time.Now()
	svc.handle529(context.Background(), account)

	require.Equal(t, 1, accountRepo.overloadCalls)
	require.WithinDuration(t, before.Add(20*time.Minute), accountRepo.lastOverloadEnd, 2*time.Second)
}

func TestHandle529_NilSettingService_ZeroConfig_DefaultsTen(t *testing.T) {
	accountRepo := &overloadAccountRepoStub{}
	svc := NewRateLimitService(accountRepo, nil, &config.Config{}, nil, nil)

	account := &Account{ID: 88, Platform: PlatformAnthropic, Type: AccountTypeOAuth}
	before := time.Now()
	svc.handle529(context.Background(), account)

	require.Equal(t, 1, accountRepo.overloadCalls)
	require.WithinDuration(t, before.Add(10*time.Minute), accountRepo.lastOverloadEnd, 2*time.Second)
}

func TestHandle529_DBReadError_FallsBackToConfig(t *testing.T) {
	accountRepo := &overloadAccountRepoStub{}
	errRepo := &errSettingRepo{readErr: context.DeadlineExceeded}
	errRepo.data = make(map[string]string)

	cfg := &config.Config{}
	cfg.RateLimit.OverloadCooldownMinutes = 7
	settingSvc := NewSettingService(errRepo, cfg)
	svc := NewRateLimitService(accountRepo, nil, cfg, nil, nil)
	svc.SetSettingService(settingSvc)

	account := &Account{ID: 99, Platform: PlatformAnthropic, Type: AccountTypeOAuth}
	before := time.Now()
	svc.handle529(context.Background(), account)

	require.Equal(t, 1, accountRepo.overloadCalls)
	require.WithinDuration(t, before.Add(7*time.Minute), accountRepo.lastOverloadEnd, 2*time.Second)
}

// ===========================================================================
// Model: defaults & JSON round-trip
// ===========================================================================

func TestDefaultOverloadCooldownSettings(t *testing.T) {
	d := DefaultOverloadCooldownSettings()
	require.True(t, d.Enabled)
	require.Equal(t, 10, d.CooldownMinutes)
	require.Zero(t, d.OAuthRetryCount)
}

func TestOverloadCooldownSettings_JSONRoundTrip(t *testing.T) {
	original := OverloadCooldownSettings{Enabled: false, CooldownMinutes: 42, OAuthRetryCount: 2}
	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded OverloadCooldownSettings
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Equal(t, original, decoded)

	// Verify JSON uses snake_case field names
	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))
	_, hasEnabled := raw["enabled"]
	_, hasCooldown := raw["cooldown_minutes"]
	_, hasOAuthRetryCount := raw["oauth_retry_count"]
	require.True(t, hasEnabled, "JSON must use 'enabled'")
	require.True(t, hasCooldown, "JSON must use 'cooldown_minutes'")
	require.True(t, hasOAuthRetryCount, "JSON must use 'oauth_retry_count'")
}

type oauth529HTTPUpstreamStub struct {
	statusCodes []int
	bodies      [][]byte
}

func (s *oauth529HTTPUpstreamStub) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	return s.DoWithTLS(req, proxyURL, accountID, accountConcurrency, nil)
}

func (s *oauth529HTTPUpstreamStub) DoWithTLS(req *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	s.bodies = append(s.bodies, body)

	index := len(s.bodies) - 1
	statusCode := s.statusCodes[index]
	responseBody := `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`
	if statusCode == http.StatusOK {
		responseBody = `{"id":"msg_test","type":"message","model":"claude-sonnet-4-5","usage":{"input_tokens":1,"output_tokens":1},"content":[]}`
	}

	return &http.Response{
		StatusCode: statusCode,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
			"x-request-id": []string{"req-test"},
		},
		Body: io.NopCloser(strings.NewReader(responseBody)),
	}, nil
}

func newOAuth529ForwardTest(t *testing.T, retryCount int, statusCodes ...int) (*GatewayService, *gin.Context, *Account, *ParsedRequest, *oauth529HTTPUpstreamStub, *overloadAccountRepoStub) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}
	settingRepo := newMockSettingRepo()
	data, err := json.Marshal(OverloadCooldownSettings{
		Enabled:         true,
		CooldownMinutes: 10,
		OAuthRetryCount: retryCount,
	})
	require.NoError(t, err)
	settingRepo.data[SettingKeyOverloadCooldownSettings] = string(data)
	settingSvc := NewSettingService(settingRepo, cfg)

	accountRepo := &overloadAccountRepoStub{}
	rateLimitSvc := &RateLimitService{
		accountRepo:    accountRepo,
		cfg:            cfg,
		settingService: settingSvc,
	}
	upstream := &oauth529HTTPUpstreamStub{statusCodes: statusCodes}
	svc := &GatewayService{
		cfg:                  cfg,
		httpUpstream:         upstream,
		rateLimitService:     rateLimitSvc,
		settingService:       settingSvc,
		tlsFPProfileService:  &TLSFingerprintProfileService{},
		responseHeaderFilter: compileResponseHeaderFilter(cfg),
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request.Header.Set("User-Agent", "claude-cli/2.1.78")

	body := []byte(`{"model":"claude-sonnet-4-5","stream":false,"max_tokens":32,"metadata":{"user_id":"session_123e4567-e89b-12d3-a456-426614174000"},"messages":[{"role":"user","content":"hello"}]}`)
	parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
	require.NoError(t, err)
	account := &Account{
		ID:          529,
		Name:        "oauth-529-test",
		Platform:    PlatformAnthropic,
		Type:        AccountTypeSetupToken,
		Concurrency: 1,
		Credentials: map[string]any{"access_token": "test-token"},
		Status:      StatusActive,
		Schedulable: true,
	}

	return svc, c, account, parsed, upstream, accountRepo
}

func TestGatewayForwardOAuth529_DefaultDoesNotRetry(t *testing.T) {
	svc, c, account, parsed, upstream, accountRepo := newOAuth529ForwardTest(t, 0, 529, http.StatusOK)

	result, err := svc.Forward(context.Background(), c, account, parsed)

	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, 529, failoverErr.StatusCode)
	require.Len(t, upstream.bodies, 1)
	require.Equal(t, 1, accountRepo.overloadCalls)
}

func TestGatewayForwardOAuth529_RetriesConfiguredCountAndReplaysBody(t *testing.T) {
	svc, c, account, parsed, upstream, accountRepo := newOAuth529ForwardTest(t, 2, 529, 529, http.StatusOK)

	result, err := svc.Forward(context.Background(), c, account, parsed)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.bodies, 3)
	require.Equal(t, upstream.bodies[0], upstream.bodies[1])
	require.Equal(t, upstream.bodies[0], upstream.bodies[2])
	require.Zero(t, accountRepo.overloadCalls)
}

func TestGatewayForwardOAuth529_ExhaustionStillAppliesCooldownAndFailover(t *testing.T) {
	svc, c, account, parsed, upstream, accountRepo := newOAuth529ForwardTest(t, 2, 529, 529, 529, http.StatusOK)

	result, err := svc.Forward(context.Background(), c, account, parsed)

	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, 529, failoverErr.StatusCode)
	require.Len(t, upstream.bodies, 3)
	require.Equal(t, upstream.bodies[0], upstream.bodies[1])
	require.Equal(t, upstream.bodies[0], upstream.bodies[2])
	require.Equal(t, 1, accountRepo.overloadCalls)
}

func TestGatewayOAuth529RetrySettingIsAnthropicSubscriptionOnly(t *testing.T) {
	settingRepo := newMockSettingRepo()
	data, err := json.Marshal(OverloadCooldownSettings{Enabled: true, CooldownMinutes: 10, OAuthRetryCount: 2})
	require.NoError(t, err)
	settingRepo.data[SettingKeyOverloadCooldownSettings] = string(data)
	svc := &GatewayService{settingService: NewSettingService(settingRepo, &config.Config{})}

	require.Equal(t, 2, svc.getOAuth529RetryCount(context.Background(), &Account{Platform: PlatformAnthropic, Type: AccountTypeOAuth}))
	require.Zero(t, svc.getOAuth529RetryCount(context.Background(), &Account{Platform: PlatformAnthropic, Type: AccountTypeAPIKey}))
	require.Zero(t, svc.getOAuth529RetryCount(context.Background(), &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}))
	require.True(t, svc.shouldRetryUpstreamError(&Account{Platform: PlatformAnthropic, Type: AccountTypeOAuth}, http.StatusForbidden))
	require.False(t, svc.shouldRetryUpstreamError(&Account{Platform: PlatformAnthropic, Type: AccountTypeAPIKey}, 529))
}
