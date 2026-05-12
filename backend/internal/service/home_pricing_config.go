package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionplan"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	SettingKeyHomePricingConfig = "home_pricing_config"

	HomePricingCTAModeExternal = "external"
	HomePricingCTAModeInternal = "internal"

	defaultHomePricingPurchaseURL = "https://pay.ldxp.cn/shop/E8WHWMVD"
)

type HomePricingLocalizedText struct {
	Zh string `json:"zh"`
	En string `json:"en"`
}

type HomePricingMetricConfig struct {
	Label HomePricingLocalizedText `json:"label"`
	Value HomePricingLocalizedText `json:"value"`
}

type HomePricingSubscriptionCardConfig struct {
	ID                 string                    `json:"id"`
	Enabled            bool                      `json:"enabled"`
	SortOrder          int                       `json:"sort_order"`
	SubscriptionPlanID int64                     `json:"subscription_plan_id"`
	Name               HomePricingLocalizedText  `json:"name"`
	Description        HomePricingLocalizedText  `json:"description"`
	Badge              HomePricingLocalizedText  `json:"badge"`
	Period             HomePricingLocalizedText  `json:"period"`
	Highlight          bool                      `json:"highlight"`
	Metrics            []HomePricingMetricConfig `json:"metrics"`
}

type HomePricingCreditCardConfig struct {
	ID             string                    `json:"id"`
	Enabled        bool                      `json:"enabled"`
	SortOrder      int                       `json:"sort_order"`
	RechargeAmount float64                   `json:"recharge_amount"`
	Name           HomePricingLocalizedText  `json:"name"`
	Description    HomePricingLocalizedText  `json:"description"`
	Badge          HomePricingLocalizedText  `json:"badge"`
	Period         HomePricingLocalizedText  `json:"period"`
	Highlight      bool                      `json:"highlight"`
	Metrics        []HomePricingMetricConfig `json:"metrics"`
}

type HomePricingGroupConfig struct {
	Title       HomePricingLocalizedText `json:"title"`
	Description HomePricingLocalizedText `json:"description"`
}

type HomePricingConfig struct {
	ExternalPurchaseURL string                              `json:"external_purchase_url"`
	CTAMode             string                              `json:"cta_mode"`
	Eyebrow             HomePricingLocalizedText            `json:"eyebrow"`
	Title               HomePricingLocalizedText            `json:"title"`
	Description         HomePricingLocalizedText            `json:"description"`
	SubscriptionGroup   HomePricingGroupConfig              `json:"subscription_group"`
	CreditGroup         HomePricingGroupConfig              `json:"credit_group"`
	SubscriptionCards   []HomePricingSubscriptionCardConfig `json:"subscription_cards"`
	CreditCards         []HomePricingCreditCardConfig       `json:"credit_cards"`
}

type HomePricingPublicConfig struct {
	ExternalPurchaseURL string                        `json:"external_purchase_url"`
	CTAMode             string                        `json:"cta_mode"`
	Eyebrow             HomePricingLocalizedText      `json:"eyebrow"`
	Title               HomePricingLocalizedText      `json:"title"`
	Description         HomePricingLocalizedText      `json:"description"`
	SubscriptionGroup   HomePricingGroupConfig        `json:"subscription_group"`
	CreditGroup         HomePricingGroupConfig        `json:"credit_group"`
	SubscriptionCards   []HomePricingPublicPlanCard   `json:"subscription_cards"`
	CreditCards         []HomePricingPublicCreditCard `json:"credit_cards"`
}

type HomePricingPublicPlanCard struct {
	ID                 string                    `json:"id"`
	Enabled            bool                      `json:"enabled"`
	SortOrder          int                       `json:"sort_order"`
	SubscriptionPlanID int64                     `json:"subscription_plan_id"`
	Name               HomePricingLocalizedText  `json:"name"`
	Description        HomePricingLocalizedText  `json:"description"`
	Badge              HomePricingLocalizedText  `json:"badge"`
	Period             HomePricingLocalizedText  `json:"period"`
	Highlight          bool                      `json:"highlight"`
	Metrics            []HomePricingMetricConfig `json:"metrics"`
	Price              float64                   `json:"price"`
	OriginalPrice      *float64                  `json:"original_price,omitempty"`
	ForSale            bool                      `json:"for_sale"`
}

