package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/shopspring/decimal"
)

var (
	ErrOfficialPriceSyncUnavailable = infraerrors.ServiceUnavailable(
		"OFFICIAL_PRICE_SYNC_UNAVAILABLE", "official price candidate source is unavailable",
	)
	ErrOfficialPriceApplyInvalid = infraerrors.BadRequest(
		"OFFICIAL_PRICE_APPLY_INVALID", "official price selection is not applicable",
	)
	ErrOfficialPriceApplyConflict = infraerrors.Conflict(
		"OFFICIAL_PRICE_APPLY_CONFLICT", "display model price changed after preview",
	)
)

const (
	OfficialPriceReasonUnsupportedBillingMode = "unsupported_billing_mode"
	OfficialPriceReasonCurrencyMismatch       = "currency_mismatch"
	OfficialPriceReasonCandidateNotFound      = "candidate_not_found"
	OfficialPriceReasonProviderMismatch       = "provider_mismatch"
	OfficialPriceReasonCandidateDisabled      = "candidate_disabled"
	OfficialPriceReasonCandidatePriceMissing  = "candidate_price_missing"
)

type OfficialPriceValues struct {
	InputPerMillion      *float64 `json:"input_per_million"`
	OutputPerMillion     *float64 `json:"output_per_million"`
	CacheWritePerMillion *float64 `json:"cache_write_per_million"`
	CacheReadPerMillion  *float64 `json:"cache_read_per_million"`
}

type OfficialPriceDiff struct {
	InputPerMillion      bool `json:"input_per_million"`
	OutputPerMillion     bool `json:"output_per_million"`
	CacheWritePerMillion bool `json:"cache_write_per_million"`
	CacheReadPerMillion  bool `json:"cache_read_per_million"`
	HasChanges           bool `json:"has_changes"`
}

type OfficialPricePreviewItem struct {
	ModelID               int64                `json:"model_id"`
	Platform              string               `json:"platform"`
	ModelName             string               `json:"model_name"`
	Provider              string               `json:"provider"`
	BillingMode           string               `json:"billing_mode"`
	Currency              string               `json:"currency"`
	Applicable            bool                 `json:"applicable"`
	Reason                string               `json:"reason,omitempty"`
	Current               OfficialPriceValues  `json:"current"`
	Proposed              *OfficialPriceValues `json:"proposed"`
	Diff                  OfficialPriceDiff    `json:"diff"`
	Changed               bool                 `json:"changed"`
	Source                string               `json:"source"`
	Confidence            string               `json:"confidence"`
	SourceUpdatedAt       *time.Time           `json:"source_updated_at"`
	OfficialReferenceURL  string               `json:"official_reference_url"`
	ExpectedUpdatedAt     time.Time            `json:"expected_updated_at"`
	CurrentPriceSource    string               `json:"current_price_source"`
	CurrentPriceSourceURL string               `json:"current_price_source_url"`
	CurrentPriceSyncedAt  *time.Time           `json:"current_price_synced_at"`
	ProposalHash          string               `json:"proposal_hash,omitempty"`
}

type OfficialPricePreview struct {
	Items     []OfficialPricePreviewItem `json:"items"`
	FetchedAt time.Time                  `json:"fetched_at"`
	Warning   *string                    `json:"warning"`
}

type OfficialPriceApplySelection struct {
	ModelID           int64     `json:"model_id"`
	ExpectedUpdatedAt time.Time `json:"expected_updated_at"`
	ProposalHash      string    `json:"proposal_hash"`
}

type OfficialPriceApplyResult struct {
	AppliedCount int       `json:"applied_count"`
	SyncedAt     time.Time `json:"synced_at"`
}

type OfficialPriceUpdate struct {
	ModelID                int64
	ExpectedUpdatedAt      time.Time
	InputPerMillion        *decimal.Decimal
	OutputPerMillion       *decimal.Decimal
	CacheWritePerMillion   *decimal.Decimal
	CacheReadPerMillion    *decimal.Decimal
	OfficialPriceSource    string
	OfficialPriceSourceURL string
	OfficialPriceSyncedAt  time.Time
}

