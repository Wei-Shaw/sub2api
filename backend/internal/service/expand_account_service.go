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
	AccountID        *int64                 `json:"account_id,omitempty"`
	LoginStatus      int64                  `json:"login_status"`
	DeviceID         string                 `json:"device_id,omitempty"`
	APIKey           string                 `json:"api_key,omitempty"`
	EmailPwd         string                 `json:"email_pwd,omitempty"`
	HelpEmail        string                 `json:"help_email,omitempty"`
	HelpEmailURL     string                 `json:"help_email_url,omitempty"`
	Channel          string                 `json:"channel,omitempty"`
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
}

type ProxyInfo struct {
	Protocol string `json:"protocol"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Region   string `json:"region,omitempty"`
}

type ExpandAccountListFilters struct {
	Search      string
	Used        string
	LoginStatus *int64
	AccountType string
}

type ExpandAccountCreateInput struct {
	Email            string
	Platform         string
	SubscriptionType string
	Country          string
	SessionKey       string
	ProxyInfo        *ProxyInfo
	Used             *bool
	EmailPwd         string
	HelpEmail        string
	HelpEmailURL     string
	Channel          string
}

type ExpandAccountUpdateInput struct {
	Email            string
	Platform         string
	SubscriptionType string
	Country          string
	SessionKey       string
	ProxyInfo        *ProxyInfo
	Used             *bool
	EmailPwd         string
	HelpEmail        string
	HelpEmailURL     string
	Channel          string
}

type ExpandAccountReportInput struct {
	ID          int64
	Email       string
	LoginStatus int64
	DeviceID    string
	APIKey      string
}

type ExpandAccountService interface {
	ListExpandAccounts(ctx context.Context, page, pageSize int, filters ExpandAccountListFilters) ([]ExpandAccount, int64, error)
	GetExpandAccount(ctx context.Context, id int64) (*ExpandAccount, error)
	CreateExpandAccount(ctx context.Context, input *ExpandAccountCreateInput) (*ExpandAccount, error)
	UpdateExpandAccount(ctx context.Context, id int64, input *ExpandAccountUpdateInput) (*ExpandAccount, error)
	DeleteExpandAccount(ctx context.Context, id int64) error
	MarkExpandAccountUsed(ctx context.Context, id int64) (*ExpandAccount, error)
	GetAndMarkExpandAccountByPlatform(ctx context.Context, platform string) (*ExpandAccount, error)
	ReportExpandAccountLogin(ctx context.Context, input *ExpandAccountReportInput) (*ExpandAccount, error)
}
