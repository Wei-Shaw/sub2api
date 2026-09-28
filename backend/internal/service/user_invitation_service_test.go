//go:build unit

package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type userInvitationRepoStub struct {
	mu        sync.Mutex
	codes     []RedeemCode
	createErr error
	nextID    int64
}

func (s *userInvitationRepoStub) activeCount(userID int64, now time.Time) int {
	count := 0
	for i := range s.codes {
		c := &s.codes[i]
		if c.CreatedBy == nil || *c.CreatedBy != userID || c.Type != RedeemTypeInvitation {
			continue
		}
		if c.IsUsed() || !c.IsExpiredAt(now) {
			count++
		}
	}
	return count
}

func (s *userInvitationRepoStub) CountActiveInvitationsByCreator(_ context.Context, userID int64, now time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.activeCount(userID, now), nil
}

func (s *userInvitationRepoStub) ListInvitationsByCreator(_ context.Context, userID int64, limit int) ([]RedeemCode, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]RedeemCode, 0)
	for i := len(s.codes) - 1; i >= 0 && len(out) < limit; i-- {
		if c := s.codes[i]; c.CreatedBy != nil && *c.CreatedBy == userID {
			out = append(out, c)
		}
	}
	return out, nil
}

func (s *userInvitationRepoStub) CreateInvitationForUser(_ context.Context, code *RedeemCode, maxCodes int, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.createErr != nil {
		return s.createErr
	}
	if maxCodes > 0 && s.activeCount(*code.CreatedBy, now) >= maxCodes {
		return ErrUserInvitationLimitReached
	}
	s.nextID++
	code.ID = s.nextID
	code.CreatedAt = now
	s.codes = append(s.codes, *code)
	return nil
}

func userInvitationSettings(overrides map[string]string) map[string]string {
	values := map[string]string{
		SettingKeyRegistrationEnabled:            "true",
		SettingKeyInvitationCodeEnabled:          "true",
		SettingKeyUserInvitationEnabled:          "true",
		SettingKeyUserInvitationMaxCodesPerUser:  "2",
		SettingKeyUserInvitationCodeValidityDays: "7",
	}
	for k, v := range overrides {
		values[k] = v
	}
	return values
}

func newUserInvitationServiceForTest(settings map[string]string, repo *userInvitationRepoStub, now time.Time) *UserInvitationService {
	svc := NewUserInvitationService(repo, NewSettingService(&settingRepoStub{values: settings}, nil))
	svc.now = func() time.Time { return now }
	return svc
}

func TestUserInvitationConfig_AvailabilityRequiresAllSwitches(t *testing.T) {
	cases := []struct {
		name      string
		overrides map[string]string
		available bool
	}{
		{name: "all enabled", available: true},
		{name: "user invitation disabled", overrides: map[string]string{SettingKeyUserInvitationEnabled: "false"}},
		{name: "invitation code mode disabled", overrides: map[string]string{SettingKeyInvitationCodeEnabled: "false"}},
		{name: "registration disabled", overrides: map[string]string{SettingKeyRegistrationEnabled: "false"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := userInvitationConfigFromSettings(userInvitationSettings(tc.overrides))
			require.Equal(t, tc.available, cfg.Available)
		})
	}
}

func TestUserInvitationConfig_ParsesLimitsWithDefaultsAndBounds(t *testing.T) {
	cfg := userInvitationConfigFromSettings(map[string]string{})
	require.False(t, cfg.Enabled)
	require.Equal(t, UserInvitationMaxCodesPerUserDefault, cfg.MaxCodesPerUser)
	require.Equal(t, UserInvitationCodeValidityDaysDefault, cfg.CodeValidityDays)

	cfg = userInvitationConfigFromSettings(map[string]string{
		SettingKeyUserInvitationMaxCodesPerUser:  "-3",
		SettingKeyUserInvitationCodeValidityDays: "999999",
	})
	require.Equal(t, 0, cfg.MaxCodesPerUser)
	require.Equal(t, UserInvitationCodeValidityDaysMax, cfg.CodeValidityDays)

	cfg = userInvitationConfigFromSettings(map[string]string{
		SettingKeyUserInvitationMaxCodesPerUser: "abc",
	})
	require.Equal(t, UserInvitationMaxCodesPerUserDefault, cfg.MaxCodesPerUser)
}

