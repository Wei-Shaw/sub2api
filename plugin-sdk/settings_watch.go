package pluginsdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	"github.com/Wei-Shaw/sub2api/plugin-sdk/streamutil"
)

// settingsWatchInitialBackoff / settingsWatchMaxBackoff control the
// exponential backoff for watch-stream reconnects. The numbers are
// deliberately conservative so a flapping host does not produce a
// reconnect storm. The schedule itself runs through streamutil.Loop;
// these constants stay here because plugin authors look here to learn
// the contract.
const (
	settingsWatchInitialBackoff = 1 * time.Second
	settingsWatchMaxBackoff     = 30 * time.Second
	// settingsWatchMultiplier was historically ×3 (more aggressive than
	// the events / log_remote ×2) for no documented reason. Unified to
	// ×2 across the SDK so a single Config tweak adjusts every reconnect
	// site uniformly.
	settingsWatchMultiplier  = 2.0
	settingsWatchJitterRatio = 0.2
)

type settingSubscription struct {
	id  uint64
	key string
	ch  chan SettingsChange
}

// startWatchLoop launches the long-lived stream that the SDK uses to
// receive change notifications from the host. It is idempotent so the
// caller can invoke it from Init without worrying about duplicate
// goroutines.
func (c *settingsClient) startWatchLoop() {
	c.watchOnce.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		c.watchCtx = ctx
		c.watchStop = cancel
		c.watchDone = make(chan struct{})
		go c.runWatchLoop(ctx)
	})
}

// stopWatchLoop is invoked from the SDK shutdown path. It cancels the
// stream and waits up to a second for the goroutine to drain so the
// final cleanup logs are emitted before the process exits.
func (c *settingsClient) stopWatchLoop() {
	if c.watchStop == nil {
		return
	}
	c.watchStop()
	if c.watchDone != nil {
		select {
		case <-c.watchDone:
		case <-time.After(time.Second):
		}
	}
}

func (c *settingsClient) Watch(ctx context.Context, key string) (<-chan SettingsChange, func(), error) {
	c.startWatchLoop()
	sub := &settingSubscription{
		key: key,
		ch:  make(chan SettingsChange, 8),
	}
	c.subMu.Lock()
	c.subsNexID++
	sub.id = c.subsNexID
	c.subs[key] = append(c.subs[key], sub)
	c.subMu.Unlock()

	cleanup := func() {
		c.removeSub(key, sub.id)
	}
	// Auto-cleanup when the caller's context expires so callers can fire
	// off a Watch and forget the cleanup func when convenient.
	if ctx != nil && ctx.Done() != nil {
		go func() {
			<-ctx.Done()
			cleanup()
		}()
	}
	return sub.ch, cleanup, nil
}

func (c *settingsClient) removeSub(key string, id uint64) {
	c.subMu.Lock()
	defer c.subMu.Unlock()
	subs := c.subs[key]
	for i, s := range subs {
		if s.id == id {
			close(s.ch)
			c.subs[key] = append(subs[:i], subs[i+1:]...)
			if len(c.subs[key]) == 0 {
				delete(c.subs, key)
			}
			return
		}
	}
}

// runWatchLoop owns the reconnecting Watch stream. It exits when ctx is
// cancelled or when GRPCDefaultClassifier marks an error fatal — the host
// rejects callers without `settings.own.read` capability with
// PermissionDenied, and without the classifier the SDK would reconnect
// forever. After fatal exit SettingsClient.Get continues to work via the
// unary RPC (TTL cache stays cold but every call sees fresh data), so
// the plugin can still read settings, just without the watch fast path.
//
// 复用 (1)：GRPCDefaultClassifier shared with jobs / events / log_remote.
func (c *settingsClient) runWatchLoop(ctx context.Context) {
	defer close(c.watchDone)
	err := streamutil.LoopWithRetryClass(ctx, streamutil.Config{
		Name:        "settings.watch",
		Initial:     settingsWatchInitialBackoff,
		Max:         settingsWatchMaxBackoff,
		Multiplier:  settingsWatchMultiplier,
		JitterRatio: settingsWatchJitterRatio,
		Logger:      c.logger,
	}, streamutil.GRPCDefaultClassifier, c.runWatchAttempt)
	if err != nil && !errors.Is(err, context.Canceled) &&
		!errors.Is(err, context.DeadlineExceeded) && c.logger != nil {
		// LoopWithRetryClass already logged the fatal classification; we
		// annotate with the plugin context so operators can correlate
		// the missing capability with this specific subscription.
		c.logger.Error("settings watch stopped; falling back to unary Get per call",
			"plugin", c.pluginName,
			"error", err)
	}
}

