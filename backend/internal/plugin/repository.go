package plugin

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// pluginsTableDDL 定义 plugins 表的 DDL。
// 该表与正式的 SQL 迁移保持兼容:正式部署时由独立迁移文件创建,这里仅作为
// 测试场景下的兜底,避免在没有迁移环境时 repository 无法工作。
//
// uninstalled_at 由 P13/C-1 软卸载特性引入: NULL 代表插件处于活动状态
// (installed/disabled/enabled), 非空时间戳代表软卸载 — 数据保留, 可通过
// MarkInstalled 恢复。
const pluginsTableDDL = `
CREATE TABLE IF NOT EXISTS plugins (
	name           TEXT PRIMARY KEY,
	display_name   TEXT NOT NULL DEFAULT '',
	description    TEXT NOT NULL DEFAULT '',
	version        TEXT NOT NULL DEFAULT '',
	enabled        BOOLEAN NOT NULL DEFAULT FALSE,
	sort_order     INTEGER NOT NULL DEFAULT 0,
	config         JSONB NOT NULL DEFAULT '{}'::jsonb,
	installed_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	uninstalled_at TIMESTAMPTZ NULL
);
`

// ErrPluginNotFound 插件记录未找到,供上层判断是否需要 Create。
var ErrPluginNotFound = errors.New("plugin not found")

// PluginRecord 是 plugins 表的一行,使用裸 SQL 操作避免引入 Ent 依赖。
//
// UninstalledAt 由 P13/C-1 软卸载特性引入: nil 代表插件处于活动状态,
// 非 nil 代表软卸载 — 数据 (plugin_settings, plugin_migrations 等) 保留,
// 可通过 MarkInstalled 撤回。除 admin "已卸载" 列表外, list 默认过滤掉
// uninstalled_at 非空的行。
type PluginRecord struct {
	Name          string
	DisplayName   string
	Description   string
	Version       string
	Enabled       bool
	SortOrder     int
	Config        map[string]string
	InstalledAt   time.Time
	UpdatedAt     time.Time
	UninstalledAt *time.Time
}

// PluginRepository 封装 plugins 表的 CRUD。
type PluginRepository struct {
	db *sql.DB
}

// NewPluginRepository 构造仓储实例。db 不能为 nil。
func NewPluginRepository(db *sql.DB) *PluginRepository {
	return &PluginRepository{db: db}
}

// EnsureSchema 确保 plugins 表存在;主要用于单元测试或首次启动时的兜底。
// 生产环境应该通过 SQL 迁移文件管理建表语句。
func (r *PluginRepository) EnsureSchema(ctx context.Context) error {
	if r.db == nil {
		return errors.New("nil db")
	}
	if _, err := r.db.ExecContext(ctx, pluginsTableDDL); err != nil {
		return fmt.Errorf("ensure plugins table: %w", err)
	}
	return nil
}

// List 返回所有活动 (uninstalled_at IS NULL) 的插件记录, 按 sort_order, name 升序。
//
// 软卸载的插件默认隐藏 — 它们的数据仍在数据库中可恢复, 但不应出现在常规
// 列表 / sidebar / 路由中。需要查看软卸载的插件请使用 ListAll。
func (r *PluginRepository) List(ctx context.Context) ([]PluginRecord, error) {
	return r.list(ctx, false)
}

// ListAll 返回所有插件记录 (含软卸载) , 按 sort_order, name 升序。
// 仅供 admin "已卸载列表" 视图使用 — 普通调用方应使用 List。
func (r *PluginRepository) ListAll(ctx context.Context) ([]PluginRecord, error) {
	return r.list(ctx, true)
}

func (r *PluginRepository) list(ctx context.Context, includeUninstalled bool) ([]PluginRecord, error) {
	const baseQuery = `
		SELECT name, display_name, description, version, enabled, sort_order, config, installed_at, updated_at, uninstalled_at
		FROM plugins
	`
	q := baseQuery
	if !includeUninstalled {
		q += ` WHERE uninstalled_at IS NULL`
	}
	q += ` ORDER BY sort_order ASC, name ASC`

	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("query plugins: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]PluginRecord, 0)
	for rows.Next() {
		rec, err := scanPluginRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate plugins: %w", err)
	}
	return out, nil
}

// Get 按 name 主键查询活动 (uninstalled_at IS NULL) 插件。未找到返回 ErrPluginNotFound。
//
// 软卸载的插件不会通过 Get 返回 — manager / handler 视它们为 "不存在", 这与
// list 过滤保持一致。需要查看软卸载记录请使用 GetIncludingUninstalled。
func (r *PluginRepository) Get(ctx context.Context, name string) (*PluginRecord, error) {
	return r.get(ctx, name, false)
}

// GetIncludingUninstalled 按 name 查询任意状态的插件记录 (含软卸载)。
// Install / Uninstall 等生命周期操作内部使用; 普通调用方应使用 Get。
func (r *PluginRepository) GetIncludingUninstalled(ctx context.Context, name string) (*PluginRecord, error) {
	return r.get(ctx, name, true)
}

func (r *PluginRepository) get(ctx context.Context, name string, includeUninstalled bool) (*PluginRecord, error) {
	const baseQuery = `
		SELECT name, display_name, description, version, enabled, sort_order, config, installed_at, updated_at, uninstalled_at
		FROM plugins WHERE name = $1
	`
	q := baseQuery
	if !includeUninstalled {
		q += ` AND uninstalled_at IS NULL`
	}
	row := r.db.QueryRowContext(ctx, q, name)
	rec, err := scanPluginRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrPluginNotFound
		}
		return nil, err
	}
	return &rec, nil
}

