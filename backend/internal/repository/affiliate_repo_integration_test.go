//go:build integration

package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	migrationspkg "github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

type failingAccrueAffiliateRepository struct {
	service.AffiliateRepository
	err error
}

func (r *failingAccrueAffiliateRepository) AccrueQuota(context.Context, service.AffiliateAccrualInput) (float64, error) {
	return 0, r.err
}

func newAdminServiceForAffiliateIntegration(
	userRepo service.UserRepository,
	redeemRepo service.RedeemCodeRepository,
	client *dbent.Client,
	settingService *service.SettingService,
	affiliateService *service.AffiliateService,
) service.AdminService {
	return service.NewAdminService(
		nil,
		userRepo,
		nil,
		nil,
		nil,
		nil,
		redeemRepo,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		client,
		settingService,
		nil,
		nil,
		nil,
		nil,
		affiliateService,
		nil,
		nil,
		nil,
	)
}

func querySingleFloat(t *testing.T, ctx context.Context, client *dbent.Client, query string, args ...any) float64 {
	t.Helper()
	rows, err := client.QueryContext(ctx, query, args...)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	require.True(t, rows.Next(), "expected one row")
	var value float64
	require.NoError(t, rows.Scan(&value))
	require.NoError(t, rows.Err())
	return value
}

func querySingleInt(t *testing.T, ctx context.Context, client *dbent.Client, query string, args ...any) int {
	t.Helper()
	rows, err := client.QueryContext(ctx, query, args...)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	require.True(t, rows.Next(), "expected one row")
	var value int
	require.NoError(t, rows.Scan(&value))
	require.NoError(t, rows.Err())
	return value
}

func registerCommittedAffiliateTestCleanup(t *testing.T, userIDs, redeemCodeIDs *[]int64) {
	t.Helper()
	t.Cleanup(func() {
		ctx := context.Background()
		tx, err := integrationDB.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()

		// 这些用例必须提交真实多事务数据；按已登记主键清理，避免污染同包后续查询。
		for _, redeemCodeID := range *redeemCodeIDs {
			_, err = tx.ExecContext(ctx, "DELETE FROM user_affiliate_ledger WHERE source_redeem_code_id = $1", redeemCodeID)
			require.NoError(t, err)
		}
		for _, userID := range *userIDs {
			_, err = tx.ExecContext(ctx, "DELETE FROM user_affiliate_ledger WHERE user_id = $1 OR source_user_id = $1", userID)
			require.NoError(t, err)
			_, err = tx.ExecContext(ctx, "DELETE FROM redeem_codes WHERE used_by = $1", userID)
			require.NoError(t, err)
		}
		for _, redeemCodeID := range *redeemCodeIDs {
			_, err = tx.ExecContext(ctx, "DELETE FROM redeem_codes WHERE id = $1", redeemCodeID)
			require.NoError(t, err)
		}
		for _, userID := range *userIDs {
			_, err = tx.ExecContext(ctx, "DELETE FROM user_affiliates WHERE user_id = $1 OR inviter_id = $1", userID)
			require.NoError(t, err)
			_, err = tx.ExecContext(ctx, "DELETE FROM users WHERE id = $1", userID)
			require.NoError(t, err)
		}
		require.NoError(t, tx.Commit())
	})
}

func registerIntegrationSettingRestore(t *testing.T, ctx context.Context, settingRepo service.SettingRepository, keys []string) {
	t.Helper()
	previousSettings, err := settingRepo.GetMultiple(ctx, keys)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, settingRepo.SetMultiple(ctx, previousSettings))
		for _, key := range keys {
			if _, existed := previousSettings[key]; !existed {
				require.NoError(t, settingRepo.Delete(ctx, key))
			}
		}
	})
}

func TestAffiliateRepository_TransferQuotaToBalance_UsesClaimedQuotaBeforeClear(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	txCtx := dbent.NewTxContext(ctx, tx)
	client := tx.Client()

	repo := NewAffiliateRepository(client, integrationDB)

	u := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("affiliate-transfer-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
		Balance:      5.5,
		Concurrency:  5,
	})

	affCode := fmt.Sprintf("AFF%09d", time.Now().UnixNano()%1_000_000_000)
	_, err := client.ExecContext(txCtx, `
INSERT INTO user_affiliates (user_id, aff_code, aff_quota, aff_history_quota, created_at, updated_at)
VALUES ($1, $2, $3, $3, NOW(), NOW())`, u.ID, affCode, 12.34)
	require.NoError(t, err)

	transferred, balance, err := repo.TransferQuotaToBalance(txCtx, u.ID)
	require.NoError(t, err)
	require.InDelta(t, 12.34, transferred, 1e-9)
	require.InDelta(t, 17.84, balance, 1e-9)

	affQuota := querySingleFloat(t, txCtx, client,
		"SELECT aff_quota::double precision FROM user_affiliates WHERE user_id = $1", u.ID)
	require.InDelta(t, 0.0, affQuota, 1e-9)

	persistedBalance := querySingleFloat(t, txCtx, client,
		"SELECT balance::double precision FROM users WHERE id = $1", u.ID)
	require.InDelta(t, 17.84, persistedBalance, 1e-9)

	ledgerCount := querySingleInt(t, txCtx, client,
		"SELECT COUNT(*) FROM user_affiliate_ledger WHERE user_id = $1 AND action = 'transfer'", u.ID)
	require.Equal(t, 1, ledgerCount)

	rows, err := client.QueryContext(txCtx, `
SELECT amount::double precision,
       balance_after::double precision,
       aff_quota_after::double precision,
       aff_frozen_quota_after::double precision,
       aff_history_quota_after::double precision
FROM user_affiliate_ledger
WHERE user_id = $1 AND action = 'transfer'
LIMIT 1`, u.ID)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	require.True(t, rows.Next(), "expected transfer ledger")
	var amount, balanceAfter, quotaAfter, frozenAfter, historyAfter float64
	require.NoError(t, rows.Scan(&amount, &balanceAfter, &quotaAfter, &frozenAfter, &historyAfter))
	require.InDelta(t, 12.34, amount, 1e-9)
	require.InDelta(t, 17.84, balanceAfter, 1e-9)
	require.InDelta(t, 0.0, quotaAfter, 1e-9)
	require.InDelta(t, 0.0, frozenAfter, 1e-9)
	require.InDelta(t, 12.34, historyAfter, 1e-9)
}

// TestAffiliateRepository_AccrueQuota_ReusesOuterTransaction guards the
// cross-layer tx propagation invariant: when AccrueQuota is called with a ctx
// that already carries a transaction (via dbent.NewTxContext), repo.withTx
// must reuse that tx rather than opening a nested one. If this invariant
// breaks, AccrueQuota would commit independently and survive a rollback of
// the outer tx, which would violate payment_fulfillment's all-or-nothing
// semantics.
func TestAffiliateRepository_AccrueQuota_ReusesOuterTransaction(t *testing.T) {
	ctx := context.Background()

	outerTx, err := integrationEntClient.Tx(ctx)
	require.NoError(t, err, "begin outer tx")
	// Defensive cleanup: if any require.* below fires before the explicit
	// Rollback, this prevents the tx from leaking until container teardown.
	// Rollback is idempotent at the driver level (extra rollback returns an
	// error we ignore).
	t.Cleanup(func() { _ = outerTx.Rollback() })
	client := outerTx.Client()
	txCtx := dbent.NewTxContext(ctx, outerTx)

	inviter := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("affiliate-inviter-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
		Concurrency:  5,
	})
	invitee := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("affiliate-invitee-%d@example.com", time.Now().UnixNano()+1),
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
		Concurrency:  5,
	})

	repo := NewAffiliateRepository(client, integrationDB)
	_, err = repo.EnsureUserAffiliate(txCtx, inviter.ID)
	require.NoError(t, err)
	_, err = repo.EnsureUserAffiliate(txCtx, invitee.ID)
	require.NoError(t, err)

	bound, err := repo.BindInviter(txCtx, invitee.ID, inviter.ID)
	require.NoError(t, err)
	require.True(t, bound, "invitee must bind to inviter")
	adminCodeID := insertHistoricalRedeemCode(t, txCtx, client, invitee.ID, "admin_balance", 17.5, time.Now())

	applied, err := repo.AccrueQuota(txCtx, service.AffiliateAccrualInput{
		InviterID:     inviter.ID,
		InviteeUserID: invitee.ID,
		Amount:        3.5,
		Source: service.AffiliateRebateSource{
			Type:         service.AffiliateRebateSourceAdminRecharge,
			BaseAmount:   17.5,
			RedeemCodeID: &adminCodeID,
		},
	})
	require.NoError(t, err)
	require.InDelta(t, 3.5, applied, 1e-9, "AccrueQuota must return the applied amount")

	// Visible inside the outer tx.
	innerQuota := querySingleFloat(t, txCtx, client,
		"SELECT aff_quota::double precision FROM user_affiliates WHERE user_id = $1", inviter.ID)
	require.InDelta(t, 3.5, innerQuota, 1e-9)

	// Roll back the outer tx; if AccrueQuota had opened its own inner tx and
	// committed it, the rows would still be visible to the global client.
	require.NoError(t, outerTx.Rollback())

	rows, err := integrationEntClient.QueryContext(ctx,
		"SELECT COUNT(*) FROM user_affiliates WHERE user_id IN ($1, $2)",
		inviter.ID, invitee.ID)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	require.True(t, rows.Next())
	var postRollbackCount int
	require.NoError(t, rows.Scan(&postRollbackCount))
	require.Equal(t, 0, postRollbackCount,
		"AccrueQuota must propagate the outer tx — found persisted rows after rollback")
}

