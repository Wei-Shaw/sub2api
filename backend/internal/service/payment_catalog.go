package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const SettingCatalogProducts = "PAYMENT_CATALOG_PRODUCTS"

const (
	CatalogProductTypeTopup        = "topup"
	CatalogProductTypeSubscription = "subscription"
)

type CatalogProduct struct {
	Slug        string   `json:"slug"`
	Title       string   `json:"title"`
	Category    string   `json:"category"`
	Summary     string   `json:"summary"`
	Description string   `json:"description"`
	Image       string   `json:"image"`
	CardImage   string   `json:"cardImage,omitempty"`
	Tags        []string `json:"tags"`
	PriceLabel  string   `json:"priceLabel"`
	Currency    string   `json:"currency,omitempty"`
	Badge       string   `json:"badge,omitempty"`
	Active      bool     `json:"active"`
	SortOrder   int      `json:"sortOrder"`
	ProductType string   `json:"productType"`
	Amount      float64  `json:"amount,omitempty"`
	PlanID      string   `json:"planId,omitempty"`
	CTAText     string   `json:"ctaText,omitempty"`
}

var defaultCatalogProducts = []CatalogProduct{
	{Slug: "an-liang-ti-yan-5-mei-jin", Title: "余额充值|体验|5美金额度", Category: "余额充值", Summary: "每个人只能兑换一次，多买无用", Description: "适合首次体验。购买后可直接用于模型调用。", Image: "https://shop.xuedingtoken.com/uploads/product/2026/04/33550c5e-188a-4e16-a669-0cc78b84d48a.webp", Tags: []string{"体验"}, PriceLabel: "1.00", Currency: "CNY", Active: true, SortOrder: 10, ProductType: CatalogProductTypeTopup, Amount: 5, CTAText: "立即购买"},
	{Slug: "mei-ri-1-5-mei-jin-xiao-hao-yue-ka", Title: "入门版 | 15美金额度/天 | 月卡", Category: "个人月卡", Summary: "≈ 0.44 ¥/USD\nopus | sonnet | haiku", Description: "适合个人轻量长期使用，按月订阅。", Image: "https://shop.xuedingtoken.com/uploads/product/2026/04/21c272a0-492d-45f4-bb06-934303098c6f.webp", Tags: []string{"体验"}, PriceLabel: "199.00", Currency: "CNY", Active: true, SortOrder: 20, ProductType: CatalogProductTypeSubscription, CTAText: "开通月卡"},
	{Slug: "mei-ri-3-0-mei-jin-xiao-hao-yue-ka", Title: "轻量版 | 30美金额度/天 | 月卡", Category: "个人月卡", Summary: "≈ 0.38 ¥/USD\n省 15%\nopus | sonnet | haiku", Description: "更高日额度，适合稳定使用。", Image: "https://shop.xuedingtoken.com/uploads/product/2026/04/7ec9bfd1-57e4-40c7-adbf-4f81fb3bf117.webp", Tags: []string{"轻量"}, PriceLabel: "339.00", Currency: "CNY", Active: true, SortOrder: 30, ProductType: CatalogProductTypeSubscription, CTAText: "开通月卡"},
	{Slug: "mei-ri-5-0-mei-jin-xiao-hao-yue-ka", Title: "标准版 ⭐ | 50美金额度/天 | 月卡", Category: "个人月卡", Summary: "≈ 0.33 ¥/USD\n省 25%\nopus | sonnet | haiku", Description: "最适合大多数用户的标准套餐。", Image: "https://shop.xuedingtoken.com/uploads/product/2026/04/faa83df5-e1eb-42b0-8462-420c54c663ab.webp", Tags: []string{"推荐⭐"}, PriceLabel: "499.00", Currency: "CNY", Active: true, SortOrder: 40, ProductType: CatalogProductTypeSubscription, CTAText: "开通月卡"},
	{Slug: "gao-ji-ban-1-2-0-mei-jin-tian-yue-ka", Title: "高级版👑 | 120美金额度/天 | 月卡", Category: "个人月卡", Summary: "≈ 0.33 ¥/USD\n省 25%\nopus | sonnet | haiku", Description: "高强度用户推荐，日额度更大。", Image: "https://shop.xuedingtoken.com/uploads/product/2026/04/f695ccc2-5bfb-4d05-a508-a5c06330ae4c.webp", Tags: []string{"进阶👑"}, PriceLabel: "1188.00", Currency: "CNY", Active: true, SortOrder: 50, ProductType: CatalogProductTypeSubscription, CTAText: "开通月卡"},
	{Slug: "an-liang-fu-fei-2-0-mei-jin", Title: "余额充值|20美金额度", Category: "余额充值", Summary: "畅快使用 opus | sonnet | haiku", Description: "适合短期使用与测试。", Image: "https://shop.xuedingtoken.com/uploads/product/2026/04/82e73a36-d5ba-4e61-8d08-4ee798ec67ff.webp", Tags: []string{}, PriceLabel: "20.00", Currency: "CNY", Active: true, SortOrder: 60, ProductType: CatalogProductTypeTopup, Amount: 20, CTAText: "立即充值"},
	{Slug: "an-liang-fu-fei-5-0-mei-jin", Title: "余额充值|50美金额度", Category: "余额充值", Summary: "畅快使用 opus | sonnet | haiku", Description: "适合中轻度用户灵活充值。", Image: "https://shop.xuedingtoken.com/uploads/product/2026/04/5ea3833a-d4fd-4646-a339-db3d227ba489.webp", Tags: []string{}, PriceLabel: "50.00", Currency: "CNY", Active: true, SortOrder: 70, ProductType: CatalogProductTypeTopup, Amount: 50, CTAText: "立即充值"},
	{Slug: "an-liang-fu-fei-1-0-0-mei-jin", Title: "余额充值|100美金额度", Category: "余额充值", Summary: "畅快使用 opus | sonnet | haiku", Description: "适合稳定使用的常规充值档位。", Image: "https://shop.xuedingtoken.com/uploads/product/2026/04/3c07ae71-3585-44b5-8d87-e38de91359d2.webp", Tags: []string{}, PriceLabel: "100.00", Currency: "CNY", Active: true, SortOrder: 80, ProductType: CatalogProductTypeTopup, Amount: 100, CTAText: "立即充值"},
	{Slug: "an-liang-fu-fei-500-mei-jin", Title: "余额充值|500美金额度", Category: "余额充值", Summary: "畅快使用 opus | sonnet | haiku", Description: "适合高频模型调用和团队共享场景。", Image: "https://shop.xuedingtoken.com/uploads/product/2026/04/d3075a4f-02c8-4c7f-adb5-50d455584b0d.webp", Tags: []string{"热门"}, PriceLabel: "500.00", Currency: "CNY", Active: true, SortOrder: 90, ProductType: CatalogProductTypeTopup, Amount: 500, CTAText: "立即充值"},
	{Slug: "an-liang-fu-fei-1-0-0-0-mei-jin", Title: "余额充值|1000美金额度", Category: "余额充值", Summary: "畅快使用 opus | sonnet | haiku", Description: "大额充值，更适合高消耗场景。", Image: "https://shop.xuedingtoken.com/uploads/product/2026/04/dc535aa3-7db6-4a34-a450-e43ac3ec04c6.webp", Tags: []string{}, PriceLabel: "1000.00", Currency: "CNY", Active: true, SortOrder: 100, ProductType: CatalogProductTypeTopup, Amount: 1000, CTAText: "立即充值"},
	{Slug: "an-liang-fu-fei-5000-mei-jin", Title: "余额充值|5000美金额度", Category: "余额充值", Summary: "畅快使用 opus | sonnet | haiku", Description: "适合企业或长期大规模使用。", Image: "https://shop.xuedingtoken.com/uploads/product/2026/04/3978268a-092b-4b92-9e41-a864c8715c08.webp", Tags: []string{}, PriceLabel: "5000.00", Currency: "CNY", Active: true, SortOrder: 110, ProductType: CatalogProductTypeTopup, Amount: 5000, CTAText: "立即充值"},
	{Slug: "an-liang-fu-fei-1-0-0-0-0-mei-jin-qi-ye", Title: "余额充值|10000美金额度|企业", Category: "余额充值", Summary: "企业级大额充值", Description: "面向企业的大额账户充值。", Image: "https://shop.xuedingtoken.com/uploads/product/2026/04/86aaa189-c952-4934-a6a8-cee40baed46b.webp", Tags: []string{"企业"}, PriceLabel: "10000.00", Currency: "CNY", Active: true, SortOrder: 120, ProductType: CatalogProductTypeTopup, Amount: 10000, CTAText: "立即充值"},
}

