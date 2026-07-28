package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/migrations"
)

// schemaMigrationsTableDDL 定义迁移记录表的 DDL。
// 该表用于跟踪已应用的迁移文件及其校验和。
// - filename: 迁移文件名，作为主键唯一标识每个迁移
// - checksum: 文件内容的 SHA256 哈希值，用于检测迁移文件是否被篡改
// - applied_at: 迁移应用时间戳
const schemaMigrationsTableDDL = `
CREATE TABLE IF NOT EXISTS schema_migrations (
	filename   TEXT PRIMARY KEY,
	checksum   TEXT NOT NULL,
	applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
`

const schemaMigrationStepsTableDDL = `
CREATE TABLE IF NOT EXISTS schema_migration_steps (
	filename            TEXT NOT NULL,
		migration_checksum  TEXT NOT NULL,
		statement_index     INTEGER NOT NULL CHECK (statement_index > 0),
		statement_checksum  TEXT NOT NULL,
		index_schema        TEXT NULL,
		index_name          TEXT NULL,
		target_relation_oid BIGINT NULL,
		created_index_oid   BIGINT NULL,
		created_index_definition TEXT NULL,
		created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	PRIMARY KEY (filename, statement_index)
);
`

const schemaMigrationStepsProofColumnsDDL = `
	ALTER TABLE schema_migration_steps
		ADD COLUMN IF NOT EXISTS created_index_oid BIGINT NULL,
		ADD COLUMN IF NOT EXISTS created_index_definition TEXT NULL;
`

const atlasSchemaRevisionsTableDDL = `
CREATE TABLE IF NOT EXISTS atlas_schema_revisions (
	version TEXT PRIMARY KEY,
	description TEXT NOT NULL,
	type INTEGER NOT NULL,
	applied INTEGER NOT NULL DEFAULT 0,
	total INTEGER NOT NULL DEFAULT 0,
	executed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	execution_time BIGINT NOT NULL DEFAULT 0,
	error TEXT NULL,
	error_stmt TEXT NULL,
	hash TEXT NOT NULL DEFAULT '',
	partial_hashes TEXT[] NULL,
	operator_version TEXT NULL
);
`

// migrationsAdvisoryLockID 是用于序列化迁移操作的 PostgreSQL Advisory Lock ID。
// 在多实例部署场景下，该锁确保同一时间只有一个实例执行迁移。
// 任何稳定的 int64 值都可以，只要不与同一数据库中的其他锁冲突即可。
const migrationsAdvisoryLockID int64 = 694208311321144027
const migrationsLockRetryInterval = 500 * time.Millisecond
const nonTransactionalMigrationSuffix = "_notx.sql"
const paymentOrdersOutTradeNoUniqueMigration = "120_enforce_payment_orders_out_trade_no_unique_notx.sql"
const postgresIdentifierMaxBytes = 63

type migrationChecksumCompatibilityRule struct {
	fileChecksum       string
	acceptedDBChecksum map[string]struct{}
	acceptedChecksums  map[string]struct{}
}

// migrationChecksumCompatibilityRules 仅用于兼容历史上误修改过的迁移文件 checksum。
// 规则必须同时匹配「迁移名 + 数据库 checksum + 当前文件 checksum」且两者都落在该迁移的已知版本集合内才会放行，
// 避免放宽全局校验，也允许将误改的历史 migration 回滚为已发布版本而不要求人工修 checksum。
var migrationChecksumCompatibilityRules = map[string]migrationChecksumCompatibilityRule{
	"054_drop_legacy_cache_columns.sql":                       newMigrationChecksumCompatibilityRule("82de761156e03876653e7a6a4eee883cd927847036f779b0b9f34c42a8af7a7d", "182c193f3359946cf094090cd9e57d5c3fd9abaffbc1e8fc378646b8a6fa12b4"),
	"061_add_usage_log_request_type.sql":                      newMigrationChecksumCompatibilityRule("66207e7aa5dd0429c2e2c0fabdaf79783ff157fa0af2e81adff2ee03790ec65c", "08a248652cbab7cfde147fc6ef8cda464f2477674e20b718312faa252e0481c0", "222b4a09c797c22e5922b6b172327c824f5463aaa8760e4f621bc5c22e2be0f3"),
	"109_auth_identity_compat_backfill.sql":                   newMigrationChecksumCompatibilityRule("0580b4602d85435edf9aca1633db580bb3932f26517f75134106f80275ec2ace", "551e498aa5616d2d91096e9d72cf9fb36e418ee22eacc557f8811cadbc9e20ee"),
	"110_pending_auth_and_provider_default_grants.sql":        newMigrationChecksumCompatibilityRule("32cf87ee787b1bb36b5c691367c96eee37518fa3eed6f3322cf68795e3745279", "e3d1f433be2b564cfbdc549adf98fce13c5c7b363ebc20fd05b765d0563b0925"),
	"112_add_payment_order_provider_key_snapshot.sql":         newMigrationChecksumCompatibilityRule("b75f8f56d39455682787696a3d92ad25b055444ca328fb7fca9a460a15d68d99", "ffd3e8a2c9295fa9cbefefd629a78268877e5b51bc970a82d9b3f46ec4ebd15e"),
	"115_auth_identity_legacy_external_backfill.sql":          newMigrationChecksumCompatibilityRule("022aadd97bb53e755f0cf7a3a957e0cb1a1353b0c39ec4de3234acd2871fd04f", "4cf39e508be9fd1a5aa41610cbbebeb80385c9adda45bf78a706de9db4f1385f"),
	"116_auth_identity_legacy_external_safety_reports.sql":    newMigrationChecksumCompatibilityRule("07edb09fa8d04ffb172b0621e3c22f4d1757d20a24ae267b3b36b087ab72d488", "f7757bd929ac67ffb08ce69fa4cf20fad39dbff9d5a5085fb2adabb7607e5877"),
	"118_wechat_dual_mode_and_auth_source_defaults.sql":       newMigrationChecksumCompatibilityRule("b54194d7a3e4fbf710e0a3590d22a2fe7966804c487052a356e0b55f53ef96b0", "e0cdf835d6c688d64100f483d31bc02ac9ebad414bf1837af239a84bf75b8227", "a38243ca0a72c3a01c0a92b7986423054d6133c0399441f853b99802852720fb"),
	"119_enforce_payment_orders_out_trade_no_unique.sql":      newMigrationChecksumCompatibilityRule("0bbe809ae48a9d811dabda1ba1c74955bd71c4a9cc610f9128816818dfa6c11e", "ebd2c67cce0116393fb4f1b5d5116a67c6aceb73820dfb5133d1ff6f36d72d34"),
	"120_enforce_payment_orders_out_trade_no_unique_notx.sql": newMigrationChecksumCompatibilityRule("34aadc0db59a4e390f92a12b73bd74642d9724f33124f73638ae00089ea5e074", "e77921f79d539bc24575cb9c16cbe566d2b23ce816190343d0a7568f6a3fcf61", "707431450603e70a43ce9fbd61e0c12fa67da4875158ccefabacea069587ab22", "04b082b5a239c525154fe9185d324ee2b05ff90da9297e10dba19f9be79aa59a"),
	"123_fix_legacy_auth_source_grant_on_signup_defaults.sql": newMigrationChecksumCompatibilityRule("2ce43c2cd89e9f9e1febd34a407ed9e84d177386c5544b6f02c1f58a21129f57", "6cd33422f215dcd1f486ab6f35c0ea5805d9ca69bb25906d94bc649156657145"),
	"159_batch_image_foundation.sql":                          newMigrationChecksumCompatibilityRule("d902b70982025ec519749faf058aab7631e82c3f48167b9a4ae4db718eb72cce", "82da85b5d98e67a0507647b873a40373e84538e4adafdeed6767c0ac8b6570b2"),
	"161_batch_image_pricing_snapshot.sql":                    newMigrationChecksumCompatibilityRule("4012af3e43636cb6af22e0176d59d1fcc70615c0f310194329461ae462c4fbd6", "96d915c9b7a6941ae99039e0ff3f1a61481eb9bddd933d11c6fadb2274554e87"),
}

