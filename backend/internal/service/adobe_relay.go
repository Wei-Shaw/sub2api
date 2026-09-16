package service

import (
	"context"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// IsAdobeRelayAccount 判断账号是否为 Adobe 组内的 OpenAI 形中转号：
// platform=adobe、type=apikey、且配置了非空 base_url。
//
// 这类账号不打 Firefly，而是把 /v1/images/generations|edits 转发到
// {base_url}，作为分组灾备。没有 base_url 的 adobe apikey 不是中转号——
// 本渠道不支持 Adobe 官方 Firefly Services API Key。
func IsAdobeRelayAccount(account *Account) bool {
	if account == nil || account.Platform != PlatformAdobe || account.Type != AccountTypeAPIKey {
		return false
	}
	return strings.TrimSpace(account.GetCredential("base_url")) != ""
}

func isAdobeRelayAccount(account *Account) bool {
	return IsAdobeRelayAccount(account)
}

// shouldSkipAdobeNativeAccount 在本次请求已撞上内容安全拒绝后，禁止再打
// Firefly Cookie 号。中转号不受影响。
func shouldSkipAdobeNativeAccount(skipNative bool, account *Account) bool {
	return skipNative && !isAdobeRelayAccount(account)
}

// adobeRelayIdentityMapping 是中转号的默认模型映射：键与 DefaultAdobeModelMapping
// 相同（调度白名单不变），值是恒等，这样上游收到的是 gpt-image-2 而不是
// firefly-gpt-image-2。
func adobeRelayIdentityMapping() map[string]string {
	src := domain.DefaultAdobeModelMapping
	out := make(map[string]string, len(src))
	for requested := range src {
		out[requested] = requested
	}
	return out
}

func defaultModelMappingForAccount(account *Account) map[string]string {
	if account == nil {
		return nil
	}
	if isAdobeRelayAccount(account) {
		return adobeRelayIdentityMapping()
	}
	return defaultModelMappingForPlatform(account.Platform)
}

// AdobeMaskRelaySelectionExclusions 构造「mask 优先走 API key 中转」阶段的选号排除集。
//
// failed 只拷贝，不就地改写，这样中转耗尽后 Cookie 号仍能被选到。
// preferRelay 时才并入 extraNativeIDs（选号循环里漏网的 native）。
func AdobeMaskRelaySelectionExclusions(failed map[int64]struct{}, preferRelay bool, extraNativeIDs map[int64]struct{}) map[int64]struct{} {
	out := make(map[int64]struct{}, len(failed)+len(extraNativeIDs))
	for id := range failed {
		out[id] = struct{}{}
	}
	if preferRelay {
		for id := range extraNativeIDs {
			out[id] = struct{}{}
		}
	}
	return out
}

// AdobePrefersMaskRelay 判断这次 images 请求是否应为 mask 优先打 API key 中转。
// 只有 GPT Images 家族（gpt-image-* / firefly-gpt-image-*）的 mask 才有官方 inpaint
// 语义；banana / flux / imagen 等忽略 mask，走正常调度。
func AdobePrefersMaskRelay(req *OpenAIImagesRequest) bool {
	if req == nil || !req.HasMask {
		return false
	}
	return adobeModelUsesOpenAIMask(req.Model)
}

func adobeModelUsesOpenAIMask(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	model = strings.TrimPrefix(model, "firefly-")
	return strings.HasPrefix(model, "gpt-image-")
}

// ExcludeAdobeNativeAccounts 把本组可调度的非中转 Adobe 账号并入 failed，
// 避免内容拒绝后还把 maxAccountSwitches 浪费在不会成功的 Cookie 号上。
func (s *GatewayService) ExcludeAdobeNativeAccounts(ctx context.Context, groupID *int64, failed map[int64]struct{}) {
	if s == nil || failed == nil {
		return
	}
	accounts, _, err := s.listSchedulableAccounts(ctx, groupID, PlatformAdobe, true)
	if err != nil {
		return
	}
	for i := range accounts {
		if !isAdobeRelayAccount(&accounts[i]) {
			failed[accounts[i].ID] = struct{}{}
		}
	}
}
