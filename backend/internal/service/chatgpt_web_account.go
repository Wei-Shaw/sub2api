package service

import (
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// ChatGPT Web 生图通道的账号级开关与参数。
//
// 开启后，该 OpenAI OAuth 账号的 /v1/images/generations 走 chatgpt.com 网页链路，
// 消耗网页侧 image_gen 额度而不是 Codex 后端额度；未开启时行为与原先完全一致。

const (
	featureKeyChatGPTWebImageGeneration = "chatgpt_web_image_generation"
	featureKeyChatGPTWebImageModel      = "chatgpt_web_image_model"

	// accountTypeWebImageLegacy 是该类型改名前的取值。仅用于识别改名之前已建的账号，
	// 使其不会因为变成"未知类型"而在能力位判定上被放行给文本流量。
	accountTypeWebImageLegacy = "web"
)

func chatGPTWebStringFromMap(values map[string]any, keys ...string) string {
	if values == nil {
		return ""
	}
	for _, key := range keys {
		if v, ok := values[key].(string); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// ChatGPTWebImageGenerationEnabled 返回账号是否启用网页生图通道。
func (a *Account) ChatGPTWebImageGenerationEnabled() bool {
	if a == nil || a.Platform != PlatformOpenAI {
		return false
	}
	if v, ok := a.Extra[featureKeyChatGPTWebImageGeneration].(bool); ok {
		return v
	}
	openaiConfig, _ := a.Extra[PlatformOpenAI].(map[string]any)
	if v, ok := openaiConfig[featureKeyChatGPTWebImageGeneration].(bool); ok {
		return v
	}
	if strings.EqualFold(chatGPTWebStringFromMap(a.Extra, featureKeyChatGPTWebImageGeneration), "true") {
		return true
	}
	return false
}

// IsOpenAIWebImage 标识只承载网页图片请求的账号类型，兼容改名前已保存的账号。
func (a *Account) IsOpenAIWebImage() bool {
	return a != nil && a.Platform == PlatformOpenAI &&
		(a.Type == AccountTypeWebImage || a.Type == accountTypeWebImageLegacy)
}

// UsesChatGPTWebImageChannel 判定该账号的图片请求是否走 ChatGPT 网页通道。
//
// 这是唯一的分派依据：网关转发与后台"测试账号连接"都调它，避免两处判断漂移。
//   - web 账号类型：直接走（类型即通道）
//   - OAuth / setup-token：按 extra 开关走
func (a *Account) UsesChatGPTWebImageChannel() bool {
	if a == nil || a.Platform != PlatformOpenAI {
		return false
	}
	switch a.Type {
	case AccountTypeWebImage, accountTypeWebImageLegacy:
		return true
	case AccountTypeOAuth, AccountTypeSetupToken:
		return a.ChatGPTWebImageGenerationEnabled()
	default:
		return false
	}
}

// ChatGPTWebImageUpstreamModel 返回网页侧使用的上游模型名，缺省为 auto。
func (a *Account) ChatGPTWebImageUpstreamModel() string {
	if a == nil || a.Platform != PlatformOpenAI {
		return ""
	}
	if value := chatGPTWebStringFromMap(a.Extra, featureKeyChatGPTWebImageModel); value != "" {
		return value
	}
	openaiConfig, _ := a.Extra[PlatformOpenAI].(map[string]any)
	return chatGPTWebStringFromMap(openaiConfig, featureKeyChatGPTWebImageModel)
}

// ChatGPTWebDevice 返回该账号稳定复用的设备指纹。
//
// 设备 ID 必须跨请求稳定：每次换新 ID 会让网页端把请求当成陌生客户端，
// 更容易触发额外风控挑战。
func (a *Account) ChatGPTWebDevice() chatGPTWebDevice {
	device := chatGPTWebDevice{}
	if a == nil {
		return device.withDefaults()
	}
	device.UserAgent = chatGPTWebStringFromMap(a.Extra, "chatgpt_web_user_agent")
	openaiConfig, _ := a.Extra[PlatformOpenAI].(map[string]any)
	if device.UserAgent == "" {
		device.UserAgent = chatGPTWebStringFromMap(openaiConfig, "chatgpt_web_user_agent")
	}
	device.DeviceID = a.stableChatGPTWebUUID("device")
	device.SessionID = a.stableChatGPTWebUUID("session")
	return device.withDefaults()
}

func (a *Account) stableChatGPTWebUUID(kind string) string {
	if a == nil {
		return ""
	}
	identity := a.stableChatGPTWebIdentity()
	if identity == "" {
		return ""
	}
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("chatgpt-web-"+kind+"-"+identity)).String()
}

func (a *Account) stableChatGPTWebIdentity() string {
	if a == nil || a.ID <= 0 {
		return ""
	}
	return strconv.FormatInt(a.ID, 10)
}