// ApplyMigrations 将嵌入的 SQL 迁移文件应用到指定的数据库。
//
// 该函数可以在每次应用启动时安全调用：
// - 已应用的迁移会被自动跳过（通过校验 filename 判断）
// - 如果迁移文件内容被修改（checksum 不匹配），会返回错误
// - 使用 PostgreSQL Advisory Lock 确保多实例并发安全
//
// 参数：
//   - ctx: 上下文，用于超时控制和取消
//   - db: 数据库连接
//
// 返回：
//   - error: 迁移过程中的任何错误
func ApplyMigrations(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return errors.New("nil sql db")
	}
	return applyMigrationsFS(ctx, db, migrations.FS)
}

// applyMigrationsFS 是迁移执行的核心实现。
// 它从指定的文件系统读取 SQL 迁移文件并按顺序应用。
//
// 迁移执行流程：
//  1. 获取 PostgreSQL Advisory Lock，防止多实例并发迁移
//  2. 确保 schema_migrations 表存在
//  3. 按文件名排序读取所有 .sql 文件
//  4. 对于每个迁移文件：
//     - 计算文件内容的 SHA256 校验和
//     - 检查该迁移是否已应用（通过 filename 查询）
//     - 如果已应用，验证校验和是否匹配
//     - 如果未应用，在事务中执行迁移并记录
//  5. 释放 Advisory Lock
//
// 参数：
//   - ctx: 上下文
//   - db: 数据库连接
//   - fsys: 包含迁移文件的文件系统（通常是 embed.FS）
func applyMigrationsFS(ctx context.Context, db *sql.DB, fsys fs.FS) error {
	if db == nil {
		return errors.New("nil sql db")
	}

	// 获取分布式锁，确保多实例部署时只有一个实例执行迁移。
	// 这是 PostgreSQL 特有的 Advisory Lock 机制。
	lockConn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire migrations lock connection: %w", err)
	}
	if err := pgAdvisoryLock(ctx, lockConn); err != nil {
		discardSQLConnection(lockConn)
		return err
	}
	defer func() {
		// 无论迁移是否成功，都要释放锁。
		// 独立超时确保原 ctx 取消后仍会尝试释放，但数据库链路异常不会
		// 无限阻塞进程退出。
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := pgAdvisoryUnlock(unlockCtx, lockConn); err != nil {
			discardSQLConnection(lockConn)
			return
		}
		_ = lockConn.Close()
	}()

	// 创建迁移记录表（如果不存在）。
	// 该表记录所有已应用的迁移及其校验和。
	if _, err := lockConn.ExecContext(ctx, schemaMigrationsTableDDL); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	if _, err := lockConn.ExecContext(ctx, schemaMigrationStepsTableDDL); err != nil {
		return fmt.Errorf("create schema_migration_steps: %w", err)
	}
	if _, err := lockConn.ExecContext(ctx, schemaMigrationStepsProofColumnsDDL); err != nil {
		return fmt.Errorf("upgrade schema_migration_steps proof columns: %w", err)
	}

	// 自动对齐 Atlas 基线（如果检测到 legacy schema_migrations 且缺失 atlas_schema_revisions）。
	if err := ensureAtlasBaselineAligned(ctx, lockConn, fsys); err != nil {
		return err
	}

	// 获取所有 .sql 迁移文件并按文件名排序。
	// 命名规范：使用零填充数字前缀（如 001_init.sql, 002_add_users.sql）。
	files, err := fs.Glob(fsys, "*.sql")
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}
	sort.Strings(files) // 确保按文件名顺序执行迁移

	for _, name := range files {
		// 读取迁移文件内容
		contentBytes, err := fs.ReadFile(fsys, name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}

		content := strings.TrimSpace(string(contentBytes))
		if content == "" {
			continue // 跳过空文件
		}

		// 计算文件内容的 SHA256 校验和，用于检测文件是否被修改。
		// 这是一种防篡改机制：如果有人修改了已应用的迁移文件，系统会拒绝启动。
		sum := sha256.Sum256([]byte(content))
		checksum := hex.EncodeToString(sum[:])

		// 检查该迁移是否已经应用
		var existing string
		rowErr := lockConn.QueryRowContext(ctx, "SELECT checksum FROM schema_migrations WHERE filename = $1", name).Scan(&existing)
		if rowErr == nil {
			// 迁移已应用，验证校验和是否匹配
			if existing != checksum {
				// 兼容特定历史误改场景（仅白名单规则），其余仍保持严格不可变约束。
				if isMigrationChecksumCompatible(name, existing, checksum) {
					continue
				}
				// 校验和不匹配意味着迁移文件在应用后被修改，这是危险的。
				// 正确的做法是创建新的迁移文件来进行变更。
				return fmt.Errorf(
					"migration %s checksum mismatch (db=%s file=%s)\n"+
						"This means the migration file was modified after being applied to the database.\n"+
						"Solutions:\n"+
						"  1. Revert to original: git log --oneline -- migrations/%s && git checkout <commit> -- migrations/%s\n"+
						"  2. For new changes, create a new migration file instead of modifying existing ones\n"+
						"Note: Modifying applied migrations breaks the immutability principle and can cause inconsistencies across environments",
					name, existing, checksum, name, name,
				)
			}
			continue // 迁移已应用且校验和匹配，跳过
		}
		if !errors.Is(rowErr, sql.ErrNoRows) {
			return fmt.Errorf("check migration %s: %w", name, rowErr)
		}

		nonTx, err := validateMigrationExecutionMode(name, content)
		if err != nil {
			return fmt.Errorf("validate migration %s: %w", name, err)
		}

		if nonTx {
			if err := applyNonTransactionalMigration(ctx, lockConn, name, checksum, content); err != nil {
				return err
			}
			continue
		}

		// 默认迁移在事务中执行，确保原子性：要么完全成功，要么完全回滚。
		tx, err := lockConn.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", name, err)
		}
		if err := configureMigrationTransaction(ctx, tx); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("configure migration %s transaction: %w", name, err)
		}

		// 执行迁移 SQL
		if _, err := tx.ExecContext(ctx, content); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", name, err)
		}

		// 记录迁移已完成，保存文件名和校验和
		if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations (filename, checksum) VALUES ($1, $2)", name, checksum); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %s: %w", name, err)
		}

		// 提交事务
		if err := tx.Commit(); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("commit migration %s: %w", name, err)
		}
	}

	return nil
}

