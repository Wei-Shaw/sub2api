//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestBillingTicket_PreCheckPlan_NilTicket：nil receiver 安全返回 nil。
func TestBillingTicket_PreCheckPlan_NilTicket(t *testing.T) {
	t.Parallel()
	var ticket *BillingTicket
	require.Nil(t, ticket.PreCheckPlan())
}

// TestBillingTicket_PreCheckPlan_NoPlan：未设置 plan 时返回 nil。
func TestBillingTicket_PreCheckPlan_NoPlan(t *testing.T) {
	t.Parallel()
	ticket := &BillingTicket{}
	require.Nil(t, ticket.PreCheckPlan())
}

// TestBillingTicket_PreCheckPlan_TwoPhase_ReturnsPlan：twoPhase=true 时
// PreCheckSelect 写入的 plan 必须能被调度阶段读到。
func TestBillingTicket_PreCheckPlan_TwoPhase_ReturnsPlan(t *testing.T) {
	t.Parallel()
	limiter := newFakeLimiter()
	rule := ruleConcurrencyAccount(7, 7, 5)
	repo := newMockSettingRepo()
	repo.data[SettingKeyServiceQuotaPreCheckTwoPhase] = "true"
	svc := newQuotaServiceForTest(t, []*ServiceQuotaRule{rule}, repo, limiter)

	plan, err := svc.PreCheckSelect(context.Background(), ServiceQuotaCheckRequest{UserID: 42})
	require.NoError(t, err)
	require.NotNil(t, plan)

	ticket := &BillingTicket{
		svc:      &BillingCacheService{serviceQuota: svc},
		quotaReq: ServiceQuotaCheckRequest{UserID: 42},
		plan:     plan,
		twoPhase: true,
	}
	defer ticket.Close()

	got := ticket.PreCheckPlan()
	require.NotNil(t, got)
	require.Equal(t, plan, got)
}
