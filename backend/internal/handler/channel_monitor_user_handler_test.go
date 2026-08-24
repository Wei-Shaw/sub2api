package handler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestRedactUserQuotaSnapshot_FullQuotaPassthrough(t *testing.T) {
	balance := 12.5
	src := &domain.MonitorQuotaSnapshot{
		Source:            "usage",
		Success:           true,
		PlanLevel:         "pro",
		Balance:           &balance,
		Tiers:             []domain.MonitorQuotaTier{{Window: "5h", UsedPercent: 40}},
		FetchedAt:         time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC),
		AccountsTotal:     4,
		AccountsHealthy:   3,
		AccountsExhausted: 1,
	}

	got := redactUserQuotaSnapshot(src, true)
	require.Same(t, src, got)
}

func TestRedactUserQuotaSnapshot_StripsSingleAccountWhenQuotaHidden(t *testing.T) {
	src := &domain.MonitorQuotaSnapshot{
		Source:    "usage",
		Success:   true,
		PlanLevel: "pro",
		Tiers:     []domain.MonitorQuotaTier{{Window: "5h", UsedPercent: 40}},
		FetchedAt: time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC),
	}

	require.Nil(t, redactUserQuotaSnapshot(src, false))
	require.Nil(t, redactUserQuotaSnapshot(nil, false))
}

func TestRedactUserQuotaSnapshot_KeepsGroupAccountCountsWhenQuotaHidden(t *testing.T) {
	src := &domain.MonitorQuotaSnapshot{
		Source:            "usage",
		Success:           true,
		PlanLevel:         "pro",
		Error:             "should not leak",
		Tiers:             []domain.MonitorQuotaTier{{Window: "5h", UsedPercent: 70, Used: 70, Limit: 100}},
		FetchedAt:         time.Date(2026, 8, 23, 4, 0, 0, 0, time.UTC),
		AccountsTotal:     10,
		AccountsHealthy:   4,
		AccountsExhausted: 6,
	}

	got := redactUserQuotaSnapshot(src, false)
	require.NotNil(t, got)
	require.NotSame(t, src, got)
	require.True(t, got.Success)
	require.Equal(t, 10, got.AccountsTotal)
	require.Equal(t, 4, got.AccountsHealthy)
	require.Equal(t, 6, got.AccountsExhausted)
	require.Equal(t, src.FetchedAt, got.FetchedAt)
	require.Empty(t, got.Source)
	require.Empty(t, got.PlanLevel)
	require.Empty(t, got.Error)
	require.Empty(t, got.Tiers)
	require.Nil(t, got.Balance)
}

func TestUserMonitorViewToItem_ExposesCheckModeAndRedactedGroupQuota(t *testing.T) {
	view := &service.UserMonitorView{
		ID:            7,
		Name:          "Kiro Free",
		Provider:      "kiro",
		PrimaryModel:  "quota",
		PrimaryStatus: "degraded",
		CheckMode:     service.MonitorCheckModeQuota,
		LatestQuota: &domain.MonitorQuotaSnapshot{
			Source:            "usage",
			Success:           true,
			PlanLevel:         "free",
			Tiers:             []domain.MonitorQuotaTier{{Window: "total", Label: "credits", UsedPercent: 100}},
			AccountsTotal:     20,
			AccountsHealthy:   5,
			AccountsExhausted: 15,
		},
	}

	item := userMonitorViewToItem(view, false)
	require.Equal(t, service.MonitorCheckModeQuota, item.CheckMode)
	require.NotNil(t, item.LatestQuota)
	require.Equal(t, 20, item.LatestQuota.AccountsTotal)
	require.Equal(t, 5, item.LatestQuota.AccountsHealthy)
	require.Equal(t, 15, item.LatestQuota.AccountsExhausted)
	require.Empty(t, item.LatestQuota.Tiers)
	require.Empty(t, item.LatestQuota.PlanLevel)
}