const migrationTransactionTimeoutsSQL = "SET LOCAL lock_timeout = '1s'; SET LOCAL statement_timeout = '10min'"

func configureMigrationTransaction(ctx context.Context, tx *sql.Tx) error {
	if tx == nil {
		return errors.New("nil migration transaction")
	}
	_, err := tx.ExecContext(ctx, migrationTransactionTimeoutsSQL)
	return err
}

type migrationConnection interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

func applyNonTransactionalMigration(ctx context.Context, db migrationConnection, name, checksum, content string) error {
	if err := prepareNonTransactionalMigration(ctx, db, name); err != nil {
		return fmt.Errorf("prepare migration %s: %w", name, err)
	}

	statements, err := splitSQLStatements(content)
	if err != nil {
		return fmt.Errorf("split migration %s: %w", name, err)
	}
	for i, statement := range statements {
		trimmed := strings.TrimSpace(statement)
		if !statementHasCode(trimmed) {
			continue
		}

		definition, createsIndex, err := parseCreateConcurrentIndex(trimmed)
		if err != nil {
			return fmt.Errorf("parse migration %s (non-tx statement %d): %w", name, i+1, err)
		}
		var index *qualifiedIndexName
		executionStatement := trimmed
		if createsIndex {
			resolved, err := resolveConcurrentIndexName(ctx, db, definition)
			if err != nil {
				return fmt.Errorf("resolve migration %s (non-tx statement %d): %w", name, i+1, err)
			}
			index = &resolved
			executionStatement = definition.strictStatement
		}

		stepChecksum := checksumText(trimmed)
		intent := nonTransactionalStepIntent{
			filename:          name,
			migrationChecksum: checksum,
			statementIndex:    i + 1,
			statementChecksum: stepChecksum,
			index:             index,
		}
		journal, err := loadNonTransactionalStepIntent(ctx, db, intent)
		if err != nil {
			return fmt.Errorf("journal migration %s (non-tx statement %d): %w", name, i+1, err)
		}

		var indexState existingIndexState
		if index != nil {
			var completed bool
			indexState, completed, err = inspectConcurrentIndexStep(ctx, db, *index, journal)
			if err != nil {
				return fmt.Errorf("prepare migration %s (non-tx statement %d): %w", name, i+1, err)
			}
			if completed {
				continue
			}
		}
		if !journal.exists {
			if err := createNonTransactionalStepIntent(ctx, db, intent); err != nil {
				return fmt.Errorf("journal migration %s (non-tx statement %d): %w", name, i+1, err)
			}
		}
		if index != nil && indexState.exists {
			if _, err := db.ExecContext(ctx, "DROP INDEX CONCURRENTLY IF EXISTS "+index.SQLName()); err != nil {
				return fmt.Errorf("prepare migration %s (non-tx statement %d): drop invalid index %s before retry: %w", name, i+1, index.SQLName(), err)
			}
		}

		if _, err := db.ExecContext(ctx, executionStatement); err != nil {
			return fmt.Errorf("apply migration %s (non-tx statement %d): %w", name, i+1, err)
		}
		if index != nil {
			state, err := inspectExistingIndex(ctx, db, *index)
			if err != nil {
				return fmt.Errorf("verify migration %s (non-tx statement %d): %w", name, i+1, err)
			}
			if !state.exists || state.targetRelationOID != index.targetRelationOID || !state.healthy {
				return fmt.Errorf("verify migration %s (non-tx statement %d): concurrent index %s is not valid and ready on the expected relation", name, i+1, index.SQLName())
			}
			if err := completeNonTransactionalIndexStep(ctx, db, intent, state); err != nil {
				return fmt.Errorf("journal migration %s (non-tx statement %d) completion: %w", name, i+1, err)
			}
		}
	}

	if err := finalizeNonTransactionalMigration(ctx, db, name, checksum); err != nil {
		return fmt.Errorf("record migration %s (non-tx): %w", name, err)
	}
	return nil
}

