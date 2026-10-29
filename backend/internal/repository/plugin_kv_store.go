package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

// pluginKVKeyPrefix 是所有插件通用键值存储在 Redis 中的根前缀。版本段 v1 便于未来
// 变更内部键结构时平滑迁移。
const pluginKVKeyPrefix = "plugin:kv:v1:"

// pluginKVScanCount 是 SCAN 每轮的 COUNT 提示，兼顾单轮开销与往返次数。
const pluginKVScanCount = 200

type pluginKVStore struct {
	rdb *redis.Client
}

// NewPluginKVStore 返回 Redis 支撑的通用插件键值存储。使用 Redis 而非进程内存，使得
// 插件状态可跨副本、跨重启共享，这正是有状态插件所需的通用能力。
func NewPluginKVStore(rdb *redis.Client) service.PluginKVStore {
	return &pluginKVStore{rdb: rdb}
}

// namespacePrefix 拼出某插件某命名空间的内部键前缀。pluginKey / namespace 均已由宿主
// 服务端按 [A-Za-z0-9._-] 校验，不含 ':' 或 glob 元字符，键结构因此无歧义。
func (s *pluginKVStore) namespacePrefix(pluginKey, namespace string) string {
	return pluginKVKeyPrefix + pluginKey + ":" + namespace + ":"
}

func (s *pluginKVStore) fullKey(pluginKey, namespace, key string) string {
	return s.namespacePrefix(pluginKey, namespace) + key
}

func (s *pluginKVStore) Get(ctx context.Context, pluginKey, namespace, key string) ([]byte, bool, error) {
	value, err := s.rdb.Get(ctx, s.fullKey(pluginKey, namespace, key)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("get plugin kv: %w", err)
	}
	return value, true, nil
}

func (s *pluginKVStore) Set(ctx context.Context, pluginKey, namespace, key string, value []byte, ttl time.Duration) error {
	// go-redis 将 ttl==0 视为不过期，正好对应“持久保存”语义。
	if err := s.rdb.Set(ctx, s.fullKey(pluginKey, namespace, key), value, ttl).Err(); err != nil {
		return fmt.Errorf("set plugin kv: %w", err)
	}
	return nil
}

func (s *pluginKVStore) Delete(ctx context.Context, pluginKey, namespace, key string) error {
	if err := s.rdb.Del(ctx, s.fullKey(pluginKey, namespace, key)).Err(); err != nil {
		return fmt.Errorf("delete plugin kv: %w", err)
	}
	return nil
}

func (s *pluginKVStore) List(ctx context.Context, pluginKey, namespace, keyPrefix string, limit int) ([]string, error) {
	if limit <= 0 {
		return nil, nil
	}
	internalPrefix := s.namespacePrefix(pluginKey, namespace)
	// keyPrefix 已按 [A-Za-z0-9._-] 校验，不含 glob 元字符，可安全拼接进 MATCH。
	pattern := internalPrefix + keyPrefix + "*"
	seen := make(map[string]struct{}, limit)
	keys := make([]string, 0, limit)
	var cursor uint64
	for {
		batch, next, err := s.rdb.Scan(ctx, cursor, pattern, pluginKVScanCount).Result()
		if err != nil {
			return nil, fmt.Errorf("scan plugin kv: %w", err)
		}
		for _, full := range batch {
			// SCAN 可能返回重复键，去重后再裁剪前缀返回面向插件的键名。
			if _, ok := seen[full]; ok {
				continue
			}
			seen[full] = struct{}{}
			keys = append(keys, strings.TrimPrefix(full, internalPrefix))
			if len(keys) >= limit {
				return keys, nil
			}
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return keys, nil
}