func TestAffiliateRepository_AccrueQuota_RedeemSourceIsIdempotentAndListable(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	txCtx := dbent.NewTxContext(ctx, tx)
	client := tx.Client()
	repo := NewAffiliateRepository(client, integrationDB)

	inviter := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("affiliate-redeem-inviter-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
	})
	invitee := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("affiliate-redeem-invitee-%d@example.com", time.Now().UnixNano()+1),
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
	})
	_, err := repo.EnsureUserAffiliate(txCtx, inviter.ID)
	require.NoError(t, err)
	_, err = repo.EnsureUserAffiliate(txCtx, invitee.ID)
	require.NoError(t, err)
	bound, err := repo.BindInviter(txCtx, invitee.ID, inviter.ID)
	require.NoError(t, err)
	require.True(t, bound)

	shortRedeemCode := fmt.Sprintf("%04X", time.Now().UnixNano()&0xffff)
	rows, err := client.QueryContext(txCtx, `
INSERT INTO redeem_codes (code, type, value, status, used_by, used_at, created_at)
VALUES ($1, 'balance', 50, 'used', $2, NOW(), NOW())
RETURNING id`, shortRedeemCode, invitee.ID)
	require.NoError(t, err)
	require.True(t, rows.Next())
	var redeemCodeID int64
	require.NoError(t, rows.Scan(&redeemCodeID))
	require.NoError(t, rows.Close())

	input := service.AffiliateAccrualInput{
		InviterID:     inviter.ID,
		InviteeUserID: invitee.ID,
		Amount:        10,
		Source: service.AffiliateRebateSource{
			Type:         service.AffiliateRebateSourceBalanceRedeem,
			BaseAmount:   50,
			RedeemCodeID: &redeemCodeID,
		},
	}
	wrongInviteeInput := input
	wrongInviteeInput.InviteeUserID = inviter.ID
	_, err = repo.AccrueQuota(txCtx, wrongInviteeInput)
	require.ErrorIs(t, err, service.ErrAffiliateRebateSource)
	wrongTypeInput := input
	wrongTypeInput.Source.Type = service.AffiliateRebateSourceAdminRecharge
	_, err = repo.AccrueQuota(txCtx, wrongTypeInput)
	require.ErrorIs(t, err, service.ErrAffiliateRebateSource)

	first, err := repo.AccrueQuota(txCtx, input)
	require.NoError(t, err)
	require.InDelta(t, 10, first, 1e-9)
	second, err := repo.AccrueQuota(txCtx, input)
	require.NoError(t, err)
	require.Zero(t, second)

	quota := querySingleFloat(t, txCtx, client,
		"SELECT aff_quota::double precision FROM user_affiliates WHERE user_id = $1", inviter.ID)
	require.InDelta(t, 10, quota, 1e-9)
	ledgerCount := querySingleInt(t, txCtx, client,
		"SELECT COUNT(*) FROM user_affiliate_ledger WHERE source_redeem_code_id = $1", redeemCodeID)
	require.Equal(t, 1, ledgerCount)
	for range 2 {
		additionalCodeID := insertHistoricalRedeemCode(t, txCtx, client, invitee.ID, "balance", 50, time.Now())
		_, err = repo.AccrueQuota(txCtx, service.AffiliateAccrualInput{
			InviterID:     inviter.ID,
			InviteeUserID: invitee.ID,
			Amount:        10,
			Source: service.AffiliateRebateSource{
				Type:         service.AffiliateRebateSourceBalanceRedeem,
				BaseAmount:   50,
				RedeemCodeID: &additionalCodeID,
			},
		})
		require.NoError(t, err)
	}

	items, total, err := repo.ListAffiliateRebateRecords(txCtx, service.AffiliateRecordFilter{
		SourceType: string(service.AffiliateRebateSourceBalanceRedeem),
		Page:       1,
		PageSize:   20,
	})
	require.NoError(t, err)
	require.Equal(t, int64(3), total)
	require.Len(t, items, 3)
	var originalRecord *service.AffiliateRebateRecord
	for i := range items {
		if items[i].RedeemCodeID != nil && *items[i].RedeemCodeID == redeemCodeID {
			originalRecord = &items[i]
			break
		}
	}
	require.NotNil(t, originalRecord)
	require.Equal(t, string(service.AffiliateRebateSourceBalanceRedeem), originalRecord.SourceType)
	require.Equal(t, "****", *originalRecord.RedeemCodeMasked)
	require.NotContains(t, *originalRecord.RedeemCodeMasked, shortRedeemCode)
	require.InDelta(t, 50, *originalRecord.BaseAmount, 1e-9)
	require.InDelta(t, 10, originalRecord.RebateAmount, 1e-9)

	pageFilter := service.AffiliateRecordFilter{
		SourceType: string(service.AffiliateRebateSourceBalanceRedeem),
		Page:       1,
		PageSize:   2,
		SortBy:     "source",
	}
	firstPage, _, err := repo.ListAffiliateRebateRecords(txCtx, pageFilter)
	require.NoError(t, err)
	pageFilter.Page = 2
	secondPage, _, err := repo.ListAffiliateRebateRecords(txCtx, pageFilter)
	require.NoError(t, err)
	require.Len(t, firstPage, 2)
	require.Len(t, secondPage, 1)
	seenLedgerIDs := map[int64]struct{}{}
	for _, item := range append(firstPage, secondPage...) {
		seenLedgerIDs[item.LedgerID] = struct{}{}
	}
	require.Len(t, seenLedgerIDs, 3, "来源字段并列时跨页记录不能重复或遗漏")
	pageFilter.Page = 1
	repeatedFirstPage, _, err := repo.ListAffiliateRebateRecords(txCtx, pageFilter)
	require.NoError(t, err)
	require.Equal(t, []int64{firstPage[0].LedgerID, firstPage[1].LedgerID}, []int64{repeatedFirstPage[0].LedgerID, repeatedFirstPage[1].LedgerID})
}

