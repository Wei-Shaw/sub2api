package repository

import (
	"context"
	"encoding/json"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func readLockedManagedAuthorization(ctx context.Context, client *dbent.Client, id int64, key string) (any, bool, error) {
	rows, err := client.QueryContext(ctx, `SELECT extra -> $2::text FROM accounts WHERE id = $1`, id, key)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return nil, false, rows.Err()
	}
	var raw []byte
	if err := rows.Scan(&raw); err != nil {
		return nil, false, err
	}
	return decodeAccountExtraJSON(raw)
}

// CompareAndSwapOpenDesignUsage 避免并发刷新覆盖换绑、解绑或账号身份更新。
func (r *accountRepository) CompareAndSwapOpenDesignUsage(ctx context.Context, account *service.Account, next json.RawMessage) (bool, error) {
	if !service.IsOpenDesignUsageAccount(account) {
		return false, nil
	}
	return r.compareAndSwapManagedAuthorization(ctx, account, service.OpenDesignUsageExtraKey, next)
}

// CompareAndSwapZhipuResetCards 仅允许在授权目标和原绑定均未改变时写入。
func (r *accountRepository) CompareAndSwapZhipuResetCards(ctx context.Context, account *service.Account, next json.RawMessage) (bool, error) {
	if !service.IsZhipuResetCardsAccount(account) {
		return false, nil
	}
	return r.compareAndSwapManagedAuthorization(ctx, account, service.ZhipuResetCardsExtraKey, next)
}

func (r *accountRepository) compareAndSwapManagedAuthorization(ctx context.Context, account *service.Account, key string, next json.RawMessage) (bool, error) {
	credentials, err := json.Marshal(normalizeJSONMap(account.Credentials))
	if err != nil {
		return false, err
	}
	var previous, nextValue any
	if value := account.Extra[key]; value != nil {
		raw, err := json.Marshal(value)
		if err != nil {
			return false, err
		}
		previous = string(raw)
	}
	if len(next) != 0 {
		nextValue = string(next)
	}
	query := `
		UPDATE accounts
		SET extra = CASE WHEN $6::jsonb IS NULL THEN COALESCE(extra, '{}'::jsonb) - $2::text
			ELSE jsonb_set(COALESCE(extra, '{}'::jsonb), ARRAY[$2::text], $6::jsonb, true) END,
			updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL AND platform = $7 AND type = 'apikey'
			AND credentials = $3::jsonb AND proxy_id IS NOT DISTINCT FROM $4
			AND parent_account_id IS NULL AND (extra -> $2::text) IS NOT DISTINCT FROM $5::jsonb
	`
	result, err := clientFromContext(ctx, r.client).ExecContext(ctx, query, account.ID, key, string(credentials), account.ProxyID, previous, nextValue, account.Platform)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected == 1, err
}
