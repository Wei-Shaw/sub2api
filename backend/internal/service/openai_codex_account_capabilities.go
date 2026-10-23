package service

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/tidwall/gjson"
)

// CodexCapabilitiesExtraKey 是账号 Codex 能力记录在 accounts.extra 中的键。
// 该键进入调度快照投影（repository.filterSchedulerExtra），不属于调度中性键，
// 管理端编辑账号其他字段时由 UpdateAccount 的受管键列表保留。
const CodexCapabilitiesExtraKey = "codex_capabilities"

type CodexAccountCapabilities = domain.CodexAccountCapabilities
type CodexModelCapabilities = domain.CodexModelCapabilities

const (
	// CodexCyberProgramStandard 表示没有 Daybreak 项目；不构成需求，也不进入账号级集合。
	CodexCyberProgramStandard = "standard"
	CodexCyberProgramBlue     = "daybreak_blue"
	CodexCyberProgramRed      = "daybreak_red"
)

// DeriveCodexAccountCapabilities 从上游 Codex 模型目录 body 推导账号能力记录。
// 返回 ok=false 表示 body 无法解析或模型列表为空，此时调用方不得覆盖已有记录。
func DeriveCodexAccountCapabilities(body []byte) (CodexAccountCapabilities, bool) {
	if !gjson.ValidBytes(body) {
		return CodexAccountCapabilities{}, false
	}
	models := gjson.GetBytes(body, "models")
	if !models.IsArray() {
		return CodexAccountCapabilities{}, false
	}
	caps := CodexAccountCapabilities{Models: make(map[string]CodexModelCapabilities)}
	tierSet := make(map[string]struct{})
	programSet := make(map[string]struct{})
	for _, model := range models.Array() {
		slug := strings.TrimSpace(model.Get("slug").String())
		if slug == "" {
			continue
		}
		entry := CodexModelCapabilities{}
		for _, tier := range model.Get("service_tiers").Array() {
			id := normalizeCodexCapabilityValue(tier.Get("id").String())
			if id == "" {
				continue
			}
			entry.ServiceTiers = appendUniqueString(entry.ServiceTiers, id)
			tierSet[id] = struct{}{}
		}
		for _, program := range model.Get("available_access_programs.cyber").Array() {
			id := normalizeCodexCapabilityValue(program.String())
			if id == "" {
				continue
			}
			entry.CyberPrograms = appendUniqueString(entry.CyberPrograms, id)
			if id != CodexCyberProgramStandard {
				programSet[id] = struct{}{}
			}
		}
		sort.Strings(entry.ServiceTiers)
		sort.Strings(entry.CyberPrograms)
		caps.Models[slug] = entry
	}
	if len(caps.Models) == 0 {
		return CodexAccountCapabilities{}, false
	}
	caps.ServiceTiers = sortedStringSet(tierSet)
	caps.CyberPrograms = sortedStringSet(programSet)
	caps.SyncedAt = time.Now().UTC()
	return caps, true
}

// CodexCapabilities 读取账号已持久化的能力记录；缺失或无法解析返回 nil。
// Extra 中的值可能是类型化结构（进程内 mergeAccountExtra 写入）或 map（JSONB / Redis 投影），
// 统一经 JSON 往返归一。
func (a *Account) CodexCapabilities() *CodexAccountCapabilities {
	if a == nil || a.Extra == nil {
		return nil
	}
	raw, ok := a.Extra[CodexCapabilitiesExtraKey]
	if !ok || raw == nil {
		return nil
	}
	switch value := raw.(type) {
	case CodexAccountCapabilities:
		if len(value.Models) == 0 {
			return nil
		}
		return &value
	case *CodexAccountCapabilities:
		if value == nil || len(value.Models) == 0 {
			return nil
		}
		return value
	}
	payload, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var caps CodexAccountCapabilities
	if err := json.Unmarshal(payload, &caps); err != nil || len(caps.Models) == 0 {
		return nil
	}
	return &caps
}

func normalizeCodexCapabilityValue(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func appendUniqueString(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func sortedStringSet(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
