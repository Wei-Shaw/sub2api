package modelcatalog

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/modelcatalog/data"
)

// Price is a human-authored rate card. Amounts are USD per million tokens
// unless a field name says otherwise.
type Price struct {
	InputPerMTok                  *float64 `json:"input_per_mtok,omitempty"`
	OutputPerMTok                 *float64 `json:"output_per_mtok,omitempty"`
	InputPriorityPerMTok          *float64 `json:"input_priority_per_mtok,omitempty"`
	OutputPriorityPerMTok         *float64 `json:"output_priority_per_mtok,omitempty"`
	CacheWritePerMTok             *float64 `json:"cache_write_per_mtok,omitempty"`
	CacheWritePriorityPerMTok     *float64 `json:"cache_write_priority_per_mtok,omitempty"`
	CacheReadPerMTok              *float64 `json:"cache_read_per_mtok,omitempty"`
	CacheReadPriorityPerMTok      *float64 `json:"cache_read_priority_per_mtok,omitempty"`
	ImageInputPerMTok             *float64 `json:"image_input_per_mtok,omitempty"`
	ImageOutputPerMTok            *float64 `json:"image_output_per_mtok,omitempty"`
	PerRequest                    *float64 `json:"per_request,omitempty"`
	LongContextInputThreshold     int      `json:"long_context_input_threshold,omitempty"`
	LongContextInputMultiplier    float64  `json:"long_context_input_multiplier,omitempty"`
	LongContextOutputMultiplier   float64  `json:"long_context_output_multiplier,omitempty"`
	LongContextThresholdInclusive bool     `json:"long_context_threshold_inclusive,omitempty"`
}

// Alias is another public model ID that shares this entry's price card and
// optional upstream rewrite.
type Alias struct {
	ID        string   `json:"id"`
	Upstream  string   `json:"upstream,omitempty"`
	Platforms []string `json:"platforms,omitempty"`
}

// Model is one first-class catalog entry.
type Model struct {
	ID          string   `json:"id"`
	DisplayName string   `json:"display_name,omitempty"`
	Platforms   []string `json:"platforms,omitempty"`
	Kind        string   `json:"kind,omitempty"`
	BillingMode string   `json:"billing_mode,omitempty"`
	Upstream    string   `json:"upstream,omitempty"`
	PriceRef    string   `json:"price_ref,omitempty"`
	LockPrice   bool     `json:"lock_price,omitempty"`
	Price       *Price   `json:"price,omitempty"`
	Aliases     []Alias  `json:"aliases,omitempty"`
}

// File is the on-disk catalog document.
type File struct {
	Version int     `json:"version"`
	Models  []Model `json:"models"`
}

// Entry is a resolved public model ID.
type Entry struct {
	ID          string
	DisplayName string
	Platforms   []string
	Kind        string
	BillingMode string
	Upstream    string
	PriceRef    string
	LockPrice   bool
	Price       *Price
	CanonicalID string
}

type Catalog struct {
	entries map[string]*Entry
}

var (
	defaultOnce  sync.Once
	defaultIndex *Catalog
	defaultErr   error
)

// Load parses a catalog document.
func Load(raw []byte) (*Catalog, error) {
	var file File
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("parse model catalog: %w", err)
	}
	if file.Version != 1 {
		return nil, fmt.Errorf("unsupported model catalog version %d", file.Version)
	}
	idx := &Catalog{entries: make(map[string]*Entry, len(file.Models)*2)}
	for i, model := range file.Models {
		if err := idx.addModel(model); err != nil {
			return nil, fmt.Errorf("catalog model[%d] %q: %w", i, model.ID, err)
		}
	}
	if err := idx.resolvePriceRefs(); err != nil {
		return nil, err
	}
	return idx, nil
}

