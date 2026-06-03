package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
)

// =====================================================================
// Pricing Plaza — anonymous public pricing/plan listing service.
//
// 该服务对外暴露两个公开端点的数据来源：
//   - GET /api/v1/plaza/models  → ListModelRows
//   - GET /api/v1/plaza/plans   → ListPlanCards
//
// 设计要点参见 openspec/changes/add-pricing-plaza/{proposal,design,spec}.md。
// =====================================================================

// ---------- Public DTOs (service layer) ----------

// PlazaCurrencyMeta 货币换算元信息，前端基于此实现 CNY ⇄ USD 切换。
//
// BalanceRechargeMultiplier 取自支付配置：
//
//	1 CNY 充值 = multiplier USD 入账。
//	即 USD = CNY × multiplier，CNY = USD ÷ multiplier。
type PlazaCurrencyMeta struct {
	BalanceRechargeMultiplier float64 `json:"balance_recharge_multiplier"`
	ModelNative               string  `json:"model_native"` // 始终 "USD"
	PlanNative                string  `json:"plan_native"`  // 始终 "CNY"
}

// PlazaImagePrices 图片生成模型的三档单价（USD per image）。
type PlazaImagePrices struct {
	Tier1K float64 `json:"tier_1k"`
	Tier2K float64 `json:"tier_2k"`
	Tier4K float64 `json:"tier_4k"`
}

// PlazaModelRow 单条模型行。
//
// 字段语义：
//   - Type: "token" | "image"
//   - InputPricePerMTok / OutputPricePerMTok: token 行的基础单价（USD per 1M tokens）
//   - SiteInputPricePerMTok / SiteOutputPricePerMTok: token 行的站点单价（已乘以倍率）
//   - BaseImagePrices: image 行的基础三档单价（USD per image）
//   - SiteImagePrices: image 行的站点三档单价（基础 × 倍率）
//   - Multiplier: 实际生效的费率倍数（image 行可能为 image_rate_multiplier）
//   - DiscountPercent: (1 - 1/Multiplier) × 100，仅当 Multiplier ≠ 1 时有意义
type PlazaModelRow struct {
	GroupID   int64  `json:"group_id"`
	GroupName string `json:"group_name"`
	Platform  string `json:"platform"`
	Model     string `json:"model"`
	Type      string `json:"type"`

	// token-only 字段（image 行为零值）
	InputPricePerMTok      float64 `json:"input_price_per_mtok,omitempty"`
	OutputPricePerMTok     float64 `json:"output_price_per_mtok,omitempty"`
	SiteInputPricePerMTok  float64 `json:"site_input_price_per_mtok,omitempty"`
	SiteOutputPricePerMTok float64 `json:"site_output_price_per_mtok,omitempty"`

	// token-only 缓存字段（5m 单档）。指针语义：
	//   - nil → JSON 中省略，前端渲染 "—"（"未知 / 不适用"）
	//   - &0  → JSON 输出 0，前端渲染 "$0"（仅当 SupportsCacheBreakdown == true 时显式置 0）
	//   - &v  → JSON 输出 v
	// 当源 ModelPricing 的对应缓存价格为 0 且 SupportsCacheBreakdown == false 时，
	// 字段保持 nil；SupportsCacheBreakdown == true 时即使为 0 也指向 0。
	CacheWritePricePerMTok     *float64 `json:"cache_write_price_per_mtok,omitempty"`
	CacheReadPricePerMTok      *float64 `json:"cache_read_price_per_mtok,omitempty"`
	SiteCacheWritePricePerMTok *float64 `json:"site_cache_write_price_per_mtok,omitempty"`
	SiteCacheReadPricePerMTok  *float64 `json:"site_cache_read_price_per_mtok,omitempty"`

	// image-only 字段（token 行为 nil）
	BaseImagePrices *PlazaImagePrices `json:"base_image_prices,omitempty"`
	SiteImagePrices *PlazaImagePrices `json:"site_image_prices,omitempty"`

	Multiplier      float64 `json:"multiplier"`
	DiscountPercent float64 `json:"discount_percent"`
}

