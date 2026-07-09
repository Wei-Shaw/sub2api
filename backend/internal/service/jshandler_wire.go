package service

import (
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service/jshandler"
)

// ProvideJSHandlerService wires the gateway JavaScript hook layer.
func ProvideJSHandlerService(settingRepo SettingRepository, cfg *config.Config, settingService *SettingService) *jshandler.Service {
	svc := jshandler.NewService(settingRepo, cfg)
	if settingService != nil {
		settingService.AddOnUpdateCallback(svc.InvalidateCache)
	}
	return svc
}