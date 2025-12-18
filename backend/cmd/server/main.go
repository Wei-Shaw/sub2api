package main

//go:generate go run github.com/google/wire/cmd/wire

import (
	"context"
	_ "embed"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"sub2api/internal/config"
	"sub2api/internal/handler"
	"sub2api/internal/middleware"
	"sub2api/internal/model"
	"sub2api/internal/pkg/timezone"
	"sub2api/internal/repository"
	"sub2api/internal/service"
	"sub2api/internal/setup"
	"sub2api/internal/web"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

//go:embed VERSION
var embeddedVersion string

// Build-time variables (can be set by ldflags)
var (
	Version   = ""
	Commit    = "unknown"
	Date      = "unknown"
	BuildType = "source" // "source" for manual builds, "release" for CI builds (set by ldflags)
)

func init() {
	// Read version from embedded VERSION file
	Version = strings.TrimSpace(embeddedVersion)
	if Version == "" {
		Version = "0.0.0-dev"
REDACTED
REDACTED

func main() {
	// Parse command line flags
	setupMode := flag.Bool("setup", false, "Run setup wizard in CLI mode")
	showVersion := flag.Bool("version", false, "Show version information")
	flag.Parse()

	if *showVersion {
		log.Printf("Sub2API %s (commit: %s, built: %s)\n", Version, Commit, Date)
		return
REDACTED

	// CLI setup mode
	if *setupMode {
		if err := setup.RunCLI(); err != nil {
			log.Fatalf("Setup failed: %v", err)
	REDACTED
		return
REDACTED

	// Check if setup is needed
	if setup.NeedsSetup() {
		// Check if auto-setup is enabled (for Docker deployment)
		if setup.AutoSetupEnabled() {
			log.Println("Auto setup mode enabled...")
			if err := setup.AutoSetupFromEnv(); err != nil {
				log.Fatalf("Auto setup failed: %v", err)
		REDACTED
			// Continue to main server after auto-setup
	REDACTED else {
			log.Println("First run detected, starting setup wizard...")
			runSetupServer()
			return
	REDACTED
REDACTED

	// Normal server mode
	runMainServer()
REDACTED

func runSetupServer() {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.CORS())

	// Register setup routes
	setup.RegisterRoutes(r)

	// Serve embedded frontend if available
	if web.HasEmbeddedFrontend() {
		r.Use(web.ServeEmbeddedFrontend())
REDACTED

	addr := ":8080"
	log.Printf("Setup wizard available at http://localhost%s", addr)
	log.Println("Complete the setup wizard to configure Sub2API")

	if err := r.Run(addr); err != nil {
		log.Fatalf("Failed to start setup server: %v", err)
REDACTED
REDACTED

func runMainServer() {
	buildInfo := handler.BuildInfo{
		Version:   Version,
		BuildType: BuildType,
REDACTED

	app, err := initializeApplication(buildInfo)
	if err != nil {
		log.Fatalf("Failed to initialize application: %v", err)
REDACTED
	defer app.Cleanup()

	// 启动服务器
	go func() {
		if err := app.Server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Failed to start server: %v", err)
	REDACTED
REDACTED()

	log.Printf("Server started on %s", app.Server.Addr)

	// 等待中断信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := app.Server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
REDACTED

	log.Println("Server exited")
REDACTED

func initDB(cfg *config.Config) (*gorm.DB, error) {
	// 初始化时区（在数据库连接之前，确保时区设置正确）
	if err := timezone.Init(cfg.Timezone); err != nil {
		return nil, err
REDACTED

	gormConfig := &gorm.Config{REDACTED
	if cfg.Server.Mode == "debug" {
		gormConfig.Logger = logger.Default.LogMode(logger.Info)
REDACTED

	// 使用带时区的 DSN 连接数据库
	db, err := gorm.Open(postgres.Open(cfg.Database.DSNWithTimezone(cfg.Timezone)), gormConfig)
	if err != nil {
		return nil, err
REDACTED

	// 自动迁移（始终执行，确保数据库结构与代码同步）
	// GORM 的 AutoMigrate 只会添加新字段，不会删除或修改已有字段，是安全的
	if err := model.AutoMigrate(db); err != nil {
		return nil, err
REDACTED

	return db, nil
REDACTED

func initRedis(cfg *config.Config) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Address(),
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
REDACTED)
REDACTED

func registerRoutes(r *gin.Engine, h *handler.Handlers, s *service.Services, repos *repository.Repositories) {
	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"REDACTED)
REDACTED)

	// Setup status endpoint (always returns needs_setup: false in normal mode)
	// This is used by the frontend to detect when the service has restarted after setup
	r.GET("/setup/status", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"data": gin.H{
				"needs_setup": false,
				"step":        "completed",
		REDACTED,
	REDACTED)
REDACTED)

	// API v1
	v1 := r.Group("/api/v1")
	{
		// 公开接口
		auth := v1.Group("/auth")
		{
			auth.POST("/register", h.Auth.Register)
			auth.POST("/login", h.Auth.Login)
			auth.POST("/send-verify-code", h.Auth.SendVerifyCode)
	REDACTED

		// 公开设置（无需认证）
		settings := v1.Group("/settings")
		{
			settings.GET("/public", h.Setting.GetPublicSettings)
	REDACTED

		// 需要认证的接口
		authenticated := v1.Group("")
		authenticated.Use(middleware.JWTAuth(s.Auth, repos.User))
		{
			// 当前用户信息
			authenticated.GET("/auth/me", h.Auth.GetCurrentUser)

			// 用户接口
			user := authenticated.Group("/users/me")
			{
				user.GET("", h.User.GetProfile)
				user.POST("/password", h.User.ChangePassword)
		REDACTED

			// API Key管理
			keys := authenticated.Group("/keys")
			{
				keys.GET("", h.APIKey.List)
				keys.GET("/:id", h.APIKey.GetByID)
				keys.POST("", h.APIKey.Create)
				keys.PUT("/:id", h.APIKey.Update)
				keys.DELETE("/:id", h.APIKey.Delete)
		REDACTED

			// 用户可用分组（非管理员接口）
			groups := authenticated.Group("/groups")
			{
				groups.GET("/available", h.APIKey.GetAvailableGroups)
		REDACTED

			// 使用记录
			usage := authenticated.Group("/usage")
			{
				usage.GET("", h.Usage.List)
				usage.GET("/:id", h.Usage.GetByID)
				usage.GET("/stats", h.Usage.Stats)
				// User dashboard endpoints
				usage.GET("/dashboard/stats", h.Usage.DashboardStats)
				usage.GET("/dashboard/trend", h.Usage.DashboardTrend)
				usage.GET("/dashboard/models", h.Usage.DashboardModels)
				usage.POST("/dashboard/api-keys-usage", h.Usage.DashboardApiKeysUsage)
		REDACTED

			// 卡密兑换
			redeem := authenticated.Group("/redeem")
			{
				redeem.POST("", h.Redeem.Redeem)
				redeem.GET("/history", h.Redeem.GetHistory)
		REDACTED

			// 用户订阅
			subscriptions := authenticated.Group("/subscriptions")
			{
				subscriptions.GET("", h.Subscription.List)
				subscriptions.GET("/active", h.Subscription.GetActive)
				subscriptions.GET("/progress", h.Subscription.GetProgress)
				subscriptions.GET("/summary", h.Subscription.GetSummary)
		REDACTED
	REDACTED

		// 管理员接口
		admin := v1.Group("/admin")
		admin.Use(middleware.JWTAuth(s.Auth, repos.User), middleware.AdminOnly())
		{
			// 仪表盘
			dashboard := admin.Group("/dashboard")
			{
				dashboard.GET("/stats", h.Admin.Dashboard.GetStats)
				dashboard.GET("/realtime", h.Admin.Dashboard.GetRealtimeMetrics)
				dashboard.GET("/trend", h.Admin.Dashboard.GetUsageTrend)
				dashboard.GET("/models", h.Admin.Dashboard.GetModelStats)
				dashboard.GET("/api-keys-trend", h.Admin.Dashboard.GetApiKeyUsageTrend)
				dashboard.GET("/users-trend", h.Admin.Dashboard.GetUserUsageTrend)
				dashboard.POST("/users-usage", h.Admin.Dashboard.GetBatchUsersUsage)
				dashboard.POST("/api-keys-usage", h.Admin.Dashboard.GetBatchApiKeysUsage)
		REDACTED

			// 用户管理
			users := admin.Group("/users")
			{
				users.GET("", h.Admin.User.List)
				users.GET("/:id", h.Admin.User.GetByID)
				users.POST("", h.Admin.User.Create)
				users.PUT("/:id", h.Admin.User.Update)
				users.DELETE("/:id", h.Admin.User.Delete)
				users.POST("/:id/balance", h.Admin.User.UpdateBalance)
				users.GET("/:id/api-keys", h.Admin.User.GetUserAPIKeys)
				users.GET("/:id/usage", h.Admin.User.GetUserUsage)
		REDACTED

			// 分组管理
			groups := admin.Group("/groups")
			{
				groups.GET("", h.Admin.Group.List)
				groups.GET("/all", h.Admin.Group.GetAll)
				groups.GET("/:id", h.Admin.Group.GetByID)
				groups.POST("", h.Admin.Group.Create)
				groups.PUT("/:id", h.Admin.Group.Update)
				groups.DELETE("/:id", h.Admin.Group.Delete)
				groups.GET("/:id/stats", h.Admin.Group.GetStats)
				groups.GET("/:id/api-keys", h.Admin.Group.GetGroupAPIKeys)
		REDACTED

			// 账号管理
			accounts := admin.Group("/accounts")
			{
				accounts.GET("", h.Admin.Account.List)
				accounts.GET("/:id", h.Admin.Account.GetByID)
				accounts.POST("", h.Admin.Account.Create)
				accounts.PUT("/:id", h.Admin.Account.Update)
				accounts.DELETE("/:id", h.Admin.Account.Delete)
				accounts.POST("/:id/test", h.Admin.Account.Test)
				accounts.POST("/:id/refresh", h.Admin.Account.Refresh)
				accounts.GET("/:id/stats", h.Admin.Account.GetStats)
				accounts.POST("/:id/clear-error", h.Admin.Account.ClearError)
				accounts.GET("/:id/usage", h.Admin.Account.GetUsage)
				accounts.GET("/:id/today-stats", h.Admin.Account.GetTodayStats)
				accounts.POST("/:id/clear-rate-limit", h.Admin.Account.ClearRateLimit)
				accounts.POST("/:id/schedulable", h.Admin.Account.SetSchedulable)
				accounts.POST("/batch", h.Admin.Account.BatchCreate)

				// OAuth routes
				accounts.POST("/generate-auth-url", h.Admin.OAuth.GenerateAuthURL)
				accounts.POST("/generate-setup-token-url", h.Admin.OAuth.GenerateSetupTokenURL)
				accounts.POST("/exchange-code", h.Admin.OAuth.ExchangeCode)
				accounts.POST("/exchange-setup-token-code", h.Admin.OAuth.ExchangeSetupTokenCode)
				accounts.POST("/cookie-auth", h.Admin.OAuth.CookieAuth)
				accounts.POST("/setup-token-cookie-auth", h.Admin.OAuth.SetupTokenCookieAuth)
		REDACTED

			// 代理管理
			proxies := admin.Group("/proxies")
			{
				proxies.GET("", h.Admin.Proxy.List)
				proxies.GET("/all", h.Admin.Proxy.GetAll)
				proxies.GET("/:id", h.Admin.Proxy.GetByID)
				proxies.POST("", h.Admin.Proxy.Create)
				proxies.PUT("/:id", h.Admin.Proxy.Update)
				proxies.DELETE("/:id", h.Admin.Proxy.Delete)
				proxies.POST("/:id/test", h.Admin.Proxy.Test)
				proxies.GET("/:id/stats", h.Admin.Proxy.GetStats)
				proxies.GET("/:id/accounts", h.Admin.Proxy.GetProxyAccounts)
				proxies.POST("/batch", h.Admin.Proxy.BatchCreate)
		REDACTED

			// 卡密管理
			codes := admin.Group("/redeem-codes")
			{
				codes.GET("", h.Admin.Redeem.List)
				codes.GET("/stats", h.Admin.Redeem.GetStats)
				codes.GET("/export", h.Admin.Redeem.Export)
				codes.GET("/:id", h.Admin.Redeem.GetByID)
				codes.POST("/generate", h.Admin.Redeem.Generate)
				codes.DELETE("/:id", h.Admin.Redeem.Delete)
				codes.POST("/batch-delete", h.Admin.Redeem.BatchDelete)
				codes.POST("/:id/expire", h.Admin.Redeem.Expire)
		REDACTED

			// 系统设置
			adminSettings := admin.Group("/settings")
			{
				adminSettings.GET("", h.Admin.Setting.GetSettings)
				adminSettings.PUT("", h.Admin.Setting.UpdateSettings)
				adminSettings.POST("/test-smtp", h.Admin.Setting.TestSmtpConnection)
				adminSettings.POST("/send-test-email", h.Admin.Setting.SendTestEmail)
		REDACTED

			// 系统管理
			system := admin.Group("/system")
			{
				system.GET("/version", h.Admin.System.GetVersion)
				system.GET("/check-updates", h.Admin.System.CheckUpdates)
				system.POST("/update", h.Admin.System.PerformUpdate)
				system.POST("/rollback", h.Admin.System.Rollback)
				system.POST("/restart", h.Admin.System.RestartService)
		REDACTED

			// 订阅管理
			subscriptions := admin.Group("/subscriptions")
			{
				subscriptions.GET("", h.Admin.Subscription.List)
				subscriptions.GET("/:id", h.Admin.Subscription.GetByID)
				subscriptions.GET("/:id/progress", h.Admin.Subscription.GetProgress)
				subscriptions.POST("/assign", h.Admin.Subscription.Assign)
				subscriptions.POST("/bulk-assign", h.Admin.Subscription.BulkAssign)
				subscriptions.POST("/:id/extend", h.Admin.Subscription.Extend)
				subscriptions.DELETE("/:id", h.Admin.Subscription.Revoke)
		REDACTED

			// 分组下的订阅列表
			admin.GET("/groups/:id/subscriptions", h.Admin.Subscription.ListByGroup)

			// 用户下的订阅列表
			admin.GET("/users/:id/subscriptions", h.Admin.Subscription.ListByUser)

			// 使用记录管理
			usage := admin.Group("/usage")
			{
				usage.GET("", h.Admin.Usage.List)
				usage.GET("/stats", h.Admin.Usage.Stats)
				usage.GET("/search-users", h.Admin.Usage.SearchUsers)
				usage.GET("/search-api-keys", h.Admin.Usage.SearchApiKeys)
		REDACTED
	REDACTED
REDACTED

	// API网关（Claude API兼容）
	gateway := r.Group("/v1")
	gateway.Use(middleware.ApiKeyAuthWithSubscription(s.ApiKey, s.Subscription))
	{
		gateway.POST("/messages", h.Gateway.Messages)
		gateway.GET("/models", h.Gateway.Models)
		gateway.GET("/usage", h.Gateway.Usage)
REDACTED
REDACTED

// setupRouter 配置路由器中间件和路由
func setupRouter(r *gin.Engine, cfg *config.Config, handlers *handler.Handlers, services *service.Services, repos *repository.Repositories) *gin.Engine {
	// 应用中间件
	r.Use(middleware.Logger())
	r.Use(middleware.CORS())

	// 注册路由
	registerRoutes(r, handlers, services, repos)

	// Serve embedded frontend if available
	if web.HasEmbeddedFrontend() {
		r.Use(web.ServeEmbeddedFrontend())
REDACTED

	return r
REDACTED

// createHTTPServer 创建HTTP服务器
func createHTTPServer(cfg *config.Config, router *gin.Engine) *http.Server {
	return &http.Server{
		Addr:    cfg.Server.Address(),
		Handler: router,
		// ReadHeaderTimeout: 读取请求头的超时时间，防止慢速请求头攻击
		ReadHeaderTimeout: time.Duration(cfg.Server.ReadHeaderTimeout) * time.Second,
		// IdleTimeout: 空闲连接超时时间，释放不活跃的连接资源
		IdleTimeout: time.Duration(cfg.Server.IdleTimeout) * time.Second,
		// 注意：不设置 WriteTimeout，因为流式响应可能持续十几分钟
		// 不设置 ReadTimeout，因为大请求体可能需要较长时间读取
REDACTED
REDACTED
