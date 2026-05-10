package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/Wei-Shaw/sub2api/ent"
	entModelPricing "github.com/Wei-Shaw/sub2api/ent/modelpricing"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

type ModelPricingEntry struct {
	ID                                  int64     `json:"id"`
	Model                               string    `json:"model"`
	InputCostPerToken                   float64   `json:"input_cost_per_token"`
	OutputCostPerToken                  float64   `json:"output_cost_per_token"`
	CacheCreationInputTokenCost         *float64  `json:"cache_creation_input_token_cost"`
	CacheCreationInputTokenCostAbove1hr *float64  `json:"cache_creation_input_token_cost_above_1hr"`
	CacheReadInputTokenCost             *float64  `json:"cache_read_input_token_cost"`
	InputCostPerTokenPriority           *float64  `json:"input_cost_per_token_priority"`
	OutputCostPerTokenPriority          *float64  `json:"output_cost_per_token_priority"`
	CacheReadInputTokenCostPriority     *float64  `json:"cache_read_input_token_cost_priority"`
	OutputCostPerImage                  *float64  `json:"output_cost_per_image"`
	OutputCostPerImageToken             *float64  `json:"output_cost_per_image_token"`
	LongContextInputTokenThreshold      *int      `json:"long_context_input_token_threshold"`
	LongContextInputCostMultiplier      *float64  `json:"long_context_input_cost_multiplier"`
	LongContextOutputCostMultiplier     *float64  `json:"long_context_output_cost_multiplier"`
	SupportsServiceTier                 bool      `json:"supports_service_tier"`
	LitellmProvider                     string    `json:"litellm_provider"`
	Mode                                string    `json:"mode"`
	SupportsPromptCaching               bool      `json:"supports_prompt_caching"`
	Locked                              bool      `json:"locked"`
	Source                              string    `json:"source"`
	CreatedAt                           time.Time `json:"created_at"`
	UpdatedAt                           time.Time `json:"updated_at"`
}

type ModelPricingListParams struct {
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
	Search   string `json:"search"`
	Source   string `json:"source"`
}

type ModelPricingListResult struct {
	Items    []ModelPricingEntry `json:"items"`
	Total    int                 `json:"total"`
	Page     int                 `json:"page"`
	PageSize int                 `json:"page_size"`
}

type SyncStatus struct {
	AutoSyncEnabled bool       `json:"auto_sync_enabled"`
	LastSyncedAt    *time.Time `json:"last_synced_at"`
	ModelCount      int        `json:"model_count"`
}

type ModelPricingAdminService struct {
	client         *ent.Client
	db             *sql.DB
	cfg            *config.Config
	channelService *ChannelService
	pricingService *PricingService
	mu             sync.RWMutex
	cache          map[string]*LiteLLMModelPricing

	remoteClient PricingRemoteClient
	stopCh       chan struct{}
	wg           sync.WaitGroup

	autoSyncEnabled bool
	lastSyncedAt    time.Time
}

func NewModelPricingAdminService(client *ent.Client, db *sql.DB, cfg *config.Config, remoteClient PricingRemoteClient, channelService *ChannelService, pricingService *PricingService) *ModelPricingAdminService {
	return &ModelPricingAdminService{
		client:          client,
		db:              db,
		cfg:             cfg,
		channelService:  channelService,
		pricingService:  pricingService,
		remoteClient:    remoteClient,
		cache:           make(map[string]*LiteLLMModelPricing),
		stopCh:          make(chan struct{}),
		autoSyncEnabled: true,
	}
}

func (s *ModelPricingAdminService) Initialize(ctx context.Context) error {
	count, err := s.client.ModelPricing.Query().Count(ctx)
	if err != nil {
		return fmt.Errorf("check model_pricing count: %w", err)
	}

	if count == 0 {
		log.Printf("[ModelPricingAdmin] No pricing data in DB, performing initial sync from remote")
		if syncErr := s.SyncFromRemote(ctx); syncErr != nil {
			log.Printf("[ModelPricingAdmin] Initial remote sync failed: %v, trying fallback file", syncErr)
			if fallbackErr := s.loadFallbackToDB(ctx); fallbackErr != nil {
				return fmt.Errorf("initial sync failed and no fallback: %w (fallback: %v)", syncErr, fallbackErr)
			}
		}
	} else {
		log.Printf("[ModelPricingAdmin] Found %d pricing entries in DB, loading cache", count)
	}

	if err := s.refreshCache(ctx); err != nil {
		return fmt.Errorf("refresh cache: %w", err)
	}

	if s.autoSyncEnabled {
		s.startSyncScheduler()
	}

	return nil
}

