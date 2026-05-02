// Package plugin — pricing_extension_client.go
//
// PricingExtensionClient is the host-side proxy that talks to plugins
// implementing the PricingExtension service (P2 skeleton). It owns:
//
//   - Bootstrap List: full snapshot of pricing overrides on plugin start,
//     loaded into PricingOverrideCache before the gateway hot path uses it.
//   - Watch loop: long-lived stream that pushes incremental updates to
//     the cache. Reconnects with exponential backoff (1s..30s) when the
//     stream ends.
//   - Periodic safety re-sync: every reSyncInterval the client invokes
//     ListPricingOverrides again to catch silently-dropped events.
//   - AdjustCost: per-request RPC the BillingService hooks in after it
//     computes the base cost. Plugin response replaces the cost; errors
//     fall back to the host-computed value with a logged warning.
//
// Connection model: the client is bound to a single plugin (one per
// PluginInstance) and uses the existing gRPC connection PluginManager
// already opened to the plugin process. Stop() must be called when the
// plugin is disabled / restarted so the goroutine and stream exit cleanly.
//
// This file deliberately does NOT register itself with the PluginManager
// today — wiring is left to a follow-up phase. See PLUGIN-PRICING design
// doc for the full integration plan.

package plugin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"google.golang.org/grpc"

	"github.com/Wei-Shaw/sub2api/internal/service"
	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

const (
	// pricingListTimeout caps a single ListPricingOverrides RPC. Plugins
	// returning bigger snapshots should chunk in a future revision.
	pricingListTimeout = 30 * time.Second
	// pricingAdjustTimeout caps a single AdjustCost RPC. The host falls
	// back to its own computed cost on timeout so the value is small
	// enough to keep gateway latency predictable.
	pricingAdjustTimeout = 1500 * time.Millisecond
	// pricingAccountStatsTimeout caps a ResolveAccountStatsCost RPC. The
	// host falls back to NULL on timeout (default formula) so this stays
	// small to keep recordUsageCore latency stable.
	pricingAccountStatsTimeout = 1500 * time.Millisecond
	// pricingReSyncInterval is the period at which the client invokes
	// ListPricingOverrides as a safety net against silently dropped Watch
	// events. 5 minutes matches the existing channel cache contract.
	pricingReSyncInterval = 5 * time.Minute

	// pricingWatchBackoffMin / Max bracket the reconnect backoff used by
	// streamutil.Loop when the Watch stream breaks. Same shape as the SDK's
	// jobs / settings / events / log_remote loops; multiplier and jitter
	// match those clients so a single bad plugin doesn't reconnect on a
	// noticeably different cadence than the SDK-side ones.
	pricingWatchBackoffMin  = 1 * time.Second
	pricingWatchBackoffMax  = 30 * time.Second
	pricingWatchMultiplier  = 2.0
	pricingWatchJitterRatio = 0.2
)

// PricingExtensionClient proxies host calls to a single plugin's
// PricingExtension server. It is safe to call AdjustCost concurrently;
// Start / Stop are not — typical lifecycle is one Start per plugin spawn
// and one Stop on shutdown.
type PricingExtensionClient struct {
	pluginName string
	cache      *service.PricingOverrideCache
	logger     *slog.Logger

	// parentCtx anchors the watch / resync loops. Decoupling them from the
	// Start ctx (which is a request-scoped HTTP / spawn ctx that gets
	// cancelled as soon as the caller returns) is what keeps the loops
	// alive past Start. Stop() owns explicit cancellation through the
	// derived loopCancel below.
	//
	// We keep this as a field rather than a Start argument so the
	// constructor caller (PluginManager.tryStartPricingExtension) can pin
	// it once with a long-lived context (today: context.Background(); in
	// the future: a manager-scoped shutdown ctx without losing trace).
	parentCtx context.Context

	// stub is set lazily on Start so a nil client (no plugin registered)
	// is a valid no-op state. AdjustCost short-circuits when stub is nil.
	mu     sync.RWMutex
	stub   pb.PricingExtensionClient
	cancel context.CancelFunc
	loopWG sync.WaitGroup
}