func TestAffiliateRepository_AccrueQuota_ConcurrentSameRedeemSourceIsIdempotent(t *testing.T) {
	ctx := context.Background()
	repo := NewAffiliateRepository(integrationEntClient, integrationDB)
	var userIDs, redeemCodeIDs []int64
	registerCommittedAffiliateTestCleanup(t, &userIDs, &redeemCodeIDs)

	inviter := mustCreateUser(t, integrationEntClient, &service.User{
		Email:        fmt.Sprintf("affiliate-source-race-inviter-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
	})
	userIDs = append(userIDs, inviter.ID)
	invitee := mustCreateUser(t, integrationEntClient, &service.User{
		Email:        fmt.Sprintf("affiliate-source-race-invitee-%d@example.com", time.Now().UnixNano()+1),
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
	})
	userIDs = append(userIDs, invitee.ID)
	_, err := repo.EnsureUserAffiliate(ctx, inviter.ID)
	require.NoError(t, err)
	_, err = repo.EnsureUserAffiliate(ctx, invitee.ID)
	require.NoError(t, err)

	rows, err := integrationEntClient.QueryContext(ctx, `
INSERT INTO redeem_codes (code, type, value, status, used_by, used_at, created_at)
VALUES ($1, 'balance', 50, 'used', $2, NOW(), NOW())
RETURNING id`, fmt.Sprintf("RACE%d", time.Now().UnixNano()), invitee.ID)
	require.NoError(t, err)
	require.True(t, rows.Next())
	var redeemCodeID int64
	require.NoError(t, rows.Scan(&redeemCodeID))
	require.NoError(t, rows.Close())
	redeemCodeIDs = append(redeemCodeIDs, redeemCodeID)

	start := make(chan struct{})
	results := make(chan struct {
		amount float64
		err    error
	}, 2)
	for range 2 {
		go func() {
			<-start
			amount, accrueErr := repo.AccrueQuota(ctx, service.AffiliateAccrualInput{
				InviterID:     inviter.ID,
				InviteeUserID: invitee.ID,
				Amount:        10,
				Source: service.AffiliateRebateSource{
					Type:         service.AffiliateRebateSourceBalanceRedeem,
					BaseAmount:   50,
					RedeemCodeID: &redeemCodeID,
				},
			})
			results <- struct {
				amount float64
				err    error
			}{amount: amount, err: accrueErr}
		}()
	}
	close(start)

	var appliedTotal float64
	for range 2 {
		result := <-results
		require.NoError(t, result.err)
		appliedTotal += result.amount
	}
	require.InDelta(t, 10, appliedTotal, 1e-9)
	ledgerCount := querySingleInt(t, ctx, integrationEntClient,
		"SELECT COUNT(*) FROM user_affiliate_ledger WHERE source_redeem_code_id = $1", redeemCodeID)
	require.Equal(t, 1, ledgerCount)
	quota := querySingleFloat(t, ctx, integrationEntClient,
		"SELECT aff_quota::double precision FROM user_affiliates WHERE user_id = $1", inviter.ID)
	require.InDelta(t, 10, quota, 1e-9)
}

func TestAffiliateRepository_AccrueQuota_TruncatesAtPerInviteeCap(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	txCtx := dbent.NewTxContext(ctx, tx)
	client := tx.Client()
	repo := NewAffiliateRepository(client, integrationDB)

	inviter := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("affiliate-cap-inviter-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
	})
	invitee := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("affiliate-cap-invitee-%d@example.com", time.Now().UnixNano()+1),
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
	})
	_, err := repo.EnsureUserAffiliate(txCtx, inviter.ID)
	require.NoError(t, err)
	_, err = repo.EnsureUserAffiliate(txCtx, invitee.ID)
	require.NoError(t, err)

	adminCodeIDs := []int64{
		insertHistoricalRedeemCode(t, txCtx, client, invitee.ID, "admin_balance", 20, time.Now()),
		insertHistoricalRedeemCode(t, txCtx, client, invitee.ID, "admin_balance", 20, time.Now()),
		insertHistoricalRedeemCode(t, txCtx, client, invitee.ID, "admin_balance", 20, time.Now()),
	}
	inputForCode := func(codeID int64) service.AffiliateAccrualInput {
		return service.AffiliateAccrualInput{
			InviterID:     inviter.ID,
			InviteeUserID: invitee.ID,
			Amount:        4.12345678,
			PerInviteeCap: 5.000000009,
			Source: service.AffiliateRebateSource{
				Type:         service.AffiliateRebateSourceAdminRecharge,
				BaseAmount:   20,
				RedeemCodeID: &codeID,
			},
		}
	}
	first, err := repo.AccrueQuota(txCtx, inputForCode(adminCodeIDs[0]))
	require.NoError(t, err)
	require.InDelta(t, 4.12345678, first, 1e-9)
	second, err := repo.AccrueQuota(txCtx, inputForCode(adminCodeIDs[1]))
	require.NoError(t, err)
	require.InDelta(t, 0.87654322, second, 1e-9)
	third, err := repo.AccrueQuota(txCtx, inputForCode(adminCodeIDs[2]))
	require.NoError(t, err)
	require.Zero(t, third)

	quota := querySingleFloat(t, txCtx, client,
		"SELECT aff_quota::double precision FROM user_affiliates WHERE user_id = $1", inviter.ID)
	require.InDelta(t, 5, quota, 1e-9)
}

