//go:build wireinject
// +build wireinject

package main

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/server"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/google/wire"
	"github.com/redis/go-redis/v9"
)

type Application struct {
	Server  *http.Server
	Cleanup func()
REDACTED

func initializeApplication(buildInfo handler.BuildInfo) (*Application, error) {
	wire.Build(
		// Infrastructure layer ProviderSets
		config.ProviderSet,

		// Business layer ProviderSets
		repository.ProviderSet,
		service.ProviderSet,
		payment.ProviderSet,
		middleware.ProviderSet,
		handler.ProviderSet,

		// Server layer ProviderSet
		server.ProviderSet,

		// Privacy client factory for OpenAI training opt-out
		providePrivacyClientFactory,

		// BuildInfo provider
		provideServiceBuildInfo,

		// Cleanup function provider
		provideCleanup,

		// Application struct
		wire.Struct(new(Application), "Server", "Cleanup"),
	)
	return nil, nil
REDACTED

func providePrivacyClientFactory() service.PrivacyClientFactory {
	return repository.CreatePrivacyReqClient
REDACTED

func provideServiceBuildInfo(buildInfo handler.BuildInfo) service.BuildInfo {
	return service.BuildInfo{
		Version:   buildInfo.Version,
		BuildType: buildInfo.BuildType,
REDACTED
REDACTED

func provideCleanup(
	entClient *ent.Client,
	rdb *redis.Client,
	opsMetricsCollector *service.OpsMetricsCollector,
	opsAggregation *service.OpsAggregationService,
	opsAlertEvaluator *service.OpsAlertEvaluatorService,
	opsCleanup *service.OpsCleanupService,
	opsScheduledReport *service.OpsScheduledReportService,
	opsSystemLogSink *service.OpsSystemLogSink,
	schedulerSnapshot *service.SchedulerSnapshotService,
	tokenRefresh *service.TokenRefreshService,
	accountExpiry *service.AccountExpiryService,
	proxyExpiry *service.ProxyExpiryService,
	subscriptionExpiry *service.SubscriptionExpiryService,
	usageCleanup *service.UsageCleanupService,
	idempotencyCleanup *service.IdempotencyCleanupService,
	pricing *service.PricingService,
	emailQueue *service.EmailQueueService,
	billingCache *service.BillingCacheService,
	usageRecordWorkerPool *service.UsageRecordWorkerPool,
	subscriptionService *service.SubscriptionService,
	oauth *service.OAuthService,
	openaiOAuth *service.OpenAIOAuthService,
	geminiOAuth *service.GeminiOAuthService,
	antigravityOAuth *service.AntigravityOAuthService,
	grokOAuth *service.GrokOAuthService,
	openAIGateway *service.OpenAIGatewayService,
	scheduledTestRunner *service.ScheduledTestRunnerService,
	backupSvc *service.BackupService,
	paymentOrderExpiry *service.PaymentOrderExpiryService,
	channelMonitorRunner *service.ChannelMonitorRunner,
	quotaFlusher *service.UserPlatformQuotaUsageFlusher,
) func() {
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		type cleanupStep struct {
			name string
			fn   func() error
	REDACTED

		// 应用层清理步骤可并行执行，基础设施资源（Redis/Ent）最后按顺序关闭。
		parallelSteps := []cleanupStep{
			{"OpsScheduledReportService", func() error {
				if opsScheduledReport != nil {
					opsScheduledReport.Stop()
			REDACTED
				return nil
	REDACTED
			{"OpsCleanupService", func() error {
				if opsCleanup != nil {
					opsCleanup.Stop()
			REDACTED
				return nil
	REDACTED
			{"OpsSystemLogSink", func() error {
				if opsSystemLogSink != nil {
					opsSystemLogSink.Stop()
			REDACTED
				return nil
	REDACTED
			{"OpsAlertEvaluatorService", func() error {
				if opsAlertEvaluator != nil {
					opsAlertEvaluator.Stop()
			REDACTED
				return nil
	REDACTED
			{"OpsAggregationService", func() error {
				if opsAggregation != nil {
					opsAggregation.Stop()
			REDACTED
				return nil
	REDACTED
			{"OpsMetricsCollector", func() error {
				if opsMetricsCollector != nil {
					opsMetricsCollector.Stop()
			REDACTED
				return nil
	REDACTED
			{"SchedulerSnapshotService", func() error {
				if schedulerSnapshot != nil {
					schedulerSnapshot.Stop()
			REDACTED
				return nil
	REDACTED
			{"UsageCleanupService", func() error {
				if usageCleanup != nil {
					usageCleanup.Stop()
			REDACTED
				return nil
	REDACTED
			{"IdempotencyCleanupService", func() error {
				if idempotencyCleanup != nil {
					idempotencyCleanup.Stop()
			REDACTED
				return nil
	REDACTED
			{"TokenRefreshService", func() error {
				tokenRefresh.Stop()
				return nil
	REDACTED
			{"AccountExpiryService", func() error {
				accountExpiry.Stop()
				return nil
	REDACTED
			{"ProxyExpiryService", func() error {
				proxyExpiry.Stop()
				return nil
	REDACTED
			{"SubscriptionExpiryService", func() error {
				subscriptionExpiry.Stop()
				return nil
	REDACTED
			{"SubscriptionService", func() error {
				if subscriptionService != nil {
					subscriptionService.Stop()
			REDACTED
				return nil
	REDACTED
			{"PricingService", func() error {
				pricing.Stop()
				return nil
	REDACTED
			{"EmailQueueService", func() error {
				emailQueue.Stop()
				return nil
	REDACTED
			{"BillingCacheService", func() error {
				billingCache.Stop()
				return nil
	REDACTED
			{"UsageRecordWorkerPool", func() error {
				if usageRecordWorkerPool != nil {
					usageRecordWorkerPool.Stop()
			REDACTED
				return nil
	REDACTED
			{"OAuthService", func() error {
				oauth.Stop()
				return nil
	REDACTED
			{"OpenAIOAuthService", func() error {
				openaiOAuth.Stop()
				return nil
	REDACTED
			{"GeminiOAuthService", func() error {
				geminiOAuth.Stop()
				return nil
	REDACTED
			{"AntigravityOAuthService", func() error {
				antigravityOAuth.Stop()
				return nil
	REDACTED
			{"GrokOAuthService", func() error {
				if grokOAuth != nil {
					grokOAuth.Stop()
			REDACTED
				return nil
	REDACTED
			{"OpenAIWSPool", func() error {
				if openAIGateway != nil {
					openAIGateway.CloseOpenAIWSPool()
			REDACTED
				return nil
	REDACTED
			{"ScheduledTestRunnerService", func() error {
				if scheduledTestRunner != nil {
					scheduledTestRunner.Stop()
			REDACTED
				return nil
	REDACTED
			{"BackupService", func() error {
				if backupSvc != nil {
					backupSvc.Stop()
			REDACTED
				return nil
	REDACTED
			{"PaymentOrderExpiryService", func() error {
				if paymentOrderExpiry != nil {
					paymentOrderExpiry.Stop()
			REDACTED
				return nil
	REDACTED
			{"ChannelMonitorRunner", func() error {
				if channelMonitorRunner != nil {
					channelMonitorRunner.Stop()
			REDACTED
				return nil
	REDACTED
			{"UserPlatformQuotaUsageFlusher", func() error {
				if quotaFlusher != nil {
					quotaFlusher.Stop()
			REDACTED
				return nil
	REDACTED
	REDACTED

		infraSteps := []cleanupStep{
			{"Redis", func() error {
				if rdb == nil {
					return nil
			REDACTED
				return rdb.Close()
	REDACTED
			{"Ent", func() error {
				if entClient == nil {
					return nil
			REDACTED
				return entClient.Close()
	REDACTED
	REDACTED

		runParallel := func(steps []cleanupStep) {
			var wg sync.WaitGroup
			for i := range steps {
				step := steps[i]
				wg.Add(1)
				go func() {
					defer wg.Done()
					if err := step.fn(); err != nil {
						log.Printf("[Cleanup] %s failed: %v", step.name, err)
						return
				REDACTED
					log.Printf("[Cleanup] %s succeeded", step.name)
			REDACTED()
		REDACTED
			wg.Wait()
	REDACTED

		runSequential := func(steps []cleanupStep) {
			for i := range steps {
				step := steps[i]
				if err := step.fn(); err != nil {
					log.Printf("[Cleanup] %s failed: %v", step.name, err)
					continue
			REDACTED
				log.Printf("[Cleanup] %s succeeded", step.name)
		REDACTED
	REDACTED

		runParallel(parallelSteps)
		runSequential(infraSteps)

		// Check if context timed out
		select {
		case <-ctx.Done():
			log.Printf("[Cleanup] Warning: cleanup timed out after 10 seconds")
		default:
			log.Printf("[Cleanup] All cleanup steps completed")
	REDACTED
REDACTED
REDACTED