// PlazaModelFilter 列模型时的过滤参数（全部可选）。
type PlazaModelFilter struct {
	GroupID  int64  // 0 = 不过滤
	Platform string // "" = 不过滤
	Q        string // 模型名子串，大小写不敏感
}

// PlazaPlanCard 套餐卡片（CNY 原价透传）。
type PlazaPlanCard struct {
	ID             int64    `json:"id"`
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Price          float64  `json:"price"`
	OriginalPrice  *float64 `json:"original_price,omitempty"`
	ValidityDays   int      `json:"validity_days"`
	ValidityUnit   string   `json:"validity_unit"`
	Features       string   `json:"features,omitempty"`
	GroupID        int64    `json:"group_id"`
	GroupName      string   `json:"group_name"`
	Platform       string   `json:"platform"`
	Models         []string `json:"models"`
	ModelsOverflow int      `json:"models_overflow"` // 超出 Models 截断（cap=50）的剩余数量
}

const (
	plazaPlanModelsCap = 50
	plazaCacheTTL      = 60 * time.Second
)

// ---------- Service ----------

// PlazaPaymentConfigSource 收窄了 PlazaService 对支付配置子系统的依赖，
// 仅暴露广场所需的两个只读方法；*PaymentConfigService 自动实现该接口。
// 引入它的目的是为单测提供轻量替身。
type PlazaPaymentConfigSource interface {
	GetPaymentConfig(ctx context.Context) (*PaymentConfig, error)
	ListPlansForSale(ctx context.Context) ([]*dbent.SubscriptionPlan, error)
}

// PlazaAccountSource 收窄了 PlazaService 对账号仓储的依赖。
// 广场只需要"列出所有 active 账号"的能力，*accountRepository 自动满足该接口。
// 引入它是为了避免在单测里实现完整的 AccountRepository（50+ 方法）。
type PlazaAccountSource interface {
	ListActive(ctx context.Context) ([]Account, error)
}

// PlazaService 聚合 model/plan 公开展示数据；无状态，依赖只读 repo / 服务。
type PlazaService struct {
	pricingService       *PricingService
	billingService       *BillingService
	groupRepo            GroupRepository
	accountRepo          PlazaAccountSource
	paymentConfigService PlazaPaymentConfigSource

	mu        sync.Mutex
	modelHits map[string]plazaCacheEntry[*plazaModelsResult]
	planHit   plazaCacheEntry[*plazaPlansResult]
}

type plazaCacheEntry[T any] struct {
	at   time.Time
	data T
}

type plazaModelsResult struct {
	Rows         []PlazaModelRow
	CurrencyMeta PlazaCurrencyMeta
}

type plazaPlansResult struct {
	Cards        []PlazaPlanCard
	CurrencyMeta PlazaCurrencyMeta
}

// NewPlazaService constructs a PlazaService.
func NewPlazaService(
	pricingService *PricingService,
	billingService *BillingService,
	groupRepo GroupRepository,
	accountRepo PlazaAccountSource,
	paymentConfigService PlazaPaymentConfigSource,
) *PlazaService {
	return &PlazaService{
		pricingService:       pricingService,
		billingService:       billingService,
		groupRepo:            groupRepo,
		accountRepo:          accountRepo,
		paymentConfigService: paymentConfigService,
		modelHits:            make(map[string]plazaCacheEntry[*plazaModelsResult]),
	}
}

// accountSupportedModels 提取 account.Credentials["model_mapping"] 的 key 集合作为可见模型名。
//
// 规则：
//   - 通过 Account.GetModelMapping() 拿到已解析的 mapping（已处理 Antigravity 默认映射等）；
//   - 通配符 key（如 "claude-3-*"）被剔除：广场只列具体模型；
//   - 空字符串被忽略。
func accountSupportedModels(a *Account) []string {
	if a == nil {
		return nil
	}
	mapping := a.GetModelMapping()
	if len(mapping) == 0 {
		return nil
	}
	out := make([]string, 0, len(mapping))
	for src := range mapping {
		name := strings.TrimSpace(src)
		if name == "" {
			continue
		}
		if _, isWild := splitWildcardSuffix(name); isWild {
			continue
		}
		out = append(out, name)
	}
	return out
}

// ---------- ListModelRows ----------