func TestAffiliateRepository_AccrueQuota_ConcurrentRequestsRespectPerInviteeCap(t *testing.T) {
	ctx := context.Background()
	repo := NewAffiliateRepository(integrationEntClient, integrationDB)
	var userIDs, redeemCodeIDs []int64
	registerCommittedAffiliateTestCleanup(t, &userIDs, &redeemCodeIDs)

	inviter := mustCreateUser(t, integrationEntClient, &service.User{
		Email:        fmt.Sprintf("affiliate-concurrent-cap-inviter-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
	})
	userIDs = append(userIDs, inviter.ID)
	invitee := mustCreateUser(t, integrationEntClient, &service.User{
		Email:        fmt.Sprintf("affiliate-concurrent-cap-invitee-%d@example.com", time.Now().UnixNano()+1),
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
	})
	userIDs = append(userIDs, invitee.ID)
	_, err := repo.EnsureUserAffiliate(ctx, inviter.ID)
	require.NoError(t, err)
	_, err = repo.EnsureUserAffiliate(ctx, invitee.ID)
	require.NoError(t, err)
	adminCodeIDs := []int64{
		insertHistoricalRedeemCode(t, ctx, integrationEntClient, invitee.ID, "admin_balance", 20, time.Now()),
		insertHistoricalRedeemCode(t, ctx, integrationEntClient, invitee.ID, "admin_balance", 20, time.Now()),
	}
	redeemCodeIDs = append(redeemCodeIDs, adminCodeIDs...)

	type result struct {
		amount float64
		err    error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	for i := range 2 {
		go func() {
			<-start
			adminCodeID := adminCodeIDs[i]
			amount, accrueErr := repo.AccrueQuota(ctx, service.AffiliateAccrualInput{
				InviterID:     inviter.ID,
				InviteeUserID: invitee.ID,
				Amount:        4,
				PerInviteeCap: 5,
				Source: service.AffiliateRebateSource{
					Type:         service.AffiliateRebateSourceAdminRecharge,
					BaseAmount:   20,
					RedeemCodeID: &adminCodeID,
				},
			})
			results <- result{amount: amount, err: accrueErr}
		}()
	}
	close(start)

	var appliedTotal float64
	for range 2 {
		result := <-results
		require.NoError(t, result.err)
		appliedTotal += result.amount
	}
	require.InDelta(t, 5, appliedTotal, 1e-9)

	quota := querySingleFloat(t, ctx, integrationEntClient,
		"SELECT aff_quota::double precision FROM user_affiliates WHERE user_id = $1", inviter.ID)
	require.InDelta(t, 5, quota, 1e-9)
	ledgerTotal := querySingleFloat(t, ctx, integrationEntClient,
		"SELECT COALESCE(SUM(amount), 0)::double precision FROM user_affiliate_ledger WHERE user_id = $1 AND source_user_id = $2 AND action = 'accrue'",
		inviter.ID, invitee.ID)
	require.InDelta(t, 5, ledgerTotal, 1e-9)
}

func TestAdminService_UpdateUserBalance_AdminRechargeCommitsOrRollsBackAsOneTransaction(t *testing.T) {
	ctx := context.Background()
	userRepo := NewUserRepository(integrationEntClient, integrationDB)
	redeemRepo := NewRedeemCodeRepository(integrationEntClient)
	affiliateRepo := NewAffiliateRepository(integrationEntClient, integrationDB)
	settingRepo := NewSettingRepository(integrationEntClient)
	settingKeys := []string{
		service.SettingKeyAffiliateEnabled,
		service.SettingKeyAffiliateAdminRechargeEnabled,
		service.SettingKeyAffiliateRebateRate,
	}
	registerIntegrationSettingRestore(t, ctx, settingRepo, settingKeys)
	require.NoError(t, settingRepo.SetMultiple(ctx, map[string]string{
		service.SettingKeyAffiliateEnabled:              "true",
		service.SettingKeyAffiliateAdminRechargeEnabled: "true",
		service.SettingKeyAffiliateRebateRate:           "20",
	}))
	settingService := service.NewSettingService(settingRepo, nil)
	var userIDs, redeemCodeIDs []int64
	registerCommittedAffiliateTestCleanup(t, &userIDs, &redeemCodeIDs)

	inviter := mustCreateUser(t, integrationEntClient, &service.User{
		Email:        fmt.Sprintf("admin-recharge-rollback-inviter-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
	})
	userIDs = append(userIDs, inviter.ID)
	invitee := mustCreateUser(t, integrationEntClient, &service.User{
		Email:        fmt.Sprintf("admin-recharge-rollback-invitee-%d@example.com", time.Now().UnixNano()+1),
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
		Balance:      10,
	})
	userIDs = append(userIDs, invitee.ID)
	_, err := affiliateRepo.EnsureUserAffiliate(ctx, inviter.ID)
	require.NoError(t, err)
	_, err = affiliateRepo.EnsureUserAffiliate(ctx, invitee.ID)
	require.NoError(t, err)
	bound, err := affiliateRepo.BindInviter(ctx, invitee.ID, inviter.ID)
	require.NoError(t, err)
	require.True(t, bound)

	successfulAffiliateService := service.NewAffiliateService(affiliateRepo, settingService, nil, nil)
	successfulAdminService := newAdminServiceForAffiliateIntegration(
		userRepo, redeemRepo, integrationEntClient, settingService, successfulAffiliateService,
	)
	updated, err := successfulAdminService.UpdateUserBalance(ctx, invitee.ID, 5, "add", "commit-test")
	require.NoError(t, err)
	require.InDelta(t, 15, updated.Balance, 1e-9)

	rows, err := integrationEntClient.QueryContext(ctx, `
SELECT rc.id,
       ual.source_type,
       ual.base_amount::double precision,
       ual.amount::double precision
FROM redeem_codes rc
JOIN user_affiliate_ledger ual ON ual.source_redeem_code_id = rc.id
WHERE rc.used_by = $1 AND rc.type = 'admin_balance'`, invitee.ID)
	require.NoError(t, err)
	require.True(t, rows.Next())
	var adjustmentRecordID int64
	var sourceType string
	var baseAmount, rebateAmount float64
	require.NoError(t, rows.Scan(&adjustmentRecordID, &sourceType, &baseAmount, &rebateAmount))
	require.NoError(t, rows.Close())
	redeemCodeIDs = append(redeemCodeIDs, adjustmentRecordID)
	require.Positive(t, adjustmentRecordID)
	require.Equal(t, string(service.AffiliateRebateSourceAdminRecharge), sourceType)
	require.InDelta(t, 5, baseAmount, 1e-9)
	require.InDelta(t, 1, rebateAmount, 1e-9)

	failingAffiliateService := service.NewAffiliateService(&failingAccrueAffiliateRepository{
		AffiliateRepository: affiliateRepo,
		err:                 errors.New("forced affiliate failure"),
	}, settingService, nil, nil)
	failingAdminService := newAdminServiceForAffiliateIntegration(
		userRepo, redeemRepo, integrationEntClient, settingService, failingAffiliateService,
	)

	updated, err = failingAdminService.UpdateUserBalance(ctx, invitee.ID, 5, "add", "rollback-test")
	require.Nil(t, updated)
	require.ErrorContains(t, err, "forced affiliate failure")

	balance := querySingleFloat(t, ctx, integrationEntClient,
		"SELECT balance::double precision FROM users WHERE id = $1", invitee.ID)
	require.InDelta(t, 15, balance, 1e-9)
	adjustmentCount := querySingleInt(t, ctx, integrationEntClient,
		"SELECT COUNT(*) FROM redeem_codes WHERE used_by = $1 AND type = 'admin_balance'", invitee.ID)
	require.Equal(t, 1, adjustmentCount)
	ledgerCount := querySingleInt(t, ctx, integrationEntClient,
		"SELECT COUNT(*) FROM user_affiliate_ledger WHERE user_id = $1 AND source_user_id = $2 AND action = 'accrue'",
		inviter.ID, invitee.ID)
	require.Equal(t, 1, ledgerCount)
}

func TestRedeemService_BalanceAndAffiliateRebateShareTransaction(t *testing.T) {
	ctx := context.Background()
	userRepo := NewUserRepository(integrationEntClient, integrationDB)
	redeemRepo := NewRedeemCodeRepository(integrationEntClient)
	affiliateRepo := NewAffiliateRepository(integrationEntClient, integrationDB)
	settingRepo := NewSettingRepository(integrationEntClient)
	settingKeys := []string{
		service.SettingKeyAffiliateEnabled,
		service.SettingKeyAffiliateRebateRate,
		service.SettingKeyAffiliateRebateFreezeHours,
		service.SettingKeyAffiliateRebateDurationDays,
		service.SettingKeyAffiliateRebatePerInviteeCap,
	}
	registerIntegrationSettingRestore(t, ctx, settingRepo, settingKeys)
	require.NoError(t, settingRepo.SetMultiple(ctx, map[string]string{
		service.SettingKeyAffiliateEnabled:             "true",
		service.SettingKeyAffiliateRebateRate:          "20",
		service.SettingKeyAffiliateRebateFreezeHours:   "0",
		service.SettingKeyAffiliateRebateDurationDays:  "0",
		service.SettingKeyAffiliateRebatePerInviteeCap: "0",
	}))
	settingService := service.NewSettingService(settingRepo, nil)
	var userIDs, redeemCodeIDs []int64
	registerCommittedAffiliateTestCleanup(t, &userIDs, &redeemCodeIDs)

	inviter := mustCreateUser(t, integrationEntClient, &service.User{
		Email:        fmt.Sprintf("redeem-transaction-inviter-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
	})
	userIDs = append(userIDs, inviter.ID)
	invitee := mustCreateUser(t, integrationEntClient, &service.User{
		Email:        fmt.Sprintf("redeem-transaction-invitee-%d@example.com", time.Now().UnixNano()+1),
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
		Balance:      10,
	})
	userIDs = append(userIDs, invitee.ID)
	_, err := affiliateRepo.EnsureUserAffiliate(ctx, inviter.ID)
	require.NoError(t, err)
	_, err = affiliateRepo.EnsureUserAffiliate(ctx, invitee.ID)
	require.NoError(t, err)
	bound, err := affiliateRepo.BindInviter(ctx, invitee.ID, inviter.ID)
	require.NoError(t, err)
	require.True(t, bound)

	codeSequence := 0
	createBalanceCode := func(value float64) *service.RedeemCode {
		codeSequence++
		code := &service.RedeemCode{
			Code:   fmt.Sprintf("REALREDEEM%d-%d", time.Now().UnixNano(), codeSequence),
			Type:   service.RedeemTypeBalance,
			Value:  value,
			Status: service.StatusUnused,
		}
		require.NoError(t, redeemRepo.Create(ctx, code))
		require.Positive(t, code.ID)
		redeemCodeIDs = append(redeemCodeIDs, code.ID)
		return code
	}
	ledgerCountForCode := func(codeID int64) int {
		return querySingleInt(t, ctx, integrationEntClient,
			"SELECT COUNT(*) FROM user_affiliate_ledger WHERE source_redeem_code_id = $1", codeID)
	}

	affiliateService := service.NewAffiliateService(affiliateRepo, settingService, nil, nil)
	redeemService := service.NewRedeemService(redeemRepo, userRepo, nil, nil, nil, integrationEntClient, nil, affiliateService)

	successCode := createBalanceCode(5)
	redeemed, err := redeemService.Redeem(ctx, invitee.ID, successCode.Code)
	require.NoError(t, err)
	require.Equal(t, service.StatusUsed, redeemed.Status)
	require.InDelta(t, 15, querySingleFloat(t, ctx, integrationEntClient,
		"SELECT balance::double precision FROM users WHERE id = $1", invitee.ID), 1e-9)
	require.InDelta(t, 1, querySingleFloat(t, ctx, integrationEntClient,
		"SELECT aff_quota::double precision FROM user_affiliates WHERE user_id = $1", inviter.ID), 1e-9)
	require.Equal(t, 1, ledgerCountForCode(successCode.ID))

	failingAffiliateService := service.NewAffiliateService(&failingAccrueAffiliateRepository{
		AffiliateRepository: affiliateRepo,
		err:                 errors.New("forced redeem affiliate failure"),
	}, settingService, nil, nil)
	failingRedeemService := service.NewRedeemService(redeemRepo, userRepo, nil, nil, nil, integrationEntClient, nil, failingAffiliateService)
	failureCode := createBalanceCode(7)
	redeemed, err = failingRedeemService.Redeem(ctx, invitee.ID, failureCode.Code)
	require.Nil(t, redeemed)
	require.ErrorContains(t, err, "forced redeem affiliate failure")
	reloadedFailureCode, err := redeemRepo.GetByID(ctx, failureCode.ID)
	require.NoError(t, err)
	require.Equal(t, service.StatusUnused, reloadedFailureCode.Status)
	require.Nil(t, reloadedFailureCode.UsedBy)
	require.InDelta(t, 15, querySingleFloat(t, ctx, integrationEntClient,
		"SELECT balance::double precision FROM users WHERE id = $1", invitee.ID), 1e-9)
	require.InDelta(t, 1, querySingleFloat(t, ctx, integrationEntClient,
		"SELECT aff_quota::double precision FROM user_affiliates WHERE user_id = $1", inviter.ID), 1e-9)
	require.Zero(t, ledgerCountForCode(failureCode.ID))

	skipCode := createBalanceCode(3)
	redeemed, err = redeemService.Redeem(service.ContextSkipRedeemAffiliate(ctx), invitee.ID, skipCode.Code)
	require.NoError(t, err)
	require.Equal(t, service.StatusUsed, redeemed.Status)
	require.InDelta(t, 18, querySingleFloat(t, ctx, integrationEntClient,
		"SELECT balance::double precision FROM users WHERE id = $1", invitee.ID), 1e-9)
	require.InDelta(t, 1, querySingleFloat(t, ctx, integrationEntClient,
		"SELECT aff_quota::double precision FROM user_affiliates WHERE user_id = $1", inviter.ID), 1e-9)
	require.Zero(t, ledgerCountForCode(skipCode.ID))

	concurrentCode := createBalanceCode(4)
	start := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			_, redeemErr := redeemService.Redeem(ctx, invitee.ID, concurrentCode.Code)
			results <- redeemErr
		}()
	}
	close(start)
	successCount := 0
	failureCount := 0
	for range 2 {
		if redeemErr := <-results; redeemErr == nil {
			successCount++
		} else {
			failureCount++
			require.ErrorIs(t, redeemErr, service.ErrRedeemCodeUsed)
		}
	}
	require.Equal(t, 1, successCount)
	require.Equal(t, 1, failureCount)
	require.InDelta(t, 22, querySingleFloat(t, ctx, integrationEntClient,
		"SELECT balance::double precision FROM users WHERE id = $1", invitee.ID), 1e-9)
	require.InDelta(t, 1.8, querySingleFloat(t, ctx, integrationEntClient,
		"SELECT aff_quota::double precision FROM user_affiliates WHERE user_id = $1", inviter.ID), 1e-9)
	require.Equal(t, 1, ledgerCountForCode(concurrentCode.ID))
}

func TestAffiliateRebateSourcesMigration_DoesNotGuessHistoryAndRepairsSupersededDraft(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	txCtx := dbent.NewTxContext(ctx, tx)
	client := tx.Client()
	repo := NewAffiliateRepository(client, integrationDB)

	inviter := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("migration-source-inviter-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
	})
	ordinaryInvitee := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("migration-source-ordinary-%d@example.com", time.Now().UnixNano()+1),
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
	})
	adminInvitee := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("migration-source-admin-%d@example.com", time.Now().UnixNano()+2),
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
	})
	_, err := repo.EnsureUserAffiliate(txCtx, inviter.ID)
	require.NoError(t, err)
	for _, inviteeID := range []int64{ordinaryInvitee.ID, adminInvitee.ID} {
		_, err = repo.EnsureUserAffiliate(txCtx, inviteeID)
		require.NoError(t, err)
		bound, bindErr := repo.BindInviter(txCtx, inviteeID, inviter.ID)
		require.NoError(t, bindErr)
		require.True(t, bound)
	}

	_, err = client.ExecContext(txCtx, "ALTER TABLE user_affiliate_ledger DROP CONSTRAINT IF EXISTS chk_user_affiliate_ledger_source_type")
	require.NoError(t, err)
	now := time.Now().UTC()
	ordinaryCodeID := insertHistoricalRedeemCode(t, txCtx, client, ordinaryInvitee.ID, "balance", 50, now.Add(-time.Minute))
	ordinaryLedgerID := insertHistoricalAffiliateLedger(t, txCtx, client, inviter.ID, ordinaryInvitee.ID, 10, now)
	adminCodeID := insertHistoricalRedeemCode(t, txCtx, client, adminInvitee.ID, "admin_balance", 25, now)
	adminLedgerID := insertHistoricalAffiliateLedger(t, txCtx, client, inviter.ID, adminInvitee.ID, 5, now.Add(-time.Minute))

	coreMigrationSQL, err := migrationspkg.FS.ReadFile("231_affiliate_rebate_sources.sql")
	require.NoError(t, err)
	for range 2 {
		_, err = client.ExecContext(txCtx, string(coreMigrationSQL))
		require.NoError(t, err)
	}

	rows, err := client.QueryContext(txCtx, `
SELECT source_type, source_redeem_code_id
FROM user_affiliate_ledger
WHERE id = $1`, ordinaryLedgerID)
	require.NoError(t, err)
	require.True(t, rows.Next())
	var ordinarySource *string
	var ordinarySourceID *int64
	require.NoError(t, rows.Scan(&ordinarySource, &ordinarySourceID))
	require.NoError(t, rows.Close())
	require.Nil(t, ordinarySource, "无可靠业务编号的历史余额返利必须保持未分类")
	require.Nil(t, ordinarySourceID)

	// 模拟早期分支迁移已经按时间窗错误关联了管理员调整记录。
	oldMigrationAppliedAt := now.Add(time.Minute)
	_, err = client.ExecContext(txCtx, `
UPDATE schema_migrations
SET checksum = $1, applied_at = $2
WHERE filename = '231_affiliate_rebate_sources.sql'`,
		"ceb508efbf81877a891a95fe6688cb3287462c2552e1a5c8a8254be9328d6806",
		oldMigrationAppliedAt,
	)
	require.NoError(t, err)
	_, err = client.ExecContext(txCtx, `
UPDATE user_affiliate_ledger
SET source_type = 'admin_recharge',
    source_redeem_code_id = $1,
    base_amount = 25
WHERE id = $2`, adminCodeID, adminLedgerID)
	require.NoError(t, err)

	constraintMigrationSQL, err := migrationspkg.FS.ReadFile("232_affiliate_rebate_source_constraints.sql")
	require.NoError(t, err)
	for range 2 {
		_, err = client.ExecContext(txCtx, string(constraintMigrationSQL))
		require.NoError(t, err)
	}

	rows, err = client.QueryContext(txCtx, `
SELECT source_type, source_redeem_code_id, base_amount::double precision
FROM user_affiliate_ledger
WHERE id = $1`, adminLedgerID)
	require.NoError(t, err)
	require.True(t, rows.Next())
	var adminSource string
	var adminSourceID *int64
	var adminBaseAmount *float64
	require.NoError(t, rows.Scan(&adminSource, &adminSourceID, &adminBaseAmount))
	require.NoError(t, rows.Close())
	require.Equal(t, string(service.AffiliateRebateSourceLegacyUnknown), adminSource)
	require.Nil(t, adminSourceID)
	require.Nil(t, adminBaseAmount)

	require.Positive(t, ordinaryCodeID)
	require.Positive(t, adminCodeID)
}

