package dto

import "time"

type User struct {
	ID            int64     `json:"id"`
	Email         string    `json:"email"`
	Username      string    `json:"username"`
	Wechat        string    `json:"wechat"`
	Notes         string    `json:"notes"`
	Role          string    `json:"role"`
	Balance       float64   `json:"balance"`
	Concurrency   int       `json:"concurrency"`
	Status        string    `json:"status"`
	AllowedGroups []int64   `json:"allowed_groups"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`

	ApiKeys       []ApiKey           `json:"api_keys,omitempty"`
	Subscriptions []UserSubscription `json:"subscriptions,omitempty"`
REDACTED

type ApiKey struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Key       string    `json:"key"`
	Name      string    `json:"name"`
	GroupID   *int64    `json:"group_id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	User  *User  `json:"user,omitempty"`
	Group *Group `json:"group,omitempty"`
REDACTED

type Group struct {
	ID             int64   `json:"id"`
	Name           string  `json:"name"`
	Description    string  `json:"description"`
	Platform       string  `json:"platform"`
	RateMultiplier float64 `json:"rate_multiplier"`
	IsExclusive    bool    `json:"is_exclusive"`
	Status         string  `json:"status"`

	SubscriptionType string   `json:"subscription_type"`
	DailyLimitUSD    *float64 `json:"daily_limit_usd"`
	WeeklyLimitUSD   *float64 `json:"weekly_limit_usd"`
	MonthlyLimitUSD  *float64 `json:"monthly_limit_usd"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	AccountGroups []AccountGroup `json:"account_groups,omitempty"`
	AccountCount  int64          `json:"account_count,omitempty"`
REDACTED

type Account struct {
	ID           int64          `json:"id"`
	Name         string         `json:"name"`
	Platform     string         `json:"platform"`
	Type         string         `json:"type"`
	Credentials  map[string]any `json:"credentials"`
	Extra        map[string]any `json:"extra"`
	ProxyID      *int64         `json:"proxy_id"`
	Concurrency  int            `json:"concurrency"`
	Priority     int            `json:"priority"`
	Status       string         `json:"status"`
	ErrorMessage string         `json:"error_message"`
	LastUsedAt   *time.Time     `json:"last_used_at"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`

	Schedulable bool `json:"schedulable"`

	RateLimitedAt    *time.Time `json:"rate_limited_at"`
	RateLimitResetAt *time.Time `json:"rate_limit_reset_at"`
	OverloadUntil    *time.Time `json:"overload_until"`

	SessionWindowStart  *time.Time `json:"session_window_start"`
	SessionWindowEnd    *time.Time `json:"session_window_end"`
	SessionWindowStatus string     `json:"session_window_status"`

	Proxy         *Proxy         `json:"proxy,omitempty"`
	AccountGroups []AccountGroup `json:"account_groups,omitempty"`

	GroupIDs []int64  `json:"group_ids,omitempty"`
	Groups   []*Group `json:"groups,omitempty"`
REDACTED

type AccountGroup struct {
	AccountID int64     `json:"account_id"`
	GroupID   int64     `json:"group_id"`
	Priority  int       `json:"priority"`
	CreatedAt time.Time `json:"created_at"`

	Account *Account `json:"account,omitempty"`
	Group   *Group   `json:"group,omitempty"`
REDACTED

type Proxy struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Protocol  string    `json:"protocol"`
	Host      string    `json:"host"`
	Port      int       `json:"port"`
	Username  string    `json:"username"`
	Password  string    `json:"-"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
REDACTED

type ProxyWithAccountCount struct {
	Proxy
	AccountCount int64 `json:"account_count"`
REDACTED

type RedeemCode struct {
	ID        int64      `json:"id"`
	Code      string     `json:"code"`
	Type      string     `json:"type"`
	Value     float64    `json:"value"`
	Status    string     `json:"status"`
	UsedBy    *int64     `json:"used_by"`
	UsedAt    *time.Time `json:"used_at"`
	Notes     string     `json:"notes"`
	CreatedAt time.Time  `json:"created_at"`

	GroupID      *int64 `json:"group_id"`
	ValidityDays int    `json:"validity_days"`

	User  *User  `json:"user,omitempty"`
	Group *Group `json:"group,omitempty"`
REDACTED

type UsageLog struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"user_id"`
	ApiKeyID  int64  `json:"api_key_id"`
	AccountID int64  `json:"account_id"`
	RequestID string `json:"request_id"`
	Model     string `json:"model"`

	GroupID        *int64 `json:"group_id"`
	SubscriptionID *int64 `json:"subscription_id"`

	InputTokens         int `json:"input_tokens"`
	OutputTokens        int `json:"output_tokens"`
	CacheCreationTokens int `json:"cache_creation_tokens"`
	CacheReadTokens     int `json:"cache_read_tokens"`

	CacheCreation5mTokens int `json:"cache_creation_5m_tokens"`
	CacheCreation1hTokens int `json:"cache_creation_1h_tokens"`

	InputCost         float64 `json:"input_cost"`
	OutputCost        float64 `json:"output_cost"`
	CacheCreationCost float64 `json:"cache_creation_cost"`
	CacheReadCost     float64 `json:"cache_read_cost"`
	TotalCost         float64 `json:"total_cost"`
	ActualCost        float64 `json:"actual_cost"`
	RateMultiplier    float64 `json:"rate_multiplier"`

	BillingType  int8 `json:"billing_type"`
	Stream       bool `json:"stream"`
	DurationMs   *int `json:"duration_ms"`
	FirstTokenMs *int `json:"first_token_ms"`

	CreatedAt time.Time `json:"created_at"`

	User         *User             `json:"user,omitempty"`
	ApiKey       *ApiKey           `json:"api_key,omitempty"`
	Account      *Account          `json:"account,omitempty"`
	Group        *Group            `json:"group,omitempty"`
	Subscription *UserSubscription `json:"subscription,omitempty"`
REDACTED

type Setting struct {
	ID        int64     `json:"id"`
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
REDACTED

type UserSubscription struct {
	ID      int64 `json:"id"`
	UserID  int64 `json:"user_id"`
	GroupID int64 `json:"group_id"`

	StartsAt  time.Time `json:"starts_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Status    string    `json:"status"`

	DailyWindowStart   *time.Time `json:"daily_window_start"`
	WeeklyWindowStart  *time.Time `json:"weekly_window_start"`
	MonthlyWindowStart *time.Time `json:"monthly_window_start"`

	DailyUsageUSD   float64 `json:"daily_usage_usd"`
	WeeklyUsageUSD  float64 `json:"weekly_usage_usd"`
	MonthlyUsageUSD float64 `json:"monthly_usage_usd"`

	AssignedBy *int64    `json:"assigned_by"`
	AssignedAt time.Time `json:"assigned_at"`
	Notes      string    `json:"notes"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	User           *User  `json:"user,omitempty"`
	Group          *Group `json:"group,omitempty"`
	AssignedByUser *User  `json:"assigned_by_user,omitempty"`
REDACTED
