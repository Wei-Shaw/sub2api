package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/authidentity"
	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func reviewOAuthSession(t *testing.T, client *dbent.Client, provider string) *dbent.PendingAuthSession {
	t.Helper()
	session, err := client.PendingAuthSession.Create().
		SetSessionToken("review-session").SetBrowserSessionKey("review-browser").
		SetIntent(oauthIntentLogin).SetProviderType(provider).SetProviderKey(provider).
		SetProviderSubject("review-subject").SetResolvedEmail("review@example.com").
		SetUpstreamIdentityClaims(map[string]any{"username": "review-user", "email_verified": true}).
		SetExpiresAt(time.Now().Add(10 * time.Minute)).Save(context.Background())
	require.NoError(t, err)
	return session
}

func reviewOAuthContext(t *testing.T, body string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/complete-registration", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.AddCookie(encodedCookie(oauthPendingSessionCookieName, "review-session"))
	c.Request.AddCookie(encodedCookie(oauthPendingBrowserCookieName, "review-browser"))
	return c, recorder
}

func TestOAuthReviewLocalEmailVerificationIsIndependentOfRegistrationToggle(t *testing.T) {
	for _, provider := range []string{"linuxdo", "oidc", "wechat", "dingtalk"} {
		for _, tc := range []struct {
			name, code, reason string
			status             int
		}{
			{"missing code", "", "EMAIL_VERIFY_REQUIRED", http.StatusBadRequest},
			{"invalid code", "000000", "INVALID_VERIFY_CODE", http.StatusBadRequest},
			{"verified application", "246810", "", http.StatusOK},
		} {
			t.Run(provider+"/"+tc.name, func(t *testing.T) {
				handler, client := newOAuthPendingFlowTestHandlerWithDependencies(t, oauthPendingFlowTestHandlerOptions{
					emailVerifyEnabled: false,
					settingValues: map[string]string{
						service.SettingKeyRegistrationReviewEnabled: "true",
					},
					emailCache: &oauthPendingFlowEmailCacheStub{verificationCodes: map[string]*service.VerificationCodeData{
						"review@example.com": {Code: "246810", ExpiresAt: time.Now().Add(15 * time.Minute)},
					}},
				})
				session := reviewOAuthSession(t, client, provider)
				body, err := json.Marshal(map[string]string{
					"email": "review@example.com", "password": "secret-123",
					"registration_reason": "Research access", "verify_code": tc.code,
				})
				require.NoError(t, err)
				c, recorder := reviewOAuthContext(t, string(body))
				handler.CreatePendingOAuthAccount(c)
				require.Equal(t, tc.status, recorder.Code, recorder.Body.String())
				storedSession, err := client.PendingAuthSession.Get(context.Background(), session.ID)
				require.NoError(t, err)
				if tc.reason != "" {
					require.Contains(t, recorder.Body.String(), tc.reason)
					count, err := client.User.Query().Count(context.Background())
					require.NoError(t, err)
					require.Zero(t, count)
					require.Nil(t, storedSession.ConsumedAt)
					return
				}
				var payload map[string]any
				require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
				require.Equal(t, true, payload["pending_review"])
				require.NotContains(t, payload, "access_token")
				require.NotContains(t, payload, "refresh_token")
				user, err := client.User.Query().Only(context.Background())
				require.NoError(t, err)
				require.Equal(t, "pending", user.Status)
				require.Equal(t, "Research access", user.RegistrationReason)
				identity, err := client.AuthIdentity.Query().Where(authidentity.UserIDEQ(user.ID)).Only(context.Background())
				require.NoError(t, err)
				require.Equal(t, provider, identity.ProviderType)
				require.NotNil(t, storedSession.ConsumedAt)
			})
		}
	}
}