// runWatchAttempt opens a single Watch stream and drains it until
// stream.Recv errors. The host's sendSnapshot fires a fresh per-key
// event for every value in the namespace as soon as the stream opens
// (see SETTINGS-V2-DESIGN §4.5 + INSPECT §2.4), so applyEvent re-
// primes the cache automatically — including the case where the host
// closed the previous stream because the plugin's schema_version
// changed.
func (c *settingsClient) runWatchAttempt(ctx context.Context) error {
	stream, err := c.grpc.Watch(ctx, &pb.SettingsWatchRequest{Key: ""})
	if err != nil {
		return fmt.Errorf("settings watch open: %w", err)
	}
	for {
		evt, recvErr := stream.Recv()
		if recvErr != nil {
			if ctx.Err() == nil && c.logger != nil {
				// EOF / Unavailable here may also mean the host closed
				// the stream because schema_version changed (DESIGN
				// §4.5 dropAllSubscribersForPlugin). Either way the
				// outer loop reconnects and the host's snapshot
				// re-primes our cache.
				c.logger.Warn("settings watch stream lost; will reconnect and re-prime cache via host snapshot",
					"plugin", c.pluginName, "error", recvErr)
			}
			return recvErr
		}
		c.applyEvent(evt)
	}
}

func (c *settingsClient) applyEvent(evt *pb.SettingsChangeEvent) {
	if evt == nil {
		return
	}
	val := append(json.RawMessage(nil), evt.GetValueJson()...)
	// V5/W6 SETTINGS-V2 §3.5: Watch events do not carry schema_version,
	// only the new value + a requires_reload marker. Preserve any cached
	// schema_version from the previous Get so a subsequent GetTyped on
	// the same key can still detect drift. The next Get RPC refreshes
	// the versions if the schema actually changed.
	prevStored, prevCurrent := c.preservedSchemaVersions(evt.GetKey())
	c.storeIfNewer(evt.GetKey(), &cachedSetting{
		value:                val,
		revision:             evt.GetRevision(),
		exists:               true,
		fetchedAt:            time.Now(),
		storedSchemaVersion:  prevStored,
		currentSchemaVersion: prevCurrent,
	})
	if evt.GetRequiresReload() && c.logger != nil {
		c.logger.Info("settings change marked requires_reload; host will reload plugin",
			"plugin", c.pluginName, "key", evt.GetKey(), "revision", evt.GetRevision())
	}
	c.fanOutChange(SettingsChange{
		Key: evt.GetKey(), Value: val, Revision: evt.GetRevision(),
	}, evt.GetKey())
}

// preservedSchemaVersions looks up the cached schema_version pair for a
// key so that watch events (which lack schema_version) can preserve it.
func (c *settingsClient) preservedSchemaVersions(key string) (stored, current string) {
	if v, ok := c.cache.Load(key); ok {
		entry := v.(*cachedSetting)
		return entry.storedSchemaVersion, entry.currentSchemaVersion
	}
	return "", ""
}

// fanOutChange delivers a change event to all matching subscribers:
// exact-key subscribers and empty-key (namespace) subscribers.
func (c *settingsClient) fanOutChange(change SettingsChange, key string) {
	c.subMu.Lock()
	targets := make([]*settingSubscription, 0)
	targets = append(targets, c.subs[key]...)
	if key != "" {
		targets = append(targets, c.subs[""]...)
	}
	c.subMu.Unlock()

	for _, sub := range targets {
		select {
		case sub.ch <- change:
		default:
			if c.logger != nil {
				c.logger.Warn("settings watcher channel full; event dropped",
					"plugin", c.pluginName, "key", key)
			}
		}
	}
}
