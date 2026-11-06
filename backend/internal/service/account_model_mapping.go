package service

import (
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/geminicli"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"reflect"
	"strings"
)

func (a *Account) GetModelMapping() map[string]string {
	runtimeVersion := xai.RuntimeModelMappingVersion()
	credentialsPtr := mapPtr(a.Credentials)
	rawMapping, _ := a.Credentials["model_mapping"].(map[string]any)
	rawPtr := mapPtr(rawMapping)
	rawLen := len(rawMapping)
	rawSig := uint64(0)
	rawSigReady := false

	if a.modelMappingCacheReady &&
		a.modelMappingCacheCredentialsPtr == credentialsPtr &&
		a.modelMappingCacheRawPtr == rawPtr &&
		a.modelMappingCacheRawLen == rawLen &&
		a.modelMappingCacheRuntimeVersion == runtimeVersion {
		if a.modelMappingCacheShared {
			return a.modelMappingCache
		}
		rawSig = modelMappingSignature(rawMapping)
		rawSigReady = true
		if a.modelMappingCacheRawSig == rawSig {
			return a.modelMappingCache
		}
	}

	mapping := a.resolveModelMapping(rawMapping)
	if !rawSigReady {
		rawSig = modelMappingSignature(rawMapping)
	}

	a.modelMappingCache = mapping
	a.modelMappingCacheReady = true
	a.modelMappingCacheCredentialsPtr = credentialsPtr
	a.modelMappingCacheRawPtr = rawPtr
	a.modelMappingCacheRawLen = rawLen
	a.modelMappingCacheRawSig = rawSig
	a.modelMappingCacheRuntimeVersion = runtimeVersion
	a.modelMappingCacheShared = false
	return mapping
}

// PrepareSharedSnapshot 预解析模型映射，并把账号标记为只读共享母本。
// 母本按值复制出的副本与母本共用 Credentials 及其中 model_mapping 的同一 map：
// 这些内层 map 在运行期不被原地修改，母本留在缓存期间持有这些 map，其地址不会被复用，
// 因此副本在指针、长度与运行时版本都匹配时直接复用预解析结果，不计算签名；
// 副本替换 Credentials 等导致任一项不匹配时回到普通的重建路径。
// 只能由构造该账号的 goroutine 在其发布给其它 goroutine 之前调用，发布后母本不得再被修改。
func (a *Account) PrepareSharedSnapshot() {
	if a == nil {
		return
	}
	a.GetModelMapping()
	a.modelMappingCacheShared = true
}

func (a *Account) resolveModelMapping(rawMapping map[string]any) map[string]string {
	if a.Credentials == nil {
		// Antigravity 平台使用默认映射
		if a.Platform == domain.PlatformAntigravity {
			return domain.DefaultAntigravityModelMapping
		}
		if a.Platform == domain.PlatformGrok {
			return xai.DefaultModelMapping()
		}
		// Bedrock 默认映射由 forwardBedrock 统一处理（需配合 region prefix 调整）
		return nil
	}
	if len(rawMapping) == 0 {
		if a.IsGeminiGoogleOne() {
			return geminicli.GoogleOneModelMapping()
		}
		// Antigravity 平台使用默认映射
		if a.Platform == domain.PlatformAntigravity {
			return domain.DefaultAntigravityModelMapping
		}
		if a.Platform == domain.PlatformGrok {
			return xai.DefaultModelMapping()
		}
		return nil
	}

	result := make(map[string]string)
	for k, v := range rawMapping {
		if s, ok := v.(string); ok {
			if a.Platform == domain.PlatformAntigravity && (strings.TrimSpace(k) == "" || strings.TrimSpace(s) == "") {
				continue
			}
			result[k] = s
		}
	}
	// An explicit Antigravity mapping is also the administrator's allowlist.
	// New catalog models and compatibility aliases must never expand it.
	if a.Platform == domain.PlatformAntigravity {
		return result
	}
	if len(result) > 0 {
		return result
	}

	// Antigravity 平台使用默认映射
	if a.IsGeminiGoogleOne() {
		return geminicli.GoogleOneModelMapping()
	}
	if a.Platform == domain.PlatformAntigravity {
		return domain.DefaultAntigravityModelMapping
	}
	if a.Platform == domain.PlatformGrok {
		return xai.DefaultModelMapping()
	}
	return nil
}

func mapPtr(m map[string]any) uintptr {
	if m == nil {
		return 0
	}
	return reflect.ValueOf(m).Pointer()
}

const (
	modelMappingSignatureFNVOffset = 14695981039346656037
	modelMappingSignatureFNVPrime  = 1099511628211
)

// modelMappingSignature 计算原始映射内容的进程内校验值，与 map 遍历顺序无关且不分配内存。
// 每个条目把 key、分隔字节 0 与 value（string 写其字节，其它类型写标记字节 1）
// 放进同一个 FNV-1a 哈希并经 fmix64 收尾，再对所有条目求和；
// key 与 value 同在一个条目哈希里，交换两个 key 的 value 会改变结果。
func modelMappingSignature(rawMapping map[string]any) uint64 {
	if len(rawMapping) == 0 {
		return 0
	}
	var sum uint64
	for k, v := range rawMapping {
		h := uint64(modelMappingSignatureFNVOffset)
		h = fnv1aString(h, k)
		h = fnv1aByte(h, 0)
		if s, ok := v.(string); ok {
			h = fnv1aString(h, s)
		} else {
			h = fnv1aByte(h, 1)
		}
		sum += fmix64(h)
	}
	return sum
}

