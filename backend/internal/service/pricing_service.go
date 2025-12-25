package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

// LiteLLMModelPricing LiteLLM价格数据结构
// 只保留我们需要的字段，使用指针来处理可能缺失的值
type LiteLLMModelPricing struct {
	InputCostPerToken           float64 `json:"input_cost_per_token"`
	OutputCostPerToken          float64 `json:"output_cost_per_token"`
	CacheCreationInputTokenCost float64 `json:"cache_creation_input_token_cost"`
	CacheReadInputTokenCost     float64 `json:"cache_read_input_token_cost"`
	LiteLLMProvider             string  `json:"litellm_provider"`
	Mode                        string  `json:"mode"`
	SupportsPromptCaching       bool    `json:"supports_prompt_caching"`
}

// PricingRemoteClient 远程价格数据获取接口
type PricingRemoteClient interface {
	FetchPricingJSON(ctx context.Context, url string) ([]byte, error)
	FetchHashText(ctx context.Context, url string) (string, error)
}

// LiteLLMRawEntry 用于解析原始JSON数据
type LiteLLMRawEntry struct {
	InputCostPerToken           *float64 `json:"input_cost_per_token"`
	OutputCostPerToken          *float64 `json:"output_cost_per_token"`
	CacheCreationInputTokenCost *float64 `json:"cache_creation_input_token_cost"`
	CacheReadInputTokenCost     *float64 `json:"cache_read_input_token_cost"`
	LiteLLMProvider             string   `json:"litellm_provider"`
	Mode                        string   `json:"mode"`
	SupportsPromptCaching       bool     `json:"supports_prompt_caching"`
}

// PricingService 动态价格服务
type PricingService struct {
	cfg          *config.Config
	remoteClient PricingRemoteClient
	mu           sync.RWMutex
	pricingData  map[string]*LiteLLMModelPricing
	lastUpdated  time.Time
	localHash    string

	// 停止信号
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewPricingService 创建价格服务
func NewPricingService(cfg *config.Config, remoteClient PricingRemoteClient) *PricingService {
	s := &PricingService{
		cfg:          cfg,
		remoteClient: remoteClient,
		pricingData:  make(map[string]*LiteLLMModelPricing),
		stopCh:       make(chan struct{}),
	}
	return s
}

// Initialize 初始化价格服务
func (s *PricingService) Initialize() error {
	// 确保数据目录存在
	if err := os.MkdirAll(s.cfg.Pricing.DataDir, 0755); err != nil {
		log.Printf("[Pricing] Failed to create data directory: %v", err)
	}

	// 首次加载价格数据
	if err := s.checkAndUpdatePricing(); err != nil {
		log.Printf("[Pricing] Initial load failed, using fallback: %v", err)
		if err := s.useFallbackPricing(); err != nil {
			return fmt.Errorf("failed to load pricing data: %w", err)
		}
	}

	// 启动定时更新
	s.startUpdateScheduler()

	log.Printf("[Pricing] Service initialized with %d models", len(s.pricingData))
	return nil
}

// Stop 停止价格服务
func (s *PricingService) Stop() {
	close(s.stopCh)
	s.wg.Wait()
	log.Println("[Pricing] Service stopped")
}

// startUpdateScheduler 启动定时更新调度器
func (s *PricingService) startUpdateScheduler() {
	// 定期检查哈希更新
	hashInterval := time.Duration(s.cfg.Pricing.HashCheckIntervalMinutes) * time.Minute
	if hashInterval < time.Minute {
		hashInterval = 10 * time.Minute
	}

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
				}
			case <-s.stopCh:
				return
			}
		}
	}()

	log.Printf("[Pricing] Update scheduler started (check every %v)", hashInterval)
}