// OfficialPriceSyncRepository is intentionally narrower than
// DisplayPricingRepository. Its mutating method can only update presentation
// official-price columns in display_model_prices.
type OfficialPriceSyncRepository interface {
	ListModels(context.Context) ([]DisplayModelPrice, error)
	ApplyOfficialPriceUpdates(context.Context, []OfficialPriceUpdate) error
}

type OfficialPriceSyncService struct {
	repo    OfficialPriceSyncRepository
	fetcher OfficialPriceCandidateFetcher
	now     func() time.Time
}

func NewOfficialPriceSyncService(display *DisplayPricingService, fetcher OfficialPriceCandidateFetcher) *OfficialPriceSyncService {
	var repo OfficialPriceSyncRepository
	if display != nil && display.repo != nil {
		repo, _ = display.repo.(OfficialPriceSyncRepository)
	}
	if fetcher == nil {
		fetcher = NewHerohaoOfficialPriceFetcher(nil)
	}
	return &OfficialPriceSyncService{repo: repo, fetcher: fetcher, now: time.Now}
}

func (s *OfficialPriceSyncService) Preview(ctx context.Context) (*OfficialPricePreview, error) {
	models, snapshot, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]OfficialPricePreviewItem, 0, len(models))
	for i := range models {
		if models[i].BillingMode != DisplayBillingModeToken {
			continue
		}
		item, _ := buildOfficialPriceProposal(models[i], snapshot)
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Provider != items[j].Provider {
			return items[i].Provider < items[j].Provider
		}
		if items[i].BillingMode != items[j].BillingMode {
			return items[i].BillingMode < items[j].BillingMode
		}
		return strings.ToLower(items[i].ModelName) < strings.ToLower(items[j].ModelName)
	})
	return &OfficialPricePreview{Items: items, FetchedAt: snapshot.FetchedAt, Warning: snapshot.Warning}, nil
}

func (s *OfficialPriceSyncService) Apply(ctx context.Context, selections []OfficialPriceApplySelection) (*OfficialPriceApplyResult, error) {
	if len(selections) == 0 {
		return nil, ErrOfficialPriceApplyInvalid
	}
	models, snapshot, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	modelByID := make(map[int64]DisplayModelPrice, len(models))
	for i := range models {
		modelByID[models[i].ID] = models[i]
	}

	seen := make(map[int64]struct{}, len(selections))
	syncedAt := s.now().UTC()
	updates := make([]OfficialPriceUpdate, 0, len(selections))
	for _, selection := range selections {
		if selection.ModelID <= 0 || selection.ExpectedUpdatedAt.IsZero() || strings.TrimSpace(selection.ProposalHash) == "" {
			return nil, ErrOfficialPriceApplyInvalid
		}
		if _, duplicate := seen[selection.ModelID]; duplicate {
			return nil, ErrOfficialPriceApplyInvalid
		}
		seen[selection.ModelID] = struct{}{}
		model, ok := modelByID[selection.ModelID]
		if !ok {
			return nil, ErrOfficialPriceApplyConflict
		}
		if !model.UpdatedAt.Equal(selection.ExpectedUpdatedAt) {
			return nil, ErrOfficialPriceApplyConflict
		}
		item, proposal := buildOfficialPriceProposal(model, snapshot)
		if !item.Applicable || proposal == nil {
			return nil, ErrOfficialPriceApplyInvalid.WithMetadata(map[string]string{
				"model_id": fmt.Sprintf("%d", selection.ModelID), "reason": item.Reason,
			})
		}
		if item.ProposalHash != strings.TrimSpace(selection.ProposalHash) {
			return nil, ErrOfficialPriceApplyConflict
		}
		updates = append(updates, OfficialPriceUpdate{
			ModelID: selection.ModelID, ExpectedUpdatedAt: selection.ExpectedUpdatedAt,
			InputPerMillion: proposal.Input, OutputPerMillion: proposal.Output,
			CacheWritePerMillion: proposal.CacheWrite, CacheReadPerMillion: proposal.CacheRead,
			OfficialPriceSource:    OfficialPriceSourceHerohaoAggregate,
			OfficialPriceSourceURL: HerohaoOfficialPriceCandidateURL,
			OfficialPriceSyncedAt:  syncedAt,
		})
	}
	if err := s.repo.ApplyOfficialPriceUpdates(ctx, updates); err != nil {
		return nil, err
	}
	return &OfficialPriceApplyResult{AppliedCount: len(updates), SyncedAt: syncedAt}, nil
}

