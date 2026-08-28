package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type ipBanRepository struct {
	db *sql.DB
}

func NewIPBanRepository(db *sql.DB) service.IPBanRepository {
	return &ipBanRepository{db: db}
}

func (r *ipBanRepository) Create(ctx context.Context, ipAddress string) (*service.IPBan, error) {
	ban := &service.IPBan{}
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO ip_bans (ip_address)
		VALUES ($1)
		RETURNING id, ip_address, created_at`, ipAddress).
		Scan(&ban.ID, &ban.IPAddress, &ban.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, service.ErrIPBanExists
		}
		return nil, fmt.Errorf("create IP ban: %w", err)
	}
	return ban, nil
}

func (r *ipBanRepository) List(ctx context.Context, params pagination.PaginationParams) ([]service.IPBan, *pagination.PaginationResult, error) {
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM ip_bans`).Scan(&total); err != nil {
		return nil, nil, fmt.Errorf("count IP bans: %w", err)
	}

	limit := params.Limit()
	offset := params.Offset()
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, ip_address, created_at
		FROM ip_bans
		ORDER BY created_at DESC, id DESC
		LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, nil, fmt.Errorf("list IP bans: %w", err)
	}
	defer rows.Close()

	bans := make([]service.IPBan, 0)
	for rows.Next() {
		var ban service.IPBan
		if err := rows.Scan(&ban.ID, &ban.IPAddress, &ban.CreatedAt); err != nil {
			return nil, nil, fmt.Errorf("scan IP ban: %w", err)
		}
		bans = append(bans, ban)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate IP bans: %w", err)
	}
	return bans, paginationResultFromTotal(total, params), nil
}

func (r *ipBanRepository) Delete(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM ip_bans WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete IP ban: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check deleted IP ban: %w", err)
	}
	if affected == 0 {
		return service.ErrIPBanNotFound
	}
	return nil
}

func (r *ipBanRepository) IsBanned(ctx context.Context, ipAddress string) (bool, error) {
	var banned bool
	if err := r.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM ip_bans WHERE ip_address = $1)`, ipAddress).Scan(&banned); err != nil {
		return false, fmt.Errorf("check IP ban: %w", err)
	}
	return banned, nil
}