func insertHistoricalRedeemCode(t *testing.T, ctx context.Context, client *dbent.Client, userID int64, codeType string, value float64, usedAt time.Time) int64 {
	t.Helper()
	rows, err := client.QueryContext(ctx, `
INSERT INTO redeem_codes (code, type, value, status, used_by, used_at, created_at)
VALUES ($1, $2, $3, 'used', $4, $5, $5)
RETURNING id`, fmt.Sprintf("HIST%d", time.Now().UnixNano()), codeType, value, userID, usedAt)
	require.NoError(t, err)
	require.True(t, rows.Next())
	var id int64
	require.NoError(t, rows.Scan(&id))
	require.NoError(t, rows.Close())
	return id
}

func insertHistoricalAffiliateLedger(t *testing.T, ctx context.Context, client *dbent.Client, inviterID, inviteeID int64, amount float64, createdAt time.Time) int64 {
	t.Helper()
	rows, err := client.QueryContext(ctx, `
INSERT INTO user_affiliate_ledger (user_id, action, amount, source_user_id, created_at, updated_at)
VALUES ($1, 'accrue', $2, $3, $4, $4)
RETURNING id`, inviterID, amount, inviteeID, createdAt)
	require.NoError(t, err)
	require.True(t, rows.Next())
	var id int64
	require.NoError(t, rows.Scan(&id))
	require.NoError(t, rows.Close())
	return id
}

func TestAffiliateRepository_TransferQuotaToBalance_EmptyQuota(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	txCtx := dbent.NewTxContext(ctx, tx)
	client := tx.Client()

	repo := NewAffiliateRepository(client, integrationDB)

	u := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("affiliate-empty-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
		Balance:      3.21,
		Concurrency:  5,
	})

	affCode := fmt.Sprintf("AFF%09d", time.Now().UnixNano()%1_000_000_000)
	_, err := client.ExecContext(txCtx, `
INSERT INTO user_affiliates (user_id, aff_code, aff_quota, aff_history_quota, created_at, updated_at)
VALUES ($1, $2, 0, 0, NOW(), NOW())`, u.ID, affCode)
	require.NoError(t, err)

	transferred, balance, err := repo.TransferQuotaToBalance(txCtx, u.ID)
	require.ErrorIs(t, err, service.ErrAffiliateQuotaEmpty)
	require.InDelta(t, 0.0, transferred, 1e-9)
	require.InDelta(t, 0.0, balance, 1e-9)

	persistedBalance := querySingleFloat(t, txCtx, client,
		"SELECT balance::double precision FROM users WHERE id = $1", u.ID)
	require.InDelta(t, 3.21, persistedBalance, 1e-9)
}

