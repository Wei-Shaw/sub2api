package service

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/group"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionplan"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// validatePlanRequired checks that all required fields for a plan are provided.
func validatePlanRequired(name string, groupID int64, price float64, validityDays int, validityUnit string, originalPrice *float64) error {
	if strings.TrimSpace(name) == "" {
		return infraerrors.BadRequest("PLAN_NAME_REQUIRED", "plan name is required")
	}
	if groupID <= 0 {
		return infraerrors.BadRequest("PLAN_GROUP_REQUIRED", "group is required")
	}
	if price <= 0 {
		return infraerrors.BadRequest("PLAN_PRICE_INVALID", "price must be > 0")
	}
	if validityDays <= 0 {
		return infraerrors.BadRequest("PLAN_VALIDITY_REQUIRED", "validity days must be > 0")
	}
	if strings.TrimSpace(validityUnit) == "" {
		return infraerrors.BadRequest("PLAN_VALIDITY_UNIT_REQUIRED", "validity unit is required")
	}
	if originalPrice != nil && *originalPrice < 0 {
		return infraerrors.BadRequest("PLAN_ORIGINAL_PRICE_INVALID", "original price must be >= 0")
	}
	return nil
}

func validateExternalSubscribeURL(rawURL string) error {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return nil
	}
	normalized := normalizeExternalSubscribeURL(trimmed)
	parsed, err := url.ParseRequestURI(normalized)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return infraerrors.BadRequest("PLAN_EXTERNAL_SUBSCRIBE_URL_INVALID", "external subscribe URL must be a valid absolute URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return infraerrors.BadRequest("PLAN_EXTERNAL_SUBSCRIBE_URL_INVALID", "external subscribe URL must use http or https")
	}
	return nil
}

func normalizeExternalSubscribeDialogText(raw string) string {
	return strings.TrimSpace(raw)
}

func hasExternalSubscribeTarget(rawURL, dialogText string) bool {
	return strings.TrimSpace(rawURL) != "" || strings.TrimSpace(dialogText) != ""
}

func validateExternalSubscribeTarget(enabled bool, rawURL, dialogText string) error {
	if strings.TrimSpace(rawURL) != "" && strings.TrimSpace(dialogText) != "" {
		return infraerrors.BadRequest("PLAN_EXTERNAL_SUBSCRIBE_TARGET_CONFLICT", "external subscribe URL and dialog text cannot both be set")
	}
	if !enabled && !hasExternalSubscribeTarget(rawURL, dialogText) {
		return nil
	}
	if !hasExternalSubscribeTarget(rawURL, dialogText) {
		return infraerrors.BadRequest("PLAN_EXTERNAL_SUBSCRIBE_TARGET_REQUIRED", "external subscribe target is required")
	}
	if err := validateExternalSubscribeURL(rawURL); err != nil {
		return err
	}
	return nil
}

func normalizeExternalSubscribeURL(rawURL string) string {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err == nil && parsed.Scheme != "" {
		return trimmed
	}
	return "https://" + trimmed
}

func (s *PaymentConfigService) validatePlanGroupType(ctx context.Context, groupID int64) error {
	exists, err := s.entClient.Group.Query().
		Where(
			group.IDEQ(groupID),
			group.SubscriptionTypeEQ(SubscriptionTypeSubscription),
		).
		Exist(ctx)
	if err != nil {
		return fmt.Errorf("check plan group type: %w", err)
	}
	if !exists {
		return ErrGroupNotSubscriptionType
	}
	return nil
}

// validatePlanPatch validates only the non-nil fields in a patch update.
func validatePlanPatch(req UpdatePlanRequest) error {
	if req.Name != nil && strings.TrimSpace(*req.Name) == "" {
		return infraerrors.BadRequest("PLAN_NAME_REQUIRED", "plan name is required")
	}
	if req.GroupID != nil && *req.GroupID <= 0 {
		return infraerrors.BadRequest("PLAN_GROUP_REQUIRED", "group is required")
	}
	if req.Price != nil && *req.Price <= 0 {
		return infraerrors.BadRequest("PLAN_PRICE_INVALID", "price must be > 0")
	}
	if req.ValidityDays != nil && *req.ValidityDays <= 0 {
		return infraerrors.BadRequest("PLAN_VALIDITY_REQUIRED", "validity days must be > 0")
	}
	if req.ValidityUnit != nil && strings.TrimSpace(*req.ValidityUnit) == "" {
		return infraerrors.BadRequest("PLAN_VALIDITY_UNIT_REQUIRED", "validity unit is required")
	}
	if req.OriginalPrice != nil && *req.OriginalPrice < 0 {
		return infraerrors.BadRequest("PLAN_ORIGINAL_PRICE_INVALID", "original price must be >= 0")
	}
	return nil
}

