//go:build unit

package server_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAPIContracts(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		setup      func(t *testing.T, deps *contractDeps)
		method     string
		path       string
		body       string
		headers    map[string]string
		wantStatus int
		wantJSON   string
	}{
		{
			name:       "GET /api/v1/auth/me",
			method:     http.MethodGet,
			path:       "/api/v1/auth/me",
			wantStatus: http.StatusOK,
			wantJSON: `{
				"code": 0,
				"message": "success",
				"data": {
					"id": 1,
					"email": "alice@example.com",
					"email_bound": true,
					"username": "alice",
					"role": "user",
					"balance": 12.5,
					"concurrency": 5,
					"rpm_limit": 0,
					"status": "active",
					"allowed_groups": null,
					"created_at": "2025-01-02T03:04:05Z",
					"updated_at": "2025-01-02T03:04:05Z",
					"balance_notify_enabled": false,
					"balance_notify_threshold_type": "",
					"balance_notify_threshold": null,
					"balance_notify_extra_emails": null,
					"total_recharged": 0,
					"linuxdo_bound": false,
					"oidc_bound": false,
					"wechat_bound": false,
					"identities": {
						"email": {
							"provider": "email",
							"provider_key": "email",
							"bound": true,
							"bound_count": 1,
							"can_bind": false,
							"can_unbind": false,
							"display_name": "alice@example.com",
							"subject_hint": "a***e@example.com",
							"note_key": "profile.authBindings.notes.emailManagedFromProfile",
							"note": "Primary account email is managed from the profile form."
						},
						"linuxdo": {
							"provider": "linuxdo",
							"bound": false,
							"bound_count": 0,
							"can_bind": true,
							"can_unbind": false,
							"bind_start_path": "/api/v1/auth/oauth/linuxdo/bind/start?intent=bind_current_user&redirect=%2Fsettings%2Fprofile"
						},
						"oidc": {
							"provider": "oidc",
							"bound": false,
							"bound_count": 0,
							"can_bind": true,
							"can_unbind": false,
							"bind_start_path": "/api/v1/auth/oauth/oidc/bind/start?intent=bind_current_user&redirect=%2Fsettings%2Fprofile"
						},
						"wechat": {
							"provider": "wechat",
							"bound": false,
							"bound_count": 0,
							"can_bind": true,
							"can_unbind": false,
							"bind_start_path": "/api/v1/auth/oauth/wechat/bind/start?intent=bind_current_user&redirect=%2Fsettings%2Fprofile"
						}
					},
					"identity_bindings": {
						"email": {
							"provider": "email",
							"provider_key": "email",
							"bound": true,
							"bound_count": 1,
							"can_bind": false,
							"can_unbind": false,
							"display_name": "alice@example.com",
							"subject_hint": "a***e@example.com",
							"note_key": "profile.authBindings.notes.emailManagedFromProfile",
							"note": "Primary account email is managed from the profile form."
						},
						"linuxdo": {
							"provider": "linuxdo",
							"bound": false,
							"bound_count": 0,
							"can_bind": true,
							"can_unbind": false,
							"bind_start_path": "/api/v1/auth/oauth/linuxdo/bind/start?intent=bind_current_user&redirect=%2Fsettings%2Fprofile"
						},
						"oidc": {
							"provider": "oidc",
							"bound": false,
							"bound_count": 0,
							"can_bind": true,
							"can_unbind": false,
							"bind_start_path": "/api/v1/auth/oauth/oidc/bind/start?intent=bind_current_user&redirect=%2Fsettings%2Fprofile"
						},
						"wechat": {
							"provider": "wechat",
							"bound": false,
							"bound_count": 0,
							"can_bind": true,
							"can_unbind": false,
							"bind_start_path": "/api/v1/auth/oauth/wechat/bind/start?intent=bind_current_user&redirect=%2Fsettings%2Fprofile"
						}
					},
					"auth_bindings": {
						"email": {
							"provider": "email",
							"provider_key": "email",
							"bound": true,
							"bound_count": 1,
							"can_bind": false,
							"can_unbind": false,
							"display_name": "alice@example.com",
							"subject_hint": "a***e@example.com",
							"note_key": "profile.authBindings.notes.emailManagedFromProfile",
							"note": "Primary account email is managed from the profile form."
						},
						"linuxdo": {
							"provider": "linuxdo",
							"bound": false,
							"bound_count": 0,
							"can_bind": true,
							"can_unbind": false,
							"bind_start_path": "/api/v1/auth/oauth/linuxdo/bind/start?intent=bind_current_user&redirect=%2Fsettings%2Fprofile"
						},
						"oidc": {
							"provider": "oidc",
							"bound": false,
							"bound_count": 0,
							"can_bind": true,
							"can_unbind": false,
							"bind_start_path": "/api/v1/auth/oauth/oidc/bind/start?intent=bind_current_user&redirect=%2Fsettings%2Fprofile"
						},
						"wechat": {
							"provider": "wechat",
							"bound": false,
							"bound_count": 0,
							"can_bind": true,
							"can_unbind": false,
							"bind_start_path": "/api/v1/auth/oauth/wechat/bind/start?intent=bind_current_user&redirect=%2Fsettings%2Fprofile"
						}
					},
					"run_mode": "standard"
				}
			}`,
		},
		{
			name:   "POST /api/v1/keys",
			method: http.MethodPost,
			path:   "/api/v1/keys",
			body:   `{"name":"Key One","custom_key":"sk_custom_1234567890"}`,
			headers: map[string]string{
				"Content-Type": "application/json",
			},
			wantStatus: http.StatusOK,
			wantJSON: `{
				"code": 0,
				"message": "success",
				"data": {
					"id": 100,
					"user_id": 1,
					"key": "sk_custom_1234567890",
					"name": "Key One",
					"group_id": null,
					"status": "active",
					"ip_whitelist": null,
					"ip_blacklist": null,
					"last_used_at": null,
					"quota": 0,
					"quota_used": 0,
					"rate_limit_5h": 0,
					"rate_limit_1d": 0,
					"rate_limit_7d": 0,
					"usage_5h": 0,
					"usage_1d": 0,
					"usage_7d": 0,
					"window_5h_start": null,
					"window_1d_start": null,
					"window_7d_start": null,
					"expires_at": null,
					"created_at": "2025-01-02T03:04:05Z",
					"updated_at": "2025-01-02T03:04:05Z"
				}
			}`,
		},
		{
			name: "GET /api/v1/keys (paginated)",
			setup: func(t *testing.T, deps *contractDeps) {
				t.Helper()
				deps.apiKeyRepo.MustSeed(&service.APIKey{
					ID:        100,
					UserID:    1,
					Key:       "sk_custom_1234567890",
					Name:      "Key One",
					Status:    service.StatusActive,
					CreatedAt: deps.now,
					UpdatedAt: deps.now,
				})
			},
			method:     http.MethodGet,
			path:       "/api/v1/keys?page=1&page_size=10",
			wantStatus: http.StatusOK,
			wantJSON: `{
				"code": 0,
				"message": "success",
				"data": {
					"items": [
						{
							"id": 100,
							"user_id": 1,
							"key": "sk_custom_1234567890",
							"name": "Key One",
							"group_id": null,
							"status": "active",
							"ip_whitelist": null,
							"ip_blacklist": null,
							"last_used_at": null,
							"quota": 0,
							"quota_used": 0,
							"rate_limit_5h": 0,
							"rate_limit_1d": 0,
							"rate_limit_7d": 0,
							"usage_5h": 0,
							"usage_1d": 0,
							"usage_7d": 0,
							"window_5h_start": null,
							"window_1d_start": null,
							"window_7d_start": null,
							"expires_at": null,
							"created_at": "2025-01-02T03:04:05Z",
							"updated_at": "2025-01-02T03:04:05Z"
						}
					],
					"total": 1,
					"page": 1,
					"page_size": 10,
					"pages": 1
				}
			}`,
		},
		{
			name: "GET /api/v1/groups/available",
			setup: func(t *testing.T, deps *contractDeps) {
				t.Helper()
				// 普通用户可见的分组列表不应包含内部字段（如 model_routing/account_count）。
				deps.groupRepo.SetActive([]service.Group{
					{
						ID:                  10,
						Name:                "Group One",
						Description:         "desc",
						Platform:            service.PlatformAnthropic,
						RateMultiplier:      1.5,
						IsExclusive:         false,
						Status:              service.StatusActive,
						SubscriptionType:    service.SubscriptionTypeStandard,
						ModelRoutingEnabled: true,
						ModelRouting: map[string][]int64{
							"claude-3-*": []int64{101, 102},
						},
						AccountCount: 2,
						CreatedAt:    deps.now,
						UpdatedAt:    deps.now,
					},
				})
				deps.userSubRepo.SetActiveByUserID(1, nil)
			},
			method:     http.MethodGet,
			path:       "/api/v1/groups/available",
			wantStatus: http.StatusOK,
			wantJSON: `{
				"code": 0,
				"message": "success",
				"data": [
					{
						"id": 10,
						"name": "Group One",
						"description": "desc",
						"platform": "anthropic",
						"rate_multiplier": 1.5,
						"is_exclusive": false,
						"status": "active",
						"subscription_type": "standard",
						"daily_limit_usd": null,
						"weekly_limit_usd": null,
						"monthly_limit_usd": null,
						"image_price_1k": null,
						"image_price_2k": null,
						"image_price_4k": null,
						"allow_image_generation": false,
						"image_rate_independent": false,
						"image_rate_multiplier": 0,
						"claude_code_only": false,
						"allow_messages_dispatch": false,
						"fallback_group_id": null,
						"fallback_group_id_on_invalid_request": null,
						"require_oauth_only": false,
						"require_privacy_set": false,
						"rpm_limit": 0,
						"created_at": "2025-01-02T03:04:05Z",
						"updated_at": "2025-01-02T03:04:05Z"
					}
				]
			}`,
		},
		{
			name: "GET /api/v1/subscriptions",
			setup: func(t *testing.T, deps *contractDeps) {
				t.Helper()
				// 普通用户订阅接口不应包含 assigned_* / notes 等管理员字段。
				deps.userSubRepo.SetByUserID(1, []service.UserSubscription{
					{
						ID:              501,
						UserID:          1,
						GroupID:         10,
						StartsAt:        deps.now,
						ExpiresAt:       time.Date(2099, 1, 2, 3, 4, 5, 0, time.UTC), // 使用未来日期避免 normalizeSubscriptionStatus 标记为过期
						Status:          service.SubscriptionStatusActive,
						DailyUsageUSD:   1.23,
						WeeklyUsageUSD:  2.34,
						MonthlyUsageUSD: 3.45,
						AssignedBy:      ptr(int64(999)),
						AssignedAt:      deps.now,
						Notes:           "admin-note",
						CreatedAt:       deps.now,
						UpdatedAt:       deps.now,
					},
				})
			},
			method:     http.MethodGet,
			path:       "/api/v1/subscriptions",
			wantStatus: http.StatusOK,
			wantJSON: `{
				"code": 0,
				"message": "success",
				"data": [
					{
						"id": 501,
						"user_id": 1,
						"group_id": 10,
						"starts_at": "2025-01-02T03:04:05Z",
						"expires_at": "2099-01-02T03:04:05Z",
						"status": "active",
						"daily_window_start": null,
						"weekly_window_start": null,
						"monthly_window_start": null,
						"daily_usage_usd": 1.23,
						"weekly_usage_usd": 2.34,
						"monthly_usage_usd": 3.45,
						"created_at": "2025-01-02T03:04:05Z",
						"updated_at": "2025-01-02T03:04:05Z"
					}
				]
			}`,
		},
		{
			name: "GET /api/v1/redeem/history",
			setup: func(t *testing.T, deps *contractDeps) {
				t.Helper()
				// 普通用户兑换历史不应包含 notes 等内部字段。
				deps.redeemRepo.SetByUser(1, []service.RedeemCode{
					{
						ID:        900,
						Code:      "CODE-123",
						Type:      service.RedeemTypeBalance,
						Value:     1.25,
						Status:    service.StatusUsed,
						UsedBy:    ptr(int64(1)),
						UsedAt:    ptr(deps.now),
						Notes:     "internal-note",
						CreatedAt: deps.now,
					},
				})
			},
			method:     http.MethodGet,
			path:       "/api/v1/redeem/history",
			wantStatus: http.StatusOK,
			wantJSON: `{
				"code": 0,
				"message": "success",
				"data": [
					{
						"id": 900,
						"code": "CODE-123",
						"type": "balance",
						"value": 1.25,
						"status": "used",
						"used_by": 1,
						"used_at": "2025-01-02T03:04:05Z",
						"created_at": "2025-01-02T03:04:05Z",
						"group_id": null,
						"validity_days": 0
					}
				]
			}`,
		},
		{
			name: "GET /api/v1/usage/stats",
			setup: func(t *testing.T, deps *contractDeps) {
				t.Helper()
				deps.usageRepo.SetUserLogs(1, []service.UsageLog{
					{
						ID:                  1,
						UserID:              1,
						APIKeyID:            100,
						AccountID:           200,
						Model:               "claude-3",
						InputTokens:         10,
						OutputTokens:        20,
						CacheCreationTokens: 1,
						CacheReadTokens:     2,
						TotalCost:           0.5,
						ActualCost:          0.5,
						DurationMs:          ptr(100),
						CreatedAt:           deps.now,
					},
					{
						ID:           2,
						UserID:       1,
						APIKeyID:     100,
						AccountID:    200,
						Model:        "claude-3",
						InputTokens:  5,
						OutputTokens: 15,
						TotalCost:    0.25,
						ActualCost:   0.25,
						DurationMs:   ptr(300),
						CreatedAt:    deps.now,
					},
				})
			},
			method:     http.MethodGet,
			path:       "/api/v1/usage/stats?start_date=2025-01-01&end_date=2025-01-02",
			wantStatus: http.StatusOK,
			wantJSON: `{
				"code": 0,
				"message": "success",
				"data": {
					"total_requests": 2,
					"total_input_tokens": 15,
					"total_output_tokens": 35,
					"total_cache_tokens": 3,
					"total_tokens": 53,
					"total_cost": 0.75,
					"total_actual_cost": 0.75,
					"average_duration_ms": 200
				}
			}`,
		},
		{
			name: "GET /api/v1/usage (paginated)",
			setup: func(t *testing.T, deps *contractDeps) {
				t.Helper()
				deps.usageRepo.SetUserLogs(1, []service.UsageLog{
					{
						ID:                    1,
						UserID:                1,
						APIKeyID:              100,
						AccountID:             200,
						AccountRateMultiplier: ptr(0.5),
						RequestID:             "req_123",
						Model:                 "claude-3",
						InputTokens:           10,
						OutputTokens:          20,
						CacheCreationTokens:   1,
						CacheReadTokens:       2,
						TotalCost:             0.5,
						ActualCost:            0.5,
						RateMultiplier:        1,
						BillingType:           service.BillingTypeBalance,
						Stream:                true,
						DurationMs:            ptr(100),
						FirstTokenMs:          ptr(50),
						CreatedAt:             deps.now,
					},
				})
			},
			method:     http.MethodGet,
			path:       "/api/v1/usage?page=1&page_size=10",
			wantStatus: http.StatusOK,
			wantJSON: `{
				"code": 0,
				"message": "success",
				"data": {
					"items": [
						{
							"id": 1,
							"user_id": 1,
							"api_key_id": 100,
							"account_id": 200,
								"request_id": "req_123",
								"model": "claude-3",
								"request_type": "stream",
								"openai_ws_mode": false,
								"group_id": null,
								"subscription_id": null,
							"input_tokens": 10,
							"output_tokens": 20,
							"cache_creation_tokens": 1,
							"cache_read_tokens": 2,
							"cache_creation_5m_tokens": 0,
							"cache_creation_1h_tokens": 0,
							"input_cost": 0,
							"output_cost": 0,
							"cache_creation_cost": 0,
							"cache_read_cost": 0,
						"total_cost": 0.5,
						"actual_cost": 0.5,
						"rate_multiplier": 1,
						"billing_type": 0,
							"stream": true,
							"duration_ms": 100,
							"first_token_ms": 50,
							"image_count": 0,
							"image_size": null,
							"media_type": null,
							"cache_ttl_overridden": false,
							"created_at": "2025-01-02T03:04:05Z",
							"user_agent": null
						}
					],
					"total": 1,
					"page": 1,
					"page_size": 10,
					"pages": 1
				}
			}`,
		},
		{
			name: "GET /api/v1/admin/settings",
			setup: func(t *testing.T, deps *contractDeps) {
				t.Helper()
				deps.settingRepo.SetAll(map[string]string{
					service.SettingKeyRegistrationEnabled:              "true",
					service.SettingKeyEmailVerifyEnabled:               "false",
					service.SettingKeyRegistrationEmailSuffixWhitelist: "[]",
					service.SettingKeyPromoCodeEnabled:                 "true",

					service.SettingKeySMTPHost:     "smtp.example.com",
					service.SettingKeySMTPPort:     "587",
					service.SettingKeySMTPUsername: "user",
					service.SettingKeySMTPPassword: "secret",
					service.SettingKeySMTPFrom:     "no-reply@example.com",
					service.SettingKeySMTPFromName: "Sub2API",
					service.SettingKeySMTPUseTLS:   "true",

					service.SettingKeyTurnstileEnabled:   "true",
					service.SettingKeyTurnstileSiteKey:   "site-key",
					service.SettingKeyTurnstileSecretKey: "secret-key",

					service.SettingKeyOIDCConnectEnabled:              "false",
					service.SettingKeyOIDCConnectProviderName:         "OIDC",
					service.SettingKeyOIDCConnectClientID:             "",
					service.SettingKeyOIDCConnectIssuerURL:            "",
					service.SettingKeyOIDCConnectDiscoveryURL:         "",
					service.SettingKeyOIDCConnectAuthorizeURL:         "",
					service.SettingKeyOIDCConnectTokenURL:             "",
					service.SettingKeyOIDCConnectUserInfoURL:          "",
					service.SettingKeyOIDCConnectJWKSURL:              "",
					service.SettingKeyOIDCConnectScopes:               "openid email profile",
					service.SettingKeyOIDCConnectRedirectURL:          "",
					service.SettingKeyOIDCConnectFrontendRedirectURL:  "/auth/oidc/callback",
					service.SettingKeyOIDCConnectTokenAuthMethod:      "client_secret_post",
					service.SettingKeyOIDCConnectUsePKCE:              "true",
					service.SettingKeyOIDCConnectValidateIDToken:      "true",
					service.SettingKeyOIDCConnectAllowedSigningAlgs:   "RS256,ES256,PS256",
					service.SettingKeyOIDCConnectClockSkewSeconds:     "120",
					service.SettingKeyOIDCConnectRequireEmailVerified: "false",
					service.SettingKeyOIDCConnectUserInfoEmailPath:    "",
					service.SettingKeyOIDCConnectUserInfoIDPath:       "",
					service.SettingKeyOIDCConnectUserInfoUsernamePath: "",

					service.SettingKeySiteName:     "Sub2API",
					service.SettingKeySiteLogo:     "",
					service.SettingKeySiteSubtitle: "Subtitle",
					service.SettingKeyAPIBaseURL:   "https://api.example.com",
					service.SettingKeyContactInfo:  "support",
					service.SettingKeyDocURL:       "https://docs.example.com",

					service.SettingKeyDefaultConcurrency:   "5",
					service.SettingKeyDefaultBalance:       "1.25",
					service.SettingKeyTableDefaultPageSize: "20",
					service.SettingKeyTablePageSizeOptions: "[10,20,50,100]",

					service.SettingKeyOpsMonitoringEnabled:           "false",
					service.SettingKeyOpsRealtimeMonitoringEnabled:   "true",
					service.SettingKeyOpsQueryModeDefault:            "auto",
					service.SettingKeyOpsMetricsIntervalSeconds:      "60",
					service.SettingPaymentVisibleMethodAlipaySource:  service.VisibleMethodSourceEasyPayAlipay,
					service.SettingPaymentVisibleMethodWxpaySource:   service.VisibleMethodSourceOfficialWechat,
					service.SettingPaymentVisibleMethodAlipayEnabled: "true",
					service.SettingPaymentVisibleMethodWxpayEnabled:  "false",
					"openai_advanced_scheduler_enabled":              "true",
				})
			},
			method:     http.MethodGet,
			path:       "/api/v1/admin/settings",
			wantStatus: http.StatusOK,
			wantJSON: `{
				"code": 0,
				"message": "success",
				"data": {
					"registration_enabled": true,
					"email_verify_enabled": false,
					"registration_email_suffix_whitelist": [],
					"promo_code_enabled": true,
					"password_reset_enabled": false,
					"frontend_url": "",
					"totp_enabled": false,
					"totp_encryption_key_configured": false,
					"smtp_host": "smtp.example.com",
					"smtp_port": 587,
					"smtp_username": "user",
					"smtp_password_configured": true,
					"smtp_from_email": "no-reply@example.com",
					"smtp_from_name": "Sub2API",
					"smtp_use_tls": true,
					"turnstile_enabled": true,
					"turnstile_site_key": "site-key",
					"turnstile_secret_key_configured": true,
						"linuxdo_connect_enabled": false,
						"linuxdo_connect_client_id": "",
						"linuxdo_connect_client_secret_configured": false,
						"linuxdo_connect_redirect_url": "",
						"oidc_connect_enabled": false,
						"oidc_connect_provider_name": "OIDC",
						"oidc_connect_client_id": "",
						"oidc_connect_client_secret_configured": false,
						"oidc_connect_issuer_url": "",
						"oidc_connect_discovery_url": "",
						"oidc_connect_authorize_url": "",
						"oidc_connect_token_url": "",
						"oidc_connect_userinfo_url": "",
						"oidc_connect_jwks_url": "",
						"oidc_connect_scopes": "openid email profile",
						"oidc_connect_redirect_url": "",
						"oidc_connect_frontend_redirect_url": "/auth/oidc/callback",
						"oidc_connect_token_auth_method": "client_secret_post",
						"oidc_connect_use_pkce": true,
						"oidc_connect_validate_id_token": true,
						"oidc_connect_allowed_signing_algs": "RS256,ES256,PS256",
						"oidc_connect_clock_skew_seconds": 120,
						"oidc_connect_require_email_verified": false,
						"oidc_connect_userinfo_email_path": "",
						"oidc_connect_userinfo_id_path": "",
						"oidc_connect_userinfo_username_path": "",
						"github_oauth_enabled": false,
						"github_oauth_client_id": "",
						"github_oauth_client_secret_configured": false,
						"github_oauth_redirect_url": "",
						"github_oauth_frontend_redirect_url": "/auth/oauth/callback",
						"google_oauth_enabled": false,
						"google_oauth_client_id": "",
						"google_oauth_client_secret_configured": false,
						"google_oauth_redirect_url": "",
						"google_oauth_frontend_redirect_url": "/auth/oauth/callback",
						"ops_monitoring_enabled": false,
						"ops_realtime_monitoring_enabled": true,
						"ops_query_mode_default": "auto",
						"ops_metrics_interval_seconds": 60,
						"site_name": "Sub2API",
						"site_logo": "",
						"site_subtitle": "Subtitle",
						"api_base_url": "https://api.example.com",
					"contact_info": "support",
					"doc_url": "https://docs.example.com",
					"auth_source_default_email_balance": 0,
					"auth_source_default_email_concurrency": 5,
					"auth_source_default_email_subscriptions": [],
					"auth_source_default_email_grant_on_signup": false,
					"auth_source_default_email_grant_on_first_bind": false,
					"auth_source_default_github_balance": 0,
					"auth_source_default_github_concurrency": 5,
					"auth_source_default_github_subscriptions": [],
					"auth_source_default_github_grant_on_signup": false,
					"auth_source_default_github_grant_on_first_bind": false,
					"auth_source_default_google_balance": 0,
					"auth_source_default_google_concurrency": 5,
					"auth_source_default_google_subscriptions": [],
					"auth_source_default_google_grant_on_signup": false,
					"auth_source_default_google_grant_on_first_bind": false,
					"auth_source_default_linuxdo_balance": 0,
					"auth_source_default_linuxdo_concurrency": 5,
					"auth_source_default_linuxdo_subscriptions": [],
					"auth_source_default_linuxdo_grant_on_signup": false,
					"auth_source_default_linuxdo_grant_on_first_bind": false,
					"auth_source_default_oidc_balance": 0,
					"auth_source_default_oidc_concurrency": 5,
					"auth_source_default_oidc_subscriptions": [],
					"auth_source_default_oidc_grant_on_signup": false,
					"auth_source_default_oidc_grant_on_first_bind": false,
					"auth_source_default_wechat_balance": 0,
					"auth_source_default_wechat_concurrency": 5,
					"auth_source_default_wechat_subscriptions": [],
					"auth_source_default_wechat_grant_on_signup": false,
					"auth_source_default_wechat_grant_on_first_bind": false,
					"force_email_on_third_party_signup": false,
					"default_concurrency": 5,
					"default_balance": 1.25,
					"affiliate_rebate_rate": 20,
					"affiliate_rebate_freeze_hours": 0,
					"affiliate_rebate_duration_days": 0,
					"affiliate_rebate_per_invitee_cap": 0,
					"default_user_rpm_limit": 0,
					"default_subscriptions": [],
					"enable_model_fallback": false,
					"fallback_model_anthropic": "claude-3-5-sonnet-20241022",
					"fallback_model_antigravity": "gemini-2.5-pro",
					"fallback_model_gemini": "gemini-2.5-pro",
						"fallback_model_openai": "gpt-4o",
						"enable_identity_patch": true,
						"identity_patch_prompt": "",
						"invitation_code_enabled": false,
						"home_content": "",
					"hide_ccs_import_button": false,
					"purchase_subscription_enabled": false,
					"purchase_subscription_url": "",
					"table_default_page_size": 20,
						"table_page_size_options": [10, 20, 50, 100],
					"min_claude_code_version": "",
					"max_claude_code_version": "",
					"allow_ungrouped_key_scheduling": false,
					"backend_mode_enabled": false,
					"enable_cch_signing": false,
					"enable_anthropic_cache_ttl_1h_injection": false,
					"enable_fingerprint_unification": true,
					"enable_metadata_passthrough": false,
					"web_search_emulation_enabled": false,
					"payment_visible_method_alipay_source": "easypay_alipay",
					"payment_visible_method_wxpay_source": "official_wxpay",
					"payment_visible_method_alipay_enabled": true,
					"payment_visible_method_wxpay_enabled": false,
					"openai_advanced_scheduler_enabled": true,
					"openai_fast_policy_settings": {
						"rules": [
							{
								"service_tier": "priority",
								"action": "filter",
								"scope": "all",
								"fallback_action": "pass"
							}
						]
					},
					"custom_menu_items": [],
					"custom_endpoints": [],
					"payment_enabled": false,
					"payment_min_amount": 0,
					"payment_max_amount": 0,
					"payment_daily_limit": 0,
					"payment_order_timeout_minutes": 0,
					"payment_max_pending_orders": 0,
					"payment_balance_disabled": false,
					"payment_balance_recharge_multiplier": 0,
					"payment_recharge_fee_rate": 0,
					"payment_load_balance_strategy": "",
					"payment_product_name_prefix": "",
					"payment_product_name_suffix": "",
					"payment_help_image_url": "",
					"payment_help_text": "",
					"payment_enabled_types": null,
					"payment_cancel_rate_limit_enabled": false,
					"payment_cancel_rate_limit_max": 0,
					"payment_cancel_rate_limit_window": 0,
					"payment_cancel_rate_limit_unit": "",
					"payment_cancel_rate_limit_window_mode": "",
					"balance_low_notify_enabled": false,
					"account_quota_notify_enabled": false,
					"balance_low_notify_threshold": 0,
					"balance_low_notify_recharge_url": "",
					"account_quota_notify_emails": [],
					"channel_monitor_enabled": true,
					"channel_monitor_default_interval_seconds": 60,
					"available_channels_enabled": false,
					"risk_control_enabled": false,
					"affiliate_enabled": false,
					"wechat_connect_enabled": false,
					"wechat_connect_app_id": "",
					"wechat_connect_app_secret_configured": false,
					"wechat_connect_mode": "open",
					"wechat_connect_open_enabled": false,
					"wechat_connect_open_app_id": "",
					"wechat_connect_open_app_secret_configured": false,
					"wechat_connect_mp_enabled": false,
					"wechat_connect_mp_app_id": "",
					"wechat_connect_mp_app_secret_configured": false,
					"wechat_connect_mobile_enabled": false,
					"wechat_connect_mobile_app_id": "",
					"wechat_connect_mobile_app_secret_configured": false,
					"wechat_connect_redirect_url": "",
					"wechat_connect_frontend_redirect_url": "/auth/wechat/callback",
					"wechat_connect_scopes": "snsapi_login"
				}
			}`,
		},
		{
			name: "GET /api/v1/admin/settings falls back to config oauth defaults",
			setup: func(t *testing.T, deps *contractDeps) {
				t.Helper()
				deps.cfg.OIDC = config.OIDCConnectConfig{
					Enabled:             true,
					ProviderName:        "ConfigOIDC",
					ClientID:            "oidc-config-client",
					ClientSecret:        "oidc-config-secret",
					IssuerURL:           "https://issuer.example.com",
					RedirectURL:         "https://api.example.com/api/v1/auth/oauth/oidc/callback",
					FrontendRedirectURL: "/auth/oidc/callback",
					Scopes:              "openid email profile",
					TokenAuthMethod:     "client_secret_post",
					UsePKCE:             true,
					ValidateIDToken:     true,
					AllowedSigningAlgs:  "RS256,ES256,PS256",
					ClockSkewSeconds:    120,
				}
				deps.cfg.WeChat = config.WeChatConnectConfig{
					Enabled:             true,
					OpenEnabled:         true,
					OpenAppID:           "wx-open-config",
					OpenAppSecret:       "wx-open-secret",
					Mode:                "open",
					Scopes:              "snsapi_login",
					FrontendRedirectURL: "/auth/wechat/callback",
				}
				deps.settingRepo.SetAll(map[string]string{
					service.SettingKeyRegistrationEnabled:              "true",
					service.SettingKeyEmailVerifyEnabled:               "false",
					service.SettingKeyRegistrationEmailSuffixWhitelist: "[]",
				})
			},
			method:     http.MethodGet,
			path:       "/api/v1/admin/settings",
			wantStatus: http.StatusOK,
			wantJSON: `{
				"code": 0,
				"message": "success",
				"data": {
					"registration_enabled": true,
					"email_verify_enabled": false,
					"registration_email_suffix_whitelist": [],
					"promo_code_enabled": true,
					"password_reset_enabled": false,
					"frontend_url": "",
					"invitation_code_enabled": false,
					"totp_enabled": false,
					"totp_encryption_key_configured": false,
					"smtp_host": "",
					"smtp_port": 587,
					"smtp_username": "",
					"smtp_password_configured": false,
					"smtp_from_email": "",
					"smtp_from_name": "",
					"smtp_use_tls": false,
					"turnstile_enabled": false,
					"turnstile_site_key": "",
					"turnstile_secret_key_configured": false,
					"linuxdo_connect_enabled": false,
					"linuxdo_connect_client_id": "",
					"linuxdo_connect_client_secret_configured": false,
					"linuxdo_connect_redirect_url": "",
					"oidc_connect_enabled": true,
					"oidc_connect_provider_name": "ConfigOIDC",
					"oidc_connect_client_id": "oidc-config-client",
					"oidc_connect_client_secret_configured": true,
					"oidc_connect_issuer_url": "https://issuer.example.com",
					"oidc_connect_discovery_url": "",
					"oidc_connect_authorize_url": "",
					"oidc_connect_token_url": "",
					"oidc_connect_userinfo_url": "",
					"oidc_connect_jwks_url": "",
					"oidc_connect_scopes": "openid email profile",
					"oidc_connect_redirect_url": "https://api.example.com/api/v1/auth/oauth/oidc/callback",
					"oidc_connect_frontend_redirect_url": "/auth/oidc/callback",
					"oidc_connect_token_auth_method": "client_secret_post",
					"oidc_connect_use_pkce": true,
					"oidc_connect_validate_id_token": true,
					"oidc_connect_allowed_signing_algs": "RS256,ES256,PS256",
					"oidc_connect_clock_skew_seconds": 120,
					"oidc_connect_require_email_verified": false,
					"oidc_connect_userinfo_email_path": "",
					"oidc_connect_userinfo_id_path": "",
					"oidc_connect_userinfo_username_path": "",
					"github_oauth_enabled": false,
					"github_oauth_client_id": "",
					"github_oauth_client_secret_configured": false,
					"github_oauth_redirect_url": "",
					"github_oauth_frontend_redirect_url": "/auth/oauth/callback",
					"google_oauth_enabled": false,
					"google_oauth_client_id": "",
					"google_oauth_client_secret_configured": false,
					"google_oauth_redirect_url": "",
					"google_oauth_frontend_redirect_url": "/auth/oauth/callback",
					"site_name": "Sub2API",
					"site_logo": "",
					"site_subtitle": "Subscription to API Conversion Platform",
					"api_base_url": "",
					"contact_info": "",
					"doc_url": "",
					"home_content": "",
					"hide_ccs_import_button": false,
					"purchase_subscription_enabled": false,
					"purchase_subscription_url": "",
					"table_default_page_size": 20,
					"table_page_size_options": [10, 20, 50],
					"custom_menu_items": [],
					"custom_endpoints": [],
					"default_concurrency": 0,
					"default_balance": 0,
					"affiliate_rebate_rate": 20,
					"affiliate_rebate_freeze_hours": 0,
					"affiliate_rebate_duration_days": 0,
					"affiliate_rebate_per_invitee_cap": 0,
					"default_user_rpm_limit": 0,
					"default_subscriptions": [],
					"enable_model_fallback": false,
					"fallback_model_anthropic": "claude-3-5-sonnet-20241022",
					"fallback_model_openai": "gpt-4o",
					"fallback_model_gemini": "gemini-2.5-pro",
					"fallback_model_antigravity": "gemini-2.5-pro",
					"enable_identity_patch": true,
					"identity_patch_prompt": "",
					"ops_monitoring_enabled": false,
					"ops_realtime_monitoring_enabled": true,
					"ops_query_mode_default": "auto",
					"ops_metrics_interval_seconds": 60,
					"min_claude_code_version": "",
					"max_claude_code_version": "",
					"allow_ungrouped_key_scheduling": false,
					"backend_mode_enabled": false,
					"enable_fingerprint_unification": true,
					"enable_metadata_passthrough": false,
					"enable_cch_signing": false,
					"enable_anthropic_cache_ttl_1h_injection": false,
					"web_search_emulation_enabled": false,
					"payment_visible_method_alipay_source": "",
					"payment_visible_method_wxpay_source": "",
					"payment_visible_method_alipay_enabled": false,
					"payment_visible_method_wxpay_enabled": false,
					"openai_advanced_scheduler_enabled": false,
					"openai_fast_policy_settings": {
						"rules": [
							{
								"service_tier": "priority",
								"action": "filter",
								"scope": "all",
								"fallback_action": "pass"
							}
						]
					},
					"payment_enabled": false,
					"payment_min_amount": 0,
					"payment_max_amount": 0,
					"payment_daily_limit": 0,
					"payment_order_timeout_minutes": 0,
					"payment_max_pending_orders": 0,
					"payment_enabled_types": null,
					"payment_balance_disabled": false,
					"payment_balance_recharge_multiplier": 0,
					"payment_recharge_fee_rate": 0,
					"payment_load_balance_strategy": "",
					"payment_product_name_prefix": "",
					"payment_product_name_suffix": "",
					"payment_help_image_url": "",
					"payment_help_text": "",
					"payment_cancel_rate_limit_enabled": false,
					"payment_cancel_rate_limit_max": 0,
					"payment_cancel_rate_limit_window": 0,
					"payment_cancel_rate_limit_unit": "",
					"payment_cancel_rate_limit_window_mode": "",
					"balance_low_notify_enabled": false,
					"account_quota_notify_enabled": false,
					"balance_low_notify_threshold": 0,
					"balance_low_notify_recharge_url": "",
					"account_quota_notify_emails": [],
					"channel_monitor_enabled": true,
					"channel_monitor_default_interval_seconds": 60,
					"available_channels_enabled": false,
					"risk_control_enabled": false,
					"affiliate_enabled": false,
					"wechat_connect_enabled": true,
					"wechat_connect_app_id": "wx-open-config",
					"wechat_connect_app_secret_configured": true,
					"wechat_connect_mode": "open",
					"wechat_connect_open_enabled": true,
					"wechat_connect_open_app_id": "wx-open-config",
					"wechat_connect_open_app_secret_configured": true,
					"wechat_connect_mp_enabled": false,
					"wechat_connect_mp_app_id": "wx-open-config",
					"wechat_connect_mp_app_secret_configured": true,
					"wechat_connect_mobile_enabled": false,
					"wechat_connect_mobile_app_id": "wx-open-config",
					"wechat_connect_mobile_app_secret_configured": true,
					"wechat_connect_redirect_url": "",
					"wechat_connect_frontend_redirect_url": "/auth/wechat/callback",
					"wechat_connect_scopes": "snsapi_login",
					"auth_source_default_email_balance": 0,
					"auth_source_default_email_concurrency": 5,
					"auth_source_default_email_subscriptions": [],
					"auth_source_default_email_grant_on_signup": false,
					"auth_source_default_email_grant_on_first_bind": false,
					"auth_source_default_github_balance": 0,
					"auth_source_default_github_concurrency": 5,
					"auth_source_default_github_subscriptions": [],
					"auth_source_default_github_grant_on_signup": false,
					"auth_source_default_github_grant_on_first_bind": false,
					"auth_source_default_google_balance": 0,
					"auth_source_default_google_concurrency": 5,
					"auth_source_default_google_subscriptions": [],
					"auth_source_default_google_grant_on_signup": false,
					"auth_source_default_google_grant_on_first_bind": false,
					"auth_source_default_linuxdo_balance": 0,
					"auth_source_default_linuxdo_concurrency": 5,
					"auth_source_default_linuxdo_subscriptions": [],
					"auth_source_default_linuxdo_grant_on_signup": false,
					"auth_source_default_linuxdo_grant_on_first_bind": false,
					"auth_source_default_oidc_balance": 0,
					"auth_source_default_oidc_concurrency": 5,
					"auth_source_default_oidc_subscriptions": [],
					"auth_source_default_oidc_grant_on_signup": false,
					"auth_source_default_oidc_grant_on_first_bind": false,
					"auth_source_default_wechat_balance": 0,
					"auth_source_default_wechat_concurrency": 5,
					"auth_source_default_wechat_subscriptions": [],
					"auth_source_default_wechat_grant_on_signup": false,
					"auth_source_default_wechat_grant_on_first_bind": false,
					"force_email_on_third_party_signup": false
				}
			}`,
		},
		{
			name:   "POST /api/v1/admin/accounts/bulk-update",
			method: http.MethodPost,
			path:   "/api/v1/admin/accounts/bulk-update",
			body:   `{"account_ids":[101,102],"schedulable":false}`,
			headers: map[string]string{
				"Content-Type": "application/json",
			},
			wantStatus: http.StatusOK,
			wantJSON: `{
				"code": 0,
				"message": "success",
				"data": {
					"success": 2,
					"failed": 0,
					"success_ids": [101, 102],
					"failed_ids": [],
					"results": [
						{"account_id": 101, "success": true},
						{"account_id": 102, "success": true}
					]
				}
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := newContractDeps(t)
			if tt.setup != nil {
				tt.setup(t, deps)
			}

			status, body := doRequest(t, deps.router, tt.method, tt.path, tt.body, tt.headers)
			require.Equal(t, tt.wantStatus, status)
			require.JSONEq(t, tt.wantJSON, body)
		})
	}
}

type contractDeps struct {
	now         time.Time
	router      http.Handler
	cfg         *config.Config
	apiKeyRepo  *stubApiKeyRepo
	groupRepo   *stubGroupRepo
	userSubRepo *stubUserSubscriptionRepo
	usageRepo   *stubUsageLogRepo
	settingRepo *stubSettingRepo
	redeemRepo  *stubRedeemCodeRepo
}

func newContractDeps(t *testing.T) *contractDeps {
	t.Helper()

	now := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	userRepo := &stubUserRepo{
		users: map[int64]*service.User{
			1: {
				ID:            1,
				Email:         "alice@example.com",
				Username:      "alice",
				Notes:         "hello",
				Role:          service.RoleUser,
				Balance:       12.5,
				Concurrency:   5,
				Status:        service.StatusActive,
				AllowedGroups: nil,
				CreatedAt:     now,
				UpdatedAt:     now,
			},
		},
	}

	apiKeyRepo := newStubApiKeyRepo(now)
	apiKeyCache := stubApiKeyCache{}
	groupRepo := &stubGroupRepo{}
	userSubRepo := &stubUserSubscriptionRepo{}
	accountRepo := stubAccountRepo{}
	proxyRepo := stubProxyRepo{}
	redeemRepo := &stubRedeemCodeRepo{}

	cfg := &config.Config{
		Default: config.DefaultConfig{
			APIKeyPrefix: "sk-",
		},
		RunMode: config.RunModeStandard,
	}

	userService := service.NewUserService(userRepo, nil, nil, nil)
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, userRepo, groupRepo, userSubRepo, nil, apiKeyCache, cfg)

	usageRepo := newStubUsageLogRepo()
	usageService := service.NewUsageService(usageRepo, userRepo, nil, nil)

	subscriptionService := service.NewSubscriptionService(groupRepo, userSubRepo, nil, nil, cfg)
	subscriptionHandler := handler.NewSubscriptionHandler(subscriptionService)

	redeemService := service.NewRedeemService(redeemRepo, userRepo, subscriptionService, nil, nil, nil, nil, nil)
	redeemHandler := handler.NewRedeemHandler(redeemService)

	settingRepo := newStubSettingRepo()
	settingService := service.NewSettingService(settingRepo, cfg)

	adminService := service.NewAdminService(userRepo, groupRepo, &accountRepo, proxyRepo, apiKeyRepo, redeemRepo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	authHandler := handler.NewAuthHandler(cfg, nil, userService, settingService, nil, redeemService, nil)
	apiKeyHandler := handler.NewAPIKeyHandler(apiKeyService)
	usageHandler := handler.NewUsageHandler(usageService, apiKeyService)
	adminSettingHandler := adminhandler.NewSettingHandler(settingService, nil, nil, nil, nil, nil)
	adminAccountHandler := adminhandler.NewAccountHandler(adminService, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	jwtAuth := func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{
			UserID:      1,
			Concurrency: 5,
		})
		c.Set(string(middleware.ContextKeyUserRole), service.RoleUser)
		c.Next()
	}
	adminAuth := func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{
			UserID:      1,
			Concurrency: 5,
		})
		c.Set(string(middleware.ContextKeyUserRole), service.RoleAdmin)
		c.Next()
	}

	r := gin.New()

	v1 := r.Group("/api/v1")

	v1Auth := v1.Group("")
	v1Auth.Use(jwtAuth)
	v1Auth.GET("/auth/me", authHandler.GetCurrentUser)

	v1Keys := v1.Group("")
	v1Keys.Use(jwtAuth)
	v1Keys.GET("/keys", apiKeyHandler.List)
	v1Keys.POST("/keys", apiKeyHandler.Create)
	v1Keys.GET("/groups/available", apiKeyHandler.GetAvailableGroups)

	v1Usage := v1.Group("")
	v1Usage.Use(jwtAuth)
	v1Usage.GET("/usage", usageHandler.List)
	v1Usage.GET("/usage/stats", usageHandler.Stats)

	v1Subs := v1.Group("")
	v1Subs.Use(jwtAuth)
	v1Subs.GET("/subscriptions", subscriptionHandler.List)

	v1Redeem := v1.Group("")
	v1Redeem.Use(jwtAuth)
	v1Redeem.GET("/redeem/history", redeemHandler.GetHistory)

	v1Admin := v1.Group("/admin")
	v1Admin.Use(adminAuth)
	v1Admin.GET("/settings", adminSettingHandler.GetSettings)
	v1Admin.POST("/accounts/bulk-update", adminAccountHandler.BulkUpdate)

	return &contractDeps{
		now:         now,
		router:      r,
		cfg:         cfg,
		apiKeyRepo:  apiKeyRepo,
		groupRepo:   groupRepo,
		userSubRepo: userSubRepo,
		usageRepo:   usageRepo,
		settingRepo: settingRepo,
		redeemRepo:  redeemRepo,
	}
}

func doRequest(t *testing.T, router http.Handler, method, path, body string, headers map[string]string) (int, string) {
	t.Helper()

	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	respBody, err := io.ReadAll(w.Result().Body)
	require.NoError(t, err)

	return w.Result().StatusCode, string(respBody)
}

func ptr[T any](v T) *T { return &v }

type stubUserRepo struct {
	users map[int64]*service.User
}

func (r *stubUserRepo) Create(ctx context.Context, user *service.User) error {
	return errors.New("not implemented")
}

func (r *stubUserRepo) GetByID(ctx context.Context, id int64) (*service.User, error) {
	user, ok := r.users[id]
	if !ok {
		return nil, service.ErrUserNotFound
	}
	clone := *user
	return &clone, nil
}

func (r *stubUserRepo) GetByEmail(ctx context.Context, email string) (*service.User, error) {
	for _, user := range r.users {
		if user.Email == email {
			clone := *user
			return &clone, nil
		}
	}
	return nil, service.ErrUserNotFound
}

func (r *stubUserRepo) GetFirstAdmin(ctx context.Context) (*service.User, error) {
	for _, user := range r.users {
		if user.Role == service.RoleAdmin && user.Status == service.StatusActive {
			clone := *user
			return &clone, nil
		}
	}
	return nil, service.ErrUserNotFound
}

func (r *stubUserRepo) Update(ctx context.Context, user *service.User) error {
	return errors.New("not implemented")
}

func (r *stubUserRepo) Delete(ctx context.Context, id int64) error {
	return errors.New("not implemented")
}

func (r *stubUserRepo) GetUserAvatar(ctx context.Context, userID int64) (*service.UserAvatar, error) {
	return nil, nil
}

func (r *stubUserRepo) UpsertUserAvatar(ctx context.Context, userID int64, input service.UpsertUserAvatarInput) (*service.UserAvatar, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUserRepo) DeleteUserAvatar(ctx context.Context, userID int64) error {
	return errors.New("not implemented")
}

func (r *stubUserRepo) List(ctx context.Context, params pagination.PaginationParams) ([]service.User, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}

func (r *stubUserRepo) ListWithFilters(ctx context.Context, params pagination.PaginationParams, filters service.UserListFilters) ([]service.User, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}

func (r *stubUserRepo) UpdateBalance(ctx context.Context, id int64, amount float64) error {
	return errors.New("not implemented")
}

func (r *stubUserRepo) DeductBalance(ctx context.Context, id int64, amount float64) error {
	return errors.New("not implemented")
}

func (r *stubUserRepo) UpdateConcurrency(ctx context.Context, id int64, amount int) error {
	return errors.New("not implemented")
}

func (r *stubUserRepo) BatchSetConcurrency(context.Context, []int64, int) (int, error) { return 0, nil }
func (r *stubUserRepo) BatchAddConcurrency(context.Context, []int64, int) (int, error) { return 0, nil }

func (r *stubUserRepo) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	return false, errors.New("not implemented")
}

func (r *stubUserRepo) RemoveGroupFromAllowedGroups(ctx context.Context, groupID int64) (int64, error) {
	return 0, errors.New("not implemented")
}

func (r *stubUserRepo) RemoveGroupFromUserAllowedGroups(ctx context.Context, userID int64, groupID int64) error {
	return errors.New("not implemented")
}

func (r *stubUserRepo) AddGroupToAllowedGroups(ctx context.Context, userID int64, groupID int64) error {
	return errors.New("not implemented")
}

func (r *stubUserRepo) ListUserAuthIdentities(ctx context.Context, userID int64) ([]service.UserAuthIdentityRecord, error) {
	return nil, nil
}

func (r *stubUserRepo) UnbindUserAuthProvider(context.Context, int64, string) error {
	return errors.New("not implemented")
}

func (r *stubUserRepo) GetLatestUsedAtByUserIDs(ctx context.Context, userIDs []int64) (map[int64]*time.Time, error) {
	return map[int64]*time.Time{}, nil
}

func (r *stubUserRepo) GetLatestUsedAtByUserID(ctx context.Context, userID int64) (*time.Time, error) {
	return nil, nil
}

func (r *stubUserRepo) UpdateUserLastActiveAt(ctx context.Context, userID int64, activeAt time.Time) error {
	return nil
}

func (r *stubUserRepo) UpdateTotpSecret(ctx context.Context, userID int64, encryptedSecret *string) error {
	return errors.New("not implemented")
}

func (r *stubUserRepo) EnableTotp(ctx context.Context, userID int64) error {
	return errors.New("not implemented")
}

func (r *stubUserRepo) DisableTotp(ctx context.Context, userID int64) error {
	return errors.New("not implemented")
}

type stubApiKeyCache struct{}

func (stubApiKeyCache) GetCreateAttemptCount(ctx context.Context, userID int64) (int, error) {
	return 0, nil
}

func (stubApiKeyCache) IncrementCreateAttemptCount(ctx context.Context, userID int64) error {
	return nil
}

func (stubApiKeyCache) DeleteCreateAttemptCount(ctx context.Context, userID int64) error {
	return nil
}

func (stubApiKeyCache) IncrementDailyUsage(ctx context.Context, apiKey string) error {
	return nil
}

func (stubApiKeyCache) SetDailyUsageExpiry(ctx context.Context, apiKey string, ttl time.Duration) error {
	return nil
}

func (stubApiKeyCache) GetAuthCache(ctx context.Context, key string) (*service.APIKeyAuthCacheEntry, error) {
	return nil, nil
}

func (stubApiKeyCache) SetAuthCache(ctx context.Context, key string, entry *service.APIKeyAuthCacheEntry, ttl time.Duration) error {
	return nil
}

func (stubApiKeyCache) DeleteAuthCache(ctx context.Context, key string) error {
	return nil
}

func (stubApiKeyCache) PublishAuthCacheInvalidation(ctx context.Context, cacheKey string) error {
	return nil
}

func (stubApiKeyCache) SubscribeAuthCacheInvalidation(ctx context.Context, handler func(cacheKey string)) error {
	return nil
}

type stubGroupRepo struct {
	active []service.Group
}

func (r *stubGroupRepo) SetActive(groups []service.Group) {
	r.active = append([]service.Group(nil), groups...)
}

func (stubGroupRepo) Create(ctx context.Context, group *service.Group) error {
	return errors.New("not implemented")
}

func (stubGroupRepo) GetByID(ctx context.Context, id int64) (*service.Group, error) {
	return nil, service.ErrGroupNotFound
}

func (stubGroupRepo) GetByIDLite(ctx context.Context, id int64) (*service.Group, error) {
	return nil, service.ErrGroupNotFound
}

func (stubGroupRepo) Update(ctx context.Context, group *service.Group) error {
	return errors.New("not implemented")
}

func (stubGroupRepo) Delete(ctx context.Context, id int64) error {
	return errors.New("not implemented")
}

func (stubGroupRepo) DeleteCascade(ctx context.Context, id int64) ([]int64, error) {
	return nil, errors.New("not implemented")
}

func (stubGroupRepo) List(ctx context.Context, params pagination.PaginationParams) ([]service.Group, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}

func (stubGroupRepo) ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, status, search string, isExclusive *bool) ([]service.Group, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}

func (r *stubGroupRepo) ListActive(ctx context.Context) ([]service.Group, error) {
	return append([]service.Group(nil), r.active...), nil
}

func (r *stubGroupRepo) ListActiveByPlatform(ctx context.Context, platform string) ([]service.Group, error) {
	out := make([]service.Group, 0, len(r.active))
	for i := range r.active {
		g := r.active[i]
		if g.Platform == platform {
			out = append(out, g)
		}
	}
	return out, nil
}

func (stubGroupRepo) ExistsByName(ctx context.Context, name string) (bool, error) {
	return false, errors.New("not implemented")
}

func (stubGroupRepo) GetAccountCount(ctx context.Context, groupID int64) (int64, int64, error) {
	return 0, 0, errors.New("not implemented")
}

func (stubGroupRepo) DeleteAccountGroupsByGroupID(ctx context.Context, groupID int64) (int64, error) {
	return 0, errors.New("not implemented")
}

func (stubGroupRepo) BindAccountsToGroup(ctx context.Context, groupID int64, accountIDs []int64) error {
	return errors.New("not implemented")
}

func (stubGroupRepo) GetAccountIDsByGroupIDs(ctx context.Context, groupIDs []int64) ([]int64, error) {
	return nil, errors.New("not implemented")
}

func (stubGroupRepo) UpdateSortOrders(ctx context.Context, updates []service.GroupSortOrderUpdate) error {
	return nil
}

type stubAccountRepo struct {
	bulkUpdateIDs []int64
}

func (s *stubAccountRepo) Create(ctx context.Context, account *service.Account) error {
	return errors.New("not implemented")
}

func (s *stubAccountRepo) GetByID(ctx context.Context, id int64) (*service.Account, error) {
	return nil, service.ErrAccountNotFound
}

func (s *stubAccountRepo) GetByIDs(ctx context.Context, ids []int64) ([]*service.Account, error) {
	return nil, errors.New("not implemented")
}

func (s *stubAccountRepo) ExistsByID(ctx context.Context, id int64) (bool, error) {
	return false, errors.New("not implemented")
}

func (s *stubAccountRepo) GetByCRSAccountID(ctx context.Context, crsAccountID string) (*service.Account, error) {
	return nil, errors.New("not implemented")
}

func (s *stubAccountRepo) FindByExtraField(ctx context.Context, key string, value any) ([]service.Account, error) {
	return nil, errors.New("not implemented")
}

func (s *stubAccountRepo) Update(ctx context.Context, account *service.Account) error {
	return errors.New("not implemented")
}

func (s *stubAccountRepo) Delete(ctx context.Context, id int64) error {
	return errors.New("not implemented")
}

func (s *stubAccountRepo) List(ctx context.Context, params pagination.PaginationParams) ([]service.Account, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}

func (s *stubAccountRepo) ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, accountType, status, search string, groupID int64, privacyMode string) ([]service.Account, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}

func (s *stubAccountRepo) ListByGroup(ctx context.Context, groupID int64) ([]service.Account, error) {
	return nil, errors.New("not implemented")
}

func (s *stubAccountRepo) ListActive(ctx context.Context) ([]service.Account, error) {
	return nil, errors.New("not implemented")
}

func (s *stubAccountRepo) ListByPlatform(ctx context.Context, platform string) ([]service.Account, error) {
	return nil, errors.New("not implemented")
}

func (s *stubAccountRepo) UpdateLastUsed(ctx context.Context, id int64) error {
	return errors.New("not implemented")
}

func (s *stubAccountRepo) BatchUpdateLastUsed(ctx context.Context, updates map[int64]time.Time) error {
	return errors.New("not implemented")
}

func (s *stubAccountRepo) SetError(ctx context.Context, id int64, errorMsg string) error {
	return errors.New("not implemented")
}

func (s *stubAccountRepo) ClearError(ctx context.Context, id int64) error {
	return errors.New("not implemented")
}

func (s *stubAccountRepo) SetSchedulable(ctx context.Context, id int64, schedulable bool) error {
	return errors.New("not implemented")
}

func (s *stubAccountRepo) AutoPauseExpiredAccounts(ctx context.Context, now time.Time) (int64, error) {
	return 0, errors.New("not implemented")
}

func (s *stubAccountRepo) BindGroups(ctx context.Context, accountID int64, groupIDs []int64) error {
	return errors.New("not implemented")
}

func (s *stubAccountRepo) ListSchedulable(ctx context.Context) ([]service.Account, error) {
	return nil, errors.New("not implemented")
}

func (s *stubAccountRepo) ListSchedulableByGroupID(ctx context.Context, groupID int64) ([]service.Account, error) {
	return nil, errors.New("not implemented")
}

func (s *stubAccountRepo) ListSchedulableByPlatform(ctx context.Context, platform string) ([]service.Account, error) {
	return nil, errors.New("not implemented")
}

func (s *stubAccountRepo) ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]service.Account, error) {
	return nil, errors.New("not implemented")
}

func (s *stubAccountRepo) ListSchedulableByPlatforms(ctx context.Context, platforms []string) ([]service.Account, error) {
	return nil, errors.New("not implemented")
}

func (s *stubAccountRepo) ListSchedulableByGroupIDAndPlatforms(ctx context.Context, groupID int64, platforms []string) ([]service.Account, error) {
	return nil, errors.New("not implemented")
}

func (s *stubAccountRepo) ListSchedulableUngroupedByPlatform(ctx context.Context, platform string) ([]service.Account, error) {
	return nil, errors.New("not implemented")
}

func (s *stubAccountRepo) ListSchedulableUngroupedByPlatforms(ctx context.Context, platforms []string) ([]service.Account, error) {
	return nil, errors.New("not implemented")
}

func (s *stubAccountRepo) SetRateLimited(ctx context.Context, id int64, resetAt time.Time) error {
	return errors.New("not implemented")
}

func (s *stubAccountRepo) SetModelRateLimit(ctx context.Context, id int64, scope string, resetAt time.Time) error {
	return errors.New("not implemented")
}

func (s *stubAccountRepo) SetOverloaded(ctx context.Context, id int64, until time.Time) error {
	return errors.New("not implemented")
}

func (s *stubAccountRepo) SetTempUnschedulable(ctx context.Context, id int64, until time.Time, reason string) error {
	return errors.New("not implemented")
}

func (s *stubAccountRepo) ClearTempUnschedulable(ctx context.Context, id int64) error {
	return errors.New("not implemented")
}

func (s *stubAccountRepo) ClearRateLimit(ctx context.Context, id int64) error {
	return errors.New("not implemented")
}

func (s *stubAccountRepo) ClearAntigravityQuotaScopes(ctx context.Context, id int64) error {
	return errors.New("not implemented")
}

func (s *stubAccountRepo) ClearModelRateLimits(ctx context.Context, id int64) error {
	return errors.New("not implemented")
}

func (s *stubAccountRepo) UpdateSessionWindow(ctx context.Context, id int64, start, end *time.Time, status string) error {
	return errors.New("not implemented")
}

func (s *stubAccountRepo) UpdateExtra(ctx context.Context, id int64, updates map[string]any) error {
	return errors.New("not implemented")
}

func (s *stubAccountRepo) IncrementQuotaUsed(ctx context.Context, id int64, amount float64) error {
	return errors.New("not implemented")
}

func (s *stubAccountRepo) ResetQuotaUsed(ctx context.Context, id int64) error {
	return errors.New("not implemented")
}

func (s *stubAccountRepo) BulkUpdate(ctx context.Context, ids []int64, updates service.AccountBulkUpdate) (int64, error) {
	s.bulkUpdateIDs = append([]int64{}, ids...)
	return int64(len(ids)), nil
}

func (s *stubAccountRepo) ListCRSAccountIDs(ctx context.Context) (map[string]int64, error) {
	return nil, errors.New("not implemented")
}

type stubProxyRepo struct{}

func (stubProxyRepo) Create(ctx context.Context, proxy *service.Proxy) error {
	return errors.New("not implemented")
}

func (stubProxyRepo) GetByID(ctx context.Context, id int64) (*service.Proxy, error) {
	return nil, service.ErrProxyNotFound
}

func (stubProxyRepo) ListByIDs(ctx context.Context, ids []int64) ([]service.Proxy, error) {
	return nil, errors.New("not implemented")
}

func (stubProxyRepo) Update(ctx context.Context, proxy *service.Proxy) error {
	return errors.New("not implemented")
}

func (stubProxyRepo) Delete(ctx context.Context, id int64) error {
	return errors.New("not implemented")
}

func (stubProxyRepo) List(ctx context.Context, params pagination.PaginationParams) ([]service.Proxy, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}

func (stubProxyRepo) ListWithFilters(ctx context.Context, params pagination.PaginationParams, protocol, status, search string) ([]service.Proxy, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}

func (stubProxyRepo) ListWithFiltersAndAccountCount(ctx context.Context, params pagination.PaginationParams, protocol, status, search string) ([]service.ProxyWithAccountCount, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}

func (stubProxyRepo) ListActive(ctx context.Context) ([]service.Proxy, error) {
	return nil, errors.New("not implemented")
}

func (stubProxyRepo) ListActiveWithAccountCount(ctx context.Context) ([]service.ProxyWithAccountCount, error) {
	return nil, errors.New("not implemented")
}

func (stubProxyRepo) ExistsByHostPortAuth(ctx context.Context, host string, port int, username, password string) (bool, error) {
	return false, errors.New("not implemented")
}

func (stubProxyRepo) CountAccountsByProxyID(ctx context.Context, proxyID int64) (int64, error) {
	return 0, errors.New("not implemented")
}

func (stubProxyRepo) ListAccountSummariesByProxyID(ctx context.Context, proxyID int64) ([]service.ProxyAccountSummary, error) {
	return nil, errors.New("not implemented")
}

type stubRedeemCodeRepo struct {
	byUser map[int64][]service.RedeemCode
}

func (r *stubRedeemCodeRepo) SetByUser(userID int64, codes []service.RedeemCode) {
	if r.byUser == nil {
		r.byUser = make(map[int64][]service.RedeemCode)
	}
	r.byUser[userID] = append([]service.RedeemCode(nil), codes...)
}

func (stubRedeemCodeRepo) Create(ctx context.Context, code *service.RedeemCode) error {
	return errors.New("not implemented")
}

func (stubRedeemCodeRepo) CreateBatch(ctx context.Context, codes []service.RedeemCode) error {
	return errors.New("not implemented")
}

func (stubRedeemCodeRepo) GetByID(ctx context.Context, id int64) (*service.RedeemCode, error) {
	return nil, service.ErrRedeemCodeNotFound
}

func (stubRedeemCodeRepo) GetByCode(ctx context.Context, code string) (*service.RedeemCode, error) {
	return nil, service.ErrRedeemCodeNotFound
}

func (stubRedeemCodeRepo) Update(ctx context.Context, code *service.RedeemCode) error {
	return errors.New("not implemented")
}

func (stubRedeemCodeRepo) Delete(ctx context.Context, id int64) error {
	return errors.New("not implemented")
}

func (stubRedeemCodeRepo) Use(ctx context.Context, id, userID int64) error {
	return errors.New("not implemented")
}

func (stubRedeemCodeRepo) List(ctx context.Context, params pagination.PaginationParams) ([]service.RedeemCode, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}

func (stubRedeemCodeRepo) ListWithFilters(ctx context.Context, params pagination.PaginationParams, codeType, status, search string) ([]service.RedeemCode, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}

func (r *stubRedeemCodeRepo) ListByUser(ctx context.Context, userID int64, limit int) ([]service.RedeemCode, error) {
	if r.byUser == nil {
		return nil, nil
	}
	codes := r.byUser[userID]
	if limit > 0 && len(codes) > limit {
		codes = codes[:limit]
	}
	return append([]service.RedeemCode(nil), codes...), nil
}

func (stubRedeemCodeRepo) ListByUserPaginated(ctx context.Context, userID int64, params pagination.PaginationParams, codeType string) ([]service.RedeemCode, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}

func (stubRedeemCodeRepo) SumPositiveBalanceByUser(ctx context.Context, userID int64) (float64, error) {
	return 0, errors.New("not implemented")
}

type stubUserSubscriptionRepo struct {
	byUser       map[int64][]service.UserSubscription
	activeByUser map[int64][]service.UserSubscription
}

func (r *stubUserSubscriptionRepo) SetByUserID(userID int64, subs []service.UserSubscription) {
	if r.byUser == nil {
		r.byUser = make(map[int64][]service.UserSubscription)
	}
	r.byUser[userID] = append([]service.UserSubscription(nil), subs...)
}

func (r *stubUserSubscriptionRepo) SetActiveByUserID(userID int64, subs []service.UserSubscription) {
	if r.activeByUser == nil {
		r.activeByUser = make(map[int64][]service.UserSubscription)
	}
	r.activeByUser[userID] = append([]service.UserSubscription(nil), subs...)
}

func (stubUserSubscriptionRepo) Create(ctx context.Context, sub *service.UserSubscription) error {
	return errors.New("not implemented")
}
func (stubUserSubscriptionRepo) GetByID(ctx context.Context, id int64) (*service.UserSubscription, error) {
	return nil, errors.New("not implemented")
}
func (stubUserSubscriptionRepo) GetByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (*service.UserSubscription, error) {
	return nil, errors.New("not implemented")
}
func (stubUserSubscriptionRepo) GetActiveByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (*service.UserSubscription, error) {
	return nil, errors.New("not implemented")
}
func (stubUserSubscriptionRepo) Update(ctx context.Context, sub *service.UserSubscription) error {
	return errors.New("not implemented")
}
func (stubUserSubscriptionRepo) Delete(ctx context.Context, id int64) error {
	return errors.New("not implemented")
}
func (r *stubUserSubscriptionRepo) ListByUserID(ctx context.Context, userID int64) ([]service.UserSubscription, error) {
	if r.byUser == nil {
		return nil, nil
	}
	return append([]service.UserSubscription(nil), r.byUser[userID]...), nil
}
func (r *stubUserSubscriptionRepo) ListActiveByUserID(ctx context.Context, userID int64) ([]service.UserSubscription, error) {
	if r.activeByUser == nil {
		return nil, nil
	}
	return append([]service.UserSubscription(nil), r.activeByUser[userID]...), nil
}
func (stubUserSubscriptionRepo) ListByGroupID(ctx context.Context, groupID int64, params pagination.PaginationParams) ([]service.UserSubscription, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}
func (stubUserSubscriptionRepo) List(ctx context.Context, params pagination.PaginationParams, userID, groupID *int64, status, platform, sortBy, sortOrder string) ([]service.UserSubscription, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}
func (stubUserSubscriptionRepo) ExistsByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (bool, error) {
	return false, errors.New("not implemented")
}
func (stubUserSubscriptionRepo) ExtendExpiry(ctx context.Context, subscriptionID int64, newExpiresAt time.Time) error {
	return errors.New("not implemented")
}
func (stubUserSubscriptionRepo) UpdateStatus(ctx context.Context, subscriptionID int64, status string) error {
	return errors.New("not implemented")
}
func (stubUserSubscriptionRepo) UpdateNotes(ctx context.Context, subscriptionID int64, notes string) error {
	return errors.New("not implemented")
}
func (stubUserSubscriptionRepo) ActivateWindows(ctx context.Context, id int64, start time.Time) error {
	return errors.New("not implemented")
}
func (stubUserSubscriptionRepo) ResetDailyUsage(ctx context.Context, id int64, newWindowStart time.Time) error {
	return errors.New("not implemented")
}
func (stubUserSubscriptionRepo) ResetWeeklyUsage(ctx context.Context, id int64, newWindowStart time.Time) error {
	return errors.New("not implemented")
}
func (stubUserSubscriptionRepo) ResetMonthlyUsage(ctx context.Context, id int64, newWindowStart time.Time) error {
	return errors.New("not implemented")
}
func (stubUserSubscriptionRepo) IncrementUsage(ctx context.Context, id int64, costUSD float64) error {
	return errors.New("not implemented")
}
func (stubUserSubscriptionRepo) BatchUpdateExpiredStatus(ctx context.Context) (int64, error) {
	return 0, errors.New("not implemented")
}

type stubApiKeyRepo struct {
	now time.Time

	nextID int64
	byID   map[int64]*service.APIKey
	byKey  map[string]*service.APIKey
}

func newStubApiKeyRepo(now time.Time) *stubApiKeyRepo {
	return &stubApiKeyRepo{
		now:    now,
		nextID: 100,
		byID:   make(map[int64]*service.APIKey),
		byKey:  make(map[string]*service.APIKey),
	}
}

func (r *stubApiKeyRepo) MustSeed(key *service.APIKey) {
	if key == nil {
		return
	}
	clone := *key
	r.byID[clone.ID] = &clone
	r.byKey[clone.Key] = &clone
}

func (r *stubApiKeyRepo) Create(ctx context.Context, key *service.APIKey) error {
	if key == nil {
		return errors.New("nil key")
	}
	if key.ID == 0 {
		key.ID = r.nextID
		r.nextID++
	}
	if key.CreatedAt.IsZero() {
		key.CreatedAt = r.now
	}
	if key.UpdatedAt.IsZero() {
		key.UpdatedAt = r.now
	}
	clone := *key
	r.byID[clone.ID] = &clone
	r.byKey[clone.Key] = &clone
	return nil
}

func (r *stubApiKeyRepo) GetByID(ctx context.Context, id int64) (*service.APIKey, error) {
	key, ok := r.byID[id]
	if !ok {
		return nil, service.ErrAPIKeyNotFound
	}
	clone := *key
	return &clone, nil
}

func (r *stubApiKeyRepo) GetKeyAndOwnerID(ctx context.Context, id int64) (string, int64, error) {
	key, ok := r.byID[id]
	if !ok {
		return "", 0, service.ErrAPIKeyNotFound
	}
	return key.Key, key.UserID, nil
}

func (r *stubApiKeyRepo) GetByKey(ctx context.Context, key string) (*service.APIKey, error) {
	found, ok := r.byKey[key]
	if !ok {
		return nil, service.ErrAPIKeyNotFound
	}
	clone := *found
	return &clone, nil
}

func (r *stubApiKeyRepo) GetByKeyForAuth(ctx context.Context, key string) (*service.APIKey, error) {
	return r.GetByKey(ctx, key)
}

func (r *stubApiKeyRepo) Update(ctx context.Context, key *service.APIKey) error {
	if key == nil {
		return errors.New("nil key")
	}
	if _, ok := r.byID[key.ID]; !ok {
		return service.ErrAPIKeyNotFound
	}
	if key.UpdatedAt.IsZero() {
		key.UpdatedAt = r.now
	}
	clone := *key
	r.byID[clone.ID] = &clone
	r.byKey[clone.Key] = &clone
	return nil
}

func (r *stubApiKeyRepo) Delete(ctx context.Context, id int64) error {
	key, ok := r.byID[id]
	if !ok {
		return service.ErrAPIKeyNotFound
	}
	delete(r.byID, id)
	delete(r.byKey, key.Key)
	return nil
}

func (r *stubApiKeyRepo) ListByUserID(ctx context.Context, userID int64, params pagination.PaginationParams, _ service.APIKeyListFilters) ([]service.APIKey, *pagination.PaginationResult, error) {
	ids := make([]int64, 0, len(r.byID))
	for id := range r.byID {
		if r.byID[id].UserID == userID {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] > ids[j] })

	start := params.Offset()
	if start > len(ids) {
		start = len(ids)
	}
	end := start + params.Limit()
	if end > len(ids) {
		end = len(ids)
	}

	out := make([]service.APIKey, 0, end-start)
	for _, id := range ids[start:end] {
		clone := *r.byID[id]
		out = append(out, clone)
	}

	total := int64(len(ids))
	pageSize := params.Limit()
	pages := int(math.Ceil(float64(total) / float64(pageSize)))
	if pages < 1 {
		pages = 1
	}
	return out, &pagination.PaginationResult{
		Total:    total,
		Page:     params.Page,
		PageSize: pageSize,
		Pages:    pages,
	}, nil
}

func (r *stubApiKeyRepo) VerifyOwnership(ctx context.Context, userID int64, apiKeyIDs []int64) ([]int64, error) {
	if len(apiKeyIDs) == 0 {
		return []int64{}, nil
	}
	seen := make(map[int64]struct{}, len(apiKeyIDs))
	out := make([]int64, 0, len(apiKeyIDs))
	for _, id := range apiKeyIDs {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		key, ok := r.byID[id]
		if ok && key.UserID == userID {
			out = append(out, id)
		}
	}
	return out, nil
}

func (r *stubApiKeyRepo) CountByUserID(ctx context.Context, userID int64) (int64, error) {
	var count int64
	for _, key := range r.byID {
		if key.UserID == userID {
			count++
		}
	}
	return count, nil
}

func (r *stubApiKeyRepo) ExistsByKey(ctx context.Context, key string) (bool, error) {
	_, ok := r.byKey[key]
	return ok, nil
}

func (r *stubApiKeyRepo) ListByGroupID(ctx context.Context, groupID int64, params pagination.PaginationParams) ([]service.APIKey, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}

func (r *stubApiKeyRepo) SearchAPIKeys(ctx context.Context, userID int64, keyword string, limit int) ([]service.APIKey, error) {
	return nil, errors.New("not implemented")
}

func (r *stubApiKeyRepo) ClearGroupIDByGroupID(ctx context.Context, groupID int64) (int64, error) {
	return 0, errors.New("not implemented")
}

func (r *stubApiKeyRepo) UpdateGroupIDByUserAndGroup(ctx context.Context, userID, oldGroupID, newGroupID int64) (int64, error) {
	var updated int64
	for id, key := range r.byID {
		if key.UserID != userID || key.GroupID == nil || *key.GroupID != oldGroupID {
			continue
		}
		clone := *key
		gid := newGroupID
		clone.GroupID = &gid
		r.byID[id] = &clone
		r.byKey[clone.Key] = &clone
		updated++
	}
	return updated, nil
}

func (r *stubApiKeyRepo) CountByGroupID(ctx context.Context, groupID int64) (int64, error) {
	return 0, errors.New("not implemented")
}

func (r *stubApiKeyRepo) ListKeysByUserID(ctx context.Context, userID int64) ([]string, error) {
	return nil, errors.New("not implemented")
}

func (r *stubApiKeyRepo) ListKeysByGroupID(ctx context.Context, groupID int64) ([]string, error) {
	return nil, errors.New("not implemented")
}

func (r *stubApiKeyRepo) IncrementQuotaUsed(ctx context.Context, id int64, amount float64) (float64, error) {
	return 0, errors.New("not implemented")
}

func (r *stubApiKeyRepo) UpdateLastUsed(ctx context.Context, id int64, usedAt time.Time) error {
	key, ok := r.byID[id]
	if !ok {
		return service.ErrAPIKeyNotFound
	}
	ts := usedAt
	key.LastUsedAt = &ts
	key.UpdatedAt = usedAt
	clone := *key
	r.byID[id] = &clone
	r.byKey[clone.Key] = &clone
	return nil
}

func (r *stubApiKeyRepo) IncrementRateLimitUsage(ctx context.Context, id int64, cost float64) error {
	return nil
}
func (r *stubApiKeyRepo) ResetRateLimitWindows(ctx context.Context, id int64) error {
	return nil
}
func (r *stubApiKeyRepo) GetRateLimitData(ctx context.Context, id int64) (*service.APIKeyRateLimitData, error) {
	return nil, nil
}

type stubUsageLogRepo struct {
	userLogs map[int64][]service.UsageLog
}

func newStubUsageLogRepo() *stubUsageLogRepo {
	return &stubUsageLogRepo{userLogs: make(map[int64][]service.UsageLog)}
}

func (r *stubUsageLogRepo) SetUserLogs(userID int64, logs []service.UsageLog) {
	r.userLogs[userID] = logs
}

func (r *stubUsageLogRepo) Create(ctx context.Context, log *service.UsageLog) (bool, error) {
	return false, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetByID(ctx context.Context, id int64) (*service.UsageLog, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) Delete(ctx context.Context, id int64) error {
	return errors.New("not implemented")
}

func (r *stubUsageLogRepo) ListByUser(ctx context.Context, userID int64, params pagination.PaginationParams) ([]service.UsageLog, *pagination.PaginationResult, error) {
	logs := r.userLogs[userID]
	total := int64(len(logs))
	out := paginateLogs(logs, params)
	return out, paginationResult(total, params), nil
}

func (r *stubUsageLogRepo) ListByAPIKey(ctx context.Context, apiKeyID int64, params pagination.PaginationParams) ([]service.UsageLog, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) ListByAccount(ctx context.Context, accountID int64, params pagination.PaginationParams) ([]service.UsageLog, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) ListByUserAndTimeRange(ctx context.Context, userID int64, startTime, endTime time.Time) ([]service.UsageLog, *pagination.PaginationResult, error) {
	logs := r.userLogs[userID]
	return logs, paginationResult(int64(len(logs)), pagination.PaginationParams{Page: 1, PageSize: 100}), nil
}

func (r *stubUsageLogRepo) ListByAPIKeyAndTimeRange(ctx context.Context, apiKeyID int64, startTime, endTime time.Time) ([]service.UsageLog, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) ListByAccountAndTimeRange(ctx context.Context, accountID int64, startTime, endTime time.Time) ([]service.UsageLog, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) ListByModelAndTimeRange(ctx context.Context, modelName string, startTime, endTime time.Time) ([]service.UsageLog, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetAccountWindowStats(ctx context.Context, accountID int64, startTime time.Time) (*usagestats.AccountStats, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetAccountTodayStats(ctx context.Context, accountID int64) (*usagestats.AccountStats, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetDashboardStats(ctx context.Context) (*usagestats.DashboardStats, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetUsageTrendWithFilters(ctx context.Context, startTime, endTime time.Time, granularity string, userID, apiKeyID, accountID, groupID int64, model string, requestType *int16, stream *bool, billingType *int8) ([]usagestats.TrendDataPoint, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetModelStatsWithFilters(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, requestType *int16, stream *bool, billingType *int8) ([]usagestats.ModelStat, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetEndpointStatsWithFilters(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, model string, requestType *int16, stream *bool, billingType *int8) ([]usagestats.EndpointStat, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetUpstreamEndpointStatsWithFilters(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, model string, requestType *int16, stream *bool, billingType *int8) ([]usagestats.EndpointStat, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetGroupStatsWithFilters(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, requestType *int16, stream *bool, billingType *int8) ([]usagestats.GroupStat, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetUserBreakdownStats(ctx context.Context, startTime, endTime time.Time, dim usagestats.UserBreakdownDimension, limit int) ([]usagestats.UserBreakdownItem, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetAPIKeyUsageTrend(ctx context.Context, startTime, endTime time.Time, granularity string, limit int) ([]usagestats.APIKeyUsageTrendPoint, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetUserUsageTrend(ctx context.Context, startTime, endTime time.Time, granularity string, limit int) ([]usagestats.UserUsageTrendPoint, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetUserSpendingRanking(ctx context.Context, startTime, endTime time.Time, limit int) (*usagestats.UserSpendingRankingResponse, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetUserStatsAggregated(ctx context.Context, userID int64, startTime, endTime time.Time) (*usagestats.UsageStats, error) {
	logs := r.userLogs[userID]
	if len(logs) == 0 {
		return &usagestats.UsageStats{}, nil
	}

	var totalRequests int64
	var totalInputTokens int64
	var totalOutputTokens int64
	var totalCacheTokens int64
	var totalCost float64
	var totalActualCost float64
	var totalDuration int64
	var durationCount int64

	for _, log := range logs {
		totalRequests++
		totalInputTokens += int64(log.InputTokens)
		totalOutputTokens += int64(log.OutputTokens)
		totalCacheTokens += int64(log.CacheCreationTokens + log.CacheReadTokens)
		totalCost += log.TotalCost
		totalActualCost += log.ActualCost
		if log.DurationMs != nil {
			totalDuration += int64(*log.DurationMs)
			durationCount++
		}
	}

	var avgDuration float64
	if durationCount > 0 {
		avgDuration = float64(totalDuration) / float64(durationCount)
	}

	return &usagestats.UsageStats{
		TotalRequests:     totalRequests,
		TotalInputTokens:  totalInputTokens,
		TotalOutputTokens: totalOutputTokens,
		TotalCacheTokens:  totalCacheTokens,
		TotalTokens:       totalInputTokens + totalOutputTokens + totalCacheTokens,
		TotalCost:         totalCost,
		TotalActualCost:   totalActualCost,
		AverageDurationMs: avgDuration,
	}, nil
}

func (r *stubUsageLogRepo) GetAPIKeyStatsAggregated(ctx context.Context, apiKeyID int64, startTime, endTime time.Time) (*usagestats.UsageStats, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetAccountStatsAggregated(ctx context.Context, accountID int64, startTime, endTime time.Time) (*usagestats.UsageStats, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetModelStatsAggregated(ctx context.Context, modelName string, startTime, endTime time.Time) (*usagestats.UsageStats, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetDailyStatsAggregated(ctx context.Context, userID int64, startTime, endTime time.Time) ([]map[string]any, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetBatchUserUsageStats(ctx context.Context, userIDs []int64, startTime, endTime time.Time) (map[int64]*usagestats.BatchUserUsageStats, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetBatchAPIKeyUsageStats(ctx context.Context, apiKeyIDs []int64, startTime, endTime time.Time) (map[int64]*usagestats.BatchAPIKeyUsageStats, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetUserDashboardStats(ctx context.Context, userID int64) (*usagestats.UserDashboardStats, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetAPIKeyDashboardStats(ctx context.Context, apiKeyID int64) (*usagestats.UserDashboardStats, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetUserUsageTrendByUserID(ctx context.Context, userID int64, startTime, endTime time.Time, granularity string) ([]usagestats.TrendDataPoint, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetUserModelStats(ctx context.Context, userID int64, startTime, endTime time.Time) ([]usagestats.ModelStat, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) ListWithFilters(ctx context.Context, params pagination.PaginationParams, filters usagestats.UsageLogFilters) ([]service.UsageLog, *pagination.PaginationResult, error) {
	logs := r.userLogs[filters.UserID]

	// Apply filters
	var filtered []service.UsageLog
	for _, log := range logs {
		// Apply APIKeyID filter
		if filters.APIKeyID > 0 && log.APIKeyID != filters.APIKeyID {
			continue
		}
		// Apply Model filter
		if filters.Model != "" && log.Model != filters.Model {
			continue
		}
		// Apply Stream filter
		if filters.Stream != nil && log.Stream != *filters.Stream {
			continue
		}
		// Apply BillingType filter
		if filters.BillingType != nil && log.BillingType != *filters.BillingType {
			continue
		}
		// Apply time range filters
		if filters.StartTime != nil && log.CreatedAt.Before(*filters.StartTime) {
			continue
		}
		if filters.EndTime != nil && log.CreatedAt.After(*filters.EndTime) {
			continue
		}
		filtered = append(filtered, log)
	}

	total := int64(len(filtered))
	out := paginateLogs(filtered, params)
	return out, paginationResult(total, params), nil
}

func (r *stubUsageLogRepo) GetGlobalStats(ctx context.Context, startTime, endTime time.Time) (*usagestats.UsageStats, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetAccountUsageStats(ctx context.Context, accountID int64, startTime, endTime time.Time) (*usagestats.AccountUsageStatsResponse, error) {
	return nil, errors.New("not implemented")
}

func (r *stubUsageLogRepo) GetStatsWithFilters(ctx context.Context, filters usagestats.UsageLogFilters) (*usagestats.UsageStats, error) {
	return nil, errors.New("not implemented")
}
func (r *stubUsageLogRepo) GetAllGroupUsageSummary(ctx context.Context, todayStart time.Time) ([]usagestats.GroupUsageSummary, error) {
	return nil, errors.New("not implemented")
}

type stubSettingRepo struct {
	all map[string]string
}

func newStubSettingRepo() *stubSettingRepo {
	return &stubSettingRepo{all: make(map[string]string)}
}

func (r *stubSettingRepo) SetAll(values map[string]string) {
	r.all = make(map[string]string, len(values))
	for k, v := range values {
		r.all[k] = v
	}
}

func (r *stubSettingRepo) Get(ctx context.Context, key string) (*service.Setting, error) {
	value, ok := r.all[key]
	if !ok {
		return nil, service.ErrSettingNotFound
	}
	return &service.Setting{Key: key, Value: value}, nil
}

func (r *stubSettingRepo) GetValue(ctx context.Context, key string) (string, error) {
	value, ok := r.all[key]
	if !ok {
		return "", service.ErrSettingNotFound
	}
	return value, nil
}

func (r *stubSettingRepo) Set(ctx context.Context, key, value string) error {
	r.all[key] = value
	return nil
}

func (r *stubSettingRepo) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		out[key] = r.all[key]
	}
	return out, nil
}

func (r *stubSettingRepo) SetMultiple(ctx context.Context, settings map[string]string) error {
	for k, v := range settings {
		r.all[k] = v
	}
	return nil
}

func (r *stubSettingRepo) GetAll(ctx context.Context) (map[string]string, error) {
	out := make(map[string]string, len(r.all))
	for k, v := range r.all {
		out[k] = v
	}
	return out, nil
}

func (r *stubSettingRepo) Delete(ctx context.Context, key string) error {
	delete(r.all, key)
	return nil
}

func paginateLogs(logs []service.UsageLog, params pagination.PaginationParams) []service.UsageLog {
	start := params.Offset()
	if start > len(logs) {
		start = len(logs)
	}
	end := start + params.Limit()
	if end > len(logs) {
		end = len(logs)
	}
	out := make([]service.UsageLog, 0, end-start)
	out = append(out, logs[start:end]...)
	return out
}

func paginationResult(total int64, params pagination.PaginationParams) *pagination.PaginationResult {
	pageSize := params.Limit()
	pages := int(math.Ceil(float64(total) / float64(pageSize)))
	if pages < 1 {
		pages = 1
	}
	return &pagination.PaginationResult{
		Total:    total,
		Page:     params.Page,
		PageSize: pageSize,
		Pages:    pages,
	}
}

// Ensure compile-time interface compliance.
var (
	_ service.UserRepository             = (*stubUserRepo)(nil)
	_ service.APIKeyRepository           = (*stubApiKeyRepo)(nil)
	_ service.APIKeyCache                = (*stubApiKeyCache)(nil)
	_ service.GroupRepository            = (*stubGroupRepo)(nil)
	_ service.UserSubscriptionRepository = (*stubUserSubscriptionRepo)(nil)
	_ service.UsageLogRepository         = (*stubUsageLogRepo)(nil)
	_ service.SettingRepository          = (*stubSettingRepo)(nil)
)
