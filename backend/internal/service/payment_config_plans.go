package service

import (
	"context"
	"fmt"
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
REDACTED
	if groupID <= 0 {
		return infraerrors.BadRequest("PLAN_GROUP_REQUIRED", "group is required")
REDACTED
	if price <= 0 {
		return infraerrors.BadRequest("PLAN_PRICE_INVALID", "price must be > 0")
REDACTED
	if validityDays <= 0 {
		return infraerrors.BadRequest("PLAN_VALIDITY_REQUIRED", "validity days must be > 0")
REDACTED
	if strings.TrimSpace(validityUnit) == "" {
		return infraerrors.BadRequest("PLAN_VALIDITY_UNIT_REQUIRED", "validity unit is required")
REDACTED
	if originalPrice != nil && *originalPrice < 0 {
		return infraerrors.BadRequest("PLAN_ORIGINAL_PRICE_INVALID", "original price must be >= 0")
REDACTED
	return nil
REDACTED

// validatePlanPatch validates only the non-nil fields in a patch update.
func validatePlanPatch(req UpdatePlanRequest) error {
	if req.Name != nil && strings.TrimSpace(*req.Name) == "" {
		return infraerrors.BadRequest("PLAN_NAME_REQUIRED", "plan name is required")
REDACTED
	if req.GroupID != nil && *req.GroupID <= 0 {
		return infraerrors.BadRequest("PLAN_GROUP_REQUIRED", "group is required")
REDACTED
	if req.Price != nil && *req.Price <= 0 {
		return infraerrors.BadRequest("PLAN_PRICE_INVALID", "price must be > 0")
REDACTED
	if req.ValidityDays != nil && *req.ValidityDays <= 0 {
		return infraerrors.BadRequest("PLAN_VALIDITY_REQUIRED", "validity days must be > 0")
REDACTED
	if req.ValidityUnit != nil && strings.TrimSpace(*req.ValidityUnit) == "" {
		return infraerrors.BadRequest("PLAN_VALIDITY_UNIT_REQUIRED", "validity unit is required")
REDACTED
	if req.OriginalPrice != nil && *req.OriginalPrice < 0 {
		return infraerrors.BadRequest("PLAN_ORIGINAL_PRICE_INVALID", "original price must be >= 0")
REDACTED
	return nil
REDACTED

// --- Plan CRUD ---

// PlanGroupInfo holds the group details needed for subscription plan display.
type PlanGroupInfo struct {
	Platform           string   `json:"platform"`
	Name               string   `json:"name"`
	RateMultiplier     float64  `json:"rate_multiplier"`
	PeakRateEnabled    bool     `json:"peak_rate_enabled"`
	PeakStart          string   `json:"peak_start"`
	PeakEnd            string   `json:"peak_end"`
	PeakRateMultiplier float64  `json:"peak_rate_multiplier"`
	DailyLimitUSD      *float64 `json:"daily_limit_usd"`
	WeeklyLimitUSD     *float64 `json:"weekly_limit_usd"`
	MonthlyLimitUSD    *float64 `json:"monthly_limit_usd"`
	ModelScopes        []string `json:"supported_model_scopes"`
REDACTED

// GetGroupPlatformMap returns a map of group_id → platform for the given plans.
func (s *PaymentConfigService) GetGroupPlatformMap(ctx context.Context, plans []*dbent.SubscriptionPlan) map[int64]string {
	info := s.GetGroupInfoMap(ctx, plans)
	m := make(map[int64]string, len(info))
	for id, gi := range info {
		m[id] = gi.Platform
REDACTED
	return m
REDACTED

// GetGroupInfoMap returns a map of group_id → PlanGroupInfo for the given plans.
func (s *PaymentConfigService) GetGroupInfoMap(ctx context.Context, plans []*dbent.SubscriptionPlan) map[int64]PlanGroupInfo {
	ids := make([]int64, 0, len(plans))
	seen := make(map[int64]bool)
	for _, p := range plans {
		if !seen[p.GroupID] {
			seen[p.GroupID] = true
			ids = append(ids, p.GroupID)
	REDACTED
REDACTED
	if len(ids) == 0 {
		return nil
REDACTED
	groups, err := s.entClient.Group.Query().Where(group.IDIn(ids...)).All(ctx)
	if err != nil {
		return nil
REDACTED
	m := make(map[int64]PlanGroupInfo, len(groups))
	for _, g := range groups {
		m[int64(g.ID)] = PlanGroupInfo{
			Platform:           g.Platform,
			Name:               g.Name,
			RateMultiplier:     g.RateMultiplier,
			PeakRateEnabled:    g.PeakRateEnabled,
			PeakStart:          g.PeakStart,
			PeakEnd:            g.PeakEnd,
			PeakRateMultiplier: g.PeakRateMultiplier,
			DailyLimitUSD:      g.DailyLimitUsd,
			WeeklyLimitUSD:     g.WeeklyLimitUsd,
			MonthlyLimitUSD:    g.MonthlyLimitUsd,
			ModelScopes:        g.SupportedModelScopes,
	REDACTED
REDACTED
	return m
REDACTED

func (s *PaymentConfigService) ListPlans(ctx context.Context) ([]*dbent.SubscriptionPlan, error) {
	return s.entClient.SubscriptionPlan.Query().Order(subscriptionplan.BySortOrder()).All(ctx)
REDACTED

func (s *PaymentConfigService) ListPlansForSale(ctx context.Context) ([]*dbent.SubscriptionPlan, error) {
	return s.entClient.SubscriptionPlan.Query().Where(subscriptionplan.ForSaleEQ(true)).Order(subscriptionplan.BySortOrder()).All(ctx)
REDACTED

func (s *PaymentConfigService) CreatePlan(ctx context.Context, req CreatePlanRequest) (*dbent.SubscriptionPlan, error) {
	if err := validatePlanRequired(req.Name, req.GroupID, req.Price, req.ValidityDays, req.ValidityUnit, req.OriginalPrice); err != nil {
		return nil, err
REDACTED
	b := s.entClient.SubscriptionPlan.Create().
		SetGroupID(req.GroupID).SetName(req.Name).SetDescription(req.Description).
		SetPrice(req.Price).SetValidityDays(req.ValidityDays).SetValidityUnit(req.ValidityUnit).
		SetFeatures(req.Features).SetProductName(req.ProductName).
		SetForSale(req.ForSale).SetSortOrder(req.SortOrder)
	if req.OriginalPrice != nil {
		b.SetOriginalPrice(*req.OriginalPrice)
REDACTED
	return b.Save(ctx)
REDACTED

// UpdatePlan updates a subscription plan by ID (patch semantics).
// NOTE: This function exceeds 30 lines due to per-field nil-check patch update boilerplate
// plus a validation guard for non-nil fields.
func (s *PaymentConfigService) UpdatePlan(ctx context.Context, id int64, req UpdatePlanRequest) (*dbent.SubscriptionPlan, error) {
	if err := validatePlanPatch(req); err != nil {
		return nil, err
REDACTED
	u := s.entClient.SubscriptionPlan.UpdateOneID(id)
	if req.GroupID != nil {
		u.SetGroupID(*req.GroupID)
REDACTED
	if req.Name != nil {
		u.SetName(*req.Name)
REDACTED
	if req.Description != nil {
		u.SetDescription(*req.Description)
REDACTED
	if req.Price != nil {
		u.SetPrice(*req.Price)
REDACTED
	if req.OriginalPrice != nil {
		u.SetOriginalPrice(*req.OriginalPrice)
REDACTED
	if req.ValidityDays != nil {
		u.SetValidityDays(*req.ValidityDays)
REDACTED
	if req.ValidityUnit != nil {
		u.SetValidityUnit(*req.ValidityUnit)
REDACTED
	if req.Features != nil {
		u.SetFeatures(*req.Features)
REDACTED
	if req.ProductName != nil {
		u.SetProductName(*req.ProductName)
REDACTED
	if req.ForSale != nil {
		u.SetForSale(*req.ForSale)
REDACTED
	if req.SortOrder != nil {
		u.SetSortOrder(*req.SortOrder)
REDACTED
	return u.Save(ctx)
REDACTED

func (s *PaymentConfigService) DeletePlan(ctx context.Context, id int64) error {
	count, err := s.countPendingOrdersByPlan(ctx, id)
	if err != nil {
		return fmt.Errorf("check pending orders: %w", err)
REDACTED
	if count > 0 {
		return infraerrors.Conflict("PENDING_ORDERS",
			fmt.Sprintf("this plan has %d in-progress orders and cannot be deleted — wait for orders to complete first", count))
REDACTED
	return s.entClient.SubscriptionPlan.DeleteOneID(id).Exec(ctx)
REDACTED

// GetPlan returns a subscription plan by ID.
func (s *PaymentConfigService) GetPlan(ctx context.Context, id int64) (*dbent.SubscriptionPlan, error) {
	plan, err := s.entClient.SubscriptionPlan.Get(ctx, id)
	if err != nil {
		return nil, infraerrors.NotFound("PLAN_NOT_FOUND", "subscription plan not found")
REDACTED
	return plan, nil
REDACTED
