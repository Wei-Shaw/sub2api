package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/channelmonitor"
	"github.com/Wei-Shaw/sub2api/ent/channelmonitorrequesttemplate"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

// channelMonitorRequestTemplateRepository 实现 service.ChannelMonitorRequestTemplateRepository。
// 与 channelMonitorRepository 分开一个文件，职责清晰。
type channelMonitorRequestTemplateRepository struct {
	client *dbent.Client
	db     *sql.DB
}

// NewChannelMonitorRequestTemplateRepository 创建模板仓储实例。
func NewChannelMonitorRequestTemplateRepository(client *dbent.Client, db *sql.DB) service.ChannelMonitorRequestTemplateRepository {
	return &channelMonitorRequestTemplateRepository{client: client, db: db}
}

// ---------- CRUD ----------

func (r *channelMonitorRequestTemplateRepository) Create(ctx context.Context, t *service.ChannelMonitorRequestTemplate) error {
	client := clientFromContext(ctx, r.client)
	builder := client.ChannelMonitorRequestTemplate.Create().
		SetName(t.Name).
		SetProvider(channelmonitorrequesttemplate.Provider(t.Provider)).
		SetAPIMode(defaultAPIModeRepo(t.APIMode)).
		SetDescription(t.Description).
		SetExtraHeaders(emptyHeadersIfNilRepo(t.ExtraHeaders)).
		SetBodyOverrideMode(defaultBodyModeRepo(t.BodyOverrideMode))
	if t.BodyOverride != nil {
		builder = builder.SetBodyOverride(t.BodyOverride)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrChannelMonitorTemplateNotFound, nil)
	}
	t.ID = created.ID
	t.CreatedAt = created.CreatedAt
	t.UpdatedAt = created.UpdatedAt
	return nil
}

func (r *channelMonitorRequestTemplateRepository) GetByID(ctx context.Context, id int64) (*service.ChannelMonitorRequestTemplate, error) {
	row, err := r.client.ChannelMonitorRequestTemplate.Query().
		Where(channelmonitorrequesttemplate.IDEQ(id)).
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrChannelMonitorTemplateNotFound, nil)
	}
	return entToServiceTemplate(row), nil
}

func (r *channelMonitorRequestTemplateRepository) Update(ctx context.Context, t *service.ChannelMonitorRequestTemplate) error {
	client := clientFromContext(ctx, r.client)
	updater := client.ChannelMonitorRequestTemplate.UpdateOneID(t.ID).
		SetName(t.Name).
		SetAPIMode(defaultAPIModeRepo(t.APIMode)).
		SetDescription(t.Description).
		SetExtraHeaders(emptyHeadersIfNilRepo(t.ExtraHeaders)).
		SetBodyOverrideMode(defaultBodyModeRepo(t.BodyOverrideMode))
	if t.BodyOverride != nil {
		updater = updater.SetBodyOverride(t.BodyOverride)
	} else {
		updater = updater.ClearBodyOverride()
	}
	updated, err := updater.Save(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrChannelMonitorTemplateNotFound, nil)
	}
	t.UpdatedAt = updated.UpdatedAt
	return nil
}

func (r *channelMonitorRequestTemplateRepository) Delete(ctx context.Context, id int64) error {
	client := clientFromContext(ctx, r.client)
	if err := client.ChannelMonitorRequestTemplate.DeleteOneID(id).Exec(ctx); err != nil {
		return translatePersistenceError(err, service.ErrChannelMonitorTemplateNotFound, nil)
	}
	return nil
}

func (r *channelMonitorRequestTemplateRepository) List(ctx context.Context, params service.ChannelMonitorRequestTemplateListParams) ([]*service.ChannelMonitorRequestTemplate, error) {
	q := r.client.ChannelMonitorRequestTemplate.Query()
	if params.Provider != "" {
		q = q.Where(channelmonitorrequesttemplate.ProviderEQ(channelmonitorrequesttemplate.Provider(params.Provider)))
	}
	if params.APIMode != "" {
		q = q.Where(channelmonitorrequesttemplate.APIModeEQ(defaultAPIModeRepo(params.APIMode)))
	}
	rows, err := q.
		Order(dbent.Asc(channelmonitorrequesttemplate.FieldProvider), dbent.Asc(channelmonitorrequesttemplate.FieldAPIMode), dbent.Asc(channelmonitorrequesttemplate.FieldName)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list monitor templates: %w", err)
	}
	out := make([]*service.ChannelMonitorRequestTemplate, 0, len(rows))
	for _, row := range rows {
		out = append(out, entToServiceTemplate(row))
	}
	return out, nil
}