// checkAndUpdatePricing 检查并更新价格数据
func (s *PricingService) checkAndUpdatePricing() error {
	pricingFile := s.getPricingFilePath()

	// 检查本地文件是否存在
	if _, err := os.Stat(pricingFile); os.IsNotExist(err) {
		log.Println("[Pricing] Local pricing file not found, downloading...")
		return s.downloadPricingData()
	}

	// 检查文件是否过期
	info, err := os.Stat(pricingFile)
	if err != nil {
		return s.downloadPricingData()
	}

	fileAge := time.Since(info.ModTime())
	maxAge := time.Duration(s.cfg.Pricing.UpdateIntervalHours) * time.Hour

	if fileAge > maxAge {
		log.Printf("[Pricing] Local file is %v old, updating...", fileAge.Round(time.Hour))
		if err := s.downloadPricingData(); err != nil {
			log.Printf("[Pricing] Download failed, using existing file: %v", err)
		}
	}

	// 加载本地文件
	return s.loadPricingData(pricingFile)
}

// syncWithRemote 与远程同步（基于哈希校验）
func (s *PricingService) syncWithRemote() error {
	pricingFile := s.getPricingFilePath()

	// 计算本地文件哈希
	localHash, err := s.computeFileHash(pricingFile)
	if err != nil {
		log.Printf("[Pricing] Failed to compute local hash: %v", err)
		return s.downloadPricingData()
	}

	// 如果配置了哈希URL，从远程获取哈希进行比对
	if s.cfg.Pricing.HashURL != "" {
		remoteHash, err := s.fetchRemoteHash()
		if err != nil {
			log.Printf("[Pricing] Failed to fetch remote hash: %v", err)
			return nil // 哈希获取失败不影响正常使用
		}

		if remoteHash != localHash {
			log.Println("[Pricing] Remote hash differs, downloading new version...")
			return s.downloadPricingData()
		}
		log.Println("[Pricing] Hash check passed, no update needed")
		return nil
	}

	// 没有哈希URL时，基于时间检查
	info, err := os.Stat(pricingFile)
	if err != nil {
		return s.downloadPricingData()
	}

	fileAge := time.Since(info.ModTime())
	maxAge := time.Duration(s.cfg.Pricing.UpdateIntervalHours) * time.Hour

	if fileAge > maxAge {
		log.Printf("[Pricing] File is %v old, downloading...", fileAge.Round(time.Hour))
		return s.downloadPricingData()
	}

	return nil
}

// downloadPricingData 从远程下载价格数据
func (s *PricingService) downloadPricingData() error {
	log.Printf("[Pricing] Downloading from %s", s.cfg.Pricing.RemoteURL)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	body, err := s.remoteClient.FetchPricingJSON(ctx, s.cfg.Pricing.RemoteURL)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	// 解析JSON数据（使用灵活的解析方式）
	data, err := s.parsePricingData(body)
	if err != nil {
		return fmt.Errorf("parse pricing data: %w", err)
	}

	// 保存到本地文件
	pricingFile := s.getPricingFilePath()
	if err := os.WriteFile(pricingFile, body, 0644); err != nil {
		log.Printf("[Pricing] Failed to save file: %v", err)
	}

	// 保存哈希
	hash := sha256.Sum256(body)
	hashStr := hex.EncodeToString(hash[:])
	hashFile := s.getHashFilePath()
	if err := os.WriteFile(hashFile, []byte(hashStr+"\n"), 0644); err != nil {
		log.Printf("[Pricing] Failed to save hash: %v", err)
	}

	// 更新内存数据
	s.mu.Lock()
	s.pricingData = data
	s.lastUpdated = time.Now()
	s.localHash = hashStr
	s.mu.Unlock()

	log.Printf("[Pricing] Downloaded %d models successfully", len(data))
	return nil
}

