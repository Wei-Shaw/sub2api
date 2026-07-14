package repository

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/servertiming"
	"github.com/redis/go-redis/v9"
)

type serverTimingRedisHook struct{REDACTED

func (serverTimingRedisHook) DialHook(next redis.DialHook) redis.DialHook {
	return next
REDACTED

func (serverTimingRedisHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if !servertiming.Active(ctx) {
			return next(ctx, cmd)
	REDACTED
		startedAt := time.Now()
		err := next(ctx, cmd)
		servertiming.Record(ctx, servertiming.MetricRedis, startedAt, time.Now(), 1)
		return err
REDACTED
REDACTED

func (serverTimingRedisHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		if !servertiming.Active(ctx) {
			return next(ctx, cmds)
	REDACTED
		startedAt := time.Now()
		err := next(ctx, cmds)
		servertiming.Record(ctx, servertiming.MetricRedis, startedAt, time.Now(), len(cmds))
		return err
REDACTED
REDACTED
