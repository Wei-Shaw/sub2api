package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/lib/pq"
)

// GetGroupIDs returns the group IDs associated with a channel.
func (r *channelRepository) GetGroupIDs(ctx context.Context, channelID int64) ([]int64, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT group_id FROM channel_groups WHERE channel_id = $1 ORDER BY group_id`, channelID,
	)
	if err != nil {
		return nil, fmt.Errorf("get group ids: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan group id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate group ids: %w", err)
	}
	return ids, nil
}

func (r *channelRepository) SetGroupIDs(ctx context.Context, channelID int64, groupIDs []int64) error {
	return setGroupIDsTx(ctx, r.db, channelID, groupIDs)
}

func (r *channelRepository) GetChannelIDByGroupID(ctx context.Context, groupID int64) (int64, error) {
	var channelID int64
	err := r.db.QueryRowContext(ctx,
		`SELECT channel_id FROM channel_groups WHERE group_id = $1`, groupID,
	).Scan(&channelID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return channelID, err
}

func (r *channelRepository) GetGroupsInOtherChannels(ctx context.Context, channelID int64, groupIDs []int64) ([]int64, error) {
	if len(groupIDs) == 0 {
		return nil, nil
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT group_id FROM channel_groups WHERE group_id = ANY($1) AND channel_id != $2`,
		pq.Array(groupIDs), channelID,
	)
	if err != nil {
		return nil, fmt.Errorf("get groups in other channels: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var conflicting []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan conflicting group id: %w", err)
		}
		conflicting = append(conflicting, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate conflicting group ids: %w", err)
	}
	return conflicting, nil
}

// GetGroupPlatforms returns a map[group_id]platform for the given IDs.
func (r *channelRepository) GetGroupPlatforms(ctx context.Context, groupIDs []int64) (map[int64]string, error) {
	if len(groupIDs) == 0 {
		return make(map[int64]string), nil
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, platform FROM groups WHERE id = ANY($1)`,
		pq.Array(groupIDs),
	)
	if err != nil {
		return nil, fmt.Errorf("get group platforms: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	result := make(map[int64]string, len(groupIDs))
	for rows.Next() {
		var id int64
		var platform string
		if err := rows.Scan(&id, &platform); err != nil {
			return nil, fmt.Errorf("scan group platform: %w", err)
		}
		result[id] = platform
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate group platforms: %w", err)
	}
	return result, nil
}

func (r *channelRepository) ExistsByName(ctx context.Context, name string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM channels WHERE name = $1)`, name,
	).Scan(&exists)
	return exists, err
}

func (r *channelRepository) ExistsByNameExcluding(ctx context.Context, name string, excludeID int64) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM channels WHERE name = $1 AND id != $2)`, name, excludeID,
	).Scan(&exists)
	return exists, err
}

// --- JSON marshalling helpers ---

func marshalModelMapping(m map[string]map[string]string) ([]byte, error) {
	if len(m) == 0 {
		return []byte("{}"), nil
	}
	data, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("marshal model_mapping: %w", err)
	}
	return data, nil
}

func unmarshalModelMapping(data []byte) map[string]map[string]string {
	if len(data) == 0 {
		return nil
	}
	var m map[string]map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}
	return m
}

func marshalFeaturesConfig(m map[string]any) ([]byte, error) {
	if len(m) == 0 {
		return []byte("{}"), nil
	}
	data, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("marshal features_config: %w", err)
	}
	return data, nil
}

func unmarshalFeaturesConfig(data []byte) map[string]any {
	if len(data) == 0 {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}
	return m
}

// batchLoadGroupIDs loads group IDs for many channels in one round trip.
func (r *channelRepository) batchLoadGroupIDs(ctx context.Context, channelIDs []int64) (map[int64][]int64, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT channel_id, group_id FROM channel_groups
		 WHERE channel_id = ANY($1) ORDER BY channel_id, group_id`,
		pq.Array(channelIDs),
	)
	if err != nil {
		return nil, fmt.Errorf("batch load group ids: %w", err)
	}
	defer func() { _ = rows.Close() }()

	groupMap := make(map[int64][]int64, len(channelIDs))
	for rows.Next() {
		var channelID, groupID int64
		if err := rows.Scan(&channelID, &groupID); err != nil {
			return nil, fmt.Errorf("scan group id: %w", err)
		}
		groupMap[channelID] = append(groupMap[channelID], groupID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate group ids: %w", err)
	}
	return groupMap, nil
}
