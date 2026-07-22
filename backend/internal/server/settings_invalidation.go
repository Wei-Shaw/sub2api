package server

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	settingsInvalidationChannel = "settings:cache:invalidate"
	settingsPublishTimeout      = 3 * time.Second
	settingsSubscribeTimeout    = 3 * time.Second
)

type settingsInvalidationBus interface {
	Publish(ctx context.Context) error
	Subscribe(ctx context.Context, handler func()) error
}

type redisSettingsInvalidationBus struct {
	rdb *redis.Client
}

func newRedisSettingsInvalidationBus(rdb *redis.Client) settingsInvalidationBus {
	if rdb == nil {
		return nil
	}
	return &redisSettingsInvalidationBus{rdb: rdb}
}

func (b *redisSettingsInvalidationBus) Publish(ctx context.Context) error {
	if b == nil || b.rdb == nil {
		return nil
	}
	return b.rdb.Publish(ctx, settingsInvalidationChannel, time.Now().UnixNano()).Err()
}

func (b *redisSettingsInvalidationBus) Subscribe(ctx context.Context, handler func()) error {
	if b == nil || b.rdb == nil {
		return nil
	}
	if handler == nil {
		return fmt.Errorf("settings invalidation handler is nil")
	}

	pubsub := b.rdb.Subscribe(ctx, settingsInvalidationChannel)
	subscribeCtx, cancel := context.WithTimeout(ctx, settingsSubscribeTimeout)
	defer cancel()
	if _, err := pubsub.Receive(subscribeCtx); err != nil {
		_ = pubsub.Close()
		return fmt.Errorf("subscribe to settings invalidation: %w", err)
	}

	go func() {
		defer func() {
			if err := pubsub.Close(); err != nil {
				log.Printf("Warning: failed to close settings invalidation subscriber: %v", err)
			}
		}()

		ch := pubsub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				if msg != nil {
					handler()
				}
			}
		}
	}()

	return nil
}

// configureSettingsInvalidation connects a local cache invalidator to the
// shared invalidation bus and returns the callback used after a settings write.
// The callback always clears local caches first, then broadcasts to peers.
func configureSettingsInvalidation(
	ctx context.Context,
	bus settingsInvalidationBus,
	invalidateLocal func(),
	onError func(error),
) func() {
	if invalidateLocal == nil {
		invalidateLocal = func() {}
	}
	if onError == nil {
		onError = func(error) {}
	}

	if bus != nil {
		if err := bus.Subscribe(ctx, invalidateLocal); err != nil {
			onError(err)
		}
	}

	return func() {
		invalidateLocal()
		if bus == nil {
			return
		}
		publishCtx, cancel := context.WithTimeout(context.Background(), settingsPublishTimeout)
		defer cancel()
		if err := bus.Publish(publishCtx); err != nil {
			onError(fmt.Errorf("publish settings invalidation: %w", err))
		}
	}
}
