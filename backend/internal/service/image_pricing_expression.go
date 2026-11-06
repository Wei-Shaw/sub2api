package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/imagepricing"
	"github.com/tidwall/gjson"
)

const ImageExpressionUnsupportedMessage = "Image price expressions require synchronous JSON text-to-image requests to /images/generations; this endpoint or request mode is not supported"

// ImagePricingSnapshot owns only immutable scalar request facts and precomputed
// prices. It may safely outlive the HTTP handler and configuration cache entry.
type ImagePricingSnapshot struct {
	facts      imagepricing.Facts
	multiplier float64
	prices     [imagepricing.MaxImageCount + 1]imagepricing.Result
}

func HasImagePriceExpression(group *Group, model string) bool {
	if group == nil {
		return false
	}
	_, ok := group.ImagePriceExpressions[model]
	return ok
}

// PrepareImagePricing validates every bounded output count before upstream work.
// The expression returns total USD; existing media multipliers are applied once.
func (s *OpenAIGatewayService) PrepareImagePricing(ctx context.Context, apiKey *APIKey, model string, body []byte, nativeGrok bool) (*ImagePricingSnapshot, error) {
	if apiKey != nil && apiKey.Group != nil && len(apiKey.Group.ImagePriceExpressions) > 0 && gjson.ValidBytes(body) {
		if err := validateImagePricingJSONKeys(body); err != nil {
			return nil, err
		}
	}
	if apiKey == nil || !HasImagePriceExpression(apiKey.Group, model) {
		return nil, nil
	}
	facts, err := imageExpressionFacts(model, body, nativeGrok)
	if err != nil {
		return nil, err
	}
	rate := apiKey.Group.RateMultiplier
	if apiKey.GroupID != nil && apiKey.User != nil {
		rate = s.ResolveUserGroupRateMultiplier(ctx, apiKey.User.ID, *apiKey.GroupID, apiKey.Group.RateMultiplier)
	}
	_, rate = computePeakAwareMultipliers(apiKey, rate, time.Now())
	return newImagePricingSnapshot(apiKey.Group.ImagePriceExpressions[model], facts, rate)
}

func newImagePricingSnapshot(source string, facts imagepricing.Facts, multiplier float64) (*ImagePricingSnapshot, error) {
	if math.IsNaN(multiplier) || math.IsInf(multiplier, 0) || multiplier < 0 {
		return nil, fmt.Errorf("invalid image price multiplier")
	}
	evaluator, err := imagepricing.Compile(source)
	if err != nil {
		return nil, err
	}
	snapshot := &ImagePricingSnapshot{facts: facts, multiplier: multiplier}
	for count := 0; count <= imagepricing.MaxImageCount; count++ {
		current := facts
		current.ImageCount = count
		result, err := evaluator.Evaluate(current)
		if err != nil {
			return nil, fmt.Errorf("invalid image price expression: %w", err)
		}
		if math.IsNaN(result.CostUSD*multiplier) || math.IsInf(result.CostUSD*multiplier, 0) || result.CostUSD*multiplier > imagepricing.MaxCostUSD {
			return nil, fmt.Errorf("invalid image price after multiplier")
		}
		if count == 0 {
			result.CostUSD = 0
		}
		snapshot.prices[count] = result
	}
	return snapshot, nil
}

// reservationCost covers partial results even for non-monotonic volume prices.
func (s *ImagePricingSnapshot) reservationCost() float64 {
	highest := 0.0
	for count := 1; count <= s.facts.RequestedCount; count++ {
		cost := s.prices[count].CostUSD * s.multiplier
		if cost > highest {
			highest = cost
		}
	}
	return highest
}

func (s *ImagePricingSnapshot) Cost(imageCount int) (*CostBreakdown, error) {
	if s == nil || imageCount < 0 || imageCount > imagepricing.MaxImageCount {
		return nil, fmt.Errorf("image expression output count is out of range")
	}
	total := s.prices[imageCount].CostUSD
	return &CostBreakdown{TotalCost: total, ActualCost: total * s.multiplier, BillingMode: string(BillingModeImage)}, nil
}