// Losing reason forwarding or returning before binding/consumption leaves an unusable application.
func TestOAuthRegistrationReviewCreatesBoundPendingAccountWithoutTokens(t *testing.T) {
	for _, provider := range []string{"github", "google", "oidc"} {
		t.Run(provider, func(t *testing.T) {
			promoRepo := newOAuthPendingFlowPromoRepoStub("REVIEWBONUS", 25)
			handler, client := newOAuthPendingFlowTestHandlerWithDependencies(t, oauthPendingFlowTestHandlerOptions{
				invitationEnabled: true,
				promoRepo:         promoRepo,
				settingValues: map[string]string{
					service.SettingKeyRegistrationReviewEnabled: "true",
					service.SettingKeyPromoCodeEnabled:          "true",
				},
				emailVerifyEnabled: true,
				emailCache: &oauthPendingFlowEmailCacheStub{verificationCodes: map[string]*service.VerificationCodeData{
					"review@example.com": {Code: "246810", ExpiresAt: time.Now().Add(15 * time.Minute)},
				}},
			})
			invitation, err := client.RedeemCode.Create().SetCode("REVIEWINVITE").SetType(service.RedeemTypeInvitation).SetStatus(service.StatusUnused).SetValue(0).Save(context.Background())
			require.NoError(t, err)
			session := reviewOAuthSession(t, client, provider)
			session, err = client.PendingAuthSession.UpdateOneID(session.ID).SetLocalFlowState(map[string]any{oauthPromoCodeStateKey: "REVIEWBONUS"}).Save(context.Background())
			require.NoError(t, err)
			c, recorder := reviewOAuthContext(t, `{"email":"review@example.com","verify_code":"246810","password":"secret-123","invitation_code":"REVIEWINVITE","registration_reason":"  Research access  "}`)
			if provider == "oidc" {
				handler.CreateOIDCOAuthAccount(c)
			} else {
				handler.completeEmailOAuthRegistration(c, provider)
			}
			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			var payload map[string]any
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
			require.Equal(t, true, payload["pending_review"])
			require.NotContains(t, payload, "access_token")
			require.NotContains(t, payload, "refresh_token")
			user, err := client.User.Query().Only(context.Background())
			require.NoError(t, err)
			require.Equal(t, "pending", user.Status)
			require.Equal(t, "Research access", user.RegistrationReason)
			require.Nil(t, user.LastLoginAt)
			require.Equal(t, float64(25), user.Balance)
			storedInvitation, err := client.RedeemCode.Get(context.Background(), invitation.ID)
			require.NoError(t, err)
			require.NotNil(t, storedInvitation.UsedBy)
			require.Equal(t, user.ID, *storedInvitation.UsedBy)
			identity, err := client.AuthIdentity.Query().Only(context.Background())
			require.NoError(t, err)
			require.Equal(t, user.ID, identity.UserID)
			storedSession, err := client.PendingAuthSession.Get(context.Background(), session.ID)
			require.NoError(t, err)
			require.NotNil(t, storedSession.ConsumedAt)
			requireCookieCleared(t, recorder, oauthPendingSessionCookieName)

			// The same identity must deny login while pending, then work after approval.
			_, err = handler.findOAuthIdentityUser(context.Background(), service.PendingAuthIdentityKey{
				ProviderType: provider, ProviderKey: provider, ProviderSubject: "review-subject",
			})
			require.Equal(t, "REGISTRATION_PENDING", infraerrors.Reason(err))
			_, err = client.User.UpdateOneID(user.ID).SetStatus(service.StatusActive).Save(context.Background())
			require.NoError(t, err)
			pair, err := handler.authService.GenerateTokenPair(context.Background(), oauthPendingFlowServiceUser(mustReviewUser(t, client, user.ID)), "")
			require.NoError(t, err)
			require.NotEmpty(t, pair.AccessToken)
		})
	}
}

func mustReviewUser(t *testing.T, client *dbent.Client, id int64) *dbent.User {
	t.Helper()
	u, err := client.User.Get(context.Background(), id)
	require.NoError(t, err)
	return u
}