func checksumText(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

type nonTransactionalStepIntent struct {
	filename          string
	migrationChecksum string
	statementIndex    int
	statementChecksum string
	index             *qualifiedIndexName
}

type nonTransactionalStepJournal struct {
	exists                 bool
	indexCompleted         bool
	createdIndexOID        int64
	createdIndexDefinition string
}

func loadNonTransactionalStepIntent(ctx context.Context, db migrationConnection, expected nonTransactionalStepIntent) (nonTransactionalStepJournal, error) {
	var (
		migrationChecksum      string
		statementChecksum      string
		indexSchema            sql.NullString
		indexName              sql.NullString
		targetRelationOID      sql.NullInt64
		createdIndexOID        sql.NullInt64
		createdIndexDefinition sql.NullString
	)
	err := db.QueryRowContext(ctx, `
		SELECT migration_checksum, statement_checksum, index_schema, index_name, target_relation_oid,
		       created_index_oid, created_index_definition
		FROM schema_migration_steps
		WHERE filename = $1 AND statement_index = $2
	`, expected.filename, expected.statementIndex).Scan(
		&migrationChecksum,
		&statementChecksum,
		&indexSchema,
		&indexName,
		&targetRelationOID,
		&createdIndexOID,
		&createdIndexDefinition,
	)
	if err == nil {
		if migrationChecksum != expected.migrationChecksum || statementChecksum != expected.statementChecksum ||
			!stepIntentIndexMatches(expected.index, indexSchema, indexName, targetRelationOID) {
			return nonTransactionalStepJournal{}, errors.New("stored non-transactional migration intent does not match the current immutable statement")
		}
		if createdIndexOID.Valid != createdIndexDefinition.Valid {
			return nonTransactionalStepJournal{}, errors.New("stored non-transactional migration index proof is incomplete")
		}
		if createdIndexOID.Valid && (createdIndexOID.Int64 <= 0 || createdIndexDefinition.String == "") {
			return nonTransactionalStepJournal{}, errors.New("stored non-transactional migration index proof is invalid")
		}
		if expected.index == nil && createdIndexOID.Valid {
			return nonTransactionalStepJournal{}, errors.New("stored non-index migration step has an unexpected index proof")
		}
		return nonTransactionalStepJournal{
			exists:                 true,
			indexCompleted:         createdIndexOID.Valid,
			createdIndexOID:        createdIndexOID.Int64,
			createdIndexDefinition: createdIndexDefinition.String,
		}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nonTransactionalStepJournal{}, err
	}
	return nonTransactionalStepJournal{}, nil
}

func createNonTransactionalStepIntent(ctx context.Context, db migrationConnection, expected nonTransactionalStepIntent) error {
	var schemaValue, nameValue any
	var oidValue any
	if expected.index != nil {
		schemaValue = expected.index.schema
		nameValue = expected.index.name
		oidValue = expected.index.targetRelationOID
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO schema_migration_steps (
			filename, migration_checksum, statement_index, statement_checksum,
			index_schema, index_name, target_relation_oid
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, expected.filename, expected.migrationChecksum, expected.statementIndex, expected.statementChecksum, schemaValue, nameValue, oidValue); err != nil {
		return err
	}
	return nil
}

func completeNonTransactionalIndexStep(ctx context.Context, db migrationConnection, expected nonTransactionalStepIntent, state existingIndexState) error {
	if expected.index == nil {
		return errors.New("cannot complete index proof for a non-index migration step")
	}
	if !state.exists || !state.healthy || state.indexOID <= 0 || state.definition == "" || state.targetRelationOID != expected.index.targetRelationOID {
		return errors.New("cannot persist proof for an invalid concurrent index state")
	}
	result, err := db.ExecContext(ctx, `
		UPDATE schema_migration_steps
		SET created_index_oid = $8, created_index_definition = $9
		WHERE filename = $1
		  AND migration_checksum = $2
		  AND statement_index = $3
		  AND statement_checksum = $4
		  AND index_schema = $5
		  AND index_name = $6
		  AND target_relation_oid = $7
		  AND created_index_oid IS NULL
		  AND created_index_definition IS NULL
	`, expected.filename, expected.migrationChecksum, expected.statementIndex, expected.statementChecksum,
		expected.index.schema, expected.index.name, expected.index.targetRelationOID,
		state.indexOID, state.definition)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read completed index proof update result: %w", err)
	}
	if rows != 1 {
		return fmt.Errorf("completed index proof update affected %d rows instead of 1", rows)
	}
	return nil
}

func stepIntentIndexMatches(expected *qualifiedIndexName, schema, name sql.NullString, targetOID sql.NullInt64) bool {
	if expected == nil {
		return !schema.Valid && !name.Valid && !targetOID.Valid
	}
	return schema.Valid && schema.String == expected.schema &&
		name.Valid && name.String == expected.name &&
		targetOID.Valid && targetOID.Int64 == expected.targetRelationOID
}

func finalizeNonTransactionalMigration(ctx context.Context, db migrationConnection, name, checksum string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations (filename, checksum) VALUES ($1, $2)", name, checksum); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM schema_migration_steps WHERE filename = $1", name); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		return err
	}
	return nil
}

func prepareNonTransactionalMigration(ctx context.Context, db migrationConnection, name string) error {
	if name == paymentOrdersOutTradeNoUniqueMigration {
		return preparePaymentOrdersOutTradeNoUniqueMigration(ctx, db)
	}
	return nil
}