func imageExpressionFacts(model string, body []byte, nativeGrok bool) (imagepricing.Facts, error) {
	facts := imagepricing.Facts{Model: model, ImageCount: 1, RequestedCount: 1}
	if !gjson.ValidBytes(body) {
		return facts, fmt.Errorf("%s", ImageExpressionUnsupportedMessage)
	}
	for _, key := range []string{"image", "images", "reference_images", "mask", "image_url", "input_image"} {
		if gjson.GetBytes(body, key).Exists() {
			return facts, fmt.Errorf("%s", ImageExpressionUnsupportedMessage)
		}
	}
	for _, key := range []string{"stream", "deferred", "async"} {
		value := gjson.GetBytes(body, key)
		if value.Exists() && value.Raw != "false" && value.Raw != "null" {
			return facts, fmt.Errorf("%s", ImageExpressionUnsupportedMessage)
		}
	}
	// OpenAI Images uses string background values (transparent/opaque/auto);
	// only a boolean true could request an asynchronous background mode.
	if gjson.GetBytes(body, "background").Type == gjson.True {
		return facts, fmt.Errorf("%s", ImageExpressionUnsupportedMessage)
	}
	if n := gjson.GetBytes(body, "n"); n.Exists() {
		if n.Type != gjson.Number || n.Float() != float64(n.Int()) || n.Int() < 1 || n.Int() > imagepricing.MaxImageCount {
			return facts, fmt.Errorf("image expression n must be an integer between 1 and %d", imagepricing.MaxImageCount)
		}
		facts.ImageCount, facts.RequestedCount = int(n.Int()), int(n.Int())
	}
	if err := validateImagePricingJSONKeys(body); err != nil {
		return facts, err
	}
	if nativeGrok {
		var err error
		body, err = applyGrokImagineImageGeometry(body)
		if err != nil {
			return facts, err
		}
	}
	for key, dst := range map[string]*string{"size": &facts.Size, "resolution": &facts.Resolution, "quality": &facts.Quality, "aspect_ratio": &facts.AspectRatio} {
		value := gjson.GetBytes(body, key)
		if value.Exists() && value.Type != gjson.String && value.Type != gjson.Null {
			return facts, fmt.Errorf("image pricing parameter %s must be a string", key)
		}
		*dst = value.String()
	}
	return facts, nil
}

type imagePricingContextKey struct{}

func WithImagePricingSnapshot(ctx context.Context, snapshot *ImagePricingSnapshot) context.Context {
	if snapshot == nil {
		return ctx
	}
	return context.WithValue(ctx, imagePricingContextKey{}, snapshot)
}

func imagePricingSnapshotFromContext(ctx context.Context) *ImagePricingSnapshot {
	snapshot, _ := ctx.Value(imagePricingContextKey{}).(*ImagePricingSnapshot)
	return snapshot
}

// Reject parameter rewrites after preflight instead of charging against stale
// facts. Model mapping is intentionally allowed; selection uses the sold model.
func validateImagePricingForwardBody(ctx context.Context, body []byte) error {
	snapshot := imagePricingSnapshotFromContext(ctx)
	if snapshot == nil {
		return nil
	}
	facts, err := imageExpressionFacts(snapshot.facts.Model, body, false)
	if err != nil {
		return err
	}
	if facts != snapshot.facts {
		return fmt.Errorf("image pricing parameters changed after preflight")
	}
	return nil
}

func ImageExpressionEndpointSupported(path, contentType string) bool {
	return strings.HasSuffix(path, "/images/generations") && !strings.HasPrefix(strings.ToLower(contentType), "multipart/")
}

// Reject ambiguous top-level billing fields. gjson and upstream JSON decoders
// may otherwise disagree about duplicate keys or differently cased names.
func validateImagePricingJSONKeys(body []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return fmt.Errorf("image pricing requires a JSON object")
	}
	seen := make(map[string]bool, 8)
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return err
		}
		key, ok := keyToken.(string)
		if !ok {
			return fmt.Errorf("invalid image request key")
		}
		lower := strings.ToLower(key)
		switch lower {
		case "model", "n", "resolution", "quality", "aspect_ratio", "size", "stream", "deferred", "async", "background", "image", "images", "reference_images", "mask", "image_url", "input_image":
			if key != lower || seen[lower] {
				return fmt.Errorf("ambiguous image pricing parameter %s", key)
			}
			seen[lower] = true
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return err
		}
	}
	return nil
}
