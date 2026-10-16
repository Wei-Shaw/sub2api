package service

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

type GroupCapabilityAllowlist = domain.GroupCapabilityAllowlist

var (
	groupCapabilityAllowlistCyberPrograms = map[string]struct{}{
		CodexCyberProgramBlue: {},
		CodexCyberProgramRed:  {},
	}
	groupCapabilityAllowlistServiceTiers = map[string]struct{}{
		OpenAIFastTierPriority:  {},
		OpenAIFastTierUltrafast: {},
	}
)

// normalizeGroupCapabilityAllowlist 在持久化前归一化分组能力白名单：
// 非 openai / composite 平台归零；取值去空白、小写、去重并排序。未知值保留给校验报错。
func normalizeGroupCapabilityAllowlist(platform string, cfg GroupCapabilityAllowlist) GroupCapabilityAllowlist {
	if !groupSupportsOpenAIFast(platform) {
		return GroupCapabilityAllowlist{}
	}
	return GroupCapabilityAllowlist{
		Enabled:       cfg.Enabled,
		CyberPrograms: normalizeCapabilityAllowlistValues(cfg.CyberPrograms),
		ServiceTiers:  normalizeCapabilityAllowlistValues(cfg.ServiceTiers),
	}
}

func normalizeCapabilityAllowlistValues(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	sort.Strings(out)
	return out
}

// validateGroupCapabilityAllowlist 校验取值合法性，并拒绝「白名单开启且不含 priority
// 却同时开启强制 Fast」的冲突配置：强制 Fast 会把每个请求改写为 priority，白名单却会拒绝它。
func validateGroupCapabilityAllowlist(cfg GroupCapabilityAllowlist, forceFast bool) error {
	for _, program := range cfg.CyberPrograms {
		if _, ok := groupCapabilityAllowlistCyberPrograms[program]; !ok {
			return infraerrors.Newf(http.StatusBadRequest, "INVALID_CAPABILITY_ALLOWLIST", "capability allowlist contains unknown cyber program %q", program)
		}
	}
	for _, tier := range cfg.ServiceTiers {
		if _, ok := groupCapabilityAllowlistServiceTiers[tier]; !ok {
			return infraerrors.Newf(http.StatusBadRequest, "INVALID_CAPABILITY_ALLOWLIST", "capability allowlist contains unknown service tier %q", tier)
		}
	}
	if cfg.Enabled && forceFast && !containsCapabilityAllowlistValue(cfg.ServiceTiers, OpenAIFastTierPriority) {
		return infraerrors.New(http.StatusBadRequest, "INVALID_CAPABILITY_ALLOWLIST", fmt.Sprintf("capability allowlist conflicts with force OpenAI Fast: forced requests become service_tier=%s, which the allowlist does not permit", OpenAIFastTierPriority))
	}
	return nil
}

func containsCapabilityAllowlistValue(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// CapabilityAllows 按分组能力白名单判定客户端原始需求；返回第一个被禁用的能力名，
// 空串表示放行。白名单关闭时允许一切；开启时空集合表示该类能力全部禁用。
func (g *Group) CapabilityAllows(req OpenAIAccountRequirement) string {
	if g == nil || !g.CapabilityAllowlist.Enabled {
		return ""
	}
	if req.CyberProgram != "" && !containsCapabilityAllowlistValue(g.CapabilityAllowlist.CyberPrograms, req.CyberProgram) {
		return req.CyberProgram
	}
	if req.ServiceTier != "" && !containsCapabilityAllowlistValue(g.CapabilityAllowlist.ServiceTiers, req.ServiceTier) {
		return req.ServiceTier
	}
	return ""
}
