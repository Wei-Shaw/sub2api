// Package repository 提供应用程序的基础设施层组件。
// 包括数据库连接初始化、ORM 客户端管理、Redis 连接、数据库迁移等核心功能。
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/migrations"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/go-sql-driver/mysql"
	"github.com/lib/pq"
)

const (
	maxDatabaseInitializationRetries = 8
	databaseInitializationRetryBase  = time.Second
	databaseInitializationRetryMax   = 30 * time.Second
)

// initializeDatabaseWithRetry retries only errors that indicate PostgreSQL is
// temporarily unavailable during startup. Permanent configuration, migration,
// and data errors are returned immediately so they remain visible to operators.
func initializeDatabaseWithRetry(ctx context.Context, initialize func(context.Context) error) error {
	return initializeDatabaseWithRetryWithWait(ctx, initialize, waitForDatabaseInitializationRetry)
}

func initializeDatabaseWithRetryWithWait(
	ctx context.Context,
	initialize func(context.Context) error,
	wait func(context.Context, time.Duration) error,
) error {
	for attempt := 1; ; attempt++ {
		if err := initialize(ctx); err == nil {
			return nil
		} else {
			if !isTransientDatabaseInitializationError(err) || attempt > maxDatabaseInitializationRetries {
				return err
			}

			delay := databaseInitializationRetryBase * time.Duration(1<<(attempt-1))
			if delay > databaseInitializationRetryMax {
				delay = databaseInitializationRetryMax
			}
			slog.Warn("database initialization temporarily unavailable; retrying",
				"retry", attempt,
				"max_retries", maxDatabaseInitializationRetries,
				"retry_in", delay,
				"error", err,
			)
			if err := wait(ctx, delay); err != nil {
				return err
			}
		}
	}
}

func waitForDatabaseInitializationRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func isTransientDatabaseInitializationError(err error) bool {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		code := string(pqErr.Code)
		return code == "57P03" || strings.HasPrefix(code, "08")
	}
	return isMySQLTransientError(err)
}

// InitEnt 初始化 Ent ORM 客户端并返回客户端实例和底层的 *sql.DB。
//
// 该函数执行以下操作：
//  1. 初始化全局时区设置，确保时间处理一致性
//  2. 按配置的 driver（postgres / mysql）建立数据库连接
//  3. 自动执行数据库迁移，确保 schema 与代码同步
//  4. 创建并返回 Ent 客户端实例
//
// 重要提示：调用者必须负责关闭返回的 ent.Client（关闭时会自动关闭底层的 driver/db）。
//
// 参数：
//   - cfg: 应用程序配置，包含数据库连接信息和时区设置
//
// 返回：
//   - *ent.Client: Ent ORM 客户端，用于执行数据库操作
//   - *sql.DB: 底层的 SQL 数据库连接，可用于直接执行原生 SQL
//   - error: 初始化过程中的错误
func InitEnt(cfg *config.Config) (*ent.Client, *sql.DB, error) {
	// 优先初始化时区设置，确保所有时间操作使用统一的时区。
	// 这对于跨时区部署和日志时间戳的一致性至关重要。
	if err := timezone.Init(cfg.Timezone); err != nil {
		return nil, nil, err
	}

	// 按配置选择 SQL 方言并登记到进程级状态，
	// 供手写原生 SQL 的仓库层运行时分支使用。
	dialectName := cfg.Database.DriverName()
	SetDialect(dialectName)

	// 建立数据库连接。
	var drv *entsql.Driver
	if dialectName == dialect.MySQL {
		var err error
		drv, err = openEntMySQL(cfg)
		if err != nil {
			return nil, nil, err
		}
	} else {
		// 构建包含时区信息的数据库连接字符串 (DSN)。
		// 时区信息会传递给 PostgreSQL，确保数据库层面的时间处理正确。
		dsn := cfg.Database.DSNWithTimezone(cfg.Timezone)

		if cfg.Server.EnableServerTiming {
			connector, err := pq.NewConnector(dsn)
			if err != nil {
				return nil, nil, err
			}
			drv = entsql.OpenDB(dialect.Postgres, sql.OpenDB(newServerTimingConnector(connector)))
		} else {
			var err error
			drv, err = entsql.Open(dialect.Postgres, dsn)
			if err != nil {
				return nil, nil, err
			}
		}
	}
	applyDBPoolSettings(drv.DB(), cfg)

	// 确保数据库 schema 已准备就绪。
	// SQL 迁移文件是 schema 的权威来源（source of truth）。
	// 这种方式比 Ent 的自动迁移更可控，支持复杂的迁移场景。
	migrationCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if err := initializeDatabaseWithRetry(migrationCtx, func(ctx context.Context) error {
		return applyMigrationsFSWithDialect(ctx, drv.DB(), migrationsForDialect(dialectName), dialectName)
	}); err != nil {
		_ = drv.Close() // 迁移失败时关闭驱动，避免资源泄露
		return nil, nil, err
	}

	// 创建 Ent 客户端，绑定到已配置的数据库驱动。
	client := ent.NewClient(ent.Driver(drv))

	// 启动阶段：从配置或数据库中确保系统密钥可用。
	if err := ensureBootstrapSecrets(migrationCtx, client, cfg); err != nil {
		_ = client.Close()
		return nil, nil, err
	}

	// 在密钥补齐后执行完整配置校验，避免空 jwt.secret 导致服务运行时失败。
	if err := cfg.Validate(); err != nil {
		_ = client.Close()
		return nil, nil, fmt.Errorf("validate config after secret bootstrap: %w", err)
	}

	seedCtx, seedCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer seedCancel()
	if err := ensureSimpleModeStartup(seedCtx, client, cfg); err != nil {
		_ = client.Close()
		return nil, nil, err
	}

	return client, drv.DB(), nil
}

// openEntMySQL 建立 MySQL 连接的 entsql 驱动。
// go-sql-driver 在 init 时注册 "mysql" 驱动名，entsql.Open(dialect.MySQL, dsn)
// 直接可用；这里单独成函数以便支持 server-timing 连接器包装。
func openEntMySQL(cfg *config.Config) (*entsql.Driver, error) {
	mysqlCfg, err := cfg.Database.MySQLConfig(cfg.Timezone)
	if err != nil {
		return nil, err
	}
	dsn := mysqlCfg.FormatDSN()
	if cfg.Server.EnableServerTiming {
		connector, err := mysql.NewConnector(mysqlCfg)
		if err != nil {
			return nil, err
		}
		return entsql.OpenDB(dialect.MySQL, sql.OpenDB(newServerTimingConnector(connector))), nil
	}
	return entsql.Open(dialect.MySQL, dsn)
}

// migrationsForDialect 按方言返回迁移文件集：
// PostgreSQL 使用根目录迁移，MySQL 使用 mysql/ 子目录的镜像翻译版本。
func migrationsForDialect(dialectName string) fs.FS {
	if dialectName == dialect.MySQL {
		return migrations.MySQLFS
	}
	return migrations.FS
}

// ensureSimpleModeStartup keeps admin concurrency setup independent of group seeding.
func ensureSimpleModeStartup(ctx context.Context, client *ent.Client, cfg *config.Config) error {
	if cfg.RunMode != config.RunModeSimple {
		return nil
	}
	if cfg.SimpleMode.AutoCreateDefaultGroups {
		if err := ensureSimpleModeDefaultGroups(ctx, client); err != nil {
			return err
		}
	}
	return ensureSimpleModeAdminConcurrency(ctx, client)
}