func (s *ModelPricingAdminService) Stop() {
	close(s.stopCh)
	s.wg.Wait()
}

func (s *ModelPricingAdminService) GetPricingFromCache(modelName string) *LiteLLMModelPricing {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cache[strings.ToLower(strings.TrimSpace(modelName))]
}

func (s *ModelPricingAdminService) GetAllPricingFromCache() map[string]*LiteLLMModelPricing {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make(map[string]*LiteLLMModelPricing, len(s.cache))
	for k, v := range s.cache {
		result[k] = v
	}
	return result
}

func (s *ModelPricingAdminService) List(ctx context.Context, params ModelPricingListParams) (*ModelPricingListResult, error) {
	if params.Page < 1 {
		params.Page = 1
	}
	if params.PageSize < 1 || params.PageSize > 200 {
		params.PageSize = 50
	}

	query := s.client.ModelPricing.Query()

	if params.Search != "" {
		query = query.Where(func(s *entsql.Selector) {
			s.Where(entsql.ContainsFold(entModelPricing.FieldModel, params.Search))
		})
	}
	if params.Source != "" {
		query = query.Where(entModelPricing.SourceEQ(params.Source))
	}

	total, err := query.Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("count model pricing: %w", err)
	}

	offset := (params.Page - 1) * params.PageSize
	items, err := query.
		Order(ent.Asc(entModelPricing.FieldModel)).
		Offset(offset).
		Limit(params.PageSize).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list model pricing: %w", err)
	}

	entries := make([]ModelPricingEntry, 0, len(items))
	for _, item := range items {
		entries = append(entries, dbModelToEntry(item))
	}

	return &ModelPricingListResult{
		Items:    entries,
		Total:    total,
		Page:     params.Page,
		PageSize: params.PageSize,
	}, nil
}

func (s *ModelPricingAdminService) Create(ctx context.Context, entry ModelPricingEntry) (*ModelPricingEntry, error) {
	entry.Model = strings.ToLower(strings.TrimSpace(entry.Model))
	if entry.Model == "" {
		return nil, fmt.Errorf("model name is required")
	}
	entry.Source = "manual"

	builder := s.client.ModelPricing.Create().
		SetModel(entry.Model).
		SetInputCostPerToken(entry.InputCostPerToken).
		SetOutputCostPerToken(entry.OutputCostPerToken).
		SetLocked(entry.Locked).
		SetSource(entry.Source).
		SetSupportsServiceTier(entry.SupportsServiceTier).
		SetSupportsPromptCaching(entry.SupportsPromptCaching)

	if entry.CacheCreationInputTokenCost != nil {
		builder.SetCacheCreationInputTokenCost(*entry.CacheCreationInputTokenCost)
	}
	if entry.CacheCreationInputTokenCostAbove1hr != nil {
		builder.SetCacheCreationInputTokenCostAbove1hr(*entry.CacheCreationInputTokenCostAbove1hr)
	}
	if entry.CacheReadInputTokenCost != nil {
		builder.SetCacheReadInputTokenCost(*entry.CacheReadInputTokenCost)
	}
	if entry.InputCostPerTokenPriority != nil {
		builder.SetInputCostPerTokenPriority(*entry.InputCostPerTokenPriority)
	}
	if entry.OutputCostPerTokenPriority != nil {
		builder.SetOutputCostPerTokenPriority(*entry.OutputCostPerTokenPriority)
	}
	if entry.CacheReadInputTokenCostPriority != nil {
		builder.SetCacheReadInputTokenCostPriority(*entry.CacheReadInputTokenCostPriority)
	}
	if entry.OutputCostPerImage != nil {
		builder.SetOutputCostPerImage(*entry.OutputCostPerImage)
	}
	if entry.OutputCostPerImageToken != nil {
		builder.SetOutputCostPerImageToken(*entry.OutputCostPerImageToken)
	}
	if entry.LongContextInputTokenThreshold != nil {
		builder.SetLongContextInputTokenThreshold(*entry.LongContextInputTokenThreshold)
	}
	if entry.LongContextInputCostMultiplier != nil {
		builder.SetLongContextInputCostMultiplier(*entry.LongContextInputCostMultiplier)
	}
	if entry.LongContextOutputCostMultiplier != nil {
		builder.SetLongContextOutputCostMultiplier(*entry.LongContextOutputCostMultiplier)
	}
	if entry.LitellmProvider != "" {
		builder.SetLitellmProvider(entry.LitellmProvider)
	}
	if entry.Mode != "" {
		builder.SetMode(entry.Mode)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("create model pricing: %w", err)
	}

	s.refreshCacheAsync(ctx)
	result := dbModelToEntry(created)
	return &result, nil
}

