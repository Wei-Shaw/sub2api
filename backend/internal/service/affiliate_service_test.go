//go:build unit

package service

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// TestResolveRebateRatePercent_PerUserOverride verifies that per-inviter
// AffRebateRatePercent overrides the global rate, that NULL falls back to the
// global rate, and that out-of-range exclusive rates are clamped silently.
//
// SettingService is left nil here so globalRebateRatePercent returns the
// documented default (AffiliateRebateRateDefault = 20%) — this exercises the
// fallback path without spinning up a settings stub.
func TestResolveRebateRatePercent_PerUserOverride(t *testing.T) {
	t.Parallel()
	svc := &AffiliateService{}

	// nil exclusive rate → falls back to global default (20%)
	require.InDelta(t, AffiliateRebateRateDefault,
		svc.resolveRebateRatePercent(context.Background(), &AffiliateSummary{}), 1e-9)

	// exclusive rate set → overrides global
	rate := 50.0
	require.InDelta(t, 50.0,
		svc.resolveRebateRatePercent(context.Background(), &AffiliateSummary{AffRebateRatePercent: &rate}), 1e-9)

	// exclusive rate 0 → returns 0 (no rebate, intentional)
	zero := 0.0
	require.InDelta(t, 0.0,
		svc.resolveRebateRatePercent(context.Background(), &AffiliateSummary{AffRebateRatePercent: &zero}), 1e-9)

	// exclusive rate above max → clamped to Max
	tooHigh := 250.0
	require.InDelta(t, AffiliateRebateRateMax,
		svc.resolveRebateRatePercent(context.Background(), &AffiliateSummary{AffRebateRatePercent: &tooHigh}), 1e-9)

	// exclusive rate below min → clamped to Min
	tooLow := -5.0
	require.InDelta(t, AffiliateRebateRateMin,
		svc.resolveRebateRatePercent(context.Background(), &AffiliateSummary{AffRebateRatePercent: &tooLow}), 1e-9)
}

func TestResolveSubscriptionRebateRatePercent_UsesEightyPercentOfEffectiveBalanceRate(t *testing.T) {
	t.Parallel()
	svc := &AffiliateService{}

	rate := 10.0
	require.InDelta(t, 8.0,
		svc.resolveSubscriptionRebateRatePercent(context.Background(), &AffiliateSummary{AffRebateRatePercent: &rate}), 1e-9)
}

func TestResolveRebateRatePercent_UsesTieredPaidInviteeMultiplier(t *testing.T) {
	t.Parallel()
	rate := 10.0
	tests := []struct {
		name         string
		paidInvitees int
		wantRate     float64
		wantTier     int
	}{
		{name: "first tier below threshold", paidInvitees: 9, wantRate: 10, wantTier: 1},
		{name: "second tier at threshold", paidInvitees: 10, wantRate: 12, wantTier: 2},
		{name: "third tier at threshold", paidInvitees: 30, wantRate: 15, wantTier: 3},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := &affiliateDetailRepoStub{paidInviteeCount: tt.paidInvitees}
			svc := &AffiliateService{
				repo: repo,
				settingService: affiliateSettingsService(map[string]string{
					SettingKeyAffiliateTieredRebateEnabled:          "true",
					SettingKeyAffiliateTier2MinPaidInvitees:         "10",
					SettingKeyAffiliateTier3MinPaidInvitees:         "30",
					SettingKeyAffiliateTier2MultiplierPercent:       "120",
					SettingKeyAffiliateTier3MultiplierPercent:       "150",
					SettingKeyAffiliateSubscriptionRebateMultiplier: "80",
				}),
			}

			got := svc.resolveRebateRatePercent(context.Background(), &AffiliateSummary{
				UserID:               7,
				AffRebateRatePercent: &rate,
			})
			tier := svc.resolveAffiliateRebateTier(context.Background(), &AffiliateSummary{
				UserID:               7,
				AffRebateRatePercent: &rate,
			})

			require.InDelta(t, tt.wantRate, got, 1e-9)
			require.Equal(t, tt.wantTier, tier.Level)
			require.Equal(t, tt.paidInvitees, tier.PaidInviteeCount)
		})
	}
}

