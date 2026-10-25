package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// MySQLDumper implements service.DBDumper using mysqldump/mysql client tools.
//
// 与 PgDumper 对等的语义：
//   - Dump：持 GET_LOCK 备份锁后执行 `mysqldump --single-transaction` 流式导出；
//   - Restore：通过 `mysql` 客户端流式导入（内容为 mysqldump 产物，
//     自带 DROP TABLE IF EXISTS，与 pg_dump --clean --if-exists 对齐）。
//
// 部署要求：二进制所在环境需安装 MySQL 8 客户端工具（mysqldump/mysql）。
type MySQLDumper struct {
	cfg            *config.DatabaseConfig
	db             *sql.DB
	commandContext func(context.Context, string, ...string) *exec.Cmd
}

// NewMySQLDumper creates a new MySQLDumper.
func NewMySQLDumper(cfg *config.Config, db *sql.DB) service.DBDumper {
	return &MySQLDumper{
		cfg:            &cfg.Database,
		db:             db,
		commandContext: exec.CommandContext,
	}
}

// mysqlBackupLockName 是备份期间的 MySQL 命名锁，与迁移锁（sub2api_migrations）
// 键空间隔离但同样由专用连接持有。
const mysqlBackupLockName = "sub2api_backup"

func acquireMySQLBackupLock(ctx context.Context, db *sql.DB) (*sql.Conn, func() error, error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("acquire backup lock connection: %w", err)
	}
	if err := mysqlGetNamedLock(ctx, conn, mysqlBackupLockName); err != nil {
		// GET_LOCK 失败后连接所有权存疑（连接级命名锁），弃用该连接。
		discardSQLConnection(conn)
		return nil, nil, fmt.Errorf("acquire backup lock: %w", err)
	}
	release := func() error {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := mysqlReleaseNamedLock(unlockCtx, conn, mysqlBackupLockName); err != nil {
			discardSQLConnection(conn)
			return fmt.Errorf("release backup lock: %w", err)
		}
		if err := conn.Close(); err != nil {
			return fmt.Errorf("close backup lock connection: %w", err)
		}
		return nil
	}
	return conn, release, nil
}

// Dump executes mysqldump and returns a streaming reader of the output.
func (d *MySQLDumper) Dump(ctx context.Context) (io.ReadCloser, error) {
	if d.db == nil {
		return nil, errors.New("acquire backup lock: nil sql db")
	}
	_, releaseLock, err := acquireMySQLBackupLock(ctx, d.db)
	if err != nil {
		return nil, err
	}

	args := []string{
		"--host=" + d.cfg.Host,
		"--port=" + fmt.Sprintf("%d", d.cfg.Port),
		"--user=" + d.cfg.User,
		d.cfg.DBName,
		// --single-transaction：InnoDB 一致性快照，不锁表（与 pg_dump 的快照语义对齐）。
		"--single-transaction",
		// 导出包含 DROP TABLE IF EXISTS，恢复时可覆盖已有对象（对齐 pg_dump --clean --if-exists）。
		"--add-drop-table",
		// 触发器随表导出；不导出 DEFINER（避免恢复到无对应用户的实例时失败）。
		"--triggers",
		"--skip-definer",
		// 不导出事件/存储过程（项目 schema 未使用；避免 DEFINER 依赖）。
		"--skip-events",
		"--routines=false",
	}

	commandContext := d.commandContext
	if commandContext == nil {
		commandContext = exec.CommandContext
	}
	cmd := commandContext(ctx, "mysqldump", args...)
	if d.cfg.Password != "" {
		cmd.Env = append(cmd.Environ(), "MYSQL_PWD="+d.cfg.Password)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, errors.Join(fmt.Errorf("create stdout pipe: %w", err), releaseLock())
	}

	if err := cmd.Start(); err != nil {
		_ = stdout.Close()
		return nil, errors.Join(fmt.Errorf("start mysqldump: %w", err), releaseLock())
	}

	return &cmdReadCloser{ReadCloser: stdout, cmd: cmd, release: releaseLock}, nil
}

// Restore executes the mysql client to restore from a streaming reader.
func (d *MySQLDumper) Restore(ctx context.Context, data io.Reader) error {
	args := []string{
		"--host=" + d.cfg.Host,
		"--port=" + fmt.Sprintf("%d", d.cfg.Port),
		"--user=" + d.cfg.User,
		d.cfg.DBName,
	}

	cmd := exec.CommandContext(ctx, "mysql", args...)
	if d.cfg.Password != "" {
		cmd.Env = append(cmd.Environ(), "MYSQL_PWD="+d.cfg.Password)
	}

	cmd.Stdin = data

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, string(output))
	}
	return nil
}

// ProvideDBDumper 按配置的数据库驱动返回对应的备份器。
func ProvideDBDumper(cfg *config.Config, db *sql.DB) service.DBDumper {
	if cfg.Database.IsMySQL() {
		return NewMySQLDumper(cfg, db)
	}
	return NewPgDumper(cfg, db)
}
