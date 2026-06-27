package service

import (
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// imagePricingMatrixCellMaxUSD 单格价格上限阈值（USD per image）。
//
// 取值依据 spec：当前公开的最贵档 3840x2160 + high = $0.401，
// 给一倍冗余防止误填（例如把 4.01 当作 0.401 输入），同时阻止显然非法的
// 大额价格（防止恶意用户开高价倒收用户额度）。
const imagePricingMatrixCellMaxUSD = 1.0

// validateImagePricingMatrix 校验二维定价矩阵：
//  1. tier_key 必须属于 6 档之一
//  2. quality_key 必须属于 low/medium/high
//  3. 每格价格 ≥ 0 且 ≤ imagePricingMatrixCellMaxUSD
//  4. 空矩阵（nil 或全空 row）合法 —— 视为分组未配置
func validateImagePricingMatrix(matrix domain.ImagePricingMatrix) error {
	if len(matrix) == 0 {
		return nil
	}
	allowedTiers := map[string]struct{}{}
	for _, t := range SortedImagePricingTiers() {
		allowedTiers[t] = struct{}{}
	}
	allowedQualities := map[string]struct{}{
		ImageQualityLow:    {},
		ImageQualityMedium: {},
		ImageQualityHigh:   {},
	}

	for tierKey, row := range matrix {
		if _, ok := allowedTiers[tierKey]; !ok {
			return fmt.Errorf("image_pricing_matrix: unknown tier %q", tierKey)
		}
		for qualityKey, price := range row {
			if _, ok := allowedQualities[qualityKey]; !ok {
				return fmt.Errorf("image_pricing_matrix[%s]: unknown quality %q", tierKey, qualityKey)
			}
			if price < 0 {
				return fmt.Errorf("image_pricing_matrix[%s][%s]: price must be >= 0", tierKey, qualityKey)
			}
			if price > imagePricingMatrixCellMaxUSD {
				return fmt.Errorf(
					"image_pricing_matrix[%s][%s]: price %.4f exceeds upper bound %.2f",
					tierKey, qualityKey, price, imagePricingMatrixCellMaxUSD,
				)
			}
		}
	}
	return nil
}

// normalizeImagePricingMatrix 删除空 row 与不合法 key（防御性，
// 仅用于落库前最后一道清洗；实际上 validateImagePricingMatrix 已挡住非法值）。
func normalizeImagePricingMatrix(matrix domain.ImagePricingMatrix) domain.ImagePricingMatrix {
	if len(matrix) == 0 {
		return nil
	}
	out := make(domain.ImagePricingMatrix, len(matrix))
	for tierKey, row := range matrix {
		if len(row) == 0 {
			continue
		}
		copied := make(map[string]float64, len(row))
		for qualityKey, price := range row {
			copied[qualityKey] = price
		}
		out[tierKey] = copied
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