// ListModelRows returns visible (group, model) pricing rows for the public model plaza.
func (s *PlazaService) ListModelRows(ctx context.Context, filter PlazaModelFilter) ([]PlazaModelRow, PlazaCurrencyMeta, error) {
	cacheKey := plazaModelsCacheKey(filter)
	if cached := s.readModelsCache(cacheKey); cached != nil {
		return cached.Rows, cached.CurrencyMeta, nil
	}

	currencyMeta, err := s.buildCurrencyMeta(ctx)
	if err != nil {
		return nil, PlazaCurrencyMeta{}, err
	}

	groups, err := s.eligibleGroups(ctx)
	if err != nil {
		return nil, PlazaCurrencyMeta{}, err
	}
	if len(groups) == 0 {
		s.writeModelsCache(cacheKey, &plazaModelsResult{Rows: []PlazaModelRow{}, CurrencyMeta: currencyMeta})
		return []PlazaModelRow{}, currencyMeta, nil
	}

	groupModels, err := s.buildGroupModelUnion(ctx, groups)
	if err != nil {
		return nil, PlazaCurrencyMeta{}, err
	}

	rows := make([]PlazaModelRow, 0, 64)
	for i := range groups {
		g := &groups[i]
		entries := groupModels[g.ID]
		if len(entries) == 0 {
			continue
		}
		// 按平台分隔后再排序：先 token 行，后 image 行；组内按模型名字典序。
		// 拆分两组先存，最后合并。
		tokenRows := make([]PlazaModelRow, 0, len(entries))
		imageRows := make([]PlazaModelRow, 0)
		for _, entry := range entries {
			row, ok := s.buildModelRow(g, entry)
			if !ok {
				continue
			}
			if !filter.match(row) {
				continue
			}
			if row.Type == "image" {
				imageRows = append(imageRows, row)
			} else {
				tokenRows = append(tokenRows, row)
			}
		}
		sort.SliceStable(tokenRows, func(i, j int) bool { return tokenRows[i].Model < tokenRows[j].Model })
		sort.SliceStable(imageRows, func(i, j int) bool { return imageRows[i].Model < imageRows[j].Model })
		rows = append(rows, tokenRows...)
		rows = append(rows, imageRows...)
	}

	result := &plazaModelsResult{Rows: rows, CurrencyMeta: currencyMeta}
	s.writeModelsCache(cacheKey, result)
	return result.Rows, result.CurrencyMeta, nil
}

// match 应用 PlazaModelFilter 的过滤条件。
func (f PlazaModelFilter) match(row PlazaModelRow) bool {
	if f.GroupID != 0 && row.GroupID != f.GroupID {
		return false
	}
	if f.Platform != "" && !strings.EqualFold(row.Platform, f.Platform) {
		return false
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		if !strings.Contains(strings.ToLower(row.Model), strings.ToLower(q)) {
			return false
		}
	}
	return true
}

// plazaModelEntry 表示一个分组下某个具体模型（带平台）。
type plazaModelEntry struct {
	Name     string
	Platform string
}

// eligibleGroups 返回可在公共模型广场展示的分组。
//
// 过滤条件：active && !is_exclusive && subscription_type=standard。
func (s *PlazaService) eligibleGroups(ctx context.Context) ([]Group, error) {
	groups, err := s.groupRepo.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("plaza: list active groups: %w", err)
	}
	out := make([]Group, 0, len(groups))
	for i := range groups {
		g := groups[i]
		if g.IsExclusive {
			continue
		}
		if g.SubscriptionType != SubscriptionTypeStandard {
			continue
		}
		out = append(out, g)
	}
	return out, nil
}

