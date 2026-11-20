package service

import (
	"context"
	"errors"
	"sync"
	"time"
)

var ErrAPIKeySlotLeaseLost = errors.New("API key concurrency lease lost")
var ErrAPIKeyConcurrencyLimit = errors.New("API key concurrency limit reached")

type apiKeyAdmissionOwnerKey struct{}
type apiKeyAdmissionOwner struct {
	ctx    context.Context
	cancel context.CancelCauseFunc
}

// WithAPIKeyAdmissionOwner installs cancellation ownership, not admission. Auth
// calls it before handlers capture ctx. The separate lifetime signal survives
// HTTP stream detachment, while ordinary client cancellation keeps its semantics.
func WithAPIKeyAdmissionOwner(ctx context.Context) (context.Context, context.CancelFunc) {
	if HasAPIKeyAdmissionOwner(ctx) {
		return ctx, func() {}
	}
	control, cancelControl := context.WithCancelCause(context.WithoutCancel(ctx))
	request, cancelRequest := context.WithCancelCause(ctx)
	owner := &apiKeyAdmissionOwner{ctx: control, cancel: func(err error) {
		cancelControl(err)
		cancelRequest(err)
	}}
	return context.WithValue(request, apiKeyAdmissionOwnerKey{}, owner), func() { owner.cancel(context.Canceled) }
}

func HasAPIKeyAdmissionOwner(ctx context.Context) bool {
	_, ok := apiKeyAdmissionOwnerFromContext(ctx)
	return ok
}

func apiKeyAdmissionOwnerFromContext(ctx context.Context) (*apiKeyAdmissionOwner, bool) {
	if ctx == nil {
		return nil, false
	}
	owner, ok := ctx.Value(apiKeyAdmissionOwnerKey{}).(*apiKeyAdmissionOwner)
	if !ok || owner == nil || owner.ctx == nil || owner.cancel == nil {
		return nil, false
	}
	return owner, true
}

func APIKeySlotLeaseLost(ctx context.Context) bool {
	owner, ok := apiKeyAdmissionOwnerFromContext(ctx)
	if !ok {
		return false
	}
	return errors.Is(context.Cause(owner.ctx), ErrAPIKeySlotLeaseLost)
}

// Detach client cancellation, but never detach the capacity owner's stop signal.
func detachAPIKeyUpstreamContext(ctx context.Context) (context.Context, context.CancelFunc) {
	base := context.WithoutCancel(ctx)
	owner, ok := apiKeyAdmissionOwnerFromContext(ctx)
	if !ok {
		return base, func() {}
	}
	// Existing builders release immediately after constructing a request. Keep
	// that no-op release contract: the auth owner, not the builder, owns lifetime.
	return apiKeyUpstreamContext{Context: owner.ctx, values: base}, func() {}
}

type apiKeyUpstreamContext struct {
	context.Context
	values context.Context
}

func (c apiKeyUpstreamContext) Value(key any) any {
	if value := c.values.Value(key); value != nil {
		return value
	}
	// WithoutCancel hides context's cancellation-cause key; recover it from the
	// control context while retaining newer request routing/identity values.
	return c.Context.Value(key)
}

type APIKeySlotLeaseCache interface {
	APIKeyConcurrencyCache
	APIKeySlotRefreshCache
	APIKeySlotTTL() time.Duration
}

// A watchdog runs independently of renewal I/O. Its deadline is based on the
// START of the last acknowledged Redis operation, never on a failed renewal or
// delayed reply. Reserve one second for Redis TIME rounding PLUS one third of
// TTL for transport shutdown before Redis can admit another owner.
func keepEnforcedAPIKeySlot(owner *apiKeyAdmissionOwner, cache APIKeySlotLeaseCache, keyID int64, requestID string, acknowledgedAt time.Time) func() {
	release, _ := keepEnforcedAPIKeySlotState(owner, cache, keyID, requestID, acknowledgedAt)
	return release
}

// keepEnforcedAPIKeySlotState returns the full stop+delete release and a
// stop-only pause. The Live handoff pauses renewal before its atomic transfer,
// so a removed regular member can never be reported as a lost lease, and the
// original request release can only stop the successor, never delete it.
func keepEnforcedAPIKeySlotState(owner *apiKeyAdmissionOwner, cache APIKeySlotLeaseCache, keyID int64, requestID string, acknowledgedAt time.Time) (release func(), pause func()) {
	ttl := cache.APIKeySlotTTL()
	margin := ttl/3 + time.Second
	validFor := ttl - margin
	refreshInterval := cache.APIKeySlotRefreshInterval()
	if refreshInterval > validFor/2 {
		refreshInterval = validFor / 2
	}
	opTimeout := 2 * time.Second
	if opTimeout > validFor/2 {
		opTimeout = validFor / 2
	}
	workerCtx, cancelWorker := context.WithCancel(context.Background())
	done := make(chan struct{})
	var watchdogMu sync.Mutex
	lost := false
	lose := func() {
		watchdogMu.Lock()
		defer watchdogMu.Unlock()
		if lost {
			return
		}
		lost = true
		owner.cancel(ErrAPIKeySlotLeaseLost)
		cancelWorker()
	}
	watchdog := time.AfterFunc(time.Until(acknowledgedAt.Add(validFor)), lose)
	go func() {
		defer close(done)
		ticker := time.NewTicker(refreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-workerCtx.Done():
				return
			case <-ticker.C:
				started := time.Now()
				opCtx, cancel := context.WithTimeout(workerCtx, opTimeout)
				exists, err := cache.RefreshAPIKeySlot(opCtx, keyID, requestID)
				cancel()
				if err == nil && !exists {
					lose()
					return
				}
				if err == nil {
					watchdogMu.Lock()
					if !lost {
						watchdog.Reset(time.Until(started.Add(validFor)))
					}
					watchdogMu.Unlock()
				}
			}
		}
	}()
	// stopRenewal joins the refresh worker and the watchdog without removing the
	// member. A late refresh reply cannot recreate it: RefreshAPIKeySlot only
	// updates an existing member.
	stopRenewal := sync.OnceFunc(func() {
		watchdogMu.Lock()
		lost = true
		watchdog.Stop()
		watchdogMu.Unlock()
		cancelWorker()
		<-done
	})
	release = sync.OnceFunc(func() {
		stopRenewal()
		// Only the forwarding owner removes capacity, after upstream shutdown.
		releaseAPIKeySlot(cache, keyID, requestID)
	})
	return release, stopRenewal
}
