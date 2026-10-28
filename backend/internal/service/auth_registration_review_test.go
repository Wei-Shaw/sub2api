//go:build unit

package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type registrationReviewSettingRepo struct {
	*settingRepoStub
	reviewErr error
}

func (r *registrationReviewSettingRepo) GetValue(ctx context.Context, key string) (string, error) {
	if key == SettingKeyRegistrationReviewEnabled {
		return "", r.reviewErr
	}
	return r.settingRepoStub.GetValue(ctx, key)
}

func TestRegistrationReview_SettingFailureRejectsSignup(t *testing.T) {
	repo := &userRepoStub{nextID: 84}
	svc := newAuthService(repo, map[string]string{SettingKeyRegistrationEnabled: "true"}, nil, nil)
	svc.settingService.settingRepo = &registrationReviewSettingRepo{settingRepoStub: &settingRepoStub{values: map[string]string{SettingKeyRegistrationEnabled: "true"}}, reviewErr: errors.New("database unavailable")}
	_, _, err := svc.RegisterWithVerification(context.Background(), "review@example.com", "password", "", "", "", "", "reason")
	require.ErrorIs(t, err, ErrServiceUnavailable)
	require.Empty(t, repo.created)
}

func TestRegistrationReview_LocalReasonValidation(t *testing.T) {
	for _, tc := range []struct {
		name, reason, code string
		review             bool
	}{
		{"empty", "", "REGISTRATION_REASON_REQUIRED", true},
		{"whitespace", " \n\t", "REGISTRATION_REASON_REQUIRED", true},
		{"500 Chinese characters", strings.Repeat("申", 500), "REGISTRATION_PENDING", true},
		{"501 Chinese characters", strings.Repeat("申", 501), "REGISTRATION_REASON_TOO_LONG", true},
		{"too long with review disabled", strings.Repeat("申", 501), "REGISTRATION_REASON_TOO_LONG", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &userRepoStub{nextID: 84}
			settings := map[string]string{SettingKeyRegistrationEnabled: "true", SettingKeyRegistrationReviewEnabled: "false"}
			if tc.review {
				settings[SettingKeyRegistrationReviewEnabled] = "true"
			}
			svc := newAuthService(repo, settings, nil, nil)
			token, user, err := svc.RegisterWithVerification(context.Background(), "review@example.com", "password", "", "", "", "", tc.reason)
			require.Error(t, err)
			require.Equal(t, tc.code, infraerrors.Reason(err))
			require.Empty(t, token)
			if tc.code == "REGISTRATION_PENDING" {
				require.Equal(t, StatusPending, user.Status)
				require.Equal(t, tc.reason, user.RegistrationReason)
			} else {
				require.Empty(t, repo.created)
			}
		})
	}
}

func TestRegistrationReview_ValidatesReasonBeforeConsumingEmailCode(t *testing.T) {
	repo := &userRepoStub{}
	cache := &reviewEmailCache{emailCacheStub: &emailCacheStub{data: &VerificationCodeData{Code: "valid-code"}}}
	svc := newAuthService(repo, map[string]string{SettingKeyRegistrationEnabled: "true", SettingKeyRegistrationReviewEnabled: "true", SettingKeyEmailVerifyEnabled: "true"}, cache, nil)
	_, _, err := svc.RegisterWithVerification(context.Background(), "review@example.com", "password", "valid-code", "", "", "", " ")
	require.Equal(t, "REGISTRATION_REASON_REQUIRED", infraerrors.Reason(err))
	require.Empty(t, repo.created)
	require.Zero(t, cache.deletions)
}

type reviewEmailCache struct {
	*emailCacheStub
	deletions int
}

func (c *reviewEmailCache) DeleteVerificationCode(context.Context, string) error {
	c.deletions++
	return nil
}

func TestRegistrationReview_MissingSettingDefaultsToActive(t *testing.T) {
	svc := newAuthService(&userRepoStub{}, map[string]string{SettingKeyRegistrationEnabled: "true"}, nil, nil)
	status, reason, err := svc.resolveRegistrationStatus(context.Background(), "  optional reason  ")
	require.NoError(t, err)
	require.Equal(t, StatusActive, status)
	require.Equal(t, "optional reason", reason)
}