// buildGroupModelUnion 构建 groupID → []plazaModelEntry。
//
// 数据源是 active accounts 的 Credentials["model_mapping"] keys（剔通配符）；
// 平台直接取 account.Platform。channel 不参与广场模型集合的构建。
func (s *PlazaService) buildGroupModelUnion(ctx context.Context, groups []Group) (map[int64][]plazaModelEntry, error) {
	accounts, err := s.accountRepo.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("plaza: list active accounts: %w", err)
	}
	groupSet := make(map[int64]struct{}, len(groups))
	for i := range groups {
		groupSet[groups[i].ID] = struct{}{}
	}
	type dedupKey struct {
		platform string
		name     string
	}
	out := make(map[int64][]plazaModelEntry, len(groups))
	seen := make(map[int64]map[dedupKey]struct{}, len(groups))

	for ai := range accounts {
		a := &accounts[ai]
		if a.Status != StatusActive {
			continue
		}
		var relevant []int64
		for _, gid := range a.GroupIDs {
			if _, ok := groupSet[gid]; ok {
				relevant = append(relevant, gid)
			}
		}
		if len(relevant) == 0 {
			continue
		}
		names := accountSupportedModels(a)
		if len(names) == 0 {
			continue
		}
		for _, name := range names {
			for _, gid := range relevant {
				perGroup, ok := seen[gid]
				if !ok {
					perGroup = make(map[dedupKey]struct{})
					seen[gid] = perGroup
				}
				key := dedupKey{platform: a.Platform, name: strings.ToLower(name)}
				if _, ok := perGroup[key]; ok {
					continue
				}
				perGroup[key] = struct{}{}
				out[gid] = append(out[gid], plazaModelEntry{Name: name, Platform: a.Platform})
			}
		}
	}
	return out, nil
}

// buildModelRow 解析一条 (group, model) 的定价行。
//
// 解析链：
//  1. PricingService.GetModelPricing → 拿到 mode 和（可选）输入/输出 token 价；
//  2. 若 mode == "image_generation" → image 行，调用 BillingService.getImageUnitPrice 解三档；
//  3. 否则 → 复用 BillingService.GetModelPricing（自带 LiteLLM → fallback 链）；
//  4. 仍无则 silent drop（返回 ok=false）。
func (s *PlazaService) buildModelRow(g *Group, entry plazaModelEntry) (PlazaModelRow, bool) {
	row := PlazaModelRow{
		GroupID:   g.ID,
		GroupName: g.Name,
		Platform:  entry.Platform,
		Model:     entry.Name,
	}

	rate := g.RateMultiplier
	if rate <= 0 {
		rate = 1.0
	}

	// 先看 LiteLLM 提供的 mode（用来识别 image_generation）。
	var raw *LiteLLMModelPricing
	if s.pricingService != nil {
		raw = s.pricingService.GetModelPricing(entry.Name)
	}
	if raw != nil && strings.EqualFold(raw.Mode, "image_generation") {
		return s.buildImageRow(g, entry, rate, row), true
	}

	// token 行：通过 BillingService 自带的 LiteLLM → fallback 链。
	pricing, err := s.billingService.GetModelPricing(entry.Name)
	if err != nil || pricing == nil {
		return PlazaModelRow{}, false
	}

	const perMTok = 1_000_000.0
	row.Type = "token"
	row.InputPricePerMTok = pricing.InputPricePerToken * perMTok
	row.OutputPricePerMTok = pricing.OutputPricePerToken * perMTok
	row.SiteInputPricePerMTok = row.InputPricePerMTok * rate
	row.SiteOutputPricePerMTok = row.OutputPricePerMTok * rate

	// Cache 单档价格（5m）。policy:
	//   - SupportsCacheBreakdown == false 且源值 == 0 → 保持 nil（前端渲染 "—"）
	//   - 否则 → 显式产出 *float64（即使为 0 也会出现在 JSON 中）
	if pricing.SupportsCacheBreakdown || pricing.CacheCreation5mPrice != 0 {
		base := pricing.CacheCreation5mPrice * perMTok
		site := base * rate
		row.CacheWritePricePerMTok = &base
		row.SiteCacheWritePricePerMTok = &site
	}
	if pricing.SupportsCacheBreakdown || pricing.CacheReadPricePerToken != 0 {
		base := pricing.CacheReadPricePerToken * perMTok
		site := base * rate
		row.CacheReadPricePerMTok = &base
		row.SiteCacheReadPricePerMTok = &site
	}

	row.Multiplier = rate
	row.DiscountPercent = discountPercentFromMultiplier(rate)
	return row, true
}