func preparePaymentOrdersOutTradeNoUniqueMigration(ctx context.Context, db migrationConnection) error {
	duplicates, err := findDuplicatePaymentOrderOutTradeNos(ctx, db)
	if err != nil {
		return fmt.Errorf("precheck duplicate out_trade_no: %w", err)
	}
	if len(duplicates) > 0 {
		return fmt.Errorf(
			"duplicate out_trade_no values block %s; remediate duplicates before retrying: %s",
			paymentOrdersOutTradeNoUniqueMigration,
			strings.Join(duplicates, ", "),
		)
	}

	return nil
}

type qualifiedIndexName struct {
	schema            string
	name              string
	targetRelationOID int64
}

type qualifiedRelationName struct {
	schema string
	name   string
}

func (n qualifiedRelationName) SQLName() string {
	if n.schema == "" {
		return quoteSQLIdentifier(n.name)
	}
	return quoteSQLIdentifier(n.schema) + "." + quoteSQLIdentifier(n.name)
}

type concurrentIndexDefinition struct {
	name            string
	table           qualifiedRelationName
	strictStatement string
}

func (n qualifiedIndexName) SQLName() string {
	return quoteSQLIdentifier(n.schema) + "." + quoteSQLIdentifier(n.name)
}

func quoteSQLIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func resolveConcurrentIndexName(ctx context.Context, db migrationConnection, definition concurrentIndexDefinition) (qualifiedIndexName, error) {
	var schema string
	var targetRelationOID int64
	err := db.QueryRowContext(ctx, `
		SELECT ns.nspname, target.oid::bigint
		FROM pg_class target
		JOIN pg_namespace ns ON ns.oid = target.relnamespace
		WHERE target.oid = to_regclass($1)
	`, definition.table.SQLName()).Scan(&schema, &targetRelationOID)
	if errors.Is(err, sql.ErrNoRows) {
		return qualifiedIndexName{}, fmt.Errorf("target relation %s does not exist", definition.table.SQLName())
	}
	if err != nil {
		return qualifiedIndexName{}, fmt.Errorf("resolve target relation %s: %w", definition.table.SQLName(), err)
	}
	return qualifiedIndexName{schema: schema, name: definition.name, targetRelationOID: targetRelationOID}, nil
}

type existingIndexState struct {
	exists            bool
	indexOID          int64
	targetRelationOID int64
	healthy           bool
	definition        string
}

func inspectConcurrentIndexStep(ctx context.Context, db migrationConnection, index qualifiedIndexName, journal nonTransactionalStepJournal) (existingIndexState, bool, error) {
	state, err := inspectExistingIndex(ctx, db, index)
	if err != nil {
		return existingIndexState{}, false, fmt.Errorf("inspect existing index %s: %w", index.SQLName(), err)
	}
	if !state.exists {
		if journal.indexCompleted {
			return existingIndexState{}, false, fmt.Errorf("completed index proof for %s no longer resolves to an index", index.SQLName())
		}
		return state, false, nil
	}
	if state.targetRelationOID != index.targetRelationOID {
		return existingIndexState{}, false, fmt.Errorf(
			"index name collision: %s belongs to relation OID %d, expected target relation OID %d",
			index.SQLName(),
			state.targetRelationOID,
			index.targetRelationOID,
		)
	}
	if journal.indexCompleted {
		if !state.healthy || state.indexOID != journal.createdIndexOID || state.definition != journal.createdIndexDefinition {
			return existingIndexState{}, false, fmt.Errorf("completed index proof for %s does not match the current index object and definition", index.SQLName())
		}
		return state, true, nil
	}
	if state.healthy {
		if !journal.exists {
			return existingIndexState{}, false, fmt.Errorf("healthy index %s already exists without a matching migration recovery intent", index.SQLName())
		}
		return existingIndexState{}, false, fmt.Errorf("healthy index %s has only a pending migration intent; refusing to drop an unproven live index", index.SQLName())
	}
	return state, false, nil
}

func findDuplicatePaymentOrderOutTradeNos(ctx context.Context, db migrationConnection) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT out_trade_no, COUNT(*) AS duplicate_count
		FROM payment_orders
		WHERE out_trade_no <> ''
		GROUP BY out_trade_no
		HAVING COUNT(*) > 1
		ORDER BY duplicate_count DESC, out_trade_no
		LIMIT 5
	`)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	duplicates := make([]string, 0, 5)
	for rows.Next() {
		var outTradeNo string
		var duplicateCount int
		if err := rows.Scan(&outTradeNo, &duplicateCount); err != nil {
			return nil, err
		}
		duplicates = append(duplicates, fmt.Sprintf("%s (count=%d)", outTradeNo, duplicateCount))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return duplicates, nil
}

func inspectExistingIndex(ctx context.Context, db migrationConnection, index qualifiedIndexName) (existingIndexState, error) {
	var state existingIndexState
	err := db.QueryRowContext(ctx, `
		SELECT idx.oid::bigint,
		       i.indrelid::bigint,
		       i.indisvalid AND i.indisready AND i.indislive,
		       pg_get_indexdef(idx.oid)
		FROM pg_class idx
		JOIN pg_namespace ns ON ns.oid = idx.relnamespace
		JOIN pg_index i ON i.indexrelid = idx.oid
		WHERE ns.nspname = $1
		  AND idx.relname = $2
	`, index.schema, index.name).Scan(&state.indexOID, &state.targetRelationOID, &state.healthy, &state.definition)
	if errors.Is(err, sql.ErrNoRows) {
		return existingIndexState{}, nil
	}
	if err != nil {
		return existingIndexState{}, err
	}
	state.exists = true
	return state, nil
}

func ensureAtlasBaselineAligned(ctx context.Context, db migrationConnection, fsys fs.FS) error {
	hasLegacy, err := tableExists(ctx, db, "schema_migrations")
	if err != nil {
		return fmt.Errorf("check schema_migrations: %w", err)
	}
	if !hasLegacy {
		return nil
	}

	hasAtlas, err := tableExists(ctx, db, "atlas_schema_revisions")
	if err != nil {
		return fmt.Errorf("check atlas_schema_revisions: %w", err)
	}
	if !hasAtlas {
		if _, err := db.ExecContext(ctx, atlasSchemaRevisionsTableDDL); err != nil {
			return fmt.Errorf("create atlas_schema_revisions: %w", err)
		}
	}

	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM atlas_schema_revisions").Scan(&count); err != nil {
		return fmt.Errorf("count atlas_schema_revisions: %w", err)
	}
	if count > 0 {
		return nil
	}

	version, description, hash, err := latestMigrationBaseline(fsys)
	if err != nil {
		return fmt.Errorf("atlas baseline version: %w", err)
	}

	if _, err := db.ExecContext(ctx, `
		INSERT INTO atlas_schema_revisions (version, description, type, applied, total, executed_at, execution_time, hash)
		VALUES ($1, $2, $3, 0, 0, NOW(), 0, $4)
	`, version, description, 1, hash); err != nil {
		return fmt.Errorf("insert atlas baseline: %w", err)
	}
	return nil
}

func tableExists(ctx context.Context, db migrationConnection, tableName string) (bool, error) {
	var exists bool
	err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = $1
		)
	`, tableName).Scan(&exists)
	return exists, err
}

