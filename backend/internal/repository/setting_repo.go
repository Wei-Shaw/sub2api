package repository

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/setting"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

const extraConcurrencySettingsUpdateFenceSettingKey = "__internal_extra_concurrency_settings_update_fence"

type settingRepository struct {
	client *ent.Client
}

func NewSettingRepository(client *ent.Client) service.SettingRepository {
	return &settingRepository{client: client}
}

func (r *settingRepository) Get(ctx context.Context, key string) (*service.Setting, error) {
	m, err := r.client.Setting.Query().Where(setting.KeyEQ(key)).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, service.ErrSettingNotFound
		}
		return nil, err
	}
	return &service.Setting{
		ID:        m.ID,
		Key:       m.Key,
		Value:     m.Value,
		UpdatedAt: m.UpdatedAt,
	}, nil
}

func (r *settingRepository) GetValue(ctx context.Context, key string) (string, error) {
	setting, err := r.Get(ctx, key)
	if err != nil {
		return "", err
	}
	return setting.Value, nil
}

func (r *settingRepository) Set(ctx context.Context, key, value string) error {
	now := time.Now()
	return r.client.Setting.
		Create().
		SetKey(key).
		SetValue(value).
		SetUpdatedAt(now).
		OnConflictColumns(setting.FieldKey).
		UpdateNewValues().
		Exec(ctx)
}

func (r *settingRepository) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	if len(keys) == 0 {
		return map[string]string{}, nil
	}
	settings, err := r.client.Setting.Query().Where(setting.KeyIn(keys...)).All(ctx)
	if err != nil {
		return nil, err
	}

	result := make(map[string]string)
	for _, s := range settings {
		result[s.Key] = s.Value
	}
	return result, nil
}

func (r *settingRepository) SetMultiple(ctx context.Context, settings map[string]string) error {
	if len(settings) == 0 {
		return nil
	}

	now := time.Now()
	builders := make([]*ent.SettingCreate, 0, len(settings))
	for key, value := range settings {
		builders = append(builders, r.client.Setting.Create().SetKey(key).SetValue(value).SetUpdatedAt(now))
	}
	return r.client.Setting.
		CreateBulk(builders...).
		OnConflictColumns(setting.FieldKey).
		UpdateNewValues().
		Exec(ctx)
}

func (r *settingRepository) ReserveSettingUpdateFence(ctx context.Context) (int64, error) {
	rows, err := r.client.QueryContext(ctx, `
INSERT INTO "settings" ("key", "value", "updated_at")
VALUES ($1, '1', $2)
ON CONFLICT ("key") DO UPDATE
SET "value" = ("settings"."value"::bigint + 1)::text,
	"updated_at" = EXCLUDED."updated_at"
RETURNING "value"::bigint
`, extraConcurrencySettingsUpdateFenceSettingKey, time.Now())
	if err != nil {
		return 0, fmt.Errorf("reserve settings update fence: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return 0, fmt.Errorf("reserve settings update fence: %w", err)
		}
		return 0, errors.New("reserve settings update fence: no fence returned")
	}
	var fence int64
	if err := rows.Scan(&fence); err != nil {
		return 0, fmt.Errorf("reserve settings update fence: %w", err)
	}
	return fence, nil
}

func (r *settingRepository) SetMultipleFenced(ctx context.Context, settings map[string]string, fence int64) error {
	if len(settings) == 0 {
		return nil
	}
	if fence <= 0 {
		return fmt.Errorf("set multiple fenced: invalid fence %d", fence)
	}
	if _, exists := settings[extraConcurrencySettingsUpdateFenceSettingKey]; exists {
		return errors.New("set multiple fenced: settings include internal fence key")
	}

	keys := make([]string, 0, len(settings))
	for key := range settings {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	now := time.Now()
	args := make([]any, 0, 3+2*len(keys))
	args = append(args, extraConcurrencySettingsUpdateFenceSettingKey, fence, now)
	var values strings.Builder
	for i, key := range keys {
		if i > 0 {
			values.WriteString(", ")
		}
		keyArg := 4 + 2*i
		valueArg := keyArg + 1
		fmt.Fprintf(&values, "($%d, $%d)", keyArg, valueArg)
		args = append(args, key, settings[key])
	}

	query := fmt.Sprintf(`
WITH active_fence AS (
	SELECT 1
	FROM "settings"
	WHERE "key" = $1 AND "value"::bigint = $2
	FOR UPDATE
)
INSERT INTO "settings" ("key", "value", "updated_at")
SELECT incoming."key", incoming."value", $3
FROM (VALUES %s) AS incoming("key", "value")
WHERE EXISTS (SELECT 1 FROM active_fence)
ON CONFLICT ("key") DO UPDATE
SET "value" = EXCLUDED."value", "updated_at" = EXCLUDED."updated_at"
`, values.String())
	result, err := r.client.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("set multiple fenced: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("set multiple fenced rows affected: %w", err)
	}
	if rows == 0 {
		return service.ErrStaleSettingUpdateFence
	}
	if rows != int64(len(settings)) {
		return fmt.Errorf("set multiple fenced: affected %d of %d settings", rows, len(settings))
	}
	return nil
}

func (r *settingRepository) GetAll(ctx context.Context) (map[string]string, error) {
	settings, err := r.client.Setting.Query().All(ctx)
	if err != nil {
		return nil, err
	}

	result := make(map[string]string)
	for _, s := range settings {
		result[s.Key] = s.Value
	}
	return result, nil
}

func (r *settingRepository) Delete(ctx context.Context, key string) error {
	_, err := r.client.Setting.Delete().Where(setting.KeyEQ(key)).Exec(ctx)
	return err
}