// buildImageRow 解析 image_generation 模型的三档定价。
func (s *PlazaService) buildImageRow(g *Group, entry plazaModelEntry, rate float64, row PlazaModelRow) PlazaModelRow {
	cfg := &ImagePriceConfig{
		Price1K: g.ImagePrice1K,
		Price2K: g.ImagePrice2K,
		Price4K: g.ImagePrice4K,
	}
	base := PlazaImagePrices{
		Tier1K: s.billingService.getImageUnitPrice(entry.Name, "1K", cfg),
		Tier2K: s.billingService.getImageUnitPrice(entry.Name, "2K", cfg),
		Tier4K: s.billingService.getImageUnitPrice(entry.Name, "4K", cfg),
	}
	imgRate := rate
	if g.ImageRateIndependent && g.ImageRateMultiplier > 0 {
		imgRate = g.ImageRateMultiplier
	}
	site := PlazaImagePrices{
		Tier1K: base.Tier1K * imgRate,
		Tier2K: base.Tier2K * imgRate,
		Tier4K: base.Tier4K * imgRate,
	}
	row.Type = "image"
	row.BaseImagePrices = &base
	row.SiteImagePrices = &site
	row.Multiplier = imgRate
	row.DiscountPercent = discountPercentFromMultiplier(imgRate)
	return row
}

// discountPercentFromMultiplier 用倍率推算折扣百分比；rate ≤ 0 / =1 返回 0。
func discountPercentFromMultiplier(rate float64) float64 {
	if rate <= 0 || rate == 1.0 {
		return 0
	}
	return (1 - 1/rate) * 100
}

// ---------- ListPlanCards ----------

// ListPlanCards 列出 for_sale=true 且分组合规的套餐卡片。
func (s *PlazaService) ListPlanCards(ctx context.Context) ([]PlazaPlanCard, PlazaCurrencyMeta, error) {
	if cached := s.readPlansCache(); cached != nil {
		return cached.Cards, cached.CurrencyMeta, nil
	}

	currencyMeta, err := s.buildCurrencyMeta(ctx)
	if err != nil {
		return nil, PlazaCurrencyMeta{}, err
	}

	plans, err := s.paymentConfigService.ListPlansForSale(ctx)
	if err != nil {
		return nil, PlazaCurrencyMeta{}, fmt.Errorf("plaza: list plans for sale: %w", err)
	}
	if len(plans) == 0 {
		result := &plazaPlansResult{Cards: []PlazaPlanCard{}, CurrencyMeta: currencyMeta}
		s.writePlansCache(result)
		return result.Cards, result.CurrencyMeta, nil
	}

	groups, err := s.groupRepo.ListActive(ctx)
	if err != nil {
		return nil, PlazaCurrencyMeta{}, fmt.Errorf("plaza: list active groups: %w", err)
	}
	groupByID := make(map[int64]*Group, len(groups))
	for i := range groups {
		groupByID[groups[i].ID] = &groups[i]
	}

	accounts, err := s.accountRepo.ListActive(ctx)
	if err != nil {
		return nil, PlazaCurrencyMeta{}, fmt.Errorf("plaza: list active accounts: %w", err)
	}
	groupModels := buildGroupModelNamesFromAccounts(accounts, groupByID)

	cards := make([]PlazaPlanCard, 0, len(plans))
	for _, p := range plans {
		g, ok := groupByID[p.GroupID]
		if !ok || g == nil || !g.IsActive() {
			continue
		}
		card := planToCard(p, g, groupModels[p.GroupID])
		cards = append(cards, card)
	}

	result := &plazaPlansResult{Cards: cards, CurrencyMeta: currencyMeta}
	s.writePlansCache(result)
	return result.Cards, result.CurrencyMeta, nil
}

