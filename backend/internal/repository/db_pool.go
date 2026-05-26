package repository

import (
	"database/sql"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

type dbPoolSettings struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

func buildDBPoolSettings(cfg *config.Config) dbPoolSettings {
	return dbPoolSettings{
		MaxOpenConns:    cfg.Database.MaxOpenConns,
		MaxIdleConns:    cfg.Database.MaxIdleConns,
		ConnMaxLifetime: time.Duration(cfg.Database.ConnMaxLifetimeMinutes) * time.Minute,
		ConnMaxIdleTime: time.Duration(cfg.Database.ConnMaxIdleTimeMinutes) * time.Minute,
	}
}

func applyDBPoolSettings(db *sql.DB, cfg *config.Config) {
	settings := buildDBPoolSettings(cfg)
	db.SetMaxOpenConns(settings.MaxOpenConns)
	db.SetMaxIdleConns(settings.MaxIdleConns)
	db.SetConnMaxLifetime(settings.ConnMaxLifetime)
	db.SetConnMaxIdleTime(settings.ConnMaxIdleTime)
}
