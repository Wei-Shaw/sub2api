// Package service provides business logic and domain services for the application.
package service

import (
	"encoding/json"
	"hash/fnv"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/domain"
)

type Account struct {
	ID          int64
	Name        string
	Notes       *string
	Platform    string
	Type        string
	Credentials map[string]any
	Extra       map[string]any
	ProxyID     *int64
	Concurrency int
	Priority    int
	// RateMultiplier 账号计费倍率（>=0，允许 0 表示该账号计费为 0）。
	// 使用指针用于兼容旧版本调度缓存（Redis）中缺字段的情况：nil 表示按 1.0 处理。
	RateMultiplier     *float64
	LoadFactor         *int // 调度负载因子；nil 表示使用 Concurrency
	Status             string
	ErrorMessage       string
	LastUsedAt         *time.Time
	ExpiresAt          *time.Time
	AutoPauseOnExpired bool
	CreatedAt          time.Time
	UpdatedAt          time.Time

	Schedulable bool

	RateLimitedAt    *time.Time
	RateLimitResetAt *time.Time
	OverloadUntil    *time.Time

	TempUnschedulableUntil  *time.Time
	TempUnschedulableReason string

	SessionWindowStart  *time.Time
	SessionWindowEnd    *time.Time
	SessionWindowStatus string

	Proxy         *Proxy
	AccountGroups []AccountGroup
	GroupIDs      []int64
	Groups        []*Group

	// model_mapping 热路径缓存（非持久化字段）
	modelMappingCache               map[string]string
	modelMappingCacheReady          bool
	modelMappingCacheCredentialsPtr uintptr
	modelMappingCacheRawPtr         uintptr
	modelMappingCacheRawLen         int
	modelMappingCacheRawSig         uint64
REDACTED

type TempUnschedulableRule struct {
	ErrorCode       int      `json:"error_code"`
	Keywords        []string `json:"keywords"`
	DurationMinutes int      `json:"duration_minutes"`
	Description     string   `json:"description"`
REDACTED

func (a *Account) IsActive() bool {
	return a.Status == StatusActive
REDACTED

// BillingRateMultiplier 返回账号计费倍率。
// - nil 表示未配置/旧缓存缺字段，按 1.0 处理
// - 允许 0，表示该账号计费为 0
// - 负数属于非法数据，出于安全考虑按 1.0 处理
func (a *Account) BillingRateMultiplier() float64 {
	if a == nil || a.RateMultiplier == nil {
		return 1.0
REDACTED
	if *a.RateMultiplier < 0 {
		return 1.0
REDACTED
	return *a.RateMultiplier
REDACTED

func (a *Account) EffectiveLoadFactor() int {
	if a == nil {
		return 1
REDACTED
	if a.LoadFactor != nil && *a.LoadFactor > 0 {
		return *a.LoadFactor
REDACTED
	if a.Concurrency > 0 {
		return a.Concurrency
REDACTED
	return 1
REDACTED

func (a *Account) IsSchedulable() bool {
	if !a.IsActive() || !a.Schedulable {
		return false
REDACTED
	now := time.Now()
	if a.AutoPauseOnExpired && a.ExpiresAt != nil && !now.Before(*a.ExpiresAt) {
		return false
REDACTED
	if a.OverloadUntil != nil && now.Before(*a.OverloadUntil) {
		return false
REDACTED
	if a.RateLimitResetAt != nil && now.Before(*a.RateLimitResetAt) {
		return false
REDACTED
	if a.TempUnschedulableUntil != nil && now.Before(*a.TempUnschedulableUntil) {
		return false
REDACTED
	return true
REDACTED

func (a *Account) IsRateLimited() bool {
	if a.RateLimitResetAt == nil {
		return false
REDACTED
	return time.Now().Before(*a.RateLimitResetAt)
REDACTED

func (a *Account) IsOverloaded() bool {
	if a.OverloadUntil == nil {
		return false
REDACTED
	return time.Now().Before(*a.OverloadUntil)
REDACTED

func (a *Account) IsOAuth() bool {
	return a.Type == AccountTypeOAuth || a.Type == AccountTypeSetupToken
REDACTED

func (a *Account) IsGemini() bool {
	return a.Platform == PlatformGemini
REDACTED

func (a *Account) GeminiOAuthType() string {
	if a.Platform != PlatformGemini || a.Type != AccountTypeOAuth {
		return ""
REDACTED
	oauthType := strings.TrimSpace(a.GetCredential("oauth_type"))
	if oauthType == "" && strings.TrimSpace(a.GetCredential("project_id")) != "" {
		return "code_assist"
REDACTED
	return oauthType
REDACTED

func (a *Account) GeminiTierID() string {
	tierID := strings.TrimSpace(a.GetCredential("tier_id"))
	return tierID
REDACTED

func (a *Account) IsGeminiCodeAssist() bool {
	if a.Platform != PlatformGemini || a.Type != AccountTypeOAuth {
		return false
REDACTED
	oauthType := a.GeminiOAuthType()
	if oauthType == "" {
		return strings.TrimSpace(a.GetCredential("project_id")) != ""
REDACTED
	return oauthType == "code_assist"
REDACTED

func (a *Account) CanGetUsage() bool {
	return a.Type == AccountTypeOAuth
REDACTED

func (a *Account) GetCredential(key string) string {
	if a.Credentials == nil {
		return ""
REDACTED
	v, ok := a.Credentials[key]
	if !ok || v == nil {
		return ""
REDACTED

	// 支持多种类型（兼容历史数据中 expires_at 等字段可能是数字或字符串）
	switch val := v.(type) {
	case string:
		return val
	case json.Number:
		// GORM datatypes.JSONMap 使用 UseNumber() 解析，数字类型为 json.Number
		return val.String()
	case float64:
		// JSON 解析后数字默认为 float64
		return strconv.FormatInt(int64(val), 10)
	case int64:
		return strconv.FormatInt(val, 10)
	case int:
		return strconv.Itoa(val)
	default:
		return ""
REDACTED
REDACTED

// GetCredentialAsTime 解析凭证中的时间戳字段，支持多种格式
// 兼容以下格式：
//   - RFC3339 字符串: "2025-01-01T00:00:00Z"
//   - Unix 时间戳字符串: "1735689600"
//   - Unix 时间戳数字: 1735689600 (float64/int64/json.Number)
func (a *Account) GetCredentialAsTime(key string) *time.Time {
	s := a.GetCredential(key)
	if s == "" {
		return nil
REDACTED
	// 尝试 RFC3339 格式
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return &t
REDACTED
	// 尝试 Unix 时间戳（纯数字字符串）
	if ts, err := strconv.ParseInt(s, 10, 64); err == nil {
		t := time.Unix(ts, 0)
		return &t
REDACTED
	return nil
REDACTED

// GetCredentialAsInt64 解析凭证中的 int64 字段
// 用于读取 _token_version 等内部字段
func (a *Account) GetCredentialAsInt64(key string) int64 {
	if a == nil || a.Credentials == nil {
		return 0
REDACTED
	val, ok := a.Credentials[key]
	if !ok || val == nil {
		return 0
REDACTED
	switch v := val.(type) {
	case int64:
		return v
	case float64:
		return int64(v)
	case int:
		return int64(v)
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return i
	REDACTED
	case string:
		if i, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
			return i
	REDACTED
REDACTED
	return 0
REDACTED

func (a *Account) IsTempUnschedulableEnabled() bool {
	if a.Credentials == nil {
		return false
REDACTED
	raw, ok := a.Credentials["temp_unschedulable_enabled"]
	if !ok || raw == nil {
		return false
REDACTED
	enabled, ok := raw.(bool)
	return ok && enabled
REDACTED

func (a *Account) GetTempUnschedulableRules() []TempUnschedulableRule {
	if a.Credentials == nil {
		return nil
REDACTED
	raw, ok := a.Credentials["temp_unschedulable_rules"]
	if !ok || raw == nil {
		return nil
REDACTED

	arr, ok := raw.([]any)
	if !ok {
		return nil
REDACTED

	rules := make([]TempUnschedulableRule, 0, len(arr))
	for _, item := range arr {
		entry, ok := item.(map[string]any)
		if !ok || entry == nil {
			continue
	REDACTED

		rule := TempUnschedulableRule{
			ErrorCode:       parseTempUnschedInt(entry["error_code"]),
			Keywords:        parseTempUnschedStrings(entry["keywords"]),
			DurationMinutes: parseTempUnschedInt(entry["duration_minutes"]),
			Description:     parseTempUnschedString(entry["description"]),
	REDACTED

		if rule.ErrorCode <= 0 || rule.DurationMinutes <= 0 || len(rule.Keywords) == 0 {
			continue
	REDACTED

		rules = append(rules, rule)
REDACTED

	return rules
REDACTED

func parseTempUnschedString(value any) string {
	s, ok := value.(string)
	if !ok {
		return ""
REDACTED
	return strings.TrimSpace(s)
REDACTED

func parseTempUnschedStrings(value any) []string {
	if value == nil {
		return nil
REDACTED

	var raw []string
	switch v := value.(type) {
	case []string:
		raw = v
	case []any:
		raw = make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				raw = append(raw, s)
		REDACTED
	REDACTED
	default:
		return nil
REDACTED

	out := make([]string, 0, len(raw))
	for _, item := range raw {
		s := strings.TrimSpace(item)
		if s != "" {
			out = append(out, s)
	REDACTED
REDACTED
	return out
REDACTED

func normalizeAccountNotes(value *string) *string {
	if value == nil {
		return nil
REDACTED
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
REDACTED
	return &trimmed
REDACTED

func parseTempUnschedInt(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return int(i)
	REDACTED
	case string:
		if i, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return i
	REDACTED
REDACTED
	return 0
REDACTED

func (a *Account) GetModelMapping() map[string]string {
	credentialsPtr := mapPtr(a.Credentials)
	rawMapping, _ := a.Credentials["model_mapping"].(map[string]any)
	rawPtr := mapPtr(rawMapping)
	rawLen := len(rawMapping)
	rawSig := uint64(0)
	rawSigReady := false

	if a.modelMappingCacheReady &&
		a.modelMappingCacheCredentialsPtr == credentialsPtr &&
		a.modelMappingCacheRawPtr == rawPtr &&
		a.modelMappingCacheRawLen == rawLen {
		rawSig = modelMappingSignature(rawMapping)
		rawSigReady = true
		if a.modelMappingCacheRawSig == rawSig {
			return a.modelMappingCache
	REDACTED
REDACTED

	mapping := a.resolveModelMapping(rawMapping)
	if !rawSigReady {
		rawSig = modelMappingSignature(rawMapping)
REDACTED

	a.modelMappingCache = mapping
	a.modelMappingCacheReady = true
	a.modelMappingCacheCredentialsPtr = credentialsPtr
	a.modelMappingCacheRawPtr = rawPtr
	a.modelMappingCacheRawLen = rawLen
	a.modelMappingCacheRawSig = rawSig
	return mapping
REDACTED

func (a *Account) resolveModelMapping(rawMapping map[string]any) map[string]string {
	if a.Credentials == nil {
		// Antigravity 平台使用默认映射
		if a.Platform == domain.PlatformAntigravity {
			return domain.DefaultAntigravityModelMapping
	REDACTED
		return nil
REDACTED
	if len(rawMapping) == 0 {
		// Antigravity 平台使用默认映射
		if a.Platform == domain.PlatformAntigravity {
			return domain.DefaultAntigravityModelMapping
	REDACTED
		return nil
REDACTED

	result := make(map[string]string)
	for k, v := range rawMapping {
		if s, ok := v.(string); ok {
			result[k] = s
	REDACTED
REDACTED
	if len(result) > 0 {
		if a.Platform == domain.PlatformAntigravity {
			ensureAntigravityDefaultPassthroughs(result, []string{
				"gemini-3-flash",
				"gemini-3.1-pro-high",
				"gemini-3.1-pro-low",
		REDACTED)
	REDACTED
		return result
REDACTED

	// Antigravity 平台使用默认映射
	if a.Platform == domain.PlatformAntigravity {
		return domain.DefaultAntigravityModelMapping
REDACTED
	return nil
REDACTED

func mapPtr(m map[string]any) uintptr {
	if m == nil {
		return 0
REDACTED
	return reflect.ValueOf(m).Pointer()
REDACTED

func modelMappingSignature(rawMapping map[string]any) uint64 {
	if len(rawMapping) == 0 {
		return 0
REDACTED
	keys := make([]string, 0, len(rawMapping))
	for k := range rawMapping {
		keys = append(keys, k)
REDACTED
	sort.Strings(keys)

	h := fnv.New64a()
	for _, k := range keys {
		_, _ = h.Write([]byte(k))
		_, _ = h.Write([]byte{0REDACTED)
		if v, ok := rawMapping[k].(string); ok {
			_, _ = h.Write([]byte(v))
	REDACTED else {
			_, _ = h.Write([]byte{1REDACTED)
	REDACTED
		_, _ = h.Write([]byte{0xffREDACTED)
REDACTED
	return h.Sum64()
REDACTED

func ensureAntigravityDefaultPassthrough(mapping map[string]string, model string) {
	if mapping == nil || model == "" {
		return
REDACTED
	if _, exists := mapping[model]; exists {
		return
REDACTED
	for pattern := range mapping {
		if matchWildcard(pattern, model) {
			return
	REDACTED
REDACTED
	mapping[model] = model
REDACTED

func ensureAntigravityDefaultPassthroughs(mapping map[string]string, models []string) {
	for _, model := range models {
		ensureAntigravityDefaultPassthrough(mapping, model)
REDACTED
REDACTED

// IsModelSupported 检查模型是否在 model_mapping 中（支持通配符）
// 如果未配置 mapping，返回 true（允许所有模型）
func (a *Account) IsModelSupported(requestedModel string) bool {
	mapping := a.GetModelMapping()
	if len(mapping) == 0 {
		return true // 无映射 = 允许所有
REDACTED
	// 精确匹配
	if _, exists := mapping[requestedModel]; exists {
		return true
REDACTED
	// 通配符匹配
	for pattern := range mapping {
		if matchWildcard(pattern, requestedModel) {
			return true
	REDACTED
REDACTED
	return false
REDACTED

// GetMappedModel 获取映射后的模型名（支持通配符，最长优先匹配）
// 如果未配置 mapping，返回原始模型名
func (a *Account) GetMappedModel(requestedModel string) string {
	mapping := a.GetModelMapping()
	if len(mapping) == 0 {
		return requestedModel
REDACTED
	// 精确匹配优先
	if mappedModel, exists := mapping[requestedModel]; exists {
		return mappedModel
REDACTED
	// 通配符匹配（最长优先）
	return matchWildcardMapping(mapping, requestedModel)
REDACTED

func (a *Account) GetBaseURL() string {
	if a.Type != AccountTypeAPIKey {
		return ""
REDACTED
	baseURL := a.GetCredential("base_url")
	if baseURL == "" {
		return "https://api.anthropic.com"
REDACTED
	if a.Platform == PlatformAntigravity {
		return strings.TrimRight(baseURL, "/") + "/antigravity"
REDACTED
	return baseURL
REDACTED

// GetGeminiBaseURL 返回 Gemini 兼容端点的 base URL。
// Antigravity 平台的 APIKey 账号自动拼接 /antigravity。
func (a *Account) GetGeminiBaseURL(defaultBaseURL string) string {
	baseURL := strings.TrimSpace(a.GetCredential("base_url"))
	if baseURL == "" {
		return defaultBaseURL
REDACTED
	if a.Platform == PlatformAntigravity && a.Type == AccountTypeAPIKey {
		return strings.TrimRight(baseURL, "/") + "/antigravity"
REDACTED
	return baseURL
REDACTED

func (a *Account) GetExtraString(key string) string {
	if a.Extra == nil {
		return ""
REDACTED
	if v, ok := a.Extra[key]; ok {
		if s, ok := v.(string); ok {
			return s
	REDACTED
REDACTED
	return ""
REDACTED

func (a *Account) GetClaudeUserID() string {
	if v := strings.TrimSpace(a.GetExtraString("claude_user_id")); v != "" {
		return v
REDACTED
	if v := strings.TrimSpace(a.GetExtraString("anthropic_user_id")); v != "" {
		return v
REDACTED
	if v := strings.TrimSpace(a.GetCredential("claude_user_id")); v != "" {
		return v
REDACTED
	if v := strings.TrimSpace(a.GetCredential("anthropic_user_id")); v != "" {
		return v
REDACTED
	return ""
REDACTED

// matchAntigravityWildcard 通配符匹配（仅支持末尾 *）
// 用于 model_mapping 的通配符匹配
func matchAntigravityWildcard(pattern, str string) bool {
	if strings.HasSuffix(pattern, "*") {
		prefix := pattern[:len(pattern)-1]
		return strings.HasPrefix(str, prefix)
REDACTED
	return pattern == str
REDACTED

// matchWildcard 通用通配符匹配（仅支持末尾 *）
// 复用 Antigravity 的通配符逻辑，供其他平台使用
func matchWildcard(pattern, str string) bool {
	return matchAntigravityWildcard(pattern, str)
REDACTED

// matchWildcardMapping 通配符映射匹配（最长优先）
// 如果没有匹配，返回原始字符串
func matchWildcardMapping(mapping map[string]string, requestedModel string) string {
	// 收集所有匹配的 pattern，按长度降序排序（最长优先）
	type patternMatch struct {
		pattern string
		target  string
REDACTED
	var matches []patternMatch

	for pattern, target := range mapping {
		if matchWildcard(pattern, requestedModel) {
			matches = append(matches, patternMatch{pattern, targetREDACTED)
	REDACTED
REDACTED

	if len(matches) == 0 {
		return requestedModel // 无匹配，返回原始模型名
REDACTED

	// 按 pattern 长度降序排序
	sort.Slice(matches, func(i, j int) bool {
		if len(matches[i].pattern) != len(matches[j].pattern) {
			return len(matches[i].pattern) > len(matches[j].pattern)
	REDACTED
		return matches[i].pattern < matches[j].pattern
REDACTED)

	return matches[0].target
REDACTED

func (a *Account) IsCustomErrorCodesEnabled() bool {
	if a.Type != AccountTypeAPIKey || a.Credentials == nil {
		return false
REDACTED
	if v, ok := a.Credentials["custom_error_codes_enabled"]; ok {
		if enabled, ok := v.(bool); ok {
			return enabled
	REDACTED
REDACTED
	return false
REDACTED

// IsPoolMode 检查 API Key 账号是否启用池模式。
// 池模式下，上游错误不标记本地账号状态，而是在同一账号上重试。
func (a *Account) IsPoolMode() bool {
	if a.Type != AccountTypeAPIKey || a.Credentials == nil {
		return false
REDACTED
	if v, ok := a.Credentials["pool_mode"]; ok {
		if enabled, ok := v.(bool); ok {
			return enabled
	REDACTED
REDACTED
	return false
REDACTED

const (
	defaultPoolModeRetryCount = 3
	maxPoolModeRetryCount     = 10
)

// GetPoolModeRetryCount 返回池模式同账号重试次数。
// 未配置或配置非法时回退为默认值 3；小于 0 按 0 处理；过大则截断到 10。
func (a *Account) GetPoolModeRetryCount() int {
	if a == nil || !a.IsPoolMode() || a.Credentials == nil {
		return defaultPoolModeRetryCount
REDACTED
	raw, ok := a.Credentials["pool_mode_retry_count"]
	if !ok || raw == nil {
		return defaultPoolModeRetryCount
REDACTED
	count := parsePoolModeRetryCount(raw)
	if count < 0 {
		return 0
REDACTED
	if count > maxPoolModeRetryCount {
		return maxPoolModeRetryCount
REDACTED
	return count
REDACTED

func parsePoolModeRetryCount(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return int(i)
	REDACTED
	case string:
		if i, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return i
	REDACTED
REDACTED
	return defaultPoolModeRetryCount
REDACTED

// isPoolModeRetryableStatus 池模式下应触发同账号重试的状态码
func isPoolModeRetryableStatus(statusCode int) bool {
	switch statusCode {
	case 401, 403, 429:
		return true
	default:
		return false
REDACTED
REDACTED

func (a *Account) GetCustomErrorCodes() []int {
	if a.Credentials == nil {
		return nil
REDACTED
	raw, ok := a.Credentials["custom_error_codes"]
	if !ok || raw == nil {
		return nil
REDACTED
	if arr, ok := raw.([]any); ok {
		result := make([]int, 0, len(arr))
		for _, v := range arr {
			if f, ok := v.(float64); ok {
				result = append(result, int(f))
		REDACTED
	REDACTED
		return result
REDACTED
	return nil
REDACTED

func (a *Account) ShouldHandleErrorCode(statusCode int) bool {
	if !a.IsCustomErrorCodesEnabled() {
		return true
REDACTED
	codes := a.GetCustomErrorCodes()
	if len(codes) == 0 {
		return true
REDACTED
	for _, code := range codes {
		if code == statusCode {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func (a *Account) IsInterceptWarmupEnabled() bool {
	if a.Credentials == nil {
		return false
REDACTED
	if v, ok := a.Credentials["intercept_warmup_requests"]; ok {
		if enabled, ok := v.(bool); ok {
			return enabled
	REDACTED
REDACTED
	return false
REDACTED

func (a *Account) IsOpenAI() bool {
	return a.Platform == PlatformOpenAI
REDACTED

func (a *Account) IsAnthropic() bool {
	return a.Platform == PlatformAnthropic
REDACTED

func (a *Account) IsOpenAIOAuth() bool {
	return a.IsOpenAI() && a.Type == AccountTypeOAuth
REDACTED

func (a *Account) IsOpenAIApiKey() bool {
	return a.IsOpenAI() && a.Type == AccountTypeAPIKey
REDACTED

func (a *Account) GetOpenAIBaseURL() string {
	if !a.IsOpenAI() {
		return ""
REDACTED
	if a.Type == AccountTypeAPIKey {
		baseURL := a.GetCredential("base_url")
		if baseURL != "" {
			return baseURL
	REDACTED
REDACTED
	return "https://api.openai.com"
REDACTED

func (a *Account) GetOpenAIAccessToken() string {
	if !a.IsOpenAI() {
		return ""
REDACTED
	return a.GetCredential("access_token")
REDACTED

func (a *Account) GetOpenAIRefreshToken() string {
	if !a.IsOpenAIOAuth() {
		return ""
REDACTED
	return a.GetCredential("refresh_token")
REDACTED

func (a *Account) GetOpenAIIDToken() string {
	if !a.IsOpenAIOAuth() {
		return ""
REDACTED
	return a.GetCredential("id_token")
REDACTED

func (a *Account) GetOpenAIApiKey() string {
	if !a.IsOpenAIApiKey() {
		return ""
REDACTED
	return a.GetCredential("api_key")
REDACTED

func (a *Account) GetOpenAIUserAgent() string {
	if !a.IsOpenAI() {
		return ""
REDACTED
	return a.GetCredential("user_agent")
REDACTED

func (a *Account) GetChatGPTAccountID() string {
	if !a.IsOpenAIOAuth() {
		return ""
REDACTED
	return a.GetCredential("chatgpt_account_id")
REDACTED

func (a *Account) GetChatGPTUserID() string {
	if !a.IsOpenAIOAuth() {
		return ""
REDACTED
	return a.GetCredential("chatgpt_user_id")
REDACTED

func (a *Account) GetOpenAIOrganizationID() string {
	if !a.IsOpenAIOAuth() {
		return ""
REDACTED
	return a.GetCredential("organization_id")
REDACTED

func (a *Account) GetOpenAITokenExpiresAt() *time.Time {
	if !a.IsOpenAIOAuth() {
		return nil
REDACTED
	return a.GetCredentialAsTime("expires_at")
REDACTED

func (a *Account) IsOpenAITokenExpired() bool {
	expiresAt := a.GetOpenAITokenExpiresAt()
	if expiresAt == nil {
		return false
REDACTED
	return time.Now().Add(60 * time.Second).After(*expiresAt)
REDACTED

// IsMixedSchedulingEnabled 检查 antigravity 账户是否启用混合调度
// 启用后可参与 anthropic/gemini 分组的账户调度
func (a *Account) IsMixedSchedulingEnabled() bool {
	if a.Platform != PlatformAntigravity {
		return false
REDACTED
	if a.Extra == nil {
		return false
REDACTED
	if v, ok := a.Extra["mixed_scheduling"]; ok {
		if enabled, ok := v.(bool); ok {
			return enabled
	REDACTED
REDACTED
	return false
REDACTED

// IsOpenAIPassthroughEnabled 返回 OpenAI 账号是否启用“自动透传（仅替换认证）”。
//
// 新字段：accounts.extra.openai_passthrough。
// 兼容字段：accounts.extra.openai_oauth_passthrough（历史 OAuth 开关）。
// 字段缺失或类型不正确时，按 false（关闭）处理。
func (a *Account) IsOpenAIPassthroughEnabled() bool {
	if a == nil || !a.IsOpenAI() || a.Extra == nil {
		return false
REDACTED
	if enabled, ok := a.Extra["openai_passthrough"].(bool); ok {
		return enabled
REDACTED
	if enabled, ok := a.Extra["openai_oauth_passthrough"].(bool); ok {
		return enabled
REDACTED
	return false
REDACTED

// IsOpenAIResponsesWebSocketV2Enabled 返回 OpenAI 账号是否开启 Responses WebSocket v2。
//
// 分类型新字段：
// - OAuth 账号：accounts.extra.openai_oauth_responses_websockets_v2_enabled
// - API Key 账号：accounts.extra.openai_apikey_responses_websockets_v2_enabled
//
// 兼容字段：
// - accounts.extra.responses_websockets_v2_enabled
// - accounts.extra.openai_ws_enabled（历史开关）
//
// 优先级：
// 1. 按账号类型读取分类型字段
// 2. 分类型字段缺失时，回退兼容字段
func (a *Account) IsOpenAIResponsesWebSocketV2Enabled() bool {
	if a == nil || !a.IsOpenAI() || a.Extra == nil {
		return false
REDACTED
	if a.IsOpenAIOAuth() {
		if enabled, ok := a.Extra["openai_oauth_responses_websockets_v2_enabled"].(bool); ok {
			return enabled
	REDACTED
REDACTED
	if a.IsOpenAIApiKey() {
		if enabled, ok := a.Extra["openai_apikey_responses_websockets_v2_enabled"].(bool); ok {
			return enabled
	REDACTED
REDACTED
	if enabled, ok := a.Extra["responses_websockets_v2_enabled"].(bool); ok {
		return enabled
REDACTED
	if enabled, ok := a.Extra["openai_ws_enabled"].(bool); ok {
		return enabled
REDACTED
	return false
REDACTED

const (
	OpenAIWSIngressModeOff         = "off"
	OpenAIWSIngressModeShared      = "shared"
	OpenAIWSIngressModeDedicated   = "dedicated"
	OpenAIWSIngressModeCtxPool     = "ctx_pool"
	OpenAIWSIngressModePassthrough = "passthrough"
)

func normalizeOpenAIWSIngressMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case OpenAIWSIngressModeOff:
		return OpenAIWSIngressModeOff
	case OpenAIWSIngressModeCtxPool:
		return OpenAIWSIngressModeCtxPool
	case OpenAIWSIngressModePassthrough:
		return OpenAIWSIngressModePassthrough
	case OpenAIWSIngressModeShared:
		return OpenAIWSIngressModeShared
	case OpenAIWSIngressModeDedicated:
		return OpenAIWSIngressModeDedicated
	default:
		return ""
REDACTED
REDACTED

func normalizeOpenAIWSIngressDefaultMode(mode string) string {
	if normalized := normalizeOpenAIWSIngressMode(mode); normalized != "" {
		if normalized == OpenAIWSIngressModeShared || normalized == OpenAIWSIngressModeDedicated {
			return OpenAIWSIngressModeCtxPool
	REDACTED
		return normalized
REDACTED
	return OpenAIWSIngressModeCtxPool
REDACTED

// ResolveOpenAIResponsesWebSocketV2Mode 返回账号在 WSv2 ingress 下的有效模式（off/ctx_pool/passthrough）。
//
// 优先级：
// 1. 分类型 mode 新字段（string）
// 2. 分类型 enabled 旧字段（bool）
// 3. 兼容 enabled 旧字段（bool）
// 4. defaultMode（非法时回退 ctx_pool）
func (a *Account) ResolveOpenAIResponsesWebSocketV2Mode(defaultMode string) string {
	resolvedDefault := normalizeOpenAIWSIngressDefaultMode(defaultMode)
	if a == nil || !a.IsOpenAI() {
		return OpenAIWSIngressModeOff
REDACTED
	if a.Extra == nil {
		return resolvedDefault
REDACTED

	resolveModeString := func(key string) (string, bool) {
		raw, ok := a.Extra[key]
		if !ok {
			return "", false
	REDACTED
		mode, ok := raw.(string)
		if !ok {
			return "", false
	REDACTED
		normalized := normalizeOpenAIWSIngressMode(mode)
		if normalized == "" {
			return "", false
	REDACTED
		return normalized, true
REDACTED
	resolveBoolMode := func(key string) (string, bool) {
		raw, ok := a.Extra[key]
		if !ok {
			return "", false
	REDACTED
		enabled, ok := raw.(bool)
		if !ok {
			return "", false
	REDACTED
		if enabled {
			return OpenAIWSIngressModeCtxPool, true
	REDACTED
		return OpenAIWSIngressModeOff, true
REDACTED

	if a.IsOpenAIOAuth() {
		if mode, ok := resolveModeString("openai_oauth_responses_websockets_v2_mode"); ok {
			return mode
	REDACTED
		if mode, ok := resolveBoolMode("openai_oauth_responses_websockets_v2_enabled"); ok {
			return mode
	REDACTED
REDACTED
	if a.IsOpenAIApiKey() {
		if mode, ok := resolveModeString("openai_apikey_responses_websockets_v2_mode"); ok {
			return mode
	REDACTED
		if mode, ok := resolveBoolMode("openai_apikey_responses_websockets_v2_enabled"); ok {
			return mode
	REDACTED
REDACTED
	if mode, ok := resolveBoolMode("responses_websockets_v2_enabled"); ok {
		return mode
REDACTED
	if mode, ok := resolveBoolMode("openai_ws_enabled"); ok {
		return mode
REDACTED
	// 兼容旧值：shared/dedicated 语义都归并到 ctx_pool。
	if resolvedDefault == OpenAIWSIngressModeShared || resolvedDefault == OpenAIWSIngressModeDedicated {
		return OpenAIWSIngressModeCtxPool
REDACTED
	return resolvedDefault
REDACTED

// IsOpenAIWSForceHTTPEnabled 返回账号级“强制 HTTP”开关。
// 字段：accounts.extra.openai_ws_force_http。
func (a *Account) IsOpenAIWSForceHTTPEnabled() bool {
	if a == nil || !a.IsOpenAI() || a.Extra == nil {
		return false
REDACTED
	enabled, ok := a.Extra["openai_ws_force_http"].(bool)
	return ok && enabled
REDACTED

// IsOpenAIWSAllowStoreRecoveryEnabled 返回账号级 store 恢复开关。
// 字段：accounts.extra.openai_ws_allow_store_recovery。
func (a *Account) IsOpenAIWSAllowStoreRecoveryEnabled() bool {
	if a == nil || !a.IsOpenAI() || a.Extra == nil {
		return false
REDACTED
	enabled, ok := a.Extra["openai_ws_allow_store_recovery"].(bool)
	return ok && enabled
REDACTED

// IsOpenAIOAuthPassthroughEnabled 兼容旧接口，等价于 OAuth 账号的 IsOpenAIPassthroughEnabled。
func (a *Account) IsOpenAIOAuthPassthroughEnabled() bool {
	return a != nil && a.IsOpenAIOAuth() && a.IsOpenAIPassthroughEnabled()
REDACTED

// IsAnthropicAPIKeyPassthroughEnabled 返回 Anthropic API Key 账号是否启用“自动透传（仅替换认证）”。
// 字段：accounts.extra.anthropic_passthrough。
// 字段缺失或类型不正确时，按 false（关闭）处理。
func (a *Account) IsAnthropicAPIKeyPassthroughEnabled() bool {
	if a == nil || a.Platform != PlatformAnthropic || a.Type != AccountTypeAPIKey || a.Extra == nil {
		return false
REDACTED
	enabled, ok := a.Extra["anthropic_passthrough"].(bool)
	return ok && enabled
REDACTED

// IsCodexCLIOnlyEnabled 返回 OpenAI OAuth 账号是否启用“仅允许 Codex 官方客户端”。
// 字段：accounts.extra.codex_cli_only。
// 字段缺失或类型不正确时，按 false（关闭）处理。
func (a *Account) IsCodexCLIOnlyEnabled() bool {
	if a == nil || !a.IsOpenAIOAuth() || a.Extra == nil {
		return false
REDACTED
	enabled, ok := a.Extra["codex_cli_only"].(bool)
	return ok && enabled
REDACTED

// WindowCostSchedulability 窗口费用调度状态
type WindowCostSchedulability int

const (
	// WindowCostSchedulable 可正常调度
	WindowCostSchedulable WindowCostSchedulability = iota
	// WindowCostStickyOnly 仅允许粘性会话
	WindowCostStickyOnly
	// WindowCostNotSchedulable 完全不可调度
	WindowCostNotSchedulable
)

// IsAnthropicOAuthOrSetupToken 判断是否为 Anthropic OAuth 或 SetupToken 类型账号
// 仅这两类账号支持 5h 窗口额度控制和会话数量控制
func (a *Account) IsAnthropicOAuthOrSetupToken() bool {
	return a.Platform == PlatformAnthropic && (a.Type == AccountTypeOAuth || a.Type == AccountTypeSetupToken)
REDACTED

// IsTLSFingerprintEnabled 检查是否启用 TLS 指纹伪装
// 仅适用于 Anthropic OAuth/SetupToken 类型账号
// 启用后将模拟 Claude Code (Node.js) 客户端的 TLS 握手特征
func (a *Account) IsTLSFingerprintEnabled() bool {
	// 仅支持 Anthropic OAuth/SetupToken 账号
	if !a.IsAnthropicOAuthOrSetupToken() {
		return false
REDACTED
	if a.Extra == nil {
		return false
REDACTED
	if v, ok := a.Extra["enable_tls_fingerprint"]; ok {
		if enabled, ok := v.(bool); ok {
			return enabled
	REDACTED
REDACTED
	return false
REDACTED

// GetUserMsgQueueMode 获取用户消息队列模式
// "serialize" = 串行队列, "throttle" = 软性限速, "" = 未设置（使用全局配置）
func (a *Account) GetUserMsgQueueMode() string {
	if a.Extra == nil {
		return ""
REDACTED
	// 优先读取新字段 user_msg_queue_mode（白名单校验，非法值视为未设置）
	if mode, ok := a.Extra["user_msg_queue_mode"].(string); ok && mode != "" {
		if mode == config.UMQModeSerialize || mode == config.UMQModeThrottle {
			return mode
	REDACTED
		return "" // 非法值 fallback 到全局配置
REDACTED
	// 向后兼容: user_msg_queue_enabled: true → "serialize"
	if enabled, ok := a.Extra["user_msg_queue_enabled"].(bool); ok && enabled {
		return config.UMQModeSerialize
REDACTED
	return ""
REDACTED

// IsSessionIDMaskingEnabled 检查是否启用会话ID伪装
// 仅适用于 Anthropic OAuth/SetupToken 类型账号
// 启用后将在一段时间内（15分钟）固定 metadata.user_id 中的 session ID，
// 使上游认为请求来自同一个会话
func (a *Account) IsSessionIDMaskingEnabled() bool {
	if !a.IsAnthropicOAuthOrSetupToken() {
		return false
REDACTED
	if a.Extra == nil {
		return false
REDACTED
	if v, ok := a.Extra["session_id_masking_enabled"]; ok {
		if enabled, ok := v.(bool); ok {
			return enabled
	REDACTED
REDACTED
	return false
REDACTED

// IsCacheTTLOverrideEnabled 检查是否启用缓存 TTL 强制替换
// 仅适用于 Anthropic OAuth/SetupToken 类型账号
// 启用后将所有 cache creation tokens 归入指定的 TTL 类型（5m 或 1h）
func (a *Account) IsCacheTTLOverrideEnabled() bool {
	if !a.IsAnthropicOAuthOrSetupToken() {
		return false
REDACTED
	if a.Extra == nil {
		return false
REDACTED
	if v, ok := a.Extra["cache_ttl_override_enabled"]; ok {
		if enabled, ok := v.(bool); ok {
			return enabled
	REDACTED
REDACTED
	return false
REDACTED

// GetCacheTTLOverrideTarget 获取缓存 TTL 强制替换的目标类型
// 返回 "5m" 或 "1h"，默认 "5m"
func (a *Account) GetCacheTTLOverrideTarget() string {
	if a.Extra == nil {
		return "5m"
REDACTED
	if v, ok := a.Extra["cache_ttl_override_target"]; ok {
		if target, ok := v.(string); ok && (target == "5m" || target == "1h") {
			return target
	REDACTED
REDACTED
	return "5m"
REDACTED

// GetQuotaLimit 获取 API Key 账号的配额限制（美元）
// 返回 0 表示未启用
func (a *Account) GetQuotaLimit() float64 {
	if a.Extra == nil {
		return 0
REDACTED
	if v, ok := a.Extra["quota_limit"]; ok {
		return parseExtraFloat64(v)
REDACTED
	return 0
REDACTED

// GetQuotaUsed 获取 API Key 账号的已用配额（美元）
func (a *Account) GetQuotaUsed() float64 {
	if a.Extra == nil {
		return 0
REDACTED
	if v, ok := a.Extra["quota_used"]; ok {
		return parseExtraFloat64(v)
REDACTED
	return 0
REDACTED

// IsQuotaExceeded 检查 API Key 账号配额是否已超限
func (a *Account) IsQuotaExceeded() bool {
	limit := a.GetQuotaLimit()
	if limit <= 0 {
		return false
REDACTED
	return a.GetQuotaUsed() >= limit
REDACTED

// GetWindowCostLimit 获取 5h 窗口费用阈值（美元）
// 返回 0 表示未启用
func (a *Account) GetWindowCostLimit() float64 {
	if a.Extra == nil {
		return 0
REDACTED
	if v, ok := a.Extra["window_cost_limit"]; ok {
		return parseExtraFloat64(v)
REDACTED
	return 0
REDACTED

// GetWindowCostStickyReserve 获取粘性会话预留额度（美元）
// 默认值为 10
func (a *Account) GetWindowCostStickyReserve() float64 {
	if a.Extra == nil {
		return 10.0
REDACTED
	if v, ok := a.Extra["window_cost_sticky_reserve"]; ok {
		val := parseExtraFloat64(v)
		if val > 0 {
			return val
	REDACTED
REDACTED
	return 10.0
REDACTED

// GetMaxSessions 获取最大并发会话数
// 返回 0 表示未启用
func (a *Account) GetMaxSessions() int {
	if a.Extra == nil {
		return 0
REDACTED
	if v, ok := a.Extra["max_sessions"]; ok {
		return parseExtraInt(v)
REDACTED
	return 0
REDACTED

// GetSessionIdleTimeoutMinutes 获取会话空闲超时分钟数
// 默认值为 5 分钟
func (a *Account) GetSessionIdleTimeoutMinutes() int {
	if a.Extra == nil {
		return 5
REDACTED
	if v, ok := a.Extra["session_idle_timeout_minutes"]; ok {
		val := parseExtraInt(v)
		if val > 0 {
			return val
	REDACTED
REDACTED
	return 5
REDACTED

// GetBaseRPM 获取基础 RPM 限制
// 返回 0 表示未启用（负数视为无效配置，按 0 处理）
func (a *Account) GetBaseRPM() int {
	if a.Extra == nil {
		return 0
REDACTED
	if v, ok := a.Extra["base_rpm"]; ok {
		val := parseExtraInt(v)
		if val > 0 {
			return val
	REDACTED
REDACTED
	return 0
REDACTED

// GetRPMStrategy 获取 RPM 策略
// "tiered" = 三区模型（默认）, "sticky_exempt" = 粘性豁免
func (a *Account) GetRPMStrategy() string {
	if a.Extra == nil {
		return "tiered"
REDACTED
	if v, ok := a.Extra["rpm_strategy"]; ok {
		if s, ok := v.(string); ok && s == "sticky_exempt" {
			return "sticky_exempt"
	REDACTED
REDACTED
	return "tiered"
REDACTED

// GetRPMStickyBuffer 获取 RPM 粘性缓冲数量
// tiered 模式下的黄区大小，默认为 base_rpm 的 20%（至少 1）
func (a *Account) GetRPMStickyBuffer() int {
	if a.Extra == nil {
		return 0
REDACTED
	if v, ok := a.Extra["rpm_sticky_buffer"]; ok {
		val := parseExtraInt(v)
		if val > 0 {
			return val
	REDACTED
REDACTED
	base := a.GetBaseRPM()
	buffer := base / 5
	if buffer < 1 && base > 0 {
		buffer = 1
REDACTED
	return buffer
REDACTED

// CheckRPMSchedulability 根据当前 RPM 计数检查调度状态
// 复用 WindowCostSchedulability 三态：Schedulable / StickyOnly / NotSchedulable
func (a *Account) CheckRPMSchedulability(currentRPM int) WindowCostSchedulability {
	baseRPM := a.GetBaseRPM()
	if baseRPM <= 0 {
		return WindowCostSchedulable
REDACTED

	if currentRPM < baseRPM {
		return WindowCostSchedulable
REDACTED

	strategy := a.GetRPMStrategy()
	if strategy == "sticky_exempt" {
		return WindowCostStickyOnly // 粘性豁免无红区
REDACTED

	// tiered: 黄区 + 红区
	buffer := a.GetRPMStickyBuffer()
	if currentRPM < baseRPM+buffer {
		return WindowCostStickyOnly
REDACTED
	return WindowCostNotSchedulable
REDACTED

// CheckWindowCostSchedulability 根据当前窗口费用检查调度状态
// - 费用 < 阈值: WindowCostSchedulable（可正常调度）
// - 费用 >= 阈值 且 < 阈值+预留: WindowCostStickyOnly（仅粘性会话）
// - 费用 >= 阈值+预留: WindowCostNotSchedulable（不可调度）
func (a *Account) CheckWindowCostSchedulability(currentWindowCost float64) WindowCostSchedulability {
	limit := a.GetWindowCostLimit()
	if limit <= 0 {
		return WindowCostSchedulable
REDACTED

	if currentWindowCost < limit {
		return WindowCostSchedulable
REDACTED

	stickyReserve := a.GetWindowCostStickyReserve()
	if currentWindowCost < limit+stickyReserve {
		return WindowCostStickyOnly
REDACTED

	return WindowCostNotSchedulable
REDACTED

// GetCurrentWindowStartTime 获取当前有效的窗口开始时间
// 逻辑：
// 1. 如果窗口未过期（SessionWindowEnd 存在且在当前时间之后），使用记录的 SessionWindowStart
// 2. 否则（窗口过期或未设置），使用新的预测窗口开始时间（从当前整点开始）
func (a *Account) GetCurrentWindowStartTime() time.Time {
	now := time.Now()

	// 窗口未过期，使用记录的窗口开始时间
	if a.SessionWindowStart != nil && a.SessionWindowEnd != nil && now.Before(*a.SessionWindowEnd) {
		return *a.SessionWindowStart
REDACTED

	// 窗口已过期或未设置，预测新的窗口开始时间（从当前整点开始）
	// 与 ratelimit_service.go 中 UpdateSessionWindow 的预测逻辑保持一致
	return time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, now.Location())
REDACTED

// parseExtraFloat64 从 extra 字段解析 float64 值
func parseExtraFloat64(value any) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case json.Number:
		if f, err := v.Float64(); err == nil {
			return f
	REDACTED
	case string:
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			return f
	REDACTED
REDACTED
	return 0
REDACTED

// parseExtraInt 从 extra 字段解析 int 值
// ParseExtraInt 从 extra 字段的 any 值解析为 int。
// 支持 int, int64, float64, json.Number, string 类型，无法解析时返回 0。
func ParseExtraInt(value any) int {
	return parseExtraInt(value)
REDACTED

func parseExtraInt(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return int(i)
	REDACTED
	case string:
		if i, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return i
	REDACTED
REDACTED
	return 0
REDACTED