func TestResolveRebateRatePercent_TieredDisabledKeepsBaseRate(t *testing.T) {
	t.Parallel()
	rate := 10.0
	repo := &affiliateDetailRepoStub{paidInviteeCount: 30}
	svc := &AffiliateService{
		repo: repo,
		settingService: affiliateSettingsService(map[string]string{
			SettingKeyAffiliateTieredRebateEnabled:    "false",
			SettingKeyAffiliateTier3MultiplierPercent: "150",
		}),
	}

	got := svc.resolveRebateRatePercent(context.Background(), &AffiliateSummary{
		UserID:               7,
		AffRebateRatePercent: &rate,
	})
	tier := svc.resolveAffiliateRebateTier(context.Background(), &AffiliateSummary{
		UserID:               7,
		AffRebateRatePercent: &rate,
	})

	require.InDelta(t, 10.0, got, 1e-9)
	require.Equal(t, 1, tier.Level)
	require.False(t, tier.Enabled)
}

func TestResolveSubscriptionRebateRatePercent_UsesTieredBalanceRate(t *testing.T) {
	t.Parallel()
	rate := 10.0
	repo := &affiliateDetailRepoStub{paidInviteeCount: 30}
	svc := &AffiliateService{
		repo: repo,
		settingService: affiliateSettingsService(map[string]string{
			SettingKeyAffiliateTieredRebateEnabled:          "true",
			SettingKeyAffiliateTier3MinPaidInvitees:         "30",
			SettingKeyAffiliateTier3MultiplierPercent:       "150",
			SettingKeyAffiliateSubscriptionRebateMultiplier: "80",
		}),
	}

	got := svc.resolveSubscriptionRebateRatePercent(context.Background(), &AffiliateSummary{
		UserID:               7,
		AffRebateRatePercent: &rate,
	})

	require.InDelta(t, 12.0, got, 1e-9)
}

func TestGetAffiliateDetailIncludesEffectiveSubscriptionRebateRate(t *testing.T) {
	t.Parallel()
	rate := 10.0
	repo := &affiliateDetailRepoStub{
		summary: &AffiliateSummary{
			UserID:               42,
			AffCode:              "AFF42",
			AffRebateRatePercent: &rate,
		},
	}
	svc := &AffiliateService{repo: repo}

	detail, err := svc.GetAffiliateDetail(context.Background(), 42)

	require.NoError(t, err)
	require.InDelta(t, 10.0, detail.EffectiveRebateRatePercent, 1e-9)
	require.InDelta(t, 8.0, detail.EffectiveSubscriptionRebateRatePercent, 1e-9)
}

func TestGetAffiliateDetailIncludesTieredRebateInfo(t *testing.T) {
	t.Parallel()
	rate := 10.0
	repo := &affiliateDetailRepoStub{
		paidInviteeCount: 10,
		summary: &AffiliateSummary{
			UserID:               42,
			AffCode:              "AFF42",
			AffRebateRatePercent: &rate,
		},
	}
	svc := &AffiliateService{
		repo: repo,
		settingService: affiliateSettingsService(map[string]string{
			SettingKeyAffiliateTieredRebateEnabled:          "true",
			SettingKeyAffiliateTier2MinPaidInvitees:         "10",
			SettingKeyAffiliateTier3MinPaidInvitees:         "30",
			SettingKeyAffiliateTier2MultiplierPercent:       "120",
			SettingKeyAffiliateTier3MultiplierPercent:       "150",
			SettingKeyAffiliateSubscriptionRebateMultiplier: "80",
		}),
	}

	detail, err := svc.GetAffiliateDetail(context.Background(), 42)

	require.NoError(t, err)
	require.Equal(t, 2, detail.RebateTierLevel)
	require.Equal(t, 10, detail.PaidInviteeCount)
	require.InDelta(t, 120.0, detail.RebateTierMultiplierPercent, 1e-9)
	require.InDelta(t, 12.0, detail.EffectiveRebateRatePercent, 1e-9)
	require.InDelta(t, 9.6, detail.EffectiveSubscriptionRebateRatePercent, 1e-9)
	require.Equal(t, 20, detail.NextTierPaidInviteeCount)
	require.Equal(t, 1, repo.countPaidInviteesCalls)
}

func TestAccrueInviteSubscriptionRebateUsesSubscriptionRate(t *testing.T) {
	t.Parallel()
	inviterID := int64(7)
	inviteeID := int64(42)
	rate := 10.0
	repo := &affiliateDetailRepoStub{
		summaries: map[int64]*AffiliateSummary{
			inviteeID: {
				UserID:    inviteeID,
				InviterID: &inviterID,
				CreatedAt: time.Now(),
			},
			inviterID: {
				UserID:               inviterID,
				AffRebateRatePercent: &rate,
			},
		},
	}
	svc := &AffiliateService{repo: repo, settingService: affiliateSettingsService(map[string]string{
		SettingKeyAffiliateEnabled: "true",
	})}
	orderID := int64(1001)

	rebate, err := svc.AccrueInviteSubscriptionRebateForOrder(context.Background(), inviteeID, 180, &orderID)

	require.NoError(t, err)
	require.InDelta(t, 14.4, rebate, 1e-9)
	require.Equal(t, affiliateAccrueCall{
		inviterID:     inviterID,
		inviteeUserID: inviteeID,
		amount:        14.4,
		freezeHours:   0,
		sourceOrderID: &orderID,
	}, repo.accrueCalls[0])
}

