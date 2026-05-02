// Package handler — hostPricingLookup adapter.
//
// Bridges pluginsdk.HostClient.ResolveModelPricing (which returns
// *decimal.Decimal fields) to the handler.ModelPricingLookup interface
// (which expects float64 fields). This adapter is the only piece that
// was missing to make the "model default pricing" autofill work: the
// plugin was passing nil as the pricingLookup argument to
// NewChannelHandler, causing the GET /admin/channels/model-pricing
// endpoint to unconditionally return {found: false}.
//
// Error handling follows the same pattern as
// service/channel_available.go: ErrHostPricingUnavailable degrades
// gracefully to nil (handler renders found=false), transient errors
// are surfaced so the handler can also render found=false.
package handler

import (
	"context"
	"errors"

	"github.com/shopspring/decimal"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
)

// hostPricingLookup adapts pluginsdk.HostClient to ModelPricingLookup,
// bridging the SDK's *decimal.Decimal fields to the handler's float64
// fields.
type hostPricingLookup struct {
	host pluginsdk.HostClient
}

// NewHostPricingLookup returns a ModelPricingLookup backed by the SDK
// HostClient. Returns nil when host is nil so callers can pass the
// result directly to NewChannelHandler without a nil-check.
func NewHostPricingLookup(host pluginsdk.HostClient) ModelPricingLookup {
	if host == nil {
		return nil
	}
	return &hostPricingLookup{host: host}
}

func (a *hostPricingLookup) GetModelPricing(model string) (*ModelDefaultPricing, error) {
	hp, err := a.host.ResolveModelPricing(context.Background(), model)
	if err != nil {
		if errors.Is(err, pluginsdk.ErrHostPricingUnavailable) {
			return nil, nil
		}
		return nil, err
	}
	if hp == nil {
		return nil, nil
	}
	return &ModelDefaultPricing{
		InputPricePerToken:         decimalPtrToFloat64(hp.InputPrice),
		OutputPricePerToken:        decimalPtrToFloat64(hp.OutputPrice),
		CacheCreationPricePerToken: decimalPtrToFloat64(hp.CacheWritePrice),
		CacheReadPricePerToken:     decimalPtrToFloat64(hp.CacheReadPrice),
		ImageOutputPricePerToken:   decimalPtrToFloat64(hp.ImageOutputPrice),
	}, nil
}

// decimalPtrToFloat64 converts a *decimal.Decimal to float64. nil
// collapses to 0 which matches the handler's JSON output contract
// (absent fields serialise as 0, and the frontend interprets 0 as
// "no price configured" for that dimension).
func decimalPtrToFloat64(d *decimal.Decimal) float64 {
	if d == nil {
		return 0
	}
	f, _ := d.Float64()
	return f
}
