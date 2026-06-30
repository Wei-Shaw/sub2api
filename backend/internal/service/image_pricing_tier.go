package service

import (
	"strings"
)

// ===== Image pricing tier (6 档分辨率) =====
//
// 见 spec media-prepay-billing「图片定价矩阵」与 design.md D1：
// 计费按图片像素总数（width*height）向上取最近档；
// 所有 > 3840x2160 的输入封顶到 3840x2160（4K UHD）。
//
// 6 档代表分辨率与像素总数（按像素总数升序）：
//
//	1024x768   ->   786432
//	1024x1024  ->  1048576
//	1024x1536  ->  1572864
//	1920x1080  ->  2073600
//	2560x1440  ->  3686400
//	3840x2160  ->  8294400  (4K 封顶档)

const (
	ImagePricingTier1024x768  = "1024x768"
	ImagePricingTier1024x1024 = "1024x1024"
	ImagePricingTier1024x1536 = "1024x1536"
	ImagePricingTier1920x1080 = "1920x1080"
	ImagePricingTier2560x1440 = "2560x1440"
	ImagePricingTier3840x2160 = "3840x2160"
)

type imagePricingTierEntry struct {
	key    string
	width  int
	height int
	pixels int64
}

// imagePricingTiersAsc 按像素总数升序，向上取档时遍历此切片。
var imagePricingTiersAsc = []imagePricingTierEntry{
	{ImagePricingTier1024x768, 1024, 768, 1024 * 768},
	{ImagePricingTier1024x1024, 1024, 1024, 1024 * 1024},
	{ImagePricingTier1024x1536, 1024, 1536, 1024 * 1536},
	{ImagePricingTier1920x1080, 1920, 1080, 1920 * 1080},
	{ImagePricingTier2560x1440, 2560, 1440, 2560 * 1440},
	{ImagePricingTier3840x2160, 3840, 2160, 3840 * 2160},
}

// ClassifyImagePricingTier6 把任意 (width, height) 映射到 6 档之一。
//
// 规则（见 design.md D1）：
//  1. 输入归一为正向尺寸（取 max/min 顺序无关，仅看像素总数）。
//  2. 若 width<=0 或 height<=0，返回空串和 false（让调用方自行决定回退）。
//  3. 否则按 imagePricingTiersAsc 顺序，找到首个 pixels >= 输入像素的档位。
//  4. 若超出最大档（3840x2160），封顶到 3840x2160。
func ClassifyImagePricingTier6(width, height int) (string, bool) {
	if width <= 0 || height <= 0 {
		return "", false
	}
	pixels := int64(width) * int64(height)
	for _, tier := range imagePricingTiersAsc {
		if pixels <= tier.pixels {
			return tier.key, true
		}
	}
	// 超出最大档，封顶到 4K UHD
	return ImagePricingTier3840x2160, true
}

// ParseImagePricingTier6 接受 "WxH"/"WIDTHxHEIGHT" 形式的字符串，
// 返回归一后的 6 档 tier_key。识别失败返回空串和 false。
//
// 同时兼容直接传入已经是 tier_key 的字符串（如 "1024x1024"）—— 此时
// 解析后再次走 ClassifyImagePricingTier6 会幂等命中自身档位。
func ParseImagePricingTier6(size string) (string, bool) {
	width, height, ok := parseImageBillingDimensions(size)
	if !ok {
		return "", false
	}
	return ClassifyImagePricingTier6(width, height)
}

// ===== Image quality 归一 =====
//
// 见 spec D2：客户端可能传 "low"/"medium"/"high"/"auto"/空串/任意大小写。
// 计费层归一规则：
//   - "auto"、空串、未识别值 -> "high"（auto 默认按 high 档计费，与
//     design.md「Q3 auto 默认算 high」一致）
//   - "low"/"medium"/"high" -> 自身（小写）

const (
	ImageQualityLow    = "low"
	ImageQualityMedium = "medium"
	ImageQualityHigh   = "high"
)

func NormalizeImageQuality(q string) string {
	switch strings.ToLower(strings.TrimSpace(q)) {
	case ImageQualityLow:
		return ImageQualityLow
	case ImageQualityMedium:
		return ImageQualityMedium
	case ImageQualityHigh:
		return ImageQualityHigh
	default:
		// auto / "" / 未识别 -> high
		return ImageQualityHigh
	}
}

// SortedImagePricingTiers 返回 6 档按像素升序排列的副本，
// 仅供前端/管理面板渲染或测试断言时使用。
func SortedImagePricingTiers() []string {
	out := make([]string, 0, len(imagePricingTiersAsc))
	for _, tier := range imagePricingTiersAsc {
		out = append(out, tier.key)
	}
	return out
}

// SortedImageQualities 返回 quality 三档按 low->high 顺序排列的副本。
func SortedImageQualities() []string {
	return []string{ImageQualityLow, ImageQualityMedium, ImageQualityHigh}
}