type HomePricingPublicCreditCard struct {
	ID             string                    `json:"id"`
	Enabled        bool                      `json:"enabled"`
	SortOrder      int                       `json:"sort_order"`
	RechargeAmount float64                   `json:"recharge_amount"`
	Name           HomePricingLocalizedText  `json:"name"`
	Description    HomePricingLocalizedText  `json:"description"`
	Badge          HomePricingLocalizedText  `json:"badge"`
	Period         HomePricingLocalizedText  `json:"period"`
	Highlight      bool                      `json:"highlight"`
	Metrics        []HomePricingMetricConfig `json:"metrics"`
	Price          float64                   `json:"price"`
}

func defaultHomePricingConfig() *HomePricingConfig {
	return &HomePricingConfig{
		ExternalPurchaseURL: defaultHomePricingPurchaseURL,
		CTAMode:             HomePricingCTAModeExternal,
		Eyebrow:             HomePricingLocalizedText{Zh: "定价", En: "Pricing"},
		Title: HomePricingLocalizedText{
			Zh: "先选择套餐，再进入控制台开始使用。",
			En: "Choose a plan, then start from the console.",
		},
		Description: HomePricingLocalizedText{
			Zh: "token 数量为营销估算，实际消耗会随模型、输入输出长度和工具行为变化。",
			En: "Token counts are marketing estimates. Actual usage varies by model, input/output length, and tool behavior.",
		},
		SubscriptionGroup: HomePricingGroupConfig{
			Title:       HomePricingLocalizedText{Zh: "订阅套餐", En: "Subscription Plans"},
			Description: HomePricingLocalizedText{Zh: "适合每天持续使用 AI 工具的用户。", En: "For users who run AI tools continuously."},
		},
		CreditGroup: HomePricingGroupConfig{
			Title:       HomePricingLocalizedText{Zh: "额度套餐", En: "Credit Packages"},
			Description: HomePricingLocalizedText{Zh: "适合月卡不够用时灵活补充，额度无每日限制。", En: "Flexible top-ups when your monthly plan is not enough. Credits have no daily limit."},
		},
		SubscriptionCards: []HomePricingSubscriptionCardConfig{},
		CreditCards: []HomePricingCreditCardConfig{
			{
				ID:             "credit-80",
				Enabled:        true,
				SortOrder:      10,
				RechargeAmount: 20,
				Name:           HomePricingLocalizedText{Zh: "$80 额度", En: "$80 Credit"},
				Description:    HomePricingLocalizedText{Zh: "不够用时补充。", En: "Top up when your plan is not enough."},
				Metrics: []HomePricingMetricConfig{
					{Label: HomePricingLocalizedText{Zh: "额度", En: "Credit"}, Value: HomePricingLocalizedText{Zh: "$80", En: "$80"}},
					{Label: HomePricingLocalizedText{Zh: "类型", En: "Type"}, Value: HomePricingLocalizedText{Zh: "余额额度", En: "balance credit"}},
				},
			},
			{
				ID:             "credit-180",
				Enabled:        true,
				SortOrder:      20,
				RechargeAmount: 40,
				Name:           HomePricingLocalizedText{Zh: "$180 额度", En: "$180 Credit"},
				Description:    HomePricingLocalizedText{Zh: "常用补充，适合临时项目加量。", En: "A practical refill for temporary project bursts."},
				Badge:          HomePricingLocalizedText{Zh: "常用", En: "Common"},
				Highlight:      true,
				Metrics: []HomePricingMetricConfig{
					{Label: HomePricingLocalizedText{Zh: "额度", En: "Credit"}, Value: HomePricingLocalizedText{Zh: "$180", En: "$180"}},
					{Label: HomePricingLocalizedText{Zh: "类型", En: "Type"}, Value: HomePricingLocalizedText{Zh: "余额额度", En: "balance credit"}},
				},
			},
			{
				ID:             "credit-1000",
				Enabled:        true,
				SortOrder:      30,
				RechargeAmount: 200,
				Name:           HomePricingLocalizedText{Zh: "$1000 额度", En: "$1000 Credit"},
				Description:    HomePricingLocalizedText{Zh: "适合高频用户、长期使用或拼车。", En: "For high-frequency users, long-running usage, or shared access."},
				Metrics: []HomePricingMetricConfig{
					{Label: HomePricingLocalizedText{Zh: "额度", En: "Credit"}, Value: HomePricingLocalizedText{Zh: "$1000", En: "$1000"}},
					{Label: HomePricingLocalizedText{Zh: "类型", En: "Type"}, Value: HomePricingLocalizedText{Zh: "余额额度", En: "balance credit"}},
				},
			},
		},
	}
}