// TestAffiliateRepository_AdminCustomCode covers the success path of admin
// invite-code rewrite + reset within a shared test transaction:
// - UpdateUserAffCode replaces aff_code, sets aff_code_custom=true, lookup works
// - the old code can no longer be found
// - ResetUserAffCode reverts aff_code_custom and assigns a new system-format code
//
// The conflict path (duplicate code → ErrAffiliateCodeTaken) lives in its own
// test because a unique-violation aborts the surrounding Postgres tx, which
// would poison subsequent assertions in the same transaction.
func TestAffiliateRepository_AdminCustomCode(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	txCtx := dbent.NewTxContext(ctx, tx)
	client := tx.Client()

	repo := NewAffiliateRepository(client, integrationDB)

	u := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("affiliate-custom-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
	})

	original, err := repo.EnsureUserAffiliate(txCtx, u.ID)
	require.NoError(t, err)
	require.False(t, original.AffCodeCustom, "system-generated codes start as non-custom")
	originalCode := original.AffCode

	// Rewrite to a custom code
	customCode := fmt.Sprintf("VIP%09d", time.Now().UnixNano()%1_000_000_000)
	require.NoError(t, repo.UpdateUserAffCode(txCtx, u.ID, customCode))

	updated, err := repo.EnsureUserAffiliate(txCtx, u.ID)
	require.NoError(t, err)
	require.Equal(t, customCode, updated.AffCode)
	require.True(t, updated.AffCodeCustom)

	// Lookup by new custom code finds the user
	byCode, err := repo.GetAffiliateByCode(txCtx, customCode)
	require.NoError(t, err)
	require.Equal(t, u.ID, byCode.UserID)

	// Old system code should no longer match
	_, err = repo.GetAffiliateByCode(txCtx, originalCode)
	require.ErrorIs(t, err, service.ErrAffiliateProfileNotFound)

	// Reset back to a fresh system code, clears custom flag
	newSysCode, err := repo.ResetUserAffCode(txCtx, u.ID)
	require.NoError(t, err)
	require.NotEqual(t, customCode, newSysCode)

	reset, err := repo.EnsureUserAffiliate(txCtx, u.ID)
	require.NoError(t, err)
	require.Equal(t, newSysCode, reset.AffCode)
	require.False(t, reset.AffCodeCustom)

	// The old custom code is now free again
	_, err = repo.GetAffiliateByCode(txCtx, customCode)
	require.ErrorIs(t, err, service.ErrAffiliateProfileNotFound)
}

// TestAffiliateRepository_AdminCustomCode_Conflict isolates the unique-violation
// path. PostgreSQL aborts the enclosing tx when a unique constraint fires, so
// this test must be the only assertion and run in its own tx — production
// callers each have their own outer tx, so this matches real behavior.
func TestAffiliateRepository_AdminCustomCode_Conflict(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	txCtx := dbent.NewTxContext(ctx, tx)
	client := tx.Client()

	repo := NewAffiliateRepository(client, integrationDB)

	taker := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("affiliate-conflict-taker-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Role:         service.RoleUser, Status: service.StatusActive,
	})
	requester := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("affiliate-conflict-req-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Role:         service.RoleUser, Status: service.StatusActive,
	})

	takenCode := fmt.Sprintf("HOT%09d", time.Now().UnixNano()%1_000_000_000)
	require.NoError(t, repo.UpdateUserAffCode(txCtx, taker.ID, takenCode))

	// Now requester tries to grab the same code → conflict.
	err := repo.UpdateUserAffCode(txCtx, requester.ID, takenCode)
	require.ErrorIs(t, err, service.ErrAffiliateCodeTaken)
}

// TestAffiliateRepository_AdminRebateRate covers per-user exclusive rate
// set/clear and the Batch variant including NULL semantics.
func TestAffiliateRepository_AdminRebateRate(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	txCtx := dbent.NewTxContext(ctx, tx)
	client := tx.Client()

	repo := NewAffiliateRepository(client, integrationDB)

	u1 := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("affiliate-rate-%d-a@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
	})
	u2 := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("affiliate-rate-%d-b@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
	})

	// Set exclusive rate for u1
	rate := 42.5
	require.NoError(t, repo.SetUserRebateRate(txCtx, u1.ID, &rate))

	got, err := repo.EnsureUserAffiliate(txCtx, u1.ID)
	require.NoError(t, err)
	require.NotNil(t, got.AffRebateRatePercent)
	require.InDelta(t, 42.5, *got.AffRebateRatePercent, 1e-9)

	// Clear exclusive rate
	require.NoError(t, repo.SetUserRebateRate(txCtx, u1.ID, nil))
	cleared, err := repo.EnsureUserAffiliate(txCtx, u1.ID)
	require.NoError(t, err)
	require.Nil(t, cleared.AffRebateRatePercent)

	// Batch set both users
	batchRate := 15.0
	require.NoError(t, repo.BatchSetUserRebateRate(txCtx, []int64{u1.ID, u2.ID}, &batchRate))

	for _, uid := range []int64{u1.ID, u2.ID} {
		v, err := repo.EnsureUserAffiliate(txCtx, uid)
		require.NoError(t, err)
		require.NotNil(t, v.AffRebateRatePercent)
		require.InDelta(t, 15.0, *v.AffRebateRatePercent, 1e-9)
	}

	// Batch clear
	require.NoError(t, repo.BatchSetUserRebateRate(txCtx, []int64{u1.ID, u2.ID}, nil))
	for _, uid := range []int64{u1.ID, u2.ID} {
		v, err := repo.EnsureUserAffiliate(txCtx, uid)
		require.NoError(t, err)
		require.Nil(t, v.AffRebateRatePercent)
	}
}

// TestAffiliateRepository_ListUsersWithCustomSettings verifies the admin list
// only includes users with at least one override applied.
func TestAffiliateRepository_ListUsersWithCustomSettings(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	txCtx := dbent.NewTxContext(ctx, tx)
	client := tx.Client()

	repo := NewAffiliateRepository(client, integrationDB)

	// User without any custom config — should NOT appear in the list.
	plainEmail := fmt.Sprintf("affiliate-plain-%d@example.com", time.Now().UnixNano())
	uPlain := mustCreateUser(t, client, &service.User{
		Email: plainEmail, PasswordHash: "hash",
		Role: service.RoleUser, Status: service.StatusActive,
	})
	_, err := repo.EnsureUserAffiliate(txCtx, uPlain.ID)
	require.NoError(t, err)

	// User with a custom code — should appear.
	uCode := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("affiliate-codeonly-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Role:         service.RoleUser, Status: service.StatusActive,
	})
	require.NoError(t, repo.UpdateUserAffCode(txCtx, uCode.ID, fmt.Sprintf("VIP%09d", time.Now().UnixNano()%1_000_000_000)))

	// User with only an exclusive rate — should appear.
	uRate := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("affiliate-rateonly-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Role:         service.RoleUser, Status: service.StatusActive,
	})
	r := 33.3
	require.NoError(t, repo.SetUserRebateRate(txCtx, uRate.ID, &r))

	entries, total, err := repo.ListUsersWithCustomSettings(txCtx, service.AffiliateAdminFilter{
		Page: 1, PageSize: 100,
	})
	require.NoError(t, err)

	// Build a quick lookup to assert per-user attributes (other tests may have
	// inserted custom rows in the same DB; we only care about our 3).
	byUserID := make(map[int64]service.AffiliateAdminEntry, len(entries))
	for _, e := range entries {
		byUserID[e.UserID] = e
	}

	require.NotContains(t, byUserID, uPlain.ID, "users without overrides must not appear")

	codeEntry, ok := byUserID[uCode.ID]
	require.True(t, ok, "custom-code user missing from list")
	require.True(t, codeEntry.AffCodeCustom)
	require.Nil(t, codeEntry.AffRebateRatePercent)

	rateEntry, ok := byUserID[uRate.ID]
	require.True(t, ok, "custom-rate user missing from list")
	require.False(t, rateEntry.AffCodeCustom)
	require.NotNil(t, rateEntry.AffRebateRatePercent)
	require.InDelta(t, 33.3, *rateEntry.AffRebateRatePercent, 1e-9)

	require.GreaterOrEqual(t, total, int64(2), "total must include at least our 2 custom rows")
}

func mustCreateAffiliateWithdrawUser(t *testing.T, client *dbent.Client, label string) *service.User {
	t.Helper()
	return mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("affiliate-withdraw-%s-%d@example.com", label, time.Now().UnixNano()),
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
		Balance:      5.5,
		Concurrency:  5,
	})
}

func mustSeedAffiliateQuota(t *testing.T, ctx context.Context, client *dbent.Client, userID int64, quota, frozen, history float64) {
	t.Helper()
	affCode := fmt.Sprintf("AFW%d-%06d", userID, time.Now().UnixNano()%1_000_000)
	_, err := client.ExecContext(ctx, `
INSERT INTO user_affiliates (user_id, aff_code, aff_quota, aff_frozen_quota, aff_history_quota, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, NOW(), NOW())`, userID, affCode, quota, frozen, history)
	require.NoError(t, err)
}

// mustCreateCommittedAffiliateWithdrawUser 在测试事务之外建用户与返利档案，
// 测试结束时删除（级联返利档案与流水）。WithdrawQuota 在这些用例里自行开启
// 并提交事务，与生产调用一致。
func mustCreateCommittedAffiliateWithdrawUser(t *testing.T, label string, quota, frozen, history float64) *service.User {
	t.Helper()
	client := testEntClient(t)
	u := mustCreateAffiliateWithdrawUser(t, client, label)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM users WHERE id = $1", u.ID)
	})
	mustSeedAffiliateQuota(t, context.Background(), client, u.ID, quota, frozen, history)
	return u
}

