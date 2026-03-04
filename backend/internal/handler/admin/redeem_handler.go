package admin

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// RedeemHandler handles admin redeem code management
type RedeemHandler struct {
	adminService  service.AdminService
	redeemService *service.RedeemService
REDACTED

// NewRedeemHandler creates a new admin redeem handler
func NewRedeemHandler(adminService service.AdminService, redeemService *service.RedeemService) *RedeemHandler {
	return &RedeemHandler{
		adminService:  adminService,
		redeemService: redeemService,
REDACTED
REDACTED

// GenerateRedeemCodesRequest represents generate redeem codes request
type GenerateRedeemCodesRequest struct {
	Count        int     `json:"count" binding:"required,min=1,max=100"`
	Type         string  `json:"type" binding:"required,oneof=balance concurrency subscription invitation"`
	Value        float64 `json:"value" binding:"min=0"`
	GroupID      *int64  `json:"group_id"`                                    // 订阅类型必填
	ValidityDays int     `json:"validity_days" binding:"omitempty,max=36500"` // 订阅类型使用，默认30天，最大100年
REDACTED

// CreateAndRedeemCodeRequest represents creating a fixed code and redeeming it for a target user.
type CreateAndRedeemCodeRequest struct {
	Code   string  `json:"code" binding:"required,min=3,max=128"`
	Type   string  `json:"type" binding:"required,oneof=balance concurrency subscription invitation"`
	Value  float64 `json:"value" binding:"required,gt=0"`
	UserID int64   `json:"user_id" binding:"required,gt=0"`
	Notes  string  `json:"notes"`
REDACTED

// List handles listing all redeem codes with pagination
// GET /api/v1/admin/redeem-codes
func (h *RedeemHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	codeType := c.Query("type")
	status := c.Query("status")
	search := c.Query("search")
	// 标准化和验证 search 参数
	search = strings.TrimSpace(search)
	if len(search) > 100 {
		search = search[:100]
REDACTED

	codes, total, err := h.adminService.ListRedeemCodes(c.Request.Context(), page, pageSize, codeType, status, search)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	out := make([]dto.AdminRedeemCode, 0, len(codes))
	for i := range codes {
		out = append(out, *dto.RedeemCodeFromServiceAdmin(&codes[i]))
REDACTED
	response.Paginated(c, out, total, page, pageSize)
REDACTED

// GetByID handles getting a redeem code by ID
// GET /api/v1/admin/redeem-codes/:id
func (h *RedeemHandler) GetByID(c *gin.Context) {
	codeID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid redeem code ID")
		return
REDACTED

	code, err := h.adminService.GetRedeemCode(c.Request.Context(), codeID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	response.Success(c, dto.RedeemCodeFromServiceAdmin(code))
REDACTED

// Generate handles generating new redeem codes
// POST /api/v1/admin/redeem-codes/generate
func (h *RedeemHandler) Generate(c *gin.Context) {
	var req GenerateRedeemCodesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	executeAdminIdempotentJSON(c, "admin.redeem_codes.generate", req, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		codes, execErr := h.adminService.GenerateRedeemCodes(ctx, &service.GenerateRedeemCodesInput{
			Count:        req.Count,
			Type:         req.Type,
			Value:        req.Value,
			GroupID:      req.GroupID,
			ValidityDays: req.ValidityDays,
	REDACTED)
		if execErr != nil {
			return nil, execErr
	REDACTED

		out := make([]dto.AdminRedeemCode, 0, len(codes))
		for i := range codes {
			out = append(out, *dto.RedeemCodeFromServiceAdmin(&codes[i]))
	REDACTED
		return out, nil
REDACTED)
REDACTED

// CreateAndRedeem creates a fixed redeem code and redeems it for a target user in one step.
// POST /api/v1/admin/redeem-codes/create-and-redeem
func (h *RedeemHandler) CreateAndRedeem(c *gin.Context) {
	if h.redeemService == nil {
		response.InternalError(c, "redeem service not configured")
		return
REDACTED

	var req CreateAndRedeemCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED
	req.Code = strings.TrimSpace(req.Code)

	executeAdminIdempotentJSON(c, "admin.redeem_codes.create_and_redeem", req, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		existing, err := h.redeemService.GetByCode(ctx, req.Code)
		if err == nil {
			return h.resolveCreateAndRedeemExisting(ctx, existing, req.UserID)
	REDACTED
		if !errors.Is(err, service.ErrRedeemCodeNotFound) {
			return nil, err
	REDACTED

		createErr := h.redeemService.CreateCode(ctx, &service.RedeemCode{
			Code:   req.Code,
			Type:   req.Type,
			Value:  req.Value,
			Status: service.StatusUnused,
			Notes:  req.Notes,
	REDACTED)
		if createErr != nil {
			// Unique code race: if code now exists, use idempotent semantics by used_by.
			existingAfterCreateErr, getErr := h.redeemService.GetByCode(ctx, req.Code)
			if getErr == nil {
				return h.resolveCreateAndRedeemExisting(ctx, existingAfterCreateErr, req.UserID)
		REDACTED
			return nil, createErr
	REDACTED

		redeemed, redeemErr := h.redeemService.Redeem(ctx, req.UserID, req.Code)
		if redeemErr != nil {
			return nil, redeemErr
	REDACTED
		return gin.H{"redeem_code": dto.RedeemCodeFromServiceAdmin(redeemed)REDACTED, nil
REDACTED)
REDACTED

func (h *RedeemHandler) resolveCreateAndRedeemExisting(ctx context.Context, existing *service.RedeemCode, userID int64) (any, error) {
	if existing == nil {
		return nil, infraerrors.Conflict("REDEEM_CODE_CONFLICT", "redeem code conflict")
REDACTED

	// If previous run created the code but crashed before redeem, redeem it now.
	if existing.CanUse() {
		redeemed, err := h.redeemService.Redeem(ctx, userID, existing.Code)
		if err == nil {
			return gin.H{"redeem_code": dto.RedeemCodeFromServiceAdmin(redeemed)REDACTED, nil
	REDACTED
		if !errors.Is(err, service.ErrRedeemCodeUsed) {
			return nil, err
	REDACTED
		latest, getErr := h.redeemService.GetByCode(ctx, existing.Code)
		if getErr == nil {
			existing = latest
	REDACTED
REDACTED

	if existing.UsedBy != nil && *existing.UsedBy == userID {
		return gin.H{"redeem_code": dto.RedeemCodeFromServiceAdmin(existing)REDACTED, nil
REDACTED

	return nil, infraerrors.Conflict("REDEEM_CODE_CONFLICT", "redeem code already used by another user")
REDACTED

// Delete handles deleting a redeem code
// DELETE /api/v1/admin/redeem-codes/:id
func (h *RedeemHandler) Delete(c *gin.Context) {
	codeID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid redeem code ID")
		return
REDACTED

	err = h.adminService.DeleteRedeemCode(c.Request.Context(), codeID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	response.Success(c, gin.H{"message": "Redeem code deleted successfully"REDACTED)
REDACTED

// BatchDelete handles batch deleting redeem codes
// POST /api/v1/admin/redeem-codes/batch-delete
func (h *RedeemHandler) BatchDelete(c *gin.Context) {
	var req struct {
		IDs []int64 `json:"ids" binding:"required,min=1"`
REDACTED
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	deleted, err := h.adminService.BatchDeleteRedeemCodes(c.Request.Context(), req.IDs)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	response.Success(c, gin.H{
		"deleted": deleted,
		"message": "Redeem codes deleted successfully",
REDACTED)
REDACTED

// Expire handles expiring a redeem code
// POST /api/v1/admin/redeem-codes/:id/expire
func (h *RedeemHandler) Expire(c *gin.Context) {
	codeID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid redeem code ID")
		return
REDACTED

	code, err := h.adminService.ExpireRedeemCode(c.Request.Context(), codeID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	response.Success(c, dto.RedeemCodeFromServiceAdmin(code))
REDACTED

// GetStats handles getting redeem code statistics
// GET /api/v1/admin/redeem-codes/stats
func (h *RedeemHandler) GetStats(c *gin.Context) {
	// Return mock data for now
	response.Success(c, gin.H{
		"total_codes":             0,
		"active_codes":            0,
		"used_codes":              0,
		"expired_codes":           0,
		"total_value_distributed": 0.0,
		"by_type": gin.H{
			"balance":     0,
			"concurrency": 0,
			"trial":       0,
	REDACTED,
REDACTED)
REDACTED

// Export handles exporting redeem codes to CSV
// GET /api/v1/admin/redeem-codes/export
func (h *RedeemHandler) Export(c *gin.Context) {
	codeType := c.Query("type")
	status := c.Query("status")

	// Get all codes without pagination (use large page size)
	codes, _, err := h.adminService.ListRedeemCodes(c.Request.Context(), 1, 10000, codeType, status, "")
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	// Create CSV buffer
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	// Write header
	if err := writer.Write([]string{"id", "code", "type", "value", "status", "used_by", "used_by_email", "used_at", "created_at"REDACTED); err != nil {
		response.InternalError(c, "Failed to export redeem codes: "+err.Error())
		return
REDACTED

	// Write data rows
	for _, code := range codes {
		usedBy := ""
		if code.UsedBy != nil {
			usedBy = fmt.Sprintf("%d", *code.UsedBy)
	REDACTED
		usedByEmail := ""
		if code.User != nil {
			usedByEmail = code.User.Email
	REDACTED
		usedAt := ""
		if code.UsedAt != nil {
			usedAt = code.UsedAt.Format("2006-01-02 15:04:05")
	REDACTED
		if err := writer.Write([]string{
			fmt.Sprintf("%d", code.ID),
			code.Code,
			code.Type,
			fmt.Sprintf("%.2f", code.Value),
			code.Status,
			usedBy,
			usedByEmail,
			usedAt,
			code.CreatedAt.Format("2006-01-02 15:04:05"),
	REDACTED); err != nil {
			response.InternalError(c, "Failed to export redeem codes: "+err.Error())
			return
	REDACTED
REDACTED

	writer.Flush()
	if err := writer.Error(); err != nil {
		response.InternalError(c, "Failed to export redeem codes: "+err.Error())
		return
REDACTED

	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", "attachment; filename=redeem_codes.csv")
	c.Data(200, "text/csv", buf.Bytes())
REDACTED