func (s *ModelPricingAdminService) Update(ctx context.Context, id int64, entry ModelPricingEntry) (*ModelPricingEntry, error) {
	builder := s.client.ModelPricing.UpdateOneID(id).
		SetInputCostPerToken(entry.InputCostPerToken).
		SetOutputCostPerToken(entry.OutputCostPerToken).
		SetLocked(entry.Locked).
		SetSupportsServiceTier(entry.SupportsServiceTier).
		SetSupportsPromptCaching(entry.SupportsPromptCaching)

	if entry.CacheCreationInputTokenCost != nil {
		builder.SetCacheCreationInputTokenCost(*entry.CacheCreationInputTokenCost)
	} else {
		builder.ClearCacheCreationInputTokenCost()
	}
	if entry.CacheCreationInputTokenCostAbove1hr != nil {
		builder.SetCacheCreationInputTokenCostAbove1hr(*entry.CacheCreationInputTokenCostAbove1hr)
	} else {
		builder.ClearCacheCreationInputTokenCostAbove1hr()
	}
	if entry.CacheReadInputTokenCost != nil {
		builder.SetCacheReadInputTokenCost(*entry.CacheReadInputTokenCost)
	} else {
		builder.ClearCacheReadInputTokenCost()
	}
	if entry.InputCostPerTokenPriority != nil {
		builder.SetInputCostPerTokenPriority(*entry.InputCostPerTokenPriority)
	} else {
		builder.ClearInputCostPerTokenPriority()
	}
	if entry.OutputCostPerTokenPriority != nil {
		builder.SetOutputCostPerTokenPriority(*entry.OutputCostPerTokenPriority)
	} else {
		builder.ClearOutputCostPerTokenPriority()
	}
	if entry.CacheReadInputTokenCostPriority != nil {
		builder.SetCacheReadInputTokenCostPriority(*entry.CacheReadInputTokenCostPriority)
	} else {
		builder.ClearCacheReadInputTokenCostPriority()
	}
	if entry.OutputCostPerImage != nil {
		builder.SetOutputCostPerImage(*entry.OutputCostPerImage)
	} else {
		builder.ClearOutputCostPerImage()
	}
	if entry.OutputCostPerImageToken != nil {
		builder.SetOutputCostPerImageToken(*entry.OutputCostPerImageToken)
	} else {
		builder.ClearOutputCostPerImageToken()
	}
	if entry.LongContextInputTokenThreshold != nil {
		builder.SetLongContextInputTokenThreshold(*entry.LongContextInputTokenThreshold)
	} else {
		builder.ClearLongContextInputTokenThreshold()
	}
	if entry.LongContextInputCostMultiplier != nil {
		builder.SetLongContextInputCostMultiplier(*entry.LongContextInputCostMultiplier)
	} else {
		builder.ClearLongContextInputCostMultiplier()
	}
	if entry.LongContextOutputCostMultiplier != nil {
		builder.SetLongContextOutputCostMultiplier(*entry.LongContextOutputCostMultiplier)
	} else {
		builder.ClearLongContextOutputCostMultiplier()
	}
	if entry.LitellmProvider != "" {
		builder.SetLitellmProvider(entry.LitellmProvider)
	}
	if entry.Mode != "" {
		builder.SetMode(entry.Mode)
	}

	updated, err := builder.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("update model pricing: %w", err)
	}

	s.refreshCacheAsync(ctx)
	result := dbModelToEntry(updated)
	return &result, nil
}

func (s *ModelPricingAdminService) Delete(ctx context.Context, id int64) error {
	err := s.client.ModelPricing.DeleteOneID(id).Exec(ctx)
	if err != nil {
		return fmt.Errorf("delete model pricing: %w", err)
	}
	s.refreshCacheAsync(ctx)
	return nil
}

func (s *ModelPricingAdminService) BulkDelete(ctx context.Context, ids []int64) error {
	_, err := s.client.ModelPricing.Delete().
		Where(entModelPricing.IDIn(ids...)).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("bulk delete model pricing: %w", err)
	}
	s.refreshCacheAsync(ctx)
	return nil
}

