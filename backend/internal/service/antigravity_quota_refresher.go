package service

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
)

// AntigravityQuotaRefresher 定时刷新 Antigravity 账户的配额信息
type AntigravityQuotaRefresher struct {
	accountRepo AccountRepository
	proxyRepo   ProxyRepository
	cfg         *config.TokenRefreshConfig

	stopCh chan struct{REDACTED
	wg     sync.WaitGroup
REDACTED

// NewAntigravityQuotaRefresher 创建配额刷新器
func NewAntigravityQuotaRefresher(
	accountRepo AccountRepository,
	proxyRepo ProxyRepository,
	_ *AntigravityOAuthService,
	cfg *config.Config,
) *AntigravityQuotaRefresher {
	return &AntigravityQuotaRefresher{
		accountRepo: accountRepo,
		proxyRepo:   proxyRepo,
		cfg:         &cfg.TokenRefresh,
		stopCh:      make(chan struct{REDACTED),
REDACTED
REDACTED

// Start 启动后台配额刷新服务
func (r *AntigravityQuotaRefresher) Start() {
	if !r.cfg.Enabled {
		log.Println("[AntigravityQuota] Service disabled by configuration")
		return
REDACTED

	r.wg.Add(1)
	go r.refreshLoop()

	log.Printf("[AntigravityQuota] Service started (check every %d minutes)", r.cfg.CheckIntervalMinutes)
REDACTED

// Stop 停止服务
func (r *AntigravityQuotaRefresher) Stop() {
	close(r.stopCh)
	r.wg.Wait()
	log.Println("[AntigravityQuota] Service stopped")
REDACTED

// refreshLoop 刷新循环
func (r *AntigravityQuotaRefresher) refreshLoop() {
	defer r.wg.Done()

	checkInterval := time.Duration(r.cfg.CheckIntervalMinutes) * time.Minute
	if checkInterval < time.Minute {
		checkInterval = 5 * time.Minute
REDACTED

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	// 启动时立即执行一次
	r.processRefresh()

	for {
		select {
		case <-ticker.C:
			r.processRefresh()
		case <-r.stopCh:
			return
	REDACTED
REDACTED
REDACTED

// processRefresh 执行一次刷新
func (r *AntigravityQuotaRefresher) processRefresh() {
	ctx := context.Background()

	// 查询所有 active 的账户，然后过滤 antigravity 平台
	allAccounts, err := r.accountRepo.ListActive(ctx)
	if err != nil {
		log.Printf("[AntigravityQuota] Failed to list accounts: %v", err)
		return
REDACTED

	// 过滤 antigravity 平台账户
	var accounts []Account
	for _, acc := range allAccounts {
		if acc.Platform == PlatformAntigravity {
			accounts = append(accounts, acc)
	REDACTED
REDACTED

	if len(accounts) == 0 {
		return
REDACTED

	refreshed, failed := 0, 0

	for i := range accounts {
		account := &accounts[i]

		if err := r.refreshAccountQuota(ctx, account); err != nil {
			log.Printf("[AntigravityQuota] Account %d (%s) failed: %v", account.ID, account.Name, err)
			failed++
	REDACTED else {
			refreshed++
	REDACTED
REDACTED

	log.Printf("[AntigravityQuota] Cycle complete: total=%d, refreshed=%d, failed=%d",
		len(accounts), refreshed, failed)
REDACTED

// refreshAccountQuota 刷新单个账户的配额
func (r *AntigravityQuotaRefresher) refreshAccountQuota(ctx context.Context, account *Account) error {
	accessToken := account.GetCredential("access_token")
	projectID := account.GetCredential("project_id")

	if accessToken == "" {
		return nil // 没有 access_token，跳过
REDACTED

	// token 过期则跳过，由 TokenRefreshService 负责刷新
	if r.isTokenExpired(account) {
		return nil
REDACTED

	// 获取代理 URL
	var proxyURL string
	if account.ProxyID != nil {
		proxy, err := r.proxyRepo.GetByID(ctx, *account.ProxyID)
		if err == nil && proxy != nil {
			proxyURL = proxy.URL()
	REDACTED
REDACTED

	client := antigravity.NewClient(proxyURL)

	// 获取账户类型（tier）和 project_id
	loadResp, _ := client.LoadCodeAssist(ctx, accessToken)
	if loadResp != nil {
		r.updateAccountTier(account, loadResp)
		// 尝试从 API 获取 project_id
		if projectID == "" && loadResp.CloudAICompanionProject != "" {
			projectID = loadResp.CloudAICompanionProject
			account.Credentials["project_id"] = projectID
	REDACTED
REDACTED

	// 如果仍然没有 project_id，随机生成一个并保存
	if projectID == "" {
		projectID = antigravity.GenerateMockProjectID()
		account.Credentials["project_id"] = projectID
		log.Printf("[AntigravityQuotaRefresher] 为账户 %d 生成随机 project_id: %s", account.ID, projectID)
REDACTED

	// 调用 API 获取配额
	modelsResp, err := client.FetchAvailableModels(ctx, accessToken, projectID)
	if err != nil {
		return err
REDACTED

	// 解析配额数据并更新 extra 字段
	r.updateAccountQuota(account, modelsResp)

	// 保存到数据库
	return r.accountRepo.Update(ctx, account)
REDACTED

// isTokenExpired 检查 token 是否过期
func (r *AntigravityQuotaRefresher) isTokenExpired(account *Account) bool {
	expiresAt := parseAntigravityExpiresAt(account)
	if expiresAt == nil {
		return false
REDACTED

	// 提前 5 分钟认为过期
	return time.Now().Add(5 * time.Minute).After(*expiresAt)
REDACTED

// updateAccountTier 更新账户类型信息
func (r *AntigravityQuotaRefresher) updateAccountTier(account *Account, loadResp *antigravity.LoadCodeAssistResponse) {
	if account.Extra == nil {
		account.Extra = make(map[string]any)
REDACTED

	tier := loadResp.GetTier()
	if tier != "" {
		account.Extra["tier"] = tier
REDACTED

	// 保存不符合条件的原因（如 INELIGIBLE_ACCOUNT）
	if len(loadResp.IneligibleTiers) > 0 && loadResp.IneligibleTiers[0] != nil {
		ineligible := loadResp.IneligibleTiers[0]
		if ineligible.ReasonCode != "" {
			account.Extra["ineligible_reason_code"] = ineligible.ReasonCode
	REDACTED
		if ineligible.ReasonMessage != "" {
			account.Extra["ineligible_reason_message"] = ineligible.ReasonMessage
	REDACTED
REDACTED
REDACTED

// updateAccountQuota 更新账户的配额信息
func (r *AntigravityQuotaRefresher) updateAccountQuota(account *Account, modelsResp *antigravity.FetchAvailableModelsResponse) {
	if account.Extra == nil {
		account.Extra = make(map[string]any)
REDACTED

	quota := make(map[string]any)

	for modelName, modelInfo := range modelsResp.Models {
		if modelInfo.QuotaInfo == nil {
			continue
	REDACTED

		// 转换 remainingFraction (0.0-1.0) 为百分比 (0-100)
		remaining := int(modelInfo.QuotaInfo.RemainingFraction * 100)

		quota[modelName] = map[string]any{
			"remaining":  remaining,
			"reset_time": modelInfo.QuotaInfo.ResetTime,
	REDACTED
REDACTED

	account.Extra["quota"] = quota
	account.Extra["last_quota_check"] = time.Now().Format(time.RFC3339)
REDACTED
