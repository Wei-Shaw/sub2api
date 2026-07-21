package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *accountRepository) GetUpstreamSiteLogoCache(ctx context.Context, key string) (*service.UpstreamSiteLogo, bool, error) {
	if r.sql == nil {
		return nil, false, errors.New("account repository SQL executor not configured")
	}
	rows, err := r.sql.QueryContext(ctx, `
		SELECT content_type, content
		FROM upstream_site_logos
		WHERE cache_key = $1
	`, key)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return nil, false, rows.Err()
	}
	var contentType sql.NullString
	var data []byte
	if err := rows.Scan(&contentType, &data); err != nil {
		return nil, false, err
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	if !contentType.Valid || len(data) == 0 {
		return nil, true, nil
	}
	return &service.UpstreamSiteLogo{ContentType: contentType.String, Data: data}, true, nil
}

func (r *accountRepository) PutUpstreamSiteLogoCache(ctx context.Context, key string, logo *service.UpstreamSiteLogo) error {
	if r.sql == nil {
		return errors.New("account repository SQL executor not configured")
	}
	var contentType any
	var data any
	if logo != nil {
		contentType = logo.ContentType
		data = logo.Data
	}
	_, err := r.sql.ExecContext(ctx, `
		INSERT INTO upstream_site_logos (cache_key, content_type, content)
		VALUES ($1, $2, $3)
		ON CONFLICT (cache_key) DO NOTHING
	`, key, contentType, data)
	return err
}