func (s *ModelPricingAdminService) SyncFromRemote(ctx context.Context) error {
	remoteURL := s.cfg.Pricing.RemoteURL
	if remoteURL == "" {
		return fmt.Errorf("remote URL not configured")
	}

	data, err := s.remoteClient.FetchPricingJSON(ctx, remoteURL)
	if err != nil {
		return fmt.Errorf("fetch remote pricing: %w", err)
	}

	var raw map[string]LiteLLMRawEntry
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("parse pricing JSON: %w", err)
	}

	synced := 0
	skipped := 0
	for modelName, entry := range raw {
		if entry.InputCostPerToken == nil && entry.OutputCostPerToken == nil {
			skipped++
			continue
		}

		modelLower := strings.ToLower(modelName)
		existing, findErr := s.client.ModelPricing.Query().
			Where(entModelPricing.ModelEQ(modelLower)).
			Only(ctx)

		if findErr != nil && !ent.IsNotFound(findErr) {
			continue
		}

		if existing != nil && existing.Locked {
			continue
		}

		pricing := rawEntryToLiteLLM(entry)

		if existing != nil {
			builder := s.client.ModelPricing.UpdateOneID(existing.ID).
				SetInputCostPerToken(pricing.InputCostPerToken).
				SetOutputCostPerToken(pricing.OutputCostPerToken).
				SetSource("remote")

			if pricing.CacheCreationInputTokenCost > 0 {
				builder.SetCacheCreationInputTokenCost(pricing.CacheCreationInputTokenCost)
			}
			if pricing.CacheCreationInputTokenCostAbove1hr > 0 {
				builder.SetCacheCreationInputTokenCostAbove1hr(pricing.CacheCreationInputTokenCostAbove1hr)
			}
			if pricing.CacheReadInputTokenCost > 0 {
				builder.SetCacheReadInputTokenCost(pricing.CacheReadInputTokenCost)
			}
			if pricing.InputCostPerTokenPriority > 0 {
				builder.SetInputCostPerTokenPriority(pricing.InputCostPerTokenPriority)
			}
			if pricing.OutputCostPerTokenPriority > 0 {
				builder.SetOutputCostPerTokenPriority(pricing.OutputCostPerTokenPriority)
			}
			if pricing.CacheReadInputTokenCostPriority > 0 {
				builder.SetCacheReadInputTokenCostPriority(pricing.CacheReadInputTokenCostPriority)
			}
			if pricing.OutputCostPerImage > 0 {
				builder.SetOutputCostPerImage(pricing.OutputCostPerImage)
			}
			if pricing.OutputCostPerImageToken > 0 {
				builder.SetOutputCostPerImageToken(pricing.OutputCostPerImageToken)
			}
			if pricing.LongContextInputTokenThreshold > 0 {
				builder.SetLongContextInputTokenThreshold(pricing.LongContextInputTokenThreshold)
			}
			if pricing.LongContextInputCostMultiplier > 0 {
				builder.SetLongContextInputCostMultiplier(pricing.LongContextInputCostMultiplier)
			}
			if pricing.LongContextOutputCostMultiplier > 0 {
				builder.SetLongContextOutputCostMultiplier(pricing.LongContextOutputCostMultiplier)
			}
			builder.SetSupportsServiceTier(pricing.SupportsServiceTier)
			if pricing.LiteLLMProvider != "" {
				builder.SetLitellmProvider(pricing.LiteLLMProvider)
			}
			if pricing.Mode != "" {
				builder.SetMode(pricing.Mode)
			}
			builder.SetSupportsPromptCaching(pricing.SupportsPromptCaching)

			if _, err := builder.Save(ctx); err != nil {
				logger.LegacyPrintf("service.model_pricing_admin", "[ModelPricingAdmin] Failed to update %s: %v", modelLower, err)
				continue
			}
		} else {
			builder := s.client.ModelPricing.Create().
				SetModel(modelLower).
				SetInputCostPerToken(pricing.InputCostPerToken).
				SetOutputCostPerToken(pricing.OutputCostPerToken).
				SetSource("remote").
				SetSupportsServiceTier(pricing.SupportsServiceTier).
				SetSupportsPromptCaching(pricing.SupportsPromptCaching)

			if pricing.CacheCreationInputTokenCost > 0 {
				builder.SetCacheCreationInputTokenCost(pricing.CacheCreationInputTokenCost)
			}
			if pricing.CacheCreationInputTokenCostAbove1hr > 0 {
				builder.SetCacheCreationInputTokenCostAbove1hr(pricing.CacheCreationInputTokenCostAbove1hr)
			}
			if pricing.CacheReadInputTokenCost > 0 {
				builder.SetCacheReadInputTokenCost(pricing.CacheReadInputTokenCost)
			}
			if pricing.InputCostPerTokenPriority > 0 {
				builder.SetInputCostPerTokenPriority(pricing.InputCostPerTokenPriority)
			}
			if pricing.OutputCostPerTokenPriority > 0 {
				builder.SetOutputCostPerTokenPriority(pricing.OutputCostPerTokenPriority)
			}
			if pricing.CacheReadInputTokenCostPriority > 0 {
				builder.SetCacheReadInputTokenCostPriority(pricing.CacheReadInputTokenCostPriority)
			}
			if pricing.OutputCostPerImage > 0 {
				builder.SetOutputCostPerImage(pricing.OutputCostPerImage)
			}
			if pricing.OutputCostPerImageToken > 0 {
				builder.SetOutputCostPerImageToken(pricing.OutputCostPerImageToken)
			}
			if pricing.LongContextInputTokenThreshold > 0 {
				builder.SetLongContextInputTokenThreshold(pricing.LongContextInputTokenThreshold)
			}
			if pricing.LongContextInputCostMultiplier > 0 {
				builder.SetLongContextInputCostMultiplier(pricing.LongContextInputCostMultiplier)
			}
			if pricing.LongContextOutputCostMultiplier > 0 {
				builder.SetLongContextOutputCostMultiplier(pricing.LongContextOutputCostMultiplier)
			}
			if pricing.LiteLLMProvider != "" {
				builder.SetLitellmProvider(pricing.LiteLLMProvider)
			}
			if pricing.Mode != "" {
				builder.SetMode(pricing.Mode)
			}

			if _, err := builder.Save(ctx); err != nil {
				logger.LegacyPrintf("service.model_pricing_admin", "[ModelPricingAdmin] Failed to create %s: %v", modelLower, err)
				continue
			}
		}
		synced++
	}

	s.lastSyncedAt = time.Now()
	log.Printf("[ModelPricingAdmin] Synced %d models (skipped %d invalid)", synced, skipped)

	if err := s.refreshCache(ctx); err != nil {
		log.Printf("[ModelPricingAdmin] Cache refresh after sync failed: %v", err)
	}

	return nil
}

