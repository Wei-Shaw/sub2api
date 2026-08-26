//go:build unit

package service

import (
	"context"
	"net/http"
	"time"

	kiropkg "github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
)

func (s *GatewayService) streamKeepaliveIntervalForAccount(account *Account) time.Duration {
	if account != nil && account.Platform == PlatformKiro {
		if s != nil && s.cfg != nil && s.cfg.Gateway.KiroStreamKeepaliveInterval > 0 {
			return time.Duration(s.cfg.Gateway.KiroStreamKeepaliveInterval) * time.Second
		}
		return defaultKiroStreamKeepalive
	}
	if s != nil && s.cfg != nil && s.cfg.Gateway.StreamKeepaliveInterval > 0 {
		return time.Duration(s.cfg.Gateway.StreamKeepaliveInterval) * time.Second
	}
	return 0
}

func (s *GatewayService) buildKiroPayloadForAccount(ctx context.Context, account *Account, parsed *ParsedRequest, anthropicBody []byte, modelID, token, requestModel string, headers http.Header) (*kiropkg.KiroBuildResult, error) {
	// 镜像生产逻辑：Q / KRS 端点都解析 profileArn（API Key → 空；缺失时上游 403）。
	profileArn := kiroResolveRequestProfileArn(account)
	return s.buildKiroPayloadForAccountWithArn(ctx, account, parsed, anthropicBody, modelID, token, requestModel, headers, profileArn)
}