func DefaultCatalogProducts() []CatalogProduct {
	out := make([]CatalogProduct, 0, len(defaultCatalogProducts))
	for _, item := range defaultCatalogProducts {
		out = append(out, cloneCatalogProduct(item))
	}
	return out
}

func (s *PaymentConfigService) GetCatalogProducts(ctx context.Context, includeInactive bool) ([]CatalogProduct, error) {
	raw, err := s.settingRepo.GetValue(ctx, SettingCatalogProducts)
	if err != nil {
		if !errors.Is(err, ErrSettingNotFound) {
			return nil, fmt.Errorf("get catalog products: %w", err)
		}
		raw = ""
	}
	if strings.TrimSpace(raw) == "" {
		defaults := DefaultCatalogProducts()
		if err := s.SetCatalogProducts(ctx, defaults); err != nil {
			return nil, err
		}
		if err := s.refreshCatalogProductPricing(ctx, defaults); err != nil {
			return nil, err
		}
		return filterCatalogProducts(defaults, includeInactive), nil
	}

	var products []CatalogProduct
	if err := json.Unmarshal([]byte(raw), &products); err != nil {
		defaults := DefaultCatalogProducts()
		if setErr := s.SetCatalogProducts(ctx, defaults); setErr != nil {
			return nil, setErr
		}
		return filterCatalogProducts(defaults, includeInactive), nil
	}
	normalized, err := normalizeCatalogProducts(products)
	if err != nil {
		return nil, err
	}
	if err := s.refreshCatalogProductPricing(ctx, normalized); err != nil {
		return nil, err
	}
	return filterCatalogProducts(normalized, includeInactive), nil
}

