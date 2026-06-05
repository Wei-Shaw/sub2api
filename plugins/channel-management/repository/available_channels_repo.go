// Package repository — available-channels queries.
//
// These helpers back the user-facing "available channels" view (W8 of the V5
// plugin migration). They run on the same SDK-provided *sql.DB as the rest of
// the plugin so all reads go through the host's connection pool.
package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/plugins/channel-management/service"
)

// availableChannelsRepository is a thin wrapper around the SDK *sql.DB that
// satisfies service.AvailableChannelsRepository. It is constructed alongside
// the existing ChannelRepository on plugin start.
type availableChannelsRepository struct {
	db *sql.DB
}

// NewAvailableChannelsRepository wires the available-channels read repository
// on top of the SDK's DB handle.
func NewAvailableChannelsRepository(db *sql.DB) service.AvailableChannelsRepository {
	return &availableChannelsRepository{db: db}
}

// ListActiveGroups returns the active groups visible to the available-channels
// view. The query mirrors host repository.groupRepo.ListActive — only the
// columns the view needs are selected so changes to wide group fields do not
// touch this read path.
func (r *availableChannelsRepository) ListActiveGroups(ctx context.Context) ([]service.AvailableGroupRef, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, platform, COALESCE(subscription_type, ''), COALESCE(rate_multiplier, 1), is_exclusive
		 FROM groups
		 WHERE status = $1 AND deleted_at IS NULL
		 ORDER BY sort_order, name`,
		service.StatusActive,
	)
	if err != nil {
		return nil, fmt.Errorf("query active groups: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]service.AvailableGroupRef, 0)
	for rows.Next() {
		var ref service.AvailableGroupRef
		if err := rows.Scan(
			&ref.ID,
			&ref.Name,
			&ref.Platform,
			&ref.SubscriptionType,
			&ref.RateMultiplier,
			&ref.IsExclusive,
		); err != nil {
			return nil, fmt.Errorf("scan group: %w", err)
		}
		out = append(out, ref)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate groups: %w", err)
	}
	return out, nil
}

// ListUserAllowedGroupIDs returns group ids the user is explicitly allowed to
// bind through the user_allowed_groups join table. Used to gate exclusive
// (non-public) standard groups.
func (r *availableChannelsRepository) ListUserAllowedGroupIDs(ctx context.Context, userID int64) ([]int64, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT group_id FROM user_allowed_groups WHERE user_id = $1`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("query allowed groups: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan allowed group id: %w", err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate allowed groups: %w", err)
	}
	return out, nil
}

// ListUserActiveSubscriptionGroupIDs returns group ids the user currently
// holds a non-expired active subscription for. Used to gate subscription-type
// groups.
func (r *availableChannelsRepository) ListUserActiveSubscriptionGroupIDs(ctx context.Context, userID int64) ([]int64, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT group_id FROM user_subscriptions
		 WHERE user_id = $1 AND status = $2 AND expires_at > $3`,
		userID, service.SubscriptionStatusActive, time.Now(),
	)
	if err != nil {
		return nil, fmt.Errorf("query active subscriptions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan subscription group id: %w", err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate subscriptions: %w", err)
	}
	return out, nil
}
