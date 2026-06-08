package repository

import (
	"context"
	"database/sql"
	"errors"
)

type sqlQueryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// scanSingleRow 执行查询并扫描第一行到 dest。
// 若无结果，可通过 errors.Is(err, sql.ErrNoRows) 判断。
// 如果 Close 失败，会与原始错误合并返回。
// 设计目的：仅依赖 QueryContext，避免 QueryRowContext 对 *sql.Tx 的强绑定，
// 让 ent.Tx 也能作为 sqlExecutor/Queryer 使用。
func scanSingleRow(ctx context.Context, q sqlQueryer, query string, args []any, dest ...any) (err error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()

	if !rows.Next() {
		if err = rows.Err(); err != nil {
			return err
		}
		return sql.ErrNoRows
	}
	if err = rows.Scan(dest...); err != nil {
		return err
	}
	if err = rows.Err(); err != nil {
		return err
	}
	return nil
}
