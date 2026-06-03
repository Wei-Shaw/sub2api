package repository

import (
	"context"
	"database/sql"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

var (
	errExpandAccountNotFound = infraerrors.NotFound("EXPAND_ACCOUNT_NOT_FOUND", "expand account not found")
	errExpandAccountExists   = infraerrors.Conflict("EXPAND_ACCOUNT_EXISTS", "expand account already exists")
	errExpandAccountUsed     = infraerrors.Conflict("EXPAND_ACCOUNT_ALREADY_USED", "expand account already marked as used")
)

type expandAccountRepository struct {
	db *sql.DB
}

func NewExpandAccountService(db *sql.DB) service.ExpandAccountService {
	return &expandAccountRepository{db: db}
}

func (r *expandAccountRepository) ListExpandAccounts(ctx context.Context, page, pageSize int, filters service.ExpandAccountListFilters) ([]service.ExpandAccount, int64, error) {
	whereClause, args := buildExpandAccountListWhere(filters)

	var total int64
	countQuery := "SELECT COUNT(*) FROM expand_accounts" + whereClause
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	listArgs := append(append([]any{}, args...), pageSize, offset)
	query := `
SELECT id, email, platform, subscription_type, country, session_key, used, created_at, updated_at
FROM expand_accounts` + whereClause + `
ORDER BY created_at DESC, id DESC
LIMIT $` + itoa(len(args)+1) + ` OFFSET $` + itoa(len(args)+2)

	rows, err := r.db.QueryContext(ctx, query, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := make([]service.ExpandAccount, 0, pageSize)
	for rows.Next() {
		var item service.ExpandAccount
		if err := rows.Scan(
			&item.ID,
			&item.Email,
			&item.Platform,
			&item.SubscriptionType,
			&item.Country,
			&item.SessionKey,
			&item.Used,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return items, total, nil
}

func (r *expandAccountRepository) GetExpandAccount(ctx context.Context, id int64) (*service.ExpandAccount, error) {
	const query = `
SELECT id, email, platform, subscription_type, country, session_key, used, created_at, updated_at
FROM expand_accounts
WHERE id = $1`

	var item service.ExpandAccount
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&item.ID,
		&item.Email,
		&item.Platform,
		&item.SubscriptionType,
		&item.Country,
		&item.SessionKey,
		&item.Used,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errExpandAccountNotFound.WithCause(err)
		}
		return nil, err
	}
	return &item, nil
}

func (r *expandAccountRepository) CreateExpandAccount(ctx context.Context, input *service.ExpandAccountCreateInput) (*service.ExpandAccount, error) {
	const query = `
INSERT INTO expand_accounts (email, platform, subscription_type, country, session_key, used)
VALUES ($1, $2, $3, $4, $5, COALESCE($6, false))
RETURNING id, email, platform, subscription_type, country, session_key, used, created_at, updated_at`

	var item service.ExpandAccount
	err := r.db.QueryRowContext(
		ctx,
		query,
		input.Email,
		input.Platform,
		input.SubscriptionType,
		input.Country,
		input.SessionKey,
		input.Used,
	).Scan(
		&item.ID,
		&item.Email,
		&item.Platform,
		&item.SubscriptionType,
		&item.Country,
		&item.SessionKey,
		&item.Used,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return nil, translatePersistenceError(err, nil, errExpandAccountExists)
	}
	return &item, nil
}

func (r *expandAccountRepository) UpdateExpandAccount(ctx context.Context, id int64, input *service.ExpandAccountUpdateInput) (*service.ExpandAccount, error) {
	const query = `
UPDATE expand_accounts
SET
	email = $2,
	platform = $3,
	subscription_type = $4,
	country = $5,
	session_key = $6,
	used = COALESCE($7, used),
	updated_at = NOW()
WHERE id = $1
RETURNING id, email, platform, subscription_type, country, session_key, used, created_at, updated_at`

	var item service.ExpandAccount
	err := r.db.QueryRowContext(
		ctx,
		query,
		id,
		input.Email,
		input.Platform,
		input.SubscriptionType,
		input.Country,
		input.SessionKey,
		input.Used,
	).Scan(
		&item.ID,
		&item.Email,
		&item.Platform,
		&item.SubscriptionType,
		&item.Country,
		&item.SessionKey,
		&item.Used,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errExpandAccountNotFound.WithCause(err)
		}
		return nil, translatePersistenceError(err, nil, errExpandAccountExists)
	}
	return &item, nil
}

func (r *expandAccountRepository) DeleteExpandAccount(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM expand_accounts WHERE id = $1", id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return errExpandAccountNotFound
	}
	return nil
}

func (r *expandAccountRepository) MarkExpandAccountUsed(ctx context.Context, id int64) (*service.ExpandAccount, error) {
	const query = `
UPDATE expand_accounts
SET used = true, updated_at = NOW()
WHERE id = $1 AND used = false
RETURNING id, email, platform, subscription_type, country, session_key, used, created_at, updated_at`

	var item service.ExpandAccount
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&item.ID,
		&item.Email,
		&item.Platform,
		&item.SubscriptionType,
		&item.Country,
		&item.SessionKey,
		&item.Used,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err == nil {
		return &item, nil
	}
	if err != sql.ErrNoRows {
		return nil, err
	}

	existing, getErr := r.GetExpandAccount(ctx, id)
	if getErr != nil {
		return nil, getErr
	}
	if existing.Used {
		return nil, errExpandAccountUsed
	}
	return nil, errExpandAccountNotFound
}

func buildExpandAccountListWhere(filters service.ExpandAccountListFilters) (string, []any) {
	clauses := make([]string, 0, 2)
	args := make([]any, 0, 4)

	search := strings.TrimSpace(filters.Search)
	if search != "" {
		search = "%" + search + "%"
		args = append(args, search, search, search)
		clauses = append(clauses,
			"(email ILIKE $"+itoa(len(args)-2)+" OR platform ILIKE $"+itoa(len(args)-1)+" OR country ILIKE $"+itoa(len(args))+")",
		)
	}

	switch strings.TrimSpace(filters.Used) {
	case "unused":
		args = append(args, false)
		clauses = append(clauses, "used = $"+itoa(len(args)))
	case "used":
		args = append(args, true)
		clauses = append(clauses, "used = $"+itoa(len(args)))
	}

	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}
