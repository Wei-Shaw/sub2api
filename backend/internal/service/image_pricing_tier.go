package service

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const (
	MaxImageAspectRatio = 3
	MaxImageLongSide    = 3840
)

var defaultImageTierResolutions = map[string]string{
	"1K": "1024x1024",
	"2K": "2048x2048",
	"4K": "4096x4096",
}

type ImageDimensions struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

func (d ImageDimensions) ShortSide() int {
	if d.Width < d.Height {
		return d.Width
	}
	return d.Height
}

func (d ImageDimensions) LongSide() int {
	if d.Width > d.Height {
		return d.Width
	}
	return d.Height
}

func (d ImageDimensions) Pixels() int64 {
	return int64(d.Width) * int64(d.Height)
}

func (d ImageDimensions) String() string {
	return fmt.Sprintf("%dx%d", d.Width, d.Height)
}

func ParseImageDimensions(value string) (ImageDimensions, error) {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(value)), "x")
	if len(parts) != 2 {
		return ImageDimensions{}, fmt.Errorf("resolution must use WIDTHxHEIGHT format")
	}
	width, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || width <= 0 {
		return ImageDimensions{}, fmt.Errorf("resolution width must be a positive integer")
	}
	height, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || height <= 0 {
		return ImageDimensions{}, fmt.Errorf("resolution height must be a positive integer")
	}
	return ImageDimensions{Width: width, Height: height}, nil
}

func ParseImageRequestDimensions(value string) (ImageDimensions, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "square":
		return ImageDimensions{Width: 512, Height: 512}, nil
	case "square_hd":
		return ImageDimensions{Width: 1024, Height: 1024}, nil
	case "portrait_4_3":
		return ImageDimensions{Width: 768, Height: 1024}, nil
	case "portrait_16_9":
		return ImageDimensions{Width: 576, Height: 1024}, nil
	case "landscape_4_3":
		return ImageDimensions{Width: 1024, Height: 768}, nil
	case "landscape_16_9":
		return ImageDimensions{Width: 1024, Height: 576}, nil
	default:
		return ParseImageDimensions(value)
	}
}

// ImagePricingTier uses Resolution as a tier bound. A request matches the
// first tier whose short edge fits; requests beyond the highest configured
// tier are billed at the highest tier, subject to the global long-edge cap.
type ImagePricingTier struct {
	Label      string
	Resolution string
	Price      *float64
}

type normalizedImagePricingTier struct {
	ImagePricingTier
	dimensions ImageDimensions
}

func MatchImagePricingTier(dimensions ImageDimensions, tiers []ImagePricingTier) (*ImagePricingTier, error) {
	if dimensions.Width <= 0 || dimensions.Height <= 0 {
		return nil, fmt.Errorf("image width and height must be positive")
	}
	shortSide := dimensions.ShortSide()
	longSide := dimensions.LongSide()
	if longSide > MaxImageLongSide {
		return nil, fmt.Errorf("image long side must not exceed %d pixels", MaxImageLongSide)
	}
	if int64(longSide) > int64(shortSide)*MaxImageAspectRatio {
		return nil, fmt.Errorf("image aspect ratio must not exceed 1:%d", MaxImageAspectRatio)
	}

	normalized, err := normalizeImagePricingTiers(tiers)
	if err != nil {
		return nil, err
	}
	if len(normalized) == 0 {
		return nil, fmt.Errorf("no image pricing tiers configured")
	}

	last := normalized[len(normalized)-1]
	for i := range normalized {
		tier := &normalized[i]
		if shortSide > tier.dimensions.ShortSide() {
			continue
		}
		matched := tier.ImagePricingTier
		return &matched, nil
	}
	// A valid request that exceeds the highest configured tier uses the
	// highest tier price instead of becoming unbillable.
	matched := last.ImagePricingTier
	return &matched, nil
}

func normalizeImagePricingTiers(tiers []ImagePricingTier) ([]normalizedImagePricingTier, error) {
	normalized := make([]normalizedImagePricingTier, 0, len(tiers))
	seen := make(map[string]struct{}, len(tiers))
	for _, tier := range tiers {
		label := imageTierLabel(tier.Label)
		if label == "" {
			return nil, fmt.Errorf("image pricing tier must be one of 1K, 2K, or 4K")
		}
		if _, exists := seen[label]; exists {
			return nil, fmt.Errorf("duplicate image pricing tier %s", label)
		}
		seen[label] = struct{}{}
		resolution := strings.TrimSpace(tier.Resolution)
		if resolution == "" {
			resolution = defaultImageTierResolutions[label]
		}
		dimensions, err := ParseImageDimensions(resolution)
		if err != nil {
			return nil, fmt.Errorf("invalid resolution for image pricing tier %s: %w", label, err)
		}
		normalized = append(normalized, normalizedImagePricingTier{
			ImagePricingTier: ImagePricingTier{Label: label, Resolution: dimensions.String(), Price: tier.Price},
			dimensions:       dimensions,
		})
	}
	sort.SliceStable(normalized, func(i, j int) bool {
		return imagePricingTierRank(normalized[i].Label) < imagePricingTierRank(normalized[j].Label)
	})
	for i := 1; i < len(normalized); i++ {
		prev := normalized[i-1].dimensions
		current := normalized[i].dimensions
		if current.ShortSide() < prev.ShortSide() || current.LongSide() < prev.LongSide() || current.Pixels() <= prev.Pixels() {
			return nil, fmt.Errorf("image pricing tier resolution bounds must not decrease and total pixels must increase")
		}
	}
	return normalized, nil
}

func imagePricingTierRank(label string) int {
	switch imageTierLabel(label) {
	case "1K":
		return 0
	case "2K":
		return 1
	case "4K":
		return 2
	default:
		return 3
	}
}

func imageTierLabel(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "1K":
		return "1K"
	case "2K":
		return "2K"
	case "4K":
		return "4K"
	default:
		return ""
	}
}

func normalizeGroupImageTierResolutions(oneK, twoK, fourK string) ([3]string, error) {
	normalized, err := normalizeImagePricingTiers([]ImagePricingTier{
		{Label: "1K", Resolution: oneK},
		{Label: "2K", Resolution: twoK},
		{Label: "4K", Resolution: fourK},
	})
	if err != nil {
		return [3]string{}, err
	}
	byLabel := make(map[string]string, len(normalized))
	for _, tier := range normalized {
		byLabel[tier.Label] = tier.Resolution
	}
	return [3]string{byLabel["1K"], byLabel["2K"], byLabel["4K"]}, nil
}

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
		return ImageQualityHigh
	}
}

func SortedImagePricingTiers() []string {
	return []string{"1K", "2K", "4K"}
}

func SortedImageQualities() []string {
	return []string{ImageQualityLow, ImageQualityMedium, ImageQualityHigh}
}
