package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// CodexCapabilitySource 是 manifest 能力投影的一个来源账号：Caps 为其已持久化的能力记录，
// UpstreamSlugOf 用该账号自身的模型映射把 manifest 中的公开别名解析回上游 slug
// （同一公开别名在不同账号上可以映射到不同上游模型）。nil 表示公开别名即上游 slug。
type CodexCapabilitySource struct {
	Caps           CodexAccountCapabilities
	UpstreamSlugOf func(publicSlug string) string
}

// ProjectCodexManifestCapabilities 把 manifest 中每个模型的 service_tiers 与
// available_access_programs.cyber 投影为分组账号能力记录的按 slug 并集，再按分组白名单裁剪：
//  1. 并集：逐来源解析上游 slug 并查其记录；任一来源命中则两个字段**替换**为命中记录的并集
//     （不保留 body 原值：body 可能来自旧缓存或硬编码模板，并入会复活已撤销能力）；
//     所有来源都不含该 slug 时保留原值。条目名称与描述沿用 body 原有条目，缺失时用内置描述。
//  2. 裁剪：白名单开启时无条件执行，剔除被禁用的 Daybreak 等级与 tier；standard 保留。
//
// 返回投影后的 body 与是否发生变化；未变化时原样返回输入。
func ProjectCodexManifestCapabilities(body []byte, sources []CodexCapabilitySource, allowlist GroupCapabilityAllowlist) ([]byte, bool, error) {
	if len(body) == 0 || (len(sources) == 0 && !allowlist.Enabled) {
		return body, false, nil
	}
	envelope, models, err := modelCatalogEntries(body, "models")
	if err != nil {
		return nil, false, err
	}
	changed := false
	projected := make([]json.RawMessage, 0, len(models))
	for _, raw := range models {
		var model map[string]json.RawMessage
		if err := json.Unmarshal(raw, &model); err != nil || model == nil {
			projected = append(projected, raw)
			continue
		}
		var slug string
		_ = json.Unmarshal(model["slug"], &slug)
		slug = strings.TrimSpace(slug)
		modelChanged := false
		if slug != "" {
			if tiers, programs, hit := unionCodexCapabilitiesForSlug(slug, sources); hit {
				model["service_tiers"] = projectCodexServiceTiers(model["service_tiers"], tiers)
				model["available_access_programs"] = projectCodexCyberPrograms(model["available_access_programs"], programs)
				modelChanged = true
			}
		}
		if allowlist.Enabled {
			if next, pruned := pruneCodexServiceTiers(model["service_tiers"], allowlist); pruned {
				model["service_tiers"] = next
				modelChanged = true
			}
			if next, pruned := pruneCodexCyberPrograms(model["available_access_programs"], allowlist); pruned {
				model["available_access_programs"] = next
				modelChanged = true
			}
		}
		if !modelChanged {
			projected = append(projected, raw)
			continue
		}
		encoded, err := json.Marshal(model)
		if err != nil {
			return nil, false, fmt.Errorf("encode projected codex model %q: %w", slug, err)
		}
		projected = append(projected, encoded)
		changed = true
	}
	if !changed {
		return body, false, nil
	}
	encodedModels, err := json.Marshal(projected)
	if err != nil {
		return nil, false, fmt.Errorf("encode projected codex models: %w", err)
	}
	envelope["models"] = encodedModels
	out, err := json.Marshal(envelope)
	if err != nil {
		return nil, false, fmt.Errorf("encode projected codex manifest: %w", err)
	}
	return out, true, nil
}

func unionCodexCapabilitiesForSlug(publicSlug string, sources []CodexCapabilitySource) (tiers []string, programs []string, hit bool) {
	tierSet := make(map[string]struct{})
	programSet := make(map[string]struct{})
	for _, source := range sources {
		upstream := publicSlug
		if source.UpstreamSlugOf != nil {
			if mapped := strings.TrimSpace(source.UpstreamSlugOf(publicSlug)); mapped != "" {
				upstream = mapped
			}
		}
		caps, ok := source.Caps.Models[upstream]
		if !ok {
			continue
		}
		hit = true
		for _, tier := range caps.ServiceTiers {
			tierSet[tier] = struct{}{}
		}
		for _, program := range caps.CyberPrograms {
			programSet[program] = struct{}{}
		}
	}
	if !hit {
		return nil, nil, false
	}
	return orderCodexServiceTierIDs(tierSet), orderCodexCyberPrograms(programSet), true
}

