package service

import "time"

type Account struct {
	ID           int64
	Name         string
	Platform     string
	Type         string
	Credentials  map[string]any
	Extra        map[string]any
	ProxyID      *int64
	Concurrency  int
	Priority     int
	Status       string
	ErrorMessage string
	LastUsedAt   *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time

	Schedulable bool

	RateLimitedAt    *time.Time
	RateLimitResetAt *time.Time
	OverloadUntil    *time.Time

	SessionWindowStart  *time.Time
	SessionWindowEnd    *time.Time
	SessionWindowStatus string

	Proxy         *Proxy
	AccountGroups []AccountGroup
	GroupIDs      []int64
	Groups        []*Group
REDACTED

func (a *Account) IsActive() bool {
	return a.Status == StatusActive
REDACTED

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

func (a *Account) CanGetUsage() bool {
	return a.Type == AccountTypeOAuth
REDACTED

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
	if a.Type != AccountTypeApiKey {
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
	return a.IsOpenAI() && a.Type == AccountTypeApiKey
REDACTED

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
	expiresAtStr := a.GetCredential("expires_at")
	if expiresAtStr == "" {
		return nil
REDACTED
	t, err := time.Parse(time.RFC3339, expiresAtStr)
	if err != nil {
		if v, ok := a.Credentials["expires_at"].(float64); ok {
			tt := time.Unix(int64(v), 0)
			return &tt
	REDACTED
		return nil
REDACTED
	return &t
REDACTED

func (a *Account) IsOpenAITokenExpired() bool {
	expiresAt := a.GetOpenAITokenExpiresAt()
	if expiresAt == nil {
		return false
REDACTED
	return time.Now().Add(60 * time.Second).After(*expiresAt)
REDACTED
