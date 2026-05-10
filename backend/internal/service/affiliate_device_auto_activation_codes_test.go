package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type affiliateDeviceAutoActivationSettingRepoStub struct {
	values map[string]string
	err    error
}

func (s *affiliateDeviceAutoActivationSettingRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	panic("unexpected Get call")
}

func (s *affiliateDeviceAutoActivationSettingRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	if v, ok := s.values[key]; ok {
		return v, nil
	}
	return "", ErrSettingNotFound
}

func (s *affiliateDeviceAutoActivationSettingRepoStub) Set(ctx context.Context, key, value string) error {
	panic("unexpected Set call")
}

func (s *affiliateDeviceAutoActivationSettingRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	panic("unexpected GetMultiple call")
}

func (s *affiliateDeviceAutoActivationSettingRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	panic("unexpected SetMultiple call")
}

func (s *affiliateDeviceAutoActivationSettingRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}

func (s *affiliateDeviceAutoActivationSettingRepoStub) Delete(ctx context.Context, key string) error {
	panic("unexpected Delete call")
}

func TestAffiliateService_DeviceAutoActivationCodes_AllowsManualOnlyEmptySetting(t *testing.T) {
	t.Parallel()
	svc := NewAffiliateService(nil, NewSettingService(&affiliateDeviceAutoActivationSettingRepoStub{values: map[string]string{
		SettingKeyDeviceAutoActivationAffCodes: "",
	}}, nil), nil, nil)

	require.Empty(t, svc.DeviceAutoActivationCodes(context.Background()))
	require.False(t, svc.IsDeviceAutoActivationCode(context.Background(), "AUTO_APPROVE"))
}

func TestAffiliateService_DeviceAutoActivationCodes_NormalizesConfiguredCodes(t *testing.T) {
	t.Parallel()
	svc := NewAffiliateService(nil, NewSettingService(&affiliateDeviceAutoActivationSettingRepoStub{values: map[string]string{
		SettingKeyDeviceAutoActivationAffCodes: "vip_auto, cn-test VIP_AUTO",
	}}, nil), nil, nil)

	require.Equal(t, []string{"VIP_AUTO", "CN-TEST"}, svc.DeviceAutoActivationCodes(context.Background()))
	require.True(t, svc.IsDeviceAutoActivationCode(context.Background(), "cn-test"))
}