func TestOAuthReviewCompletionAdvertisesReasonAndActiveLoginStillWorks(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandlerWithDependencies(t, oauthPendingFlowTestHandlerOptions{
		settingValues: map[string]string{service.SettingKeyRegistrationReviewEnabled: "true"},
	})
	c, recorder := reviewOAuthContext(t, "")
	c.Request.Method = http.MethodGet
	handler.emailOAuthCallbackWithProfile(c, "github", config.EmailOAuthProviderConfig{}, "/auth/oauth/callback", "/dashboard", &emailOAuthProfile{
		Subject: "review-subject", Email: "review@example.com", EmailVerified: true, Username: "review-user",
	})
	require.Equal(t, http.StatusFound, c.Writer.Status())
	session, err := client.PendingAuthSession.Query().Only(context.Background())
	require.NoError(t, err)
	completion, ok := readCompletionResponse(session.LocalFlowState)
	require.True(t, ok)
	require.Equal(t, true, completion["registration_review_enabled"])

	user, err := client.User.Create().SetEmail("active@example.com").SetPasswordHash("hash").SetRole(service.RoleUser).SetStatus(service.StatusActive).Save(context.Background())
	require.NoError(t, err)
	_, err = client.AuthIdentity.Create().SetUserID(user.ID).SetProviderType("github").SetProviderKey("github").SetProviderSubject("active-subject").Save(context.Background())
	require.NoError(t, err)
	c, recorder = reviewOAuthContext(t, "")
	c.Request.Method = http.MethodGet
	handler.emailOAuthCallbackWithProfile(c, "github", config.EmailOAuthProviderConfig{}, "/auth/oauth/callback", "/dashboard", &emailOAuthProfile{
		Subject: "active-subject", Email: "active@example.com", EmailVerified: true, Username: "active-user",
	})
	require.Contains(t, recorder.Header().Get("Location"), "access_token=")
	count, err := client.AuthIdentity.Query().Where(authidentity.UserIDEQ(user.ID), authidentity.ProviderTypeEQ("github")).Count(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestLegacyOAuthRegistrationReviewRequiresReasonAndReturnsPending(t *testing.T) {
	for _, provider := range []string{"linuxdo", "oidc", "wechat", "dingtalk"} {
		for _, reason := range []string{"", "Research access"} {
			t.Run(provider+"/"+reason, func(t *testing.T) {
				handler, client := newOAuthPendingFlowTestHandlerWithDependencies(t, oauthPendingFlowTestHandlerOptions{
					settingValues: map[string]string{service.SettingKeyRegistrationReviewEnabled: "true"},
				})
				session := reviewOAuthSession(t, client, provider)
				body, err := json.Marshal(map[string]string{"invitation_code": "IGNORED", "registration_reason": reason})
				require.NoError(t, err)
				c, recorder := reviewOAuthContext(t, string(body))
				switch provider {
				case "linuxdo":
					handler.CompleteLinuxDoOAuthRegistration(c)
				case "oidc":
					handler.CompleteOIDCOAuthRegistration(c)
				case "wechat":
					handler.CompleteWeChatOAuthRegistration(c)
				case "dingtalk":
					handler.CompleteDingTalkOAuthRegistration(c)
				}
				if reason == "" {
					require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
					require.Contains(t, recorder.Body.String(), "REGISTRATION_REASON_REQUIRED")
					count, err := client.User.Query().Count(context.Background())
					require.NoError(t, err)
					require.Zero(t, count)
				} else {
					require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
					var payload map[string]any
					require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
					require.Equal(t, true, payload["pending_review"])
					require.NotContains(t, payload, "access_token")
					user, err := client.User.Query().Only(context.Background())
					require.NoError(t, err)
					require.Equal(t, "pending", user.Status)
					identity, err := client.AuthIdentity.Query().Only(context.Background())
					require.NoError(t, err)
					require.Equal(t, user.ID, identity.UserID)
				}
				stored, err := client.PendingAuthSession.Get(context.Background(), session.ID)
				require.NoError(t, err)
				if reason == "" {
					require.Nil(t, stored.ConsumedAt)
				} else {
					require.NotNil(t, stored.ConsumedAt)
				}
			})
		}
	}
}

func TestOIDCReviewFastPathAllowsExistingActiveEmailAndDefersNewSignup(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(boolSettingValue(existing), func(t *testing.T) {
			handler, client := newOAuthPendingFlowTestHandlerWithDependencies(t, oauthPendingFlowTestHandlerOptions{
				settingValues: map[string]string{service.SettingKeyRegistrationReviewEnabled: "true"},
			})
			if existing {
				_, err := client.User.Create().SetEmail("review@example.com").SetPasswordHash("hash").SetRole(service.RoleUser).SetStatus(service.StatusActive).Save(context.Background())
				require.NoError(t, err)
			}
			c, recorder := reviewOAuthContext(t, "")
			c.Request.Method = http.MethodGet
			handled := handler.tryOIDCVerifiedEmailFastPath(c, "/auth/oauth/callback", "/dashboard", service.PendingAuthIdentityKey{
				ProviderType: "oidc", ProviderKey: "https://issuer.example", ProviderSubject: "review-subject",
			}, "review@example.com", "review-user", nil)
			require.Equal(t, existing, handled)
			if existing {
				require.Contains(t, recorder.Header().Get("Location"), "access_token=")
			} else {
				count, err := client.User.Query().Count(context.Background())
				require.NoError(t, err)
				require.Zero(t, count)
			}
		})
	}
}

func TestLinuxDoReviewCallbackRoutesNewSignupToCompletion(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/token" {
			_, _ = w.Write([]byte(`{"access_token":"upstream-token","token_type":"Bearer"}`))
		} else {
			_, _ = w.Write([]byte(`{"id":"review-subject","username":"review-user"}`))
		}
	}))
	t.Cleanup(upstream.Close)
	handler, client := newOAuthPendingFlowTestHandlerWithDependencies(t, oauthPendingFlowTestHandlerOptions{
		settingValues: map[string]string{service.SettingKeyRegistrationReviewEnabled: "true"},
	})
	handler.settingSvc = service.NewSettingService(&oauthPendingFlowSettingRepoStub{values: map[string]string{
		service.SettingKeyRegistrationReviewEnabled: "true",
	}}, &config.Config{LinuxDo: config.LinuxDoConnectConfig{
		Enabled: true, ClientID: "client", ClientSecret: "secret", AuthorizeURL: upstream.URL + "/authorize",
		TokenURL: upstream.URL + "/token", UserInfoURL: upstream.URL + "/userinfo",
		RedirectURL: "https://api.example/oauth/callback", FrontendRedirectURL: "/auth/linuxdo/callback", TokenAuthMethod: "client_secret_post",
	}})
	c, recorder := reviewOAuthContext(t, "")
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/linuxdo/callback?code=code&state=state", nil)
	c.Request.AddCookie(encodedCookie(linuxDoOAuthStateCookieName, "state"))
	c.Request.AddCookie(encodedCookie(linuxDoOAuthRedirectCookie, "/dashboard"))
	c.Request.AddCookie(encodedCookie(oauthPendingBrowserCookieName, "review-browser"))
	handler.LinuxDoOAuthCallback(c)
	require.Equal(t, "/auth/linuxdo/callback", recorder.Header().Get("Location"))
	count, err := client.User.Query().Count(context.Background())
	require.NoError(t, err)
	require.Zero(t, count)
	session, err := client.PendingAuthSession.Query().Only(context.Background())
	require.NoError(t, err)
	payload, ok := readCompletionResponse(session.LocalFlowState)
	require.True(t, ok)
	require.Equal(t, true, payload["registration_review_enabled"])
	require.Equal(t, "choose_account_action_required", payload["step"])
}

