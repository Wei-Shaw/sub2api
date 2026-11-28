package service

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/pkg/modelcatalog"
)

type PluginCatalogGroup struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}
type PluginCatalogInventoryModel struct {
	Source     string          `json:"source"`
	Descriptor json.RawMessage `json:"descriptor"`
}
type PluginCatalogInventory struct {
	GroupID             int64                         `json:"group_id"`
	Groups              []PluginCatalogGroup          `json:"groups"`
	Models              []PluginCatalogInventoryModel `json:"models"`
	ImageModels         []PluginCatalogInventoryModel `json:"image_models"`
	UpstreamImageModels []PluginCatalogInventoryModel `json:"upstream_image_models"`
}

// CatalogInventory reads the same account mappings and saved metadata as the
// native catalog builder. It performs no upstream calls and returns no accounts,
// credentials or outbound identities to the sandboxed catalog plugin.
func (m *PluginManager) CatalogInventory(ctx context.Context, pluginID, groupID int64) (*PluginCatalogInventory, error) {
	installation, err := m.repo.GetByID(ctx, pluginID)
	if err != nil {
		return nil, err
	}
	if !isCatalogPlugin(installation.Manifest) {
		return nil, infraerrors.Forbidden("PLUGIN_CATALOG_REQUIRED", "此插件未声明模型目录能力")
	}
	if m.catalogGroups == nil || m.catalogAccounts == nil {
		return nil, fmt.Errorf("模型目录数据源尚未装配")
	}
	groups, err := m.catalogGroups.ListActiveByPlatform(ctx, PlatformOpenAI)
	if err != nil {
		return nil, fmt.Errorf("读取模型目录分组: %w", err)
	}
	result := &PluginCatalogInventory{Groups: []PluginCatalogGroup{}, Models: []PluginCatalogInventoryModel{}, ImageModels: []PluginCatalogInventoryModel{}, UpstreamImageModels: []PluginCatalogInventoryModel{}}
	var selected *Group
	for i := range groups {
		group := &groups[i]
		if group.Platform != PlatformOpenAI {
			continue
		}
		result.Groups = append(result.Groups, PluginCatalogGroup{ID: group.ID, Name: group.Name})
		if group.ID == groupID || groupID == 0 && selected == nil {
			selected = group
		}
	}
	if selected == nil {
		if groupID != 0 {
			return nil, ErrGroupNotFound
		}
		return result, nil
	}
	result.GroupID = selected.ID
	visible, accounts, err := loadCodexGroupCatalogAccounts(ctx, m.catalogAccounts, selected.ID)
	if err != nil {
		return nil, fmt.Errorf("读取模型目录账号映射: %w", err)
	}
	raw, err := m.decryptConfig(installation)
	if err != nil {
		return nil, err
	}
	c, err := modelcatalog.Parse(raw)
	if err != nil {
		return nil, err
	}
	p := newCatalogPolicy(c, selected.ID, "")
	nativeIDs := openAIConfiguredCodexModelIDsForGroup(visible, selected)
	ids := FilterCodexModelIDsForGroup(nativeIDs, selected)
	ids = slices.DeleteFunc(ids, func(id string) bool { return p.imageModel(id, visible) })
	if selected.AllowImageGeneration {
		err = p.inventoryImages(ctx, selected, visible, nativeIDs, result)
		if err != nil {
			return nil, err
		}
	}
	body, err := buildCodexModelsManifestForAccounts(PlatformOpenAI, ids, accounts, selected, nil, true)
	if err != nil {
		return nil, err
	}
	var native struct {
		Models []json.RawMessage `json:"models"`
	}
	if err = json.Unmarshal(body, &native); err != nil {
		return nil, err
	}
	builtins := map[string]bool{}
	for _, model := range openai.DefaultModels {
		builtins[model.ID] = true
	}
	for _, descriptor := range native.Models {
		var identity struct {
			Slug string `json:"slug"`
		}
		if err = json.Unmarshal(descriptor, &identity); err != nil {
			return nil, err
		}
		source := "upstream"
		if builtins[identity.Slug] {
			source = "builtin"
		}
		result.Models = append(result.Models, PluginCatalogInventoryModel{Source: source, Descriptor: descriptor})
	}
	return result, nil
}

func (p *catalogPolicy) allowsImageAccount(_ context.Context, group *Group, a *Account, id string) bool {
	if group == nil || a == nil || !group.AllowImageGeneration || a.Platform != PlatformOpenAI ||
		!group.ModelAllowlist.Allows(id) || !a.IsModelSupported(id) {
		return false
	}
	request := OpenAIImagesRequest{RequiredCapability: OpenAIImagesCapabilityNative}
	return a.SupportsOpenAIImageCapability(request.RequiredCapabilityForModel(a.GetMappedModel(id)))
}

func (p *catalogPolicy) inventoryImages(ctx context.Context, group *Group, accounts []Account, ids []string, result *PluginCatalogInventory) error {
	ids = group.ModelAllowlist.FilterForListing(ids)
	available := make(map[string]PluginCatalogInventoryModel)
	for _, id := range ids {
		eligible := make([]Account, 0)
		for i := range accounts {
			a := &accounts[i]
			if p.imageModel(id, accounts[i:i+1]) && p.allowsImageAccount(ctx, group, a, id) {
				eligible = append(eligible, *a)
			}
		}
		if len(eligible) == 0 {
			continue
		}
		// Use saved upstream metadata only; absence of edit support is not
		// evidence that a library checkbox can enable it.
		fields := map[string]any{"slug": id, "display_name": id}
		metadata, ok := groupCodexModelMetadata(PlatformOpenAI, id, eligible, group, nil, true)
		if ok {
			if metadata.DisplayName != "" {
				fields["display_name"] = metadata.DisplayName
			}
			if metadata.Description != "" {
				fields["description"] = metadata.Description
			}
			if metadata.InputModalities != nil {
				fields["input_modalities"] = metadata.InputModalities
			}
		}
		descriptor, err := json.Marshal(fields)
		if err != nil {
			return err
		}
		row := PluginCatalogInventoryModel{Source: "upstream", Descriptor: descriptor}
		available[id] = row
		result.UpstreamImageModels = append(result.UpstreamImageModels, row)
	}
	// Publication only narrows the eligible inventory; it must not narrow discovery.
	if p.selection != nil {
		ids = p.selection.ImageModels
	}
	for _, id := range ids {
		row, ok := available[id]
		entry, matched := p.matches[id]
		if !ok || entry.Hidden || (!matched && p.versionLimited[id]) {
			continue
		}
		if item, ok := p.library[id]; ok {
			descriptor, err := applyCatalogLibrary(row.Descriptor, item)
			if err != nil {
				return err
			}
			row.Descriptor = descriptor
		}
		result.ImageModels = append(result.ImageModels, row)
	}
	return nil
}