func TestRegistrationReview_PendingLocalRegistrationKeepsSignupGrants(t *testing.T) {
	repo := &userRepoStub{nextID: 84}
	quotaRepo := &userPlatformQuotaRepoStub{}
	assigner := &defaultSubscriptionAssignerStub{}
	svc := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled: "true", SettingKeyRegistrationReviewEnabled: "true",
		SettingKeyDefaultSubscriptions:  `[{"group_id":91,"validity_days":3}]`,
		SettingKeyDefaultPlatformQuotas: `{"openai":{"weekly":12.34}}`,
	}, nil, quotaRepo)
	svc.defaultSubAssigner = assigner
	token, user, err := svc.RegisterWithVerification(context.Background(), "review@example.com", "password", "", "", "", "", "  申请使用  ")
	require.ErrorIs(t, err, ErrRegistrationPending)
	require.Empty(t, token)
	require.Equal(t, StatusPending, user.Status)
	require.Equal(t, "申请使用", user.RegistrationReason)
	require.Equal(t, 3.5, user.Balance)
	require.Equal(t, 2, user.Concurrency)
	require.Len(t, assigner.calls, 1)
	require.Equal(t, int64(91), assigner.calls[0].GroupID)
	require.Len(t, quotaRepo.bulkInsertCalls, 1)
}

func TestRegistrationReview_AutomaticOAuthRequiresCompletion(t *testing.T) {
	repo := &userRepoStub{nextID: 84}
	svc := newAuthService(repo, map[string]string{SettingKeyRegistrationEnabled: "true", SettingKeyRegistrationReviewEnabled: "true"}, nil, nil)
	user, err := svc.createEmailOAuthUser(context.Background(), "review@example.com", "review", "google", "", "")
	require.Equal(t, "REGISTRATION_REASON_REQUIRED", infraerrors.Reason(err))
	require.Nil(t, user)
	require.Empty(t, repo.created)
}

func TestRegistrationReview_OAuthRequiresReason(t *testing.T) {
	for _, flow := range []string{"legacy", "pair", "promo pair", "verified email", "local email"} {
		t.Run(flow, func(t *testing.T) {
			repo := &userRepoStub{nextID: 84}
			svc := newAuthService(repo, map[string]string{SettingKeyRegistrationEnabled: "true", SettingKeyRegistrationReviewEnabled: "true"}, nil, nil)
			svc.refreshTokenCache = &refreshTokenCacheStub{}
			var err error
			switch flow {
			case "legacy":
				_, _, err = svc.LoginOrRegisterOAuth(context.Background(), "review@example.com", "review")
			case "pair":
				_, _, err = svc.LoginOrRegisterOAuthWithTokenPair(context.Background(), "review@example.com", "review", "", "", "oidc")
			case "promo pair":
				_, _, err = svc.LoginOrRegisterOAuthWithTokenPairAndPromoCode(context.Background(), "review@example.com", "review", "", "", "", "oidc")
			case "verified email":
				_, _, err = svc.RegisterVerifiedOAuthEmailAccount(context.Background(), "review@example.com", "password", "", "google")
			case "local email":
				_, _, err = svc.RegisterOAuthEmailAccount(context.Background(), "review@example.com", "password", "code", "", "google")
			}
			require.Equal(t, "REGISTRATION_REASON_REQUIRED", infraerrors.Reason(err))
			require.Empty(t, repo.created)
		})
	}
}

