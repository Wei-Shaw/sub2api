package handler

import (
	"sub2api/internal/handler/admin"
)

// AdminHandlers contains all admin-related HTTP handlers
type AdminHandlers struct {
	Dashboard    *admin.DashboardHandler
	User         *admin.UserHandler
	Group        *admin.GroupHandler
	Account      *admin.AccountHandler
	OAuth        *admin.OAuthHandler
	OpenAIOAuth  *admin.OpenAIOAuthHandler
	Proxy        *admin.ProxyHandler
	Redeem       *admin.RedeemHandler
	Setting      *admin.SettingHandler
	System       *admin.SystemHandler
	Subscription *admin.SubscriptionHandler
	Usage        *admin.UsageHandler
}

// Handlers contains all HTTP handlers
type Handlers struct {
	Auth          *AuthHandler
	User          *UserHandler
	APIKey        *APIKeyHandler
	Usage         *UsageHandler
	Redeem        *RedeemHandler
	Subscription  *SubscriptionHandler
	Admin         *AdminHandlers
	Gateway       *GatewayHandler
	OpenAIGateway *OpenAIGatewayHandler
	Setting       *SettingHandler
}

// BuildInfo contains build-time information
type BuildInfo struct {
	Version   string
	BuildType string // "source" for manual builds, "release" for CI builds
}