func (s *OfficialPriceSyncService) load(ctx context.Context) ([]DisplayModelPrice, *OfficialPriceSourceSnapshot, error) {
	if s == nil || s.repo == nil || s.fetcher == nil {
		return nil, nil, ErrOfficialPriceSyncUnavailable
	}
	models, err := s.repo.ListModels(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("list display models for official price sync: %w", err)
	}
	snapshot, err := s.fetcher.Fetch(ctx)
	if err != nil {
		return nil, nil, ErrOfficialPriceSyncUnavailable.WithCause(err)
	}
	if snapshot == nil {
		return nil, nil, ErrOfficialPriceSyncUnavailable
	}
	return models, snapshot, nil
}

type officialPriceDecimalValues struct {
	Input      *decimal.Decimal
	Output     *decimal.Decimal
	CacheWrite *decimal.Decimal
	CacheRead  *decimal.Decimal
}

func buildOfficialPriceProposal(model DisplayModelPrice, snapshot *OfficialPriceSourceSnapshot) (OfficialPricePreviewItem, *officialPriceDecimalValues) {
	item := OfficialPricePreviewItem{
		ModelID: model.ID, Platform: model.Platform, ModelName: model.ModelName, Provider: model.Provider,
		BillingMode: model.BillingMode, Currency: model.Currency, ExpectedUpdatedAt: model.UpdatedAt,
		Current: officialPriceValuesFromModel(model), Source: OfficialPriceSourceHerohaoAggregate,
		Confidence: OfficialPriceSourceConfidence, SourceUpdatedAt: snapshot.UpdatedAt,
		OfficialReferenceURL: officialReferenceURL(model.Provider),
		CurrentPriceSource:   model.OfficialPriceSource, CurrentPriceSourceURL: model.OfficialPriceSourceURL,
		CurrentPriceSyncedAt: model.OfficialPriceSyncedAt,
	}
	if model.BillingMode != DisplayBillingModeToken {
		item.Reason = OfficialPriceReasonUnsupportedBillingMode
		return item, nil
	}
	if model.Currency != DisplayCurrencyCNY {
		item.Reason = OfficialPriceReasonCurrencyMismatch
		return item, nil
	}
	candidate, ok := snapshot.Models[model.ModelName] // exact, case-sensitive identity only
	if !ok {
		item.Reason = OfficialPriceReasonCandidateNotFound
		return item, nil
	}
	if candidate.UpdatedAt != nil {
		item.SourceUpdatedAt = candidate.UpdatedAt
	}
	if expectedProvider := herohaoDisplayProvider(candidate.ProviderKey); expectedProvider == "" || expectedProvider != model.Provider {
		item.Reason = OfficialPriceReasonProviderMismatch
		return item, nil
	}
	if candidate.Currency != DisplayCurrencyCNY {
		item.Reason = OfficialPriceReasonCurrencyMismatch
		return item, nil
	}
	if !candidate.Enabled {
		item.Reason = OfficialPriceReasonCandidateDisabled
		return item, nil
	}
	if candidate.Input == nil && candidate.Output == nil && candidate.CacheWrite == nil && candidate.CacheRead == nil {
		item.Reason = OfficialPriceReasonCandidatePriceMissing
		return item, nil
	}

	proposal := &officialPriceDecimalValues{
		Input:      mergeOfficialDecimal(candidate.Input, model.OfficialInputPerMillion),
		Output:     mergeOfficialDecimal(candidate.Output, model.OfficialOutputPerMillion),
		CacheWrite: mergeOfficialDecimal(candidate.CacheWrite, model.OfficialCacheWritePerMillion),
		CacheRead:  mergeOfficialDecimal(candidate.CacheRead, model.OfficialCacheReadPerMillion),
	}
	proposed := officialPriceValuesFromDecimal(proposal)
	item.Proposed = &proposed
	item.Diff = compareOfficialPriceValues(item.Current, proposed)
	item.Changed = item.Diff.HasChanges
	item.ProposalHash = officialPriceProposalHash(model.ID, proposed)
	item.Applicable = true
	return item, proposal
}

