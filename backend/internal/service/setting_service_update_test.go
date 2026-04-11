//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type settingUpdateRepoStub struct {
	updates map[string]string
}

func (s *settingUpdateRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	panic("unexpected Get call")
}

func (s *settingUpdateRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	panic("unexpected GetValue call")
}

func (s *settingUpdateRepoStub) Set(ctx context.Context, key, value string) error {
	panic("unexpected Set call")
}

func (s *settingUpdateRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	panic("unexpected GetMultiple call")
}

func (s *settingUpdateRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	s.updates = make(map[string]string, len(settings))
	for k, v := range settings {
		s.updates[k] = v
	}
	return nil
}

func (s *settingUpdateRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}

func (s *settingUpdateRepoStub) Delete(ctx context.Context, key string) error {
	panic("unexpected Delete call")
}

type defaultSubGroupReaderStub struct {
	byID  map[int64]*Group
	errBy map[int64]error
	calls []int64
}

func (s *defaultSubGroupReaderStub) GetByID(ctx context.Context, id int64) (*Group, error) {
	s.calls = append(s.calls, id)
	if err, ok := s.errBy[id]; ok {
		return nil, err
	}
	if g, ok := s.byID[id]; ok {
		return g, nil
	}
	return nil, ErrGroupNotFound
}

func TestSettingService_UpdateSettings_DefaultSubscriptions_ValidGroup(t *testing.T) {
	repo := &settingUpdateRepoStub{}
	groupReader := &defaultSubGroupReaderStub{
		byID: map[int64]*Group{
			11: {ID: 11, SubscriptionType: SubscriptionTypeSubscription},
		},
	}
	svc := NewSettingService(repo, &config.Config{})
	svc.SetDefaultSubscriptionGroupReader(groupReader)

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		DefaultSubscriptions: []DefaultSubscriptionSetting{
			{GroupID: 11, ValidityDays: 30},
		},
	})
	require.NoError(t, err)
	require.Equal(t, []int64{11}, groupReader.calls)

	raw, ok := repo.updates[SettingKeyDefaultSubscriptions]
	require.True(t, ok)

	var got []DefaultSubscriptionSetting
	require.NoError(t, json.Unmarshal([]byte(raw), &got))
	require.Equal(t, []DefaultSubscriptionSetting{
		{GroupID: 11, ValidityDays: 30},
	}, got)
}

func TestSettingService_UpdateSettings_DefaultSubscriptions_RejectsNonSubscriptionGroup(t *testing.T) {
	repo := &settingUpdateRepoStub{}
	groupReader := &defaultSubGroupReaderStub{
		byID: map[int64]*Group{
			12: {ID: 12, SubscriptionType: SubscriptionTypeStandard},
		},
	}
	svc := NewSettingService(repo, &config.Config{})
	svc.SetDefaultSubscriptionGroupReader(groupReader)

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		DefaultSubscriptions: []DefaultSubscriptionSetting{
			{GroupID: 12, ValidityDays: 7},
		},
	})
	require.Error(t, err)
	require.Equal(t, "DEFAULT_SUBSCRIPTION_GROUP_INVALID", infraerrors.Reason(err))
	require.Nil(t, repo.updates)
}

func TestSettingService_UpdateSettings_DefaultSubscriptions_RejectsNotFoundGroup(t *testing.T) {
	repo := &settingUpdateRepoStub{}
	groupReader := &defaultSubGroupReaderStub{
		errBy: map[int64]error{
			13: ErrGroupNotFound,
		},
	}
	svc := NewSettingService(repo, &config.Config{})
	svc.SetDefaultSubscriptionGroupReader(groupReader)

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		DefaultSubscriptions: []DefaultSubscriptionSetting{
			{GroupID: 13, ValidityDays: 7},
		},
	})
	require.Error(t, err)
	require.Equal(t, "DEFAULT_SUBSCRIPTION_GROUP_INVALID", infraerrors.Reason(err))
	require.Equal(t, "13", infraerrors.FromError(err).Metadata["group_id"])
	require.Nil(t, repo.updates)
}

