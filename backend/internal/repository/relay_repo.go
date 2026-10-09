package repository

import (
	"context"
	"database/sql"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/account"
	"github.com/Wei-Shaw/sub2api/ent/group"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type relayRepository struct {
	client *dbent.Client
	db     *sql.DB
}

func NewRelayRepository(client *dbent.Client, db *sql.DB) service.RelayRepository {
	return &relayRepository{client: client, db: db}
}

func (r *relayRepository) ListPrivateGroups(ctx context.Context, userID int64) ([]service.Group, error) {
	rows, err := r.client.Group.Query().Where(group.OwnerUserIDEQ(userID)).Order(dbent.Asc(group.FieldID)).All(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]service.Group, 0, len(rows))
	for _, row := range rows {
		result = append(result, *groupEntityToService(row))
	}
	return result, nil
}

func (r *relayRepository) ListOwnedAccountIDs(ctx context.Context, userID int64, page, size int) ([]int64, int64, error) {
	q := r.client.Account.Query().Where(account.OwnerUserIDEQ(userID))
	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, 0, err
	}
	ids, err := q.Order(dbent.Desc(account.FieldID)).Offset((page - 1) * size).Limit(size).IDs(ctx)
	return ids, int64(total), err
}

func (r *relayRepository) Earnings(ctx context.Context, userID int64, page, size int) (*service.RelayEarnings, error) {
	out := &service.RelayEarnings{Items: []service.RelayEarning{}}
	var err error
	out.BalanceChanged, err = r.settlePendingEarnings(ctx, userID)
	if err != nil {
		return nil, err
	}
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(amount) FILTER (WHERE credited_at IS NOT NULL),0),
		COALESCE(SUM(amount) FILTER (WHERE credited_at >= date_trunc('day',NOW() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'),0),
		COALESCE(SUM(amount) FILTER (WHERE credited_at IS NULL),0)
		FROM relay_earnings WHERE supplier_user_id=$1`, userID).Scan(&out.Total, &out.TotalEarned, &out.TodayEarned, &out.PendingEarned); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id,account_id,group_id,request_id,charged_amount,supplier_rate,amount,
		consumer_user_id=supplier_user_id,created_at,credited_at FROM relay_earnings WHERE supplier_user_id=$1
		ORDER BY id DESC LIMIT $2 OFFSET $3`, userID, size, (page-1)*size)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var v service.RelayEarning
		if err = rows.Scan(&v.ID, &v.AccountID, &v.GroupID, &v.RequestID, &v.ChargedAmount, &v.SupplierRate, &v.Amount, &v.SelfUse, &v.CreatedAt, &v.CreditedAt); err != nil {
			return nil, err
		}
		out.Items = append(out.Items, v)
	}
	return out, rows.Err()
}
