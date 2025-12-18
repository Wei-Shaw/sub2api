package handler

import (
	"sub2api/internal/handler/admin"
	"sub2api/internal/repository"
	"sub2api/internal/service"

	"github.com/redis/go-redis/v9"
)

// AdminHandlers contains all admin-related HTTP handlers
type AdminHandlers struct {
	Dashboard    *admin.DashboardHandler
	User         *admin.UserHandler
	Group        *admin.GroupHandler
	Account      *admin.AccountHandler
	OAuth        *admin.OAuthHandler
	Proxy        *admin.ProxyHandler
	Redeem       *admin.RedeemHandler
	Setting      *admin.SettingHandler
	System       *admin.SystemHandler
	Subscription *admin.SubscriptionHandler
	Usage        *admin.UsageHandler
REDACTED

// Handlers contains all HTTP handlers
type Handlers struct {
	Auth         *AuthHandler
	User         *UserHandler
	APIKey       *APIKeyHandler
	Usage        *UsageHandler
	Redeem       *RedeemHandler
	Subscription *SubscriptionHandler
	Admin        *AdminHandlers
	Gateway      *GatewayHandler
	Setting      *SettingHandler
REDACTED

// BuildInfo contains build-time information
type BuildInfo struct {
	Version   string
	BuildType string // "source" for manual builds, "release" for CI builds
REDACTED

// NewHandlers creates a new Handlers instance with all handlers initialized
func NewHandlers(services *service.Services, repos *repository.Repositories, rdb *redis.Client, buildInfo BuildInfo) *Handlers {
	return &Handlers{
		Auth:         NewAuthHandler(services.Auth),
		User:         NewUserHandler(services.User),
		APIKey:       NewAPIKeyHandler(services.ApiKey),
		Usage:        NewUsageHandler(services.Usage, repos.UsageLog, services.ApiKey),
		Redeem:       NewRedeemHandler(services.Redeem),
		Subscription: NewSubscriptionHandler(services.Subscription),
		Admin: &AdminHandlers{
			Dashboard:    admin.NewDashboardHandler(services.Admin, repos.UsageLog),
			User:         admin.NewUserHandler(services.Admin),
			Group:        admin.NewGroupHandler(services.Admin),
			Account:      admin.NewAccountHandler(services.Admin, services.OAuth, services.RateLimit, services.AccountUsage, services.AccountTest),
			OAuth:        admin.NewOAuthHandler(services.OAuth, services.Admin),
			Proxy:        admin.NewProxyHandler(services.Admin),
			Redeem:       admin.NewRedeemHandler(services.Admin),
			Setting:      admin.NewSettingHandler(services.Setting, services.Email),
			System:       admin.NewSystemHandler(rdb, buildInfo.Version, buildInfo.BuildType),
			Subscription: admin.NewSubscriptionHandler(services.Subscription),
			Usage:        admin.NewUsageHandler(repos.UsageLog, repos.ApiKey, services.Usage, services.Admin),
	REDACTED,
		Gateway: NewGatewayHandler(services.Gateway, services.User, services.Concurrency, services.BillingCache),
		Setting: NewSettingHandler(services.Setting, buildInfo.Version),
REDACTED
REDACTED
