package service

import (
	"sort"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// PlatformCatalog 是前端使用的平台清单：domain 平台清单加上多协议供应商的
// profile（接入模式、各协议默认基址、内置分流规则）。它被写成前端内置清单
// frontend/src/constants/platformCatalog.builtin.json（见
// TestFrontendBuiltinPlatformCatalogInSync），前端据此生成平台选项、展示名与
// 多协议账号表单，新登记的平台只需重新生成该文件，无需修改前端代码。
type PlatformCatalog struct {
	Platforms []PlatformCatalogEntry `json:"platforms"`
	// CompositePrecedence 是组合分组逐平台回退的顺序（先命中者生效）。
	CompositePrecedence []string `json:"composite_precedence"`
}

// PlatformCatalogEntry 描述一个具体平台；MultiProtocol 仅多协议 API Key 供应商有值。
type PlatformCatalogEntry struct {
	domain.PlatformSpec
	MultiProtocol *ProviderProfileInfo `json:"multi_protocol,omitempty"`
}

// ProviderProfileInfo 是 ProviderProfile 的对外形状。
type ProviderProfileInfo struct {
	DefaultMode   string          `json:"default_mode"`
	Routing       ProviderRouting `json:"routing"`
	ResponsesPath string          `json:"responses_path,omitempty"`
	// ModelCatalog 表示模型支持的协议由上游 /models 自动获取（账号规则仍优先）。
	ModelCatalog bool `json:"model_catalog,omitempty"`
	// Modes 以默认模式开头，其余按模式名排序。
	Modes []ProviderModeInfo `json:"modes"`
}

// ProviderModeInfo 是某接入模式的默认端点；BaseURLs 缺少某协议即该模式不提供该原生端点。
type ProviderModeInfo struct {
	Mode          string            `json:"mode"`
	BaseURLs      map[string]string `json:"base_urls"`
	ProtocolRules []ProtocolRule    `json:"protocol_rules,omitempty"`
}

// BuildPlatformCatalog 按展示顺序返回全部具体平台（不含 composite）。
func BuildPlatformCatalog() PlatformCatalog {
	specs := domain.Platforms()
	catalog := PlatformCatalog{
		Platforms:           make([]PlatformCatalogEntry, 0, len(specs)),
		CompositePrecedence: domain.CompositePrecedencePlatformIDs(),
	}
	for _, spec := range specs {
		catalog.Platforms = append(catalog.Platforms, PlatformCatalogEntry{
			PlatformSpec:  spec,
			MultiProtocol: providerProfileInfo(LookupProviderProfile(spec.ID)),
		})
	}
	return catalog
}

func providerProfileInfo(profile *ProviderProfile) *ProviderProfileInfo {
	if profile == nil {
		return nil
	}
	routing := profile.Routing
	if routing == "" {
		routing = ProviderRoutingByInbound
	}
	modes := make([]string, 0, len(profile.Modes))
	for mode := range profile.Modes {
		if mode != profile.DefaultMode {
			modes = append(modes, mode)
		}
	}
	sort.Strings(modes)
	if _, ok := profile.Modes[profile.DefaultMode]; ok {
		modes = append([]string{profile.DefaultMode}, modes...)
	}
	info := &ProviderProfileInfo{
		DefaultMode:   profile.DefaultMode,
		Routing:       routing,
		ResponsesPath: profile.ResponsesPath,
		ModelCatalog:  profile.ModelCatalog,
		Modes:         make([]ProviderModeInfo, 0, len(modes)),
	}
	for _, mode := range modes {
		endpoints := profile.Modes[mode]
		baseURLs := make(map[string]string, len(endpoints.BaseURLs))
		for protocol, baseURL := range endpoints.BaseURLs {
			baseURLs[protocol] = baseURL
		}
		info.Modes = append(info.Modes, ProviderModeInfo{
			Mode:          mode,
			BaseURLs:      baseURLs,
			ProtocolRules: cloneProtocolRules(endpoints.ProtocolRules),
		})
	}
	return info
}

func cloneProtocolRules(rules []ProtocolRule) []ProtocolRule {
	if rules == nil {
		return nil
	}
	out := make([]ProtocolRule, len(rules))
	for i, rule := range rules {
		out[i] = rule
		out[i].Protocols = append([]string(nil), rule.Protocols...)
	}
	return out
}