func fnv1aString(h uint64, s string) uint64 {
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= modelMappingSignatureFNVPrime
	}
	return h
}

func fnv1aByte(h uint64, b byte) uint64 {
	h ^= uint64(b)
	h *= modelMappingSignatureFNVPrime
	return h
}

// fmix64 是 MurmurHash3 的 64 位收尾函数，使条目哈希各位充分混合后再求和。
func fmix64(h uint64) uint64 {
	h ^= h >> 33
	h *= 0xff51afd7ed558ccd
	h ^= h >> 33
	h *= 0xc4ceb9fe1a85ec53
	h ^= h >> 33
	return h
}

func normalizeRequestedModelForLookup(platform, requestedModel string) string {
	trimmed := strings.TrimSpace(requestedModel)
	if trimmed == "" {
		return ""
	}
	if platform != PlatformGemini && platform != PlatformAntigravity {
		return trimmed
	}
	if trimmed == "gemini-3.1-pro-preview-customtools" {
		return "gemini-3.1-pro-preview"
	}
	return trimmed
}

func mappingSupportsRequestedModel(mapping map[string]string, requestedModel string) bool {
	if requestedModel == "" {
		return false
	}
	if _, exists := mapping[requestedModel]; exists {
		return true
	}
	for pattern := range mapping {
		if matchWildcard(pattern, requestedModel) {
			return true
		}
	}
	return false
}

func resolveRequestedModelInMapping(mapping map[string]string, requestedModel string) (mappedModel string, matched bool) {
	if requestedModel == "" {
		return "", false
	}
	if mappedModel, exists := mapping[requestedModel]; exists {
		return mappedModel, true
	}
	return matchWildcardMappingResult(mapping, requestedModel)
}

// IsModelSupported 检查模型是否在 model_mapping 中（支持通配符）
// 如果未配置 mapping，返回 true（允许所有模型）。
//
// 例外：OpenAI OAuth 账号（Codex 上游）的空映射会排除明确属于其他厂商
// 家族的模型（deepseek-*/glm-* 等）——转发阶段 normalizeOpenAIModelForUpstream
// 会把未知模型原样透传，Codex 上游对这类模型必然返回不可重试的 400，导致
// 请求卡死在该账号上、无法 failover 到真正支持该模型的 API Key 账号（#3662）。
// 未知/自定义别名仍保持允许（兼容渠道级映射），见 isOpenAIOAuthServableModel。
//
// 例外：DeepSeek 平台的空映射不再是「允许所有」，改按官方模型白名单判定
// （isDeepseekServableModel）——未知模型名透传上游只会得到 404/400，并误触发
// per-(账号,模型) 30 分钟冷却；带 [1m] 上下文后缀的写法先归一化再比对。
func (a *Account) IsModelSupported(requestedModel string) bool {
	return supportsOpenAIRequestModel(a, requestedModel, false)
}

func (a *Account) isModelMappingSupported(requestedModel string) bool {
	// 透传模式仅替换认证、模型语义完全交由上游决定，因此放行所有模型。
	// 该短路必须在 model_mapping 判定之前：账号从"白名单模式"切换到透传后，
	// credentials 里常残留旧的非空 model_mapping，若不在此放行，透传账号会被
	// model_mapping 白名单错误排除出候选集，导致 no available accounts / 404（issue #4936）。
	if a.IsOpenAIPassthroughEnabled() {
		return true
	}
	mapping := a.GetModelMapping()
	if len(mapping) == 0 {
		if a.Platform == domain.PlatformAntigravity {
			return false
		}
		if a.IsOpenAIOAuth() {
			return isOpenAIOAuthServableModel(requestedModel)
		}
		if a.Platform == PlatformDeepseek {
			return isDeepseekServableModel(requestedModel)
		}
		return true // 无映射 = 允许所有
	}
	if mappingSupportsRequestedModel(mapping, requestedModel) {
		return true
	}
	normalized := normalizeRequestedModelForLookup(a.Platform, requestedModel)
	return normalized != requestedModel && mappingSupportsRequestedModel(mapping, normalized)
}

// GetMappedModel 获取映射后的模型名（支持通配符，最长优先匹配）
// 如果未配置 mapping，返回原始模型名
func (a *Account) GetMappedModel(requestedModel string) string {
	mappedModel, _ := a.ResolveMappedModel(requestedModel)
	return mappedModel
}

// ResolveMappedModel 获取映射后的模型名，并返回是否命中了账号级映射。
// matched=true 表示命中了精确映射或通配符映射，即使映射结果与原模型名相同。
func (a *Account) ResolveMappedModel(requestedModel string) (mappedModel string, matched bool) {
	mapping := a.GetModelMapping()
	if len(mapping) == 0 {
		return requestedModel, false
	}
	if mappedModel, matched := resolveRequestedModelInMapping(mapping, requestedModel); matched {
		return mappedModel, true
	}
	normalized := normalizeRequestedModelForLookup(a.Platform, requestedModel)
	if normalized != requestedModel {
		if mappedModel, matched := resolveRequestedModelInMapping(mapping, normalized); matched {
			return mappedModel, true
		}
	}
	return requestedModel, false
}
