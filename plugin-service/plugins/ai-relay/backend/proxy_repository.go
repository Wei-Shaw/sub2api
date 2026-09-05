package backend

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/plugin-service/internal/config"

	_ "github.com/lib/pq"
)

var (
	ErrInvalidProxyID          = errors.New("invalid proxy id")
	ErrProxyNotAvailable       = errors.New("proxy is not available")
	ErrProxyStorageUnavailable = errors.New("proxy storage is unavailable")
)

type ProxyConfig struct {
	ID        int64
	Protocol  string
	Host      string
	Port      int
	Username  string
	Password  string
	UpdatedAt time.Time
}

type ProxyResolver interface {
	Resolve(context.Context, int64) (ProxyConfig, error)
}

type SQLProxyResolver struct {
	db *sql.DB
}

func NewSQLProxyResolver(db *sql.DB) *SQLProxyResolver {
	return &SQLProxyResolver{db: db}
}

func NewProxyResolver(database config.DatabaseConfig) (ProxyResolver, error) {
	if !database.Enabled {
		return unavailableProxyResolver{}, nil
	}
	db, err := sql.Open("postgres", database.DSN())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(30 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return NewSQLProxyResolver(db), nil
}

func (r *SQLProxyResolver) Resolve(ctx context.Context, id int64) (ProxyConfig, error) {
	if id < 1 {
		return ProxyConfig{}, ErrInvalidProxyID
	}
	if r == nil || r.db == nil {
		return ProxyConfig{}, ErrProxyStorageUnavailable
	}
	row := r.db.QueryRowContext(ctx, `SELECT id, protocol, host, port,
		COALESCE(username, ''), COALESCE(password, ''), updated_at
		FROM proxies
		WHERE id = $1
		  AND deleted_at IS NULL
		  AND status = 'active'
		  AND (expires_at IS NULL OR expires_at > NOW())`, id)
	var proxy ProxyConfig
	if err := row.Scan(&proxy.ID, &proxy.Protocol, &proxy.Host, &proxy.Port, &proxy.Username, &proxy.Password, &proxy.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ProxyConfig{}, ErrProxyNotAvailable
		}
		return ProxyConfig{}, err
	}
	return proxy, nil
}

type unavailableProxyResolver struct{}

func (unavailableProxyResolver) Resolve(context.Context, int64) (ProxyConfig, error) {
	return ProxyConfig{}, ErrProxyStorageUnavailable
}
