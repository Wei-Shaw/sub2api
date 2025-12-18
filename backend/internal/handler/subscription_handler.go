package handler

import (
	"sub2api/internal/model"
	"sub2api/internal/pkg/response"
	"sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// SubscriptionSummaryItem represents a subscription item in summary
type SubscriptionSummaryItem struct {
	ID              int64   `json:"id"`
	GroupID         int64   `json:"group_id"`
	GroupName       string  `json:"group_name"`
	Status          string  `json:"status"`
	DailyUsedUSD    float64 `json:"daily_used_usd,omitempty"`
	DailyLimitUSD   float64 `json:"daily_limit_usd,omitempty"`
	WeeklyUsedUSD   float64 `json:"weekly_used_usd,omitempty"`
	WeeklyLimitUSD  float64 `json:"weekly_limit_usd,omitempty"`
	MonthlyUsedUSD  float64 `json:"monthly_used_usd,omitempty"`
	MonthlyLimitUSD float64 `json:"monthly_limit_usd,omitempty"`
	ExpiresAt       *string `json:"expires_at,omitempty"`
REDACTED

// SubscriptionProgressInfo represents subscription with progress info
type SubscriptionProgressInfo struct {
	Subscription *model.UserSubscription       `json:"subscription"`
	Progress     *service.SubscriptionProgress `json:"progress"`
REDACTED

// SubscriptionHandler handles user subscription operations
type SubscriptionHandler struct {
	subscriptionService *service.SubscriptionService
REDACTED

// NewSubscriptionHandler creates a new user subscription handler
func NewSubscriptionHandler(subscriptionService *service.SubscriptionService) *SubscriptionHandler {
	return &SubscriptionHandler{
		subscriptionService: subscriptionService,
REDACTED
REDACTED

// List handles listing current user's subscriptions
// GET /api/v1/subscriptions
func (h *SubscriptionHandler) List(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "User not found in context")
		return
REDACTED

	u, ok := user.(*model.User)
	if !ok {
		response.InternalError(c, "Invalid user in context")
		return
REDACTED

	subscriptions, err := h.subscriptionService.ListUserSubscriptions(c.Request.Context(), u.ID)
	if err != nil {
		response.InternalError(c, "Failed to list subscriptions: "+err.Error())
		return
REDACTED

	response.Success(c, subscriptions)
REDACTED

// GetActive handles getting current user's active subscriptions
// GET /api/v1/subscriptions/active
func (h *SubscriptionHandler) GetActive(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "User not found in context")
		return
REDACTED

	u, ok := user.(*model.User)
	if !ok {
		response.InternalError(c, "Invalid user in context")
		return
REDACTED

	subscriptions, err := h.subscriptionService.ListActiveUserSubscriptions(c.Request.Context(), u.ID)
	if err != nil {
		response.InternalError(c, "Failed to get active subscriptions: "+err.Error())
		return
REDACTED

	response.Success(c, subscriptions)
REDACTED

// GetProgress handles getting subscription progress for current user
// GET /api/v1/subscriptions/progress
func (h *SubscriptionHandler) GetProgress(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "User not found in context")
		return
REDACTED

	u, ok := user.(*model.User)
	if !ok {
		response.InternalError(c, "Invalid user in context")
		return
REDACTED

	// Get all active subscriptions with progress
	subscriptions, err := h.subscriptionService.ListActiveUserSubscriptions(c.Request.Context(), u.ID)
	if err != nil {
		response.InternalError(c, "Failed to get subscriptions: "+err.Error())
		return
REDACTED

	result := make([]SubscriptionProgressInfo, 0, len(subscriptions))
	for i := range subscriptions {
		sub := &subscriptions[i]
		progress, err := h.subscriptionService.GetSubscriptionProgress(c.Request.Context(), sub.ID)
		if err != nil {
			// Skip subscriptions with errors
			continue
	REDACTED
		result = append(result, SubscriptionProgressInfo{
			Subscription: sub,
			Progress:     progress,
	REDACTED)
REDACTED

	response.Success(c, result)
REDACTED

// GetSummary handles getting a summary of current user's subscription status
// GET /api/v1/subscriptions/summary
func (h *SubscriptionHandler) GetSummary(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "User not found in context")
		return
REDACTED

	u, ok := user.(*model.User)
	if !ok {
		response.InternalError(c, "Invalid user in context")
		return
REDACTED

	// Get all active subscriptions
	subscriptions, err := h.subscriptionService.ListActiveUserSubscriptions(c.Request.Context(), u.ID)
	if err != nil {
		response.InternalError(c, "Failed to get subscriptions: "+err.Error())
		return
REDACTED

	var totalUsed float64
	items := make([]SubscriptionSummaryItem, 0, len(subscriptions))

	for _, sub := range subscriptions {
		item := SubscriptionSummaryItem{
			ID:             sub.ID,
			GroupID:        sub.GroupID,
			Status:         sub.Status,
			DailyUsedUSD:   sub.DailyUsageUSD,
			WeeklyUsedUSD:  sub.WeeklyUsageUSD,
			MonthlyUsedUSD: sub.MonthlyUsageUSD,
	REDACTED

		// Add group info if preloaded
		if sub.Group != nil {
			item.GroupName = sub.Group.Name
			if sub.Group.DailyLimitUSD != nil {
				item.DailyLimitUSD = *sub.Group.DailyLimitUSD
		REDACTED
			if sub.Group.WeeklyLimitUSD != nil {
				item.WeeklyLimitUSD = *sub.Group.WeeklyLimitUSD
		REDACTED
			if sub.Group.MonthlyLimitUSD != nil {
				item.MonthlyLimitUSD = *sub.Group.MonthlyLimitUSD
		REDACTED
	REDACTED

		// Format expiration time
		if !sub.ExpiresAt.IsZero() {
			formatted := sub.ExpiresAt.Format("2006-01-02T15:04:05Z07:00")
			item.ExpiresAt = &formatted
	REDACTED

		// Track total usage (use monthly as the most comprehensive)
		totalUsed += sub.MonthlyUsageUSD

		items = append(items, item)
REDACTED

	summary := struct {
		ActiveCount   int                       `json:"active_count"`
		TotalUsedUSD  float64                   `json:"total_used_usd"`
		Subscriptions []SubscriptionSummaryItem `json:"subscriptions"`
REDACTED{
		ActiveCount:   len(subscriptions),
		TotalUsedUSD:  totalUsed,
		Subscriptions: items,
REDACTED

	response.Success(c, summary)
REDACTED