// orderCodexServiceTierIDs 按 Codex 客户端的展示习惯排序：priority、ultrafast 在前，其余按字典序。
func orderCodexServiceTierIDs(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for _, known := range []string{OpenAIFastTierPriority, OpenAIFastTierUltrafast} {
		if _, ok := set[known]; ok {
			out = append(out, known)
			delete(set, known)
		}
	}
	rest := sortedStringSet(set)
	return append(out, rest...)
}

// orderCodexCyberPrograms 把 standard 放在首位，其余按字典序。
func orderCodexCyberPrograms(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	if _, ok := set[CodexCyberProgramStandard]; ok {
		out = append(out, CodexCyberProgramStandard)
		delete(set, CodexCyberProgramStandard)
	}
	return append(out, sortedStringSet(set)...)
}

// projectCodexServiceTiers 用并集 ids 重建 service_tiers：条目优先沿用 body 原有条目，
// 缺失时用内置描述。
func projectCodexServiceTiers(existing json.RawMessage, ids []string) json.RawMessage {
	existingByID := make(map[string]json.RawMessage)
	var entries []json.RawMessage
	if len(existing) > 0 {
		_ = json.Unmarshal(existing, &entries)
	}
	for _, entry := range entries {
		var descriptor struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(entry, &descriptor); err == nil {
			if id := normalizeCodexCapabilityValue(descriptor.ID); id != "" {
				existingByID[id] = entry
			}
		}
	}
	out := make([]json.RawMessage, 0, len(ids))
	for _, id := range ids {
		if entry, ok := existingByID[id]; ok {
			out = append(out, entry)
			continue
		}
		encoded, err := json.Marshal(builtinCodexServiceTier(id))
		if err != nil {
			continue
		}
		out = append(out, encoded)
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return json.RawMessage(`[]`)
	}
	return encoded
}

// builtinCodexServiceTier 返回 Codex 已知 tier 的内置名称与描述；未知 tier 以 id 作名称。
func builtinCodexServiceTier(id string) configuredCodexServiceTier {
	switch id {
	case OpenAIFastTierPriority:
		return configuredCodexServiceTier{ID: id, Name: "Fast", Description: "Priority processing for lower latency."}
	case OpenAIFastTierUltrafast:
		return configuredCodexServiceTier{ID: id, Name: "Ultrafast", Description: "Ultra-low latency processing."}
	default:
		return configuredCodexServiceTier{ID: id, Name: id}
	}
}

// projectCodexCyberPrograms 把 available_access_programs.cyber 替换为并集，保留对象内其他键。
func projectCodexCyberPrograms(existing json.RawMessage, programs []string) json.RawMessage {
	object := make(map[string]json.RawMessage)
	if len(existing) > 0 {
		_ = json.Unmarshal(existing, &object)
		if object == nil {
			object = make(map[string]json.RawMessage)
		}
	}
	if programs == nil {
		programs = []string{}
	}
	encodedPrograms, err := json.Marshal(programs)
	if err != nil {
		return existing
	}
	object["cyber"] = encodedPrograms
	encoded, err := json.Marshal(object)
	if err != nil {
		return existing
	}
	return encoded
}

// pruneCodexServiceTiers 剔除白名单禁用的 tier（只作用于可配置的 priority / ultrafast）。
func pruneCodexServiceTiers(existing json.RawMessage, allowlist GroupCapabilityAllowlist) (json.RawMessage, bool) {
	var entries []json.RawMessage
	if len(existing) == 0 || json.Unmarshal(existing, &entries) != nil {
		return existing, false
	}
	kept := make([]json.RawMessage, 0, len(entries))
	pruned := false
	for _, entry := range entries {
		var descriptor struct {
			ID string `json:"id"`
		}
		id := ""
		if err := json.Unmarshal(entry, &descriptor); err == nil {
			id = normalizeCodexCapabilityValue(descriptor.ID)
		}
		if _, configurable := groupCapabilityAllowlistServiceTiers[id]; configurable && !containsCapabilityAllowlistValue(allowlist.ServiceTiers, id) {
			pruned = true
			continue
		}
		kept = append(kept, entry)
	}
	if !pruned {
		return existing, false
	}
	encoded, err := json.Marshal(kept)
	if err != nil {
		return existing, false
	}
	return encoded, true
}