// parsePricingData 解析价格数据（处理各种格式）
func (s *PricingService) parsePricingData(body []byte) (map[string]*LiteLLMModelPricing, error) {
	// 首先解析为 map[string]json.RawMessage
	var rawData map[string]json.RawMessage
	if err := json.Unmarshal(body, &rawData); err != nil {
		return nil, fmt.Errorf("parse raw JSON: %w", err)
	}

	result := make(map[string]*LiteLLMModelPricing)
	skipped := 0

	for modelName, rawEntry := range rawData {
		// 跳过 sample_spec 等文档条目
		if modelName == "sample_spec" {
			continue
		}

		// 尝试解析每个条目
		var entry LiteLLMRawEntry
		if err := json.Unmarshal(rawEntry, &entry); err != nil {
			skipped++
			continue
		}

		// 只保留有有效价格的条目
		if entry.InputCostPerToken == nil && entry.OutputCostPerToken == nil {
			continue
		}

		pricing := &LiteLLMModelPricing{
			LiteLLMProvider:       entry.LiteLLMProvider,
			Mode:                  entry.Mode,
			SupportsPromptCaching: entry.SupportsPromptCaching,
		}

		if entry.InputCostPerToken != nil {
			pricing.InputCostPerToken = *entry.InputCostPerToken
		}
		if entry.OutputCostPerToken != nil {
			pricing.OutputCostPerToken = *entry.OutputCostPerToken
		}
		if entry.CacheCreationInputTokenCost != nil {
			pricing.CacheCreationInputTokenCost = *entry.CacheCreationInputTokenCost
		}
		if entry.CacheReadInputTokenCost != nil {
			pricing.CacheReadInputTokenCost = *entry.CacheReadInputTokenCost
		}

		result[modelName] = pricing
	}

	if skipped > 0 {
		log.Printf("[Pricing] Skipped %d invalid entries", skipped)
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no valid pricing entries found")
	}

	return result, nil
}

// loadPricingData 从本地文件加载价格数据
func (s *PricingService) loadPricingData(filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read file failed: %w", err)
	}

	// 使用灵活的解析方式
	pricingData, err := s.parsePricingData(data)
	if err != nil {
		return fmt.Errorf("parse pricing data: %w", err)
	}

	// 计算哈希
	hash := sha256.Sum256(data)
	hashStr := hex.EncodeToString(hash[:])

	s.mu.Lock()
	s.pricingData = pricingData
	s.localHash = hashStr

	info, _ := os.Stat(filePath)
	if info != nil {
		s.lastUpdated = info.ModTime()
	} else {
		s.lastUpdated = time.Now()
	}
	s.mu.Unlock()

	log.Printf("[Pricing] Loaded %d models from %s", len(pricingData), filePath)
	return nil
}

// useFallbackPricing 使用回退价格文件
func (s *PricingService) useFallbackPricing() error {
	fallbackFile := s.cfg.Pricing.FallbackFile

	if _, err := os.Stat(fallbackFile); os.IsNotExist(err) {
		return fmt.Errorf("fallback file not found: %s", fallbackFile)
	}

	log.Printf("[Pricing] Using fallback file: %s", fallbackFile)

	// 复制到数据目录
	data, err := os.ReadFile(fallbackFile)
	if err != nil {
		return fmt.Errorf("read fallback failed: %w", err)
	}

	pricingFile := s.getPricingFilePath()
	if err := os.WriteFile(pricingFile, data, 0644); err != nil {
		log.Printf("[Pricing] Failed to copy fallback: %v", err)
	}

	return s.loadPricingData(fallbackFile)
}

// fetchRemoteHash 从远程获取哈希值
func (s *PricingService) fetchRemoteHash() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return s.remoteClient.FetchHashText(ctx, s.cfg.Pricing.HashURL)
}

