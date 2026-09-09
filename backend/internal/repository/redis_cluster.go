package repository

import (
	"context"
	"sync"

	"github.com/redis/go-redis/v9"
)

// scanRedisKeys scans every primary when the client is cluster-backed. A plain
// SCAN sent through ClusterClient reaches only one node and can silently miss
// keys stored on the other primaries.
func scanRedisKeys(ctx context.Context, client redis.UniversalClient, pattern string, count int64) ([]string, error) {
	scanNode := func(ctx context.Context, node redis.Cmdable) ([]string, error) {
		var (
			cursor uint64
			keys   []string
		)
		for {
			page, next, err := node.Scan(ctx, cursor, pattern, count).Result()
			if err != nil {
				return nil, err
			}
			keys = append(keys, page...)
			cursor = next
			if cursor == 0 {
				return keys, nil
			}
		}
	}

	cluster, ok := client.(*redis.ClusterClient)
	if !ok {
		return scanNode(ctx, client)
	}

	var (
		mu   sync.Mutex
		keys []string
	)
	err := cluster.ForEachMaster(ctx, func(ctx context.Context, node *redis.Client) error {
		page, err := scanNode(ctx, node)
		if err != nil {
			return err
		}
		mu.Lock()
		keys = append(keys, page...)
		mu.Unlock()
		return nil
	})
	return keys, err
}

// deleteRedisKeys uses a regular pipeline so a cluster client can route each
// key independently. DEL with keys from different slots returns CROSSSLOT.
func deleteRedisKeys(ctx context.Context, client redis.UniversalClient, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	pipe := client.Pipeline()
	for _, key := range keys {
		pipe.Del(ctx, key)
	}
	_, err := pipe.Exec(ctx)
	return err
}
