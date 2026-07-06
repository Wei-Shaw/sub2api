package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type settingHandlerRepoStub struct {
	values      map[string]string
	lastUpdates map[string]string
REDACTED

func (s *settingHandlerRepoStub) Get(ctx context.Context, key string) (*service.Setting, error) {
	panic("unexpected Get call")
REDACTED

func (s *settingHandlerRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	if s.values != nil {
		if value, ok := s.values[key]; ok {
			return value, nil
	REDACTED
REDACTED
	return "", nil
REDACTED

func (s *settingHandlerRepoStub) Set(ctx context.Context, key, value string) error {
	panic("unexpected Set call")
REDACTED

func (s *settingHandlerRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
	REDACTED
REDACTED
	return out, nil
REDACTED

func (s *settingHandlerRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	s.lastUpdates = make(map[string]string, len(settings))
	for key, value := range settings {
		s.lastUpdates[key] = value
		if s.values == nil {
			s.values = map[string]string{REDACTED
	REDACTED
		s.values[key] = value
REDACTED
	return nil
REDACTED

func (s *settingHandlerRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	out := make(map[string]string, len(s.values))
	for key, value := range s.values {
		out[key] = value
REDACTED
	return out, nil
REDACTED

func (s *settingHandlerRepoStub) Delete(ctx context.Context, key string) error {
	panic("unexpected Delete call")
REDACTED

type failingAuthSourceSettingsRepoStub struct {
	values map[string]string
	err    error
REDACTED

func (s *failingAuthSourceSettingsRepoStub) Get(ctx context.Context, key string) (*service.Setting, error) {
	panic("unexpected Get call")
REDACTED

func (s *failingAuthSourceSettingsRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	panic("unexpected GetValue call")
REDACTED

func (s *failingAuthSourceSettingsRepoStub) Set(ctx context.Context, key, value string) error {
	panic("unexpected Set call")
REDACTED

func (s *failingAuthSourceSettingsRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
	REDACTED
REDACTED
	return out, nil
REDACTED

func (s *failingAuthSourceSettingsRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	if _, ok := settings[service.SettingKeyAuthSourceDefaultEmailBalance]; ok {
		return s.err
REDACTED
	for key, value := range settings {
		if s.values == nil {
			s.values = map[string]string{REDACTED
	REDACTED
		s.values[key] = value
REDACTED
	return nil
REDACTED

func (s *failingAuthSourceSettingsRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	out := make(map[string]string, len(s.values))
	for key, value := range s.values {
		out[key] = value
REDACTED
	return out, nil
REDACTED

func (s *failingAuthSourceSettingsRepoStub) Delete(ctx context.Context, key string) error {
	panic("unexpected Delete call")
REDACTED

func TestSettingHandler_GetSettings_InjectsAuthSourceDefaults(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &settingHandlerRepoStub{
		values: map[string]string{
			service.SettingKeyRegistrationEnabled:                 "true",
			service.SettingKeyPromoCodeEnabled:                    "true",
			service.SettingKeyAuthSourceDefaultEmailBalance:       "9.5",
			service.SettingKeyAuthSourceDefaultEmailConcurrency:   "8",
			service.SettingKeyAuthSourceDefaultEmailSubscriptions: `[{"group_id":31,"validity_days":15REDACTED]`,
			service.SettingKeyForceEmailOnThirdPartySignup:        "true",
	REDACTED,
REDACTED
	svc := service.NewSettingService(repo, &config.Config{Default: config.DefaultConfig{UserConcurrency: 5REDACTEDREDACTED)
	handler := NewSettingHandler(svc, nil, nil, nil, nil, nil, nil)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings", nil)

	handler.GetSettings(c)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp response.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	data, ok := resp.Data.(map[string]any)
	require.True(t, ok)
	require.Equal(t, 9.5, data["auth_source_default_email_balance"])
	require.Equal(t, float64(8), data["auth_source_default_email_concurrency"])
	require.Equal(t, true, data["force_email_on_third_party_signup"])

	subscriptions, ok := data["auth_source_default_email_subscriptions"].([]any)
	require.True(t, ok)
	require.Len(t, subscriptions, 1)
REDACTED

func TestSettingHandler_UpdateSettings_PreservesOmittedAuthSourceDefaults(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &settingHandlerRepoStub{
		values: map[string]string{
			service.SettingKeyRegistrationEnabled:                    "false",
			service.SettingKeyPromoCodeEnabled:                       "true",
			service.SettingKeyAuthSourceDefaultEmailBalance:          "9.5",
			service.SettingKeyAuthSourceDefaultEmailConcurrency:      "8",
			service.SettingKeyAuthSourceDefaultEmailSubscriptions:    `[{"group_id":31,"validity_days":15REDACTED]`,
			service.SettingKeyAuthSourceDefaultEmailGrantOnSignup:    "true",
			service.SettingKeyAuthSourceDefaultEmailGrantOnFirstBind: "false",
			service.SettingKeyForceEmailOnThirdPartySignup:           "true",
	REDACTED,
REDACTED
	svc := service.NewSettingService(repo, &config.Config{Default: config.DefaultConfig{UserConcurrency: 5REDACTEDREDACTED)
	handler := NewSettingHandler(svc, nil, nil, nil, nil, nil, nil)

	body := map[string]any{
		"registration_enabled":              true,
		"promo_code_enabled":                true,
		"auth_source_default_email_balance": 12.75,
REDACTED
	rawBody, err := json.Marshal(body)
REDACTED

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(rawBody))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateSettings(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "12.75000000", repo.values[service.SettingKeyAuthSourceDefaultEmailBalance])
	require.Equal(t, "8", repo.values[service.SettingKeyAuthSourceDefaultEmailConcurrency])
	require.Equal(t, `[{"group_id":31,"validity_days":15REDACTED]`, repo.values[service.SettingKeyAuthSourceDefaultEmailSubscriptions])
	require.Equal(t, "true", repo.values[service.SettingKeyForceEmailOnThirdPartySignup])

	var resp response.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	data, ok := resp.Data.(map[string]any)
	require.True(t, ok)
	require.Equal(t, 12.75, data["auth_source_default_email_balance"])
	require.Equal(t, float64(8), data["auth_source_default_email_concurrency"])
	require.Equal(t, true, data["force_email_on_third_party_signup"])
REDACTED

func TestSettingHandler_UpdateSettings_PersistsPaymentVisibleMethodsAndAdvancedScheduler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &settingHandlerRepoStub{
		values: map[string]string{
			service.SettingKeyPromoCodeEnabled: "true",
	REDACTED,
REDACTED
	svc := service.NewSettingService(repo, &config.Config{Default: config.DefaultConfig{UserConcurrency: 5REDACTEDREDACTED)
	handler := NewSettingHandler(svc, nil, nil, nil, nil, nil, nil)

	body := map[string]any{
		"promo_code_enabled":                                      true,
		"payment_visible_method_alipay_source":                    "easypay",
		"payment_visible_method_wxpay_source":                     "wxpay",
		"payment_visible_method_alipay_enabled":                   true,
		"payment_visible_method_wxpay_enabled":                    false,
		"openai_advanced_scheduler_enabled":                       true,
		"openai_advanced_scheduler_subscription_priority_enabled": true,
REDACTED
	rawBody, err := json.Marshal(body)
REDACTED

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(rawBody))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateSettings(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, service.VisibleMethodSourceEasyPayAlipay, repo.values[service.SettingPaymentVisibleMethodAlipaySource])
	require.Equal(t, service.VisibleMethodSourceOfficialWechat, repo.values[service.SettingPaymentVisibleMethodWxpaySource])
	require.Equal(t, "true", repo.values[service.SettingPaymentVisibleMethodAlipayEnabled])
	require.Equal(t, "false", repo.values[service.SettingPaymentVisibleMethodWxpayEnabled])
	require.Equal(t, "true", repo.values["openai_advanced_scheduler_enabled"])
	require.Equal(t, "true", repo.values[service.SettingKeyOpenAIAdvancedSchedulerSubscriptionPriorityEnabled])

	var resp response.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	data, ok := resp.Data.(map[string]any)
	require.True(t, ok)
	require.Equal(t, service.VisibleMethodSourceEasyPayAlipay, data["payment_visible_method_alipay_source"])
	require.Equal(t, service.VisibleMethodSourceOfficialWechat, data["payment_visible_method_wxpay_source"])
	require.Equal(t, true, data["payment_visible_method_alipay_enabled"])
	require.Equal(t, false, data["payment_visible_method_wxpay_enabled"])
	require.Equal(t, true, data["openai_advanced_scheduler_enabled"])
	require.Equal(t, true, data["openai_advanced_scheduler_subscription_priority_enabled"])
REDACTED

func TestSettingHandler_UpdateSettings_PreservesLegacyBlankPaymentVisibleMethodSource(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &settingHandlerRepoStub{
		values: map[string]string{
			service.SettingKeyPromoCodeEnabled:               "true",
			service.SettingPaymentVisibleMethodAlipayEnabled: "true",
			service.SettingPaymentVisibleMethodAlipaySource:  "",
			service.SettingPaymentVisibleMethodWxpayEnabled:  "false",
			service.SettingPaymentVisibleMethodWxpaySource:   "",
	REDACTED,
REDACTED
	svc := service.NewSettingService(repo, &config.Config{Default: config.DefaultConfig{UserConcurrency: 5REDACTEDREDACTED)
	handler := NewSettingHandler(svc, nil, nil, nil, nil, nil, nil)

	body := map[string]any{
		"promo_code_enabled": false,
REDACTED
	rawBody, err := json.Marshal(body)
REDACTED

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(rawBody))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateSettings(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "", repo.values[service.SettingPaymentVisibleMethodAlipaySource])
	require.Equal(t, "true", repo.values[service.SettingPaymentVisibleMethodAlipayEnabled])
REDACTED

func TestSettingHandler_UpdateSettings_PersistsExplicitFalseOIDCCompatibilityFlags(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &settingHandlerRepoStub{
		values: map[string]string{
			service.SettingKeyPromoCodeEnabled:               "true",
			service.SettingKeyOIDCConnectEnabled:             "true",
			service.SettingKeyOIDCConnectProviderName:        "OIDC",
			service.SettingKeyOIDCConnectClientID:            "oidc-client",
			service.SettingKeyOIDCConnectClientSecret:        "oidc-secret",
			service.SettingKeyOIDCConnectIssuerURL:           "https://issuer.example.com",
			service.SettingKeyOIDCConnectAuthorizeURL:        "https://issuer.example.com/auth",
			service.SettingKeyOIDCConnectTokenURL:            "https://issuer.example.com/token",
			service.SettingKeyOIDCConnectUserInfoURL:         "https://issuer.example.com/userinfo",
			service.SettingKeyOIDCConnectJWKSURL:             "https://issuer.example.com/jwks",
			service.SettingKeyOIDCConnectScopes:              "openid email profile",
			service.SettingKeyOIDCConnectRedirectURL:         "https://example.com/api/v1/auth/oauth/oidc/callback",
			service.SettingKeyOIDCConnectFrontendRedirectURL: "/auth/oidc/callback",
			service.SettingKeyOIDCConnectTokenAuthMethod:     "client_secret_post",
			service.SettingKeyOIDCConnectUsePKCE:             "true",
			service.SettingKeyOIDCConnectValidateIDToken:     "true",
			service.SettingKeyOIDCConnectAllowedSigningAlgs:  "RS256",
			service.SettingKeyOIDCConnectClockSkewSeconds:    "120",
	REDACTED,
REDACTED
	svc := service.NewSettingService(repo, &config.Config{Default: config.DefaultConfig{UserConcurrency: 5REDACTEDREDACTED)
	handler := NewSettingHandler(svc, nil, nil, nil, nil, nil, nil)

	body := map[string]any{
		"promo_code_enabled":                true,
		"oidc_connect_enabled":              true,
		"oidc_connect_use_pkce":             false,
		"oidc_connect_validate_id_token":    false,
		"oidc_connect_allowed_signing_algs": "",
REDACTED
	rawBody, err := json.Marshal(body)
REDACTED

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(rawBody))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateSettings(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "false", repo.values[service.SettingKeyOIDCConnectUsePKCE])
	require.Equal(t, "false", repo.values[service.SettingKeyOIDCConnectValidateIDToken])

	var resp response.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	data, ok := resp.Data.(map[string]any)
	require.True(t, ok)
	require.Equal(t, false, data["oidc_connect_use_pkce"])
	require.Equal(t, false, data["oidc_connect_validate_id_token"])
REDACTED

func TestSettingHandler_UpdateSettings_DoesNotSolidifyImplicitOIDCSecurityDefaultsOnLegacyUpgrade(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &settingHandlerRepoStub{
		values: map[string]string{
			service.SettingKeyPromoCodeEnabled:                "true",
			service.SettingKeyOIDCConnectEnabled:              "true",
			service.SettingKeyOIDCConnectProviderName:         "OIDC",
			service.SettingKeyOIDCConnectClientID:             "oidc-client",
			service.SettingKeyOIDCConnectClientSecret:         "oidc-secret",
			service.SettingKeyOIDCConnectIssuerURL:            "https://issuer.example.com",
			service.SettingKeyOIDCConnectAuthorizeURL:         "https://issuer.example.com/auth",
			service.SettingKeyOIDCConnectTokenURL:             "https://issuer.example.com/token",
			service.SettingKeyOIDCConnectUserInfoURL:          "https://issuer.example.com/userinfo",
			service.SettingKeyOIDCConnectJWKSURL:              "https://issuer.example.com/jwks",
			service.SettingKeyOIDCConnectScopes:               "openid email profile",
			service.SettingKeyOIDCConnectRedirectURL:          "https://example.com/api/v1/auth/oauth/oidc/callback",
			service.SettingKeyOIDCConnectFrontendRedirectURL:  "/auth/oidc/callback",
			service.SettingKeyOIDCConnectTokenAuthMethod:      "client_secret_post",
			service.SettingKeyOIDCConnectAllowedSigningAlgs:   "RS256",
			service.SettingKeyOIDCConnectClockSkewSeconds:     "120",
			service.SettingKeyOIDCConnectRequireEmailVerified: "false",
			service.SettingKeyOIDCConnectUserInfoEmailPath:    "",
			service.SettingKeyOIDCConnectUserInfoIDPath:       "",
			service.SettingKeyOIDCConnectUserInfoUsernamePath: "",
	REDACTED,
REDACTED
	svc := service.NewSettingService(repo, &config.Config{
		Default: config.DefaultConfig{UserConcurrency: 5REDACTED,
		OIDC: config.OIDCConnectConfig{
			Enabled:             true,
			ProviderName:        "OIDC",
			ClientID:            "oidc-client",
			ClientSecret:        "oidc-secret",
			IssuerURL:           "https://issuer.example.com",
			AuthorizeURL:        "https://issuer.example.com/auth",
			TokenURL:            "https://issuer.example.com/token",
			UserInfoURL:         "https://issuer.example.com/userinfo",
			JWKSURL:             "https://issuer.example.com/jwks",
			Scopes:              "openid email profile",
			RedirectURL:         "https://example.com/api/v1/auth/oauth/oidc/callback",
			FrontendRedirectURL: "/auth/oidc/callback",
			TokenAuthMethod:     "client_secret_post",
			UsePKCE:             true,
			ValidateIDToken:     true,
			AllowedSigningAlgs:  "RS256",
			ClockSkewSeconds:    120,
	REDACTED,
REDACTED)
	handler := NewSettingHandler(svc, nil, nil, nil, nil, nil, nil)

	body := map[string]any{
		"promo_code_enabled":   true,
		"oidc_connect_enabled": true,
REDACTED
	rawBody, err := json.Marshal(body)
REDACTED

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(rawBody))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateSettings(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "false", repo.values[service.SettingKeyOIDCConnectUsePKCE])
	require.Equal(t, "false", repo.values[service.SettingKeyOIDCConnectValidateIDToken])
REDACTED

func TestSettingHandler_UpdateSettings_RejectsInvalidPaymentVisibleMethodSource(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &settingHandlerRepoStub{
		values: map[string]string{
			service.SettingKeyPromoCodeEnabled: "true",
	REDACTED,
REDACTED
	svc := service.NewSettingService(repo, &config.Config{Default: config.DefaultConfig{UserConcurrency: 5REDACTEDREDACTED)
	handler := NewSettingHandler(svc, nil, nil, nil, nil, nil, nil)

	body := map[string]any{
		"promo_code_enabled":                   true,
		"payment_visible_method_alipay_source": "bogus",
REDACTED
	rawBody, err := json.Marshal(body)
REDACTED

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(rawBody))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateSettings(c)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.NotContains(t, repo.values, service.SettingPaymentVisibleMethodAlipaySource)
REDACTED

func TestSettingHandler_UpdateSettings_DoesNotPersistPartialSystemSettingsWhenAuthSourceDefaultsFail(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &failingAuthSourceSettingsRepoStub{
		values: map[string]string{
			service.SettingKeyRegistrationEnabled:                 "false",
			service.SettingKeyPromoCodeEnabled:                    "true",
			service.SettingKeyAuthSourceDefaultEmailBalance:       "9.5",
			service.SettingKeyAuthSourceDefaultEmailConcurrency:   "8",
			service.SettingKeyAuthSourceDefaultEmailSubscriptions: `[{"group_id":31,"validity_days":15REDACTED]`,
	REDACTED,
		err: errors.New("write auth source defaults failed"),
REDACTED
	svc := service.NewSettingService(repo, &config.Config{Default: config.DefaultConfig{UserConcurrency: 5REDACTEDREDACTED)
	handler := NewSettingHandler(svc, nil, nil, nil, nil, nil, nil)

	body := map[string]any{
		"registration_enabled":              true,
		"promo_code_enabled":                true,
		"auth_source_default_email_balance": 12.75,
REDACTED
	rawBody, err := json.Marshal(body)
REDACTED

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(rawBody))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateSettings(c)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Equal(t, "false", repo.values[service.SettingKeyRegistrationEnabled])
	require.Equal(t, "9.5", repo.values[service.SettingKeyAuthSourceDefaultEmailBalance])
REDACTED

func TestDiffSettings_IncludesAuthSourceDefaultsAndForceEmail(t *testing.T) {
	changed := diffSettings(
		&service.SystemSettings{REDACTED,
		&service.SystemSettings{REDACTED,
		&service.AuthSourceDefaultSettings{
			Email: service.ProviderDefaultGrantSettings{
				Balance:          0,
				Concurrency:      5,
				Subscriptions:    nil,
				GrantOnSignup:    true,
				GrantOnFirstBind: false,
		REDACTED,
			ForceEmailOnThirdPartySignup: false,
	REDACTED,
		&service.AuthSourceDefaultSettings{
			Email: service.ProviderDefaultGrantSettings{
				Balance:          12.5,
				Concurrency:      7,
				Subscriptions:    []service.DefaultSubscriptionSetting{{GroupID: 21, ValidityDays: 30REDACTEDREDACTED,
				GrantOnSignup:    false,
				GrantOnFirstBind: true,
		REDACTED,
			ForceEmailOnThirdPartySignup: true,
	REDACTED,
		UpdateSettingsRequest{REDACTED,
	)

	require.Contains(t, changed, "auth_source_default_email_balance")
	require.Contains(t, changed, "auth_source_default_email_concurrency")
	require.Contains(t, changed, "auth_source_default_email_subscriptions")
	require.Contains(t, changed, "auth_source_default_email_grant_on_signup")
	require.Contains(t, changed, "auth_source_default_email_grant_on_first_bind")
	require.Contains(t, changed, "force_email_on_third_party_signup")
REDACTED
