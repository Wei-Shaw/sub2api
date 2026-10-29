package service

import (
	"math"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Official Imagine image geometry: https://docs.x.ai/developers/model-capabilities/images/generation
var grokImagineAspectRatioValues = []struct {
	label string
	ratio float64
}{
	{"1:1", 1},
	{"16:9", 16.0 / 9.0},
	{"9:16", 9.0 / 16.0},
	{"4:3", 4.0 / 3.0},
	{"3:4", 3.0 / 4.0},
	{"3:2", 1.5},
	{"2:3", 2.0 / 3.0},
	{"2:1", 2},
	{"1:2", 0.5},
	{"19.5:9", 19.5 / 9.0},
	{"9:19.5", 9.0 / 19.5},
	{"20:9", 20.0 / 9.0},
	{"9:20", 9.0 / 20.0},
}

func applyGrokImagineImageGeometry(body []byte) ([]byte, error) {
	size := strings.TrimSpace(gjson.GetBytes(body, "size").String())
	resolution := grokImagineImageResolution(gjson.GetBytes(body, "resolution").String())
	aspect := strings.TrimSpace(gjson.GetBytes(body, "aspect_ratio").String())
	out := append([]byte(nil), body...)

	if resolution == "" {
		if derived := grokImagineImageResolutionFromSize(size); derived != "" {
			next, err := sjson.SetBytes(out, "resolution", derived)
			if err != nil {
				return nil, err
			}
			out = next
		}
	} else if gjson.GetBytes(body, "resolution").String() != resolution {
		next, err := sjson.SetBytes(out, "resolution", resolution)
		if err != nil {
			return nil, err
		}
		out = next
	}

	if aspect == "" {
		if derived := grokImagineAspectRatioFromSize(size); derived != "" {
			next, err := sjson.SetBytes(out, "aspect_ratio", derived)
			if err != nil {
				return nil, err
			}
			out = next
		}
	}

	if !gjson.GetBytes(out, "size").Exists() {
		return out, nil
	}
	return sjson.DeleteBytes(out, "size")
}

func assignGrokMediaResolution(value string, info *GrokMediaRequestInfo) {
	if info == nil {
		return
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	if img := grokImagineImageResolution(value); img != "" {
		info.ImageResolution = img
		return
	}
	info.Resolution = value
}

func grokImagineImageResolution(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1k":
		return "1k"
	case "2k":
		return "2k"
	default:
		return ""
	}
}

func grokImagineImageResolutionFromSize(size string) string {
	if explicit := grokImagineImageResolution(size); explicit != "" {
		return explicit
	}
	tier, ok := ClassifyImageBillingTier(size)
	if !ok {
		return ""
	}
	if tier == ImageBillingSize1K {
		return "1k"
	}
	return "2k"
}

func grokImagineAspectRatioFromSize(size string) string {
	width, height, ok := parseImageBillingDimensions(strings.TrimSpace(size))
	if !ok || width <= 0 || height <= 0 {
		return ""
	}
	div := grokImagineGCD(width, height)
	exact := strconv.Itoa(width/div) + ":" + strconv.Itoa(height/div)
	for _, candidate := range grokImagineAspectRatioValues {
		if candidate.label == exact {
			return exact
		}
	}
	ratio := float64(width) / float64(height)
	bestLabel := ""
	bestDelta := math.MaxFloat64
	for _, candidate := range grokImagineAspectRatioValues {
		delta := math.Abs(ratio - candidate.ratio)
		if delta < bestDelta {
			bestDelta = delta
			bestLabel = candidate.label
		}
	}
	return bestLabel
}

func grokImagineGCD(a, b int) int {
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	for b != 0 {
		a, b = b, a%b
	}
	if a == 0 {
		return 1
	}
	return a
}