func TestUserInvitationService_CreateInvitation(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	repo := &userInvitationRepoStub{}
	svc := newUserInvitationServiceForTest(userInvitationSettings(nil), repo, now)

	code, err := svc.CreateInvitation(context.Background(), 42)
	require.NoError(t, err)
	require.Equal(t, RedeemTypeInvitation, code.Type)
	require.Equal(t, StatusUnused, code.Status)
	require.NotEmpty(t, code.Code)
	require.NotNil(t, code.CreatedBy)
	require.Equal(t, int64(42), *code.CreatedBy)
	require.NotNil(t, code.ExpiresAt)
	require.Equal(t, now.Add(7*24*time.Hour), *code.ExpiresAt)
	require.True(t, code.CanUse(), "generated code must be usable by the existing registration flow")
}

func TestUserInvitationService_CreateInvitationRejectedWhenUnavailable(t *testing.T) {
	now := time.Now()
	repo := &userInvitationRepoStub{}
	svc := newUserInvitationServiceForTest(userInvitationSettings(map[string]string{
		SettingKeyInvitationCodeEnabled: "false",
	}), repo, now)

	_, err := svc.CreateInvitation(context.Background(), 42)
	require.ErrorIs(t, err, ErrUserInvitationDisabled)
	require.Empty(t, repo.codes)
}

func TestUserInvitationService_CreateInvitationEnforcesLimit(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	repo := &userInvitationRepoStub{}
	svc := newUserInvitationServiceForTest(userInvitationSettings(nil), repo, now)

	for i := 0; i < 2; i++ {
		_, err := svc.CreateInvitation(context.Background(), 42)
		require.NoError(t, err)
	}
	_, err := svc.CreateInvitation(context.Background(), 42)
	require.ErrorIs(t, err, ErrUserInvitationLimitReached)

	// 其他用户的名额互不影响
	_, err = svc.CreateInvitation(context.Background(), 43)
	require.NoError(t, err)

	// 过期未使用的邀请码释放名额
	svc.now = func() time.Time { return now.Add(8 * 24 * time.Hour) }
	_, err = svc.CreateInvitation(context.Background(), 42)
	require.NoError(t, err)
}

func TestUserInvitationService_UnlimitedAndNeverExpiring(t *testing.T) {
	now := time.Now()
	repo := &userInvitationRepoStub{}
	svc := newUserInvitationServiceForTest(userInvitationSettings(map[string]string{
		SettingKeyUserInvitationMaxCodesPerUser:  "0",
		SettingKeyUserInvitationCodeValidityDays: "0",
	}), repo, now)

	for i := 0; i < 5; i++ {
		code, err := svc.CreateInvitation(context.Background(), 42)
		require.NoError(t, err)
		require.Nil(t, code.ExpiresAt)
	}

	overview, err := svc.GetOverview(context.Background(), 42)
	require.NoError(t, err)
	require.True(t, overview.Available)
	require.Equal(t, 5, overview.UsedQuota)
	require.Equal(t, -1, overview.Remaining())
	require.Len(t, overview.Codes, 5)
}

func TestUserInvitationService_GetOverviewWhenDisabledStillListsHistory(t *testing.T) {
	now := time.Now()
	creator := int64(42)
	repo := &userInvitationRepoStub{codes: []RedeemCode{{
		ID: 1, Code: "abc", Type: RedeemTypeInvitation, Status: StatusUsed, CreatedBy: &creator,
	}}}
	svc := newUserInvitationServiceForTest(userInvitationSettings(map[string]string{
		SettingKeyUserInvitationEnabled: "false",
	}), repo, now)

	overview, err := svc.GetOverview(context.Background(), 42)
	require.NoError(t, err)
	require.False(t, overview.Available)
	require.Equal(t, 1, overview.UsedQuota)
	require.Equal(t, 1, overview.Remaining())
	require.Len(t, overview.Codes, 1)
}
