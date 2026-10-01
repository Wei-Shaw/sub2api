package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type ModelConfigSearchResult struct {
	Provider     string                     `json:"provider"`
	ProviderName string                     `json:"provider_name,omitempty"`
	ID           string                     `json:"id"`
	Name         string                     `json:"name"`
	Fields       map[string]json.RawMessage `json:"fields"`
}

// codexFieldsFromMetadata 仅导入数据源明确提供的值，不将缺失信息推断为能力关闭。
func codexFieldsFromMetadata(metadata UpstreamModelMetadata) map[string]json.RawMessage {
	// 目录只需能力字段，无需为每个模型构造完整的 Codex 指令模板。
	descriptor := configuredCodexModelDescriptor{InputModalities: []string{"text"}}
	applyUpstreamModelMetadataToCodexDescriptor(&descriptor, codexModelMetadataOverride{UpstreamModelMetadata: metadata})
	// 完整目录包含大量模型，仅编码可导入字段，避免重复序列化 Codex 指令模板。
	all := map[string]any{
		"display_name":               descriptor.DisplayName,
		"description":                descriptor.Description,
		"context_window":             descriptor.ContextWindow,
		"max_context_window":         descriptor.MaxContextWindow,
		"input_modalities":           descriptor.InputModalities,
		"default_reasoning_level":    descriptor.DefaultReasoningLevel,
		"supported_reasoning_levels": descriptor.SupportedReasoningLevels,
	}
	fields := make(map[string]json.RawMessage)
	include := func(condition bool, keys ...string) {
		if condition {
			for _, key := range keys {
				fields[key], _ = json.Marshal(all[key])
			}
		}
	}
	include(metadata.DisplayName != "", "display_name")
	include(metadata.Description != "", "description")
	include(metadata.ContextWindow > 0, "context_window")
	include(metadata.MaxContextWindow > 0 || metadata.ContextWindow > 0, "max_context_window")
	include(len(metadata.InputModalities) > 0, "input_modalities")
	// reasoning=true 不代表存在可调推理档位，目录未列出档位时留给管理员确认。
	include(metadata.Reasoning != nil && (!*metadata.Reasoning || len(metadata.SupportedReasoningLevels) > 0), "default_reasoning_level", "supported_reasoning_levels")
	for key, value := range metadata.CodexToolCapabilities {
		fields[key] = value
	}
	return fields
}

func (s *AccountTestService) SearchModelConfigs(ctx context.Context, query string) ([]ModelConfigSearchResult, error) {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return []ModelConfigSearchResult{}, nil
	}
	registry, err := s.fetchModelsDevRegistry(ctx, &Account{})
	if err != nil {
		return nil, err
	}
	return modelConfigCatalogResults(registry, query, 100), nil
}

// ListModelConfigs 一次返回完整目录，供弹窗加载后在浏览器中即时筛选。
func (s *AccountTestService) ListModelConfigs(ctx context.Context) ([]ModelConfigSearchResult, error) {
	registry, err := s.fetchModelsDevRegistry(ctx, &Account{})
	if err != nil {
		return nil, err
	}
	return modelConfigCatalogResults(registry, "", 0), nil
}

func modelConfigCatalogResults(registry map[string]modelsDevProvider, query string, limit int) []ModelConfigSearchResult {
	results := make([]ModelConfigSearchResult, 0)
	for providerID, provider := range registry {
		for id, model := range provider.Models {
			if !strings.Contains(strings.ToLower(providerID+" "+provider.Name+" "+id+" "+model.Name), query) {
				continue
			}
			metadata := upstreamMetadataFromModelsDevModel(id, model)
			fields := codexFieldsFromMetadata(metadata)
			results = append(results, ModelConfigSearchResult{Provider: providerID, ProviderName: provider.Name, ID: id, Name: model.Name, Fields: fields})
		}
	}
	sort.Slice(results, func(i, j int) bool {
		iExact, jExact := strings.EqualFold(results[i].ID, query), strings.EqualFold(results[j].ID, query)
		if iExact != jExact {
			return iExact
		}
		return results[i].Provider+"/"+results[i].ID < results[j].Provider+"/"+results[j].ID
	})
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results
}

// FetchGroupUpstreamModelConfig 只读导入，不同步账号配置，也不覆盖已保存的人工字段。
func (s *AccountTestService) FetchGroupUpstreamModelConfig(ctx context.Context, group *Group, modelID string) (map[string]json.RawMessage, error) {
	accounts, err := s.accountRepo.ListByGroup(ctx, group.ID)
	if err != nil {
		return nil, err
	}
	targetPlatform, publicTarget := group.Platform, modelID
	if group.Platform == PlatformComposite {
		eligible := make([]Account, 0, len(accounts))
		for _, account := range accounts {
			if account.IsActive() && account.Schedulable {
				eligible = append(eligible, account)
			}
		}
		var routes []CompositeModelRoute
		if s.compositeResolver != nil && s.compositeResolver.repo != nil {
			routes, err = s.compositeResolver.repo.ListByGroup(ctx, group.ID, false)
			if err != nil {
				return nil, fmt.Errorf("load composite model routes: %w", err)
			}
		}
		var resolved bool
		targetPlatform, publicTarget, resolved = resolveCodexCompositeModelTarget(modelID, eligible, routes, true)
		if !resolved {
			return nil, fmt.Errorf("model %q has no unambiguous Composite Responses route", modelID)
		}
	}
	var lastErr error
	for i := range accounts {
		account := &accounts[i]
		if !account.IsActive() || !account.Schedulable || account.Platform != targetPlatform {
			continue
		}
		target := publicTarget
		if !account.IsOpenAIPassthroughEnabled() && len(account.GetModelMapping()) > 0 {
			mapped, matched := account.ResolveMappedModel(publicTarget)
			if !matched {
				continue
			}
			target = mapped
		}
		var body []byte
		if account.Platform == PlatformOpenAI && s.openaiGatewayService != nil {
			manifest, fetchErr := s.openaiGatewayService.FetchCodexModelsManifest(ctx, account, CodexCanonicalClientVersion(), "")
			err = fetchErr
			if manifest != nil {
				body = manifest.Body
				if len(manifest.upstreamSourceBody) > 0 {
					body = manifest.upstreamSourceBody
				}
			}
		} else {
			_, body, err = s.fetchUpstreamModelList(ctx, account)
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err != nil {
			lastErr = err
			continue
		}
		// manifest 原样保留未知字段；普通列表只提取已知能力，避免 id/object 等污染配置。
		var envelope struct {
			Models []map[string]json.RawMessage `json:"models"`
		}
		if json.Unmarshal(body, &envelope) == nil {
			for _, fields := range envelope.Models {
				var slug string
				_ = json.Unmarshal(fields["slug"], &slug)
				if slug == target {
					delete(fields, "slug")
					delete(fields, "id")
					return fields, nil
				}
			}
		}
		_, metadata, parseErr := extractUpstreamModelCatalog(body, account.IsGrok())
		if parseErr == nil {
			if entry, found := metadata[target]; found {
				fields := codexFieldsFromMetadata(entry)
				if len(fields) > 0 {
					return fields, nil
				}
			}
		}
	}
	if lastErr != nil {
		return nil, fmt.Errorf("upstream model configuration unavailable: %w", lastErr)
	}
	return nil, fmt.Errorf("upstreams did not provide configuration for model %q", modelID)
}