// ApplyToMonitors 把模板当前配置覆盖到 monitorIDs 列表里的关联监控。
// WHERE 双重过滤：template_id = id AND id IN (monitorIDs)，防止用户传了未关联本模板的 id
// 就被覆盖。模板字段通过 ent UpdateMany 更新以保留 hooks；extra_headers 在同一事务中
// 单独合并，以保留仅用于幂等恢复、绝不会发往上游的内部 operation ID。
func (r *channelMonitorRequestTemplateRepository) ApplyToMonitors(ctx context.Context, id int64, monitorIDs []int64) (int64, error) {
	if len(monitorIDs) == 0 {
		return 0, nil
	}
	if tx := dbent.TxFromContext(ctx); tx != nil {
		return r.applyToMonitorsWithClient(ctx, tx.Client(), id, monitorIDs)
	}

	tx, err := r.client.Tx(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin apply template transaction: %w", err)
	}
	txCtx := dbent.NewTxContext(ctx, tx)
	affected, err := r.applyToMonitorsWithClient(txCtx, tx.Client(), id, monitorIDs)
	if err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit apply template transaction: %w", err)
	}
	return affected, nil
}

func (r *channelMonitorRequestTemplateRepository) applyToMonitorsWithClient(
	ctx context.Context,
	client *dbent.Client,
	id int64,
	monitorIDs []int64,
) (int64, error) {
	tpl, err := client.ChannelMonitorRequestTemplate.Query().
		Where(channelmonitorrequesttemplate.IDEQ(id)).
		Only(ctx)
	if err != nil {
		return 0, translatePersistenceError(err, service.ErrChannelMonitorTemplateNotFound, nil)
	}

	updater := client.ChannelMonitor.Update().
		Where(
			channelmonitor.TemplateIDEQ(id),
			channelmonitor.IDIn(monitorIDs...),
			channelmonitor.ProviderEQ(channelmonitor.Provider(tpl.Provider)),
			channelmonitor.APIModeEQ(defaultAPIModeRepo(tpl.APIMode)),
		).
		SetAPIMode(defaultAPIModeRepo(tpl.APIMode)).
		SetBodyOverrideMode(defaultBodyModeRepo(tpl.BodyOverrideMode))
	if tpl.BodyOverride != nil {
		updater = updater.SetBodyOverride(tpl.BodyOverride)
	} else {
		updater = updater.ClearBodyOverride()
	}

	affected, err := updater.Save(ctx)
	if err != nil {
		return 0, fmt.Errorf("apply template to monitors: %w", err)
	}
	// MySQL 的 UPDATE RowsAffected 统计「值发生变化」的行（PG 统计「匹配」的行）。
	// 目标行全部字段与模板值相同、且 TimeMixin 写入的 updated_at 恰与原值同毫秒时，
	// MySQL 返回 0 并提前退出，跳过后续 extra_headers 合并（PG 会继续）。此处用
	// 相同谓词计数还原 matched 语义；PG 分支不受影响。
	if IsMySQL() && affected == 0 {
		matched, err := client.ChannelMonitor.Query().
			Where(
				channelmonitor.TemplateIDEQ(id),
				channelmonitor.IDIn(monitorIDs...),
				channelmonitor.ProviderEQ(channelmonitor.Provider(tpl.Provider)),
				channelmonitor.APIModeEQ(defaultAPIModeRepo(tpl.APIMode)),
			).
			Count(ctx)
		if err != nil {
			return 0, fmt.Errorf("count monitors for template apply: %w", err)
		}
		affected = matched
	}
	if affected == 0 {
		return 0, nil
	}

	templateHeaders := channelMonitorHeadersForPersistence(&service.ChannelMonitor{
		ExtraHeaders: tpl.ExtraHeaders,
	})
	templateHeadersJSON, err := json.Marshal(templateHeaders)
	if err != nil {
		return 0, fmt.Errorf("marshal template headers: %w", err)
	}
	var result sql.Result
	if IsMySQL() {
		result, err = execApplyTemplateHeadersMySQL(ctx, client, id, monitorIDs, tpl, templateHeadersJSON)
	} else {
		result, err = client.ExecContext(ctx, `
			UPDATE channel_monitors
			SET extra_headers = $1::jsonb || CASE
				WHEN COALESCE(extra_headers, '{}'::jsonb) ? ($2::text)
				THEN jsonb_build_object($2::text, COALESCE(extra_headers, '{}'::jsonb) -> ($2::text))
				ELSE '{}'::jsonb
			END
			WHERE template_id = $3
			  AND id = ANY($4)
			  AND provider = $5
			  AND api_mode = $6
		`, string(templateHeadersJSON), service.ChannelMonitorDuplicateOperationIDMetadataKey,
			id, pq.Array(monitorIDs), string(tpl.Provider), defaultAPIModeRepo(tpl.APIMode))
	}
	if err != nil {
		return 0, fmt.Errorf("apply template headers to monitors: %w", err)
	}
	headersAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count applied template headers: %w", err)
	}
	if headersAffected != int64(affected) {
		return 0, fmt.Errorf("apply template headers: affected %d rows, expected %d", headersAffected, affected)
	}
	return headersAffected, nil
}

// CountAssociatedMonitors 统计关联监控数（UI 展示「N 个配置」用）。
func (r *channelMonitorRequestTemplateRepository) CountAssociatedMonitors(ctx context.Context, id int64) (int64, error) {
	count, err := r.client.ChannelMonitor.Query().
		Where(channelmonitor.TemplateIDEQ(id)).
		Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("count monitors for template %d: %w", id, err)
	}
	return int64(count), nil
}

