package service

import "time"

type ProvisionPlan struct {
	ID            int64     `json:"id"`
	Code          string    `json:"code"`
	Name          string    `json:"name"`
	GroupID       int64     `json:"group_id"`
	Balance       float64   `json:"balance"`
	Quota         float64   `json:"quota"`
	ExpiresInDays *int      `json:"expires_in_days,omitempty"`
	RateLimit5h   float64   `json:"rate_limit_5h"`
	RateLimit1d   float64   `json:"rate_limit_1d"`
	RateLimit7d   float64   `json:"rate_limit_7d"`
	Concurrency   int       `json:"concurrency"`
	RPMLimit      int       `json:"rpm_limit"`
	Enabled       bool      `json:"enabled"`
	Group         *Group    `json:"group,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type ProvisionPlanInput struct {
	Code          string  `json:"code"`
	Name          string  `json:"name"`
	GroupID       int64   `json:"group_id"`
	Balance       float64 `json:"balance"`
	Quota         float64 `json:"quota"`
	ExpiresInDays *int    `json:"expires_in_days"`
	RateLimit5h   float64 `json:"rate_limit_5h"`
	RateLimit1d   float64 `json:"rate_limit_1d"`
	RateLimit7d   float64 `json:"rate_limit_7d"`
	Concurrency   int     `json:"concurrency"`
	RPMLimit      int     `json:"rpm_limit"`
	Enabled       bool    `json:"enabled"`
}

type ProvisionOrder struct {
	ID            int64             `json:"id"`
	OrderID       string            `json:"order_id"`
	PlanID        *int64            `json:"plan_id,omitempty"`
	PlanCode      string            `json:"plan_code"`
	PlanSnapshot  ProvisionSnapshot `json:"plan_snapshot"`
	UserID        *int64            `json:"user_id,omitempty"`
	APIKeyID      *int64            `json:"api_key_id,omitempty"`
	Status        string            `json:"status"`
	CustomerLabel string            `json:"customer_label"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

type ProvisionSnapshot struct {
	PlanID         int64   `json:"plan_id"`
	PlanCode       string  `json:"plan_code"`
	PlanName       string  `json:"plan_name"`
	GroupID        int64   `json:"group_id"`
	GroupName      string  `json:"group_name"`
	Platform       string  `json:"platform"`
	RateMultiplier float64 `json:"rate_multiplier"`
	Balance        float64 `json:"balance"`
	Quota          float64 `json:"quota"`
	ExpiresInDays  *int    `json:"expires_in_days,omitempty"`
	RateLimit5h    float64 `json:"rate_limit_5h"`
	RateLimit1d    float64 `json:"rate_limit_1d"`
	RateLimit7d    float64 `json:"rate_limit_7d"`
	Concurrency    int     `json:"concurrency"`
	RPMLimit       int     `json:"rpm_limit"`
}

type ProvisionResult struct {
	OrderID        string  `json:"order_id"`
	APIKey         string  `json:"api_key"`
	KeyID          int64   `json:"key_id"`
	UserID         int64   `json:"user_id"`
	PlanCode       string  `json:"plan_code"`
	GroupID        int64   `json:"group_id"`
	Balance        float64 `json:"balance"`
	Quota          float64 `json:"quota"`
	RateMultiplier float64 `json:"rate_multiplier"`
}

type ProvisionAPIKeyInput struct {
	OrderID       string `json:"order_id"`
	PlanCode      string `json:"plan_code"`
	CustomerLabel string `json:"customer_label"`
}
