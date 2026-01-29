package admin

import (
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// SoraAccountHandler Sora 账号扩展管理
// 提供 Sora 扩展表的查询与更新能力。
type SoraAccountHandler struct {
	adminService    service.AdminService
	soraAccountRepo service.SoraAccountRepository
	usageRepo       service.SoraUsageStatRepository
REDACTED

// NewSoraAccountHandler 创建 SoraAccountHandler
func NewSoraAccountHandler(adminService service.AdminService, soraAccountRepo service.SoraAccountRepository, usageRepo service.SoraUsageStatRepository) *SoraAccountHandler {
	return &SoraAccountHandler{
		adminService:    adminService,
		soraAccountRepo: soraAccountRepo,
		usageRepo:       usageRepo,
REDACTED
REDACTED

// SoraAccountUpdateRequest 更新/创建 Sora 账号扩展请求
// 使用指针类型区分未提供与设置为空值。
type SoraAccountUpdateRequest struct {
	AccessToken        *string `json:"access_token"`
	SessionToken       *string `json:"session_token"`
	RefreshToken       *string `json:"refresh_token"`
	ClientID           *string `json:"client_id"`
	Email              *string `json:"email"`
	Username           *string `json:"username"`
	Remark             *string `json:"remark"`
	UseCount           *int    `json:"use_count"`
	PlanType           *string `json:"plan_type"`
	PlanTitle          *string `json:"plan_title"`
	SubscriptionEnd    *int64  `json:"subscription_end"`
	SoraSupported      *bool   `json:"sora_supported"`
	SoraInviteCode     *string `json:"sora_invite_code"`
	SoraRedeemedCount  *int    `json:"sora_redeemed_count"`
	SoraRemainingCount *int    `json:"sora_remaining_count"`
	SoraTotalCount     *int    `json:"sora_total_count"`
	SoraCooldownUntil  *int64  `json:"sora_cooldown_until"`
	CooledUntil        *int64  `json:"cooled_until"`
	ImageEnabled       *bool   `json:"image_enabled"`
	VideoEnabled       *bool   `json:"video_enabled"`
	ImageConcurrency   *int    `json:"image_concurrency"`
	VideoConcurrency   *int    `json:"video_concurrency"`
	IsExpired          *bool   `json:"is_expired"`
REDACTED

// SoraAccountBatchRequest 批量导入请求
// accounts 支持批量 upsert。
type SoraAccountBatchRequest struct {
	Accounts []SoraAccountBatchItem `json:"accounts"`
REDACTED

// SoraAccountBatchItem 批量导入条目
type SoraAccountBatchItem struct {
	AccountID int64 `json:"account_id"`
	SoraAccountUpdateRequest
REDACTED

// SoraAccountBatchResult 批量导入结果
// 仅返回成功/失败数量与明细。
type SoraAccountBatchResult struct {
	Success int                          `json:"success"`
	Failed  int                          `json:"failed"`
	Results []SoraAccountBatchItemResult `json:"results"`
REDACTED

// SoraAccountBatchItemResult 批量导入单条结果
type SoraAccountBatchItemResult struct {
	AccountID int64  `json:"account_id"`
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
REDACTED

// List 获取 Sora 账号扩展列表
// GET /api/v1/admin/sora/accounts
func (h *SoraAccountHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	search := strings.TrimSpace(c.Query("search"))

	accounts, total, err := h.adminService.ListAccounts(c.Request.Context(), page, pageSize, service.PlatformSora, "", "", search)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	accountIDs := make([]int64, 0, len(accounts))
	for i := range accounts {
		accountIDs = append(accountIDs, accounts[i].ID)
REDACTED

	soraMap := map[int64]*service.SoraAccount{REDACTED
	if h.soraAccountRepo != nil {
		soraMap, _ = h.soraAccountRepo.GetByAccountIDs(c.Request.Context(), accountIDs)
REDACTED

	usageMap := map[int64]*service.SoraUsageStat{REDACTED
	if h.usageRepo != nil {
		usageMap, _ = h.usageRepo.GetByAccountIDs(c.Request.Context(), accountIDs)
REDACTED

	result := make([]dto.SoraAccount, 0, len(accounts))
	for i := range accounts {
		acc := accounts[i]
		item := dto.SoraAccountFromService(&acc, soraMap[acc.ID], usageMap[acc.ID])
		if item != nil {
			result = append(result, *item)
	REDACTED
REDACTED

	response.Paginated(c, result, total, page, pageSize)
REDACTED

// Get 获取单个 Sora 账号扩展
// GET /api/v1/admin/sora/accounts/:id
func (h *SoraAccountHandler) Get(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "账号 ID 无效")
		return
REDACTED

	account, err := h.adminService.GetAccount(c.Request.Context(), accountID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	if account.Platform != service.PlatformSora {
		response.BadRequest(c, "账号不是 Sora 平台")
		return
REDACTED

	var soraAcc *service.SoraAccount
	if h.soraAccountRepo != nil {
		soraAcc, _ = h.soraAccountRepo.GetByAccountID(c.Request.Context(), accountID)
REDACTED
	var usage *service.SoraUsageStat
	if h.usageRepo != nil {
		usage, _ = h.usageRepo.GetByAccountID(c.Request.Context(), accountID)
REDACTED

	response.Success(c, dto.SoraAccountFromService(account, soraAcc, usage))
REDACTED

// Upsert 更新或创建 Sora 账号扩展
// PUT /api/v1/admin/sora/accounts/:id
func (h *SoraAccountHandler) Upsert(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "账号 ID 无效")
		return
REDACTED

	var req SoraAccountUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数无效: "+err.Error())
		return
REDACTED

	account, err := h.adminService.GetAccount(c.Request.Context(), accountID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	if account.Platform != service.PlatformSora {
		response.BadRequest(c, "账号不是 Sora 平台")
		return
REDACTED

	updates := buildSoraAccountUpdates(&req)
	if h.soraAccountRepo != nil && len(updates) > 0 {
		if err := h.soraAccountRepo.Upsert(c.Request.Context(), accountID, updates); err != nil {
			response.ErrorFrom(c, err)
			return
	REDACTED
REDACTED

	var soraAcc *service.SoraAccount
	if h.soraAccountRepo != nil {
		soraAcc, _ = h.soraAccountRepo.GetByAccountID(c.Request.Context(), accountID)
REDACTED
	var usage *service.SoraUsageStat
	if h.usageRepo != nil {
		usage, _ = h.usageRepo.GetByAccountID(c.Request.Context(), accountID)
REDACTED

	response.Success(c, dto.SoraAccountFromService(account, soraAcc, usage))
REDACTED

// BatchUpsert 批量导入 Sora 账号扩展
// POST /api/v1/admin/sora/accounts/import
func (h *SoraAccountHandler) BatchUpsert(c *gin.Context) {
	var req SoraAccountBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数无效: "+err.Error())
		return
REDACTED
	if len(req.Accounts) == 0 {
		response.BadRequest(c, "accounts 不能为空")
		return
REDACTED

	ids := make([]int64, 0, len(req.Accounts))
	for _, item := range req.Accounts {
		if item.AccountID > 0 {
			ids = append(ids, item.AccountID)
	REDACTED
REDACTED

	accountMap := make(map[int64]*service.Account, len(ids))
	if len(ids) > 0 {
		accounts, _ := h.adminService.GetAccountsByIDs(c.Request.Context(), ids)
		for _, acc := range accounts {
			if acc != nil {
				accountMap[acc.ID] = acc
		REDACTED
	REDACTED
REDACTED

	result := SoraAccountBatchResult{
		Results: make([]SoraAccountBatchItemResult, 0, len(req.Accounts)),
REDACTED

	for _, item := range req.Accounts {
		entry := SoraAccountBatchItemResult{AccountID: item.AccountIDREDACTED
		acc := accountMap[item.AccountID]
		if acc == nil {
			entry.Error = "账号不存在"
			result.Results = append(result.Results, entry)
			result.Failed++
			continue
	REDACTED
		if acc.Platform != service.PlatformSora {
			entry.Error = "账号不是 Sora 平台"
			result.Results = append(result.Results, entry)
			result.Failed++
			continue
	REDACTED
		updates := buildSoraAccountUpdates(&item.SoraAccountUpdateRequest)
		if h.soraAccountRepo != nil && len(updates) > 0 {
			if err := h.soraAccountRepo.Upsert(c.Request.Context(), item.AccountID, updates); err != nil {
				entry.Error = err.Error()
				result.Results = append(result.Results, entry)
				result.Failed++
				continue
		REDACTED
	REDACTED
		entry.Success = true
		result.Results = append(result.Results, entry)
		result.Success++
REDACTED

	response.Success(c, result)
REDACTED

// ListUsage 获取 Sora 调用统计
// GET /api/v1/admin/sora/usage
func (h *SoraAccountHandler) ListUsage(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	params := pagination.PaginationParams{Page: page, PageSize: pageSizeREDACTED
	if h.usageRepo == nil {
		response.Paginated(c, []dto.SoraUsageStat{REDACTED, 0, page, pageSize)
		return
REDACTED
	stats, paginationResult, err := h.usageRepo.List(c.Request.Context(), params)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	result := make([]dto.SoraUsageStat, 0, len(stats))
	for _, stat := range stats {
		item := dto.SoraUsageStatFromService(stat)
		if item != nil {
			result = append(result, *item)
	REDACTED
REDACTED
	response.Paginated(c, result, paginationResult.Total, paginationResult.Page, paginationResult.PageSize)
REDACTED

func buildSoraAccountUpdates(req *SoraAccountUpdateRequest) map[string]any {
	if req == nil {
		return nil
REDACTED
	updates := make(map[string]any)
	setString := func(key string, value *string) {
		if value == nil {
			return
	REDACTED
		updates[key] = strings.TrimSpace(*value)
REDACTED
	setString("access_token", req.AccessToken)
	setString("session_token", req.SessionToken)
	setString("refresh_token", req.RefreshToken)
	setString("client_id", req.ClientID)
	setString("email", req.Email)
	setString("username", req.Username)
	setString("remark", req.Remark)
	setString("plan_type", req.PlanType)
	setString("plan_title", req.PlanTitle)
	setString("sora_invite_code", req.SoraInviteCode)

	if req.UseCount != nil {
		updates["use_count"] = *req.UseCount
REDACTED
	if req.SoraSupported != nil {
		updates["sora_supported"] = *req.SoraSupported
REDACTED
	if req.SoraRedeemedCount != nil {
		updates["sora_redeemed_count"] = *req.SoraRedeemedCount
REDACTED
	if req.SoraRemainingCount != nil {
		updates["sora_remaining_count"] = *req.SoraRemainingCount
REDACTED
	if req.SoraTotalCount != nil {
		updates["sora_total_count"] = *req.SoraTotalCount
REDACTED
	if req.ImageEnabled != nil {
		updates["image_enabled"] = *req.ImageEnabled
REDACTED
	if req.VideoEnabled != nil {
		updates["video_enabled"] = *req.VideoEnabled
REDACTED
	if req.ImageConcurrency != nil {
		updates["image_concurrency"] = *req.ImageConcurrency
REDACTED
	if req.VideoConcurrency != nil {
		updates["video_concurrency"] = *req.VideoConcurrency
REDACTED
	if req.IsExpired != nil {
		updates["is_expired"] = *req.IsExpired
REDACTED
	if req.SubscriptionEnd != nil && *req.SubscriptionEnd > 0 {
		updates["subscription_end"] = time.Unix(*req.SubscriptionEnd, 0).UTC()
REDACTED
	if req.SoraCooldownUntil != nil && *req.SoraCooldownUntil > 0 {
		updates["sora_cooldown_until"] = time.Unix(*req.SoraCooldownUntil, 0).UTC()
REDACTED
	if req.CooledUntil != nil && *req.CooledUntil > 0 {
		updates["cooled_until"] = time.Unix(*req.CooledUntil, 0).UTC()
REDACTED
	return updates
REDACTED
