//go:build unit

package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// affiliateSettingRepoStub 是一个只读的内存 SettingRepository。
type affiliateSettingRepoStub struct {
	values map[string]string
}

func (r *affiliateSettingRepoStub) Get(context.Context, string) (*Setting, error) {
	return nil, ErrSettingNotFound
}

func (r *affiliateSettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	v, ok := r.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return v, nil
}

func (r *affiliateSettingRepoStub) Set(context.Context, string, string) error { return nil }

func (r *affiliateSettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		if v, ok := r.values[k]; ok {
			out[k] = v
		}
	}
	return out, nil
}

func (r *affiliateSettingRepoStub) SetMultiple(context.Context, map[string]string) error { return nil }

func (r *affiliateSettingRepoStub) GetAll(context.Context) (map[string]string, error) {
	return r.values, nil
}

func (r *affiliateSettingRepoStub) Delete(context.Context, string) error { return nil }

// affiliateCapRaceRepoStub 是一个「真事务」假仓储：AccrueQuota 的整个
// 「读已累计 -> 按上限截断 -> 写入」都在同一个临界区里完成，正是真实实现里
// SELECT ... FOR UPDATE 提供的语义。
//
// 这让本测试可以在单元层面真实并发：服务层的其余部分（读 profile、读配置、
// 算比例）仍然并行执行，只有入账被串行化。修复前上限是在服务层、临界区之外
// 判断的，两个 goroutine 都会读到 existing=0 并各自足额入账；修复后上限参数
// 被透传进临界区，总额必须刚好停在上限上。
type affiliateCapRaceRepoStub struct {
	mu       sync.Mutex
	accrued  map[[2]int64]float64
	inviteeM map[int64]*AffiliateSummary

	// readDelay 放大服务层「读 -> 决策」的窗口，让未修复的实现必然翻车。
	readDelay time.Duration
}

func newAffiliateCapRaceRepoStub(inviterID, inviteeID int64, readDelay time.Duration) *affiliateCapRaceRepoStub {
	inviter := inviterID
	return &affiliateCapRaceRepoStub{
		accrued: map[[2]int64]float64{},
		inviteeM: map[int64]*AffiliateSummary{
			inviteeID: {UserID: inviteeID, AffCode: "INVITEE", InviterID: &inviter, CreatedAt: time.Now().Add(-time.Hour)},
			inviterID: {UserID: inviterID, AffCode: "INVITER", CreatedAt: time.Now().Add(-time.Hour)},
		},
		readDelay: readDelay,
	}
}

func (r *affiliateCapRaceRepoStub) EnsureUserAffiliate(_ context.Context, userID int64) (*AffiliateSummary, error) {
	if r.readDelay > 0 {
		time.Sleep(r.readDelay)
	}
	if s, ok := r.inviteeM[userID]; ok {
		cp := *s
		return &cp, nil
	}
	return &AffiliateSummary{UserID: userID, AffCode: "OTHER", CreatedAt: time.Now().Add(-time.Hour)}, nil
}

func (r *affiliateCapRaceRepoStub) AccrueQuota(
	_ context.Context,
	inviterID, inviteeUserID int64,
	amount float64,
	_ int,
	_ *int64,
	perInviteeCap float64,
) (float64, error) {
	key := [2]int64{inviterID, inviteeUserID}

	r.mu.Lock()
	defer r.mu.Unlock()

	credit := amount
	if perInviteeCap > 0 {
		remaining := perInviteeCap - r.accrued[key]
		if remaining <= 0 {
			return 0, nil
		}
		if credit > remaining {
			credit = remaining
		}
	}
	r.accrued[key] += credit
	return credit, nil
}

func (r *affiliateCapRaceRepoStub) total(inviterID, inviteeID int64) float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.accrued[[2]int64{inviterID, inviteeID}]
}

func (r *affiliateCapRaceRepoStub) GetAffiliateByCode(context.Context, string) (*AffiliateSummary, error) {
	panic("unexpected GetAffiliateByCode call")
}

func (r *affiliateCapRaceRepoStub) BindInviter(context.Context, int64, int64) (bool, error) {
	panic("unexpected BindInviter call")
}

func (r *affiliateCapRaceRepoStub) ThawFrozenQuota(context.Context, int64) (float64, error) {
	panic("unexpected ThawFrozenQuota call")
}

func (r *affiliateCapRaceRepoStub) TransferQuotaToBalance(context.Context, int64) (float64, float64, error) {
	panic("unexpected TransferQuotaToBalance call")
}

func (r *affiliateCapRaceRepoStub) ListInvitees(context.Context, int64, int) ([]AffiliateInvitee, error) {
	panic("unexpected ListInvitees call")
}

func (r *affiliateCapRaceRepoStub) UpdateUserAffCode(context.Context, int64, string) error {
	panic("unexpected UpdateUserAffCode call")
}

func (r *affiliateCapRaceRepoStub) ResetUserAffCode(context.Context, int64) (string, error) {
	panic("unexpected ResetUserAffCode call")
}

func (r *affiliateCapRaceRepoStub) SetUserRebateRate(context.Context, int64, *float64) error {
	panic("unexpected SetUserRebateRate call")
}

func (r *affiliateCapRaceRepoStub) BatchSetUserRebateRate(context.Context, []int64, *float64) error {
	panic("unexpected BatchSetUserRebateRate call")
}

func (r *affiliateCapRaceRepoStub) ListUsersWithCustomSettings(context.Context, AffiliateAdminFilter) ([]AffiliateAdminEntry, int64, error) {
	panic("unexpected ListUsersWithCustomSettings call")
}

