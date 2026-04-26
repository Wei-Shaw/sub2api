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

// ApplyToMonitors 把模板当前配置覆盖到 monitorIDs 列表里的关联监控。
// WHERE 双重过滤：template_id = id AND id IN (monitorIDs)，防止用户传了未关联本模板的 id
// 就被覆盖。走 ent UpdateMany 保留 hooks。
func (r *channelMonitorRequestTemplateRepository) ApplyToMonitors(ctx context.Context, id int64, monitorIDs []int64) (int64, error) {
	if len(monitorIDs) == 0 {
		return 0, nil
	}
	client := clientFromContext(ctx, r.client)
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
		).
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
			Enabled:  row.Enabled,
		})
	}
	return out, nil
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
