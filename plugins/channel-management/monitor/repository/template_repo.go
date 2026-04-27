package monitorrepository

import (
	"context"
	"database/sql"

	monitorservice "github.com/Wei-Shaw/sub2api/plugins/channel-management/monitor/service"
)

// channelMonitorTemplateRepository implements the request-template data
// access interface. Like channelMonitorRepository above, every method is a
// build-time stub returning ErrNotPorted; the real raw-SQL bodies (sourced
// from upstream commit 09fd83ab backend/internal/repository/
// channel_monitor_template_repo.go) land in a follow-up commit.
type channelMonitorTemplateRepository struct {
	db *sql.DB
}

// NewChannelMonitorTemplateRepository wires the template repo on top of the
// SDK DB handle.
func NewChannelMonitorTemplateRepository(db *sql.DB) monitorservice.ChannelMonitorRequestTemplateRepository {
	return &channelMonitorTemplateRepository{db: db}
}

func (r *channelMonitorTemplateRepository) Create(ctx context.Context, t *monitorservice.ChannelMonitorRequestTemplate) error {
	return ErrNotPorted
}

func (r *channelMonitorTemplateRepository) GetByID(ctx context.Context, id int64) (*monitorservice.ChannelMonitorRequestTemplate, error) {
	return nil, ErrNotPorted
}

func (r *channelMonitorTemplateRepository) Update(ctx context.Context, t *monitorservice.ChannelMonitorRequestTemplate) error {
	return ErrNotPorted
}

func (r *channelMonitorTemplateRepository) Delete(ctx context.Context, id int64) error {
	return ErrNotPorted
}

func (r *channelMonitorTemplateRepository) List(ctx context.Context, params monitorservice.ChannelMonitorRequestTemplateListParams) ([]*monitorservice.ChannelMonitorRequestTemplate, error) {
	return nil, ErrNotPorted
}

func (r *channelMonitorTemplateRepository) ApplyToMonitors(ctx context.Context, id int64, monitorIDs []int64) (int64, error) {
	return 0, ErrNotPorted
}

func (r *channelMonitorTemplateRepository) CountAssociatedMonitors(ctx context.Context, id int64) (int64, error) {
	return 0, ErrNotPorted
}

func (r *channelMonitorTemplateRepository) ListAssociatedMonitors(ctx context.Context, id int64) ([]*monitorservice.AssociatedMonitorBrief, error) {
	return nil, ErrNotPorted
}
