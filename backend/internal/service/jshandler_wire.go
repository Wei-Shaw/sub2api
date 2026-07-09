package service

import (
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service/jshandler"
)

// ProvideJSHandlerService wires the gateway JavaScript hook layer.
func ProvideJSHandlerService(settingRepo SettingRepository, cfg *config.Config) *jshandler.Service {
	return jshandler.NewService(settingRepo, cfg)
}