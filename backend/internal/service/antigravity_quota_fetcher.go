package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
)

// AntigravityQuotaFetcher 从 Antigravity API 获取额度
type AntigravityQuotaFetcher struct {
	proxyRepo ProxyRepository
REDACTED

// NewAntigravityQuotaFetcher 创建 AntigravityQuotaFetcher
func NewAntigravityQuotaFetcher(proxyRepo ProxyRepository) *AntigravityQuotaFetcher {
	return &AntigravityQuotaFetcher{proxyRepo: proxyRepoREDACTED
REDACTED

// CanFetch 检查是否可以获取此账户的额度
func (f *AntigravityQuotaFetcher) CanFetch(account *Account) bool {
	if f == nil || account == nil {
		return false
REDACTED
	if account.Platform != PlatformAntigravity {
		return false
REDACTED
	accessToken := account.GetCredential("access_token")
	return accessToken != ""
REDACTED

// FetchQuota 获取 Antigravity 账户额度信息
func (f *AntigravityQuotaFetcher) FetchQuota(ctx context.Context, account *Account, proxyURL string) (*QuotaResult, error) {
	if f == nil {
		return nil, fmt.Errorf("antigravity quota fetcher is nil")
REDACTED
	if account == nil {
		return nil, fmt.Errorf("account is nil")
REDACTED
	accessToken := account.GetCredential("access_token")
	projectID := account.GetCredential("project_id")

	// 如果没有 project_id，生成一个随机的
	if projectID == "" {
		projectID = antigravity.GenerateMockProjectID()
REDACTED

	client := antigravity.NewClient(proxyURL)

	// 调用 API 获取配额
	modelsResp, modelsRaw, err := client.FetchAvailableModels(ctx, accessToken, projectID)
	if err != nil {
		return nil, err
REDACTED

	// 转换为 UsageInfo
	usageInfo := f.buildUsageInfo(modelsResp)

	return &QuotaResult{
		UsageInfo: usageInfo,
		Raw:       modelsRaw,
REDACTED, nil
REDACTED

// buildUsageInfo 将 API 响应转换为 UsageInfo
func (f *AntigravityQuotaFetcher) buildUsageInfo(modelsResp *antigravity.FetchAvailableModelsResponse) *UsageInfo {
	now := time.Now()
	info := &UsageInfo{
		UpdatedAt:        &now,
		AntigravityQuota: make(map[string]*AntigravityModelQuota),
REDACTED

	if modelsResp == nil {
		return info
REDACTED

	// 遍历所有模型，填充 AntigravityQuota
	for modelName, modelInfo := range modelsResp.Models {
		if modelInfo.QuotaInfo == nil {
			continue
	REDACTED

		// remainingFraction 是剩余比例 (0.0-1.0)，转换为使用率百分比
		utilization := clampInt(int((1.0-modelInfo.QuotaInfo.RemainingFraction)*100), 0, 100)

		info.AntigravityQuota[modelName] = &AntigravityModelQuota{
			Utilization: utilization,
			ResetTime:   modelInfo.QuotaInfo.ResetTime,
	REDACTED
REDACTED

	// 同时设置 FiveHour 用于兼容展示（取主要模型）
	priorityModels := []string{"claude-sonnet-4-20250514", "claude-sonnet-4", "gemini-2.5-pro"REDACTED
	for _, modelName := range priorityModels {
		if modelInfo, ok := modelsResp.Models[modelName]; ok && modelInfo.QuotaInfo != nil {
			utilization := clampFloat64((1.0-modelInfo.QuotaInfo.RemainingFraction)*100, 0, 100)
			progress := &UsageProgress{
				Utilization: utilization,
		REDACTED
			if modelInfo.QuotaInfo.ResetTime != "" {
				if resetTime, err := time.Parse(time.RFC3339, modelInfo.QuotaInfo.ResetTime); err == nil {
					progress.ResetsAt = &resetTime
					progress.RemainingSeconds = remainingSecondsUntil(resetTime)
			REDACTED
		REDACTED
			info.FiveHour = progress
			break
	REDACTED
REDACTED

	return info
REDACTED

// GetProxyURL 获取账户的代理 URL
func (f *AntigravityQuotaFetcher) GetProxyURL(ctx context.Context, account *Account) (string, error) {
	if f == nil {
		return "", fmt.Errorf("antigravity quota fetcher is nil")
REDACTED
	if account == nil {
		return "", fmt.Errorf("account is nil")
REDACTED
	if account.ProxyID == nil || f.proxyRepo == nil {
		return "", nil
REDACTED
	proxy, err := f.proxyRepo.GetByID(ctx, *account.ProxyID)
	if err != nil {
		return "", err
REDACTED
	if proxy == nil {
		return "", nil
REDACTED
	return proxy.URL(), nil
REDACTED
