package service

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/modelcatalog"
	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/geminicli"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
)

// PlatformDefaultModelIDs returns the fallback public list for a platform:
// handwritten package defaults plus this fork's catalog IDs. Catalog-only
// models like Gemini 3.7 therefore show up without waiting for upstream
// DefaultModels updates. This list is not the user shelf when a channel
// storefront is bound.
func PlatformDefaultModelIDs(platform string) []string {
	platform = strings.ToLower(strings.TrimSpace(platform))
	if platform == PlatformComposite {
		return mergeUniqueModelIDs(
			PlatformDefaultModelIDs(PlatformAnthropic),
			PlatformDefaultModelIDs(PlatformGemini),
			PlatformDefaultModelIDs(PlatformOpenAI),
			PlatformDefaultModelIDs(PlatformAntigravity),
			PlatformDefaultModelIDs(PlatformGrok),
		)
	}
	return mergeUniqueModelIDs(packageDefaultModelIDs(platform), modelcatalog.PublicIDs(platform))
}

func packageDefaultModelIDs(platform string) []string {
	switch platform {
	case PlatformOpenAI:
		return openai.DefaultModelIDs()
	case PlatformGemini:
		ids := make([]string, 0, len(geminicli.DefaultModels))
		for _, model := range geminicli.DefaultModels {
			ids = append(ids, model.ID)
		}
		return ids
	case PlatformAntigravity:
		models := antigravity.DefaultModels()
		ids := make([]string, 0, len(models))
		for _, model := range models {
			ids = append(ids, model.ID)
		}
		return ids
	case PlatformGrok:
		return xai.DefaultModelIDs()
	default:
		ids := make([]string, 0, len(claude.DefaultModels))
		for _, model := range claude.DefaultModels {
			ids = append(ids, model.ID)
		}
		return ids
	}
}

func mergeUniqueModelIDs(groups ...[]string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, group := range groups {
		for _, model := range group {
			model = strings.TrimSpace(model)
			if model == "" {
				continue
			}
			if _, ok := seen[model]; ok {
				continue
			}
			seen[model] = struct{}{}
			out = append(out, model)
		}
	}
	return out
}

func applyCatalogDefaultMappings(account *Account, mapping map[string]string) map[string]string {
	if account == nil || account.ModelMappingRestricts() {
		return mapping
	}
	extras := modelcatalog.DefaultMappings(account.Platform)
	if len(extras) == 0 {
		return mapping
	}
	out := mapping
	cloned := false
	for src, dst := range extras {
		src = strings.TrimSpace(src)
		dst = strings.TrimSpace(dst)
		if src == "" || dst == "" {
			continue
		}
		if _, exists := out[src]; exists {
			continue
		}
		if !cloned {
			next := make(map[string]string, len(mapping)+len(extras))
			for key, value := range mapping {
				next[key] = value
			}
			out = next
			cloned = true
		}
		out[src] = dst
	}
	return out
}
