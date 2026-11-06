package reqcompress

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	sharedAlgorithmKeyPrefix = "reqcompress:account:"
	sharedStateWriteTimeout  = 5 * time.Second
)

var deleteSharedAlgorithmIfMatches = redis.NewScript(`
if redis.call('GET', KEYS[1]) == ARGV[1] then
    return redis.call('DEL', KEYS[1])
end
return 0
`)

// ConfigureRedis attaches the process-wide registry to the shared negotiation
// store. A nil client restores the memory-only behavior used by tests and by
// deployments that have not initialized Redis yet.
func ConfigureRedis(client *redis.Client) {
	defaultRegistry.SetRedis(client)
}

// SetRedis attaches this registry to the shared negotiation store. The request
// hot path keeps negotiated state in memory and reads Redis only while
// initializing an account; administrative display reads use the latest shared
// snapshot, and writes follow state transitions.
func (r *Registry) SetRedis(client *redis.Client) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.shared = client
}

func (r *Registry) sharedRedis() *redis.Client {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.shared
}

func sharedAlgorithmKey(accountID int64) string {
	return sharedAlgorithmKeyPrefix + strconv.FormatInt(accountID, 10) + ":algorithm"
}

func parseSharedAlgorithm(raw string) (Algorithm, bool) {
	algorithm := Algorithm(strings.TrimSpace(raw))
	switch algorithm {
	case AlgorithmBrotli, AlgorithmZstd, AlgorithmDeflate:
		return algorithm, true
	default:
		return AlgorithmNone, false
	}
}

// hydrateShared initializes one account from Redis. Holding the state lock
// across the read prevents a concurrent local trial from racing the snapshot.
// A failed read is retried on a later initialization attempt; the caller sends
// identity in the meantime instead of restarting negotiation with stale or
// unknown shared state.
func (r *Registry) hydrateShared(ctx context.Context, accountID int64, negotiation *state) error {
	client := r.sharedRedis()
	if client == nil || negotiation == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	negotiation.mu.Lock()
	defer negotiation.mu.Unlock()
	if negotiation.sharedLoaded || negotiation.tested || negotiation.negotiating {
		return nil
	}

	raw, err := client.Get(ctx, sharedAlgorithmKey(accountID)).Result()
	if errors.Is(err, redis.Nil) {
		negotiation.sharedLoaded = true
		return nil
	}
	if err != nil {
		return err
	}
	algorithm, ok := parseSharedAlgorithm(raw)
	if !ok {
		return fmt.Errorf("invalid shared request compression algorithm %q", raw)
	}

	negotiation.sharedLoaded = true
	negotiation.current = algorithm
	negotiation.tested = true
	negotiation.negotiating = false
	negotiation.remaining = keepFrom(TrialOrder, algorithm)
	return nil
}

func sharedWriteContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	// The upstream request may be canceled immediately after a successful
	// response. Shared state must still reflect that completed negotiation.
	return context.WithTimeout(context.WithoutCancel(ctx), sharedStateWriteTimeout)
}

func (r *Registry) persistSharedAlgorithm(ctx context.Context, accountID int64, algorithm Algorithm) {
	client := r.sharedRedis()
	if client == nil || algorithm == AlgorithmNone {
		return
	}
	ctx, cancel := sharedWriteContext(ctx)
	defer cancel()

	// Do not let a request that finishes after an administrative disable/reset
	// resurrect its old result. The slot lock orders this check and write with
	// the local state transition; a following disable then removes the value.
	slot := r.existingSlot(accountID)
	if slot == nil {
		return
	}
	slot.mu.Lock()
	if slot.state == nil || slot.state.settledAlgorithm() != algorithm {
		slot.mu.Unlock()
		return
	}
	// A zero expiration is intentional: provider coding support is stable, and
	// an actual rejection below retires the value and triggers renegotiation.
	err := client.Set(ctx, sharedAlgorithmKey(accountID), string(algorithm), 0).Err()
	slot.mu.Unlock()
	if err != nil {
		slog.Warn("reqcompress shared state store failed", "account_id", accountID,
			"algorithm", algorithm, "error", err)
	}
}

// deleteSharedAlgorithm removes a coding that is no longer valid. When expected
// is non-empty, deletion is conditional so an old failure cannot remove a
// different algorithm another instance has already settled. An empty expected
// value means an administrative reset and deletes unconditionally.
func (r *Registry) deleteSharedAlgorithm(ctx context.Context, accountID int64, expected Algorithm) {
	client := r.sharedRedis()
	if client == nil {
		return
	}
	ctx, cancel := sharedWriteContext(ctx)
	defer cancel()
	var err error
	if expected == AlgorithmNone {
		err = client.Del(ctx, sharedAlgorithmKey(accountID)).Err()
	} else {
		err = deleteSharedAlgorithmIfMatches.Run(ctx, client,
			[]string{sharedAlgorithmKey(accountID)}, string(expected)).Err()
	}
	if err != nil {
		slog.Warn("reqcompress shared state delete failed", "account_id", accountID,
			"algorithm", expected, "error", err)
	}
}

// ResetAccount drops both the local and shared negotiation result. It is used
// when the administrator toggles the switch or deletes the account; ordinary
// cache observations must not call it, or every read would force a new probe.
func ResetAccount(ctx context.Context, accountID int64) {
	defaultRegistry.Reset(ctx, accountID)
}

// Reset drops both the local and shared negotiation result for accountID.
func (r *Registry) Reset(ctx context.Context, accountID int64) {
	if r == nil || accountID <= 0 {
		return
	}
	r.mu.Lock()
	if slot := r.entries[accountID]; slot != nil {
		slot.mu.Lock()
		slot.state = nil
		slot.mu.Unlock()
	}
	delete(r.entries, accountID)
	r.mu.Unlock()
	r.deleteSharedAlgorithm(ctx, accountID, AlgorithmNone)
}

// SettledAlgorithm returns the currently pinned coding, if any. Administrative
// responses use this read-only view rather than changing the request hot path.
func SettledAlgorithm(ctx context.Context, accountID int64) Algorithm {
	return defaultRegistry.SettledAlgorithm(ctx, accountID)
}

// SettledAlgorithm returns the latest shared coding for administrative display.
// Reading Redis on each call keeps list refreshes consistent across instances;
// process-local state remains the fallback for memory-only mode and Redis errors.
func (r *Registry) SettledAlgorithm(ctx context.Context, accountID int64) Algorithm {
	slot := r.existingSlot(accountID)
	if slot == nil {
		return AlgorithmNone
	}
	negotiation := slot.currentState()
	if negotiation == nil {
		return AlgorithmNone
	}
	client := r.sharedRedis()
	if client == nil {
		return negotiation.settledAlgorithm()
	}
	if ctx == nil {
		ctx = context.Background()
	}
	raw, err := client.Get(ctx, sharedAlgorithmKey(accountID)).Result()
	if errors.Is(err, redis.Nil) {
		return AlgorithmNone
	}
	if err != nil {
		slog.Warn("reqcompress shared state load failed", "account_id", accountID, "error", err)
		return negotiation.settledAlgorithm()
	}
	algorithm, ok := parseSharedAlgorithm(raw)
	if !ok {
		slog.Warn("reqcompress shared state load failed", "account_id", accountID,
			"error", fmt.Errorf("invalid shared request compression algorithm %q", raw))
		return negotiation.settledAlgorithm()
	}
	return algorithm
}