func (idx *Catalog) addModel(model Model) error {
	canonical := normalizeID(model.ID)
	if canonical == "" {
		return fmt.Errorf("id is required")
	}
	if _, exists := idx.entries[canonical]; exists {
		return fmt.Errorf("duplicate id %q", canonical)
	}
	if model.Price != nil && strings.TrimSpace(model.PriceRef) != "" {
		return fmt.Errorf("price and price_ref cannot both be set")
	}

	entry := &Entry{
		ID:          canonical,
		DisplayName: strings.TrimSpace(model.DisplayName),
		Platforms:   normalizePlatforms(model.Platforms),
		Kind:        strings.TrimSpace(model.Kind),
		BillingMode: strings.TrimSpace(model.BillingMode),
		Upstream:    normalizeID(firstNonEmpty(model.Upstream, model.ID)),
		PriceRef:    normalizeID(model.PriceRef),
		LockPrice:   model.LockPrice,
		Price:       model.Price,
		CanonicalID: canonical,
	}
	idx.entries[canonical] = entry

	for _, alias := range model.Aliases {
		aliasID := normalizeID(alias.ID)
		if aliasID == "" {
			return fmt.Errorf("alias id is required")
		}
		if _, exists := idx.entries[aliasID]; exists {
			return fmt.Errorf("duplicate alias %q", aliasID)
		}
		platforms := alias.Platforms
		if len(platforms) == 0 {
			platforms = model.Platforms
		}
		idx.entries[aliasID] = &Entry{
			ID:          aliasID,
			DisplayName: entry.DisplayName,
			Platforms:   normalizePlatforms(platforms),
			Kind:        entry.Kind,
			BillingMode: entry.BillingMode,
			Upstream:    normalizeID(firstNonEmpty(alias.Upstream, model.Upstream, alias.ID)),
			PriceRef:    firstNonEmpty(entry.PriceRef, canonical),
			LockPrice:   entry.LockPrice,
			CanonicalID: canonical,
		}
	}
	return nil
}

func (idx *Catalog) resolvePriceRefs() error {
	for id, entry := range idx.entries {
		if entry.Price != nil {
			continue
		}
		ref := entry.PriceRef
		if ref == "" {
			continue
		}
		seen := map[string]struct{}{id: {}}
		for ref != "" {
			if _, loop := seen[ref]; loop {
				return fmt.Errorf("price_ref cycle at %q", id)
			}
			seen[ref] = struct{}{}
			target, ok := idx.entries[ref]
			if !ok {
				return fmt.Errorf("price_ref %q on %q does not exist", ref, id)
			}
			if target.Price != nil {
				entry.Price = target.Price
				if !entry.LockPrice {
					entry.LockPrice = target.LockPrice
				}
				break
			}
			ref = target.PriceRef
		}
		if entry.Price == nil {
			return fmt.Errorf("price_ref %q on %q has no price card", entry.PriceRef, id)
		}
	}
	return nil
}

// Default returns the embedded fork catalog.
func Default() *Catalog {
	defaultOnce.Do(func() {
		defaultIndex, defaultErr = Load(data.CatalogJSON)
	})
	if defaultErr != nil {
		panic(defaultErr)
	}
	return defaultIndex
}

// Lookup returns the catalog entry for a public or alias model ID.
func Lookup(modelID string) *Entry {
	return Default().Lookup(modelID)
}

// Lookup returns the catalog entry for a public or alias model ID.
func (idx *Catalog) Lookup(modelID string) *Entry {
	if idx == nil {
		return nil
	}
	return idx.entries[normalizeID(modelID)]
}

// PriceCardID returns the canonical price-card ID for a model.
func PriceCardID(modelID string) string {
	return Default().PriceCardID(modelID)
}

// PriceCardID returns the canonical price-card ID for a model.
func (idx *Catalog) PriceCardID(modelID string) string {
	entry := idx.Lookup(modelID)
	if entry == nil {
		return ""
	}
	if entry.PriceRef != "" {
		return entry.PriceRef
	}
	return entry.CanonicalID
}

var thinkingTierSuffixes = []string{"-high", "-low", "-medium", "-tiered"}

// SharedRateCardID returns the base model ID only when this name is a
// thinking-tier variant that should share the base published rate card.
// Commercial aliases such as gpt-5.5 or codex-auto-review must not remap.
func SharedRateCardID(modelID string) string {
	return Default().SharedRateCardID(modelID)
}

// SharedRateCardID returns the base model ID only when this name is a
// thinking-tier variant that should share the base published rate card.
func (idx *Catalog) SharedRateCardID(modelID string) string {
	entry := idx.Lookup(modelID)
	if entry == nil {
		return ""
	}
	if entry.ID == entry.CanonicalID {
		return ""
	}
	for _, suffix := range thinkingTierSuffixes {
		if entry.ID == entry.CanonicalID+suffix {
			return entry.CanonicalID
		}
	}
	return ""
}

// IsCanonical reports whether this entry is the primary catalog row.
func (e *Entry) IsCanonical() bool {
	return e != nil && e.ID == e.CanonicalID
}

// StorefrontItem is one checkbox on the channel shelf.
type StorefrontItem struct {
	ID          string
	DisplayName string
	Platforms   []string
	CanonicalID string
	Price       *Price
}

