package repository

import (
	"context"
	"database/sql"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

const upstreamZstdAccountsRefreshInterval = 15 * time.Second

// Migration 045 provides the GIN index on extra. Only opted-in API key accounts are loaded.
const upstreamZstdAccountsQuery = `SELECT id FROM accounts WHERE deleted_at IS NULL AND type = 'apikey' AND extra @> '{"` + service.AccountExtraUpstreamRequestZstd + `":true}'::jsonb`

// The shared transport receives account IDs, not account objects. Keep the indexed
// lookup off the request path and publish a snapshot only after a complete read.
type upstreamZstdAccounts struct {
	db       *sql.DB
	ids      atomic.Pointer[map[int64]struct{}]
	loadedAt atomic.Int64
	loading  atomic.Bool
}

func ProvideHTTPUpstream(cfg *config.Config, db *sql.DB) service.HTTPUpstream {
	s := newHTTPUpstream(cfg)
	if db != nil {
		s.zstdAccounts = &upstreamZstdAccounts{db: db}
		s.zstdAccounts.refresh(context.Background())
	}
	return s
}

func (r *upstreamZstdAccounts) enabled(accountID int64) bool {
	if r == nil || accountID <= 0 {
		return false
	}
	if time.Since(time.Unix(0, r.loadedAt.Load())) >= upstreamZstdAccountsRefreshInterval && r.loading.CompareAndSwap(false, true) {
		go func() {
			defer r.loading.Store(false)
			r.refresh(context.Background())
		}()
	}
	ids := r.ids.Load()
	if ids == nil {
		return false
	}
	_, enabled := (*ids)[accountID]
	return enabled
}

func (r *upstreamZstdAccounts) refresh(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	defer func() { r.loadedAt.Store(time.Now().UnixNano()) }()
	ids, err := r.loadIDs(ctx)
	if err != nil {
		slog.Warn("failed to refresh upstream request compression settings", "error", err)
		return
	}
	r.ids.Store(&ids)
}

func (r *upstreamZstdAccounts) loadIDs(ctx context.Context) (map[int64]struct{}, error) {
	rows, err := r.db.QueryContext(ctx, upstreamZstdAccountsQuery)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	ids := make(map[int64]struct{})
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids[id] = struct{}{}
	}
	return ids, rows.Err()
}