func (s *PaymentConfigService) SetCatalogProducts(ctx context.Context, products []CatalogProduct) error {
	normalized, err := normalizeCatalogProducts(products)
	if err != nil {
		return err
	}
	if err := s.refreshCatalogProductPricing(ctx, normalized); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal catalog products: %w", err)
	}
	return s.settingRepo.Set(ctx, SettingCatalogProducts, string(payload))
}

func (s *PaymentConfigService) refreshCatalogProductPricing(ctx context.Context, products []CatalogProduct) error {
	cfg, err := s.GetPaymentConfig(ctx)
	if err != nil {
		return fmt.Errorf("get payment config for catalog products: %w", err)
	}
	applyCatalogProductPricing(products, cfg)
	return nil
}

func normalizeCatalogProducts(products []CatalogProduct) ([]CatalogProduct, error) {
	out := make([]CatalogProduct, 0, len(products))
	for i, item := range products {
		product, err := normalizeCatalogProduct(item, i)
		if err != nil {
			return nil, err
		}
		out = append(out, product)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].SortOrder < out[j].SortOrder })
	return out, nil
}

func normalizeCatalogProduct(input CatalogProduct, index int) (CatalogProduct, error) {
	slug := strings.TrimSpace(input.Slug)
	title := strings.TrimSpace(input.Title)
	if slug == "" {
		return CatalogProduct{}, fmt.Errorf("products[%d].slug is required", index)
	}
	if title == "" {
		return CatalogProduct{}, fmt.Errorf("products[%d].title is required", index)
	}

	productType := strings.TrimSpace(input.ProductType)
	if productType != CatalogProductTypeSubscription {
		productType = CatalogProductTypeTopup
	}
	sortOrder := input.SortOrder
	if sortOrder == 0 {
		sortOrder = (index + 1) * 10
	}

	return CatalogProduct{
		Slug:        slug,
		Title:       title,
		Category:    defaultString(strings.TrimSpace(input.Category), "Uncategorized"),
		Summary:     strings.TrimSpace(input.Summary),
		Description: strings.TrimSpace(input.Description),
		Image:       strings.TrimSpace(input.Image),
		CardImage:   strings.TrimSpace(input.CardImage),
		Tags:        normalizeStringList(input.Tags),
		PriceLabel:  strings.TrimSpace(input.PriceLabel),
		Currency:    defaultString(strings.TrimSpace(input.Currency), "CNY"),
		Badge:       strings.TrimSpace(input.Badge),
		Active:      input.Active,
		SortOrder:   sortOrder,
		ProductType: productType,
		Amount:      normalizeCatalogAmount(productType, input.Amount),
		PlanID:      normalizeCatalogPlanID(productType, input.PlanID),
		CTAText:     strings.TrimSpace(input.CTAText),
	}, nil
}

func filterCatalogProducts(products []CatalogProduct, includeInactive bool) []CatalogProduct {
	out := make([]CatalogProduct, 0, len(products))
	for _, item := range products {
		if includeInactive || item.Active {
			out = append(out, cloneCatalogProduct(item))
		}
	}
	return out
}

func cloneCatalogProduct(item CatalogProduct) CatalogProduct {
	item.Tags = append([]string(nil), item.Tags...)
	return item
}

func normalizeStringList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func normalizeCatalogAmount(productType string, amount float64) float64 {
	if productType != CatalogProductTypeTopup || amount <= 0 {
		return 0
	}
	return amount
}

func applyCatalogProductPricing(products []CatalogProduct, cfg *PaymentConfig) {
	for i := range products {
		if products[i].ProductType != CatalogProductTypeTopup {
			products[i].Amount = 0
			continue
		}
		payAmount, ok := parseCatalogPriceLabelAmount(products[i].PriceLabel)
		if !ok {
			products[i].Amount = 0
			continue
		}
		products[i].Amount = cfg.ResolveBalancePricingTier(payAmount).CreditedAmount
	}
}

func parseCatalogPriceLabelAmount(priceLabel string) (float64, bool) {
	cleaned := strings.TrimSpace(priceLabel)
	cleaned = strings.NewReplacer(",", "", "¥", "", "￥", "", "$", "").Replace(cleaned)
	if fields := strings.Fields(cleaned); len(fields) > 0 {
		cleaned = fields[0]
	}
	amount, err := strconv.ParseFloat(cleaned, 64)
	if err != nil || amount <= 0 {
		return 0, false
	}
	return amount, true
}

func normalizeCatalogPlanID(productType, planID string) string {
	if productType != CatalogProductTypeSubscription {
		return ""
	}
	return strings.TrimSpace(planID)
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
