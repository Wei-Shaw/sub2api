package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"sub2api/internal/config"
)

// LiteLLMModelPricing LiteLLM价格数据结构
// 只保留我们需要的字段，使用指针来处理可能缺失的值
type LiteLLMModelPricing struct {
	InputCostPerToken            float64 `json:"input_cost_per_token"`
	OutputCostPerToken           float64 `json:"output_cost_per_token"`
	CacheCreationInputTokenCost  float64 `json:"cache_creation_input_token_cost"`
	CacheReadInputTokenCost      float64 `json:"cache_read_input_token_cost"`
	LiteLLMProvider              string  `json:"litellm_provider"`
	Mode                         string  `json:"mode"`
	SupportsPromptCaching        bool    `json:"supports_prompt_caching"`
REDACTED

// LiteLLMRawEntry 用于解析原始JSON数据
type LiteLLMRawEntry struct {
	InputCostPerToken           *float64 `json:"input_cost_per_token"`
	OutputCostPerToken          *float64 `json:"output_cost_per_token"`
	CacheCreationInputTokenCost *float64 `json:"cache_creation_input_token_cost"`
	CacheReadInputTokenCost     *float64 `json:"cache_read_input_token_cost"`
	LiteLLMProvider             string   `json:"litellm_provider"`
	Mode                        string   `json:"mode"`
	SupportsPromptCaching       bool     `json:"supports_prompt_caching"`
REDACTED

// PricingService 动态价格服务
type PricingService struct {
	cfg         *config.Config
	mu          sync.RWMutex
	pricingData map[string]*LiteLLMModelPricing
	lastUpdated time.Time
	localHash   string

	// 停止信号
	stopCh chan struct{REDACTED
	wg     sync.WaitGroup
REDACTED

// NewPricingService 创建价格服务
func NewPricingService(cfg *config.Config) *PricingService {
	s := &PricingService{
		cfg:         cfg,
		pricingData: make(map[string]*LiteLLMModelPricing),
		stopCh:      make(chan struct{REDACTED),
REDACTED
	return s
REDACTED

// Initialize 初始化价格服务
func (s *PricingService) Initialize() error {
	// 确保数据目录存在
	if err := os.MkdirAll(s.cfg.Pricing.DataDir, 0755); err != nil {
		log.Printf("[Pricing] Failed to create data directory: %v", err)
REDACTED

	// 首次加载价格数据
	if err := s.checkAndUpdatePricing(); err != nil {
		log.Printf("[Pricing] Initial load failed, using fallback: %v", err)
		if err := s.useFallbackPricing(); err != nil {
			return fmt.Errorf("failed to load pricing data: %w", err)
	REDACTED
REDACTED

	// 启动定时更新
	s.startUpdateScheduler()

	log.Printf("[Pricing] Service initialized with %d models", len(s.pricingData))
	return nil
REDACTED

// Stop 停止价格服务
func (s *PricingService) Stop() {
	close(s.stopCh)
	s.wg.Wait()
	log.Println("[Pricing] Service stopped")
REDACTED

// startUpdateScheduler 启动定时更新调度器
func (s *PricingService) startUpdateScheduler() {
	// 定期检查哈希更新
	hashInterval := time.Duration(s.cfg.Pricing.HashCheckIntervalMinutes) * time.Minute
	if hashInterval < time.Minute {
		hashInterval = 10 * time.Minute
REDACTED

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(hashInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := s.syncWithRemote(); err != nil {
					log.Printf("[Pricing] Sync failed: %v", err)
			REDACTED
			case <-s.stopCh:
				return
		REDACTED
	REDACTED
REDACTED()

	log.Printf("[Pricing] Update scheduler started (check every %v)", hashInterval)
REDACTED

// checkAndUpdatePricing 检查并更新价格数据
func (s *PricingService) checkAndUpdatePricing() error {
	pricingFile := s.getPricingFilePath()

	// 检查本地文件是否存在
	if _, err := os.Stat(pricingFile); os.IsNotExist(err) {
		log.Println("[Pricing] Local pricing file not found, downloading...")
		return s.downloadPricingData()
REDACTED

	// 检查文件是否过期
	info, err := os.Stat(pricingFile)
	if err != nil {
		return s.downloadPricingData()
REDACTED

	fileAge := time.Since(info.ModTime())
	maxAge := time.Duration(s.cfg.Pricing.UpdateIntervalHours) * time.Hour

	if fileAge > maxAge {
		log.Printf("[Pricing] Local file is %v old, updating...", fileAge.Round(time.Hour))
		if err := s.downloadPricingData(); err != nil {
			log.Printf("[Pricing] Download failed, using existing file: %v", err)
	REDACTED
REDACTED

	// 加载本地文件
	return s.loadPricingData(pricingFile)
REDACTED

// syncWithRemote 与远程同步（基于哈希校验）
func (s *PricingService) syncWithRemote() error {
	pricingFile := s.getPricingFilePath()

	// 计算本地文件哈希
	localHash, err := s.computeFileHash(pricingFile)
	if err != nil {
		log.Printf("[Pricing] Failed to compute local hash: %v", err)
		return s.downloadPricingData()
REDACTED

	// 如果配置了哈希URL，从远程获取哈希进行比对
	if s.cfg.Pricing.HashURL != "" {
		remoteHash, err := s.fetchRemoteHash()
		if err != nil {
			log.Printf("[Pricing] Failed to fetch remote hash: %v", err)
			return nil // 哈希获取失败不影响正常使用
	REDACTED

		if remoteHash != localHash {
			log.Println("[Pricing] Remote hash differs, downloading new version...")
			return s.downloadPricingData()
	REDACTED
		log.Println("[Pricing] Hash check passed, no update needed")
		return nil
REDACTED

	// 没有哈希URL时，基于时间检查
	info, err := os.Stat(pricingFile)
	if err != nil {
		return s.downloadPricingData()
REDACTED

	fileAge := time.Since(info.ModTime())
	maxAge := time.Duration(s.cfg.Pricing.UpdateIntervalHours) * time.Hour

	if fileAge > maxAge {
		log.Printf("[Pricing] File is %v old, downloading...", fileAge.Round(time.Hour))
		return s.downloadPricingData()
REDACTED

	return nil
REDACTED

// downloadPricingData 从远程下载价格数据
func (s *PricingService) downloadPricingData() error {
	log.Printf("[Pricing] Downloading from %s", s.cfg.Pricing.RemoteURL)

	client := &http.Client{Timeout: 30 * time.SecondREDACTED
	resp, err := client.Get(s.cfg.Pricing.RemoteURL)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
REDACTED
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
REDACTED

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response failed: %w", err)
REDACTED

	// 解析JSON数据（使用灵活的解析方式）
	data, err := s.parsePricingData(body)
	if err != nil {
		return fmt.Errorf("parse pricing data: %w", err)
REDACTED

	// 保存到本地文件
	pricingFile := s.getPricingFilePath()
	if err := os.WriteFile(pricingFile, body, 0644); err != nil {
		log.Printf("[Pricing] Failed to save file: %v", err)
REDACTED

	// 保存哈希
	hash := sha256.Sum256(body)
	hashStr := hex.EncodeToString(hash[:])
	hashFile := s.getHashFilePath()
	if err := os.WriteFile(hashFile, []byte(hashStr+"\n"), 0644); err != nil {
		log.Printf("[Pricing] Failed to save hash: %v", err)
REDACTED

	// 更新内存数据
	s.mu.Lock()
	s.pricingData = data
	s.lastUpdated = time.Now()
	s.localHash = hashStr
	s.mu.Unlock()

	log.Printf("[Pricing] Downloaded %d models successfully", len(data))
	return nil
REDACTED

// parsePricingData 解析价格数据（处理各种格式）
func (s *PricingService) parsePricingData(body []byte) (map[string]*LiteLLMModelPricing, error) {
	// 首先解析为 map[string]json.RawMessage
	var rawData map[string]json.RawMessage
	if err := json.Unmarshal(body, &rawData); err != nil {
		return nil, fmt.Errorf("parse raw JSON: %w", err)
REDACTED

	result := make(map[string]*LiteLLMModelPricing)
	skipped := 0

	for modelName, rawEntry := range rawData {
		// 跳过 sample_spec 等文档条目
		if modelName == "sample_spec" {
			continue
	REDACTED

		// 尝试解析每个条目
		var entry LiteLLMRawEntry
		if err := json.Unmarshal(rawEntry, &entry); err != nil {
			skipped++
			continue
	REDACTED

		// 只保留有有效价格的条目
		if entry.InputCostPerToken == nil && entry.OutputCostPerToken == nil {
			continue
	REDACTED

		pricing := &LiteLLMModelPricing{
			LiteLLMProvider:       entry.LiteLLMProvider,
			Mode:                  entry.Mode,
			SupportsPromptCaching: entry.SupportsPromptCaching,
	REDACTED

		if entry.InputCostPerToken != nil {
			pricing.InputCostPerToken = *entry.InputCostPerToken
	REDACTED
		if entry.OutputCostPerToken != nil {
			pricing.OutputCostPerToken = *entry.OutputCostPerToken
	REDACTED
		if entry.CacheCreationInputTokenCost != nil {
			pricing.CacheCreationInputTokenCost = *entry.CacheCreationInputTokenCost
	REDACTED
		if entry.CacheReadInputTokenCost != nil {
			pricing.CacheReadInputTokenCost = *entry.CacheReadInputTokenCost
	REDACTED

		result[modelName] = pricing
REDACTED

	if skipped > 0 {
		log.Printf("[Pricing] Skipped %d invalid entries", skipped)
REDACTED

	if len(result) == 0 {
		return nil, fmt.Errorf("no valid pricing entries found")
REDACTED

	return result, nil
REDACTED

// loadPricingData 从本地文件加载价格数据
func (s *PricingService) loadPricingData(filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read file failed: %w", err)
REDACTED

	// 使用灵活的解析方式
	pricingData, err := s.parsePricingData(data)
	if err != nil {
		return fmt.Errorf("parse pricing data: %w", err)
REDACTED

	// 计算哈希
	hash := sha256.Sum256(data)
	hashStr := hex.EncodeToString(hash[:])

	s.mu.Lock()
	s.pricingData = pricingData
	s.localHash = hashStr

	info, _ := os.Stat(filePath)
	if info != nil {
		s.lastUpdated = info.ModTime()
REDACTED else {
		s.lastUpdated = time.Now()
REDACTED
	s.mu.Unlock()

	log.Printf("[Pricing] Loaded %d models from %s", len(pricingData), filePath)
	return nil
REDACTED

// useFallbackPricing 使用回退价格文件
func (s *PricingService) useFallbackPricing() error {
	fallbackFile := s.cfg.Pricing.FallbackFile

	if _, err := os.Stat(fallbackFile); os.IsNotExist(err) {
		return fmt.Errorf("fallback file not found: %s", fallbackFile)
REDACTED

	log.Printf("[Pricing] Using fallback file: %s", fallbackFile)

	// 复制到数据目录
	data, err := os.ReadFile(fallbackFile)
	if err != nil {
		return fmt.Errorf("read fallback failed: %w", err)
REDACTED

	pricingFile := s.getPricingFilePath()
	if err := os.WriteFile(pricingFile, data, 0644); err != nil {
		log.Printf("[Pricing] Failed to copy fallback: %v", err)
REDACTED

	return s.loadPricingData(fallbackFile)
REDACTED

// fetchRemoteHash 从远程获取哈希值
func (s *PricingService) fetchRemoteHash() (string, error) {
	client := &http.Client{Timeout: 10 * time.SecondREDACTED
	resp, err := client.Get(s.cfg.Pricing.HashURL)
	if err != nil {
		return "", err
REDACTED
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
REDACTED

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
REDACTED

	// 哈希文件格式：hash  filename 或者纯 hash
	hash := strings.TrimSpace(string(body))
	parts := strings.Fields(hash)
	if len(parts) > 0 {
		return parts[0], nil
REDACTED
	return hash, nil
REDACTED

// computeFileHash 计算文件哈希
func (s *PricingService) computeFileHash(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
REDACTED
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
REDACTED

// GetModelPricing 获取模型价格（带模糊匹配）
func (s *PricingService) GetModelPricing(modelName string) *LiteLLMModelPricing {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if modelName == "" {
		return nil
REDACTED

	// 标准化模型名称
	modelLower := strings.ToLower(modelName)

	// 1. 精确匹配
	if pricing, ok := s.pricingData[modelLower]; ok {
		return pricing
REDACTED
	if pricing, ok := s.pricingData[modelName]; ok {
		return pricing
REDACTED

	// 2. 处理常见的模型名称变体
	// claude-opus-4-5-20251101 -> claude-opus-4.5-20251101
	normalized := strings.ReplaceAll(modelLower, "-4-5-", "-4.5-")
	if pricing, ok := s.pricingData[normalized]; ok {
		return pricing
REDACTED

	// 3. 尝试模糊匹配（去掉版本号后缀）
	// claude-opus-4-5-20251101 -> claude-opus-4.5
	baseName := s.extractBaseName(modelLower)
	for key, pricing := range s.pricingData {
		keyBase := s.extractBaseName(strings.ToLower(key))
		if keyBase == baseName {
			return pricing
	REDACTED
REDACTED

	// 4. 基于模型系列匹配
	return s.matchByModelFamily(modelLower)
REDACTED

// extractBaseName 提取基础模型名称（去掉日期版本号）
func (s *PricingService) extractBaseName(model string) string {
	// 移除日期后缀 (如 -20251101, -20241022)
	parts := strings.Split(model, "-")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		// 跳过看起来像日期的部分（8位数字）
		if len(part) == 8 && isNumeric(part) {
			continue
	REDACTED
		// 跳过版本号（如 v1:0）
		if strings.Contains(part, ":") {
			continue
	REDACTED
		result = append(result, part)
REDACTED
	return strings.Join(result, "-")
REDACTED

// matchByModelFamily 基于模型系列匹配
func (s *PricingService) matchByModelFamily(model string) *LiteLLMModelPricing {
	// Claude模型系列匹配规则
	familyPatterns := map[string][]string{
		"opus-4.5":       {"claude-opus-4.5", "claude-opus-4-5"REDACTED,
		"opus-4":         {"claude-opus-4", "claude-3-opus"REDACTED,
		"sonnet-4.5":     {"claude-sonnet-4.5", "claude-sonnet-4-5"REDACTED,
		"sonnet-4":       {"claude-sonnet-4", "claude-3-5-sonnet"REDACTED,
		"sonnet-3.5":     {"claude-3-5-sonnet", "claude-3.5-sonnet"REDACTED,
		"sonnet-3":       {"claude-3-sonnet"REDACTED,
		"haiku-3.5":      {"claude-3-5-haiku", "claude-3.5-haiku"REDACTED,
		"haiku-3":        {"claude-3-haiku"REDACTED,
REDACTED

	// 确定模型属于哪个系列
	var matchedFamily string
	for family, patterns := range familyPatterns {
		for _, pattern := range patterns {
			if strings.Contains(model, pattern) || strings.Contains(model, strings.ReplaceAll(pattern, "-", "")) {
				matchedFamily = family
				break
		REDACTED
	REDACTED
		if matchedFamily != "" {
			break
	REDACTED
REDACTED

	if matchedFamily == "" {
		// 简单的系列匹配
		if strings.Contains(model, "opus") {
			if strings.Contains(model, "4.5") || strings.Contains(model, "4-5") {
				matchedFamily = "opus-4.5"
		REDACTED else {
				matchedFamily = "opus-4"
		REDACTED
	REDACTED else if strings.Contains(model, "sonnet") {
			if strings.Contains(model, "4.5") || strings.Contains(model, "4-5") {
				matchedFamily = "sonnet-4.5"
		REDACTED else if strings.Contains(model, "3-5") || strings.Contains(model, "3.5") {
				matchedFamily = "sonnet-3.5"
		REDACTED else {
				matchedFamily = "sonnet-4"
		REDACTED
	REDACTED else if strings.Contains(model, "haiku") {
			if strings.Contains(model, "3-5") || strings.Contains(model, "3.5") {
				matchedFamily = "haiku-3.5"
		REDACTED else {
				matchedFamily = "haiku-3"
		REDACTED
	REDACTED
REDACTED

	if matchedFamily == "" {
		return nil
REDACTED

	// 在价格数据中查找该系列的模型
	patterns := familyPatterns[matchedFamily]
	for _, pattern := range patterns {
		for key, pricing := range s.pricingData {
			keyLower := strings.ToLower(key)
			if strings.Contains(keyLower, pattern) {
				log.Printf("[Pricing] Fuzzy matched %s -> %s", model, key)
				return pricing
		REDACTED
	REDACTED
REDACTED

	return nil
REDACTED

// GetStatus 获取服务状态
func (s *PricingService) GetStatus() map[string]interface{REDACTED {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return map[string]interface{REDACTED{
		"model_count":  len(s.pricingData),
		"last_updated": s.lastUpdated,
		"local_hash":   s.localHash[:min(8, len(s.localHash))],
REDACTED
REDACTED

// ForceUpdate 强制更新
func (s *PricingService) ForceUpdate() error {
	return s.downloadPricingData()
REDACTED

// getPricingFilePath 获取价格文件路径
func (s *PricingService) getPricingFilePath() string {
	return filepath.Join(s.cfg.Pricing.DataDir, "model_pricing.json")
REDACTED

// getHashFilePath 获取哈希文件路径
func (s *PricingService) getHashFilePath() string {
	return filepath.Join(s.cfg.Pricing.DataDir, "model_pricing.sha256")
REDACTED

// isNumeric 检查字符串是否为纯数字
func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
	REDACTED
REDACTED
	return true
REDACTED
