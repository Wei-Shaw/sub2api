package service

import (
	"slices"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// geminiImageModalityEnum 是 proto JSON 中 Modality.IMAGE 的整数枚举值。
const geminiImageModalityEnum = 2

// GeminiImageGenerationDenied 判断 Gemini 系请求是否因分组关闭生图而必须拒绝。
// 门禁只作用于后台展示该开关的 Gemini 系分组（gemini / antigravity / composite）；
// 无分组的 Key 与其他平台分组不受影响。models 中任一名字属于生图模型，
// 或请求体要求输出 IMAGE 模态，即视为生图请求。
func GeminiImageGenerationDenied(group *Group, geminiBody []byte, models ...string) bool {
	if group == nil || group.AllowImageGeneration {
		return false
	}
	switch group.Platform {
	case PlatformGemini, PlatformAntigravity, PlatformComposite:
	default:
		return false
	}
	return isGeminiImageGenerationIntent(geminiBody, models...)
}

// geminiImageGenerationDenied 是转发层在账号模型映射之后的门禁：models 传入
// 转发时可见的模型名（入参模型名与账号映射后的上游模型名）。命中时把请求标记为
// 本地功能门禁，调用方负责按各自协议写出 400。
func geminiImageGenerationDenied(c *gin.Context, geminiBody []byte, models ...string) bool {
	if c == nil || !GeminiImageGenerationDenied(apiKeyGroup(getAPIKeyFromContext(c)), geminiBody, models...) {
		return false
	}
	MarkOpsClientBusinessLimited(c, OpsClientBusinessLimitedReasonLocalFeatureGate)
	return true
}

func isGeminiImageGenerationIntent(geminiBody []byte, models ...string) bool {
	return slices.ContainsFunc(models, isGeminiImageOutputModel) || geminiRequestAsksForImageOutput(geminiBody)
}

// isGeminiImageOutputModel 匹配以图片为输出的 Gemini 模型名，与图片接口共用
// isGeminiCompatibleImageModel 判定。
func isGeminiImageOutputModel(model string) bool {
	return isGeminiCompatibleImageModel(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(model)), "models/"))
}

// geminiRequestAsksForImageOutput 判断 generationConfig.responseModalities 是否包含 IMAGE。
// Gemini REST 同时接受 camelCase 与 snake_case 字段名、字符串与整数枚举；
// 重复出现的键逐个检查。
func geminiRequestAsksForImageOutput(body []byte) bool {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return false
	}
	found := false
	gjson.ParseBytes(body).ForEach(func(key, config gjson.Result) bool {
		if key.Str == "generationConfig" || key.Str == "generation_config" {
			found = geminiGenerationConfigAsksForImage(config)
		}
		return !found
	})
	return found
}

func geminiGenerationConfigAsksForImage(config gjson.Result) bool {
	if !config.IsObject() {
		return false
	}
	found := false
	config.ForEach(func(key, modalities gjson.Result) bool {
		if key.Str == "responseModalities" || key.Str == "response_modalities" {
			found = geminiModalitiesContainImage(modalities)
		}
		return !found
	})
	return found
}

func geminiModalitiesContainImage(modalities gjson.Result) bool {
	found := false
	modalities.ForEach(func(_, modality gjson.Result) bool {
		switch modality.Type {
		case gjson.String:
			found = strings.EqualFold(strings.TrimSpace(modality.Str), "IMAGE")
		case gjson.Number:
			found = modality.Num == geminiImageModalityEnum
		}
		return !found
	})
	return found
}