func (s *ModelPricingAdminService) GetSyncStatus(ctx context.Context) (*SyncStatus, error) {
	count, err := s.client.ModelPricing.Query().Count(ctx)
	if err != nil {
		return nil, err
	}
	return &SyncStatus{
		AutoSyncEnabled: s.autoSyncEnabled,
		LastSyncedAt:    &s.lastSyncedAt,
		ModelCount:      count,
	}, nil
}

func (s *ModelPricingAdminService) SetAutoSync(enabled bool) {
	s.autoSyncEnabled = enabled
	if enabled {
		s.startSyncScheduler()
	}
}

func (s *ModelPricingAdminService) refreshCache(ctx context.Context) error {
	items, err := s.client.ModelPricing.Query().All(ctx)
	if err != nil {
		return err
	}

	newCache := make(map[string]*LiteLLMModelPricing, len(items))
	for _, item := range items {
		newCache[item.Model] = dbModelToLiteLLM(item)
	}

	s.mu.Lock()
	s.cache = newCache
	s.mu.Unlock()

	log.Printf("[ModelPricingAdmin] Cache refreshed with %d models", len(newCache))
	return nil
}

func (s *ModelPricingAdminService) refreshCacheAsync(ctx context.Context) {
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := s.refreshCache(bgCtx); err != nil {
			log.Printf("[ModelPricingAdmin] Async cache refresh failed: %v", err)
		}
	}()
}

func (s *ModelPricingAdminService) startSyncScheduler() {
	interval := 24
	if s.cfg.Pricing.UpdateIntervalHours > 0 {
		interval = s.cfg.Pricing.UpdateIntervalHours
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(time.Duration(interval) * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-s.stopCh:
				return
			case <-ticker.C:
				if !s.autoSyncEnabled {
					continue
				}
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
				if err := s.SyncFromRemote(ctx); err != nil {
					log.Printf("[ModelPricingAdmin] Scheduled sync failed: %v", err)
				}
				cancel()
			}
		}
	}()
}

