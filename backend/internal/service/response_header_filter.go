package service

import (
	"github.com/Wei-Shaw/Nub2api/internal/config"
	"github.com/Wei-Shaw/Nub2api/internal/util/responseheaders"
)

func compileResponseHeaderFilter(cfg *config.Config) *responseheaders.CompiledHeaderFilter {
	if cfg == nil {
		return nil
	}
	return responseheaders.CompileHeaderFilter(cfg.Security.ResponseHeaders)
}
