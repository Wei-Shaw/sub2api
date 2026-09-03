package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

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

// SummarizeContributor 汇总 owner 作为贡献者的收益。
func (r *shareRevenueRepository) SummarizeContributor(ctx context.Context, ownerUserID int64) (*service.ShareRevenueSummary, error) {
	if r == nil || r.client == nil || ownerUserID <= 0 {
		return &service.ShareRevenueSummary{}, nil
	}
	client := clientFromContext(ctx, r.client)
	rows, err := client.QueryContext(ctx, `
		SELECT COALESCE(SUM(user_amount), 0)::float8, COUNT(*)::bigint
		FROM share_revenue_ledgers
		WHERE owner_user_id = $1 AND revenue_mode = 'share_split' AND user_amount > 0
	`, ownerUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := &service.ShareRevenueSummary{}
	if rows.Next() {
		if err := rows.Scan(&out.TotalEarned, &out.TotalRecords); err != nil {
			return nil, err
		}
	}
	return out, rows.Err()
}

// ListContributorLedgers 分页列出贡献者流水。
func (r *shareRevenueRepository) ListContributorLedgers(ctx context.Context, ownerUserID int64, page, pageSize int) ([]service.ShareRevenueLedgerItem, int64, error) {
	if r == nil || r.client == nil || ownerUserID <= 0 {
		return []service.ShareRevenueLedgerItem{}, 0, nil
	}
	client := clientFromContext(ctx, r.client)
	var total int64
	countRows, err := client.QueryContext(ctx, `
		SELECT COUNT(*)::bigint FROM share_revenue_ledgers
		WHERE owner_user_id = $1 AND revenue_mode = 'share_split' AND user_amount > 0
	`, ownerUserID)
	if err != nil {
		return nil, 0, err
	}
	if countRows.Next() {
		_ = countRows.Scan(&total)
	}
	_ = countRows.Close()

	offset := (page - 1) * pageSize
	rows, err := client.QueryContext(ctx, `
		SELECT id, request_id, COALESCE(account_id, 0), COALESCE(group_id, 0), revenue_mode,
		       total_cost::float8, user_amount::float8, created_at
		FROM share_revenue_ledgers
		WHERE owner_user_id = $1 AND revenue_mode = 'share_split' AND user_amount > 0
		ORDER BY created_at DESC, id DESC
		LIMIT $2 OFFSET $3
	`, ownerUserID, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := make([]service.ShareRevenueLedgerItem, 0, pageSize)
	for rows.Next() {
		var item service.ShareRevenueLedgerItem
		var createdAt interface{}
		if err := rows.Scan(
			&item.ID, &item.RequestID, &item.AccountID, &item.GroupID, &item.RevenueMode,
			&item.TotalCost, &item.UserAmount, &createdAt,
		); err != nil {
			return nil, 0, err
		}
		switch v := createdAt.(type) {
		case time.Time:
			item.CreatedAt = v.UTC().Format(time.RFC3339)
		default:
			item.CreatedAt = fmt.Sprint(v)
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

// 编译期接口断言
var (
	_ service.AffiliateInviterLookup  = (*shareRevenueRepository)(nil)
	_ service.ShareRevenueLedgerWriter = (*shareRevenueRepository)(nil)
	_ service.ShareRevenueQuery        = (*shareRevenueRepository)(nil)
)