func (r *affiliateCapRaceRepoStub) ListAffiliateInviteRecords(context.Context, AffiliateRecordFilter) ([]AffiliateInviteRecord, int64, error) {
	panic("unexpected ListAffiliateInviteRecords call")
}

func (r *affiliateCapRaceRepoStub) ListAffiliateRebateRecords(context.Context, AffiliateRecordFilter) ([]AffiliateRebateRecord, int64, error) {
	panic("unexpected ListAffiliateRebateRecords call")
}

func (r *affiliateCapRaceRepoStub) ListAffiliateTransferRecords(context.Context, AffiliateRecordFilter) ([]AffiliateTransferRecord, int64, error) {
	panic("unexpected ListAffiliateTransferRecords call")
}

func (r *affiliateCapRaceRepoStub) GetAffiliateUserOverview(context.Context, int64) (*AffiliateUserOverview, error) {
	panic("unexpected GetAffiliateUserOverview call")
}

var _ AffiliateRepository = (*affiliateCapRaceRepoStub)(nil)

// TestAccrueInviteRebateForOrder_ConcurrentOrdersRespectPerInviteeCap 是 H1 的回归测试。
//
// 修复前：单人返利上限由 AffiliateService 在入账事务之外用一次裸读判断
// （GetAccruedRebateFromInvitee）。同一个被邀请人的两笔并发充值各自持有自己的
// 审计 claim，都会读到 existing=0 并各自足额入账，上限被绕过。
//
// 本测试真实并发：concurrentOrders 个 goroutine 同时进入 AccrueInviteRebateForOrder，
// 由同一个 start channel 释放；假仓储只在 AccrueQuota 内部串行化（等价于行锁），
// 服务层的读与决策仍然并行。断言最终总额必须刚好等于上限。
func TestAccrueInviteRebateForOrder_ConcurrentOrdersRespectPerInviteeCap(t *testing.T) {
	const (
		inviterID        = int64(11)
		inviteeID        = int64(22)
		concurrentOrders = 8
		rechargeAmount   = 100.0 // 20% => 每笔 20
		perInviteeCap    = 50.0
	)

	repo := newAffiliateCapRaceRepoStub(inviterID, inviteeID, 2*time.Millisecond)
	settings := NewSettingService(&affiliateSettingRepoStub{values: map[string]string{
		SettingKeyAffiliateEnabled:             "true",
		SettingKeyAffiliateRebateRate:          "20",
		SettingKeyAffiliateRebateFreezeHours:   "0",
		SettingKeyAffiliateRebateDurationDays:  "0",
		SettingKeyAffiliateRebatePerInviteeCap: "50",
	}}, &config.Config{})
	svc := NewAffiliateService(repo, settings, nil, nil)

	start := make(chan struct{})
	rebates := make([]float64, concurrentOrders)
	errs := make([]error, concurrentOrders)
	var wg sync.WaitGroup
	for i := 0; i < concurrentOrders; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			orderID := int64(1000 + idx)
			rebates[idx], errs[idx] = svc.AccrueInviteRebateForOrder(
				context.Background(), inviteeID, rechargeAmount, &orderID)
		}(i)
	}
	close(start)
	wg.Wait()

	var reported float64
	for i := 0; i < concurrentOrders; i++ {
		require.NoError(t, errs[i], "goroutine %d", i)
		require.GreaterOrEqual(t, rebates[i], 0.0)
		reported += rebates[i]
	}

	stored := repo.total(inviterID, inviteeID)
	require.InDelta(t, perInviteeCap, stored, 1e-6,
		"并发入账总额必须停在单人上限，实际 %v", stored)
	require.InDelta(t, stored, reported, 1e-6,
		"服务返回的返利金额必须与实际入账一致")
}

// TestAccrueInviteRebateForOrder_ForwardsCapAndFreezeToRepository 明确钉住
// 「上限判定必须下沉到仓储层」这一契约：服务层不再自己读一次已累计返利。
func TestAccrueInviteRebateForOrder_ForwardsCapAndFreezeToRepository(t *testing.T) {
	const (
		inviterID = int64(31)
		inviteeID = int64(32)
	)

	repo := newAffiliateCapRaceRepoStub(inviterID, inviteeID, 0)
	settings := NewSettingService(&affiliateSettingRepoStub{values: map[string]string{
		SettingKeyAffiliateEnabled:             "true",
		SettingKeyAffiliateRebateRate:          "10",
		SettingKeyAffiliateRebateFreezeHours:   "24",
		SettingKeyAffiliateRebateDurationDays:  "0",
		SettingKeyAffiliateRebatePerInviteeCap: "7",
	}}, &config.Config{})
	svc := NewAffiliateService(repo, settings, nil, nil)

	// 10% of 100 = 10，但上限只剩 7，必须由仓储层截断到 7。
	rebate, err := svc.AccrueInviteRebateForOrder(context.Background(), inviteeID, 100, nil)
	require.NoError(t, err)
	require.InDelta(t, 7.0, rebate, 1e-9)
	require.InDelta(t, 7.0, repo.total(inviterID, inviteeID), 1e-9)

	// 上限用满后再充值不再产生返利。
	rebate, err = svc.AccrueInviteRebateForOrder(context.Background(), inviteeID, 100, nil)
	require.NoError(t, err)
	require.Zero(t, rebate)
	require.InDelta(t, 7.0, repo.total(inviterID, inviteeID), 1e-9)
}
