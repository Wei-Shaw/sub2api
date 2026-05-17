package service

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	geminiImageSize1K      = "1K"
	geminiImageSize2K      = "2K"
	geminiImageSize4K      = "4K"
	geminiImageSizeDefault = geminiImageSize1K
)

// normalizeGeminiImageSize 统一 Gemini 图片尺寸到现有计费档位。
//
// 优先保留官方 imageSize 语义：
// - 512 / 1K / 2K / 4K
// - 某些客户端若传具体分辨率（如 1584x672、3168x1344），按等效像素面积映射回 1K/2K/4K。
//
// 这样可以继续沿用现有 1K/2K/4K 计费配置，而不需要为每个长宽比单独建价目表。
func normalizeGeminiImageSize(raw string) string {
	normalized := strings.ToUpper(strings.TrimSpace(raw))
	switch normalized {
	case "":
		return ""
	case "512", "0.5K":
		return geminiImageSize1K
	case geminiImageSize1K, geminiImageSize2K, geminiImageSize4K:
		return normalized
	}

	if side, err := strconv.Atoi(normalized); err == nil && side > 0 {
		return classifyGeminiImageSizeByEquivalentSide(side)
	}

	width, height, ok := parseGeminiImageResolution(normalized)
	if !ok {
		return ""
	}
	return classifyGeminiImageSizeByPixels(width * height)
}

func normalizeGeminiImageSizeOrDefault(raw string) string {
	if normalized := normalizeGeminiImageSize(raw); normalized != "" {
		return normalized
	}
	return geminiImageSizeDefault
}

func parseGeminiImageResolution(raw string) (int, int, bool) {
	for idx, r := range raw {
		if r != 'X' && r != '×' {
			continue
		}

		nextIdx := idx + utf8.RuneLen(r)
		if idx <= 0 || nextIdx >= len(raw) {
			return 0, 0, false
		}

		left := strings.TrimSpace(raw[:idx])
		right := strings.TrimSpace(raw[nextIdx:])
		width, err := strconv.Atoi(left)
		if err != nil || width <= 0 {
			return 0, 0, false
		}
		height, err := strconv.Atoi(right)
		if err != nil || height <= 0 {
			return 0, 0, false
		}
		return width, height, true
	}
	return 0, 0, false
}

func classifyGeminiImageSizeByEquivalentSide(side int) string {
	switch {
	case side <= 1536:
		return geminiImageSize1K
	case side <= 3072:
		return geminiImageSize2K
	default:
		return geminiImageSize4K
	}
}

func classifyGeminiImageSizeByPixels(pixels int) string {
	switch {
	// 1536^2: 兼容 512 和所有官方 1K 长宽比分辨率。
	case pixels <= 2359296:
		return geminiImageSize1K
	// 3072^2: 兼容所有官方 2K 长宽比分辨率。
	case pixels <= 9437184:
		return geminiImageSize2K
	default:
		return geminiImageSize4K
	}
}
