package service

import (
	"context"
	"time"
)

type ExpandAccount struct {
	ID               int64                  `json:"id"`
	Email            string                 `json:"email"`
	Platform         string                 `json:"platform"`
	SubscriptionType string                 `json:"subscription_type"`
	Country          string                 `json:"country"`
	SessionKey       string                 `json:"session_key"`
	ProxyID          *int64                 `json:"proxy_id,omitempty"`
	ProxyInfo        *ProxyInfo             `json:"proxy_info,omitempty"`
	Proxy            *ProxyWithAccountCount `json:"proxy,omitempty"`
	Used             bool                   `json:"used"`
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
}

type ProxyInfo struct {
	Protocol string `json:"protocol"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

type ExpandAccountListFilters struct {
	Search string
	Used   string
}

type ExpandAccountCreateInput struct {
	Email            string
	Platform         string
	SubscriptionType string
	Country          string
	SessionKey       string
	ProxyInfo        *ProxyInfo
	Used             *bool
}

type ExpandAccountUpdateInput struct {
	Email            string
	Platform         string
	SubscriptionType string
	Country          string
	SessionKey       string
	ProxyInfo        *ProxyInfo
	Used             *bool
}

type ExpandAccountService interface {
	ListExpandAccounts(ctx context.Context, page, pageSize int, filters ExpandAccountListFilters) ([]ExpandAccount, int64, error)
	GetExpandAccount(ctx context.Context, id int64) (*ExpandAccount, error)
	CreateExpandAccount(ctx context.Context, input *ExpandAccountCreateInput) (*ExpandAccount, error)
	UpdateExpandAccount(ctx context.Context, id int64, input *ExpandAccountUpdateInput) (*ExpandAccount, error)
	DeleteExpandAccount(ctx context.Context, id int64) error
	MarkExpandAccountUsed(ctx context.Context, id int64) (*ExpandAccount, error)
	GetAndMarkExpandAccountByPlatform(ctx context.Context, platform string) (*ExpandAccount, error)
}
