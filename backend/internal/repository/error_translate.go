package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/lib/pq"
)

// clientFromContext 从 context 中获取事务 client，如果不存在则返回默认 client。
//
// 这个辅助函数支持 repository 方法在事务上下文中工作：
// - 如果 context 中存在事务（通过 ent.NewTxContext 设置），返回事务的 client
// - 否则返回传入的默认 client
//
// 使用示例：
//
//	func (r *someRepo) SomeMethod(ctx context.Context) error {
//	    client := clientFromContext(ctx, r.client)
//	    return client.SomeEntity.Create().Save(ctx)
//	}
func clientFromContext(ctx context.Context, defaultClient *dbent.Client) *dbent.Client {
	if tx := dbent.TxFromContext(ctx); tx != nil {
		return tx.Client()
	}
	return defaultClient
}

// translatePersistenceError 将数据库层错误翻译为业务层错误。
//
// 这是 Repository 层的核心错误处理函数，确保数据库细节不会泄露到业务层。
// 通过统一的错误翻译，业务层可以使用语义明确的错误类型（如 ErrUserNotFound）
// 而不是依赖于特定数据库的错误（如 sql.ErrNoRows）。
//
// 参数：
//   - err: 原始数据库错误
//   - notFound: 当记录不存在时返回的业务错误（可为 nil 表示不处理）
//   - conflict: 当违反唯一约束时返回的业务错误（可为 nil 表示不处理）
//
// 返回：
//   - 翻译后的业务错误，或原始错误（如果不匹配任何规则）
//
// 示例：
//
//	err := translatePersistenceError(dbErr, service.ErrUserNotFound, service.ErrEmailExists)
func translatePersistenceError(err error, notFound, conflict *infraerrors.ApplicationError) error {
	if err == nil {
		return nil
	}

	// 兼容 Ent ORM 和标准 database/sql 的 NotFound 行为。
	// Ent 使用自定义的 NotFoundError，而标准库使用 sql.ErrNoRows。
	// 这里同时处理两种情况，保持业务错误映射一致。
	if notFound != nil && (errors.Is(err, sql.ErrNoRows) || dbent.IsNotFound(err)) {
		return notFound.WithCause(err)
	}

	// 处理唯一约束冲突（如邮箱已存在、名称重复等）
	if conflict != nil && isUniqueConstraintViolation(err) {
		return conflict.WithCause(err)
	}

	// 未匹配任何规则，返回原始错误
	return err
}

// isUniqueConstraintViolation 判断错误是否为唯一约束冲突。
//
// 支持多种检测方式：
//  1. PostgreSQL 特定错误码 23505（唯一约束冲突）
//  2. 错误消息中包含的通用关键词
//
// 这种多层次的检测确保了对不同数据库驱动和 ORM 的兼容性。
func isUniqueConstraintViolation(err error) bool {
	if err == nil {
		return false
	}

	// 优先检测 PostgreSQL 特定错误码（最精确）。
	// 错误码 23505 对应 unique_violation。
	// 参考：https://www.postgresql.org/docs/current/errcodes-appendix.html
	var pgErr *pq.Error
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}

	// 回退到错误消息检测（兼容其他场景）。
	// 这些关键词覆盖了 PostgreSQL、MySQL 等主流数据库的错误消息。
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate key") ||
		strings.Contains(msg, "unique constraint") ||
		strings.Contains(msg, "duplicate entry")
}