func (s *ModelPricingAdminService) loadFallbackToDB(ctx context.Context) error {
	fallbackFile := s.cfg.Pricing.FallbackFile
	data, err := loadFallbackData(fallbackFile)
	if err != nil {
		return err
	}

	var raw map[string]LiteLLMRawEntry
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("parse fallback JSON: %w", err)
	}

	loaded := 0
	for modelName, entry := range raw {
		if entry.InputCostPerToken == nil && entry.OutputCostPerToken == nil {
			continue
		}

		modelLower := strings.ToLower(modelName)
		pricing := rawEntryToLiteLLM(entry)

		builder := s.client.ModelPricing.Create().
			SetModel(modelLower).
			SetInputCostPerToken(pricing.InputCostPerToken).
			SetOutputCostPerToken(pricing.OutputCostPerToken).
			SetSource("remote").
			SetSupportsServiceTier(pricing.SupportsServiceTier).
			SetSupportsPromptCaching(pricing.SupportsPromptCaching)

		if pricing.CacheCreationInputTokenCost > 0 {
			builder.SetCacheCreationInputTokenCost(pricing.CacheCreationInputTokenCost)
		}
		if pricing.CacheReadInputTokenCost > 0 {
			builder.SetCacheReadInputTokenCost(pricing.CacheReadInputTokenCost)
		}
		if pricing.InputCostPerTokenPriority > 0 {
			builder.SetInputCostPerTokenPriority(pricing.InputCostPerTokenPriority)
		}
		if pricing.OutputCostPerTokenPriority > 0 {
			builder.SetOutputCostPerTokenPriority(pricing.OutputCostPerTokenPriority)
		}
		if pricing.CacheReadInputTokenCostPriority > 0 {
			builder.SetCacheReadInputTokenCostPriority(pricing.CacheReadInputTokenCostPriority)
		}
		if pricing.LiteLLMProvider != "" {
			builder.SetLitellmProvider(pricing.LiteLLMProvider)
		}
		if pricing.Mode != "" {
			builder.SetMode(pricing.Mode)
		}

		if _, err := builder.Save(ctx); err != nil {
			continue
		}
		loaded++
	}

	log.Printf("[ModelPricingAdmin] Loaded %d models from fallback file", loaded)
	return nil
}

func dbModelToLiteLLM(m *ent.ModelPricing) *LiteLLMModelPricing {
	p := &LiteLLMModelPricing{
		InputCostPerToken:     m.InputCostPerToken,
		OutputCostPerToken:    m.OutputCostPerToken,
		SupportsServiceTier:   m.SupportsServiceTier,
		LiteLLMProvider:       m.LitellmProvider,
		Mode:                  m.Mode,
		SupportsPromptCaching: m.SupportsPromptCaching,
	}
	if m.CacheCreationInputTokenCost != nil {
		p.CacheCreationInputTokenCost = *m.CacheCreationInputTokenCost
	}
	if m.CacheCreationInputTokenCostAbove1hr != nil {
		p.CacheCreationInputTokenCostAbove1hr = *m.CacheCreationInputTokenCostAbove1hr
	}
	if m.CacheReadInputTokenCost != nil {
		p.CacheReadInputTokenCost = *m.CacheReadInputTokenCost
	}
	if m.InputCostPerTokenPriority != nil {
		p.InputCostPerTokenPriority = *m.InputCostPerTokenPriority
	}
	if m.OutputCostPerTokenPriority != nil {
		p.OutputCostPerTokenPriority = *m.OutputCostPerTokenPriority
	}
	if m.CacheReadInputTokenCostPriority != nil {
		p.CacheReadInputTokenCostPriority = *m.CacheReadInputTokenCostPriority
	}
	if m.OutputCostPerImage != nil {
		p.OutputCostPerImage = *m.OutputCostPerImage
	}
	if m.OutputCostPerImageToken != nil {
		p.OutputCostPerImageToken = *m.OutputCostPerImageToken
	}
	if m.LongContextInputTokenThreshold != nil {
		p.LongContextInputTokenThreshold = *m.LongContextInputTokenThreshold
	}
	if m.LongContextInputCostMultiplier != nil {
		p.LongContextInputCostMultiplier = *m.LongContextInputCostMultiplier
	}
	if m.LongContextOutputCostMultiplier != nil {
		p.LongContextOutputCostMultiplier = *m.LongContextOutputCostMultiplier
	}
	return p
}

