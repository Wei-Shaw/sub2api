package handler

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// GroupCodexModelCatalog 与客户端使用相同目录来源；管理页面清除覆盖后读取基线。
func (h *GatewayHandler) GroupCodexModelCatalog(ctx context.Context, group *service.Group) ([]byte, error) {
	if group.Platform == service.PlatformOpenAI {
		manifest, _, err := h.openAIGatewayService.GetGroupCodexModelsManifest(ctx, group, service.CodexCanonicalClientVersion(), "", h.maxAccountSwitches)
		if err != nil {
			return nil, err
		}
		return manifest.Body, nil
	}
	ids := service.FilterCodexModelIDsForGroup(h.codexModelIDsForGroup(ctx, group, ""), group)
	body, err := h.gatewayService.BuildCodexModelsManifestForGroup(ctx, group, "", ids)
	if err != nil {
		return nil, err
	}
	return service.ApplyGroupCodexModelOverrides(body, group)
}
