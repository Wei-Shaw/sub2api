package service

import (
	"context"
	"sort"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/modelcatalog"
)

// CatalogStorefrontModel is one checkbox candidate on the channel shelf.
type CatalogStorefrontModel struct {
	ID             string   `json:"id"`
	DisplayName    string   `json:"display_name"`
	CanonicalID    string   `json:"canonical_id"`
	Platforms      []string `json:"platforms"`
	BillingMode    string   `json:"billing_mode"`
	InputPrice     *float64 `json:"input_price,omitempty"`
	OutputPrice    *float64 `json:"output_price,omitempty"`
	CacheWrite     *float64 `json:"cache_write_price,omitempty"`
	CacheRead      *float64 `json:"cache_read_price,omitempty"`
	ImageInput     *float64 `json:"image_input_price,omitempty"`
	ImageOutput    *float64 `json:"image_output_price,omitempty"`
	PerRequest     *float64 `json:"per_request_price,omitempty"`
	CoverageHave   *int     `json:"coverage_have,omitempty"`
	CoverageTotal  *int     `json:"coverage_total,omitempty"`
	CoverageSynced *int     `json:"coverage_synced,omitempty"`
}

// ListCatalogStorefrontModels returns fork-owned catalog models for a platform.
func ListCatalogStorefrontModels(platform string) []CatalogStorefrontModel {
	items := modelcatalog.StorefrontItems(platform)
	out := make([]CatalogStorefrontModel, 0, len(items))
	for _, item := range items {
		entry := CatalogStorefrontModel{
			ID:          item.ID,
			DisplayName: item.DisplayName,
			CanonicalID: item.CanonicalID,
			Platforms:   item.Platforms,
			BillingMode: "token",
		}
		if item.Price != nil {
			entry.InputPrice = perTokenPtr(item.Price.InputPerMTok)
			entry.OutputPrice = perTokenPtr(item.Price.OutputPerMTok)
			entry.CacheWrite = perTokenPtr(item.Price.CacheWritePerMTok)
			entry.CacheRead = perTokenPtr(item.Price.CacheReadPerMTok)
			entry.ImageInput = perTokenPtr(item.Price.ImageInputPerMTok)
			entry.ImageOutput = perTokenPtr(item.Price.ImageOutputPerMTok)
			entry.PerRequest = item.Price.PerRequest
		}
		out = append(out, entry)
	}
	return out
}

func perTokenPtr(perMTok *float64) *float64 {
	if perMTok == nil {
		return nil
	}
	value := modelcatalog.PerToken(*perMTok)
	return &value
}

// ListStorefrontModels returns the models a bound channel is currently selling
// when model restriction is on. enabled=false means fall back to the old
// account-mapping / default list.
func (s *ChannelService) ListStorefrontModels(ctx context.Context, groupID int64, platform string) (models []string, enabled bool) {
	if s == nil {
		return nil, false
	}
	lk, err := s.lookupGroupChannel(ctx, groupID)
	if err != nil || lk == nil || lk.channel == nil || !lk.channel.RestrictModels {
		return nil, false
	}

	want := strings.ToLower(strings.TrimSpace(platform))
	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, pricing := range lk.channel.ModelPricing {
		if want != "" && strings.ToLower(strings.TrimSpace(pricing.Platform)) != want {
			continue
		}
		for _, model := range pricing.Models {
			model = strings.TrimSpace(model)
			if model == "" {
				continue
			}
			key := strings.ToLower(model)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, model)
		}
	}
	sort.Strings(out)
	return out, true
}

func (s *ChannelService) SetAccountRepository(accountRepo AccountRepository) {
	if s == nil {
		return
	}
	s.accountRepo = accountRepo
}

// ListCatalogStorefrontModelsWithCoverage returns catalog models and, when
// group IDs are provided, how many bound accounts currently have each model.
func (s *ChannelService) ListCatalogStorefrontModelsWithCoverage(ctx context.Context, platform string, groupIDs []int64) []CatalogStorefrontModel {
	models := ListCatalogStorefrontModels(platform)
	if s == nil || s.accountRepo == nil || len(groupIDs) == 0 {
		return models
	}
	accounts := make([]Account, 0)
	seenGroups := make(map[int64]struct{}, len(groupIDs))
	for _, groupID := range groupIDs {
		if groupID <= 0 {
			continue
		}
		if _, ok := seenGroups[groupID]; ok {
			continue
		}
		seenGroups[groupID] = struct{}{}
		groupAccounts, err := s.accountRepo.ListByGroup(ctx, groupID)
		if err != nil {
			continue
		}
		accounts = append(accounts, groupAccounts...)
	}
	return AnnotateCatalogStorefrontCoverage(models, filterStorefrontCoverageAccounts(accounts, platform))
}

func filterStorefrontCoverageAccounts(accounts []Account, platform string) []Account {
	want := strings.ToLower(strings.TrimSpace(platform))
	if want == "" {
		return nil
	}
	out := make([]Account, 0, len(accounts))
	seen := make(map[int64]struct{}, len(accounts))
	for _, account := range accounts {
		if account.ID != 0 {
			if _, ok := seen[account.ID]; ok {
				continue
			}
			seen[account.ID] = struct{}{}
		}
		if account.IsShadow() {
			continue
		}
		if strings.ToLower(strings.TrimSpace(account.Platform)) != want {
			continue
		}
		out = append(out, account)
	}
	return out
}

// AnnotateCatalogStorefrontCoverage fills per-model account coverage from
// snapshots. It never uses the intersection as the user-facing shelf.
func AnnotateCatalogStorefrontCoverage(models []CatalogStorefrontModel, accounts []Account) []CatalogStorefrontModel {
	total := len(accounts)
	synced := 0
	indexed := make([]struct {
		account *Account
		models  []string
	}, 0, len(accounts))
	for i := range accounts {
		snapshot := accounts[i].UpstreamModelSnapshot()
		if snapshot == nil || len(snapshot.Models) == 0 {
			continue
		}
		synced++
		indexed = append(indexed, struct {
			account *Account
			models  []string
		}{account: &accounts[i], models: snapshot.Models})
	}
	for i := range models {
		have := 0
		for _, item := range indexed {
			if snapshotCoversRequestedModel(item.account, item.models, models[i].ID) {
				have++
			}
		}
		haveCopy, totalCopy, syncedCopy := have, total, synced
		models[i].CoverageHave = &haveCopy
		models[i].CoverageTotal = &totalCopy
		models[i].CoverageSynced = &syncedCopy
	}
	return models
}
