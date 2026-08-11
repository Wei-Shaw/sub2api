package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type web3IdentityRepository struct {
	db *sql.DB
}

func NewWeb3IdentityRepository(db *sql.DB) service.Web3IdentityRepository {
	return &web3IdentityRepository{db: db}
}

func (r *web3IdentityRepository) GetUserIDByAddress(ctx context.Context, address string) (int64, error) {
	var userID int64
	err := r.db.QueryRowContext(ctx, `
		SELECT wi.user_id
		FROM web3_identities wi
		JOIN users u ON u.id = wi.user_id
		WHERE wi.address = $1
		  AND wi.deleted_at IS NULL
		  AND u.deleted_at IS NULL
	`, address).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, service.ErrWeb3IdentityNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("get web3 identity: %w", err)
	}
	return userID, nil
}

func (r *web3IdentityRepository) GetAddressByUserID(ctx context.Context, userID int64) (string, bool, error) {
	var address string
	err := r.db.QueryRowContext(ctx, `
		SELECT address
		FROM web3_identities
		WHERE user_id = $1 AND deleted_at IS NULL
	`, userID).Scan(&address)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("get web3 address by user ID: %w", err)
	}
	return address, true, nil
}

func (r *web3IdentityRepository) ExistsByAddress(ctx context.Context, address string) (bool, error) {
	var exists bool
	if err := r.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM web3_identities
			WHERE address = $1 AND deleted_at IS NULL
		)
	`, address).Scan(&exists); err != nil {
		return false, fmt.Errorf("check web3 identity: %w", err)
	}
	return exists, nil
}

func (r *web3IdentityRepository) CreateUserWithIdentity(ctx context.Context, input service.Web3UserCreateInput) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin web3 registration transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var userID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO users (
			email, password_hash, username, role, status,
			balance, concurrency, rpm_limit, signup_source
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'email')
		RETURNING id
	`,
		input.Email,
		input.PasswordHash,
		input.Username,
		input.Role,
		input.Status,
		input.Balance,
		input.Concurrency,
		input.RPMLimit,
	).Scan(&userID)
	if err != nil {
		return 0, translateWeb3PersistenceError("create web3 user", err)
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO web3_identities (user_id, address)
		VALUES ($1, $2)
	`, userID, input.Address); err != nil {
		return 0, translateWeb3PersistenceError("create web3 identity", err)
	}
	if err = tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit web3 registration transaction: %w", err)
	}
	return userID, nil
}

func translateWeb3PersistenceError(operation string, err error) error {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) && pqErr.Code == "23505" {
		return service.ErrWeb3IdentityExists
	}
	return fmt.Errorf("%s: %w", operation, err)
}