func dbModelToEntry(m *ent.ModelPricing) ModelPricingEntry {
	return ModelPricingEntry{
		ID:                                  m.ID,
		Model:                               m.Model,
		InputCostPerToken:                   m.InputCostPerToken,
		OutputCostPerToken:                  m.OutputCostPerToken,
		CacheCreationInputTokenCost:         m.CacheCreationInputTokenCost,
		CacheCreationInputTokenCostAbove1hr: m.CacheCreationInputTokenCostAbove1hr,
		CacheReadInputTokenCost:             m.CacheReadInputTokenCost,
		InputCostPerTokenPriority:           m.InputCostPerTokenPriority,
		OutputCostPerTokenPriority:          m.OutputCostPerTokenPriority,
		CacheReadInputTokenCostPriority:     m.CacheReadInputTokenCostPriority,
		OutputCostPerImage:                  m.OutputCostPerImage,
		OutputCostPerImageToken:             m.OutputCostPerImageToken,
		LongContextInputTokenThreshold:      m.LongContextInputTokenThreshold,
		LongContextInputCostMultiplier:      m.LongContextInputCostMultiplier,
		LongContextOutputCostMultiplier:     m.LongContextOutputCostMultiplier,
		SupportsServiceTier:                 m.SupportsServiceTier,
		LitellmProvider:                     m.LitellmProvider,
		Mode:                                m.Mode,
		SupportsPromptCaching:               m.SupportsPromptCaching,
		Locked:                              m.Locked,
		Source:                              m.Source,
		CreatedAt:                           m.CreatedAt,
		UpdatedAt:                           m.UpdatedAt,
	}
}

func loadFallbackData(filePath string) ([]byte, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read fallback file %s: %w", filePath, err)
	}
	return data, nil
}

func rawEntryToLiteLLM(entry LiteLLMRawEntry) LiteLLMModelPricing {
	p := LiteLLMModelPricing{}
	if entry.InputCostPerToken != nil {
		p.InputCostPerToken = *entry.InputCostPerToken
	}
	if entry.OutputCostPerToken != nil {
		p.OutputCostPerToken = *entry.OutputCostPerToken
	}
	if entry.InputCostPerTokenPriority != nil {
		p.InputCostPerTokenPriority = *entry.InputCostPerTokenPriority
	}
	if entry.OutputCostPerTokenPriority != nil {
		p.OutputCostPerTokenPriority = *entry.OutputCostPerTokenPriority
	}
	if entry.CacheCreationInputTokenCost != nil {
		p.CacheCreationInputTokenCost = *entry.CacheCreationInputTokenCost
	}
	if entry.CacheCreationInputTokenCostAbove1hr != nil {
		p.CacheCreationInputTokenCostAbove1hr = *entry.CacheCreationInputTokenCostAbove1hr
	}
	if entry.CacheReadInputTokenCost != nil {
		p.CacheReadInputTokenCost = *entry.CacheReadInputTokenCost
	}
	if entry.CacheReadInputTokenCostPriority != nil {
		p.CacheReadInputTokenCostPriority = *entry.CacheReadInputTokenCostPriority
	}
	if entry.OutputCostPerImage != nil {
		p.OutputCostPerImage = *entry.OutputCostPerImage
	}
	if entry.OutputCostPerImageToken != nil {
		p.OutputCostPerImageToken = *entry.OutputCostPerImageToken
	}
	p.SupportsServiceTier = entry.SupportsServiceTier
	p.LiteLLMProvider = entry.LiteLLMProvider
	p.Mode = entry.Mode
	p.SupportsPromptCaching = entry.SupportsPromptCaching
	return p
}

type PublicPricingGroup struct {
	ID             int64                `json:"id"`
	Name           string               `json:"name"`
	Platform       string               `json:"platform"`
	RateMultiplier float64              `json:"rate_multiplier"`
	Models         []PublicPricingModel `json:"models"`
}

type PublicPricingModel struct {
	ModelName            string  `json:"model_name"`
	InputCostPerMillion  float64 `json:"input_cost_per_million"`
	OutputCostPerMillion float64 `json:"output_cost_per_million"`
	EffectiveInput       float64 `json:"effective_input"`
	EffectiveOutput      float64 `json:"effective_output"`
	RequestCount         int     `json:"request_count"`
}