// Create 插入一条新插件记录。
func (r *PluginRepository) Create(ctx context.Context, rec *PluginRecord) error {
	if rec == nil {
		return errors.New("nil plugin record")
	}
	if rec.Name == "" {
		return errors.New("plugin name is required")
	}
	// DB 层兜底: 即便 admin handler / discovery 校验都失效, 这里挡住非法
	// 名字, 防止它落进 plugins 表 (PRIMARY KEY) 后被 EnablePlugin 接管。
	if !IsValidPluginName(rec.Name) {
		return ErrInvalidPluginName
	}
	cfgJSON, err := marshalConfig(rec.Config)
	if err != nil {
		return err
	}
	const q = `
		INSERT INTO plugins (name, display_name, description, version, enabled, sort_order, config, installed_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())
	`
	if _, err := r.db.ExecContext(ctx, q,
		rec.Name, rec.DisplayName, rec.Description, rec.Version,
		rec.Enabled, rec.SortOrder, cfgJSON,
	); err != nil {
		return fmt.Errorf("insert plugin %s: %w", rec.Name, err)
	}
	return nil
}

// Update 更新已有记录,name 不可修改。
func (r *PluginRepository) Update(ctx context.Context, rec *PluginRecord) error {
	if rec == nil {
		return errors.New("nil plugin record")
	}
	if rec.Name == "" {
		return errors.New("plugin name is required")
	}
	cfgJSON, err := marshalConfig(rec.Config)
	if err != nil {
		return err
	}
	const q = `
		UPDATE plugins
		SET display_name = $2,
		    description  = $3,
		    version      = $4,
		    enabled      = $5,
		    sort_order   = $6,
		    config       = $7,
		    updated_at   = NOW()
		WHERE name = $1
	`
	res, err := r.db.ExecContext(ctx, q,
		rec.Name, rec.DisplayName, rec.Description, rec.Version,
		rec.Enabled, rec.SortOrder, cfgJSON,
	)
	if err != nil {
		return fmt.Errorf("update plugin %s: %w", rec.Name, err)
	}
	rows, err := res.RowsAffected()
	if err == nil && rows == 0 {
		return ErrPluginNotFound
	}
	return nil
}

// UpdateConfig 仅覆写 config 字段,不影响 enabled / sort_order 等其他字段。
//
// 配置以 JSONB 形式整体替换:调用方负责传入完整的最终配置(merge 语义不在这里实现,
// 避免 admin API 与 manager 状态不一致)。
func (r *PluginRepository) UpdateConfig(ctx context.Context, name string, cfg map[string]string) error {
	cfgJSON, err := marshalConfig(cfg)
	if err != nil {
		return err
	}
	const q = `UPDATE plugins SET config = $2, updated_at = NOW() WHERE name = $1`
	res, err := r.db.ExecContext(ctx, q, name, cfgJSON)
	if err != nil {
		return fmt.Errorf("update plugin config %s: %w", name, err)
	}
	rows, err := res.RowsAffected()
	if err == nil && rows == 0 {
		return ErrPluginNotFound
	}
	return nil
}

// SetEnabled 仅切换启用状态,避免读取-修改-写回带来的竞态。
func (r *PluginRepository) SetEnabled(ctx context.Context, name string, enabled bool) error {
	const q = `UPDATE plugins SET enabled = $2, updated_at = NOW() WHERE name = $1`
	res, err := r.db.ExecContext(ctx, q, name, enabled)
	if err != nil {
		return fmt.Errorf("set enabled %s: %w", name, err)
	}
	rows, err := res.RowsAffected()
	if err == nil && rows == 0 {
		return ErrPluginNotFound
	}
	return nil
}