// computeFileHash 计算文件哈希
func (s *PricingService) computeFileHash(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

// GetModelPricing 获取模型价格（带模糊匹配）
func (s *PricingService) GetModelPricing(modelName string) *LiteLLMModelPricing {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if modelName == "" {
		return nil
	}

	// 标准化模型名称
	modelLower := strings.ToLower(modelName)

	// 1. 精确匹配
	if pricing, ok := s.pricingData[modelLower]; ok {
		return pricing
	}
	if pricing, ok := s.pricingData[modelName]; ok {
		return pricing
	}

	// 2. 处理常见的模型名称变体
	// claude-opus-4-5-20251101 -> claude-opus-4.5-20251101
	normalized := strings.ReplaceAll(modelLower, "-4-5-", "-4.5-")
	if pricing, ok := s.pricingData[normalized]; ok {
		return pricing
	}

	// 3. 尝试模糊匹配（去掉版本号后缀）
	// claude-opus-4-5-20251101 -> claude-opus-4.5
	baseName := s.extractBaseName(modelLower)
	for key, pricing := range s.pricingData {
		keyBase := s.extractBaseName(strings.ToLower(key))
		if keyBase == baseName {
			return pricing
		}
	}

	// 4. 基于模型系列匹配（Claude）
	if pricing := s.matchByModelFamily(modelLower); pricing != nil {
		return pricing
	}

	// 5. OpenAI 模型回退策略
	if strings.HasPrefix(modelLower, "gpt-") {
		return s.matchOpenAIModel(modelLower)
	}

	return nil
}

// extractBaseName 提取基础模型名称（去掉日期版本号）
func (s *PricingService) extractBaseName(model string) string {
	// 移除日期后缀 (如 -20251101, -20241022)
	parts := strings.Split(model, "-")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		// 跳过看起来像日期的部分（8位数字）
		if len(part) == 8 && isNumeric(part) {
			continue
		}
		// 跳过版本号（如 v1:0）
		if strings.Contains(part, ":") {
			continue
		}
		result = append(result, part)
	}
	return strings.Join(result, "-")
}

// matchByModelFamily 基于模型系列匹配
func (s *PricingService) matchByModelFamily(model string) *LiteLLMModelPricing {
	// Claude模型系列匹配规则
	familyPatterns := map[string][]string{
		"opus-4.5":   {"claude-opus-4.5", "claude-opus-4-5"},
		"opus-4":     {"claude-opus-4", "claude-3-opus"},
		"sonnet-4.5": {"claude-sonnet-4.5", "claude-sonnet-4-5"},
		"sonnet-4":   {"claude-sonnet-4", "claude-3-5-sonnet"},
		"sonnet-3.5": {"claude-3-5-sonnet", "claude-3.5-sonnet"},
		"sonnet-3":   {"claude-3-sonnet"},
		"haiku-3.5":  {"claude-3-5-haiku", "claude-3.5-haiku"},
		"haiku-3":    {"claude-3-haiku"},
	}

	// 确定模型属于哪个系列
	var matchedFamily string
	for family, patterns := range familyPatterns {
		for _, pattern := range patterns {
			if strings.Contains(model, pattern) || strings.Contains(model, strings.ReplaceAll(pattern, "-", "")) {
				matchedFamily = family
				break
			}
		}
		if matchedFamily != "" {
			break
		}
	}

	if matchedFamily == "" {
		// 简单的系列匹配
		if strings.Contains(model, "opus") {
			if strings.Contains(model, "4.5") || strings.Contains(model, "4-5") {
				matchedFamily = "opus-4.5"
			} else {
				matchedFamily = "opus-4"
			}
		} else if strings.Contains(model, "sonnet") {
			if strings.Contains(model, "4.5") || strings.Contains(model, "4-5") {
				matchedFamily = "sonnet-4.5"
			} else if strings.Contains(model, "3-5") || strings.Contains(model, "3.5") {
				matchedFamily = "sonnet-3.5"
			} else {
				matchedFamily = "sonnet-4"
			}
		} else if strings.Contains(model, "haiku") {
			if strings.Contains(model, "3-5") || strings.Contains(model, "3.5") {
				matchedFamily = "haiku-3.5"
			} else {
				matchedFamily = "haiku-3"
			}
		}
	}

	if matchedFamily == "" {
		return nil
	}

	// 在价格数据中查找该系列的模型
	patterns := familyPatterns[matchedFamily]
	for _, pattern := range patterns {
		for key, pricing := range s.pricingData {
			keyLower := strings.ToLower(key)
			if strings.Contains(keyLower, pattern) {
				log.Printf("[Pricing] Fuzzy matched %s -> %s", model, key)
				return pricing
			}
		}
	}

	return nil
}

