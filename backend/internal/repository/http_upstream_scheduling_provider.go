package repository

import (
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func ProvideHTTPUpstreamWithSchedulingPools(cfg *config.Config, pools *service.AccountSchedulingPoolService) service.HTTPUpstream {
	upstream := NewHTTPUpstream(cfg)
	transport, ok := upstream.(*httpUpstreamService)
	if !ok {
		panic("HTTP upstream does not support account scheduling admission")
	}
	transport.SetAccountSchedulingPools(pools)
	return upstream
}