// NewPricingExtensionClient constructs the client. cache must be non-nil
// in production; tests may pass nil to exercise AdjustCost without the
// data layer.
//
// parentCtx is the long-lived ancestor of the watch / resync loops. Pass
// context.Background() when you want loops to run until Stop() is called;
// pass a manager-scoped shutdown ctx if you want a process-wide cancellation
// signal to tear them down for free. Nil is treated as Background.
//
// The caller is expected to invoke Start once a plugin's gRPC connection
// is ready, and Stop on plugin teardown. The constructor itself does not
// dial anything.
func NewPricingExtensionClient(
	parentCtx context.Context,
	pluginName string,
	cache *service.PricingOverrideCache,
	logger *slog.Logger,
) *PricingExtensionClient {
	if logger == nil {
		logger = slog.Default()
	}
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	return &PricingExtensionClient{
		pluginName: pluginName,
		cache:      cache,
		logger:     logger.With("component", "pricing_ext_client", "plugin", pluginName),
		parentCtx:  parentCtx,
	}
}

// Start binds the client to the supplied gRPC connection and runs the
// initial List + background Watch + periodic re-sync. It returns once
// the initial List has completed (or failed); caller should treat a
// non-nil error as "plugin is up but PricingExtension not usable".
func (c *PricingExtensionClient) Start(ctx context.Context, conn grpc.ClientConnInterface) error {
	if c == nil {
		return errors.New("pricing extension client: nil receiver")
	}
	if conn == nil {
		return errors.New("pricing extension client: nil grpc conn")
	}

	stub := pb.NewPricingExtensionClient(conn)

	c.mu.Lock()
	c.stub = stub
	// Derive the loop ctx from parentCtx (set by the constructor), not from
	// the Start ctx. Start ctx is request-scoped and cancels as soon as the
	// caller returns; the loops must outlive that. The trace context (if any
	// is attached to parentCtx) flows through automatically via the gRPC
	// client interceptor — no manual metadata handling here.
	loopCtx, cancel := context.WithCancel(c.parentCtx)
	c.cancel = cancel
	c.mu.Unlock()

	// Initial snapshot. Errors are not fatal — the cache simply stays
	// empty and AdjustCost still works.
	if err := c.fullSync(ctx); err != nil {
		c.logger.Warn("initial ListPricingOverrides failed", "error", err)
	}

	// Background loops. Both check loopCtx and exit on Stop.
	c.loopWG.Add(2)
	go func() {
		defer c.loopWG.Done()
		c.watchLoop(loopCtx)
	}()
	go func() {
		defer c.loopWG.Done()
		c.reSyncLoop(loopCtx)
	}()
	return nil
}

// Stop tears down the watch + resync loops and waits for them to exit.
// Subsequent calls to AdjustCost return modified=false.
func (c *PricingExtensionClient) Stop() {
	if c == nil {
		return
	}
	c.mu.Lock()
	cancel := c.cancel
	c.cancel = nil
	c.stub = nil
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	c.loopWG.Wait()
}

// stubSnapshot returns the current stub under the read lock. Callers
// short-circuit when nil.
func (c *PricingExtensionClient) stubSnapshot() pb.PricingExtensionClient {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.stub
}

// fullSync invokes ListPricingOverrides and replaces the cache contents.
// Ctx scopes only the RPC; the cache write is non-cancellable.
func (c *PricingExtensionClient) fullSync(ctx context.Context) error {
	stub := c.stubSnapshot()
	if stub == nil {
		return errors.New("pricing extension client: not started")
	}
	rpcCtx, cancel := context.WithTimeout(ctx, pricingListTimeout)
	defer cancel()

	resp, err := stub.ListPricingOverrides(rpcCtx, &pb.ListPricingOverridesRequest{})
	if err != nil {
		return fmt.Errorf("ListPricingOverrides: %w", err)
	}
	if c.cache == nil {
		return nil
	}
	overrides := make([]service.PricingOverride, 0, len(resp.GetOverrides()))
	for _, o := range resp.GetOverrides() {
		overrides = append(overrides, c.protoToOverride(o))
	}
	c.cache.ReplaceAll(overrides, resp.GetVersion())
	c.logger.Info("pricing override cache resynced",
		"count", len(overrides),
		"version", resp.GetVersion(),
	)
	return nil
}
