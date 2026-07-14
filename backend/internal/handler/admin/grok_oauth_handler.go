package admin

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const grokSSOImportConcurrency = 3

type GrokOAuthHandler struct {
	grokOAuthService *service.GrokOAuthService
	adminService     service.AdminService
	quotaService     *service.GrokQuotaService
REDACTED

func NewGrokOAuthHandler(
	grokOAuthService *service.GrokOAuthService,
	adminService service.AdminService,
	quotaService *service.GrokQuotaService,
) *GrokOAuthHandler {
	return &GrokOAuthHandler{
		grokOAuthService: grokOAuthService,
		adminService:     adminService,
		quotaService:     quotaService,
REDACTED
REDACTED

type GrokGenerateAuthURLRequest struct {
	ProxyID     *int64 `json:"proxy_id"`
	RedirectURI string `json:"redirect_uri"`
REDACTED

func (h *GrokOAuthHandler) GenerateAuthURL(c *gin.Context) {
	var req GrokGenerateAuthURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req = GrokGenerateAuthURLRequest{REDACTED
REDACTED
	result, err := h.grokOAuthService.GenerateAuthURL(c.Request.Context(), req.ProxyID, req.RedirectURI)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, result)
REDACTED

type GrokExchangeCodeRequest struct {
	SessionID   string `json:"session_id" binding:"required"`
	Code        string `json:"code" binding:"required"`
	State       string `json:"state"`
	RedirectURI string `json:"redirect_uri"`
	ProxyID     *int64 `json:"proxy_id"`
REDACTED

func (h *GrokOAuthHandler) ExchangeCode(c *gin.Context) {
	var req GrokExchangeCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED
	tokenInfo, err := h.grokOAuthService.ExchangeCode(c.Request.Context(), &service.GrokExchangeCodeInput{
		SessionID:   req.SessionID,
		Code:        req.Code,
		State:       req.State,
		RedirectURI: req.RedirectURI,
		ProxyID:     req.ProxyID,
REDACTED)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, tokenInfo)
REDACTED

type GrokRefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token"`
	RT           string `json:"rt"`
	ClientID     string `json:"client_id"`
	ProxyID      *int64 `json:"proxy_id"`
REDACTED

func (h *GrokOAuthHandler) RefreshToken(c *gin.Context) {
	var req GrokRefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED
	refreshToken := strings.TrimSpace(req.RefreshToken)
	if refreshToken == "" {
		refreshToken = strings.TrimSpace(req.RT)
REDACTED
	if refreshToken == "" {
		response.BadRequest(c, "refresh_token is required")
		return
REDACTED

	var proxyURL string
	if req.ProxyID != nil {
		proxy, err := h.adminService.GetProxy(c.Request.Context(), *req.ProxyID)
		if err == nil && proxy != nil {
			proxyURL = proxy.URL()
	REDACTED
REDACTED
	tokenInfo, err := h.grokOAuthService.RefreshToken(c.Request.Context(), refreshToken, proxyURL, req.ClientID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, tokenInfo)
REDACTED

func (h *GrokOAuthHandler) RefreshAccountToken(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
REDACTED
	account, err := h.adminService.GetAccount(c.Request.Context(), accountID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	if account.Platform != service.PlatformGrok {
		response.BadRequest(c, "Account platform does not match Grok OAuth endpoint")
		return
REDACTED
	if !account.IsOAuth() {
		response.BadRequest(c, "Cannot refresh non-OAuth account credentials")
		return
REDACTED
	tokenInfo, err := h.grokOAuthService.RefreshAccountToken(c.Request.Context(), account)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	newCredentials := h.grokOAuthService.BuildAccountCredentials(tokenInfo)
	newCredentials = service.MergeCredentials(account.Credentials, newCredentials)
	if baseURL := strings.TrimSpace(account.GetCredential("base_url")); baseURL != "" {
		newCredentials["base_url"] = baseURL
REDACTED
	updatedAccount, err := h.adminService.UpdateAccount(c.Request.Context(), accountID, &service.UpdateAccountInput{
		Credentials: newCredentials,
REDACTED)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, dto.AccountFromService(updatedAccount))
REDACTED

func (h *GrokOAuthHandler) CreateAccountFromOAuth(c *gin.Context) {
	var req struct {
		SessionID   string  `json:"session_id" binding:"required"`
		Code        string  `json:"code" binding:"required"`
		State       string  `json:"state"`
		RedirectURI string  `json:"redirect_uri"`
		ProxyID     *int64  `json:"proxy_id"`
		Name        string  `json:"name"`
		Concurrency int     `json:"concurrency"`
		Priority    int     `json:"priority"`
		GroupIDs    []int64 `json:"group_ids"`
REDACTED
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED
	tokenInfo, err := h.grokOAuthService.ExchangeCode(c.Request.Context(), &service.GrokExchangeCodeInput{
		SessionID:   req.SessionID,
		Code:        req.Code,
		State:       req.State,
		RedirectURI: req.RedirectURI,
		ProxyID:     req.ProxyID,
REDACTED)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	credentials := h.grokOAuthService.BuildAccountCredentials(tokenInfo)

	name := strings.TrimSpace(req.Name)
	if name == "" && tokenInfo.Email != "" {
		name = tokenInfo.Email
REDACTED
	if name == "" {
		name = "Grok OAuth Account"
REDACTED

	account, err := h.adminService.CreateAccount(c.Request.Context(), &service.CreateAccountInput{
		Name:        name,
		Platform:    service.PlatformGrok,
		Type:        service.AccountTypeOAuth,
		Credentials: credentials,
		ProxyID:     req.ProxyID,
		Concurrency: req.Concurrency,
		Priority:    req.Priority,
		GroupIDs:    req.GroupIDs,
REDACTED)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, dto.AccountFromService(account))
REDACTED

type GrokSSOToOAuthRequest struct {
	SSOTokens          []string       `json:"sso_tokens"`
	SSOToken           string         `json:"sso_token"`
	Name               string         `json:"name"`
	Notes              *string        `json:"notes"`
	ProxyID            *int64         `json:"proxy_id"`
	GroupIDs           []int64        `json:"group_ids"`
	Credentials        map[string]any `json:"credentials"`
	Extra              map[string]any `json:"extra"`
	Concurrency        int            `json:"concurrency"`
	LoadFactor         *int           `json:"load_factor"`
	Priority           int            `json:"priority"`
	RateMultiplier     *float64       `json:"rate_multiplier"`
	ExpiresAt          *int64         `json:"expires_at"`
	AutoPauseOnExpired *bool          `json:"auto_pause_on_expired"`
REDACTED

type GrokSSOToOAuthItemResult struct {
	Index   int          `json:"index"`
	Name    string       `json:"name,omitempty"`
	Email   string       `json:"email,omitempty"`
	Account *dto.Account `json:"account,omitempty"`
	Error   string       `json:"error,omitempty"`
REDACTED

type GrokSSOToOAuthResponse struct {
	Created []GrokSSOToOAuthItemResult `json:"created"`
	Failed  []GrokSSOToOAuthItemResult `json:"failed"`
REDACTED

type grokSSOImportJob struct {
	index int
	token string
REDACTED

type grokSSOImportWorkerResult struct {
	created bool
	item    GrokSSOToOAuthItemResult
REDACTED

func (h *GrokOAuthHandler) CreateAccountsFromSSO(c *gin.Context) {
	var req GrokSSOToOAuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED
	tokens := normalizeSSOImportTokens(req.SSOTokens, req.SSOToken)
	if len(tokens) == 0 {
		response.BadRequest(c, "sso_tokens is required")
		return
REDACTED

	ctx := c.Request.Context()
	workerCount := grokSSOImportConcurrency
	if len(tokens) < workerCount {
		workerCount = len(tokens)
REDACTED
	jobs := make(chan grokSSOImportJob)
	items := make([]grokSSOImportWorkerResult, len(tokens))
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				items[job.index] = h.safeCreateAccountFromSSOToken(ctx, req, job.token, job.index+1, len(tokens))
		REDACTED
	REDACTED()
REDACTED
	for i, token := range tokens {
		jobs <- grokSSOImportJob{index: i, token: tokenREDACTED
REDACTED
	close(jobs)
	wg.Wait()

	result := GrokSSOToOAuthResponse{
		Created: make([]GrokSSOToOAuthItemResult, 0, len(tokens)),
		Failed:  make([]GrokSSOToOAuthItemResult, 0),
REDACTED
	for _, item := range items {
		if item.created {
			result.Created = append(result.Created, item.item)
	REDACTED else {
			result.Failed = append(result.Failed, item.item)
	REDACTED
REDACTED
	response.Success(c, result)
REDACTED

func (h *GrokOAuthHandler) safeCreateAccountFromSSOToken(ctx context.Context, req GrokSSOToOAuthRequest, token string, index, total int) (result grokSSOImportWorkerResult) {
	defer func() {
		if recovered := recover(); recovered != nil {
			slog.Error("grok_sso_import_worker_panic", "index", index, "recover", recovered)
			result = grokSSOImportWorkerResult{
				item: GrokSSOToOAuthItemResult{
					Index: index,
					Error: fmt.Sprintf("internal worker panic: %v", recovered),
			REDACTED,
		REDACTED
	REDACTED
REDACTED()
	return h.createAccountFromSSOToken(ctx, req, token, index, total)
REDACTED

func (h *GrokOAuthHandler) createAccountFromSSOToken(ctx context.Context, req GrokSSOToOAuthRequest, token string, index, total int) grokSSOImportWorkerResult {
	tokenInfo, err := h.grokOAuthService.ConvertFromSSO(ctx, token, req.ProxyID)
	if err != nil {
		return grokSSOImportWorkerResult{item: GrokSSOToOAuthItemResult{Index: index, Error: grokSSOImportErrorMessage(err)REDACTEDREDACTED
REDACTED

	credentials := h.grokOAuthService.BuildAccountCredentials(tokenInfo)
	credentials = service.MergeCredentials(cloneGrokSSOMap(req.Credentials), credentials)
	name := grokSSOImportAccountName(req.Name, tokenInfo, index, total)
	expiresAt, autoPauseOnExpired := grokSSOImportExpiry(req.ExpiresAt, req.AutoPauseOnExpired, tokenInfo)
	account, err := h.adminService.CreateAccount(ctx, &service.CreateAccountInput{
		Name:               name,
		Notes:              req.Notes,
		Platform:           service.PlatformGrok,
		Type:               service.AccountTypeOAuth,
		Credentials:        credentials,
		Extra:              cloneGrokSSOMap(req.Extra),
		ProxyID:            req.ProxyID,
		Concurrency:        req.Concurrency,
		LoadFactor:         req.LoadFactor,
		Priority:           req.Priority,
		RateMultiplier:     req.RateMultiplier,
		GroupIDs:           append([]int64(nil), req.GroupIDs...),
		ExpiresAt:          expiresAt,
		AutoPauseOnExpired: autoPauseOnExpired,
REDACTED)
	if err != nil {
		return grokSSOImportWorkerResult{item: GrokSSOToOAuthItemResult{Index: index, Name: name, Email: tokenInfo.Email, Error: grokSSOImportErrorMessage(err)REDACTEDREDACTED
REDACTED
	return grokSSOImportWorkerResult{
		created: true,
		item: GrokSSOToOAuthItemResult{
			Index:   index,
			Name:    name,
			Email:   tokenInfo.Email,
			Account: dto.AccountFromService(account),
	REDACTED,
REDACTED
REDACTED

func grokSSOImportExpiry(requestExpiresAt *int64, requestAutoPause *bool, tokenInfo *service.GrokTokenInfo) (*int64, *bool) {
	if tokenInfo == nil || strings.TrimSpace(tokenInfo.RefreshToken) != "" || tokenInfo.ExpiresAt <= 0 {
		return requestExpiresAt, requestAutoPause
REDACTED

	expiresAt := tokenInfo.ExpiresAt
	if requestExpiresAt != nil && *requestExpiresAt > 0 && *requestExpiresAt < expiresAt {
		expiresAt = *requestExpiresAt
REDACTED
	autoPause := true
	return &expiresAt, &autoPause
REDACTED

func cloneGrokSSOMap(source map[string]any) map[string]any {
	if source == nil {
		return nil
REDACTED
	clone := make(map[string]any, len(source))
	for key, value := range source {
		clone[key] = cloneGrokSSOValue(value)
REDACTED
	return clone
REDACTED

func cloneGrokSSOValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		return cloneGrokSSOMap(v)
	case []any:
		clone := make([]any, len(v))
		for i, item := range v {
			clone[i] = cloneGrokSSOValue(item)
	REDACTED
		return clone
	default:
		return value
REDACTED
REDACTED

func normalizeSSOImportTokens(tokens []string, single string) []string {
	items := make([]string, 0, len(tokens)+1)
	if strings.TrimSpace(single) != "" {
		items = append(items, single)
REDACTED
	items = append(items, tokens...)
	seen := make(map[string]struct{REDACTED, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		parts := strings.Split(strings.NewReplacer(",", "\n", "\r", "\n").Replace(item), "\n")
		for _, token := range parts {
			if token = xai.NormalizeSSOToken(token); token == "" {
				continue
		REDACTED
			if _, ok := seen[token]; ok {
				continue
		REDACTED
			seen[token] = struct{REDACTED{REDACTED
			result = append(result, token)
	REDACTED
REDACTED
	return result
REDACTED

func grokSSOImportAccountName(base string, tokenInfo *service.GrokTokenInfo, index, total int) string {
	base = strings.TrimSpace(base)
	if base == "" && tokenInfo != nil {
		base = strings.TrimSpace(tokenInfo.Email)
REDACTED
	if base == "" {
		base = "Grok OAuth Account"
REDACTED
	if total > 1 {
		return base + " #" + strconv.Itoa(index)
REDACTED
	return base
REDACTED

func grokSSOImportErrorMessage(err error) string {
	status := infraerrors.FromError(err)
	if status == nil {
		return ""
REDACTED
	if status.Reason != "" {
		return status.Reason + ": " + status.Message
REDACTED
	return status.Message
REDACTED

func (h *GrokOAuthHandler) QueryQuota(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
REDACTED
	if h.quotaService == nil {
		response.BadRequest(c, "grok quota service is not enabled")
		return
REDACTED
	result, err := h.quotaService.QueryQuota(c.Request.Context(), accountID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, result)
REDACTED

func (h *GrokOAuthHandler) ResetQuota(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
REDACTED
	if h.quotaService == nil {
		response.BadRequest(c, "grok quota service is not enabled")
		return
REDACTED
	result, err := h.quotaService.ResetQuota(c.Request.Context(), accountID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, result)
REDACTED

func (h *GrokOAuthHandler) RuntimeSanity(c *gin.Context) {
	response.Success(c, xai.RuntimeSanity())
REDACTED
