package handler

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
)

// webhookDedupTTL bounds how long a notification ID is treated as "seen".
// Set comfortably larger than paymentGraceMinutes (5m) so that even after an
// order expires and the grace window closes, the same provider notify_id
// cannot be replayed to revive it via the OrderStatusEQ(Expired) branch in
// PaymentService.toPaid.
const webhookDedupTTL = 30 * time.Minute

// webhookDedupKeyPrefix is the Redis key prefix for replay-protection
// reservations. The plugin's per-plugin Redis namespace adds another layer
// of isolation on top of this.
const webhookDedupKeyPrefix = "payment:webhook:dedup:"

// ErrWebhookReplay signals that the provider's notify_id has already been
// processed. Webhook handlers should respond with the provider's success
// ack (e.g. "success" / 200 OK) so the provider stops retrying.
var ErrWebhookReplay = errors.New("webhook: notification already processed")

// webhookDedup is a thin Redis SETNX-style guard. The first call for a
// given (provider, notifyID) pair returns nil and reserves the key for
// webhookDedupTTL; subsequent calls return ErrWebhookReplay.
//
// When redis is nil (driver outage / not wired) the dedup degrades to
// "always allow" with a slog.Warn — this is fail-open by design because
// the per-order CAS in toPaid still prevents double-spend within a single
// order, and webhook delivery loss is worse than replay risk during a
// brief Redis outage.
//
// Concurrency: the SDK RedisClient is safe for concurrent use by design,
// so a single shared *webhookDedup can fan out across handler goroutines
// without additional synchronisation.
type webhookDedup struct {
	redis  pluginsdk.RedisClient
	logger *slog.Logger
	ttl    time.Duration
}

// newWebhookDedup builds a guard that uses the supplied Redis client. A nil
// client is permitted — callers that pass nil must accept fail-open
// semantics (see type doc).
func newWebhookDedup(redis pluginsdk.RedisClient, logger *slog.Logger) *webhookDedup {
	if logger == nil {
		logger = slog.Default()
	}
	return &webhookDedup{redis: redis, logger: logger, ttl: webhookDedupTTL}
}

// Reserve returns nil on first observation, ErrWebhookReplay on replay.
//
// Empty notifyID skips dedup with a debug log line — providers with no
// canonical event ID cannot benefit from this protection (in practice all
// four supported providers populate PaymentNotification.TradeNo after
// signature verification, so this branch should never fire in production).
//
// When the underlying SetNX call fails (Redis transport error, command
// error) we log a warning and fail open. The caller's per-order CAS still
// protects against double-fulfillment for a single order.
func (d *webhookDedup) Reserve(ctx context.Context, provider, notifyID string) error {
	if d == nil || d.redis == nil {
		return nil
	}
	notifyID = strings.TrimSpace(notifyID)
	if notifyID == "" {
		d.logger.Debug("webhook dedup skipped: empty notifyID", "provider", provider)
		return nil
	}
	key := webhookDedupKeyPrefix + provider + ":" + notifyID
	// SET key value NX EX <ttl-seconds>. Returns true when the key was set
	// (first observation), false when the key already existed (replay).
	ok, err := d.redis.SetNX(ctx, key, "1", d.ttl).Result()
	if err != nil {
		d.logger.Warn("webhook dedup setnx failed; falling open",
			"provider", provider, "notifyID", notifyID, "error", err)
		return nil
	}
	if !ok {
		return ErrWebhookReplay
	}
	return nil
}
