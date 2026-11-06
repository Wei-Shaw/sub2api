//go:build unit

package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/imagepricing"
	"github.com/stretchr/testify/require"
)

const grokImageTestExpression = `(param("resolution") == nil || param("resolution") == "1k") ? (param("quality") == "medium" ? tier("1k-medium", fixed(0.06)) : tier("1k-low", fixed(0.04))) * image_count : param("resolution") == "1.5k" ? (param("quality") == "medium" ? tier("1.5k-medium", fixed(0.07)) : tier("1.5k-low", fixed(0.05))) * image_count : (param("quality") == "medium" ? tier("2k-medium", fixed(0.08)) : tier("2k-low", fixed(0.06))) * image_count`

func TestImagePricingSnapshotOfficialTiersAndPartialOutput(t *testing.T) {
	for _, tc := range []struct {
		resolution, quality, tier string
		price                     float64
	}{
		{"1k", "low", "1k-low", .04}, {"1k", "medium", "1k-medium", .06},
		{"1.5k", "low", "1.5k-low", .05}, {"1.5k", "medium", "1.5k-medium", .07},
		{"2k", "low", "2k-low", .06}, {"2k", "medium", "2k-medium", .08},
		{"", "", "1k-low", .04}, {"", "auto", "1k-low", .04},
	} {
		t.Run(tc.tier+tc.quality, func(t *testing.T) {
			group := &Group{RateMultiplier: 1.5, ImagePriceExpressions: map[string]string{"public-image": grokImageTestExpression}}
			body := []byte(fmt.Sprintf(`{"model":"public-image","n":3,"resolution":%q,"quality":%q}`, tc.resolution, tc.quality))
			svc := &OpenAIGatewayService{}
			snapshot, err := svc.PrepareImagePricing(context.Background(), &APIKey{Group: group}, "public-image", body, true)
			require.NoError(t, err)
			require.NotNil(t, snapshot)
			group.ImagePriceExpressions["public-image"] = "fixed(999)"
			group.RateMultiplier = 999
			// Final settlement reads no mutable group price and uses actual, not requested n.
			cost, err := svc.calculateOpenAIRecordUsageCost(context.Background(), &OpenAIForwardResult{ImagePricingSnapshot: snapshot, ImageCount: 2}, &APIKey{Group: group}, []string{"upstream-model"}, 999, 999, 999, 999, UsageTokens{}, "", nil, false, time.Time{})
			require.NoError(t, err)
			require.InDelta(t, tc.price*2, cost.TotalCost, 1e-12)
			require.InDelta(t, tc.price*2*1.5, cost.ActualCost, 1e-12)
			require.Equal(t, tc.tier, snapshot.prices[2].Tier)
			estimate, ok := (inflightEstimateDeps{}).estimate(context.Background(), nil, InflightEstimateRequest{ImagePricingSnapshot: snapshot})
			require.True(t, ok)
			require.InDelta(t, tc.price*3*1.5, estimate, 1e-12)
			zero, err := snapshot.Cost(0)
			require.NoError(t, err)
			require.Zero(t, zero.ActualCost)
		})
	}
}

func TestImagePricingPreflightFrozenNAndAllCountValidation(t *testing.T) {
	facts := imagepricing.Facts{Model: "image", RequestedCount: 3, ImageCount: 3}
	snapshot, err := newImagePricingSnapshot(`fixed(param("n") == 3 ? 0.04 : 9) * image_count`, facts, 1)
	require.NoError(t, err)
	cost, err := snapshot.Cost(2)
	require.NoError(t, err)
	require.InDelta(t, .08, cost.TotalCost, 1e-12)
	_, err = newImagePricingSnapshot(`image_count == 2 ? -1 : 0.04 * image_count`, facts, 1)
	require.Error(t, err, "a partial-result branch must fail before upstream work")
	_, err = newImagePricingSnapshot(`fixed(0.04)*image_count`, facts, -1)
	require.Error(t, err)
}

func TestImagePricingPreflightRejectsUnsupportedOrAmbiguousRequests(t *testing.T) {
	svc := &OpenAIGatewayService{}
	key := &APIKey{Group: &Group{RateMultiplier: 1, ImagePriceExpressions: map[string]string{"image": "fixed(0.04)*image_count"}}}
	for _, body := range []string{
		`{"n":0}`, `{"n":1.5}`, `{"n":129}`, `{"n":"2"}`, `{"stream":true}`, `{"deferred":true}`,
		`{"image":"https://example.test/input.png"}`, `{"images":[]}`, `{"mask":null}`, `{"quality":3}`,
		`{"quality":"low","quality":"medium"}`, `{"Resolution":"2k"}`, `[]`,
	} {
		t.Run(body, func(t *testing.T) {
			_, err := svc.PrepareImagePricing(context.Background(), key, "image", []byte(body), true)
			require.Error(t, err)
		})
	}
	unmatched, err := svc.PrepareImagePricing(context.Background(), key, "IMAGE", []byte(`{"stream":true}`), true)
	require.NoError(t, err)
	require.Nil(t, unmatched, "matching is exact and legacy models are unchanged")
}

