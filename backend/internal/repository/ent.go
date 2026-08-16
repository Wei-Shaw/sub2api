// Package repository 提供应用程序的基础设施层组件。
// 包括数据库连接初始化、ORM 客户端管理、Redis 连接、数据库迁移等核心功能。
package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/migrations"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/lib/pq"
)

// InitEnt 初始化 Ent ORM 客户端并返回客户端实例和底层的 *sql.DB。
//
// 该函数执行以下操作：
//  1. 初始化全局时区设置，确保时间处理一致性
//  2. 建立 PostgreSQL 数据库连接
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
type EntInitOptions struct {
	RunMigrations bool
	Bootstrap     bool
}

// InitEnt initializes the full standalone Ent runtime.
func InitEnt(cfg *config.Config) (*ent.Client, *sql.DB, error) {
	return InitEntWithOptions(cfg, EntInitOptions{RunMigrations: true, Bootstrap: true})
}

// OpenEnt opens an Ent connection without schema or bootstrap side effects.
func OpenEnt(cfg *config.Config) (*ent.Client, *sql.DB, error) {
	return InitEntWithOptions(cfg, EntInitOptions{})
}

func InitEntWithOptions(cfg *config.Config, options EntInitOptions) (*ent.Client, *sql.DB, error) {
	// 优先初始化时区设置，确保所有时间操作使用统一的时区。
	// 这对于跨时区部署和日志时间戳的一致性至关重要。
	if err := timezone.Init(cfg.Timezone); err != nil {
		return nil, nil, err
	}

	// 构建包含时区信息的数据库连接字符串 (DSN)。
	// 时区信息会传递给 PostgreSQL，确保数据库层面的时间处理正确。
	dsn := cfg.Database.DSNWithTimezone(cfg.Timezone)

	// 使用 Ent 的 SQL 驱动打开 PostgreSQL 连接。
	// dialect.Postgres 指定使用 PostgreSQL 方言进行 SQL 生成。
	var drv *entsql.Driver
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
	applyDBPoolSettings(drv.DB(), cfg)

	if options.RunMigrations {
		migrationCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		if err := applyMigrationsFS(migrationCtx, drv.DB(), migrations.FS); err != nil {
			_ = drv.Close()
			return nil, nil, err
		}
	}

	client := ent.NewClient(ent.Driver(drv))
	if !options.Bootstrap {
		return client, drv.DB(), nil
	}

	bootstrapCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if err := BootstrapInstallation(bootstrapCtx, drv.DB(), client, cfg); err != nil {
		_ = client.Close()
		return nil, nil, err
	}

	return client, drv.DB(), nil
}

func BootstrapEnt(ctx context.Context, client *ent.Client, cfg *config.Config) error {
	if err := ensureBootstrapSecrets(ctx, client, cfg); err != nil {
		return err
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("validate config after secret bootstrap: %w", err)
	}

	if cfg.RunMode != config.RunModeSimple {
		return nil
	}

	seedCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := ensureSimpleModeDefaultGroups(seedCtx, client); err != nil {
		return err
	}
	if err := ensureSimpleModeAdminConcurrency(seedCtx, client); err != nil {
		return err
	}
	return nil
}
