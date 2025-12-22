package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
)

// JSONB 用于存储JSONB数据
type JSONB map[string]any

func (j JSONB) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
REDACTED
	return json.Marshal(j)
REDACTED

func (j *JSONB) Scan(value any) error {
	if value == nil {
		*j = nil
		return nil
REDACTED
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
REDACTED
	return json.Unmarshal(bytes, j)
REDACTED

type Account struct {
	ID           int64          `gorm:"primaryKey" json:"id"`
	Name         string         `gorm:"size:100;not null" json:"name"`
	Platform     string         `gorm:"size:50;not null" json:"platform"`           // anthropic/openai/gemini
	Type         string         `gorm:"size:20;not null" json:"type"`               // oauth/apikey
	Credentials  JSONB          `gorm:"type:jsonb;default:'{REDACTED'" json:"credentials"` // 凭证(加密存储)
	Extra        JSONB          `gorm:"type:jsonb;default:'{REDACTED'" json:"extra"`       // 扩展信息
	ProxyID      *int64         `gorm:"index" json:"proxy_id"`
	Concurrency  int            `gorm:"default:3;not null" json:"concurrency"`
	Priority     int            `gorm:"default:50;not null" json:"priority"`           // 1-100，越小越高
	Status       string         `gorm:"size:20;default:active;not null" json:"status"` // active/disabled/error
	ErrorMessage string         `gorm:"type:text" json:"error_message"`
	LastUsedAt   *time.Time     `gorm:"index" json:"last_used_at"`
	CreatedAt    time.Time      `gorm:"not null" json:"created_at"`
	UpdatedAt    time.Time      `gorm:"not null" json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`

	// 调度控制
	Schedulable bool `gorm:"default:true;not null" json:"schedulable"`

	// 限流状态 (429)
	RateLimitedAt    *time.Time `gorm:"index" json:"rate_limited_at"`
	RateLimitResetAt *time.Time `gorm:"index" json:"rate_limit_reset_at"`

	// 过载状态 (529)
	OverloadUntil *time.Time `gorm:"index" json:"overload_until"`

	// 5小时时间窗口
	SessionWindowStart  *time.Time `json:"session_window_start"`
	SessionWindowEnd    *time.Time `json:"session_window_end"`
	SessionWindowStatus string     `gorm:"size:20" json:"session_window_status"` // allowed/allowed_warning/rejected

	// 关联
	Proxy         *Proxy         `gorm:"foreignKey:ProxyID" json:"proxy,omitempty"`
	AccountGroups []AccountGroup `gorm:"foreignKey:AccountID" json:"account_groups,omitempty"`

	// 虚拟字段 (不存储到数据库)
	GroupIDs []int64 `gorm:"-" json:"group_ids,omitempty"`
REDACTED

func (Account) TableName() string {
	return "accounts"
REDACTED

// IsActive 检查是否激活
func (a *Account) IsActive() bool {
	return a.Status == "active"
REDACTED

// IsSchedulable 检查账号是否可调度
func (a *Account) IsSchedulable() bool {
	if !a.IsActive() || !a.Schedulable {
		return false
REDACTED
	now := time.Now()
	if a.OverloadUntil != nil && now.Before(*a.OverloadUntil) {
		return false
REDACTED
	if a.RateLimitResetAt != nil && now.Before(*a.RateLimitResetAt) {
		return false
REDACTED
	return true
REDACTED

// IsRateLimited 检查是否处于限流状态
func (a *Account) IsRateLimited() bool {
	if a.RateLimitResetAt == nil {
		return false
REDACTED
	return time.Now().Before(*a.RateLimitResetAt)
REDACTED

// IsOverloaded 检查是否处于过载状态
func (a *Account) IsOverloaded() bool {
	if a.OverloadUntil == nil {
		return false
REDACTED
	return time.Now().Before(*a.OverloadUntil)
REDACTED

// IsOAuth 检查是否为OAuth类型账号（包括oauth和setup-token）
func (a *Account) IsOAuth() bool {
	return a.Type == AccountTypeOAuth || a.Type == AccountTypeSetupToken
REDACTED

// CanGetUsage 检查账号是否可以获取usage信息（只有oauth类型可以，setup-token没有profile权限）
func (a *Account) CanGetUsage() bool {
	return a.Type == AccountTypeOAuth
REDACTED

// GetCredential 获取凭证字段
func (a *Account) GetCredential(key string) string {
	if a.Credentials == nil {
		return ""
REDACTED
	if v, ok := a.Credentials[key]; ok {
		if s, ok := v.(string); ok {
			return s
	REDACTED
REDACTED
	return ""
REDACTED

// GetModelMapping 获取模型映射配置
// 返回格式: map[请求模型名]实际模型名
func (a *Account) GetModelMapping() map[string]string {
	if a.Credentials == nil {
		return nil
REDACTED
	raw, ok := a.Credentials["model_mapping"]
	if !ok || raw == nil {
		return nil
REDACTED
	// 处理map[string]interface{REDACTED类型
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

// IsModelSupported 检查请求的模型是否被该账号支持
// 如果没有设置模型映射，则支持所有模型
func (a *Account) IsModelSupported(requestedModel string) bool {
	mapping := a.GetModelMapping()
	if len(mapping) == 0 {
		return true // 没有映射配置，支持所有模型
REDACTED
	_, exists := mapping[requestedModel]
	return exists
REDACTED

// GetMappedModel 获取映射后的实际模型名
// 如果没有映射，返回原始模型名
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

// GetBaseURL 获取API基础URL（用于apikey类型账号）
func (a *Account) GetBaseURL() string {
	if a.Type != AccountTypeApiKey {
		return ""
REDACTED
	baseURL := a.GetCredential("base_url")
	if baseURL == "" {
		return "https://api.anthropic.com" // 默认URL
REDACTED
	return baseURL
REDACTED

// GetExtraString 从Extra字段获取字符串值
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

// IsCustomErrorCodesEnabled 检查是否启用自定义错误码功能（仅适用于 apikey 类型）
func (a *Account) IsCustomErrorCodesEnabled() bool {
	if a.Type != AccountTypeApiKey || a.Credentials == nil {
		return false
REDACTED
	if v, ok := a.Credentials["custom_error_codes_enabled"]; ok {
		if enabled, ok := v.(bool); ok {
			return enabled
	REDACTED
REDACTED
	return false
REDACTED

// GetCustomErrorCodes 获取自定义错误码列表
func (a *Account) GetCustomErrorCodes() []int {
	if a.Credentials == nil {
		return nil
REDACTED
	raw, ok := a.Credentials["custom_error_codes"]
	if !ok || raw == nil {
		return nil
REDACTED
	// 处理 []interface{REDACTED 类型（JSON反序列化后的格式）
	if arr, ok := raw.([]any); ok {
		result := make([]int, 0, len(arr))
		for _, v := range arr {
			// JSON 数字默认解析为 float64
			if f, ok := v.(float64); ok {
				result = append(result, int(f))
		REDACTED
	REDACTED
		return result
REDACTED
	return nil
REDACTED

// ShouldHandleErrorCode 检查指定错误码是否应该被处理（停止调度/标记限流等）
// 如果未启用自定义错误码或列表为空，返回 true（使用默认策略）
// 如果启用且列表非空，只有在列表中的错误码才返回 true
func (a *Account) ShouldHandleErrorCode(statusCode int) bool {
	if !a.IsCustomErrorCodesEnabled() {
		return true // 未启用，使用默认策略
REDACTED
	codes := a.GetCustomErrorCodes()
	if len(codes) == 0 {
		return true // 启用但列表为空，fallback到默认策略
REDACTED
	// 检查是否在自定义列表中
	for _, code := range codes {
		if code == statusCode {
			return true
	REDACTED
REDACTED
	return false
REDACTED

// IsInterceptWarmupEnabled 检查是否启用预热请求拦截
// 启用后，标题生成、Warmup等预热请求将返回mock响应，不消耗上游token
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

// =============== OpenAI 相关方法 ===============

// IsOpenAI 检查是否为 OpenAI 平台账号
func (a *Account) IsOpenAI() bool {
	return a.Platform == PlatformOpenAI
REDACTED

// IsAnthropic 检查是否为 Anthropic 平台账号
func (a *Account) IsAnthropic() bool {
	return a.Platform == PlatformAnthropic
REDACTED

// IsOpenAIOAuth 检查是否为 OpenAI OAuth 类型账号
func (a *Account) IsOpenAIOAuth() bool {
	return a.IsOpenAI() && a.Type == AccountTypeOAuth
REDACTED

// IsOpenAIApiKey 检查是否为 OpenAI API Key 类型账号（Response 账号）
func (a *Account) IsOpenAIApiKey() bool {
	return a.IsOpenAI() && a.Type == AccountTypeApiKey
REDACTED

// GetOpenAIBaseURL 获取 OpenAI API 基础 URL
// 对于 API Key 类型账号，从 credentials 中获取 base_url
// 对于 OAuth 类型账号，返回默认的 OpenAI API URL
func (a *Account) GetOpenAIBaseURL() string {
	if !a.IsOpenAI() {
		return ""
REDACTED
	if a.Type == AccountTypeApiKey {
		baseURL := a.GetCredential("base_url")
		if baseURL != "" {
			return baseURL
	REDACTED
REDACTED
	return "https://api.openai.com" // OpenAI 默认 API URL
REDACTED

// GetOpenAIAccessToken 获取 OpenAI 访问令牌
func (a *Account) GetOpenAIAccessToken() string {
	if !a.IsOpenAI() {
		return ""
REDACTED
	return a.GetCredential("access_token")
REDACTED

// GetOpenAIRefreshToken 获取 OpenAI 刷新令牌
func (a *Account) GetOpenAIRefreshToken() string {
	if !a.IsOpenAIOAuth() {
		return ""
REDACTED
	return a.GetCredential("refresh_token")
REDACTED

// GetOpenAIIDToken 获取 OpenAI ID Token（JWT，包含用户信息）
func (a *Account) GetOpenAIIDToken() string {
	if !a.IsOpenAIOAuth() {
		return ""
REDACTED
	return a.GetCredential("id_token")
REDACTED

// GetOpenAIApiKey 获取 OpenAI API Key（用于 Response 账号）
func (a *Account) GetOpenAIApiKey() string {
	if !a.IsOpenAIApiKey() {
		return ""
REDACTED
	return a.GetCredential("api_key")
REDACTED

// GetOpenAIUserAgent 获取 OpenAI 自定义 User-Agent
// 返回空字符串表示透传原始 User-Agent
func (a *Account) GetOpenAIUserAgent() string {
	if !a.IsOpenAI() {
		return ""
REDACTED
	return a.GetCredential("user_agent")
REDACTED

// GetChatGPTAccountID 获取 ChatGPT 账号 ID（从 ID Token 解析）
func (a *Account) GetChatGPTAccountID() string {
	if !a.IsOpenAIOAuth() {
		return ""
REDACTED
	return a.GetCredential("chatgpt_account_id")
REDACTED

// GetChatGPTUserID 获取 ChatGPT 用户 ID（从 ID Token 解析）
func (a *Account) GetChatGPTUserID() string {
	if !a.IsOpenAIOAuth() {
		return ""
REDACTED
	return a.GetCredential("chatgpt_user_id")
REDACTED

// GetOpenAIOrganizationID 获取 OpenAI 组织 ID
func (a *Account) GetOpenAIOrganizationID() string {
	if !a.IsOpenAIOAuth() {
		return ""
REDACTED
	return a.GetCredential("organization_id")
REDACTED

// GetOpenAITokenExpiresAt 获取 OpenAI Token 过期时间
func (a *Account) GetOpenAITokenExpiresAt() *time.Time {
	if !a.IsOpenAIOAuth() {
		return nil
REDACTED
	expiresAtStr := a.GetCredential("expires_at")
	if expiresAtStr == "" {
		return nil
REDACTED
	// 尝试解析时间
	t, err := time.Parse(time.RFC3339, expiresAtStr)
	if err != nil {
		// 尝试解析为 Unix 时间戳
		if v, ok := a.Credentials["expires_at"].(float64); ok {
			t = time.Unix(int64(v), 0)
			return &t
	REDACTED
		return nil
REDACTED
	return &t
REDACTED

// IsOpenAITokenExpired 检查 OpenAI Token 是否过期
func (a *Account) IsOpenAITokenExpired() bool {
	expiresAt := a.GetOpenAITokenExpiresAt()
	if expiresAt == nil {
		return false // 没有过期时间信息，假设未过期
REDACTED
	// 提前 60 秒认为过期，便于刷新
	return time.Now().Add(60 * time.Second).After(*expiresAt)
REDACTED
