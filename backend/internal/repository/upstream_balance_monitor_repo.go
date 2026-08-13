package repository

import (
	"context"
	"fmt"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/upstreambalancemonitor"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type upstreamBalanceMonitorRepository struct{ client *dbent.Client }

func NewUpstreamBalanceMonitorRepository(client *dbent.Client) service.UpstreamBalanceMonitorRepository {
	return &upstreamBalanceMonitorRepository{client: client}
}

func (r *upstreamBalanceMonitorRepository) Create(ctx context.Context, m *service.UpstreamBalanceMonitor) error {
	created, err := r.client.UpstreamBalanceMonitor.Create().
		SetName(m.Name).SetType(upstreambalancemonitor.Type(m.Type)).SetBaseURL(m.BaseURL).
		SetAPIKeyEncrypted(m.APIKey).SetEnabled(m.Enabled).SetDisplayOrder(m.DisplayOrder).
		SetProbeIntervalMinutes(m.ProbeIntervalMinutes).SetLowBalanceThresholdUsd(m.LowBalanceThresholdUSD).
		SetLastProbeStatus(m.LastProbeStatus).SetSnapshotData(nonNilSnapshot(m.SnapshotData)).
		SetNillableNextProbeAt(m.NextProbeAt).Save(ctx)
	if err != nil {
		return fmt.Errorf("create upstream balance monitor: %w", err)
	}
	m.ID, m.CreatedAt, m.UpdatedAt = created.ID, created.CreatedAt, created.UpdatedAt
	return nil
}

func (r *upstreamBalanceMonitorRepository) GetByID(ctx context.Context, id int64) (*service.UpstreamBalanceMonitor, error) {
	row, err := r.client.UpstreamBalanceMonitor.Get(ctx, id)
	if dbent.IsNotFound(err) {
		return nil, service.ErrUpstreamBalanceMonitorNotFound
	}
	if err != nil {
		return nil, err
	}
	return upstreamBalanceEntToService(row), nil
}

func (r *upstreamBalanceMonitorRepository) Update(ctx context.Context, m *service.UpstreamBalanceMonitor) error {
	updated, err := r.client.UpstreamBalanceMonitor.UpdateOneID(m.ID).
		SetName(m.Name).SetType(upstreambalancemonitor.Type(m.Type)).SetBaseURL(m.BaseURL).
		SetAPIKeyEncrypted(m.APIKey).SetEnabled(m.Enabled).SetDisplayOrder(m.DisplayOrder).
		SetProbeIntervalMinutes(m.ProbeIntervalMinutes).SetLowBalanceThresholdUsd(m.LowBalanceThresholdUSD).
		SetNillableNextProbeAt(m.NextProbeAt).Save(ctx)
	if dbent.IsNotFound(err) {
		return service.ErrUpstreamBalanceMonitorNotFound
	}
	if err != nil {
		return fmt.Errorf("update upstream balance monitor: %w", err)
	}
	m.UpdatedAt = updated.UpdatedAt
	return nil
}

func (r *upstreamBalanceMonitorRepository) Delete(ctx context.Context, id int64) error {
	err := r.client.UpstreamBalanceMonitor.DeleteOneID(id).Exec(ctx)
	if dbent.IsNotFound(err) {
		return service.ErrUpstreamBalanceMonitorNotFound
	}
	return err
}

func (r *upstreamBalanceMonitorRepository) List(ctx context.Context) ([]*service.UpstreamBalanceMonitor, error) {
	rows, err := r.client.UpstreamBalanceMonitor.Query().
		Order(dbent.Asc(upstreambalancemonitor.FieldDisplayOrder), dbent.Asc(upstreambalancemonitor.FieldID)).All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*service.UpstreamBalanceMonitor, 0, len(rows))
	for _, row := range rows {
		out = append(out, upstreamBalanceEntToService(row))
	}
	return out, nil
}

func (r *upstreamBalanceMonitorRepository) ListDue(ctx context.Context, now time.Time, limit int) ([]*service.UpstreamBalanceMonitor, error) {
	rows, err := r.client.UpstreamBalanceMonitor.Query().Where(
		upstreambalancemonitor.EnabledEQ(true),
		upstreambalancemonitor.Or(upstreambalancemonitor.NextProbeAtIsNil(), upstreambalancemonitor.NextProbeAtLTE(now)),
	).Order(dbent.Asc(upstreambalancemonitor.FieldNextProbeAt)).Limit(limit).All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*service.UpstreamBalanceMonitor, 0, len(rows))
	for _, row := range rows {
		out = append(out, upstreamBalanceEntToService(row))
	}
	return out, nil
}

func (r *upstreamBalanceMonitorRepository) UpdateProbeResult(ctx context.Context, m *service.UpstreamBalanceMonitor) error {
	u := r.client.UpstreamBalanceMonitor.UpdateOneID(m.ID).
		SetNillableLastProbeAt(m.LastProbeAt).SetLastProbeStatus(m.LastProbeStatus).
		SetSnapshotData(nonNilSnapshot(m.SnapshotData)).SetNillableNextProbeAt(m.NextProbeAt).
		SetFailureCount(m.FailureCount)
	if m.LastProbeError != nil {
		u = u.SetLastProbeError(*m.LastProbeError)
	} else {
		u = u.ClearLastProbeError()
	}
	_, err := u.Save(ctx)
	if dbent.IsNotFound(err) {
		return service.ErrUpstreamBalanceMonitorNotFound
	}
	return err
}

func upstreamBalanceEntToService(row *dbent.UpstreamBalanceMonitor) *service.UpstreamBalanceMonitor {
	if row == nil {
		return nil
	}
	return &service.UpstreamBalanceMonitor{ID: row.ID, Name: row.Name, Type: string(row.Type), BaseURL: row.BaseURL,
		APIKey: row.APIKeyEncrypted, Enabled: row.Enabled, DisplayOrder: row.DisplayOrder,
		ProbeIntervalMinutes: row.ProbeIntervalMinutes, LowBalanceThresholdUSD: row.LowBalanceThresholdUsd,
		LastProbeAt: row.LastProbeAt, LastProbeStatus: row.LastProbeStatus, LastProbeError: row.LastProbeError,
		SnapshotData: nonNilSnapshot(row.SnapshotData), NextProbeAt: row.NextProbeAt, FailureCount: row.FailureCount,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}

func nonNilSnapshot(v map[string]any) map[string]any {
	if v == nil {
		return map[string]any{}
	}
	return v
}

var _ service.UpstreamBalanceMonitorRepository = (*upstreamBalanceMonitorRepository)(nil)