// buildGroupModelNamesFromAccounts 在 ListPlanCards 路径下复用，返回 groupID → 排序后的模型名列表。
//
// 数据源同 buildGroupModelUnion：active accounts 的 Credentials["model_mapping"] keys（已剔通配符）。
// 同分组下重复模型名按 case-insensitive 去重，首次出现的原始大小写胜出；最终按原始大小写字典序排序。
func buildGroupModelNamesFromAccounts(accounts []Account, groupByID map[int64]*Group) map[int64][]string {
	type setEntry = struct{}
	out := make(map[int64][]string, len(groupByID))
	seen := make(map[int64]map[string]setEntry, len(groupByID))
	for ai := range accounts {
		a := &accounts[ai]
		if a.Status != StatusActive {
			continue
		}
		var relevant []int64
		for _, gid := range a.GroupIDs {
			if _, ok := groupByID[gid]; ok {
				relevant = append(relevant, gid)
			}
		}
		if len(relevant) == 0 {
			continue
		}
		names := accountSupportedModels(a)
		if len(names) == 0 {
			continue
		}
		for _, name := range names {
			for _, gid := range relevant {
				perGroup, ok := seen[gid]
				if !ok {
					perGroup = make(map[string]setEntry)
					seen[gid] = perGroup
				}
				low := strings.ToLower(name)
				if _, ok := perGroup[low]; ok {
					continue
				}
				perGroup[low] = setEntry{}
				out[gid] = append(out[gid], name)
			}
		}
	}
	for gid := range out {
		sort.Strings(out[gid])
	}
	return out
}

// planToCard 把 ent 套餐映射到展示 DTO；models 截断到 plazaPlanModelsCap。
func planToCard(p *dbent.SubscriptionPlan, g *Group, models []string) PlazaPlanCard {
	card := PlazaPlanCard{
		ID:            int64(p.ID),
		Name:          p.Name,
		Description:   p.Description,
		Price:         p.Price,
		OriginalPrice: p.OriginalPrice,
		ValidityDays:  p.ValidityDays,
		ValidityUnit:  p.ValidityUnit,
		Features:      p.Features,
		GroupID:       p.GroupID,
		GroupName:     g.Name,
		Platform:      g.Platform,
	}
	if len(models) > plazaPlanModelsCap {
		card.Models = append([]string(nil), models[:plazaPlanModelsCap]...)
		card.ModelsOverflow = len(models) - plazaPlanModelsCap
	} else {
		card.Models = append([]string(nil), models...)
	}
	return card
}

// ---------- helpers ----------

func (s *PlazaService) buildCurrencyMeta(ctx context.Context) (PlazaCurrencyMeta, error) {
	pc, err := s.paymentConfigService.GetPaymentConfig(ctx)
	if err != nil {
		return PlazaCurrencyMeta{}, fmt.Errorf("plaza: load payment config: %w", err)
	}
	mult := pc.BalanceRechargeMultiplier
	if mult <= 0 {
		mult = 1
	}
	return PlazaCurrencyMeta{
		BalanceRechargeMultiplier: mult,
		ModelNative:               "USD",
		PlanNative:                "CNY",
	}, nil
}

// ---------- cache ----------

func plazaModelsCacheKey(f PlazaModelFilter) string {
	return fmt.Sprintf("g=%d|p=%s|q=%s", f.GroupID, strings.ToLower(f.Platform), strings.ToLower(strings.TrimSpace(f.Q)))
}

func (s *PlazaService) readModelsCache(key string) *plazaModelsResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.modelHits[key]
	if !ok {
		return nil
	}
	if time.Since(entry.at) > plazaCacheTTL {
		delete(s.modelHits, key)
		return nil
	}
	return entry.data
}

func (s *PlazaService) writeModelsCache(key string, data *plazaModelsResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.modelHits[key] = plazaCacheEntry[*plazaModelsResult]{at: time.Now(), data: data}
}

func (s *PlazaService) readPlansCache() *plazaPlansResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.planHit.data == nil {
		return nil
	}
	if time.Since(s.planHit.at) > plazaCacheTTL {
		s.planHit = plazaCacheEntry[*plazaPlansResult]{}
		return nil
	}
	return s.planHit.data
}

func (s *PlazaService) writePlansCache(data *plazaPlansResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.planHit = plazaCacheEntry[*plazaPlansResult]{at: time.Now(), data: data}
}

// InvalidateCache 让外部（管理操作）显式失效广场缓存。
func (s *PlazaService) InvalidateCache() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.modelHits = make(map[string]plazaCacheEntry[*plazaModelsResult])
	s.planHit = plazaCacheEntry[*plazaPlansResult]{}
}