func TestRegistrationReview_ExistingPendingOAuthCannotLogin(t *testing.T) {
	for _, flow := range []string{"legacy", "pair"} {
		t.Run(flow, func(t *testing.T) {
			repo := &userRepoStub{user: &User{ID: 84, Email: "review@example.com", Role: RoleUser, Status: StatusPending}}
			svc := newAuthService(repo, map[string]string{SettingKeyRegistrationEnabled: "false", SettingKeyRegistrationReviewEnabled: "true"}, nil, nil)
			svc.refreshTokenCache = &refreshTokenCacheStub{}
			var err error
			if flow == "legacy" {
				_, _, err = svc.LoginOrRegisterOAuth(context.Background(), "review@example.com", "")
			} else {
				_, _, err = svc.LoginOrRegisterOAuthWithTokenPair(context.Background(), "review@example.com", "", "", "", "oidc")
			}
			require.ErrorIs(t, err, ErrRegistrationPending)
			require.Empty(t, repo.created)
		})
	}
}

func TestRegistrationReview_OAuthCompletionCreatesPendingWithoutTokens(t *testing.T) {
	for _, flow := range []string{"legacy", "pair", "promo pair", "verified email", "local email"} {
		t.Run(flow, func(t *testing.T) {
			repo := &userRepoStub{nextID: 84}
			cache := &emailCacheStub{data: &VerificationCodeData{Code: "123456", ExpiresAt: time.Now().Add(time.Hour)}}
			svc := newAuthService(repo, map[string]string{SettingKeyRegistrationEnabled: "true", SettingKeyRegistrationReviewEnabled: "true", SettingKeyDefaultBalance: "4"}, cache, nil)
			// Pending registration must succeed without any token cache.
			var pair *TokenPair
			var user *User
			var token string
			var err error
			switch flow {
			case "legacy":
				token, user, err = svc.LoginOrRegisterOAuth(context.Background(), "review@example.com", "review", "  申请使用  ")
			case "pair":
				pair, user, err = svc.LoginOrRegisterOAuthWithTokenPair(context.Background(), "review@example.com", "review", "", "", "oidc", "  申请使用  ")
			case "promo pair":
				pair, user, err = svc.LoginOrRegisterOAuthWithTokenPairAndPromoCode(context.Background(), "review@example.com", "review", "", "", "", "oidc", "  申请使用  ")
			case "verified email":
				pair, user, err = svc.RegisterVerifiedOAuthEmailAccount(context.Background(), "review@example.com", "password", "", "google", "  申请使用  ")
			case "local email":
				pair, user, err = svc.RegisterOAuthEmailAccount(context.Background(), "review@example.com", "password", "123456", "", "google", "  申请使用  ")
			}
			require.NoError(t, err)
			require.Nil(t, pair)
			require.Empty(t, token)
			require.NotNil(t, user)
			require.Equal(t, StatusPending, user.Status)
			require.Equal(t, "申请使用", user.RegistrationReason)
			require.Len(t, repo.created, 1)
		})
	}
}

func TestRegistrationReview_ExistingActiveOAuthLoginIgnoresSignupRequirements(t *testing.T) {
	for _, flow := range []string{"legacy", "pair"} {
		t.Run(flow, func(t *testing.T) {
			repo := &userRepoStub{user: &User{ID: 84, Email: "review@example.com", Role: RoleUser, Status: StatusActive, Username: "review"}}
			svc := newAuthService(repo, map[string]string{SettingKeyRegistrationEnabled: "false", SettingKeyRegistrationReviewEnabled: "true"}, nil, nil)
			svc.refreshTokenCache = &refreshTokenCacheStub{}
			if flow == "legacy" {
				token, user, err := svc.LoginOrRegisterOAuth(context.Background(), "review@example.com", "review")
				require.NoError(t, err)
				require.NotEmpty(t, token)
				require.Equal(t, int64(84), user.ID)
			} else {
				pair, user, err := svc.LoginOrRegisterOAuthWithTokenPair(context.Background(), "review@example.com", "review", "", "", "oidc")
				require.NoError(t, err)
				require.NotEmpty(t, pair.AccessToken)
				require.Equal(t, int64(84), user.ID)
			}
			require.Empty(t, repo.created)
		})
	}
}

