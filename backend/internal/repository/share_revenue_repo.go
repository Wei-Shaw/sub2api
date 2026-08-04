package repository

import (
	"context"
	"database/sql"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// shareRevenueRepository 邀请人查询 + 分账流水。
type shareRevenueRepository struct {
	client *dbent.Client
}

// NewShareRevenueRepository 创建仓库。
func NewShareRevenueRepository(client *dbent.Client) *shareRevenueRepository {
	return &shareRevenueRepository{client: client}
}

// GetAffiliateInviterUserID 查询用户的邀请人 ID。
func (r *shareRevenueRepository) GetAffiliateInviterUserID(ctx context.Context, userID int64) (*int64, error) {
	if r == nil || r.client == nil || userID <= 0 {
		return nil, nil
	}
	client := clientFromContext(ctx, r.client)
	rows, err := client.QueryContext(ctx,
		`SELECT inviter_id FROM user_affiliates WHERE user_id = $1 AND inviter_id IS NOT NULL LIMIT 1`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, rows.Err()
	}
	var inviter sql.NullInt64
	if err := rows.Scan(&inviter); err != nil {
		return nil, err
	}
	if !inviter.Valid || inviter.Int64 <= 0 {
		return nil, nil
	}
	id := inviter.Int64
	return &id, nil
}

// InsertShareRevenueLedger 写入分账流水（request_id 幂等）。
func (r *shareRevenueRepository) InsertShareRevenueLedger(ctx context.Context, row *service.ShareRevenueLedgerRow) error {
	if r == nil || r.client == nil || row == nil {
		return nil
	}
	client := clientFromContext(ctx, r.client)
	const q = `
		INSERT INTO share_revenue_ledgers (
			request_id, usage_user_id, account_id, group_id, revenue_mode,
			total_cost, billed_amount, invite_amount, user_amount, platform_amount,
			owner_user_id, inviter_user_id, created_at
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9, $10,
			$11, $12, NOW()
		)
		ON CONFLICT (request_id) DO NOTHING
	`
	var owner any
	if row.OwnerUserID != nil {
		owner = *row.OwnerUserID
	}
	var inviter any
	if row.InviterUserID != nil {
		inviter = *row.InviterUserID
	}
	var groupID any
	if row.GroupID > 0 {
		groupID = row.GroupID
	}
	var accountID any
	if row.AccountID > 0 {
		accountID = row.AccountID
	}
	_, err := client.ExecContext(ctx, q,
		row.RequestID,
		row.UsageUserID,
		accountID,
		groupID,
		row.RevenueMode,
		row.TotalCost,
		row.BilledAmount,
		row.InviteAmount,
		row.UserAmount,
		row.PlatformAmount,
		owner,
		inviter,
	)
	return err
}

// 编译期接口断言
var (
	_ service.AffiliateInviterLookup  = (*shareRevenueRepository)(nil)
	_ service.ShareRevenueLedgerWriter = (*shareRevenueRepository)(nil)
)
