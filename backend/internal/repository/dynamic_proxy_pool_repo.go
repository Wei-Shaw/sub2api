package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/dynamicproxypool"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type dynamicProxyPoolRepository struct {
	client *dbent.Client
}

// NewDynamicProxyPoolRepository creates the repository.
func NewDynamicProxyPoolRepository(client *dbent.Client) service.DynamicProxyPoolRepository {
	return &dynamicProxyPoolRepository{client: client}
}

func (r *dynamicProxyPoolRepository) Create(ctx context.Context, m *service.DynamicProxyPool) error {
	client := clientFromContext(ctx, r.client)
	builder := client.DynamicProxyPool.Create().
		SetName(m.Name).
		SetEnabled(m.Enabled).
		SetSourceType(m.SourceType).
		SetExtractURL(m.ExtractURL).
		SetProtocol(m.Protocol).
		SetAuthMode(m.AuthMode).
		SetUsername(m.Username).
		SetPassword(m.Password).
		SetResponseFormat(m.ResponseFormat).
		SetLineSeparator(m.LineSeparator).
		SetIPFieldPath(m.IPFieldPath).
		SetPortFieldPath(m.PortFieldPath).
		SetRefreshIntervalSec(m.RefreshIntervalSec).
		SetIPDurationSec(m.IPDurationSec).
		SetExtractCount(m.ExtractCount).
		SetMinAlive(m.MinAlive).
		SetNamePrefix(m.NamePrefix).
		SetLastExtractStatus(m.LastExtractStatus).
		SetLastExtractError(m.LastExtractError).
		SetAliveCount(m.AliveCount)
	if m.SubscriptionID != nil {
		builder = builder.SetSubscriptionID(*m.SubscriptionID)
	}
	if m.LastExtractAt != nil {
		builder = builder.SetLastExtractAt(*m.LastExtractAt)
	}
	created, err := builder.Save(ctx)
	if err != nil {
		return fmt.Errorf("create dynamic proxy pool: %w", err)
	}
	*m = *entToDynamicProxyPool(created)
	return nil
}

func (r *dynamicProxyPoolRepository) GetByID(ctx context.Context, id int64) (*service.DynamicProxyPool, error) {
	client := clientFromContext(ctx, r.client)
	row, err := client.DynamicProxyPool.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get dynamic proxy pool: %w", err)
	}
	return entToDynamicProxyPool(row), nil
}

func (r *dynamicProxyPoolRepository) Update(ctx context.Context, m *service.DynamicProxyPool) error {
	client := clientFromContext(ctx, r.client)
	updater := client.DynamicProxyPool.UpdateOneID(m.ID).
		SetName(m.Name).
		SetEnabled(m.Enabled).
		SetSourceType(m.SourceType).
		SetExtractURL(m.ExtractURL).
		SetProtocol(m.Protocol).
		SetAuthMode(m.AuthMode).
		SetUsername(m.Username).
		SetPassword(m.Password).
		SetResponseFormat(m.ResponseFormat).
		SetLineSeparator(m.LineSeparator).
		SetIPFieldPath(m.IPFieldPath).
		SetPortFieldPath(m.PortFieldPath).
		SetRefreshIntervalSec(m.RefreshIntervalSec).
		SetIPDurationSec(m.IPDurationSec).
		SetExtractCount(m.ExtractCount).
		SetMinAlive(m.MinAlive).
		SetNamePrefix(m.NamePrefix).
		SetAliveCount(m.AliveCount).
		SetHealthCheckIntervalSec(m.HealthCheckIntervalSec)
	if m.SubscriptionID != nil {
		updater = updater.SetSubscriptionID(*m.SubscriptionID)
	} else {
		updater = updater.ClearSubscriptionID()
	}
	if m.LastExtractAt != nil {
		updater = updater.SetLastExtractAt(*m.LastExtractAt)
	} else {
		updater = updater.ClearLastExtractAt()
	}
	updated, err := updater.Save(ctx)
	if err != nil {
		return fmt.Errorf("update dynamic proxy pool: %w", err)
	}
	m.UpdatedAt = updated.UpdatedAt
	return nil
}

func (r *dynamicProxyPoolRepository) Delete(ctx context.Context, id int64) error {
	client := clientFromContext(ctx, r.client)
	if err := client.DynamicProxyPool.DeleteOneID(id).Exec(ctx); err != nil {
		return fmt.Errorf("delete dynamic proxy pool: %w", err)
	}
	return nil
}