func (s *ModelPricingAdminService) GetGroupsWithModelsAndPricing(ctx context.Context) ([]PublicPricingGroup, error) {
	query := `
		WITH group_models AS (
			SELECT DISTINCT
				ag.group_id,
				COALESCE(
					a.credentials->'model_mapping'->>mk.key,
					mk.key
				) AS model_name
			FROM account_groups ag
			JOIN accounts a ON a.id = ag.account_id
			CROSS JOIN LATERAL jsonb_object_keys(
				CASE
					WHEN a.credentials->'model_mapping' IS NOT NULL
					THEN a.credentials->'model_mapping'
					ELSE '{}'::jsonb
				END
			) AS mk(key)
			WHERE a.deleted_at IS NULL
			  AND mk.key NOT LIKE '%*%'
		)
		SELECT
			g.id,
			COALESCE(g.name, '') AS name,
			COALESCE(g.platform, '') AS platform,
			g.rate_multiplier,
			COALESCE(
				json_agg(gm.model_name ORDER BY gm.model_name),
				'[]'::json
			) AS models
		FROM groups g
		LEFT JOIN group_models gm ON gm.group_id = g.id
		WHERE g.status = 'active'
		  AND g.deleted_at IS NULL
		GROUP BY g.id, g.name, g.platform, g.rate_multiplier
		ORDER BY g.sort_order, g.name`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query groups with models: %w", err)
	}
	defer rows.Close()

	groupMap := make(map[int64]*PublicPricingGroup)
	var order []int64

	for rows.Next() {
		var g PublicPricingGroup
		var modelsJSON string
		if err := rows.Scan(&g.ID, &g.Name, &g.Platform, &g.RateMultiplier, &modelsJSON); err != nil {
			return nil, fmt.Errorf("scan group row: %w", err)
		}

		var modelNames []string
		if err := json.Unmarshal([]byte(modelsJSON), &modelNames); err != nil {
			g.Models = []PublicPricingModel{}
		} else {
			g.Models = make([]PublicPricingModel, len(modelNames))
			for idx, name := range modelNames {
				g.Models[idx].ModelName = name
			}
		}

		for i := range g.Models {
			modelName := g.Models[i].ModelName
			basePricing := s.GetPricingFromCache(modelName)
			if basePricing == nil && s.pricingService != nil {
				basePricing = s.pricingService.GetModelPricing(modelName)
			}
			if basePricing != nil {
				if basePricing.InputCostPerToken > 0 {
					g.Models[i].InputCostPerMillion = basePricing.InputCostPerToken * 1000000
					g.Models[i].EffectiveInput = g.Models[i].InputCostPerMillion * g.RateMultiplier
				}
				if basePricing.OutputCostPerToken > 0 {
					g.Models[i].OutputCostPerMillion = basePricing.OutputCostPerToken * 1000000
					g.Models[i].EffectiveOutput = g.Models[i].OutputCostPerMillion * g.RateMultiplier
				} else if basePricing.OutputCostPerImage > 0 {
					g.Models[i].OutputCostPerMillion = basePricing.OutputCostPerImage
					g.Models[i].EffectiveOutput = g.Models[i].OutputCostPerMillion * g.RateMultiplier
				} else if basePricing.OutputCostPerImageToken > 0 {
					g.Models[i].OutputCostPerMillion = basePricing.OutputCostPerImageToken * 1000000
					g.Models[i].EffectiveOutput = g.Models[i].OutputCostPerMillion * g.RateMultiplier
				}
			}
			if s.channelService == nil {
				continue
			}
			channelPricing := s.channelService.GetChannelModelPricing(ctx, g.ID, modelName)
			if channelPricing == nil {
				continue
			}
			switch channelPricing.BillingMode {
			case BillingModePerRequest, BillingModeImage:
				price := 0.0
				if channelPricing.PerRequestPrice != nil {
					price = *channelPricing.PerRequestPrice
				}
				if price <= 0 {
					if iv := channelPricing.GetIntervalForContext(0); iv != nil && iv.PerRequestPrice != nil {
						price = *iv.PerRequestPrice
					}
				}
				g.Models[i].InputCostPerMillion = 0
				g.Models[i].EffectiveInput = 0
				g.Models[i].OutputCostPerMillion = price
				g.Models[i].EffectiveOutput = price * g.RateMultiplier
			default:
				if channelPricing.InputPrice != nil {
					g.Models[i].InputCostPerMillion = *channelPricing.InputPrice * 1000000
					g.Models[i].EffectiveInput = g.Models[i].InputCostPerMillion * g.RateMultiplier
				}
				if channelPricing.OutputPrice != nil {
					g.Models[i].OutputCostPerMillion = *channelPricing.OutputPrice * 1000000
					g.Models[i].EffectiveOutput = g.Models[i].OutputCostPerMillion * g.RateMultiplier
				} else if channelPricing.ImageOutputPrice != nil {
					g.Models[i].OutputCostPerMillion = *channelPricing.ImageOutputPrice
					g.Models[i].EffectiveOutput = g.Models[i].OutputCostPerMillion * g.RateMultiplier
				}
			}
		}
		groupMap[g.ID] = &g
		order = append(order, g.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	result := make([]PublicPricingGroup, len(order))
	for i, id := range order {
		result[i] = *groupMap[id]
	}
	return result, nil
}
