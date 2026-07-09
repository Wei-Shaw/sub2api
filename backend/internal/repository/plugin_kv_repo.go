package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/pluginkv"
	"github.com/Wei-Shaw/sub2api/internal/pluginhost"
	"github.com/Wei-Shaw/sub2api/internal/pluginkit"
)

// PluginKVRepository 是 pluginhost.KVStore 的持久化实现：
// plugin_kvs 表承载外部插件的 KV 能力，(plugin_id, key) 唯一键
// 实现命名空间硬隔离，卸载时按 plugin_id 整体清空。
type PluginKVRepository struct {
	client *ent.Client
}

var _ pluginhost.KVStore = (*PluginKVRepository)(nil)

// NewPluginKVRepository 创建插件 KV 仓储。
func NewPluginKVRepository(client *ent.Client) *PluginKVRepository {
	return &PluginKVRepository{client: client}
}

// Get 读取一个键；不存在返回 pluginhost.ErrKVNotFound。
func (r *PluginKVRepository) Get(ctx context.Context, pluginID pluginkit.ID, key string) (string, error) {
	row, err := r.client.PluginKV.Query().
		Where(pluginkv.PluginIDEQ(string(pluginID)), pluginkv.KeyEQ(key)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return "", fmt.Errorf("%w: %s/%s", pluginhost.ErrKVNotFound, pluginID, key)
		}
		return "", fmt.Errorf("plugin kv repo: get %s/%s: %w", pluginID, key, err)
	}
	return row.Value, nil
}

// Set 写入或覆盖一个键（(plugin_id, key) upsert）。
// 冲突时显式只更新 value/updated_at：created_at 是 Immutable 的首写时间，
// 不依赖 UpdateNewValues 对 create 默认值的条件忽略行为。
func (r *PluginKVRepository) Set(ctx context.Context, pluginID pluginkit.ID, key, value string) error {
	if err := r.client.PluginKV.Create().
		SetPluginID(string(pluginID)).
		SetKey(key).
		SetValue(value).
		SetUpdatedAt(time.Now()).
		OnConflictColumns(pluginkv.FieldPluginID, pluginkv.FieldKey).
		Update(func(u *ent.PluginKVUpsert) {
			u.SetValue(value)
			u.SetUpdatedAt(time.Now())
		}).
		Exec(ctx); err != nil {
		return fmt.Errorf("plugin kv repo: set %s/%s: %w", pluginID, key, err)
	}
	return nil
}

// Delete 删除一个键（不存在时为 no-op）。
func (r *PluginKVRepository) Delete(ctx context.Context, pluginID pluginkit.ID, key string) error {
	if _, err := r.client.PluginKV.Delete().
		Where(pluginkv.PluginIDEQ(string(pluginID)), pluginkv.KeyEQ(key)).
		Exec(ctx); err != nil {
		return fmt.Errorf("plugin kv repo: delete %s/%s: %w", pluginID, key, err)
	}
	return nil
}

// Keys 列出命名空间内的键（prefix 为空时列全部），按键稳定排序。
// 前缀匹配在应用侧完成：能力面键集小（单插件自用），
// 避免 LIKE 通配符转义在方言间的差异。
func (r *PluginKVRepository) Keys(ctx context.Context, pluginID pluginkit.ID, prefix string) ([]string, error) {
	keys, err := r.client.PluginKV.Query().
		Where(pluginkv.PluginIDEQ(string(pluginID))).
		Order(ent.Asc(pluginkv.FieldKey)).
		Select(pluginkv.FieldKey).
		Strings(ctx)
	if err != nil {
		return nil, fmt.Errorf("plugin kv repo: keys %s: %w", pluginID, err)
	}
	if prefix == "" {
		return keys, nil
	}
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		if strings.HasPrefix(k, prefix) {
			out = append(out, k)
		}
	}
	return out, nil
}

// DeleteAll 清空整个命名空间（卸载插件时调用）。
func (r *PluginKVRepository) DeleteAll(ctx context.Context, pluginID pluginkit.ID) error {
	if _, err := r.client.PluginKV.Delete().
		Where(pluginkv.PluginIDEQ(string(pluginID))).
		Exec(ctx); err != nil {
		return fmt.Errorf("plugin kv repo: delete all %s: %w", pluginID, err)
	}
	return nil
}
