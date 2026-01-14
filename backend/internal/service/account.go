// Package service provides business logic and domain services for the application.
package service

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
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
	if a.Credentials == nil {
		return nil
REDACTED
	raw, ok := a.Credentials["model_mapping"]
	if !ok || raw == nil {
		return nil
REDACTED
	if m, ok := raw.(map[string]any); ok {
		result := make(map[string]string)
		for k, v := range m {
			if s, ok := v.(string); ok {
				result[k] = s
		REDACTED
	REDACTED
		if len(result) > 0 {
			return result
	REDACTED
REDACTED
	return nil
REDACTED

func (a *Account) IsModelSupported(requestedModel string) bool {
	mapping := a.GetModelMapping()
	if len(mapping) == 0 {
		return true
REDACTED
	_, exists := mapping[requestedModel]
	return exists
REDACTED

func (a *Account) GetMappedModel(requestedModel string) string {
	mapping := a.GetModelMapping()
	if len(mapping) == 0 {
		return requestedModel
REDACTED
	if mappedModel, exists := mapping[requestedModel]; exists {
		return mappedModel
REDACTED
	return requestedModel
REDACTED

func (a *Account) GetBaseURL() string {
	if a.Type != AccountTypeAPIKey {
		return ""
REDACTED
	baseURL := a.GetCredential("base_url")
	if baseURL == "" {
		return "https://api.anthropic.com"
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
