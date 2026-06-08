// server_encode.go owns the pricing snapshot + override encoding helpers
// (encodeChannelsAsOverrides / expandChannelOverrides / encodeOverride /
// encodeIntervals / bumpVersion / platformMatches / collectGroupIDs /
// formatVersion). Splitting them out of server.go (T48 + T53) keeps the
// RPC entry points focused on lifecycle + flow control; encoding concerns
// stay co-located with the data they shape.

package pricing

import (
	"strings"
	"time"

	sdkdecimalx "github.com/Wei-Shaw/sub2api/plugin-sdk/decimalx"
	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	"github.com/Wei-Shaw/sub2api/plugins/channel-management/internal/platforms"
	chService "github.com/Wei-Shaw/sub2api/plugins/channel-management/service"
)

// encodeChannelsAsOverrides walks every active channel, expands each model
// pricing row per attached group, and produces the flat proto list plus the
// monotonic max UpdatedAt used as the snapshot version.
func encodeChannelsAsOverrides(
	channels []chService.Channel,
	platforms map[int64]string,
) ([]*pb.PricingOverride, int64) {
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
		out = append(out, expandChannelOverrides(ch, platforms)...)
	}
	return out, maxUpdated
}

// expandChannelOverrides fans one channel out into the (group × platform ×
// model) override rows the host cache expects. Factored out so
// encodeChannelsAsOverrides stays a single loop — adding a new dimension to
// the expansion (e.g. service_tier) means extending this helper, not the
// top-level encoder.
func expandChannelOverrides(ch *chService.Channel, platforms map[int64]string) []*pb.PricingOverride {
	rows := make([]*pb.PricingOverride, 0)
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
				rows = append(rows, encodeOverride(gid, groupPlatform, modelLower, p))
			}
		}
	}
	return rows
}

// bumpVersion writes the resolved max UpdatedAt into s.version and returns
// the formatted string. Falls back to wall-clock if no channels were
// active — keeps consecutive snapshots distinguishable.
func (s *Server) bumpVersion(maxUpdated int64) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if maxUpdated == 0 {
		maxUpdated = time.Now().UnixNano()
	}
	s.version = formatVersion(maxUpdated)
	return s.version
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
// group's platform. Single source: platforms.Match (trim + lowercase + strict
// equality). chService.isPlatformPricingMatch reuses the same package, so
// both gRPC publish and in-process cache go through the same rule.
func platformMatches(groupPlatform, pricingPlatform string) bool {
	return platforms.Match(groupPlatform, pricingPlatform)
}

// encodeOverride converts one (group, platform, model) tuple plus its
// matching ChannelModelPricing into a proto PricingOverride.
//
// T24: prices are encoded as decimal strings (ToProtoString) so the
// canonical decimal.NullDecimal value flows end-to-end from the plugin
// repo to the host cache without crossing IEEE-754. Unset prices map to
// "" — the host treats empty string as "not set" and falls back to base
// pricing, matching the legacy zero-as-unset semantics.
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
		InputPrice:       sdkdecimalx.ToProtoString(p.InputPrice),
		OutputPrice:      sdkdecimalx.ToProtoString(p.OutputPrice),
		CacheWritePrice:  sdkdecimalx.ToProtoString(p.CacheWritePrice),
		CacheReadPrice:   sdkdecimalx.ToProtoString(p.CacheReadPrice),
		ImageOutputPrice: sdkdecimalx.ToProtoString(p.ImageOutputPrice),
		PerRequestPrice:  sdkdecimalx.ToProtoString(p.PerRequestPrice),
		Intervals:        encodeIntervals(p.Intervals),
	}
}

// encodeIntervals translates the plugin's PricingInterval list to proto.
// Unset prices map to "" (T24: the proto contract treats empty string as
// "not set"). Numeric values flow through sdkdecimalx.ToProtoString so no
// round-trip through float64 happens.
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
			MinTokens:        int64(iv.MinTokens),
			MaxTokens:        maxTokens,
			InputPrice:       sdkdecimalx.ToProtoString(iv.InputPrice),
			OutputPrice:      sdkdecimalx.ToProtoString(iv.OutputPrice),
			CacheWritePrice:  sdkdecimalx.ToProtoString(iv.CacheWritePrice),
			CacheReadPrice:   sdkdecimalx.ToProtoString(iv.CacheReadPrice),
			ImageOutputPrice: sdkdecimalx.ToProtoString(iv.ImageOutputPrice),
			PerRequestPrice:  sdkdecimalx.ToProtoString(iv.PerRequestPrice),
		})
	}
	return out
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
