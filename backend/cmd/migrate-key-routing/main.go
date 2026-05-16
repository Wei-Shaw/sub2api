package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	_ "github.com/lib/pq"
)

type migrationStats struct {
	RetailUsers       int
	UsersWithoutKeys  int
	KeysToUnbind      int
	CreatedDefaultKey int
	UnboundKeys       int64
}

func main() {
	execute := flag.Bool("execute", false, "apply changes; default is dry-run")
	defaultKeyName := flag.String("default-key-name", "Default Key", "name for default keys created for users without keys")
	flag.Parse()

	logger.InitBootstrap()
	defer logger.Sync()

	cfg, err := config.LoadForBootstrap()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	client, sqlDB, err := repository.InitEnt(cfg)
	if err != nil {
		log.Fatalf("init database: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	stats, err := migrate(ctx, sqlDB, cfg.Default.APIKeyPrefix, strings.TrimSpace(*defaultKeyName), *execute)
	if err != nil {
		log.Fatalf("migration failed: %v", err)
	}

	mode := "DRY RUN"
	if *execute {
		mode = "EXECUTED"
	}
	fmt.Printf("%s\n", mode)
	fmt.Printf("retail users: %d\n", stats.RetailUsers)
	fmt.Printf("users without keys: %d\n", stats.UsersWithoutKeys)
	fmt.Printf("keys with group_id to clear: %d\n", stats.KeysToUnbind)
	fmt.Printf("default keys created: %d\n", stats.CreatedDefaultKey)
	fmt.Printf("keys unbound: %d\n", stats.UnboundKeys)
	if !*execute {
		fmt.Println("No changes were written. Re-run with --execute to apply.")
	}
}

func migrate(ctx context.Context, db *sql.DB, keyPrefix, defaultKeyName string, execute bool) (*migrationStats, error) {
	if db == nil {
		return nil, fmt.Errorf("database is nil")
	}
	if defaultKeyName == "" {
		defaultKeyName = "Default Key"
	}
	if keyPrefix == "" {
		keyPrefix = "sk-"
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	stats := &migrationStats{}
	if err := tx.QueryRowContext(ctx, `
		SELECT count(*)
		FROM users
		WHERE deleted_at IS NULL
		  AND role = $1
		  AND customer_type = $2
	`, domain.RoleUser, domain.CustomerTypeRetail).Scan(&stats.RetailUsers); err != nil {
		return nil, fmt.Errorf("count retail users: %w", err)
	}

	userRows, err := tx.QueryContext(ctx, `
		SELECT u.id
		FROM users u
		WHERE u.deleted_at IS NULL
		  AND u.role = $1
		  AND u.customer_type = $2
		  AND NOT EXISTS (
		    SELECT 1
		    FROM api_keys k
		    WHERE k.user_id = u.id
		      AND k.deleted_at IS NULL
		  )
		ORDER BY u.id
	`, domain.RoleUser, domain.CustomerTypeRetail)
	if err != nil {
		return nil, fmt.Errorf("list users without keys: %w", err)
	}
	defer userRows.Close()

	userIDs := make([]int64, 0)
	for userRows.Next() {
		var userID int64
		if err := userRows.Scan(&userID); err != nil {
			return nil, fmt.Errorf("scan user without key: %w", err)
		}
		userIDs = append(userIDs, userID)
	}
	if err := userRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate users without keys: %w", err)
	}
	stats.UsersWithoutKeys = len(userIDs)

	if err := tx.QueryRowContext(ctx, `
		SELECT count(*)
		FROM api_keys k
		JOIN users u ON u.id = k.user_id
		WHERE k.deleted_at IS NULL
		  AND k.group_id IS NOT NULL
		  AND u.deleted_at IS NULL
		  AND u.role = $1
		  AND u.customer_type = $2
	`, domain.RoleUser, domain.CustomerTypeRetail).Scan(&stats.KeysToUnbind); err != nil {
		return nil, fmt.Errorf("count keys to unbind: %w", err)
	}

	if !execute {
		if err := tx.Rollback(); err != nil {
			return nil, err
		}
		return stats, nil
	}

	for _, userID := range userIDs {
		key, err := generateUniqueKey(ctx, tx, keyPrefix)
		if err != nil {
			return nil, fmt.Errorf("generate default key for user %d: %w", userID, err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO api_keys (
				user_id, key, name, status, ip_whitelist, ip_blacklist, ip_lock_mode, limit_action,
				quota, quota_used, rate_limit_5h, rate_limit_1d, rate_limit_7d, rate_limit_1mo,
				usage_5h, usage_1d, usage_7d, usage_1mo, created_at, updated_at
			)
			VALUES ($1, $2, $3, $4, '[]', '[]', 'off', 'hard_block', 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, NOW(), NOW())
		`, userID, key, defaultKeyName, domain.StatusActive); err != nil {
			return nil, fmt.Errorf("insert default key for user %d: %w", userID, err)
		}
		stats.CreatedDefaultKey++
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE api_keys k
		SET group_id = NULL,
		    updated_at = NOW()
		FROM users u
		WHERE u.id = k.user_id
		  AND k.deleted_at IS NULL
		  AND k.group_id IS NOT NULL
		  AND u.deleted_at IS NULL
		  AND u.role = $1
		  AND u.customer_type = $2
	`, domain.RoleUser, domain.CustomerTypeRetail)
	if err != nil {
		return nil, fmt.Errorf("clear key group bindings: %w", err)
	}
	stats.UnboundKeys, _ = result.RowsAffected()

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return stats, nil
}

func generateUniqueKey(ctx context.Context, tx *sql.Tx, prefix string) (string, error) {
	for i := 0; i < 20; i++ {
		key, err := generateKey(prefix)
		if err != nil {
			return "", err
		}
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM api_keys WHERE key = $1 AND deleted_at IS NULL)`, key).Scan(&exists); err != nil {
			return "", err
		}
		if !exists {
			return key, nil
		}
	}
	return "", fmt.Errorf("failed to generate a unique key")
}

func generateKey(prefix string) (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(bytes), nil
}
