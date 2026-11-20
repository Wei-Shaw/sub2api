package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/Wei-Shaw/sub2api/pkg/modelcatalog"
)

type pluginCatalogSnapshot struct {
	pluginID int64
	config   *modelcatalog.Config
}

// Add a registered name only when existing routing permits at least one group
// member to serve it (including an explicit wildcard mapping). Registration
// never rewrites account credentials, channel mappings or group permissions.
func (m *PluginManager) appendCatalogModelIDs(ids []string, accounts []Account, group *Group, clientVersion string) []string {
	if group == nil {
		return ids
	}
	return m.catalogPolicy(group.ID, clientVersion).appendLegacyIDs(ids, accounts, group)
}

func (p *catalogPolicy) appendLegacyIDs(ids []string, accounts []Account, group *Group) []string {
	if p == nil {
		return ids
	}
	seen := map[string]bool{}
	for _, id := range ids {
		seen[id] = true
	}
	for _, id := range p.legacyOrder {
		if p.matches[id].Hidden || seen[id] {
			continue
		}
		if catalogAccountAllows(accounts, group, id) {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	return ids
}

func isCatalogPlugin(manifest PluginManifest) bool {
	for _, c := range manifest.Capabilities {
		if c.ID == PluginCapabilityCodexModelCatalog {
			return true
		}
	}
	return false
}
func hasEnabledCatalogBinding(bindings []PluginBinding) bool {
	for _, b := range bindings {
		if b.Enabled && b.Capability == PluginCapabilityCodexModelCatalog && b.Platform == PlatformOpenAI && b.AccountType == "catalog" {
			return true
		}
	}
	return false
}

// Called under operationMu. A catalog never receives account credentials, owns
// an outbound route, or drains a transport plugin. The DB remains authoritative.
func (m *PluginManager) reconcileCatalog(ctx context.Context, installations []*PluginInstallation) error {
	var enabled *PluginInstallation
	for _, i := range installations {
		if !hasEnabledCatalogBinding(i.Bindings) {
			continue
		}
		if enabled != nil {
			m.catalog.Store(nil)
			return errors.New("存在多个启用的模型目录插件")
		}
		enabled = i
	}
	m.mu.Lock()
	var stale []*pluginRuntime
	for id, r := range m.runtimes {
		if isCatalogPlugin(r.installation.Manifest) && (enabled == nil || id != enabled.ID) {
			stale = append(stale, m.removeRuntimeLocked(id))
		}
	}
	var current *pluginRuntime
	if enabled != nil {
		current = m.runtimes[enabled.ID]
	}
	m.mu.Unlock()
	for _, r := range stale {
		r.drain(10 * time.Second)
	}
	if enabled == nil {
		m.catalog.Store(nil)
		return nil
	}
	if enabled.State == PluginStateStarting && !m.startingStateExpired(enabled) {
		return nil
	}
	if current != nil && !current.client.Exited() && current.installation.BinarySHA256 == enabled.BinarySHA256 && current.installation.ConfigEncrypted == enabled.ConfigEncrypted {
		healthCtx, cancel := context.WithTimeout(ctx, pluginHealthTimeout)
		err := current.checkHealth(healthCtx)
		cancel()
		if err == nil {
			return nil
		}
	}
	// Remove stale metadata before starting a replacement, including invalid
	// cross-instance updates. Never serve an old descriptor for a new DB config.
	m.catalog.Store(nil)
	local, err := m.ensureLocalInstallation(ctx, enabled)
	if err != nil {
		return err
	}
	r, err := m.prepareRuntime(ctx, local, true)
	if err != nil {
		return err
	}
	latest, err := m.repo.GetByID(ctx, enabled.ID)
	if err != nil {
		r.kill()
		return err
	}
	if !hasEnabledCatalogBinding(latest.Bindings) || latest.BinarySHA256 != enabled.BinarySHA256 || latest.ConfigEncrypted != enabled.ConfigEncrypted || latest.State == PluginStateStarting && !m.startingStateExpired(latest) {
		r.kill()
		return nil
	}
	if err = m.repo.MarkRuntimeHealthy(ctx, enabled.ID, enabled.BinarySHA256, enabled.ConfigEncrypted); err != nil {
		r.kill()
		return err
	}
	m.mu.Lock()
	m.publishRuntimeLocked(local, r)
	m.mu.Unlock()
	return nil
}

// overlayCodexCatalog replaces only entries already authorized by the normal
// group/account routing and allowlist pipeline. Metadata is never an access grant.
// Unknown descriptor fields, instructions and tool capabilities survive intact.
func (m *PluginManager) overlayCodexCatalog(body []byte, groupID int64, clientVersion string) ([]byte, bool, error) {
	return m.catalogPolicy(groupID, clientVersion).project(body, true, nil)
}

type catalogPolicy struct {
	explicit       bool
	restrictPlain  bool
	selection      *modelcatalog.Selection
	rank           map[string]int32
	matches        map[string]modelcatalog.Model
	legacyOrder    []string
	versionLimited map[string]bool
	library        map[string]modelcatalog.LibraryModel
	imageIDs       map[string]bool
	plainImageRank map[string]int32
}

func (m *PluginManager) catalogPolicy(groupID int64, version string) *catalogPolicy {
	if m == nil {
		return nil
	}
	snapshot := m.catalog.Load()
	if snapshot == nil || snapshot.config == nil {
		return nil
	}
	return newCatalogPolicy(snapshot.config, groupID, version)
}

func newCatalogPolicy(c *modelcatalog.Config, groupID int64, version string) *catalogPolicy {
	p := &catalogPolicy{explicit: c.ExplicitOnly(groupID), selection: c.SelectionForGroup(groupID),
		rank: map[string]int32{}, matches: map[string]modelcatalog.Model{}, versionLimited: map[string]bool{},
		library: map[string]modelcatalog.LibraryModel{}, imageIDs: map[string]bool{}}
	p.restrictPlain = p.explicit
	for _, entry := range c.Models {
		if !slices.Contains(entry.GroupIDs, groupID) {
			continue
		}
		id := entry.Slug()
		if entry.Hidden && (version == "" || entry.Matches(groupID, version)) {
			p.restrictPlain = true
		}
		if entry.Matches(groupID, version) {
			p.matches[id] = entry
			p.legacyOrder = append(p.legacyOrder, id)
		} else if !entry.Hidden || version == "" {
			p.versionLimited[id] = true
		}
	}
	for _, item := range c.Library {
		p.library[item.Slug] = item
		if item.Kind == "image" {
			p.imageIDs[item.Slug] = true
		}
	}
	for _, preset := range c.Presets {
		for _, id := range preset.ImageModels {
			p.imageIDs[id] = true
		}
	}
	for _, group := range c.GroupCatalogs {
		for _, id := range group.ImageModels {
			p.imageIDs[id] = true
		}
	}
	if p.selection != nil {
		p.plainImageRank = make(map[string]int32, len(p.selection.ImageModels))
		for i, id := range p.selection.ImageModels {
			if entry, matched := p.matches[id]; !entry.Hidden && (matched || !p.versionLimited[id]) {
				p.plainImageRank[id] = int32(len(p.selection.ChatModels) + i)
			}
		}
		for i, id := range p.selection.ChatModels {
			p.rank[id] = int32(i)
		}
	}
	return p
}

func (p *catalogPolicy) imageModel(id string, accounts []Account) bool {
	isImage := func(id string) bool {
		model := codexProviderQualifiedModelID(id)
		return p.imageIDs[id] || isOpenAIImageGenerationModel(model) || isGeminiCompatibleImageModel(model) || model == "dall-e-2" || model == "dall-e-3"
	}
	if isImage(id) {
		return true
	}
	for i := range accounts {
		a := &accounts[i]
		if a.Platform == PlatformOpenAI && !a.IsOpenAIPassthroughEnabled() {
			target := a.GetMappedModel(id)
			if isImage(target) {
				return true
			}
		}
	}
	return false
}

func (p *catalogPolicy) chatOrder(id string, accounts []Account, codex bool) (*int32, bool) {
	entry, matched := p.matches[id]
	if entry.Hidden || isCodexDedicatedMediaModel(id) || p.imageModel(id, accounts) || (!codex && !matched && p.versionLimited[id]) {
		return nil, false
	}
	if p.selection != nil {
		order, listed := p.rank[id]
		return &order, listed && (matched || !p.versionLimited[id])
	}
	return entry.ClientOrder, !p.explicit || matched
}

func catalogOrderLess(a, b *int32) bool { return a != nil && (b == nil || *a < *b) }

// Both list shapes enter here after native membership and permission checks.
// Only the Codex shape receives full descriptors or a default-model hint.
func (p *catalogPolicy) project(body []byte, codex bool, accounts []Account) ([]byte, bool, error) {
	if p == nil || (!codex && !p.restrictPlain) {
		return body, false, nil
	}
	field := "models"
	if !codex {
		field = "data"
	}
	root, models, err := modelCatalogEntries(body, field)
	if err != nil {
		return nil, false, err
	}
	changed := p.selection != nil
	visible := make([]json.RawMessage, 0, len(models))
	type rankedModel struct {
		index int
		order *int32
	}
	ranks := make([]rankedModel, 0, len(models))
	ordered := false
	hintVisible := false
	for _, raw := range models {
		var f struct {
			Slug string `json:"slug"`
			ID   string `json:"id"`
		}
		if err := json.Unmarshal(raw, &f); err != nil {
			return nil, false, err
		}
		id := f.Slug
		if !codex {
			id = f.ID
		}
		order, allowed := p.chatOrder(id, accounts, codex)
		// The pinned plain list has already checked each image's contributing account.
		if rank, ok := p.plainImageRank[id]; !codex && ok {
			order, allowed = &rank, true
		}
		if !allowed {
			changed = true
			continue
		}
		entry, matched := p.matches[id]
		if matched && codex {
			// Existing transport restrictions win over imported capabilities.
			var original, replacement map[string]json.RawMessage
			_ = json.Unmarshal(raw, &original)
			_ = json.Unmarshal(entry.Descriptor, &replacement)
			if string(original["use_responses_lite"]) == "false" {
				replacement["use_responses_lite"] = json.RawMessage("false")
			}
			raw, _ = json.Marshal(replacement)
			changed = true
		}
		if p.selection != nil {
			hintVisible = hintVisible || (p.selection.DefaultChatModel != nil && *p.selection.DefaultChatModel == id)
			if item, ok := p.library[id]; ok && codex {
				raw, err = applyCatalogLibrary(raw, item)
				if err != nil {
					return nil, false, err
				}
			}
		}
		ranks = append(ranks, rankedModel{index: len(visible), order: order})
		visible = append(visible, raw)
		ordered = ordered || order != nil
	}
	if ordered {
		sort.SliceStable(ranks, func(i, j int) bool {
			a, b := ranks[i].order, ranks[j].order
			return catalogOrderLess(a, b)
		})
		// Some clients sort by descriptor priority rather than response position.
		sorted := make([]json.RawMessage, len(visible))
		for i, rank := range ranks {
			if !codex {
				sorted[i] = visible[rank.index]
				continue
			}
			var descriptor map[string]json.RawMessage
			if err := json.Unmarshal(visible[rank.index], &descriptor); err != nil || descriptor == nil {
				return nil, false, fmt.Errorf("模型目录条目不是对象")
			}
			descriptor["priority"], _ = json.Marshal(i)
			sorted[i], _ = json.Marshal(descriptor)
		}
		visible = sorted
	}
	if !changed && !ordered {
		return body, false, nil
	}
	if codex && p.explicit {
		if _, ok := root["default_chat_model"]; ok {
			delete(root, "default_chat_model")
		}
		if hintVisible {
			root["default_chat_model"], _ = json.Marshal(p.selection.DefaultChatModel)
		}
	}
	root[field], _ = json.Marshal(visible)
	result, err := json.Marshal(root)
	if err == nil && len(result) > modelcatalog.MaxBytes {
		err = fmt.Errorf("模型目录响应超过 1 MiB")
	}
	return result, true, err
}

func catalogAccountAllows(accounts []Account, group *Group, id string) bool {
	if group == nil || !group.ModelAllowlist.Allows(id) {
		return false
	}
	for i := range accounts {
		a := &accounts[i]
		if a.Platform == PlatformOpenAI && a.IsModelSupported(id) {
			return true
		}
	}
	return false
}

// FilterPluginCatalogModelIDs handles non-pinned /models and /models/:model.
// The native listing can fall back to defaults even for an empty account set;
// never treat those defaults as evidence of an eligible route.
func (s *OpenAIGatewayService) FilterPluginCatalogModelIDs(ctx context.Context, group *Group, available []string) ([]string, bool, error) {
	if s == nil || group == nil || group.Platform != PlatformOpenAI {
		return nil, false, nil
	}
	p := s.pluginManager.catalogPolicy(group.ID, "")
	if p == nil || !p.restrictPlain {
		return nil, false, nil
	}
	accounts, err := s.accountRepo.ListSchedulableByGroupID(ctx, group.ID)
	if err != nil {
		return nil, true, err
	}
	ids := FilterCodexModelIDsForGroup(available, group)
	out := make([]string, 0, len(ids))
	ranks := map[string]*int32{}
	for _, id := range ids {
		order, allowed := p.chatOrder(id, accounts, false)
		if allowed && catalogAccountAllows(accounts, group, id) {
			out = append(out, id)
			ranks[id] = order
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return catalogOrderLess(ranks[out[i]], ranks[out[j]]) })
	if p.selection != nil && group.AllowImageGeneration {
		availableImages := group.ModelAllowlist.FilterForListing(available)
		for _, id := range p.selection.ImageModels {
			if _, published := p.plainImageRank[id]; !published || !slices.Contains(availableImages, id) {
				continue
			}
			for i := range accounts {
				if p.allowsImageAccount(ctx, group, &accounts[i], id) {
					out = append(out, id)
					break
				}
			}
		}
	}
	return out, true, nil
}

func applyCatalogLibrary(raw json.RawMessage, item modelcatalog.LibraryModel) (json.RawMessage, error) {
	var d map[string]json.RawMessage
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, err
	}
	if item.DisplayName != nil {
		d["display_name"], _ = json.Marshal(item.DisplayName)
	}
	if item.Description != nil {
		d["description"], _ = json.Marshal(item.Description)
	}
	if item.InputModalities != nil {
		var inherited []string
		if json.Unmarshal(d["input_modalities"], &inherited) == nil && inherited != nil {
			kept := slices.DeleteFunc(inherited, func(mode string) bool { return !slices.Contains(item.InputModalities, mode) })
			d["input_modalities"], _ = json.Marshal(kept)
			if !slices.Contains(kept, "image") && d["supports_image_detail_original"] != nil {
				d["supports_image_detail_original"] = json.RawMessage("false")
			}
		}
	}
	for key, flag := range map[string]*bool{"image_generation": item.ImageGeneration, "image_edit": item.ImageEdit} {
		if flag != nil && !*flag {
			d[key] = json.RawMessage("false")
		}
	}
	return json.Marshal(d)
}