// pruneCodexCyberPrograms 剔除白名单禁用的 Daybreak 等级；standard 与未知值保留。
func pruneCodexCyberPrograms(existing json.RawMessage, allowlist GroupCapabilityAllowlist) (json.RawMessage, bool) {
	object := make(map[string]json.RawMessage)
	if len(existing) == 0 || json.Unmarshal(existing, &object) != nil || object == nil {
		return existing, false
	}
	var programs []string
	if raw, ok := object["cyber"]; !ok || json.Unmarshal(raw, &programs) != nil {
		return existing, false
	}
	kept := make([]string, 0, len(programs))
	pruned := false
	for _, program := range programs {
		id := normalizeCodexCapabilityValue(program)
		if _, configurable := groupCapabilityAllowlistCyberPrograms[id]; configurable && !containsCapabilityAllowlistValue(allowlist.CyberPrograms, id) {
			pruned = true
			continue
		}
		kept = append(kept, program)
	}
	if !pruned {
		return existing, false
	}
	encodedPrograms, err := json.Marshal(kept)
	if err != nil {
		return existing, false
	}
	object["cyber"] = encodedPrograms
	encoded, err := json.Marshal(object)
	if err != nil {
		return existing, false
	}
	return encoded, true
}

// codexCapabilitySourcesForGroup 为 OpenAI 分组逐个可调度 OAuth-like 账号构造投影来源
// （一次分组查询）；Composite 分组没有直属账号，返回空。
func (s *OpenAIGatewayService) codexCapabilitySourcesForGroup(ctx context.Context, group *Group) ([]CodexCapabilitySource, error) {
	if s == nil || s.accountRepo == nil || group == nil || group.Platform != PlatformOpenAI {
		return nil, nil
	}
	accounts, err := s.accountRepo.ListSchedulableByGroupID(ctx, group.ID)
	if err != nil {
		return nil, err
	}
	sources := make([]CodexCapabilitySource, 0, len(accounts))
	for i := range accounts {
		account := &accounts[i]
		if !isCodexCapabilityProbeAccount(account) {
			continue
		}
		caps := account.CodexCapabilities()
		if caps == nil {
			continue
		}
		sources = append(sources, CodexCapabilitySource{
			Caps: *caps,
			UpstreamSlugOf: func(publicSlug string) string {
				if mapped, matched := account.ResolveMappedModel(publicSlug); matched && strings.TrimSpace(mapped) != "" {
					return mapped
				}
				return publicSlug
			},
		})
	}
	return sources, nil
}

// FinalizeCodexModelsManifest 是三条 manifest 生成路径（固定账号、本地生成、调度拉取）写响应前
// 共用的后处理：能力投影 → 按投影后的 body 重算 ETag → 判断 304。
func (s *OpenAIGatewayService) FinalizeCodexModelsManifest(ctx context.Context, group *Group, manifest *OpenAIModelsResponse, ifNoneMatch string) error {
	if manifest == nil || manifest.NotModified || len(manifest.Body) == 0 {
		return nil
	}
	if group != nil {
		sources, err := s.codexCapabilitySourcesForGroup(ctx, group)
		if err != nil {
			return fmt.Errorf("load codex capability sources: %w", err)
		}
		body, changed, err := ProjectCodexManifestCapabilities(manifest.Body, sources, group.CapabilityAllowlist)
		if err != nil {
			return fmt.Errorf("project codex manifest capabilities: %w", err)
		}
		if changed {
			manifest.Body = body
			manifest.ETag = codexModelsManifestBodyETag(body)
		}
	}
	if strings.TrimSpace(manifest.ETag) == "" {
		manifest.ETag = codexModelsManifestBodyETag(manifest.Body)
	}
	if codexModelsManifestETagMatches(ifNoneMatch, manifest.ETag) {
		manifest.Body = nil
		manifest.NotModified = true
	}
	return nil
}
