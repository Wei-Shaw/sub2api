package repository

import (
	"database/sql"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"

	_ "github.com/lib/pq"
)

func TestClampDBPoolSettings(t *testing.T) {
	tests := []struct {
		name                string
		connMaxLifetime     int
		connMaxIdleTime     int
		wantMaxLifetime     time.Duration
		wantConnMaxIdleTime time.Duration
	}{
		{
			name:                "zero values fall back to safe defaults",
			connMaxLifetime:     0,
			connMaxIdleTime:     0,
			wantMaxLifetime:     30 * time.Minute,
			wantConnMaxIdleTime: 5 * time.Minute,
		},
		{
			name:                "negative values fall back to safe defaults",
			connMaxLifetime:     -1,
			connMaxIdleTime:     -5,
			wantMaxLifetime:     30 * time.Minute,
			wantConnMaxIdleTime: 5 * time.Minute,
		},
		{
			name:                "reasonable values pass through",
			connMaxLifetime:     15,
			connMaxIdleTime:     3,
			wantMaxLifetime:     15 * time.Minute,
			wantConnMaxIdleTime: 3 * time.Minute,
		},
		{
			name:                "values over twenty four hours fall back to safe defaults",
			connMaxLifetime:     24*60 + 1,
			connMaxIdleTime:     24*60 + 1,
			wantMaxLifetime:     30 * time.Minute,
			wantConnMaxIdleTime: 5 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Database: config.DatabaseConfig{
					MaxOpenConns:           50,
					MaxIdleConns:           10,
					ConnMaxLifetimeMinutes: tt.connMaxLifetime,
					ConnMaxIdleTimeMinutes: tt.connMaxIdleTime,
				},
			}

			settings := clampDBPoolSettings(cfg)
			require.Equal(t, 50, settings.MaxOpenConns)
			require.Equal(t, 10, settings.MaxIdleConns)
			require.Equal(t, tt.wantMaxLifetime, settings.ConnMaxLifetime)
			require.Equal(t, tt.wantConnMaxIdleTime, settings.ConnMaxIdleTime)
		})
	}
}

func TestApplyDBPoolSettings(t *testing.T) {
	cfg := &config.Config{
		Database: config.DatabaseConfig{
			MaxOpenConns:           40,
			MaxIdleConns:           8,
			ConnMaxLifetimeMinutes: 15,
			ConnMaxIdleTimeMinutes: 3,
		},
	}

	db, err := sql.Open("postgres", "host=127.0.0.1 port=5432 user=postgres sslmode=disable")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = db.Close()
	})

	applyDBPoolSettings(db, cfg)
	stats := db.Stats()
	require.Equal(t, 40, stats.MaxOpenConnections)
}