func TestImagePricingFactsMatchFinalGrokGeometry(t *testing.T) {
	svc := &OpenAIGatewayService{}
	key := &APIKey{Group: &Group{RateMultiplier: 1, ImagePriceExpressions: map[string]string{"image": grokImageTestExpression}}}
	for _, tc := range []struct{ body, resolution, aspect string }{
		{`{"resolution":"1.5k","size":"2048x1024","quality":"medium"}`, "1.5k", "2:1"},
		{`{"size":"2048x1024"}`, "2k", "2:1"},
	} {
		snapshot, err := svc.PrepareImagePricing(context.Background(), key, "image", []byte(tc.body), true)
		require.NoError(t, err)
		require.Equal(t, tc.resolution, snapshot.facts.Resolution)
		require.Equal(t, tc.aspect, snapshot.facts.AspectRatio)
		final, err := applyGrokImagineImageGeometry([]byte(tc.body))
		require.NoError(t, err)
		ctx := WithImagePricingSnapshot(context.Background(), snapshot)
		require.NoError(t, validateImagePricingForwardBody(ctx, final))
		require.Error(t, validateImagePricingForwardBody(ctx, []byte(`{"resolution":"1k"}`)))
	}
}

func TestImageExpressionEndpointSupport(t *testing.T) {
	require.True(t, ImageExpressionEndpointSupported("/v1/images/generations", "application/json"))
	for _, path := range []string{"/v1/images/edits", "/v1/images/generations/async", "/v1/responses"} {
		require.False(t, ImageExpressionEndpointSupported(path, "application/json"))
	}
	require.False(t, ImageExpressionEndpointSupported("/v1/images/generations", "multipart/form-data; boundary=x"))
}

func TestImagePricingNativeGrokTransportPreservesBillingFacts(t *testing.T) {
	body := []byte(`{"model":"grok-imagine-image-2.0","prompt":"draw","n":3,"resolution":"1.5k","quality":"medium","size":"2048x1024"}`)
	upstream := &grokMediaContentUpstreamStub{response: grokMediaContentStatusResponse(`{"data":[{"url":"https://example.test/1.png"},{"url":"https://example.test/2.png"}]}`)}
	svc := &OpenAIGatewayService{httpUpstream: upstream}
	key := &APIKey{Group: &Group{RateMultiplier: 1, ImagePriceExpressions: map[string]string{"grok-imagine-image-2.0": grokImageTestExpression}}}
	snapshot, err := svc.PrepareImagePricing(context.Background(), key, "grok-imagine-image-2.0", body, true)
	require.NoError(t, err)
	ctx := WithImagePricingSnapshot(context.Background(), snapshot)
	c, _ := grokMediaContentTestContext("POST", "https://api.example/v1/images/generations", nil)
	result, err := svc.ForwardGrokMedia(ctx, c, grokMediaContentTestAccount(), GrokMediaEndpointImagesGenerations, "", body, "application/json")
	require.NoError(t, err)
	require.Equal(t, 2, result.ImageCount)
	cost, err := snapshot.Cost(result.ImageCount)
	require.NoError(t, err)
	require.InDelta(t, .14, cost.ActualCost, 1e-12)
	require.Len(t, upstream.requests, 1)
	// A changed attempt must be refused before making another upstream request.
	_, err = svc.ForwardGrokMedia(ctx, c, grokMediaContentTestAccount(), GrokMediaEndpointImagesGenerations, "", []byte(`{"model":"grok-imagine-image-2.0","n":3,"resolution":"2k","quality":"medium"}`), "application/json")
	require.ErrorContains(t, err, "changed after preflight")
	require.Len(t, upstream.requests, 1)
}

func TestImagePricingReservationCoversNonMonotonicPartialCost(t *testing.T) {
	snapshot, err := newImagePricingSnapshot(`image_count == 3 ? 0.10 : 1`, imagepricing.Facts{Model: "image", RequestedCount: 3, ImageCount: 3}, 2)
	require.NoError(t, err)
	estimate, ok := (inflightEstimateDeps{}).estimate(context.Background(), nil, InflightEstimateRequest{ImagePricingSnapshot: snapshot})
	require.True(t, ok)
	require.InDelta(t, 2, estimate, 1e-12)
}

func TestImageExpressionOpenAICompatibleSize(t *testing.T) {
	svc := &OpenAIGatewayService{}
	key := &APIKey{Group: &Group{RateMultiplier: 1, ImagePriceExpressions: map[string]string{"image": `fixed(param("size") == "1536x1024" ? 0.08 : 0.04) * image_count`}}}
	snapshot, err := svc.PrepareImagePricing(context.Background(), key, "image", []byte(`{"size":"1536x1024"}`), false)
	require.NoError(t, err)
	cost, err := snapshot.Cost(1)
	require.NoError(t, err)
	require.InDelta(t, .08, cost.TotalCost, 1e-12)
	require.Equal(t, "1536x1024", snapshot.facts.Size)
}

func TestImageExpressionBatchRejectedBeforePricingSnapshot(t *testing.T) {
	groupID := int64(42)
	svc := &BatchImagePublicService{GroupRepo: &publicBatchImageGroupRepo{groups: map[int64]*Group{groupID: {ID: groupID, AllowBatchImageGeneration: true, ImagePriceExpressions: map[string]string{"image": "0.04*image_count"}}}}}
	snapshot, err := svc.resolvePricingSnapshot(context.Background(), BatchImageOwner{GroupID: &groupID}, BatchImageSubmitRequest{Model: "image"}, "gemini", nil)
	require.ErrorContains(t, err, ImageExpressionUnsupportedMessage)
	require.Nil(t, snapshot)
}

func TestImageExpressionRejectsOversizedScaledCostBeforeUpstream(t *testing.T) {
	_, err := newImagePricingSnapshot(`fixed(1000000)`, imagepricing.Facts{Model: "image", ImageCount: 1, RequestedCount: 1}, 10000)
	require.ErrorContains(t, err, "invalid image price after multiplier")
}