func TestPendingOAuthBindingRejectsExistingPendingAccount(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandler(t, false)
	user, err := client.User.Create().SetEmail("review@example.com").SetPasswordHash("hash").SetRole(service.RoleUser).SetStatus(service.StatusPending).Save(context.Background())
	require.NoError(t, err)
	session := reviewOAuthSession(t, client, "github")
	err = applyPendingOAuthBinding(context.Background(), client, handler.authService, handler.userService, session, nil, &user.ID, true, false)
	require.Equal(t, "REGISTRATION_PENDING", infraerrors.Reason(err))
	count, err := client.AuthIdentity.Query().Count(context.Background())
	require.NoError(t, err)
	require.Zero(t, count)
}

func TestWeChatReviewCompletionRollsBackAccountOnIdentityConflict(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandlerWithDependencies(t, oauthPendingFlowTestHandlerOptions{
		invitationEnabled: true,
		settingValues:     map[string]string{service.SettingKeyRegistrationReviewEnabled: "true"},
	})
	owner, err := client.User.Create().SetEmail("owner@example.com").SetPasswordHash("hash").SetRole(service.RoleUser).SetStatus(service.StatusActive).Save(context.Background())
	require.NoError(t, err)
	_, err = client.AuthIdentity.Create().SetUserID(owner.ID).SetProviderType("wechat").SetProviderKey("wechat").SetProviderSubject("review-subject").Save(context.Background())
	require.NoError(t, err)
	invitation, err := client.RedeemCode.Create().SetCode("REVIEWINVITE").SetType(service.RedeemTypeInvitation).SetStatus(service.StatusUnused).SetValue(0).Save(context.Background())
	require.NoError(t, err)
	session := reviewOAuthSession(t, client, "wechat")
	c, recorder := reviewOAuthContext(t, `{"invitation_code":"REVIEWINVITE","registration_reason":"Research access"}`)
	handler.CompleteWeChatOAuthRegistration(c)
	require.Equal(t, http.StatusConflict, recorder.Code, recorder.Body.String())
	count, err := client.User.Query().Count(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, count)
	storedInvitation, err := client.RedeemCode.Get(context.Background(), invitation.ID)
	require.NoError(t, err)
	require.Equal(t, service.StatusUnused, storedInvitation.Status)
	require.Nil(t, storedInvitation.UsedBy)
	storedSession, err := client.PendingAuthSession.Get(context.Background(), session.ID)
	require.NoError(t, err)
	require.Nil(t, storedSession.ConsumedAt)
}