// --- Plan CRUD ---

// PlanGroupInfo holds the group details needed for subscription plan display.
type PlanGroupInfo struct {
	Platform        string   `json:"platform"`
	Name            string   `json:"name"`
	RateMultiplier  float64  `json:"rate_multiplier"`
	DailyLimitUSD   *float64 `json:"daily_limit_usd"`
	WeeklyLimitUSD  *float64 `json:"weekly_limit_usd"`
	MonthlyLimitUSD *float64 `json:"monthly_limit_usd"`
	ModelScopes     []string `json:"supported_model_scopes"`
}

// GetGroupPlatformMap returns a map of group_id → platform for the given plans.
func (s *PaymentConfigService) GetGroupPlatformMap(ctx context.Context, plans []*dbent.SubscriptionPlan) map[int64]string {
	info := s.GetGroupInfoMap(ctx, plans)
	m := make(map[int64]string, len(info))
	for id, gi := range info {
		m[id] = gi.Platform
	}
	return m
}

// GetGroupInfoMap returns a map of group_id → PlanGroupInfo for the given plans.
func (s *PaymentConfigService) GetGroupInfoMap(ctx context.Context, plans []*dbent.SubscriptionPlan) map[int64]PlanGroupInfo {
	ids := make([]int64, 0, len(plans))
	seen := make(map[int64]bool)
	for _, p := range plans {
		if !seen[p.GroupID] {
			seen[p.GroupID] = true
			ids = append(ids, p.GroupID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	groups, err := s.entClient.Group.Query().Where(group.IDIn(ids...)).All(ctx)
	if err != nil {
		return nil
	}
	m := make(map[int64]PlanGroupInfo, len(groups))
	for _, g := range groups {
		m[int64(g.ID)] = PlanGroupInfo{
			Platform:        g.Platform,
			Name:            g.Name,
			RateMultiplier:  g.RateMultiplier,
			DailyLimitUSD:   g.DailyLimitUsd,
			WeeklyLimitUSD:  g.WeeklyLimitUsd,
			MonthlyLimitUSD: g.MonthlyLimitUsd,
			ModelScopes:     g.SupportedModelScopes,
		}
	}
	return m
}

func (s *PaymentConfigService) ListPlans(ctx context.Context) ([]*dbent.SubscriptionPlan, error) {
	return s.entClient.SubscriptionPlan.Query().Order(subscriptionplan.BySortOrder()).All(ctx)
}

func (s *PaymentConfigService) ListPlansForSale(ctx context.Context) ([]*dbent.SubscriptionPlan, error) {
	return s.entClient.SubscriptionPlan.Query().Where(subscriptionplan.ForSaleEQ(true)).Order(subscriptionplan.BySortOrder()).All(ctx)
}

func (s *PaymentConfigService) CreatePlan(ctx context.Context, req CreatePlanRequest) (*dbent.SubscriptionPlan, error) {
	if err := validatePlanRequired(req.Name, req.GroupID, req.Price, req.ValidityDays, req.ValidityUnit, req.OriginalPrice); err != nil {
		return nil, err
	}
	if err := s.validatePlanGroupType(ctx, req.GroupID); err != nil {
		return nil, err
	}
	if err := validateExternalSubscribeTarget(req.ExternalSubscribeEnabled, req.ExternalSubscribeURL, req.ExternalSubscribeDialogText); err != nil {
		return nil, err
	}
	b := s.entClient.SubscriptionPlan.Create().
		SetGroupID(req.GroupID).SetName(req.Name).SetDescription(req.Description).
		SetPrice(req.Price).SetValidityDays(req.ValidityDays).SetValidityUnit(req.ValidityUnit).
		SetFeatures(req.Features).SetProductName(req.ProductName).
		SetExternalSubscribeEnabled(req.ExternalSubscribeEnabled).
		SetExternalSubscribeURL(normalizeExternalSubscribeURL(req.ExternalSubscribeURL)).
		SetExternalSubscribeDialogText(normalizeExternalSubscribeDialogText(req.ExternalSubscribeDialogText)).
		SetForSale(req.ForSale).SetSortOrder(req.SortOrder)
	if req.OriginalPrice != nil {
		b.SetOriginalPrice(*req.OriginalPrice)
	}
	return b.Save(ctx)
}

// UpdatePlan updates a subscription plan by ID (patch semantics).
// NOTE: This function exceeds 30 lines due to per-field nil-check patch update boilerplate
// plus a validation guard for non-nil fields.
func (s *PaymentConfigService) UpdatePlan(ctx context.Context, id int64, req UpdatePlanRequest) (*dbent.SubscriptionPlan, error) {
	if err := validatePlanPatch(req); err != nil {
		return nil, err
	}
	// Only validate explicit group reassignment so legacy invalid plans can still
	// be disabled or repaired without being trapped by unrelated field edits.
	if req.GroupID != nil {
		if err := s.validatePlanGroupType(ctx, *req.GroupID); err != nil {
			return nil, err
		}
	}
	if req.ExternalSubscribeEnabled != nil || req.ExternalSubscribeURL != nil || req.ExternalSubscribeDialogText != nil {
		current, err := s.entClient.SubscriptionPlan.Get(ctx, id)
		if err != nil {
			return nil, infraerrors.NotFound("PLAN_NOT_FOUND", "subscription plan not found")
		}
		enabled := current.ExternalSubscribeEnabled
		rawURL := current.ExternalSubscribeURL
		dialogText := current.ExternalSubscribeDialogText
		if req.ExternalSubscribeEnabled != nil {
			enabled = *req.ExternalSubscribeEnabled
		}
		if req.ExternalSubscribeURL != nil {
			rawURL = *req.ExternalSubscribeURL
		}
		if req.ExternalSubscribeDialogText != nil {
			dialogText = *req.ExternalSubscribeDialogText
		}
		if err := validateExternalSubscribeTarget(enabled, rawURL, dialogText); err != nil {
			return nil, err
		}
	}
	u := s.entClient.SubscriptionPlan.UpdateOneID(id)
	if req.GroupID != nil {
		u.SetGroupID(*req.GroupID)
	}
	if req.Name != nil {
		u.SetName(*req.Name)
	}
	if req.Description != nil {
		u.SetDescription(*req.Description)
	}
	if req.Price != nil {
		u.SetPrice(*req.Price)
	}
	if req.OriginalPrice != nil {
		u.SetOriginalPrice(*req.OriginalPrice)
	}
	if req.ValidityDays != nil {
		u.SetValidityDays(*req.ValidityDays)
	}
	if req.ValidityUnit != nil {
		u.SetValidityUnit(*req.ValidityUnit)
	}
	if req.Features != nil {
		u.SetFeatures(*req.Features)
	}
	if req.ProductName != nil {
		u.SetProductName(*req.ProductName)
	}
	if req.ExternalSubscribeEnabled != nil {
		u.SetExternalSubscribeEnabled(*req.ExternalSubscribeEnabled)
	}
	if req.ExternalSubscribeURL != nil {
		u.SetExternalSubscribeURL(normalizeExternalSubscribeURL(*req.ExternalSubscribeURL))
	}
	if req.ExternalSubscribeDialogText != nil {
		u.SetExternalSubscribeDialogText(normalizeExternalSubscribeDialogText(*req.ExternalSubscribeDialogText))
	}
	if req.ForSale != nil {
		u.SetForSale(*req.ForSale)
	}
	if req.SortOrder != nil {
		u.SetSortOrder(*req.SortOrder)
	}
	return u.Save(ctx)
}

func (s *PaymentConfigService) DeletePlan(ctx context.Context, id int64) error {
	count, err := s.countPendingOrdersByPlan(ctx, id)
	if err != nil {
		return fmt.Errorf("check pending orders: %w", err)
	}
	if count > 0 {
		return infraerrors.Conflict("PENDING_ORDERS",
			fmt.Sprintf("this plan has %d in-progress orders and cannot be deleted — wait for orders to complete first", count))
	}
	return s.entClient.SubscriptionPlan.DeleteOneID(id).Exec(ctx)
}

// GetPlan returns a subscription plan by ID.
func (s *PaymentConfigService) GetPlan(ctx context.Context, id int64) (*dbent.SubscriptionPlan, error) {
	plan, err := s.entClient.SubscriptionPlan.Get(ctx, id)
	if err != nil {
		return nil, infraerrors.NotFound("PLAN_NOT_FOUND", "subscription plan not found")
	}
	return plan, nil
}