func TestAccrueInviteSubscriptionRebateSkipsWhenBaseAmountMissing(t *testing.T) {
	t.Parallel()
	inviterID := int64(7)
	inviteeID := int64(42)
	rate := 10.0
	repo := &affiliateDetailRepoStub{
		summaries: map[int64]*AffiliateSummary{
			inviteeID: {UserID: inviteeID, InviterID: &inviterID, CreatedAt: time.Now()},
			inviterID: {UserID: inviterID, AffRebateRatePercent: &rate},
		},
	}
	svc := &AffiliateService{repo: repo, settingService: affiliateSettingsService(map[string]string{
		SettingKeyAffiliateEnabled: "true",
	})}

	rebate, err := svc.AccrueInviteSubscriptionRebateForOrder(context.Background(), inviteeID, 0, nil)

	require.NoError(t, err)
	require.Zero(t, rebate)
	require.Empty(t, repo.accrueCalls)
}

// TestIsEnabled_NilSettingServiceReturnsDefault verifies that IsEnabled
// safely handles a nil settingService dependency by returning the default
// (off). This protects callers from nil-pointer crashes in misconfigured
// environments.
func TestIsEnabled_NilSettingServiceReturnsDefault(t *testing.T) {
	t.Parallel()
	svc := &AffiliateService{}
	require.False(t, svc.IsEnabled(context.Background()))
	require.Equal(t, AffiliateEnabledDefault, svc.IsEnabled(context.Background()))
}

// TestValidateExclusiveRate_BoundaryAndInvalid covers the validator used by
// admin-facing rate setters: nil is always valid (clear), in-range values
// are accepted, NaN/Inf and out-of-range values produce a typed BadRequest.
func TestValidateExclusiveRate_BoundaryAndInvalid(t *testing.T) {
	t.Parallel()
	require.NoError(t, validateExclusiveRate(nil))

	for _, v := range []float64{0, 0.01, 50, 99.99, 100} {
		v := v
		require.NoError(t, validateExclusiveRate(&v), "value %v should be valid", v)
	}

	for _, v := range []float64{-0.01, 100.01, -100, 200} {
		v := v
		require.Error(t, validateExclusiveRate(&v), "value %v should be rejected", v)
	}

	nan := math.NaN()
	require.Error(t, validateExclusiveRate(&nan))
	posInf := math.Inf(1)
	require.Error(t, validateExclusiveRate(&posInf))
	negInf := math.Inf(-1)
	require.Error(t, validateExclusiveRate(&negInf))
}

func TestMaskEmail(t *testing.T) {
	t.Parallel()
	require.Equal(t, "a***@g***.com", maskEmail("alice@gmail.com"))
	require.Equal(t, "x***@d***", maskEmail("x@domain"))
	require.Equal(t, "", maskEmail(""))
}

func TestIsValidAffiliateCodeFormat(t *testing.T) {
	t.Parallel()

	// 邀请码格式校验同时服务于：
	// 1) 系统自动生成的 12 位随机码（A-Z 去 I/O，2-9 去 0/1）
	// 2) 管理员设置的自定义专属码（如 "VIP2026"、"NEW_USER-1"）
	// 因此校验放宽到 [A-Z0-9_-]{4,32}（要求调用方先 ToUpper）。
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"valid canonical 12-char", "ABCDEFGHJKLM", true},
		{"valid all digits 2-9", "234567892345", true},
		{"valid mixed", "A2B3C4D5E6F7", true},
		{"valid admin custom short", "VIP1", true},
		{"valid admin custom with hyphen", "NEW-USER", true},
		{"valid admin custom with underscore", "VIP_2026", true},
		{"valid 32-char max", "ABCDEFGHIJKLMNOPQRSTUVWXYZ012345", true},
		// Previously-excluded chars (I/O/0/1) are now allowed since admins may use them.
		{"letter I now allowed", "IBCDEFGHJKLM", true},
		{"letter O now allowed", "OBCDEFGHJKLM", true},
		{"digit 0 now allowed", "0BCDEFGHJKLM", true},
		{"digit 1 now allowed", "1BCDEFGHJKLM", true},
		{"too short (3 chars)", "ABC", false},
		{"too long (33 chars)", "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456", false},
		{"lowercase rejected (caller must ToUpper first)", "abcdefghjklm", false},
		{"empty", "", false},
		{"utf8 non-ascii", "ÄÄÄÄÄÄ", false}, // bytes out of charset
		{"ascii punctuation .", "ABCDEFGHJK.M", false},
		{"whitespace", "ABCDEFGHJK M", false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, isValidAffiliateCodeFormat(tc.in))
		})
	}
}