func TestDingTalkCallbackReportsPendingRegistrationInsteadOfStartingAnotherSignup(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/token":
			_, _ = w.Write([]byte(`{"accessToken":"upstream-token","corpId":"corp"}`))
		case "/userinfo":
			_, _ = w.Write([]byte(`{"unionId":"review-subject","nick":"review-user"}`))
		default:
			_, _ = w.Write([]byte(`{"errcode":1,"errmsg":"unavailable"}`))
		}
	}))
	t.Cleanup(upstream.Close)
	handler, client := newOAuthPendingFlowTestHandlerWithDependencies(t, oauthPendingFlowTestHandlerOptions{
		settingValues: map[string]string{service.SettingKeyRegistrationReviewEnabled: "true"},
	})
	handler.settingSvc = service.NewSettingService(&oauthPendingFlowSettingRepoStub{values: map[string]string{
		service.SettingKeyRegistrationReviewEnabled: "true",
	}}, &config.Config{DingTalk: config.DingTalkConnectConfig{
		DingTalkAppKind: "internal_app", AppType: "public",
		Enabled: true, ClientID: "client", ClientSecret: "secret", AuthorizeURL: upstream.URL + "/authorize",
		TokenURL: upstream.URL + "/token", UserInfoURL: upstream.URL + "/userinfo",
		RedirectURL: "https://api.example/oauth/callback", FrontendRedirectURL: "/auth/dingtalk/callback", CorpRestrictionPolicy: "none",
	}})
	user, err := client.User.Create().SetEmail("review@example.com").SetPasswordHash("hash").SetRole(service.RoleUser).SetStatus(service.StatusPending).Save(context.Background())
	require.NoError(t, err)
	_, err = client.AuthIdentity.Create().SetUserID(user.ID).SetProviderType("dingtalk").SetProviderKey("dingtalk").SetProviderSubject("review-subject").Save(context.Background())
	require.NoError(t, err)
	c, recorder := reviewOAuthContext(t, "")
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/dingtalk/callback?code=code&state=state", nil)
	c.Request.AddCookie(encodedCookie(dingTalkOAuthStateCookieName, "state"))
	c.Request.AddCookie(encodedCookie(oauthPendingBrowserCookieName, "review-browser"))
	handler.DingTalkOAuthCallback(c)
	require.Contains(t, recorder.Header().Get("Location"), "REGISTRATION_PENDING")
	count, err := client.PendingAuthSession.Query().Count(context.Background())
	require.NoError(t, err)
	require.Zero(t, count)
}

type oauthReviewTxFailureDriver struct {
	dialect.Driver
	failNext bool
}

func (d *oauthReviewTxFailureDriver) Tx(ctx context.Context) (dialect.Tx, error) {
	if d.failNext {
		d.failNext = false
		return nil, errors.New("binding transaction unavailable")
	}
	return d.Driver.Tx(ctx)
}

func TestEmailOAuthReviewTransactionStartupFailureRollsBackAndAllowsRetry(t *testing.T) {
	var failingDriver *oauthReviewTxFailureDriver
	handler, client := newOAuthPendingFlowTestHandlerWithDependencies(t, oauthPendingFlowTestHandlerOptions{
		settingValues: map[string]string{service.SettingKeyRegistrationReviewEnabled: "true"},
		driverWrapper: func(driver dialect.Driver) dialect.Driver {
			failingDriver = &oauthReviewTxFailureDriver{Driver: driver}
			return failingDriver
		},
	})
	session := reviewOAuthSession(t, client, "google")
	failOnce := true
	client.User.Use(func(next dbent.Mutator) dbent.Mutator {
		return dbent.MutateFunc(func(ctx context.Context, mutation dbent.Mutation) (dbent.Value, error) {
			value, err := next.Mutate(ctx, mutation)
			if err == nil && mutation.Op().Is(dbent.OpCreate) && failOnce {
				failOnce = false
				failingDriver.failNext = true
			}
			return value, err
		})
	})
	body := `{"password":"secret-123","registration_reason":"Research access"}`
	c, recorder := reviewOAuthContext(t, body)
	handler.CompleteGoogleOAuthRegistration(c)
	require.Equal(t, http.StatusInternalServerError, recorder.Code, recorder.Body.String())
	count, err := client.User.Query().Count(context.Background())
	require.NoError(t, err)
	require.Zero(t, count)
	storedSession, err := client.PendingAuthSession.Get(context.Background(), session.ID)
	require.NoError(t, err)
	require.Nil(t, storedSession.ConsumedAt)

	c, recorder = reviewOAuthContext(t, body)
	handler.CompleteGoogleOAuthRegistration(c)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), `"pending_review":true`)
	identity, err := client.AuthIdentity.Query().Only(context.Background())
	require.NoError(t, err)
	user, err := client.User.Get(context.Background(), identity.UserID)
	require.NoError(t, err)
	require.Equal(t, "pending", user.Status)
}
