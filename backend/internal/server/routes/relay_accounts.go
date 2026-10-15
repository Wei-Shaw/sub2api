package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/gin-gonic/gin"
)

// Reuse account/OAuth handlers, not Admin authorization. Every saved-account
// endpoint has an ownership gate, and the scope also guards shared services.
func registerRelayAccountRoutes(relay *gin.RouterGroup, h *handler.Handlers) {
	relay.Use(h.Relay.AccountScope)
	accounts := relay.Group("/accounts")
	accounts.GET("/:id", h.Relay.RequireOwnedAccount, h.Admin.Account.GetByID)
	accounts.GET("/:id/models", h.Relay.RequireOwnedAccount, h.Admin.Account.GetAvailableModels)
	accounts.POST("/:id/test", h.Relay.RequireOwnedAccount, h.Admin.Account.Test)
	accounts.POST("/:id/refresh", h.Relay.RequireOwnedAccount, h.Admin.Account.Refresh)
	accounts.POST("/:id/apply-oauth-credentials", h.Relay.RequireOwnedAccount, h.Admin.Account.ApplyOAuthCredentials)
	accounts.POST("/:id/clear-error", h.Relay.RequireOwnedAccount, h.Admin.Account.ClearError)
	accounts.GET("/:id/usage", h.Relay.RequireOwnedAccount, h.Admin.Account.GetUsage)
	accounts.GET("/:id/stats", h.Relay.RequireOwnedAccount, h.Admin.Account.GetStats)
	accounts.POST("/:id/models/sync-upstream", h.Relay.RequireOwnedAccount, h.Admin.Account.SyncUpstreamModels)
	accounts.GET("/:id/grok-media-eligibility", h.Relay.RequireOwnedAccount, h.Admin.Account.GetGrokMediaEligibility)
	accounts.PUT("/:id/grok-media-eligibility", h.Relay.RequireOwnedAccount, h.Admin.Account.UpdateGrokMediaEligibility)
	accounts.GET("/:id/opencode-go-usage", h.Relay.RequireOwnedAccount, h.Admin.Account.GetOpenCodeGoUsage)
	accounts.PUT("/:id/opencode-go-usage/auto-refresh", h.Relay.RequireOwnedAccount, h.Admin.Account.SetOpenCodeGoUsageAutoRefresh)
	accounts.POST("/:id/opencode-go-usage/refresh", h.Relay.RequireOwnedAccount, h.Admin.Account.RefreshOpenCodeGoUsage)
	accounts.POST("/:id/upstream-billing-probe", h.Relay.RequireOwnedAccount, h.Admin.Account.ProbeUpstreamBilling)
	accounts.POST("/models/sync-upstream-preview", h.Admin.Account.SyncUpstreamModelsPreview)
	accounts.GET("/antigravity/default-model-mapping", h.Admin.Account.GetAntigravityDefaultModelMapping)
	accounts.POST("/check-mixed-channel", h.Relay.CheckMixedChannel)
	accounts.POST("/import/codex-session", h.Admin.Account.ImportCodexSession)
	accounts.POST("/generate-auth-url", h.Admin.OAuth.GenerateAuthURL)
	accounts.POST("/generate-setup-token-url", h.Admin.OAuth.GenerateSetupTokenURL)
	accounts.POST("/exchange-code", h.Admin.OAuth.ExchangeCode)
	accounts.POST("/exchange-setup-token-code", h.Admin.OAuth.ExchangeSetupTokenCode)
	accounts.POST("/cookie-auth", h.Admin.OAuth.CookieAuth)
	accounts.POST("/setup-token-cookie-auth", h.Admin.OAuth.SetupTokenCookieAuth)

	openai := relay.Group("/openai")
	openai.POST("/generate-auth-url", h.Admin.OpenAIOAuth.GenerateAuthURL)
	openai.POST("/exchange-code", h.Admin.OpenAIOAuth.ExchangeCode)
	openai.POST("/refresh-token", h.Admin.OpenAIOAuth.RefreshToken)
	openai.POST("/create-from-codex-pat", h.Admin.OpenAIOAuth.CreateAccountFromCodexPAT)

	gemini := relay.Group("/gemini/oauth")
	gemini.POST("/auth-url", h.Admin.GeminiOAuth.GenerateAuthURL)
	gemini.POST("/exchange-code", h.Admin.GeminiOAuth.ExchangeCode)
	gemini.GET("/capabilities", h.Admin.GeminiOAuth.GetCapabilities)

	antigravity := relay.Group("/antigravity/oauth")
	antigravity.POST("/auth-url", h.Admin.AntigravityOAuth.GenerateAuthURL)
	antigravity.POST("/exchange-code", h.Admin.AntigravityOAuth.ExchangeCode)
	antigravity.POST("/refresh-token", h.Admin.AntigravityOAuth.RefreshToken)

	grok := relay.Group("/grok")
	grok.GET("/oauth/capabilities", h.Admin.GrokOAuth.GetCapabilities)
	grok.POST("/oauth/auth-url", h.Admin.GrokOAuth.GenerateAuthURL)
	grok.POST("/oauth/exchange-code", h.Admin.GrokOAuth.ExchangeCode)
	grok.POST("/oauth/refresh-token", h.Admin.GrokOAuth.RefreshToken)
	grok.POST("/oauth/sso-token", h.Admin.GrokOAuth.ValidateSSOToken)
	grok.POST("/oauth/password", h.Admin.GrokOAuth.AuthorizePassword)
	grok.POST("/sso-to-oauth", h.Admin.GrokOAuth.CreateAccountsFromSSO)
	grok.GET("/accounts/:id/quota", h.Relay.RequireOwnedAccount, h.Admin.GrokOAuth.QueryQuota)
	grok.POST("/accounts/:id/reset-quota", h.Relay.RequireOwnedAccount, h.Admin.GrokOAuth.ResetQuota)
}
