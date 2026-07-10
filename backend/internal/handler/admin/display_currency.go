package admin

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func resolveUSDCNYRate(ctx context.Context, settingService *service.SettingService) float64 {
	if settingService == nil {
		return service.DefaultUSDCNYRate
	}
	return settingService.GetUSDCNYRate(ctx)
}
