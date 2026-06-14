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
	panic("unexpected GetMultiple call")
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