// matchOpenAIModel OpenAI 模型回退匹配策略
// 回退顺序：
// 1. gpt-5.2-codex -> gpt-5.2（去掉后缀如 -codex, -mini, -max 等）
// 2. gpt-5.2-20251222 -> gpt-5.2（去掉日期版本号）
// 3. 最终回退到 DefaultTestModel (gpt-5.1-codex)
func (s *PricingService) matchOpenAIModel(model string) *LiteLLMModelPricing {
	// 正则匹配日期后缀 (如 -20251222)
	datePattern := regexp.MustCompile(`-\d{8}$`)

	// 尝试的回退变体
	variants := s.generateOpenAIModelVariants(model, datePattern)

	for _, variant := range variants {
		if pricing, ok := s.pricingData[variant]; ok {
			log.Printf("[Pricing] OpenAI fallback matched %s -> %s", model, variant)
			return pricing
		}
	}

	// 最终回退到 DefaultTestModel
	defaultModel := strings.ToLower(openai.DefaultTestModel)
	if pricing, ok := s.pricingData[defaultModel]; ok {
		log.Printf("[Pricing] OpenAI fallback to default model %s -> %s", model, defaultModel)
		return pricing
	}

	return nil
}

// generateOpenAIModelVariants 生成 OpenAI 模型的回退变体列表
func (s *PricingService) generateOpenAIModelVariants(model string, datePattern *regexp.Regexp) []string {
	seen := make(map[string]bool)
	var variants []string

	addVariant := func(v string) {
		if v != model && !seen[v] {
			seen[v] = true
			variants = append(variants, v)
		}
	}

	// 1. 去掉日期版本号: gpt-5.2-20251222 -> gpt-5.2
	withoutDate := datePattern.ReplaceAllString(model, "")
	if withoutDate != model {
		addVariant(withoutDate)
	}

	// 2. 提取基础版本号: gpt-5.2-codex -> gpt-5.2
	// 只匹配纯数字版本号格式 gpt-X 或 gpt-X.Y，不匹配 gpt-4o 这种带字母后缀的
	basePattern := regexp.MustCompile(`^(gpt-\d+(?:\.\d+)?)(?:-|$)`)
	if matches := basePattern.FindStringSubmatch(model); len(matches) > 1 {
		addVariant(matches[1])
	}

	// 3. 同时去掉日期后再提取基础版本号
	if withoutDate != model {
		if matches := basePattern.FindStringSubmatch(withoutDate); len(matches) > 1 {
			addVariant(matches[1])
		}
	}

	return variants
}

// GetStatus 获取服务状态
func (s *PricingService) GetStatus() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return map[string]any{
		"model_count":  len(s.pricingData),
		"last_updated": s.lastUpdated,
		"local_hash":   s.localHash[:min(8, len(s.localHash))],
	}
}

// ForceUpdate 强制更新
func (s *PricingService) ForceUpdate() error {
	return s.downloadPricingData()
}

// getPricingFilePath 获取价格文件路径
func (s *PricingService) getPricingFilePath() string {
	return filepath.Join(s.cfg.Pricing.DataDir, "model_pricing.json")
}

// getHashFilePath 获取哈希文件路径
func (s *PricingService) getHashFilePath() string {
	return filepath.Join(s.cfg.Pricing.DataDir, "model_pricing.sha256")
}

// isNumeric 检查字符串是否为纯数字
func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
