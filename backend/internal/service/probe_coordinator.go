package service

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type ProbeCoordinator struct{ rdb *redis.Client }

func NewProbeCoordinator(rdb *redis.Client) *ProbeCoordinator { return &ProbeCoordinator{rdb: rdb} }

func (p *ProbeCoordinator) Run(ctx context.Context, key string, ttl time.Duration, fn func() error) (bool, error) {
	if p == nil || p.rdb == nil {
		return true, fn()
	}
	owner := fmt.Sprintf("%d", time.Now().UnixNano())
	redisKey := "probe_lease:v1:" + key
	acquired, err := p.rdb.SetNX(ctx, redisKey, owner, ttl).Result()
	if err != nil {
		// Probe coordination must never turn a Redis outage into an upstream outage.
		return true, fn()
	}
	if !acquired {
		return false, nil
	}
	defer func() {
		const releaseScript = `if redis.call("get", KEYS[1]) == ARGV[1] then return redis.call("del", KEYS[1]) else return 0 end`
		_, _ = p.rdb.Eval(context.WithoutCancel(ctx), releaseScript, []string{redisKey}, owner).Result()
	}()
	return true, fn()
}
