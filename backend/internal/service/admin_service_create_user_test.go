//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestAdminService_CreateUser_Success(t *testing.T) {
	repo := &userRepoStub{nextID: 10}
	svc := &adminServiceImpl{userRepo: repo}
	balance := 12.5

	input := &CreateUserInput{
		Email:         "user@test.com",
		Password:      "strong-pass",
		Username:      "tester",
		Notes:         "note",
		Balance:       &balance,
		Concurrency:   7,
		AllowedGroups: []int64{3, 5},
	}

	user, err := svc.CreateUser(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, user)
	require.Equal(t, int64(10), user.ID)
	require.Equal(t, input.Email, user.Email)
	require.Equal(t, input.Username, user.Username)
	require.Equal(t, input.Notes, user.Notes)
	require.Equal(t, balance, user.Balance)
	require.Equal(t, input.Concurrency, user.Concurrency)
	require.Equal(t, input.AllowedGroups, user.AllowedGroups)
	require.Equal(t, RoleUser, user.Role)
	require.Equal(t, StatusActive, user.Status)
	require.True(t, user.CheckPassword(input.Password))
	require.Len(t, repo.created, 1)
	require.Equal(t, user, repo.created[0])
}

func TestAdminService_CreateUser_UsesDefaultBalanceWhenBalanceOmitted(t *testing.T) {
	repo := &userRepoStub{nextID: 11}
	cfg := &config.Config{
		Default: config.DefaultConfig{
			UserBalance: 0,
		},
	}
	settingService := NewSettingService(&settingRepoStub{values: map[string]string{
		SettingKeyDefaultBalance: "0.02",
	}}, cfg)
	svc := &adminServiceImpl{userRepo: repo, settingService: settingService}

	user, err := svc.CreateUser(context.Background(), &CreateUserInput{
		Email:    "default-balance@test.com",
		Password: "strong-pass",
	})

	require.NoError(t, err)
	require.NotNil(t, user)
	require.Equal(t, 0.02, user.Balance)
	require.Len(t, repo.created, 1)
	require.Equal(t, 0.02, repo.created[0].Balance)
}

func TestAdminService_CreateUser_ExplicitZeroBalanceOverridesDefault(t *testing.T) {
	repo := &userRepoStub{nextID: 12}
	cfg := &config.Config{
		Default: config.DefaultConfig{
			UserBalance: 0,
		},
	}
	settingService := NewSettingService(&settingRepoStub{values: map[string]string{
		SettingKeyDefaultBalance: "0.02",
	}}, cfg)
	svc := &adminServiceImpl{userRepo: repo, settingService: settingService}
	balance := 0.0

	user, err := svc.CreateUser(context.Background(), &CreateUserInput{
		Email:    "zero-balance@test.com",
		Password: "strong-pass",
		Balance:  &balance,
	})

	require.NoError(t, err)
	require.NotNil(t, user)
	require.Equal(t, 0.0, user.Balance)
	require.Len(t, repo.created, 1)
	require.Equal(t, 0.0, repo.created[0].Balance)
}

func TestAdminService_CreateUser_EmailExists(t *testing.T) {
	repo := &userRepoStub{createErr: ErrEmailExists}
	svc := &adminServiceImpl{userRepo: repo}

	_, err := svc.CreateUser(context.Background(), &CreateUserInput{
		Email:    "dup@test.com",
		Password: "password",
	})
	require.ErrorIs(t, err, ErrEmailExists)
	require.Empty(t, repo.created)
}

func TestAdminService_CreateUser_CreateError(t *testing.T) {
	createErr := errors.New("db down")
	repo := &userRepoStub{createErr: createErr}
	quotas := &userPlatformQuotaRepoStub{}
	svc := &adminServiceImpl{
		userRepo: repo,
		settingService: NewSettingService(&settingRepoStub{values: map[string]string{
			SettingKeyDefaultPlatformQuotas: `{"openai":{"daily":100}}`,
		}}, &config.Config{}),
		userPlatformQuotaRepo: quotas,
	}

	_, err := svc.CreateUser(context.Background(), &CreateUserInput{
		Email:    "user@test.com",
		Password: "password",
	})
	require.ErrorIs(t, err, createErr)
	require.Empty(t, repo.created)
	require.Empty(t, quotas.bulkInsertCalls, "failed user creation must not initialize platform quotas")
}

func TestAdminService_CreateUser_AssignsDefaultSubscriptions(t *testing.T) {
	repo := &userRepoStub{nextID: 21}
	assigner := &defaultSubscriptionAssignerStub{}
	cfg := &config.Config{
		Default: config.DefaultConfig{
			UserBalance:     0,
			UserConcurrency: 1,
		},
	}
	settingService := NewSettingService(&settingRepoStub{values: map[string]string{
		SettingKeyDefaultSubscriptions: `[{"group_id":5,"validity_days":30}]`,
	}}, cfg)
	svc := &adminServiceImpl{
		userRepo:           repo,
		settingService:     settingService,
		defaultSubAssigner: assigner,
	}

	_, err := svc.CreateUser(context.Background(), &CreateUserInput{
		Email:    "new-user@test.com",
		Password: "password",
	})
	require.NoError(t, err)
	require.Len(t, assigner.calls, 1)
	require.Equal(t, int64(21), assigner.calls[0].UserID)
	require.Equal(t, int64(5), assigner.calls[0].GroupID)
	require.Equal(t, 30, assigner.calls[0].ValidityDays)
}