func latestMigrationBaseline(fsys fs.FS) (string, string, string, error) {
	files, err := fs.Glob(fsys, "*.sql")
	if err != nil {
		return "", "", "", err
	}
	if len(files) == 0 {
		return "baseline", "baseline", "", nil
	}
	sort.Strings(files)
	name := files[len(files)-1]
	contentBytes, err := fs.ReadFile(fsys, name)
	if err != nil {
		return "", "", "", err
	}
	content := strings.TrimSpace(string(contentBytes))
	sum := sha256.Sum256([]byte(content))
	hash := hex.EncodeToString(sum[:])
	version := strings.TrimSuffix(name, ".sql")
	return version, version, hash, nil
}

func checksumSet(values ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}

func newMigrationChecksumCompatibilityRule(fileChecksum string, acceptedDBChecksums ...string) migrationChecksumCompatibilityRule {
	return migrationChecksumCompatibilityRule{
		fileChecksum:       fileChecksum,
		acceptedDBChecksum: checksumSet(acceptedDBChecksums...),
		acceptedChecksums:  checksumSet(append([]string{fileChecksum}, acceptedDBChecksums...)...),
	}
}

func isMigrationChecksumCompatible(name, dbChecksum, fileChecksum string) bool {
	rule, ok := migrationChecksumCompatibilityRules[name]
	if !ok {
		return false
	}
	_, dbOK := rule.acceptedChecksums[dbChecksum]
	if !dbOK {
		return false
	}
	_, fileOK := rule.acceptedChecksums[fileChecksum]
	return fileOK
}

func validateMigrationExecutionMode(name, content string) (bool, error) {
	normalizedName := strings.ToLower(strings.TrimSpace(name))
	nonTx := strings.HasSuffix(normalizedName, nonTransactionalMigrationSuffix)
	statements, err := splitSQLStatements(content)
	if err != nil {
		return false, err
	}

	if !nonTx {
		for _, statement := range statements {
			if statementContainsKeyword(statement, "CONCURRENTLY") {
				return false, errors.New("CONCURRENTLY statements must be placed in *_notx.sql migrations")
			}
		}
		return false, nil
	}

	for _, stmt := range statements {
		if !statementHasCode(stmt) {
			continue
		}

		_, isCreate, err := parseCreateConcurrentIndex(stmt)
		if err != nil {
			return false, err
		}
		if isCreate {
			continue
		}
		isDrop, err := parseDropConcurrentIndex(stmt)
		if err != nil {
			return false, err
		}
		if isDrop {
			continue
		}
		return false, errors.New("*_notx.sql currently only supports CREATE/DROP INDEX CONCURRENTLY statements")
	}

	return true, nil
}

func splitSQLStatements(content string) ([]string, error) {
	statements := make([]string, 0, strings.Count(content, ";")+1)
	statementStart := 0
	for i := 0; i < len(content); {
		switch {
		case strings.HasPrefix(content[i:], "--"):
			i = skipSQLLineComment(content, i)
		case strings.HasPrefix(content[i:], "/*"):
			next, err := skipSQLBlockComment(content, i)
			if err != nil {
				return nil, err
			}
			i = next
		case content[i] == '\'':
			next, err := skipSQLSingleQuoted(content, i, isEscapeStringPrefix(content, i))
			if err != nil {
				return nil, err
			}
			i = next
		case content[i] == '"':
			next, err := skipSQLDoubleQuoted(content, i)
			if err != nil {
				return nil, err
			}
			i = next
		case content[i] == '$':
			delimiter, ok := dollarQuoteDelimiterAt(content, i)
			if !ok {
				i++
				continue
			}
			closingOffset := strings.Index(content[i+len(delimiter):], delimiter)
			if closingOffset < 0 {
				return nil, fmt.Errorf("unterminated dollar-quoted string starting at byte %d", i)
			}
			i += len(delimiter) + closingOffset + len(delimiter)
		case content[i] == ';':
			if statement := content[statementStart:i]; strings.TrimSpace(statement) != "" {
				statements = append(statements, statement)
			}
			i++
			statementStart = i
		default:
			i++
		}
	}
	if statement := content[statementStart:]; strings.TrimSpace(statement) != "" {
		statements = append(statements, statement)
	}
	return statements, nil
}

func skipSQLLineComment(content string, start int) int {
	if newline := strings.IndexByte(content[start+2:], '\n'); newline >= 0 {
		return start + 2 + newline + 1
	}
	return len(content)
}

