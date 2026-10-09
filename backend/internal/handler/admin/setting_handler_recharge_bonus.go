package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// rechargeBonusTiersFromDTO 请求 nil 表示未携带该字段（保持现值）；空数组表示清空阶梯。
func rechargeBonusTiersFromDTO(items *[]dto.RechargeBonusTier) *[]service.RechargeBonusTier {
	if items == nil {
		return nil
	}
	out := make([]service.RechargeBonusTier, 0, len(*items))
	for _, item := range *items {
		out = append(out, service.RechargeBonusTier{MinAmount: item.MinAmount, BonusPercent: item.BonusPercent})
	}
	return &out
}

// rechargeBonusModeToDTO 输出已归一化的模式（空/非法按 bonus）。
func rechargeBonusModeToDTO(mode string) string {
	normalized, _ := service.NormalizeRechargeBonusMode(mode)
	return normalized
}

// rechargeBonusTiersToDTO 始终返回非 nil 切片，空配置输出 []。
func rechargeBonusTiersToDTO(items []service.RechargeBonusTier) []dto.RechargeBonusTier {
	out := make([]dto.RechargeBonusTier, 0, len(items))
	for _, item := range items {
		out = append(out, dto.RechargeBonusTier{MinAmount: item.MinAmount, BonusPercent: item.BonusPercent})
	}
	return out
}