func TestAdminService_CreateUser_SnapshotsDefaultPlatformQuotas(t *testing.T) {
	users := &userRepoStub{nextID: 40}
	quotas := &userPlatformQuotaRepoStub{}
	settings := NewSettingService(&settingRepoStub{values: map[string]string{
		SettingKeyDefaultPlatformQuotas: `{
			"openai": {"daily": 100, "weekly": 700, "monthly": 3000},
			"anthropic": {"daily": 100},
			"gemini": {"daily": 500},
			"antigravity": {"daily": 500},
			"grok": {"daily": 0},
			"cline": {},
			"deepseek": null
		}`,
		SettingKeyAuthSourceDefaultEmailGrantOnSignup: "true",
		SettingKeyAuthSourcePlatformQuotas("email"):   `{"openai":{"daily":3}}`,
	}}, &config.Config{})
	svc := NewAdminService(nil, users, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, settings, nil, nil, nil, nil, nil, nil, nil, nil, quotas)

	user, err := svc.CreateUser(context.Background(), &CreateUserInput{
		Email:    "quota-defaults@example.test",
		Password: "strong-pass",
	})

	require.NoError(t, err)
	require.Equal(t, int64(40), user.ID)
	require.Len(t, quotas.bulkInsertCalls, 1, "admin-created users must receive the configured platform quotas")
	records := make(map[string]UserPlatformQuotaRecord)
	for _, record := range quotas.bulkInsertCalls[0] {
		require.Equal(t, user.ID, record.UserID)
		records[record.Platform] = record
	}
	require.Len(t, records, 5, "unconfigured platforms must not receive quota rows")
	require.Equal(t, 100.0, *records["openai"].DailyLimitUSD, "admin creation uses system defaults, not email signup overrides")
	require.Equal(t, 700.0, *records["openai"].WeeklyLimitUSD)
	require.Equal(t, 3000.0, *records["openai"].MonthlyLimitUSD)
	require.Equal(t, 100.0, *records["anthropic"].DailyLimitUSD)
	require.Equal(t, 500.0, *records["gemini"].DailyLimitUSD)
	require.Equal(t, 500.0, *records["antigravity"].DailyLimitUSD)
	require.NotNil(t, records["grok"].DailyLimitUSD, "zero is an explicit limit")
	require.Zero(t, *records["grok"].DailyLimitUSD)
	require.Nil(t, records["anthropic"].WeeklyLimitUSD)
	require.Nil(t, records["anthropic"].MonthlyLimitUSD)
}

func TestAdminService_CreateUser_UnlimitedDefaultsDoNotCreateQuotaRows(t *testing.T) {
	quotas := &userPlatformQuotaRepoStub{}
	svc := &adminServiceImpl{
		userRepo: &userRepoStub{nextID: 41},
		settingService: NewSettingService(&settingRepoStub{values: map[string]string{
			SettingKeyDefaultPlatformQuotas: `{"openai":{"daily":null,"weekly":null,"monthly":null}}`,
		}}, &config.Config{}),
		userPlatformQuotaRepo: quotas,
	}

	_, err := svc.CreateUser(context.Background(), &CreateUserInput{
		Email:    "unlimited@example.test",
		Password: "strong-pass",
	})

	require.NoError(t, err)
	require.Empty(t, quotas.bulkInsertCalls)
}

func TestAdminService_CreateUser_AdminSkipsDefaultPlatformQuotas(t *testing.T) {
	quotas := &userPlatformQuotaRepoStub{}
	svc := &adminServiceImpl{
		userRepo: &userRepoStub{nextID: 42},
		settingService: NewSettingService(&settingRepoStub{values: map[string]string{
			SettingKeyDefaultPlatformQuotas: `{"openai":{"daily":100}}`,
		}}, &config.Config{}),
		userPlatformQuotaRepo: quotas,
	}

	user, err := svc.CreateUser(context.Background(), &CreateUserInput{
		Email:    "admin@example.test",
		Password: "strong-pass",
		Role:     RoleAdmin,
	})

	require.NoError(t, err)
	require.Equal(t, RoleAdmin, user.Role)
	require.Empty(t, quotas.bulkInsertCalls)
}

func TestAdminService_CreateUser_QuotaWriteFailureDoesNotFailCreation(t *testing.T) {
	users := &userRepoStub{nextID: 43}
	quotas := &userPlatformQuotaRepoStub{bulkInsertErr: errors.New("quota insert failed")}
	svc := &adminServiceImpl{
		userRepo: users,
		settingService: NewSettingService(&settingRepoStub{values: map[string]string{
			SettingKeyDefaultPlatformQuotas: `{"openai":{"daily":100}}`,
		}}, &config.Config{}),
		userPlatformQuotaRepo: quotas,
	}

	user, err := svc.CreateUser(context.Background(), &CreateUserInput{
		Email:    "quota-failure@example.test",
		Password: "strong-pass",
	})

	require.NoError(t, err)
	require.Equal(t, int64(43), user.ID)
	require.Len(t, users.created, 1)
	require.Len(t, quotas.bulkInsertCalls, 1)
}
