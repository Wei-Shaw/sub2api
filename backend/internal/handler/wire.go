package handler

import (
	"sub2api/internal/handler/admin"
	"sub2api/internal/service"

	"github.com/google/wire"
)

// ProvideAdminHandlers creates the AdminHandlers struct
func ProvideAdminHandlers(
	dashboardHandler *admin.DashboardHandler,
	userHandler *admin.UserHandler,
	groupHandler *admin.GroupHandler,
	accountHandler *admin.AccountHandler,
	oauthHandler *admin.OAuthHandler,
	proxyHandler *admin.ProxyHandler,
	redeemHandler *admin.RedeemHandler,
	settingHandler *admin.SettingHandler,
	systemHandler *admin.SystemHandler,
	subscriptionHandler *admin.SubscriptionHandler,
	usageHandler *admin.UsageHandler,
) *AdminHandlers {
	return &AdminHandlers{
		Dashboard:    dashboardHandler,
		User:         userHandler,
		Group:        groupHandler,
		Account:      accountHandler,
		OAuth:        oauthHandler,
		Proxy:        proxyHandler,
		Redeem:       redeemHandler,
		Setting:      settingHandler,
		System:       systemHandler,
		Subscription: subscriptionHandler,
		Usage:        usageHandler,
REDACTED
REDACTED

// ProvideSystemHandler creates admin.SystemHandler with UpdateService
func ProvideSystemHandler(updateService *service.UpdateService) *admin.SystemHandler {
	return admin.NewSystemHandler(updateService)
REDACTED

// ProvideSettingHandler creates SettingHandler with version from BuildInfo
func ProvideSettingHandler(settingService *service.SettingService, buildInfo BuildInfo) *SettingHandler {
	return NewSettingHandler(settingService, buildInfo.Version)
REDACTED

// ProvideHandlers creates the Handlers struct
func ProvideHandlers(
	authHandler *AuthHandler,
	userHandler *UserHandler,
	apiKeyHandler *APIKeyHandler,
	usageHandler *UsageHandler,
	redeemHandler *RedeemHandler,
	subscriptionHandler *SubscriptionHandler,
	adminHandlers *AdminHandlers,
	gatewayHandler *GatewayHandler,
	settingHandler *SettingHandler,
) *Handlers {
	return &Handlers{
		Auth:         authHandler,
		User:         userHandler,
		APIKey:       apiKeyHandler,
		Usage:        usageHandler,
		Redeem:       redeemHandler,
		Subscription: subscriptionHandler,
		Admin:        adminHandlers,
		Gateway:      gatewayHandler,
		Setting:      settingHandler,
REDACTED
REDACTED

// ProviderSet is the Wire provider set for all handlers
var ProviderSet = wire.NewSet(
	// Top-level handlers
	NewAuthHandler,
	NewUserHandler,
	NewAPIKeyHandler,
	NewUsageHandler,
	NewRedeemHandler,
	NewSubscriptionHandler,
	NewGatewayHandler,
	ProvideSettingHandler,

	// Admin handlers
	admin.NewDashboardHandler,
	admin.NewUserHandler,
	admin.NewGroupHandler,
	admin.NewAccountHandler,
	admin.NewOAuthHandler,
	admin.NewProxyHandler,
	admin.NewRedeemHandler,
	admin.NewSettingHandler,
	ProvideSystemHandler,
	admin.NewSubscriptionHandler,
	admin.NewUsageHandler,

	// AdminHandlers and Handlers constructors
	ProvideAdminHandlers,
	ProvideHandlers,
)