func newAffiliateWithdrawOperationID(label string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s-%d", label, time.Now().UnixNano())))
	return hex.EncodeToString(sum[:])
}

// TestAffiliateRepository_WithdrawQuota_DeductsAndRecordsLedger 覆盖线下提现主路径：
// 只扣可提取额度，余额与累计返利不变，流水 action=withdraw 带额度快照，并出现在提取记录中。
func TestAffiliateRepository_WithdrawQuota_DeductsAndRecordsLedger(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	txCtx := dbent.NewTxContext(ctx, tx)
	client := tx.Client()
	repo := NewAffiliateRepository(client, integrationDB)

	u := mustCreateAffiliateWithdrawUser(t, client, "user")
	mustSeedAffiliateQuota(t, txCtx, client, u.ID, 20, 0, 30)
	operationID := newAffiliateWithdrawOperationID("deduct")

	result, err := repo.WithdrawQuota(txCtx, u.ID, 12.5, operationID)
	require.NoError(t, err)
	require.Positive(t, result.LedgerID)
	require.False(t, result.Replayed)
	require.InDelta(t, 12.5, result.Amount, 1e-9)
	require.InDelta(t, 7.5, result.AvailableQuotaAfter, 1e-9)
	require.InDelta(t, 0.0, result.FrozenQuotaAfter, 1e-9)
	require.InDelta(t, 30.0, result.HistoryQuotaAfter, 1e-9)

	require.InDelta(t, 7.5, querySingleFloat(t, txCtx, client,
		"SELECT aff_quota::double precision FROM user_affiliates WHERE user_id = $1", u.ID), 1e-9)
	require.InDelta(t, 30.0, querySingleFloat(t, txCtx, client,
		"SELECT aff_history_quota::double precision FROM user_affiliates WHERE user_id = $1", u.ID), 1e-9)
	require.InDelta(t, 5.5, querySingleFloat(t, txCtx, client,
		"SELECT balance::double precision FROM users WHERE id = $1", u.ID), 1e-9)

	records, total, err := repo.ListAffiliateTransferRecords(txCtx, service.AffiliateRecordFilter{
		Search:   u.Email,
		Page:     1,
		PageSize: 20,
		SortDesc: true,
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, records, 1)
	record := records[0]
	require.Equal(t, result.LedgerID, record.LedgerID)
	require.Equal(t, "withdraw", record.Action)
	require.Equal(t, u.ID, record.UserID)
	require.InDelta(t, 12.5, record.Amount, 1e-9)
	require.True(t, record.SnapshotAvailable)
	require.InDelta(t, 5.5, *record.BalanceAfter, 1e-9)
	require.InDelta(t, 7.5, *record.AvailableQuotaAfter, 1e-9)
	require.InDelta(t, 30.0, *record.HistoryQuotaAfter, 1e-9)

	rows, err := client.QueryContext(txCtx, "SELECT operation_id FROM user_affiliate_ledger WHERE id = $1", result.LedgerID)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	require.True(t, rows.Next())
	var storedOperationID string
	require.NoError(t, rows.Scan(&storedOperationID))
	require.Equal(t, operationID, storedOperationID)
}

// TestAffiliateRepository_WithdrawQuota_ThawsMaturedQuotaFirst 验证已过冻结期
// 但尚未解冻的额度在扣减前解冻并可被提现。
func TestAffiliateRepository_WithdrawQuota_ThawsMaturedQuotaFirst(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	txCtx := dbent.NewTxContext(ctx, tx)
	client := tx.Client()
	repo := NewAffiliateRepository(client, integrationDB)

	u := mustCreateAffiliateWithdrawUser(t, client, "thaw")
	mustSeedAffiliateQuota(t, txCtx, client, u.ID, 0, 10, 10)
	_, err := client.ExecContext(txCtx, `
INSERT INTO user_affiliate_ledger (user_id, action, amount, frozen_until, created_at, updated_at)
VALUES ($1, 'accrue', 10, NOW() - INTERVAL '1 hour', NOW(), NOW())`, u.ID)
	require.NoError(t, err)

	result, err := repo.WithdrawQuota(txCtx, u.ID, 10, newAffiliateWithdrawOperationID("thaw"))
	require.NoError(t, err)
	require.InDelta(t, 0.0, result.AvailableQuotaAfter, 1e-9)
	require.InDelta(t, 0.0, result.FrozenQuotaAfter, 1e-9)
	require.Equal(t, 1, querySingleInt(t, txCtx, client,
		"SELECT COUNT(*) FROM user_affiliate_ledger WHERE user_id = $1 AND action = 'withdraw'", u.ID))
}

// TestAffiliateRepository_WithdrawQuota_InsufficientLeavesQuotaUntouched 验证未到期的
// 冻结额度不可提现：额度不足时不扣减、不写流水，占位随事务回滚、不占用该
// operation_id，同一标识改成可提取范围内的金额仍可登记；没有返利档案的用户
// 同样按额度不足处理。
func TestAffiliateRepository_WithdrawQuota_InsufficientLeavesQuotaUntouched(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewAffiliateRepository(client, integrationDB)

	u := mustCreateCommittedAffiliateWithdrawUser(t, "insufficient", 5, 10, 15)
	_, err := integrationDB.ExecContext(ctx, `
INSERT INTO user_affiliate_ledger (user_id, action, amount, frozen_until, created_at, updated_at)
VALUES ($1, 'accrue', 10, NOW() + INTERVAL '1 hour', NOW(), NOW())`, u.ID)
	require.NoError(t, err)
	operationID := newAffiliateWithdrawOperationID("insufficient")

	_, err = repo.WithdrawQuota(ctx, u.ID, 6, operationID)
	require.ErrorIs(t, err, service.ErrAffiliateQuotaInsufficient)

	require.InDelta(t, 5.0, querySingleFloat(t, ctx, client,
		"SELECT aff_quota::double precision FROM user_affiliates WHERE user_id = $1", u.ID), 1e-9)
	require.InDelta(t, 10.0, querySingleFloat(t, ctx, client,
		"SELECT aff_frozen_quota::double precision FROM user_affiliates WHERE user_id = $1", u.ID), 1e-9)
	require.Equal(t, 0, querySingleInt(t, ctx, client,
		"SELECT COUNT(*) FROM user_affiliate_ledger WHERE user_id = $1 AND action = 'withdraw'", u.ID))

	result, err := repo.WithdrawQuota(ctx, u.ID, 5, operationID)
	require.NoError(t, err)
	require.False(t, result.Replayed)
	require.InDelta(t, 0.0, result.AvailableQuotaAfter, 1e-9)

	noProfile := mustCreateAffiliateWithdrawUser(t, client, "no-profile")
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM users WHERE id = $1", noProfile.ID)
	})
	_, err = repo.WithdrawQuota(ctx, noProfile.ID, 1, newAffiliateWithdrawOperationID("no-profile"))
	require.ErrorIs(t, err, service.ErrAffiliateQuotaInsufficient)
	require.Equal(t, 0, querySingleInt(t, ctx, client,
		"SELECT COUNT(*) FROM user_affiliate_ledger WHERE user_id = $1", noProfile.ID))
}

// TestAffiliateRepository_WithdrawQuota_RetryAfterCommitReplaysFirstResult 复现
// 「提交成功但响应丢失后重试」：首次登记已在独立事务中提交，以同一 operation_id
// 重试不重复扣减、不重复写流水，返回首次登记的流水与额度快照。两笔合法的等额打款
// 使用不同的 operation_id，各扣一次。
func TestAffiliateRepository_WithdrawQuota_RetryAfterCommitReplaysFirstResult(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewAffiliateRepository(client, integrationDB)

	u := mustCreateCommittedAffiliateWithdrawUser(t, "retry", 100, 0, 100)
	operationID := newAffiliateWithdrawOperationID("retry")

	first, err := repo.WithdrawQuota(ctx, u.ID, 10, operationID)
	require.NoError(t, err)
	require.False(t, first.Replayed)

	retry, err := repo.WithdrawQuota(ctx, u.ID, 10, operationID)
	require.NoError(t, err)
	require.True(t, retry.Replayed)
	require.Equal(t, first.LedgerID, retry.LedgerID)
	require.Equal(t, u.ID, retry.UserID)
	require.InDelta(t, 10.0, retry.Amount, 1e-9)
	require.InDelta(t, 90.0, retry.AvailableQuotaAfter, 1e-9)
	require.InDelta(t, first.FrozenQuotaAfter, retry.FrozenQuotaAfter, 1e-9)
	require.InDelta(t, 100.0, retry.HistoryQuotaAfter, 1e-9)

	require.InDelta(t, 90.0, querySingleFloat(t, ctx, client,
		"SELECT aff_quota::double precision FROM user_affiliates WHERE user_id = $1", u.ID), 1e-9)
	require.Equal(t, 1, querySingleInt(t, ctx, client,
		"SELECT COUNT(*) FROM user_affiliate_ledger WHERE user_id = $1 AND action = 'withdraw'", u.ID))

	second, err := repo.WithdrawQuota(ctx, u.ID, 10, newAffiliateWithdrawOperationID("retry-second"))
	require.NoError(t, err)
	require.False(t, second.Replayed)
	require.NotEqual(t, first.LedgerID, second.LedgerID)
	require.InDelta(t, 80.0, querySingleFloat(t, ctx, client,
		"SELECT aff_quota::double precision FROM user_affiliates WHERE user_id = $1", u.ID), 1e-9)
	require.Equal(t, 2, querySingleInt(t, ctx, client,
		"SELECT COUNT(*) FROM user_affiliate_ledger WHERE user_id = $1 AND action = 'withdraw'", u.ID))
}

