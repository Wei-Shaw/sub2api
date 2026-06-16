// pricing_extension_client_watch.go isolates the long-lived loops that keep
// the PricingOverrideCache in sync with the plugin's authoritative state.
// Two background loops live here:
//
//   - watchLoop: streamutil.Loop wrapper around runWatchOnce; opens
//     WatchPricingOverrides and applies events to the cache, with
//     exponential backoff on stream errors that resets on every successful
//     run (audit-S10 fix).
//   - reSyncLoop: ticker-driven safety net that re-fetches the full
//     snapshot every pricingReSyncInterval, defending against silently
//     dropped Watch events.
//
// Both are launched from Start (in pricing_extension_client.go) and torn
// down via the loopCtx that Stop cancels. They share the codec helpers
// (applyEvent / protoToOverride) defined in pricing_extension_client_codec.go.

package plugin

import (
	"errors"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	"github.com/Wei-Shaw/sub2api/plugin-sdk/streamutil"

	"context"
)

// watchLoop opens WatchPricingOverrides and applies events to the cache.
// Reconnect cadence is delegated to streamutil.Loop so this host-side
// client shares one backoff schedule with the SDK-side jobs / settings /
// events / log_remote loops. Crucially, streamutil.Loop resets backoff to
// pricingWatchBackoffMin whenever runWatchOnce returns nil — that is the
// audit-S10 fix for "stream ran successfully for an hour, lost, reconnect
// waited 30s instead of 1s". Exits when loopCtx is cancelled.
func (c *PricingExtensionClient) watchLoop(loopCtx context.Context) {
	_ = streamutil.Loop(loopCtx, streamutil.Config{
		Name:        "pricing.watch",
		Initial:     pricingWatchBackoffMin,
		Max:         pricingWatchBackoffMax,
		Multiplier:  pricingWatchMultiplier,
		JitterRatio: pricingWatchJitterRatio,
		Logger:      c.logger,
	}, c.runWatchOnce)
}

// runWatchOnce opens a single Watch stream and processes events until
// the stream ends. Returns the terminating error (io.EOF for clean
// close).
func (c *PricingExtensionClient) runWatchOnce(loopCtx context.Context) error {
	stub := c.stubSnapshot()
	if stub == nil {
		return errors.New("pricing extension client: not started")
	}
	since := ""
	if c.cache != nil {
		since = c.cache.Version()
	}
	stream, err := stub.WatchPricingOverrides(loopCtx, &pb.WatchPricingOverridesRequest{
		SinceVersion: since,
	})
	if err != nil {
		return fmt.Errorf("open watch stream: %w", err)
	}
	for {
		evt, err := stream.Recv()
		if err != nil {
			return err
		}
		c.applyEvent(evt)
	}
}

// applyEvent translates a single proto event into a cache mutation.
func (c *PricingExtensionClient) applyEvent(evt *pb.PricingOverrideEvent) {
	if evt == nil || c.cache == nil {
		return
	}
	switch evt.GetOp() {
	case pb.PricingOverrideEvent_UPSERT:
		if o := evt.GetOverride(); o != nil {
			c.cache.Set(c.protoToOverride(o))
		}
	case pb.PricingOverrideEvent_DELETE:
		if k := evt.GetDeletedKey(); k != nil {
			c.cache.Delete(service.PricingOverrideKey{
				GroupID:  k.GetGroupId(),
				Platform: k.GetPlatform(),
				Model:    k.GetModel(),
			})
		}
	}
}

// reSyncLoop periodically full-syncs from the plugin to defend against
// dropped Watch events. Runs until loopCtx is cancelled.
func (c *PricingExtensionClient) reSyncLoop(loopCtx context.Context) {
	t := time.NewTicker(pricingReSyncInterval)
	defer t.Stop()
	for {
		select {
		case <-loopCtx.Done():
			return
		case <-t.C:
			if err := c.fullSync(loopCtx); err != nil {
				c.logger.Warn("pricing override resync failed", "error", err)
			}
		}
	}
}