func parseHomePricingConfig(raw string) (*HomePricingConfig, error) {
	if strings.TrimSpace(raw) == "" {
		return defaultHomePricingConfig(), nil
	}
	var cfg HomePricingConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return nil, err
	}
	normalizeHomePricingConfig(&cfg)
	return &cfg, nil
}

func normalizeHomePricingConfig(cfg *HomePricingConfig) {
	if strings.TrimSpace(cfg.ExternalPurchaseURL) == "" {
		cfg.ExternalPurchaseURL = defaultHomePricingPurchaseURL
	}
	if cfg.CTAMode != HomePricingCTAModeInternal {
		cfg.CTAMode = HomePricingCTAModeExternal
	}
}

func (s *PaymentConfigService) GetHomePricingConfig(ctx context.Context) (*HomePricingConfig, error) {
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyHomePricingConfig)
	if err != nil && !errors.Is(err, ErrSettingNotFound) {
		return nil, fmt.Errorf("get home pricing config: %w", err)
	}
	cfg, err := parseHomePricingConfig(raw)
	if err != nil {
		return nil, infraerrors.InternalServer("HOME_PRICING_CONFIG_INVALID", "home pricing config is invalid").WithCause(err)
	}
	return cfg, nil
}

func (s *PaymentConfigService) UpdateHomePricingConfig(ctx context.Context, cfg HomePricingConfig) (*HomePricingConfig, error) {
	normalizeHomePricingConfig(&cfg)
	if err := s.validateHomePricingConfig(ctx, &cfg); err != nil {
		return nil, err
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal home pricing config: %w", err)
	}
	if err := s.settingRepo.Set(ctx, SettingKeyHomePricingConfig, string(data)); err != nil {
		return nil, fmt.Errorf("save home pricing config: %w", err)
	}
	return &cfg, nil
}

func (s *PaymentConfigService) ResolvePublicHomePricingConfig(ctx context.Context, raw string) json.RawMessage {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	cfg, err := parseHomePricingConfig(raw)
	if err != nil {
		return nil
	}
	publicCfg := s.resolveHomePricingConfig(ctx, cfg)
	if len(publicCfg.SubscriptionCards) == 0 && len(publicCfg.CreditCards) == 0 {
		return nil
	}
	data, err := json.Marshal(publicCfg)
	if err != nil {
		return nil
	}
	return data
}

func (s *PaymentConfigService) resolveHomePricingConfig(ctx context.Context, cfg *HomePricingConfig) HomePricingPublicConfig {
	plansByID := s.homePricingPlansByID(ctx, cfg.SubscriptionCards)
	out := HomePricingPublicConfig{
		ExternalPurchaseURL: cfg.ExternalPurchaseURL,
		CTAMode:             cfg.CTAMode,
		Eyebrow:             cfg.Eyebrow,
		Title:               cfg.Title,
		Description:         cfg.Description,
		SubscriptionGroup:   cfg.SubscriptionGroup,
		CreditGroup:         cfg.CreditGroup,
		SubscriptionCards:   []HomePricingPublicPlanCard{},
		CreditCards:         []HomePricingPublicCreditCard{},
	}
	for _, card := range cfg.SubscriptionCards {
		plan, ok := plansByID[card.SubscriptionPlanID]
		if !card.Enabled || !ok || !plan.ForSale {
			continue
		}
		out.SubscriptionCards = append(out.SubscriptionCards, HomePricingPublicPlanCard{
			ID:                 card.ID,
			Enabled:            card.Enabled,
			SortOrder:          card.SortOrder,
			SubscriptionPlanID: card.SubscriptionPlanID,
			Name:               card.Name,
			Description:        card.Description,
			Badge:              card.Badge,
			Period:             card.Period,
			Highlight:          card.Highlight,
			Metrics:            card.Metrics,
			Price:              plan.Price,
			OriginalPrice:      plan.OriginalPrice,
			ForSale:            plan.ForSale,
		})
	}
	for _, card := range cfg.CreditCards {
		if !card.Enabled {
			continue
		}
		out.CreditCards = append(out.CreditCards, HomePricingPublicCreditCard{
			ID:             card.ID,
			Enabled:        card.Enabled,
			SortOrder:      card.SortOrder,
			RechargeAmount: card.RechargeAmount,
			Name:           card.Name,
			Description:    card.Description,
			Badge:          card.Badge,
			Period:         card.Period,
			Highlight:      card.Highlight,
			Metrics:        card.Metrics,
			Price:          card.RechargeAmount,
		})
	}
	sort.SliceStable(out.SubscriptionCards, func(i, j int) bool {
		return out.SubscriptionCards[i].SortOrder < out.SubscriptionCards[j].SortOrder
	})
	sort.SliceStable(out.CreditCards, func(i, j int) bool {
		return out.CreditCards[i].SortOrder < out.CreditCards[j].SortOrder
	})
	return out
}