// TestAffiliateRepository_WithdrawQuota_SameOperationWaitsForInFlightRegistration
// 固定「并发同键」的交错：首个登记已占位并扣减但未提交时，同一 operation_id 的
// 请求在唯一索引上等待。首个登记提交后，等待的请求不重复扣减，返回同一条流水；
// 首个登记回滚后，等待的请求自行完成登记。两种结局都只扣一次。
func TestAffiliateRepository_WithdrawQuota_SameOperationWaitsForInFlightRegistration(t *testing.T) {
	for _, commitFirst := range []bool{true, false} {
		name := "first rolls back"
		if commitFirst {
			name = "first commits"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			client := testEntClient(t)
			repo := NewAffiliateRepository(client, integrationDB)

			u := mustCreateCommittedAffiliateWithdrawUser(t, "inflight", 100, 0, 100)
			operationID := newAffiliateWithdrawOperationID("inflight")

			tx, err := client.Tx(ctx)
			require.NoError(t, err)
			t.Cleanup(func() { _ = tx.Rollback() })
			first, err := repo.WithdrawQuota(dbent.NewTxContext(ctx, tx), u.ID, 10, operationID)
			require.NoError(t, err)

			type outcome struct {
				result *service.AffiliateWithdrawResult
				err    error
			}
			waiting := make(chan outcome, 1)
			go func() {
				result, err := repo.WithdrawQuota(ctx, u.ID, 10, operationID)
				waiting <- outcome{result: result, err: err}
			}()

			select {
			case got := <-waiting:
				t.Fatalf("same operation must wait for the in-flight registration, got result=%+v err=%v", got.result, got.err)
			case <-time.After(300 * time.Millisecond):
			}

			if commitFirst {
				require.NoError(t, tx.Commit())
			} else {
				require.NoError(t, tx.Rollback())
			}

			var got outcome
			select {
			case got = <-waiting:
			case <-time.After(10 * time.Second):
				t.Fatal("waiting registration did not finish after the in-flight one ended")
			}
			require.NoError(t, got.err)
			if commitFirst {
				require.True(t, got.result.Replayed)
				require.Equal(t, first.LedgerID, got.result.LedgerID)
			} else {
				require.False(t, got.result.Replayed)
				require.NotEqual(t, first.LedgerID, got.result.LedgerID)
			}
			require.InDelta(t, 90.0, got.result.AvailableQuotaAfter, 1e-9)

			require.InDelta(t, 90.0, querySingleFloat(t, ctx, client,
				"SELECT aff_quota::double precision FROM user_affiliates WHERE user_id = $1", u.ID), 1e-9)
			require.Equal(t, 1, querySingleInt(t, ctx, client,
				"SELECT COUNT(*) FROM user_affiliate_ledger WHERE user_id = $1 AND action = 'withdraw'", u.ID))
		})
	}
}

// TestAffiliateRepository_WithdrawQuota_ConcurrentSameOperationDeductsOnce 让多个请求
// 同时以同一 operation_id 登记：只有一个执行扣减，其余返回同一条流水。
func TestAffiliateRepository_WithdrawQuota_ConcurrentSameOperationDeductsOnce(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewAffiliateRepository(client, integrationDB)

	u := mustCreateCommittedAffiliateWithdrawUser(t, "concurrent", 100, 0, 100)
	operationID := newAffiliateWithdrawOperationID("concurrent")

	const workers = 8
	results := make([]*service.AffiliateWithdrawResult, workers)
	errs := make([]error, workers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i], errs[i] = repo.WithdrawQuota(ctx, u.ID, 10, operationID)
		}(i)
	}
	close(start)
	wg.Wait()

	executed := 0
	for i := 0; i < workers; i++ {
		require.NoError(t, errs[i])
		require.Equal(t, results[0].LedgerID, results[i].LedgerID)
		require.InDelta(t, 90.0, results[i].AvailableQuotaAfter, 1e-9)
		if !results[i].Replayed {
			executed++
		}
	}
	require.Equal(t, 1, executed)
	require.InDelta(t, 90.0, querySingleFloat(t, ctx, client,
		"SELECT aff_quota::double precision FROM user_affiliates WHERE user_id = $1", u.ID), 1e-9)
	require.Equal(t, 1, querySingleInt(t, ctx, client,
		"SELECT COUNT(*) FROM user_affiliate_ledger WHERE user_id = $1 AND action = 'withdraw'", u.ID))
}

// TestAffiliateRepository_WithdrawQuota_RejectsOperationReuseWithDifferentRequest 验证
// 同一 operation_id 换了金额或用户时以 ErrIdempotencyKeyConflict 拒绝，
// 不扣减、不写流水。
func TestAffiliateRepository_WithdrawQuota_RejectsOperationReuseWithDifferentRequest(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewAffiliateRepository(client, integrationDB)

	u := mustCreateCommittedAffiliateWithdrawUser(t, "conflict", 100, 0, 100)
	other := mustCreateCommittedAffiliateWithdrawUser(t, "conflict-other", 100, 0, 100)
	operationID := newAffiliateWithdrawOperationID("conflict")

	_, err := repo.WithdrawQuota(ctx, u.ID, 10, operationID)
	require.NoError(t, err)

	_, err = repo.WithdrawQuota(ctx, u.ID, 11, operationID)
	require.ErrorIs(t, err, service.ErrIdempotencyKeyConflict)
	_, err = repo.WithdrawQuota(ctx, other.ID, 10, operationID)
	require.ErrorIs(t, err, service.ErrIdempotencyKeyConflict)

	require.InDelta(t, 90.0, querySingleFloat(t, ctx, client,
		"SELECT aff_quota::double precision FROM user_affiliates WHERE user_id = $1", u.ID), 1e-9)
	require.InDelta(t, 100.0, querySingleFloat(t, ctx, client,
		"SELECT aff_quota::double precision FROM user_affiliates WHERE user_id = $1", other.ID), 1e-9)
	require.Equal(t, 1, querySingleInt(t, ctx, client,
		"SELECT COUNT(*) FROM user_affiliate_ledger WHERE user_id IN ($1, $2) AND action = 'withdraw'", u.ID, other.ID))
}

// TestAffiliateRepository_ListAffiliateRebateRecords_IncludesNonOrderAccruals 验证兑换码
// 等非订单来源的返利出现在返利记录中，订单字段为空。
func TestAffiliateRepository_ListAffiliateRebateRecords_IncludesNonOrderAccruals(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	txCtx := dbent.NewTxContext(ctx, tx)
	client := tx.Client()
	repo := NewAffiliateRepository(client, integrationDB)

	inviter := mustCreateAffiliateWithdrawUser(t, client, "inviter")
	invitee := mustCreateAffiliateWithdrawUser(t, client, "invitee")
	mustSeedAffiliateQuota(t, txCtx, client, inviter.ID, 0, 0, 0)

	redeemCodeID := insertHistoricalRedeemCode(t, txCtx, client, invitee.ID, "balance", 10, time.Now())
	applied, err := repo.AccrueQuota(txCtx, service.AffiliateAccrualInput{
		InviterID:     inviter.ID,
		InviteeUserID: invitee.ID,
		Amount:        2,
		Source: service.AffiliateRebateSource{
			Type:         service.AffiliateRebateSourceBalanceRedeem,
			BaseAmount:   10,
			RedeemCodeID: &redeemCodeID,
		},
	})
	require.NoError(t, err)
	require.InDelta(t, 2, applied, 1e-9)

	records, total, err := repo.ListAffiliateRebateRecords(txCtx, service.AffiliateRecordFilter{
		Search:   invitee.Email,
		Page:     1,
		PageSize: 20,
		SortDesc: true,
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, records, 1)
	record := records[0]
	require.Nil(t, record.OrderID)
	require.Nil(t, record.OrderAmount)
	require.Nil(t, record.PayAmount)
	require.Empty(t, record.OutTradeNo)
	require.Equal(t, inviter.ID, record.InviterID)
	require.NotNil(t, record.InviteeID)
	require.Equal(t, invitee.ID, *record.InviteeID)
	require.InDelta(t, 2.0, record.RebateAmount, 1e-9)
}
