//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type affiliateSettingRepoStub struct {
	values map[string]string
	err    error
}

func (s *affiliateSettingRepoStub) Get(context.Context, string) (*Setting, error) {
	panic("unexpected Get call")
}

func (s *affiliateSettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	return s.values[key], nil
}

func (s *affiliateSettingRepoStub) Set(context.Context, string, string) error {
	panic("unexpected Set call")
}

func (s *affiliateSettingRepoStub) GetMultiple(context.Context, []string) (map[string]string, error) {
	if s.err != nil {
		return nil, s.err
	}
	out := map[string]string{}
	for key, value := range s.values {
		out[key] = value
	}
	return out, nil
}

func (s *affiliateSettingRepoStub) SetMultiple(context.Context, map[string]string) error {
	panic("unexpected SetMultiple call")
}

func (s *affiliateSettingRepoStub) GetAll(context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}

func (s *affiliateSettingRepoStub) Delete(context.Context, string) error {
	panic("unexpected Delete call")
}

func TestGetAffiliateSubscriptionRebateMultiplierPercent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		raw  string
		err  error
		want float64
	}{
		{name: "default on missing", raw: "", want: AffiliateSubscriptionRebateMultiplierDefault},
		{name: "configured value", raw: "75", want: 75},
		{name: "zero disables subscription rebate", raw: "0", want: 0},
		{name: "clamps high value", raw: "150", want: 100},
		{name: "invalid falls back", raw: "bad", want: AffiliateSubscriptionRebateMultiplierDefault},
		{name: "repo error falls back", err: errors.New("db down"), want: AffiliateSubscriptionRebateMultiplierDefault},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc := NewSettingService(&affiliateSettingRepoStub{
				values: map[string]string{SettingKeyAffiliateSubscriptionRebateMultiplier: tt.raw},
				err:    tt.err,
			}, &config.Config{})

			got := svc.GetAffiliateSubscriptionRebateMultiplierPercent(context.Background())

			require.InDelta(t, tt.want, got, 1e-9)
		})
	}
}

func TestGetAffiliateTieredRebateConfig(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		values map[string]string
		want   AffiliateTieredRebateConfig
	}{
		{
			name:   "defaults when missing",
			values: map[string]string{},
			want: AffiliateTieredRebateConfig{
				Enabled:                AffiliateTieredRebateEnabledDefault,
				Tier2MinPaidInvitees:   AffiliateTier2MinPaidInviteesDefault,
				Tier3MinPaidInvitees:   AffiliateTier3MinPaidInviteesDefault,
				Tier2MultiplierPercent: AffiliateTier2MultiplierPercentDefault,
				Tier3MultiplierPercent: AffiliateTier3MultiplierPercentDefault,
			},
		},
		{
			name: "configured values",
			values: map[string]string{
				SettingKeyAffiliateTieredRebateEnabled:    "true",
				SettingKeyAffiliateTier2MinPaidInvitees:   "5",
				SettingKeyAffiliateTier3MinPaidInvitees:   "20",
				SettingKeyAffiliateTier2MultiplierPercent: "130",
				SettingKeyAffiliateTier3MultiplierPercent: "180",
			},
			want: AffiliateTieredRebateConfig{
				Enabled:                true,
				Tier2MinPaidInvitees:   5,
				Tier3MinPaidInvitees:   20,
				Tier2MultiplierPercent: 130,
				Tier3MultiplierPercent: 180,
			},
		},
		{
			name: "invalid thresholds normalized",
			values: map[string]string{
				SettingKeyAffiliateTieredRebateEnabled:    "true",
				SettingKeyAffiliateTier2MinPaidInvitees:   "-1",
				SettingKeyAffiliateTier3MinPaidInvitees:   "5",
				SettingKeyAffiliateTier2MultiplierPercent: "bad",
				SettingKeyAffiliateTier3MultiplierPercent: "250",
			},
			want: AffiliateTieredRebateConfig{
				Enabled:                true,
				Tier2MinPaidInvitees:   AffiliateTier2MinPaidInviteesDefault,
				Tier3MinPaidInvitees:   AffiliateTier3MinPaidInviteesDefault,
				Tier2MultiplierPercent: AffiliateTier2MultiplierPercentDefault,
				Tier3MultiplierPercent: 200,
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc := NewSettingService(&affiliateSettingRepoStub{values: tt.values}, &config.Config{})

			got := svc.GetAffiliateTieredRebateConfig(context.Background())

			require.Equal(t, tt.want.Enabled, got.Enabled)
			require.Equal(t, tt.want.Tier2MinPaidInvitees, got.Tier2MinPaidInvitees)
			require.Equal(t, tt.want.Tier3MinPaidInvitees, got.Tier3MinPaidInvitees)
			require.InDelta(t, tt.want.Tier2MultiplierPercent, got.Tier2MultiplierPercent, 1e-9)
			require.InDelta(t, tt.want.Tier3MultiplierPercent, got.Tier3MultiplierPercent, 1e-9)
		})
	}
}
