// Package pricing implements the channel-management plugin's
// PricingExtension gRPC server (P3+P5). The host calls
// ListPricingOverrides on plugin start to bootstrap its in-memory pricing
// cache and keeps a long-lived WatchPricingOverrides stream for incremental
// updates. AdjustCost is invoked per-request after the host computes the
// base cost; the channel-management plugin currently returns Modified=false
// because every pricing decision is already encoded in the override snapshot.
//
// Watch implementation note: this version pushes the current snapshot to a
// freshly-subscribed client and then keeps the stream open without further
// events. Cache freshness for CRUD changes is provided by:
//
//  1. The host's 5-minute periodic re-sync (pricing_extension_client.go),
//     which calls ListPricingOverrides again and replaces the cache.
//  2. A reconnect after stream end (e.g. plugin restart) — the client passes
//     SinceVersion and we resend a fresh snapshot regardless.
//
// A future revision can wire ChannelService.{Create,Update,Delete} into a
// broker that pushes UPSERT/DELETE events on this stream for sub-second
// freshness; the proto contract is already shaped for it.
package pricing

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	chService "github.com/Wei-Shaw/sub2api/plugins/channel-management/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// channelLister is the minimal repo surface the server needs. ChannelService
// satisfies it via its embedded repository; tests pass a fake.
type channelLister interface {
	ListAll(ctx context.Context) ([]chService.Channel, error)
	GetGroupPlatforms(ctx context.Context, groupIDs []int64) (map[int64]string, error)
}

// Server implements pluginsdk.PricingExtensionServer for the
// channel-management plugin.
//
// The server is constructed at process startup (before plugin.Init) so it
// can register on the SDK's grpc.Server, but the data dependency is wired
// lazily via SetLister once Init has built the repository. RPCs that arrive
// before SetLister return codes.Unavailable so the host's pricing extension
// client treats the plugin as "not ready yet" and retries via the periodic
// re-sync ticker.
type Server struct {
	pb.UnimplementedPricingExtensionServer

	lister atomic.Pointer[listerHolder]

	mu      sync.Mutex
	version string
}

// listerHolder wraps the channelLister so atomic.Pointer can store a typed
// nil-safe value without going through reflection.
type listerHolder struct {
	inner channelLister
}

// NewServer returns a Server with no lister wired. Call SetLister from
// Plugin.Init once the repository is available.
func NewServer() *Server {
	return &Server{}
}

// SetLister wires the data source. Calling SetLister with a nil lister
// detaches the previous one; subsequent RPCs return Unavailable until the
// next non-nil call.
func (s *Server) SetLister(lister channelLister) {
	if lister == nil {
		s.lister.Store(nil)
		return
	}
	s.lister.Store(&listerHolder{inner: lister})
}

// loadLister returns the wired lister or nil. Hot path; cheap atomic load.
func (s *Server) loadLister() channelLister {
	h := s.lister.Load()
	if h == nil {
		return nil
	}
	return h.inner
}

// ListPricingOverrides walks every active channel, expands each pricing row
// per attached group, and emits one PricingOverride for each
// (group_id, platform, model) tuple.
func (s *Server) ListPricingOverrides(
	ctx context.Context,
	_ *pb.ListPricingOverridesRequest,
) (*pb.ListPricingOverridesResponse, error) {
	overrides, version, err := s.snapshot(ctx)
	if err != nil {
		return nil, err
	}
	return &pb.ListPricingOverridesResponse{
		Overrides: overrides,
		Version:   version,
	}, nil
}

// WatchPricingOverrides streams the current snapshot as UPSERT events and
// keeps the stream open until the client disconnects. See the package doc
// for the simplified-broker rationale.
func (s *Server) WatchPricingOverrides(
	req *pb.WatchPricingOverridesRequest,
	stream pb.PricingExtension_WatchPricingOverridesServer,
) error {
	overrides, version, err := s.snapshot(stream.Context())
	if err != nil {
		return err
	}
	for _, o := range overrides {
		evt := &pb.PricingOverrideEvent{
			Op:       pb.PricingOverrideEvent_UPSERT,
			Override: o,
			Version:  version,
		}
		if err := stream.Send(evt); err != nil {
			return err
		}
	}
	// Hold the stream open. The host treats stream end as a signal to
	// reconnect with the last-known version, and the 5-minute re-sync
	// covers CRUD changes that happened in between. Exit on cancellation.
	_ = req // since_version intentionally ignored — we always resend full snapshot
	<-stream.Context().Done()
	return nil
}

// AdjustCost is currently a typed no-op: returns Modified=false so the host
// keeps its own cost. The RPC channel is wired so future revisions can
// migrate channel-level multipliers from the host into the plugin without
// further proto changes.
func (s *Server) AdjustCost(
	_ context.Context,
	_ *pb.AdjustCostRequest,
) (*pb.AdjustCostResponse, error) {
	return &pb.AdjustCostResponse{
		Modified:         false,
		AdjustmentReason: "channel-management: AdjustCost not implemented yet",
	}, nil
}

