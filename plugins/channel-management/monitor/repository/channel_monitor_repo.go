// Package monitorrepository implements the channel-monitor data access layer
// on top of the SDK-provided *sql.DB. The host plugin module does not ship
// its own ent client, so all queries are written against PostgreSQL via raw
// SQL (in line with V5-CURATE D5 — "no plugin ent module in V5").
//
// The current file is a build-time stub: every method returns ErrNotPorted
// so the plugin can wire its handlers, manifests, and migrations end-to-end
// while the real query bodies (~750 lines, mostly the same as the upstream
// implementation in commit 09fd83ab) are ported in subsequent commits.
//
// Porting plan:
//  1. Replace each ent.Builder call with the equivalent INSERT / SELECT /
//     UPDATE statement using the repository's *sql.DB and pq array helpers.
//  2. Re-introduce the aggregation queries (ListLatestForMonitorIDs,
//     ComputeAvailabilityForMonitors, ListRecentHistoryForMonitors) directly
//     — they were already raw SQL in the upstream code, so no rewrite is
//     needed apart from changing the package name and import paths.
//  3. Translate the rollup maintenance queries (UpsertDailyRollupsFor,
//     DeleteRollupsBefore, LoadAggregationWatermark, UpdateAggregationWatermark)
//     verbatim — they were also raw SQL upstream.
package monitorrepository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	monitorservice "github.com/Wei-Shaw/sub2api/plugins/channel-management/monitor/service"
)

// ErrNotPorted is the placeholder error every stub method returns until the
// real port lands. It is wrapped (not aliased) so the higher-level service
// can disambiguate "feature not yet implemented" from genuine database
// failures using errors.Is.
var ErrNotPorted = errors.New("channel-monitor repository: method not yet ported from upstream commit 09fd83ab")

// channelMonitorRepository is the channel-monitor data access implementation.
// db is the SDK-provided handle whose driver proxies queries through gRPC
// back to the host's connection pool.
type channelMonitorRepository struct {
	db *sql.DB
}

// NewChannelMonitorRepository wires the repo on top of the SDK DB handle.
// It returns the package-level interface so callers can swap the stub for
// the real implementation transparently once the port completes.
func NewChannelMonitorRepository(db *sql.DB) monitorservice.ChannelMonitorRepository {
	return &channelMonitorRepository{db: db}
}

// ---------- CRUD ----------

func (r *channelMonitorRepository) Create(ctx context.Context, m *monitorservice.ChannelMonitor) error {
	return ErrNotPorted
}

func (r *channelMonitorRepository) GetByID(ctx context.Context, id int64) (*monitorservice.ChannelMonitor, error) {
	return nil, ErrNotPorted
}

func (r *channelMonitorRepository) Update(ctx context.Context, m *monitorservice.ChannelMonitor) error {
	return ErrNotPorted
}

func (r *channelMonitorRepository) Delete(ctx context.Context, id int64) error {
	return ErrNotPorted
}

func (r *channelMonitorRepository) List(ctx context.Context, params monitorservice.ChannelMonitorListParams) ([]*monitorservice.ChannelMonitor, int64, error) {
	return nil, 0, ErrNotPorted
}

// ---------- Scheduler helpers ----------

func (r *channelMonitorRepository) ListEnabled(ctx context.Context) ([]*monitorservice.ChannelMonitor, error) {
	return nil, ErrNotPorted
}

func (r *channelMonitorRepository) MarkChecked(ctx context.Context, id int64, checkedAt time.Time) error {
	return ErrNotPorted
}

func (r *channelMonitorRepository) InsertHistoryBatch(ctx context.Context, rows []*monitorservice.ChannelMonitorHistoryRow) error {
	return ErrNotPorted
}

func (r *channelMonitorRepository) DeleteHistoryBefore(ctx context.Context, before time.Time) (int64, error) {
	return 0, ErrNotPorted
}

// ---------- History ----------

func (r *channelMonitorRepository) ListHistory(ctx context.Context, monitorID int64, model string, limit int) ([]*monitorservice.ChannelMonitorHistoryEntry, error) {
	return nil, ErrNotPorted
}

// ---------- Per-monitor aggregations ----------

func (r *channelMonitorRepository) ListLatestPerModel(ctx context.Context, monitorID int64) ([]*monitorservice.ChannelMonitorLatest, error) {
	return nil, ErrNotPorted
}

func (r *channelMonitorRepository) ComputeAvailability(ctx context.Context, monitorID int64, windowDays int) ([]*monitorservice.ChannelMonitorAvailability, error) {
	return nil, ErrNotPorted
}

// ---------- Batch aggregations (N+1 elimination) ----------

func (r *channelMonitorRepository) ListLatestForMonitorIDs(ctx context.Context, ids []int64) (map[int64][]*monitorservice.ChannelMonitorLatest, error) {
	return nil, ErrNotPorted
}

func (r *channelMonitorRepository) ComputeAvailabilityForMonitors(ctx context.Context, ids []int64, windowDays int) (map[int64][]*monitorservice.ChannelMonitorAvailability, error) {
	return nil, ErrNotPorted
}

func (r *channelMonitorRepository) ListRecentHistoryForMonitors(ctx context.Context, ids []int64, primaryModels map[int64]string, perMonitorLimit int) (map[int64][]*monitorservice.ChannelMonitorHistoryEntry, error) {
	return nil, ErrNotPorted
}

// ---------- Daily rollup maintenance ----------

func (r *channelMonitorRepository) UpsertDailyRollupsFor(ctx context.Context, targetDate time.Time) (int64, error) {
	return 0, ErrNotPorted
}

func (r *channelMonitorRepository) DeleteRollupsBefore(ctx context.Context, beforeDate time.Time) (int64, error) {
	return 0, ErrNotPorted
}

func (r *channelMonitorRepository) LoadAggregationWatermark(ctx context.Context) (*time.Time, error) {
	return nil, ErrNotPorted
}

func (r *channelMonitorRepository) UpdateAggregationWatermark(ctx context.Context, date time.Time) error {
	return ErrNotPorted
}
