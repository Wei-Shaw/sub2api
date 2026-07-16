// Package repository contains persistence infrastructure helpers.
//
// DB pool lifetimes are clamped here because lib/pq starts watchCancel
// goroutines for context-aware queries. If a cloud proxy silently drops idle
// TCP without RST/FIN, those goroutines can block in Read until database/sql
// retires the connection. This is a short-term mitigation; the long-term
// follow-up is migrating PostgreSQL access to jackc/pgx/v5/stdlib.
package repository

import (
	"database/sql"
	"log/slog"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

const (
	defaultConnMaxLifetime = 30 * time.Minute
	defaultConnMaxIdleTime = 5 * time.Minute
	maxConfiguredConnAge   = 24 * time.Hour
)

type dbPoolSettings struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

func clampDBPoolSettings(cfg *config.Config) dbPoolSettings {
	return dbPoolSettings{
		MaxOpenConns:    cfg.Database.MaxOpenConns,
		MaxIdleConns:    cfg.Database.MaxIdleConns,
		ConnMaxLifetime: clampDBPoolDuration("database.conn_max_lifetime_minutes", cfg.Database.ConnMaxLifetimeMinutes, defaultConnMaxLifetime),
		ConnMaxIdleTime: clampDBPoolDuration("database.conn_max_idle_time_minutes", cfg.Database.ConnMaxIdleTimeMinutes, defaultConnMaxIdleTime),
	}
}

func clampDBPoolDuration(key string, minutes int, fallback time.Duration) time.Duration {
	if minutes <= 0 || minutes > int(maxConfiguredConnAge/time.Minute) {
		slog.Warn("database connection pool duration clamped",
			"key", key,
			"before", minutes,
			"after", int(fallback/time.Minute),
		)
		return fallback
	}

	return time.Duration(minutes) * time.Minute
}

func applyDBPoolSettings(db *sql.DB, cfg *config.Config) {
	settings := clampDBPoolSettings(cfg)
	db.SetMaxOpenConns(settings.MaxOpenConns)
	db.SetMaxIdleConns(settings.MaxIdleConns)
	db.SetConnMaxLifetime(settings.ConnMaxLifetime)
	db.SetConnMaxIdleTime(settings.ConnMaxIdleTime)

	slog.Info("database connection pool configured",
		slog.Group("effective",
			slog.Int("max_open", settings.MaxOpenConns),
			slog.Int("max_idle", settings.MaxIdleConns),
			slog.Duration("max_lifetime", settings.ConnMaxLifetime),
			slog.Duration("max_idle_time", settings.ConnMaxIdleTime),
		),
	)
}