func (r *dynamicProxyPoolRepository) List(ctx context.Context, params service.DynamicProxyPoolListParams) ([]*service.DynamicProxyPool, int64, error) {
	client := clientFromContext(ctx, r.client)
	q := client.DynamicProxyPool.Query()
	if params.Enabled != nil {
		q = q.Where(dynamicproxypool.EnabledEQ(*params.Enabled))
	}
	if s := strings.TrimSpace(params.Search); s != "" {
		q = q.Where(dynamicproxypool.Or(
			dynamicproxypool.NameContainsFold(s),
			dynamicproxypool.NamePrefixContainsFold(s),
		))
	}
	total, err := q.Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count dynamic proxy pools: %w", err)
	}
	pageSize := params.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	page := params.Page
	if page <= 0 {
		page = 1
	}
	rows, err := q.
		Order(dbent.Desc(dynamicproxypool.FieldID)).
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("list dynamic proxy pools: %w", err)
	}
	out := make([]*service.DynamicProxyPool, 0, len(rows))
	for _, row := range rows {
		out = append(out, entToDynamicProxyPool(row))
	}
	return out, int64(total), nil
}

func (r *dynamicProxyPoolRepository) ListEnabled(ctx context.Context) ([]*service.DynamicProxyPool, error) {
	client := clientFromContext(ctx, r.client)
	rows, err := client.DynamicProxyPool.Query().
		Where(dynamicproxypool.EnabledEQ(true)).
		Order(dbent.Asc(dynamicproxypool.FieldID)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list enabled dynamic proxy pools: %w", err)
	}
	out := make([]*service.DynamicProxyPool, 0, len(rows))
	for _, row := range rows {
		out = append(out, entToDynamicProxyPool(row))
	}
	return out, nil
}

func (r *dynamicProxyPoolRepository) ExistsNamePrefix(ctx context.Context, prefix string, excludeID int64) (bool, error) {
	client := clientFromContext(ctx, r.client)
	q := client.DynamicProxyPool.Query().Where(dynamicproxypool.NamePrefixEQ(prefix))
	if excludeID > 0 {
		q = q.Where(dynamicproxypool.IDNEQ(excludeID))
	}
	n, err := q.Count(ctx)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (r *dynamicProxyPoolRepository) UpdateExtractState(ctx context.Context, id int64, status, errMsg string, lastExtractAt *time.Time) error {
	client := clientFromContext(ctx, r.client)
	updater := client.DynamicProxyPool.UpdateOneID(id).
		SetLastExtractStatus(status).
		SetLastExtractError(errMsg)
	if lastExtractAt != nil {
		updater = updater.SetLastExtractAt(*lastExtractAt)
	}
	return updater.Exec(ctx)
}

func (r *dynamicProxyPoolRepository) UpdateAliveCount(ctx context.Context, id int64, count int) error {
	client := clientFromContext(ctx, r.client)
	return client.DynamicProxyPool.UpdateOneID(id).
		SetAliveCount(count).
		Exec(ctx)
}

func entToDynamicProxyPool(row *dbent.DynamicProxyPool) *service.DynamicProxyPool {
	if row == nil {
		return nil
	}
	return &service.DynamicProxyPool{
		ID:                 row.ID,
		Name:               row.Name,
		Enabled:            row.Enabled,
		SourceType:         row.SourceType,
		SubscriptionID:     row.SubscriptionID,
		ExtractURL:         row.ExtractURL,
		Protocol:           row.Protocol,
		AuthMode:           row.AuthMode,
		Username:           row.Username,
		Password:           row.Password,
		ResponseFormat:     row.ResponseFormat,
		LineSeparator:      row.LineSeparator,
		IPFieldPath:        row.IPFieldPath,
		PortFieldPath:      row.PortFieldPath,
		RefreshIntervalSec: row.RefreshIntervalSec,
		IPDurationSec:      row.IPDurationSec,
		ExtractCount:       row.ExtractCount,
		MinAlive:           row.MinAlive,
		NamePrefix:         row.NamePrefix,
		LastExtractAt:      row.LastExtractAt,
		LastExtractStatus:  row.LastExtractStatus,
		LastExtractError:   row.LastExtractError,
		AliveCount:         row.AliveCount,
			HealthCheckIntervalSec: row.HealthCheckIntervalSec,
		CreatedAt:          row.CreatedAt,
		UpdatedAt:          row.UpdatedAt,
	}
}