func TestSettingService_UpdateSettings_DefaultSubscriptions_RejectsDuplicateGroup(t *testing.T) {
	repo := &settingUpdateRepoStub{}
	groupReader := &defaultSubGroupReaderStub{
		byID: map[int64]*Group{
			11: {ID: 11, SubscriptionType: SubscriptionTypeSubscription},
		},
	}
	svc := NewSettingService(repo, &config.Config{})
	svc.SetDefaultSubscriptionGroupReader(groupReader)

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		DefaultSubscriptions: []DefaultSubscriptionSetting{
			{GroupID: 11, ValidityDays: 30},
			{GroupID: 11, ValidityDays: 60},
		},
	})
	require.Error(t, err)
	require.Equal(t, "DEFAULT_SUBSCRIPTION_GROUP_DUPLICATE", infraerrors.Reason(err))
	require.Equal(t, "11", infraerrors.FromError(err).Metadata["group_id"])
	require.Nil(t, repo.updates)
}

func TestSettingService_UpdateSettings_DefaultSubscriptions_RejectsDuplicateGroupWithoutGroupReader(t *testing.T) {
	repo := &settingUpdateRepoStub{}
	svc := NewSettingService(repo, &config.Config{})

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		DefaultSubscriptions: []DefaultSubscriptionSetting{
			{GroupID: 11, ValidityDays: 30},
			{GroupID: 11, ValidityDays: 60},
		},
	})
	require.Error(t, err)
	require.Equal(t, "DEFAULT_SUBSCRIPTION_GROUP_DUPLICATE", infraerrors.Reason(err))
	require.Equal(t, "11", infraerrors.FromError(err).Metadata["group_id"])
	require.Nil(t, repo.updates)
}

func TestSettingService_UpdateSettings_RegistrationEmailSuffixWhitelist_Normalized(t *testing.T) {
	repo := &settingUpdateRepoStub{}
	svc := NewSettingService(repo, &config.Config{})

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		RegistrationEmailSuffixWhitelist: []string{"example.com", "@EXAMPLE.com", " @foo.bar "},
	})
	require.NoError(t, err)
	require.Equal(t, `["@example.com","@foo.bar"]`, repo.updates[SettingKeyRegistrationEmailSuffixWhitelist])
}

func TestSettingService_UpdateSettings_RegistrationEmailSuffixWhitelist_Invalid(t *testing.T) {
	repo := &settingUpdateRepoStub{}
	svc := NewSettingService(repo, &config.Config{})

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		RegistrationEmailSuffixWhitelist: []string{"@invalid_domain"},
	})
	require.Error(t, err)
	require.Equal(t, "INVALID_REGISTRATION_EMAIL_SUFFIX_WHITELIST", infraerrors.Reason(err))
}

func TestParseDefaultSubscriptions_NormalizesValues(t *testing.T) {
	got := parseDefaultSubscriptions(`[{"group_id":11,"validity_days":30},{"group_id":11,"validity_days":60},{"group_id":0,"validity_days":10},{"group_id":12,"validity_days":99999}]`)
	require.Equal(t, []DefaultSubscriptionSetting{
		{GroupID: 11, ValidityDays: 30},
		{GroupID: 11, ValidityDays: 60},
		{GroupID: 12, ValidityDays: MaxValidityDays},
	}, got)
}

func TestSettingService_UpdateSettings_TablePreferences(t *testing.T) {
	repo := &settingUpdateRepoStub{}
	svc := NewSettingService(repo, &config.Config{})

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		TableDefaultPageSize:  50,
		TablePageSizeOptions: []int{20, 50, 100},
	})
	require.NoError(t, err)
	require.Equal(t, "50", repo.updates[SettingKeyTableDefaultPageSize])
	require.Equal(t, "[20,50,100]", repo.updates[SettingKeyTablePageSizeOptions])

	err = svc.UpdateSettings(context.Background(), &SystemSettings{
		TableDefaultPageSize:  1000,
		TablePageSizeOptions: []int{20, 100},
	})
	require.NoError(t, err)
	require.Equal(t, "1000", repo.updates[SettingKeyTableDefaultPageSize])
	require.Equal(t, "[20,100]", repo.updates[SettingKeyTablePageSizeOptions])
}
