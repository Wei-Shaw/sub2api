package service

import (
	"errors"
	"math"

	"github.com/Wei-Shaw/sub2api/internal/pkg/minimax"
)

// USD list prices, international PAYG catalog, reviewed 2026-10-29.
// Missing overrides use these defaults; an explicit zero remains free.
func MiniMaxDefaultPrices() map[string]map[string]float64 {
	p := map[string]map[string]float64{}
	for _, m := range []string{minimax.SpeechHD, minimax.SpeechTurbo, "speech-2.6-hd", "speech-2.6-turbo", "speech-02-hd", "speech-02-turbo"} {
		price := 60.0
		if minimax.IsHDSpeech(m) {
			price = 100
		}
		p[m] = map[string]float64{"characters_per_million": price}
	}
	p["voice_clone"] = map[string]float64{"voice": 1.5}
	p["voice_design"] = map[string]float64{"voice": 3, "characters_per_million": 60}
	p[minimax.Image] = map[string]float64{"image": 0.0035}
	for key, v := range map[string][3]float64{
		minimax.H3 + "/768P": {0.08, 0.08, 0.04}, minimax.H3 + "/2K": {0.13, 0.13, 0.04},
		minimax.H3Highspeed + "/768P": {0.06, 0.06, 0.03},
		minimax.H3Lite + "/768P":      {0.04, 0.04, 0.02}, minimax.H3Lite + "/480P": {0.025, 0.025, 0.02},
		minimax.H3Max + "/768P": {0.08, 0.143, 0.074}, minimax.H3Max + "/480P": {0.05, 0.0553, 0.074},
	} {
		p[key] = map[string]float64{"output_second": v[0], "input_second": v[1], "extra_image": v[2]}
	}
	return p
}
func ValidateMiniMaxMediaPrices(prices map[string]map[string]float64) error {
	defaults := MiniMaxDefaultPrices()
	for card, rates := range prices {
		valid, ok := defaults[card]
		if !ok {
			return errors.New("unknown MiniMax pricing model/resolution")
		}
		for unit, v := range rates {
			if _, ok := valid[unit]; !ok || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 1e6 {
				return errors.New("invalid MiniMax media unit price")
			}
		}
	}
	return nil
}
func NormalizeMiniMaxMediaPrices(prices map[string]map[string]float64) map[string]map[string]float64 {
	if prices == nil {
		return nil
	}
	out := make(map[string]map[string]float64, len(prices))
	for k, rates := range prices {
		out[k] = make(map[string]float64, len(rates))
		for u, v := range rates {
			out[k][u] = v
		}
	}
	return out
}
func miniMaxCard(model, resolution string) string {
	if resolution != "" {
		return model + "/" + resolution
	}
	return model
}
func miniMaxRate(r MiniMaxMediaRates, unit string) *float64 {
	switch unit {
	case "characters_per_million":
		return r.CharactersPerMillion
	case "output_second":
		return r.OutputSecond
	case "input_second":
		return r.InputSecond
	case "extra_image":
		return r.ExtraImage
	case "image":
		return r.Image
	case "voice":
		return r.Voice
	}
	return nil
}
func miniMaxRates(prices map[string]float64) MiniMaxMediaRates {
	ptr := func(k string) *float64 {
		if v, ok := prices[k]; ok {
			return &v
		}
		return nil
	}
	return MiniMaxMediaRates{CharactersPerMillion: ptr("characters_per_million"), OutputSecond: ptr("output_second"), InputSecond: ptr("input_second"), ExtraImage: ptr("extra_image"), Image: ptr("image"), Voice: ptr("voice")}
}