func skipSQLBlockComment(content string, start int) (int, error) {
	depth := 1
	for i := start + 2; i < len(content); {
		switch {
		case strings.HasPrefix(content[i:], "/*"):
			depth++
			i += 2
		case strings.HasPrefix(content[i:], "*/"):
			depth--
			i += 2
			if depth == 0 {
				return i, nil
			}
		default:
			i++
		}
	}
	return 0, fmt.Errorf("unterminated block comment starting at byte %d", start)
}

func skipSQLSingleQuoted(content string, start int, escapeBackslash bool) (int, error) {
	for i := start + 1; i < len(content); i++ {
		if escapeBackslash && content[i] == '\\' {
			i++
			continue
		}
		if content[i] != '\'' {
			continue
		}
		if i+1 < len(content) && content[i+1] == '\'' {
			i++
			continue
		}
		return i + 1, nil
	}
	return 0, fmt.Errorf("unterminated string literal starting at byte %d", start)
}

func skipSQLDoubleQuoted(content string, start int) (int, error) {
	for i := start + 1; i < len(content); i++ {
		if content[i] != '"' {
			continue
		}
		if i+1 < len(content) && content[i+1] == '"' {
			i++
			continue
		}
		return i + 1, nil
	}
	return 0, fmt.Errorf("unterminated quoted identifier starting at byte %d", start)
}

func isEscapeStringPrefix(content string, quote int) bool {
	return quote >= 1 && (content[quote-1] == 'e' || content[quote-1] == 'E') &&
		(quote == 1 || !isSQLIdentifierContinue(content[quote-2]))
}

func dollarQuoteDelimiterAt(content string, start int) (string, bool) {
	if start+1 >= len(content) {
		return "", false
	}
	if start > 0 && isSQLIdentifierContinue(content[start-1]) {
		return "", false
	}
	if content[start+1] == '$' {
		return "$$", true
	}
	if !isSQLIdentifierStart(content[start+1]) {
		return "", false
	}
	for i := start + 2; i < len(content); i++ {
		if content[i] == '$' {
			return content[start : i+1], true
		}
		if !isSQLIdentifierContinue(content[i]) || content[i] == '$' {
			return "", false
		}
	}
	return "", false
}

func isSQLIdentifierStart(ch byte) bool {
	return ch == '_' || ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z'
}

func isSQLIdentifierContinue(ch byte) bool {
	return isSQLIdentifierStart(ch) || ch >= '0' && ch <= '9' || ch == '$'
}

func isSQLWhitespace(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' || ch == '\f'
}

type sqlCursor struct {
	statement string
	pos       int
}

func (c *sqlCursor) skipSpaceAndComments() error {
	for c.pos < len(c.statement) {
		switch {
		case isSQLWhitespace(c.statement[c.pos]):
			c.pos++
		case strings.HasPrefix(c.statement[c.pos:], "--"):
			c.pos = skipSQLLineComment(c.statement, c.pos)
		case strings.HasPrefix(c.statement[c.pos:], "/*"):
			next, err := skipSQLBlockComment(c.statement, c.pos)
			if err != nil {
				return err
			}
			c.pos = next
		default:
			return nil
		}
	}
	return nil
}

func (c *sqlCursor) keyword(expected string) (bool, error) {
	if err := c.skipSpaceAndComments(); err != nil {
		return false, err
	}
	start := c.pos
	if start >= len(c.statement) || !isSQLIdentifierStart(c.statement[start]) {
		return false, nil
	}
	c.pos++
	for c.pos < len(c.statement) && isSQLIdentifierContinue(c.statement[c.pos]) {
		c.pos++
	}
	if !strings.EqualFold(c.statement[start:c.pos], expected) {
		c.pos = start
		return false, nil
	}
	return true, nil
}

func (c *sqlCursor) requireKeyword(expected string) error {
	ok, err := c.keyword(expected)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("expected %s", expected)
	}
	return nil
}

func (c *sqlCursor) identifier() (string, bool, error) {
	if err := c.skipSpaceAndComments(); err != nil {
		return "", false, err
	}
	if c.pos >= len(c.statement) {
		return "", false, nil
	}
	if c.pos+2 < len(c.statement) && (c.statement[c.pos] == 'u' || c.statement[c.pos] == 'U') &&
		c.statement[c.pos+1] == '&' && c.statement[c.pos+2] == '"' {
		return "", false, errors.New("unicode escaped index identifiers are not supported")
	}
	if c.statement[c.pos] == '"' {
		start := c.pos
		next, err := skipSQLDoubleQuoted(c.statement, start)
		if err != nil {
			return "", false, err
		}
		encoded := c.statement[start+1 : next-1]
		c.pos = next
		value := strings.ReplaceAll(encoded, `""`, `"`)
		if len(value) > postgresIdentifierMaxBytes {
			return "", false, fmt.Errorf("quoted identifier exceeds PostgreSQL's %d-byte limit", postgresIdentifierMaxBytes)
		}
		return value, true, nil
	}
	if !isSQLIdentifierStart(c.statement[c.pos]) {
		return "", false, nil
	}
	start := c.pos
	c.pos++
	for c.pos < len(c.statement) && isSQLIdentifierContinue(c.statement[c.pos]) {
		c.pos++
	}
	value := c.statement[start:c.pos]
	if len(value) > postgresIdentifierMaxBytes {
		return "", false, fmt.Errorf("unquoted identifier exceeds PostgreSQL's %d-byte limit", postgresIdentifierMaxBytes)
	}
	return strings.ToLower(value), true, nil
}

func (c *sqlCursor) qualifiedRelationName(defaultSchema string) (qualifiedRelationName, error) {
	first, ok, err := c.identifier()
	if err != nil {
		return qualifiedRelationName{}, err
	}
	if !ok {
		return qualifiedRelationName{}, errors.New("expected relation name")
	}
	if err := c.skipSpaceAndComments(); err != nil {
		return qualifiedRelationName{}, err
	}
	if c.pos >= len(c.statement) || c.statement[c.pos] != '.' {
		return qualifiedRelationName{schema: defaultSchema, name: first}, nil
	}
	c.pos++
	second, ok, err := c.identifier()
	if err != nil {
		return qualifiedRelationName{}, err
	}
	if !ok {
		return qualifiedRelationName{}, errors.New("expected relation name after schema qualifier")
	}
	return qualifiedRelationName{schema: first, name: second}, nil
}