func TestRegistrationReview_TokenIssuanceRejectsNonActive(t *testing.T) {
	for _, status := range []string{StatusPending, StatusDisabled} {
		t.Run(status, func(t *testing.T) {
			svc := newAuthService(&userRepoStub{}, map[string]string{}, nil, nil)
			svc.refreshTokenCache = &refreshTokenCacheStub{}
			user := &User{ID: 84, Email: "review@example.com", Role: RoleUser, Status: status}
			token, err := svc.GenerateToken(context.Background(), user)
			require.Error(t, err)
			require.Empty(t, token)
			pair, pairErr := svc.GenerateTokenPair(context.Background(), user, "")
			require.Error(t, pairErr)
			require.Nil(t, pair)
			code := "USER_NOT_ACTIVE"
			if status == StatusPending {
				code = "REGISTRATION_PENDING"
			}
			require.Equal(t, code, infraerrors.Reason(err))
			require.Equal(t, code, infraerrors.Reason(pairErr))
		})
	}
}

func TestRegistrationReview_AdminStatusGuard(t *testing.T) {
	for _, actor := range []int64{42, 99} {
		for _, status := range []string{StatusPending, StatusDisabled, "unknown"} {
			repo := &rpmUserRepoStub{userRepoStub: &userRepoStub{user: &User{ID: 42, Role: RoleAdmin, Status: StatusActive}}}
			svc := &adminServiceImpl{userRepo: repo}
			_, err := svc.UpdateUser(context.Background(), 42, &UpdateUserInput{Status: status, ActorAdminID: actor})
			require.Error(t, err, "actor=%d status=%s", actor, status)
			require.Nil(t, repo.lastUpdated)
		}
	}
}

func TestRegistrationReview_AdminPromotionRequiresActiveStatus(t *testing.T) {
	for _, tc := range []struct{ name, status, requestedStatus string }{
		{"pending account", StatusPending, ""},
		{"disabled account", StatusDisabled, ""},
		{"active account requested pending", StatusActive, StatusPending},
		{"active account requested disabled", StatusActive, StatusDisabled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &rpmUserRepoStub{userRepoStub: &userRepoStub{user: &User{ID: 42, Role: RoleUser, Status: tc.status}}}
			svc := &adminServiceImpl{userRepo: repo}
			_, err := svc.UpdateUser(context.Background(), 42, &UpdateUserInput{Role: RoleAdmin, Status: tc.requestedStatus})
			require.Error(t, err)
			require.Nil(t, repo.lastUpdated)
			require.Equal(t, RoleUser, repo.user.Role, "rejected update must not mutate the repository user")
			require.Equal(t, tc.status, repo.user.Status)
		})
	}
}

func TestRegistrationReview_AdminPromotionCanApprovePendingAccount(t *testing.T) {
	repo := &rpmUserRepoStub{userRepoStub: &userRepoStub{user: &User{ID: 42, Role: RoleUser, Status: StatusPending}}}
	svc := &adminServiceImpl{userRepo: repo}
	user, err := svc.UpdateUser(context.Background(), 42, &UpdateUserInput{Role: RoleAdmin, Status: StatusActive})
	require.NoError(t, err)
	require.Equal(t, RoleAdmin, user.Role)
	require.Equal(t, StatusActive, user.Status)
}

func TestRegistrationReview_ConfigurableReasonLimit(t *testing.T) {
	for _, tc := range []struct{ limit, reason, want string }{
		{"3", "申申申", ""},
		{"3", "申申申申", "REGISTRATION_REASON_TOO_LONG"},
		{"3", "😀😀😀", ""},
		{"1000", strings.Repeat("申", 600), ""},
		{"bad", strings.Repeat("申", 501), "REGISTRATION_REASON_TOO_LONG"},
	} {
		t.Run(tc.limit+tc.want, func(t *testing.T) {
			svc := newAuthService(&userRepoStub{}, map[string]string{SettingKeyRegistrationReviewEnabled: "true", "registration_reason_max_length": tc.limit}, nil, nil)
			status, _, err := svc.resolveRegistrationStatus(context.Background(), tc.reason)
			if tc.want != "" {
				require.Equal(t, tc.want, infraerrors.Reason(err))
				return
			}
			require.NoError(t, err)
			require.Equal(t, StatusPending, status)
		})
	}
}
