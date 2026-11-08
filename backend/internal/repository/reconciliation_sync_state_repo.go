package repository

import (
	"context"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/reconciliationsyncstate"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// reconciliationSyncStateRepository 是同步状态键值表（增量拉取游标等）的读写实现。
type reconciliationSyncStateRepository struct {
	client *dbent.Client
}

// NewReconciliationSyncStateRepository 创建同步状态仓库。
func NewReconciliationSyncStateRepository(client *dbent.Client) service.ReconciliationSyncStateRepository {
	return &reconciliationSyncStateRepository{client: client}
}

// Get 读取同步状态的值。
//
// 语义（关键）：键不存在时返回 ("", nil)，不返回 NotFound 错误。
// 调用方把「从未同步过」当作正常的初始状态（游标为空 → 从头拉），
// 用错误表达会逼着每次首跑都走一遍错误分支判断。
func (r *reconciliationSyncStateRepository) Get(ctx context.Context, key string) (string, error) {
	client := clientFromContext(ctx, r.client)
	row, err := client.ReconciliationSyncState.Query().
		Where(reconciliationsyncstate.KeyEQ(key)).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return "", nil
		}
		return "", translatePersistenceError(err, nil, nil)
	}
	return row.Value, nil
}

// Set 按 key upsert 同步状态，并刷新 updated_at。
//
// 幂等语义：key 上有唯一索引，重复写同一个 key 只覆盖 value 与 updated_at，
// 不会新增行；并发写同一 key 也只会留下最后提交的那份值。
func (r *reconciliationSyncStateRepository) Set(ctx context.Context, key, value string) error {
	client := clientFromContext(ctx, r.client)
	now := time.Now()
	err := client.ReconciliationSyncState.Create().
		SetKey(key).
		SetValue(value).
		SetUpdatedAt(now).
		OnConflictColumns(reconciliationsyncstate.FieldKey).
		UpdateNewValues().
		Exec(ctx)
	return translatePersistenceError(err, nil, nil)
}

// GetMultiple 批量读取同步状态。
//
// 纯读语义：一次查询取回所有命中的 key；未写入过的 key 不会出现在返回值里
// （与 setting 仓库的 GetMultiple 保持一致，调用方按 map 零值 "" 处理即可）。
func (r *reconciliationSyncStateRepository) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	if len(keys) == 0 {
		return map[string]string{}, nil
	}

	client := clientFromContext(ctx, r.client)
	rows, err := client.ReconciliationSyncState.Query().
		Where(reconciliationsyncstate.KeyIn(keys...)).
		All(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, nil, nil)
	}

	result := make(map[string]string, len(rows))
	for _, row := range rows {
		result[row.Key] = row.Value
	}
	return result, nil
}