// ListAssociatedMonitors 列出模板关联的所有监控简略字段。
// ORDER BY name 稳定输出方便前端展示。
func (r *channelMonitorRequestTemplateRepository) ListAssociatedMonitors(ctx context.Context, id int64) ([]*service.AssociatedMonitorBrief, error) {
	rows, err := r.client.ChannelMonitor.Query().
		Where(channelmonitor.TemplateIDEQ(id)).
		Order(dbent.Asc(channelmonitor.FieldName)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list associated monitors for template %d: %w", id, err)
	}
	out := make([]*service.AssociatedMonitorBrief, 0, len(rows))
	for _, row := range rows {
		out = append(out, &service.AssociatedMonitorBrief{
			ID:       row.ID,
			Name:     row.Name,
			Provider: string(row.Provider),
			APIMode:  defaultAPIModeRepo(row.APIMode),
			Enabled:  row.Enabled,
		})
	}
	return out, nil
}

// ---------- helpers ----------

// execApplyTemplateHeadersMySQL 是 applyToMonitorsWithClient 内 extra_headers 合并
// UPDATE 的 MySQL 实现，语义与 PG 版对齐：模板 headers 覆盖目标行，但保留行内既有的
// 内部 operation ID 键（幂等恢复用，绝不下发上游）。映射（见 DIALECT_MYSQL_GUIDE.md）：
//   - `$1::jsonb || CASE ...` → JSON_MERGE_PATCH(CAST(? AS JSON), CASE ...)：
//     右侧仅含单个标量字符串键值（headers 为 map[string]string），与 PG `||` 浅合并等价；
//   - `COALESCE(extra_headers,'{}') ? ($2::text)` → JSON_CONTAINS_PATH(..., 'one', path)；
//   - `jsonb_build_object($2::text, extra_headers -> ($2::text))` →
//     JSON_OBJECT(key, JSON_UNQUOTE(JSON_EXTRACT(...)))（值恒为字符串，反引号后语义一致）；
//   - `'{}'::jsonb` → CAST('{}' AS JSON)；
//   - `id = ANY($4)` → IN 展开。
//
// operation ID 键为服务层常量（仅字母/冒号/下划线），以带引号 JSON path 字面量嵌入
// （`$."key"`，键名内无 `"` 与 `\`，无需转义）。
// REVIEW: 额外写 updated_at = NOW(3)：go-sql-driver 默认 RowsAffected 返回 changed 而非
// matched 行数；上方 ent 更新经 TimeMixin 必然改写 updated_at（affected=matched），本语句
// 若仅写 extra_headers 且合并结果与原值相同会少计行数，触发 headersAffected != affected
// 误报。再写一次 NOW(3)（与 ent 写入的 Go 时钟值不同）保证 changed=matched。PG 版不写。
func execApplyTemplateHeadersMySQL(ctx context.Context, client *dbent.Client, id int64, monitorIDs []int64, tpl *dbent.ChannelMonitorRequestTemplate, templateHeadersJSON []byte) (sql.Result, error) {
	opKey := service.ChannelMonitorDuplicateOperationIDMetadataKey
	opPath := `'$."` + opKey + `"'`
	query := fmt.Sprintf(`
		UPDATE channel_monitors
		SET extra_headers = JSON_MERGE_PATCH(
		    CAST(? AS JSON),
		    CASE
		        WHEN JSON_CONTAINS_PATH(COALESCE(extra_headers, CAST('{}' AS JSON)), 'one', %[1]s)
		        THEN JSON_OBJECT('%[2]s', JSON_UNQUOTE(JSON_EXTRACT(COALESCE(extra_headers, CAST('{}' AS JSON)), %[1]s)))
		        ELSE CAST('{}' AS JSON)
		    END
		),
		    updated_at = NOW(3)
		WHERE template_id = ?
		  AND id IN (%[3]s)
		  AND provider = ?
		  AND api_mode = ?
	`, opPath, opKey, Placeholders(len(monitorIDs)))
	args := make([]any, 0, len(monitorIDs)+4)
	args = append(args, string(templateHeadersJSON), id)
	for _, mid := range monitorIDs {
		args = append(args, mid)
	}
	args = append(args, string(tpl.Provider), defaultAPIModeRepo(tpl.APIMode))
	return client.ExecContext(ctx, query, args...)
}

func entToServiceTemplate(row *dbent.ChannelMonitorRequestTemplate) *service.ChannelMonitorRequestTemplate {
	if row == nil {
		return nil
	}
	headers := row.ExtraHeaders
	if headers == nil {
		headers = map[string]string{}
	}
	return &service.ChannelMonitorRequestTemplate{
		ID:               row.ID,
		Name:             row.Name,
		Provider:         string(row.Provider),
		APIMode:          defaultAPIModeRepo(row.APIMode),
		Description:      row.Description,
		ExtraHeaders:     headers,
		BodyOverrideMode: row.BodyOverrideMode,
		BodyOverride:     row.BodyOverride,
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
	}
}
