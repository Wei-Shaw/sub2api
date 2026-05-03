package admin

import (
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// AffiliateHandler handles admin affiliate (邀请返利) management:
// listing users with custom settings, updating per-user invite codes
// and exclusive rebate rates, and batch operations.
type AffiliateHandler struct {
	affiliateService *service.AffiliateService
	adminService     service.AdminService
REDACTED

// NewAffiliateHandler creates a new admin affiliate handler.
func NewAffiliateHandler(affiliateService *service.AffiliateService, adminService service.AdminService) *AffiliateHandler {
	return &AffiliateHandler{
		affiliateService: affiliateService,
		adminService:     adminService,
REDACTED
REDACTED

// ListUsers returns paginated users with custom affiliate settings.
// GET /api/v1/admin/affiliates/users
func (h *AffiliateHandler) ListUsers(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	search := c.Query("search")

	entries, total, err := h.affiliateService.AdminListCustomUsers(c.Request.Context(), service.AffiliateAdminFilter{
		Search:   search,
		Page:     page,
		PageSize: pageSize,
REDACTED)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Paginated(c, entries, total, page, pageSize)
REDACTED

// UpdateUserSettings updates a user's affiliate settings.
// PUT /api/v1/admin/affiliates/users/:user_id
//
// Both fields are optional and applied independently.
type UpdateAffiliateUserRequest struct {
	AffCode              *string  `json:"aff_code"`
	AffRebateRatePercent *float64 `json:"aff_rebate_rate_percent"`
	// ClearRebateRate explicitly clears the per-user rate (sets it to NULL).
	// Used to disambiguate from "field not provided".
	ClearRebateRate bool `json:"clear_rebate_rate"`
REDACTED

func (h *AffiliateHandler) UpdateUserSettings(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("user_id"), 10, 64)
	if err != nil || userID <= 0 {
		response.BadRequest(c, "Invalid user_id")
		return
REDACTED

	var req UpdateAffiliateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	if req.AffCode != nil {
		if err := h.affiliateService.AdminUpdateUserAffCode(c.Request.Context(), userID, *req.AffCode); err != nil {
			response.ErrorFrom(c, err)
			return
	REDACTED
REDACTED

	if req.ClearRebateRate {
		if err := h.affiliateService.AdminSetUserRebateRate(c.Request.Context(), userID, nil); err != nil {
			response.ErrorFrom(c, err)
			return
	REDACTED
REDACTED else if req.AffRebateRatePercent != nil {
		if err := h.affiliateService.AdminSetUserRebateRate(c.Request.Context(), userID, req.AffRebateRatePercent); err != nil {
			response.ErrorFrom(c, err)
			return
	REDACTED
REDACTED

	response.Success(c, gin.H{"user_id": userIDREDACTED)
REDACTED

// ClearUserSettings removes ALL of a user's custom affiliate settings — clears
// the exclusive rebate rate AND regenerates the invite code as a new system
// random one. Conceptually this "removes the user from the custom list".
//
// Both writes happen in this handler; failure of one leaves the other applied,
// but the operation is idempotent so the admin can re-run it safely.
// DELETE /api/v1/admin/affiliates/users/:user_id
func (h *AffiliateHandler) ClearUserSettings(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("user_id"), 10, 64)
	if err != nil || userID <= 0 {
		response.BadRequest(c, "Invalid user_id")
		return
REDACTED
	if err := h.affiliateService.AdminSetUserRebateRate(c.Request.Context(), userID, nil); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	if _, err := h.affiliateService.AdminResetUserAffCode(c.Request.Context(), userID); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, gin.H{"user_id": userIDREDACTED)
REDACTED

// BatchSetRate applies the same rebate rate (or clears it) to multiple users.
//
// Protocol: pass `clear: true` to clear rates (aff_rebate_rate_percent is
// ignored). Otherwise aff_rebate_rate_percent is required and applied to
// every user_id. The explicit `clear` flag exists because Go's JSON unmarshal
// can't distinguish a missing field from `null`, and a silent clear from a
// frontend that forgot to include the rate would be a footgun.
//
// POST /api/v1/admin/affiliates/users/batch-rate
type BatchSetRateRequest struct {
	UserIDs              []int64  `json:"user_ids" binding:"required"`
	AffRebateRatePercent *float64 `json:"aff_rebate_rate_percent"`
	Clear                bool     `json:"clear"`
REDACTED

func (h *AffiliateHandler) BatchSetRate(c *gin.Context) {
	var req BatchSetRateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED
	if len(req.UserIDs) == 0 {
		response.BadRequest(c, "user_ids cannot be empty")
		return
REDACTED
	if !req.Clear && req.AffRebateRatePercent == nil {
		response.BadRequest(c, "aff_rebate_rate_percent is required unless clear=true")
		return
REDACTED
	rate := req.AffRebateRatePercent
	if req.Clear {
		rate = nil
REDACTED
	if err := h.affiliateService.AdminBatchSetUserRebateRate(c.Request.Context(), req.UserIDs, rate); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, gin.H{"affected": len(req.UserIDs)REDACTED)
REDACTED

// AffiliateUserSummary is the minimal user shape returned by LookupUsers,
// shared with the frontend's add-custom-user picker.
type AffiliateUserSummary struct {
	ID       int64  `json:"id"`
	Email    string `json:"email"`
	Username string `json:"username"`
REDACTED

// LookupUsers searches users by email/username for the "add custom user" modal.
// GET /api/v1/admin/affiliates/users/lookup?q=
func (h *AffiliateHandler) LookupUsers(c *gin.Context) {
	keyword := c.Query("q")
	if keyword == "" {
		response.Success(c, []AffiliateUserSummary{REDACTED)
		return
REDACTED
	users, _, err := h.adminService.ListUsers(c.Request.Context(), 1, 20, service.UserListFilters{Search: keywordREDACTED, "email", "asc")
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	result := make([]AffiliateUserSummary, len(users))
	for i, u := range users {
		result[i] = AffiliateUserSummary{ID: u.ID, Email: u.Email, Username: u.UsernameREDACTED
REDACTED
	response.Success(c, result)
REDACTED

// GetUserOverview returns one user's affiliate overview.
// GET /api/v1/admin/affiliates/users/:user_id/overview
func (h *AffiliateHandler) GetUserOverview(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("user_id"), 10, 64)
	if err != nil || userID <= 0 {
		response.BadRequest(c, "Invalid user_id")
		return
REDACTED
	overview, err := h.affiliateService.AdminGetUserOverview(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, overview)
REDACTED

// ListInviteRecords returns all inviter-invitee relationships.
// GET /api/v1/admin/affiliates/invites
func (h *AffiliateHandler) ListInviteRecords(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	filter := parseAffiliateRecordFilter(c, page, pageSize)
	items, total, err := h.affiliateService.AdminListInviteRecords(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Paginated(c, items, total, filter.Page, filter.PageSize)
REDACTED

// ListRebateRecords returns all order-level affiliate rebate records.
// GET /api/v1/admin/affiliates/rebates
func (h *AffiliateHandler) ListRebateRecords(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	filter := parseAffiliateRecordFilter(c, page, pageSize)
	items, total, err := h.affiliateService.AdminListRebateRecords(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Paginated(c, items, total, filter.Page, filter.PageSize)
REDACTED

// ListTransferRecords returns all affiliate quota-to-balance transfer records.
// GET /api/v1/admin/affiliates/transfers
func (h *AffiliateHandler) ListTransferRecords(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	filter := parseAffiliateRecordFilter(c, page, pageSize)
	items, total, err := h.affiliateService.AdminListTransferRecords(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Paginated(c, items, total, filter.Page, filter.PageSize)
REDACTED

func parseAffiliateRecordFilter(c *gin.Context, page, pageSize int) service.AffiliateRecordFilter {
	filter := service.AffiliateRecordFilter{
		Search:   c.Query("search"),
		Page:     page,
		PageSize: pageSize,
		SortBy:   c.Query("sort_by"),
		SortDesc: c.Query("sort_order") != "asc",
REDACTED
	if filter.PageSize > 100 {
		filter.PageSize = 100
REDACTED
	userTZ := c.Query("timezone")
	if t := parseAffiliateRecordStartTime(c.Query("start_at"), userTZ); t != nil {
		filter.StartAt = t
REDACTED
	if t := parseAffiliateRecordEndTime(c.Query("end_at"), userTZ); t != nil {
		filter.EndAt = t
REDACTED
	return filter
REDACTED

func parseAffiliateRecordStartTime(raw string, userTZ string) *time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
REDACTED
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return &parsed
REDACTED
	if parsed, err := timezone.ParseInUserLocation("2006-01-02", raw, userTZ); err == nil {
		return &parsed
REDACTED
	return nil
REDACTED

func parseAffiliateRecordEndTime(raw string, userTZ string) *time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
REDACTED
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return &parsed
REDACTED
	if parsed, err := timezone.ParseInUserLocation("2006-01-02", raw, userTZ); err == nil {
		end := parsed.AddDate(0, 0, 1).Add(-time.Nanosecond)
		return &end
REDACTED
	return nil
REDACTED