// snapshot loads the current pricing override set and a monotonic version
// string. version is the latest channel UpdatedAt unix-nano; ties are
// resolved by ID so two channels with the same UpdatedAt do not collide.
func (s *Server) snapshot(ctx context.Context) ([]*pb.PricingOverride, string, error) {
	lister := s.loadLister()
	if lister == nil {
		return nil, "", status.Error(codes.Unavailable,
			"channel-management: PricingExtension lister not yet wired (plugin Init pending)")
	}

	channels, err := lister.ListAll(ctx)
	if err != nil {
		return nil, "", err
	}

	// Collect group IDs to resolve the per-group platform.
	allGroups := collectGroupIDs(channels)
	platforms := map[int64]string{}
	if len(allGroups) > 0 {
		platforms, err = lister.GetGroupPlatforms(ctx, allGroups)
		if err != nil {
			return nil, "", err
		}
	}

	out := make([]*pb.PricingOverride, 0)
	var maxUpdated int64
	for i := range channels {
		ch := &channels[i]
		if !ch.IsActive() {
			continue
		}
		if u := ch.UpdatedAt.UnixNano(); u > maxUpdated {
			maxUpdated = u
		}
		for _, gid := range ch.GroupIDs {
			groupPlatform := platforms[gid]
			if groupPlatform == "" {
				continue
			}
			for j := range ch.ModelPricing {
				p := &ch.ModelPricing[j]
				if !platformMatches(groupPlatform, p.Platform) {
					continue
				}
				for _, model := range p.Models {
					modelLower := strings.ToLower(strings.TrimSpace(model))
					if modelLower == "" {
						continue
					}
					out = append(out, encodeOverride(gid, groupPlatform, modelLower, p))
				}
			}
		}
	}

	s.mu.Lock()
	if maxUpdated == 0 {
		maxUpdated = time.Now().UnixNano()
	}
	s.version = formatVersion(maxUpdated)
	v := s.version
	s.mu.Unlock()
	return out, v, nil
}

// collectGroupIDs flattens every channel's GroupIDs into a deduplicated
// slice. Order is not significant because the platform map is keyed by ID.
func collectGroupIDs(channels []chService.Channel) []int64 {
	seen := make(map[int64]struct{})
	out := make([]int64, 0)
	for i := range channels {
		for _, gid := range channels[i].GroupIDs {
			if _, ok := seen[gid]; ok {
				continue
			}
			seen[gid] = struct{}{}
			out = append(out, gid)
		}
	}
	return out
}

// platformMatches reports whether a pricing row's platform applies to a
// group's platform. Mirrors chService.isPlatformPricingMatch (strict equality
// on lowercase) without re-exporting the helper.
func platformMatches(groupPlatform, pricingPlatform string) bool {
	return strings.ToLower(strings.TrimSpace(groupPlatform)) ==
		strings.ToLower(strings.TrimSpace(pricingPlatform))
}

// encodeOverride converts one (group, platform, model) tuple plus its
// matching ChannelModelPricing into a proto PricingOverride.
func encodeOverride(
	groupID int64,
	platform string,
	model string,
	p *chService.ChannelModelPricing,
) *pb.PricingOverride {
	return &pb.PricingOverride{
		Key: &pb.PricingOverrideKey{
			GroupId:  groupID,
			Platform: platform,
			Model:    model,
		},
		BillingMode:      string(p.BillingMode),
		InputPrice:       deref(p.InputPrice),
		OutputPrice:      deref(p.OutputPrice),
		CacheWritePrice:  deref(p.CacheWritePrice),
		CacheReadPrice:   deref(p.CacheReadPrice),
		ImageOutputPrice: deref(p.ImageOutputPrice),
		PerRequestPrice:  deref(p.PerRequestPrice),
		Intervals:        encodeIntervals(p.Intervals),
	}
}

// encodeIntervals translates the plugin's PricingInterval list to proto.
// nil prices map to zero (the proto contract treats zero as "unset").
func encodeIntervals(in []chService.PricingInterval) []*pb.PricingInterval {
	if len(in) == 0 {
		return nil
	}
	out := make([]*pb.PricingInterval, 0, len(in))
	for i := range in {
		iv := &in[i]
		var maxTokens int64
		if iv.MaxTokens != nil {
			maxTokens = int64(*iv.MaxTokens)
		}
		out = append(out, &pb.PricingInterval{
			MinTokens:       int64(iv.MinTokens),
			MaxTokens:       maxTokens,
			InputPrice:      deref(iv.InputPrice),
			OutputPrice:     deref(iv.OutputPrice),
			CacheWritePrice: deref(iv.CacheWritePrice),
			CacheReadPrice:  deref(iv.CacheReadPrice),
			PerRequestPrice: deref(iv.PerRequestPrice),
		})
	}
	return out
}

// deref returns the dereferenced value, treating nil as zero. Callers feed
// the result into proto fields where zero already means "unset".
func deref(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

// formatVersion encodes an int64 unix-nano as a fixed-width decimal string
// so lexicographic compare matches numeric compare.
func formatVersion(n int64) string {
	const width = 20
	digits := make([]byte, 0, width)
	if n < 0 {
		n = 0
	}
	for n > 0 {
		digits = append(digits, byte('0'+n%10))
		n /= 10
	}
	for len(digits) < width {
		digits = append(digits, '0')
	}
	// Reverse in place.
	for i, j := 0, len(digits)-1; i < j; i, j = i+1, j-1 {
		digits[i], digits[j] = digits[j], digits[i]
	}
	return string(digits)
}