func parseCreateConcurrentIndex(statement string) (concurrentIndexDefinition, bool, error) {
	cursor := sqlCursor{statement: statement}
	isCreate, err := cursor.keyword("CREATE")
	if err != nil || !isCreate {
		return concurrentIndexDefinition{}, false, err
	}
	_, err = cursor.keyword("UNIQUE")
	if err != nil {
		return concurrentIndexDefinition{}, false, err
	}
	for _, keyword := range []string{"INDEX", "CONCURRENTLY"} {
		if err := cursor.requireKeyword(keyword); err != nil {
			return concurrentIndexDefinition{}, false, fmt.Errorf("CREATE INDEX CONCURRENTLY IF NOT EXISTS: %w", err)
		}
	}
	ifNotExistsStart := cursor.pos
	for _, keyword := range []string{"IF", "NOT", "EXISTS"} {
		if err := cursor.requireKeyword(keyword); err != nil {
			return concurrentIndexDefinition{}, false, fmt.Errorf("CREATE INDEX CONCURRENTLY IF NOT EXISTS: %w", err)
		}
	}
	ifNotExistsEnd := cursor.pos
	indexName, ok, err := cursor.identifier()
	if err != nil {
		return concurrentIndexDefinition{}, false, fmt.Errorf("CREATE INDEX CONCURRENTLY IF NOT EXISTS: %w", err)
	}
	if !ok {
		return concurrentIndexDefinition{}, false, errors.New("CREATE INDEX CONCURRENTLY IF NOT EXISTS: expected index name")
	}
	if err := cursor.requireKeyword("ON"); err != nil {
		return concurrentIndexDefinition{}, false, fmt.Errorf("CREATE INDEX CONCURRENTLY IF NOT EXISTS: %w", err)
	}
	_, err = cursor.keyword("ONLY")
	if err != nil {
		return concurrentIndexDefinition{}, false, err
	}
	table, err := cursor.qualifiedRelationName("")
	if err != nil {
		return concurrentIndexDefinition{}, false, fmt.Errorf("CREATE INDEX CONCURRENTLY IF NOT EXISTS: target table: %w", err)
	}
	return concurrentIndexDefinition{
		name:            indexName,
		table:           table,
		strictStatement: statement[:ifNotExistsStart] + statement[ifNotExistsEnd:],
	}, true, nil
}

func parseDropConcurrentIndex(statement string) (bool, error) {
	cursor := sqlCursor{statement: statement}
	isDrop, err := cursor.keyword("DROP")
	if err != nil || !isDrop {
		return false, err
	}
	for _, keyword := range []string{"INDEX", "CONCURRENTLY", "IF", "EXISTS"} {
		if err := cursor.requireKeyword(keyword); err != nil {
			return false, fmt.Errorf("DROP INDEX CONCURRENTLY IF EXISTS: %w", err)
		}
	}
	if _, err := cursor.qualifiedRelationName("public"); err != nil {
		return false, fmt.Errorf("DROP INDEX CONCURRENTLY IF EXISTS: %w", err)
	}
	return true, nil
}

func statementHasCode(statement string) bool {
	cursor := sqlCursor{statement: statement}
	return cursor.skipSpaceAndComments() == nil && cursor.pos < len(statement)
}

func statementContainsKeyword(statement, keyword string) bool {
	for i := 0; i < len(statement); {
		switch {
		case isSQLWhitespace(statement[i]):
			i++
		case strings.HasPrefix(statement[i:], "--"):
			i = skipSQLLineComment(statement, i)
		case strings.HasPrefix(statement[i:], "/*"):
			next, err := skipSQLBlockComment(statement, i)
			if err != nil {
				return false
			}
			i = next
		case statement[i] == '\'':
			next, err := skipSQLSingleQuoted(statement, i, isEscapeStringPrefix(statement, i))
			if err != nil {
				return false
			}
			i = next
		case statement[i] == '"':
			next, err := skipSQLDoubleQuoted(statement, i)
			if err != nil {
				return false
			}
			i = next
		case statement[i] == '$':
			delimiter, ok := dollarQuoteDelimiterAt(statement, i)
			if !ok {
				i++
				continue
			}
			closingOffset := strings.Index(statement[i+len(delimiter):], delimiter)
			if closingOffset < 0 {
				return false
			}
			i += len(delimiter) + closingOffset + len(delimiter)
		case isSQLIdentifierStart(statement[i]):
			start := i
			i++
			for i < len(statement) && isSQLIdentifierContinue(statement[i]) {
				i++
			}
			if strings.EqualFold(statement[start:i], keyword) {
				return true
			}
		default:
			i++
		}
	}
	return false
}

// pgAdvisoryLock 获取 PostgreSQL Advisory Lock。
// Advisory Lock 是一种轻量级的锁机制，不与任何特定的数据库对象关联。
// 它非常适合用于应用层面的分布式锁场景，如迁移序列化。
type advisoryLockConnection interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func pgAdvisoryLock(ctx context.Context, db advisoryLockConnection) error {
	ticker := time.NewTicker(migrationsLockRetryInterval)
	defer ticker.Stop()

	for {
		var locked bool
		if err := db.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", migrationsAdvisoryLockID).Scan(&locked); err != nil {
			return fmt.Errorf("acquire migrations lock: %w", err)
		}
		if locked {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("acquire migrations lock: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

// pgAdvisoryUnlock 释放 PostgreSQL Advisory Lock。
// 必须在获取锁后确保释放，否则会阻塞其他实例的迁移操作。
func pgAdvisoryUnlock(ctx context.Context, db advisoryLockConnection) error {
	_, err := db.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", migrationsAdvisoryLockID)
	if err != nil {
		return fmt.Errorf("release migrations lock: %w", err)
	}
	return nil
}