// StorefrontItems returns public catalog IDs that can be sold on a platform.
func StorefrontItems(platform string) []StorefrontItem {
	return Default().StorefrontItems(platform)
}

// StorefrontItems returns public catalog IDs that can be sold on a platform.
func (idx *Catalog) StorefrontItems(platform string) []StorefrontItem {
	if idx == nil {
		return nil
	}
	platform = strings.ToLower(strings.TrimSpace(platform))
	out := make([]StorefrontItem, 0)
	for _, entry := range idx.entries {
		if entry == nil || entry.Price == nil {
			continue
		}
		if platform != "" && !entryHasPlatform(entry, platform) {
			continue
		}
		out = append(out, StorefrontItem{
			ID:          entry.ID,
			DisplayName: firstNonEmpty(entry.DisplayName, entry.ID),
			Platforms:   append([]string(nil), entry.Platforms...),
			CanonicalID: entry.CanonicalID,
			Price:       entry.Price,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func entryHasPlatform(entry *Entry, platform string) bool {
	if entry == nil {
		return false
	}
	if len(entry.Platforms) == 0 {
		return platform == ""
	}
	for _, item := range entry.Platforms {
		if item == platform {
			return true
		}
	}
	return false
}

// PublicIDs returns every public catalog ID for a platform, including aliases.
func PublicIDs(platform string) []string {
	return Default().PublicIDs(platform)
}

// PublicIDs returns every public catalog ID for a platform, including aliases.
func (idx *Catalog) PublicIDs(platform string) []string {
	if idx == nil {
		return nil
	}
	platform = strings.ToLower(strings.TrimSpace(platform))
	out := make([]string, 0)
	for _, entry := range idx.entries {
		if entry == nil || strings.TrimSpace(entry.ID) == "" {
			continue
		}
		if platform != "" && !entryHasPlatform(entry, platform) {
			continue
		}
		out = append(out, entry.ID)
	}
	sort.Strings(out)
	return out
}

// DefaultMappings returns catalog upstream rewrites for a platform.
// Existing keys in an account mapping should win over these defaults.
func DefaultMappings(platform string) map[string]string {
	return Default().DefaultMappings(platform)
}

// DefaultMappings returns catalog upstream rewrites for a platform.
func (idx *Catalog) DefaultMappings(platform string) map[string]string {
	if idx == nil {
		return nil
	}
	platform = strings.ToLower(strings.TrimSpace(platform))
	out := make(map[string]string)
	for _, entry := range idx.entries {
		if entry == nil {
			continue
		}
		if platform != "" && !entryHasPlatform(entry, platform) {
			continue
		}
		upstream := strings.TrimSpace(entry.Upstream)
		if upstream == "" || upstream == entry.ID {
			continue
		}
		out[entry.ID] = upstream
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// Locked reports whether this model uses a human-owned price card.
func Locked(modelID string) bool {
	return Default().Locked(modelID)
}

// Locked reports whether this model uses a human-owned price card.
func (idx *Catalog) Locked(modelID string) bool {
	entry := idx.Lookup(modelID)
	return entry != nil && entry.LockPrice && entry.Price != nil
}

// Entries returns all indexed public IDs including aliases.
func (idx *Catalog) Entries() []*Entry {
	if idx == nil {
		return nil
	}
	out := make([]*Entry, 0, len(idx.entries))
	for _, entry := range idx.entries {
		out = append(out, entry)
	}
	return out
}

func normalizeID(modelID string) string {
	modelID = strings.ToLower(strings.TrimSpace(modelID))
	modelID = strings.TrimPrefix(modelID, "models/")
	return modelID
}

func normalizePlatforms(platforms []string) []string {
	if len(platforms) == 0 {
		return nil
	}
	out := make([]string, 0, len(platforms))
	seen := make(map[string]struct{}, len(platforms))
	for _, platform := range platforms {
		platform = strings.ToLower(strings.TrimSpace(platform))
		if platform == "" {
			continue
		}
		if _, ok := seen[platform]; ok {
			continue
		}
		seen[platform] = struct{}{}
		out = append(out, platform)
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// PerToken converts a per-million-token amount to a per-token amount.
func PerToken(perMTok float64) float64 {
	return perMTok * 1e-6
}

// DerefPerToken converts an optional per-million amount to per-token.
func DerefPerToken(perMTok *float64) float64 {
	if perMTok == nil {
		return 0
	}
	return PerToken(*perMTok)
}