// MarkUninstalled 把插件标记为软卸载 (uninstalled_at = NOW(), enabled = false)。
//
// 不会删除任何数据 — plugin_settings / plugin_settings_schemas / plugin_migrations
// 全部保留, 可通过 MarkInstalled 撤回。在 manager.Uninstall 内部调用; 如果
// 当前 enabled, 调用方需要先关闭子进程后再调用。未找到返回 ErrPluginNotFound;
// 已经处于软卸载状态视为幂等无操作 (RowsAffected=0 但不返回错误)。
//
// P13/C-1: 软卸载是可撤回的; 硬卸载 (清数据) 留给后续 C-2。
func (r *PluginRepository) MarkUninstalled(ctx context.Context, name string) error {
	const q = `
		UPDATE plugins
		SET uninstalled_at = NOW(),
		    enabled        = FALSE,
		    updated_at     = NOW()
		WHERE name = $1 AND uninstalled_at IS NULL
	`
	res, err := r.db.ExecContext(ctx, q, name)
	if err != nil {
		return fmt.Errorf("mark uninstalled %s: %w", name, err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return nil
	}
	if rows == 0 {
		// 区分: 行根本不存在 vs 已经软卸载。前者上层 (Uninstall) 视为 NotFound,
		// 后者视为幂等。
		exists, _ := r.exists(ctx, name)
		if !exists {
			return ErrPluginNotFound
		}
	}
	return nil
}

// MarkInstalled 撤销软卸载 (uninstalled_at = NULL); enabled 字段保持原值, 调用方
// 自行决定是否后续 enable 启动进程。
//
// 幂等: 在已经活动 (uninstalled_at IS NULL) 的插件上调用是 no-op。未找到 (无任何
// 行) 返回 ErrPluginNotFound。
func (r *PluginRepository) MarkInstalled(ctx context.Context, name string) error {
	const q = `
		UPDATE plugins
		SET uninstalled_at = NULL,
		    updated_at     = NOW()
		WHERE name = $1
	`
	res, err := r.db.ExecContext(ctx, q, name)
	if err != nil {
		return fmt.Errorf("mark installed %s: %w", name, err)
	}
	rows, err := res.RowsAffected()
	if err == nil && rows == 0 {
		return ErrPluginNotFound
	}
	return nil
}

// IsUninstalled 当且仅当插件存在且 uninstalled_at IS NOT NULL 时返回 true。
// 不存在 / 活动状态 / 查询错误一律返回 false。
func (r *PluginRepository) IsUninstalled(ctx context.Context, name string) bool {
	const q = `SELECT uninstalled_at FROM plugins WHERE name = $1`
	var ts sql.NullTime
	if err := r.db.QueryRowContext(ctx, q, name).Scan(&ts); err != nil {
		return false
	}
	return ts.Valid
}

// exists 是 MarkUninstalled 内部用于区分 NotFound 与 already-uninstalled 的辅助。
func (r *PluginRepository) exists(ctx context.Context, name string) (bool, error) {
	const q = `SELECT 1 FROM plugins WHERE name = $1`
	var x int
	err := r.db.QueryRowContext(ctx, q, name).Scan(&x)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// Delete 删除插件记录;通常仅用于卸载流程。
func (r *PluginRepository) Delete(ctx context.Context, name string) error {
	const q = `DELETE FROM plugins WHERE name = $1`
	res, err := r.db.ExecContext(ctx, q, name)
	if err != nil {
		return fmt.Errorf("delete plugin %s: %w", name, err)
	}
	rows, err := res.RowsAffected()
	if err == nil && rows == 0 {
		return ErrPluginNotFound
	}
	return nil
}

// rowScanner 用于复用 *sql.Row 与 *sql.Rows 的扫描逻辑。
type rowScanner interface {
	Scan(dest ...any) error
}

func scanPluginRow(s rowScanner) (PluginRecord, error) {
	var (
		rec      PluginRecord
		cfgRaw   []byte
		uninstAt sql.NullTime
	)
	if err := s.Scan(
		&rec.Name, &rec.DisplayName, &rec.Description, &rec.Version,
		&rec.Enabled, &rec.SortOrder, &cfgRaw,
		&rec.InstalledAt, &rec.UpdatedAt, &uninstAt,
	); err != nil {
		return PluginRecord{}, err
	}
	cfg, err := unmarshalConfig(cfgRaw)
	if err != nil {
		return PluginRecord{}, fmt.Errorf("decode config for %s: %w", rec.Name, err)
	}
	rec.Config = cfg
	if uninstAt.Valid {
		t := uninstAt.Time
		rec.UninstalledAt = &t
	}
	return rec, nil
}

func marshalConfig(cfg map[string]string) ([]byte, error) {
	if cfg == nil {
		return []byte(`{}`), nil
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal plugin config: %w", err)
	}
	return b, nil
}

func unmarshalConfig(raw []byte) (map[string]string, error) {
	out := make(map[string]string)
	if len(raw) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}
