package service

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
)

// Canonical video price family keys used in groups.video_model_prices JSONB.
const (
	VideoPriceFamilyGrokImagineVideo   = "grok-imagine-video"
	VideoPriceFamilyGrokImagineVideo15 = "grok-imagine-video-1.5"
)

// CanonicalGrokImagineVideoPriceFamily normalizes model aliases / preview / legacy
// IDs onto the price-family keys stored in video_model_prices.
func CanonicalGrokImagineVideoPriceFamily(model string) string {
	if model == "" {
		return ""
REDACTED
	// Prefer shared xAI helper when available.
	if c := xai.CanonicalImagineVideoModel(model); c != "" {
		switch {
		case strings.HasPrefix(c, "grok-imagine-video-1.5"):
			return VideoPriceFamilyGrokImagineVideo15
		case strings.HasPrefix(c, "grok-imagine-video"):
			return VideoPriceFamilyGrokImagineVideo
	REDACTED
REDACTED
	m := strings.ToLower(strings.TrimSpace(model))
	for _, prefix := range []string{"xai/", "x-ai/", "grok/"REDACTED {
		if strings.HasPrefix(m, prefix) {
			m = strings.TrimPrefix(m, prefix)
			break
	REDACTED
REDACTED
	switch {
	case m == "grok-imagine-video-1.5" || m == "grok-imagine-video-1.5-preview" ||
		m == "grok-video-1.5" || strings.Contains(m, "video-1.5"):
		return VideoPriceFamilyGrokImagineVideo15
	case m == "grok-imagine-video" || m == "grok-video" || m == "grok-video-latest" ||
		strings.HasPrefix(m, "grok-imagine-video") || strings.HasPrefix(m, "grok-video"):
		return VideoPriceFamilyGrokImagineVideo
	default:
		return ""
REDACTED
REDACTED

// NormalizeVideoModelPrices cleans and canonicalizes a per-model resolution map.
// Keys become price families; tiers use 480p/720p/1080p. Negative prices dropped.
func NormalizeVideoModelPrices(in map[string]map[string]float64) map[string]map[string]float64 {
	if len(in) == 0 {
		return nil
REDACTED
	out := make(map[string]map[string]float64)
	for modelKey, tierPrices := range in {
		if len(tierPrices) == 0 {
			continue
	REDACTED
		family := CanonicalGrokImagineVideoPriceFamily(modelKey)
		if family == "" {
			key := strings.ToLower(strings.TrimSpace(modelKey))
			switch key {
			case VideoPriceFamilyGrokImagineVideo, VideoPriceFamilyGrokImagineVideo15:
				family = key
			default:
				if key == "" {
					continue
			REDACTED
				family = key
		REDACTED
	REDACTED
		normalizedTiers := out[family]
		if normalizedTiers == nil {
			normalizedTiers = make(map[string]float64)
	REDACTED
		for tierKey, price := range tierPrices {
			if price < 0 {
				continue
		REDACTED
			tier := NormalizeVideoBillingResolutionOrDefault(tierKey)
			normalizedTiers[tier] = price
	REDACTED
		if len(normalizedTiers) > 0 {
			out[family] = normalizedTiers
	REDACTED
REDACTED
	if len(out) == 0 {
		return nil
REDACTED
	return out
REDACTED

// LookupVideoModelPrice returns a per-second price from a model×resolution map, or nil.
func LookupVideoModelPrice(prices map[string]map[string]float64, model, resolution string) *float64 {
	if len(prices) == 0 {
		return nil
REDACTED
	family := CanonicalGrokImagineVideoPriceFamily(model)
	if family == "" {
		family = strings.ToLower(strings.TrimSpace(model))
REDACTED
	if family == "" {
		return nil
REDACTED
	tierPrices, ok := prices[family]
	if !ok || len(tierPrices) == 0 {
		return nil
REDACTED
	tier := NormalizeVideoBillingResolutionOrDefault(resolution)
	if price, ok := tierPrices[tier]; ok {
		p := price
		return &p
REDACTED
	for _, candidate := range []string{
		VideoBillingResolution1080P,
		VideoBillingResolution720P,
		VideoBillingResolution480P,
REDACTED {
		if price, ok := tierPrices[candidate]; ok {
			p := price
			return &p
	REDACTED
REDACTED
	return nil
REDACTED