func officialPriceProposalHash(modelID int64, values OfficialPriceValues) string {
	parts := []string{
		strconv.FormatInt(modelID, 10),
		officialPriceHashValue(values.InputPerMillion),
		officialPriceHashValue(values.OutputPerMillion),
		officialPriceHashValue(values.CacheWritePerMillion),
		officialPriceHashValue(values.CacheReadPerMillion),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

func officialPriceHashValue(value *float64) string {
	if value == nil {
		return "<nil>"
	}
	return decimal.NewFromFloat(*value).Round(8).StringFixed(8)
}

func mergeOfficialDecimal(candidate *decimal.Decimal, current *float64) *decimal.Decimal {
	if candidate != nil {
		value := candidate.Round(8)
		return &value
	}
	if current == nil {
		return nil
	}
	value := decimal.NewFromFloat(*current).Round(8)
	return &value
}

func officialPriceValuesFromModel(model DisplayModelPrice) OfficialPriceValues {
	return OfficialPriceValues{
		InputPerMillion:      cloneFloatPtr(model.OfficialInputPerMillion),
		OutputPerMillion:     cloneFloatPtr(model.OfficialOutputPerMillion),
		CacheWritePerMillion: cloneFloatPtr(model.OfficialCacheWritePerMillion),
		CacheReadPerMillion:  cloneFloatPtr(model.OfficialCacheReadPerMillion),
	}
}

func officialPriceValuesFromDecimal(values *officialPriceDecimalValues) OfficialPriceValues {
	return OfficialPriceValues{
		InputPerMillion: decimalFloatPtr(values.Input), OutputPerMillion: decimalFloatPtr(values.Output),
		CacheWritePerMillion: decimalFloatPtr(values.CacheWrite), CacheReadPerMillion: decimalFloatPtr(values.CacheRead),
	}
}

func decimalFloatPtr(value *decimal.Decimal) *float64 {
	if value == nil {
		return nil
	}
	out, _ := value.Round(8).Float64()
	return &out
}

func cloneFloatPtr(value *float64) *float64 {
	if value == nil {
		return nil
	}
	out := *value
	return &out
}

func compareOfficialPriceValues(current, proposed OfficialPriceValues) OfficialPriceDiff {
	diff := OfficialPriceDiff{
		InputPerMillion:      !equalOfficialFloat(current.InputPerMillion, proposed.InputPerMillion),
		OutputPerMillion:     !equalOfficialFloat(current.OutputPerMillion, proposed.OutputPerMillion),
		CacheWritePerMillion: !equalOfficialFloat(current.CacheWritePerMillion, proposed.CacheWritePerMillion),
		CacheReadPerMillion:  !equalOfficialFloat(current.CacheReadPerMillion, proposed.CacheReadPerMillion),
	}
	diff.HasChanges = diff.InputPerMillion || diff.OutputPerMillion || diff.CacheWritePerMillion || diff.CacheReadPerMillion
	return diff
}

func equalOfficialFloat(left, right *float64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return decimal.NewFromFloat(*left).Round(8).Equal(decimal.NewFromFloat(*right).Round(8))
}

func herohaoDisplayProvider(providerKey string) string {
	return map[string]string{
		"deepseek": "deepseek", "glm": "zhipu", "kimi": "moonshot", "minimax": "minimax",
		"qwen": "qwen", "mimo": "mimo", "hunyuan": "hunyuan",
	}[strings.ToLower(strings.TrimSpace(providerKey))]
}

func officialReferenceURL(provider string) string {
	return map[string]string{
		"deepseek":  "https://api-docs.deepseek.com/zh-cn/quick_start/pricing",
		"zhipu":     "https://open.bigmodel.cn/pricing",
		"moonshot":  "https://platform.kimi.com/docs/pricing/chat",
		"minimax":   "https://platform.minimaxi.com/docs/guides/pricing-paygo",
		"qwen":      "https://help.aliyun.com/zh/model-studio/model-pricing",
		"mimo":      "https://mimo.mi.com/docs/zh-CN/price/pay-as-you-go",
		"hunyuan":   "https://cloud.tencent.com/document/product/1729",
		"openai":    "https://developers.openai.com/api/docs/pricing",
		"anthropic": "https://platform.claude.com/docs/en/about-claude/pricing",
		"gemini":    "https://ai.google.dev/gemini-api/docs/pricing",
		"grok":      "https://docs.x.ai/developers/pricing",
	}[normalizeDisplayProvider(provider)]
}
