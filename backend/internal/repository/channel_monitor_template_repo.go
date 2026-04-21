package repository

import (
	"context"
	"database/sql"
	"fmt"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/channelmonitor"
	"github.com/Wei-Shaw/sub2api/ent/channelmonitorrequesttemplate"
	"github.com/Wei-Shaw/sub2api/internal/service"
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
	rows, err := q.
		Order(dbent.Asc(channelmonitorrequesttemplate.FieldProvider), dbent.Asc(channelmonitorrequesttemplate.FieldName)).
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

// ApplyToMonitors 把模板当前配置批量覆盖到 template_id = id 的监控上。
//
// 用一条 UPDATE 完成：extra_headers / body_override_mode / body_override 都覆盖。
// 走 ent 的 UpdateMany 保证走 ent hooks；走原生 SQL 也可以但 ent jsonb 序列化更省心。
func (r *channelMonitorRequestTemplateRepository) ApplyToMonitors(ctx context.Context, id int64) (int64, error) {
	client := clientFromContext(ctx, r.client)
	tpl, err := client.ChannelMonitorRequestTemplate.Query().
		Where(channelmonitorrequesttemplate.IDEQ(id)).
		Only(ctx)
	if err != nil {
		return 0, translatePersistenceError(err, service.ErrChannelMonitorTemplateNotFound, nil)
	}

	updater := client.ChannelMonitor.Update().
		Where(channelmonitor.TemplateIDEQ(id)).
		SetExtraHeaders(emptyHeadersIfNilRepo(tpl.ExtraHeaders)).
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
	return int64(affected), nil
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

// ---------- helpers ----------

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
		Description:      row.Description,
		ExtraHeaders:     headers,
		BodyOverrideMode: row.BodyOverrideMode,
		BodyOverride:     row.BodyOverride,
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
	}
}