type affiliateDetailRepoStub struct {
	summary                *AffiliateSummary
	summaries              map[int64]*AffiliateSummary
	paidInviteeCount       int
	countPaidInviteesCalls int
	accrueCalls            []affiliateAccrueCall
}

type affiliateAccrueCall struct {
	inviterID     int64
	inviteeUserID int64
	amount        float64
	freezeHours   int
	sourceOrderID *int64
}

func affiliateSettingsService(values map[string]string) *SettingService {
	return NewSettingService(&affiliateSettingRepoStub{values: values}, &config.Config{})
}

func (r *affiliateDetailRepoStub) EnsureUserAffiliate(_ context.Context, userID int64) (*AffiliateSummary, error) {
	if r.summaries != nil {
		return r.summaries[userID], nil
	}
	return r.summary, nil
}

func (r *affiliateDetailRepoStub) ListInvitees(context.Context, int64, int) ([]AffiliateInvitee, error) {
	return nil, nil
}

func (r *affiliateDetailRepoStub) CountPaidInvitees(context.Context, int64) (int, error) {
	r.countPaidInviteesCalls++
	return r.paidInviteeCount, nil
}

func (r *affiliateDetailRepoStub) ThawFrozenQuota(context.Context, int64) (float64, error) {
	return 0, nil
}

func (r *affiliateDetailRepoStub) GetAffiliateByCode(context.Context, string) (*AffiliateSummary, error) {
	panic("unexpected GetAffiliateByCode call")
}

func (r *affiliateDetailRepoStub) BindInviter(context.Context, int64, int64) (bool, error) {
	panic("unexpected BindInviter call")
}

func (r *affiliateDetailRepoStub) AccrueQuota(_ context.Context, inviterID, inviteeUserID int64, amount float64, freezeHours int, sourceOrderID *int64) (bool, error) {
	r.accrueCalls = append(r.accrueCalls, affiliateAccrueCall{
		inviterID:     inviterID,
		inviteeUserID: inviteeUserID,
		amount:        amount,
		freezeHours:   freezeHours,
		sourceOrderID: sourceOrderID,
	})
	return true, nil
}

func (r *affiliateDetailRepoStub) GetAccruedRebateFromInvitee(context.Context, int64, int64) (float64, error) {
	panic("unexpected GetAccruedRebateFromInvitee call")
}

func (r *affiliateDetailRepoStub) TransferQuotaToBalance(context.Context, int64) (float64, float64, error) {
	panic("unexpected TransferQuotaToBalance call")
}

func (r *affiliateDetailRepoStub) UpdateUserAffCode(context.Context, int64, string) error {
	panic("unexpected UpdateUserAffCode call")
}

func (r *affiliateDetailRepoStub) ResetUserAffCode(context.Context, int64) (string, error) {
	panic("unexpected ResetUserAffCode call")
}

func (r *affiliateDetailRepoStub) SetUserRebateRate(context.Context, int64, *float64) error {
	panic("unexpected SetUserRebateRate call")
}

func (r *affiliateDetailRepoStub) BatchSetUserRebateRate(context.Context, []int64, *float64) error {
	panic("unexpected BatchSetUserRebateRate call")
}

func (r *affiliateDetailRepoStub) ListUsersWithCustomSettings(context.Context, AffiliateAdminFilter) ([]AffiliateAdminEntry, int64, error) {
	panic("unexpected ListUsersWithCustomSettings call")
}

func (r *affiliateDetailRepoStub) ListAffiliateInviteRecords(context.Context, AffiliateRecordFilter) ([]AffiliateInviteRecord, int64, error) {
	panic("unexpected ListAffiliateInviteRecords call")
}

func (r *affiliateDetailRepoStub) ListAffiliateRebateRecords(context.Context, AffiliateRecordFilter) ([]AffiliateRebateRecord, int64, error) {
	panic("unexpected ListAffiliateRebateRecords call")
}

func (r *affiliateDetailRepoStub) ListAffiliateTransferRecords(context.Context, AffiliateRecordFilter) ([]AffiliateTransferRecord, int64, error) {
	panic("unexpected ListAffiliateTransferRecords call")
}

func (r *affiliateDetailRepoStub) GetAffiliateUserOverview(context.Context, int64) (*AffiliateUserOverview, error) {
	panic("unexpected GetAffiliateUserOverview call")
}