func (s *PaymentConfigService) homePricingPlansByID(ctx context.Context, cards []HomePricingSubscriptionCardConfig) map[int64]*dbent.SubscriptionPlan {
	ids := make([]int64, 0, len(cards))
	seen := make(map[int64]bool, len(cards))
	for _, card := range cards {
		if card.SubscriptionPlanID > 0 && !seen[card.SubscriptionPlanID] {
			seen[card.SubscriptionPlanID] = true
			ids = append(ids, card.SubscriptionPlanID)
		}
	}
	if len(ids) == 0 {
		return map[int64]*dbent.SubscriptionPlan{}
	}
	plans, err := s.entClient.SubscriptionPlan.Query().Where(subscriptionplan.IDIn(ids...)).All(ctx)
	if err != nil {
		return map[int64]*dbent.SubscriptionPlan{}
	}
	out := make(map[int64]*dbent.SubscriptionPlan, len(plans))
	for _, plan := range plans {
		out[int64(plan.ID)] = plan
	}
	return out
}

func (s *PaymentConfigService) validateHomePricingConfig(ctx context.Context, cfg *HomePricingConfig) error {
	if cfg.CTAMode != HomePricingCTAModeExternal && cfg.CTAMode != HomePricingCTAModeInternal {
		return infraerrors.BadRequest("HOME_PRICING_CTA_MODE_INVALID", "cta_mode must be external or internal")
	}
	if cfg.CTAMode == HomePricingCTAModeExternal {
		u, err := url.ParseRequestURI(strings.TrimSpace(cfg.ExternalPurchaseURL))
		if err != nil || u.Scheme == "" || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return infraerrors.BadRequest("HOME_PRICING_PURCHASE_URL_INVALID", "external purchase url must be a valid http(s) URL")
		}
	}
	if err := requireLocalizedText(cfg.Eyebrow, "eyebrow"); err != nil {
		return err
	}
	if err := requireLocalizedText(cfg.Title, "title"); err != nil {
		return err
	}
	if err := requireLocalizedText(cfg.Description, "description"); err != nil {
		return err
	}
	if err := requireLocalizedText(cfg.SubscriptionGroup.Title, "subscription_group.title"); err != nil {
		return err
	}
	if err := requireLocalizedText(cfg.CreditGroup.Title, "credit_group.title"); err != nil {
		return err
	}
	plansByID := s.homePricingPlansByID(ctx, cfg.SubscriptionCards)
	for i, card := range cfg.SubscriptionCards {
		prefix := fmt.Sprintf("subscription_cards[%d]", i)
		if card.SubscriptionPlanID <= 0 {
			return infraerrors.BadRequest("HOME_PRICING_PLAN_REQUIRED", prefix+" must bind a subscription plan")
		}
		if _, ok := plansByID[card.SubscriptionPlanID]; !ok {
			return infraerrors.BadRequest("HOME_PRICING_PLAN_NOT_FOUND", prefix+" references a missing subscription plan")
		}
		if err := requireLocalizedText(card.Name, prefix+".name"); err != nil {
			return err
		}
		if err := requireLocalizedText(card.Description, prefix+".description"); err != nil {
			return err
		}
	}
	for i, card := range cfg.CreditCards {
		prefix := fmt.Sprintf("credit_cards[%d]", i)
		if card.RechargeAmount <= 0 {
			return infraerrors.BadRequest("HOME_PRICING_RECHARGE_AMOUNT_INVALID", prefix+" recharge amount must be > 0")
		}
		if err := requireLocalizedText(card.Name, prefix+".name"); err != nil {
			return err
		}
		if err := requireLocalizedText(card.Description, prefix+".description"); err != nil {
			return err
		}
	}
	return nil
}

func requireLocalizedText(value HomePricingLocalizedText, field string) error {
	if strings.TrimSpace(value.Zh) == "" || strings.TrimSpace(value.En) == "" {
		return infraerrors.BadRequest("HOME_PRICING_I18N_REQUIRED", field+" requires both zh and en")
	}
	return nil
}
