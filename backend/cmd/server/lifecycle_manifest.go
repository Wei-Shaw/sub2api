package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/runtime"
	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func provideAllLifecycle(
	serverComponent runtime.Component,
	timingWheel *service.TimingWheelService,
	deferredService *service.DeferredService,
	opsIngressRejectAggregator *service.OpsIngressRejectAggregator,
	oauthService *service.OAuthService,
	openAIOAuthService *service.OpenAIOAuthService,
	geminiOAuthService *service.GeminiOAuthService,
	antigravityOAuthService *service.AntigravityOAuthService,
	grokOAuthService *service.GrokOAuthService,
	openAIGatewayService *service.OpenAIGatewayService,
	pricingService *service.PricingService,
	concurrencyService *service.ConcurrencyService,
	userMessageQueueService *service.UserMessageQueueService,
	tlsFingerprintProfileService *service.TLSFingerprintProfileService,
	errorPassthroughService *service.ErrorPassthroughService,
	cfg *config.Config,
	apiKeyService *service.APIKeyService,
	opsService *service.OpsService,
	opsSystemLogSink *service.OpsSystemLogSink,
	auditLog *service.AuditLogService,
	emailQueue *service.EmailQueueService,
	billingCache *service.BillingCacheService,
	usageRecordWorkerPool *service.UsageRecordWorkerPool,
	subscriptionService *service.SubscriptionService,
	contentModerationService *service.ContentModerationService,
	authCacheInvalidationWorker *service.AuthCacheInvalidationWorker,
	batchImageCleanup *service.BatchImageCleanupService,
	batchImageWorker *service.BatchImageWorkerRuntime,
	promptAudit *securityaudit.PromptService,
	tokenRefresh *service.TokenRefreshService,
	schedulerSnapshot *service.SchedulerSnapshotService,
	dashboardAggregation *service.DashboardAggregationService,
	usageCleanup *service.UsageCleanupService,
	accountExpiry *service.AccountExpiryService,
	codexVersionSync *service.OpenAICodexVersionSyncService,
	proxyExpiry *service.ProxyExpiryService,
	subscriptionExpiry *service.SubscriptionExpiryService,
	opsMetrics *service.OpsMetricsCollector,
	opsAggregation *service.OpsAggregationService,
	opsAlert *service.OpsAlertEvaluatorService,
	opsCleanup *service.OpsCleanupService,
	idempotencyCleanup *service.IdempotencyCleanupService,
	scheduledTests *service.ScheduledTestRunnerService,
	opsReports *service.OpsScheduledReportService,
	backup *service.BackupService,
	quotaFlusher *service.UserPlatformQuotaUsageFlusher,
	paymentExpiry *service.PaymentOrderExpiryService,
	channelMonitor *service.ChannelMonitorRunner,
	channelMonitorV2 *service.ChannelMonitorV2Aggregator,
	upstreamBillingProbe *service.UpstreamBillingProbeService,
	ollamaCloudUsage *service.OllamaCloudUsageService,
) (*runtime.Lifecycle, error) {
	component := func(name string, roles []runtime.Role, start func(), stop func()) runtime.Component {
		return runtime.Component{
			Name:  name,
			Roles: roles,
			Start: func(context.Context) error { start(); return nil },
			Stop:  func(context.Context) error { stop(); return nil },
		}
	}
	manifest := []runtime.Component{
		{
			Name:  "timing-wheel",
			Roles: []runtime.Role{runtime.RoleAPI, runtime.RoleWorker, runtime.RoleScheduler},
			Start: func(context.Context) error { return timingWheel.Start() },
			Stop:  func(context.Context) error { timingWheel.Stop(); return nil },
		},
		component("deferred-last-used-flush", []runtime.Role{runtime.RoleAPI}, deferredService.Start, deferredService.Stop),
		component("ops-ingress-reject-aggregation", []runtime.Role{runtime.RoleAPI}, opsIngressRejectAggregator.Start, opsIngressRejectAggregator.Stop),
		component("oauth-session-workers", []runtime.Role{runtime.RoleAPI, runtime.RoleWorker, runtime.RoleScheduler}, func() {
			startOAuthServices(oauthService, openAIOAuthService, geminiOAuthService, antigravityOAuthService, grokOAuthService)
		}, func() {
			stopOAuthServices(oauthService, openAIOAuthService, geminiOAuthService, antigravityOAuthService, grokOAuthService)
		}),
		component("openai-websocket-pool", []runtime.Role{runtime.RoleAPI}, func() {}, openAIGatewayService.CloseOpenAIWSPool),
		{
			Name:  "concurrency-slot-cleanup",
			Roles: []runtime.Role{runtime.RoleAPI},
			Start: func(ctx context.Context) error {
				if err := concurrencyService.CleanupStaleProcessSlots(ctx); err != nil {
					log.Printf("stale concurrency slot cleanup failed: %v", err)
				}
				if cfg != nil {
					concurrencyService.StartSlotCleanupWorker(cfg.Gateway.Scheduling.SlotCleanupInterval)
				}
				return nil
			},
			Stop: func(context.Context) error {
				concurrencyService.StopSlotCleanupWorker()
				return nil
			},
		},
		component("user-message-lock-cleanup", []runtime.Role{runtime.RoleAPI}, func() {
			if cfg != nil {
				userMessageQueueService.StartCleanupWorker(time.Duration(cfg.Gateway.UserMessageQueue.CleanupIntervalSeconds) * time.Second)
			}
		}, userMessageQueueService.Stop),
		component("tls-fingerprint-profile-runtime", []runtime.Role{runtime.RoleAPI}, tlsFingerprintProfileService.Start, tlsFingerprintProfileService.Stop),
		component("error-passthrough-runtime", []runtime.Role{runtime.RoleAPI}, errorPassthroughService.Start, errorPassthroughService.Stop),
		component("api-key-cache-subscriber", []runtime.Role{runtime.RoleAPI}, func() { apiKeyService.StartAuthCacheInvalidationSubscriber(context.Background()) }, apiKeyService.StopAuthCacheInvalidationSubscriber),
		component("ops-runtime-settings", []runtime.Role{runtime.RoleAPI}, func() { opsService.StartRuntimeSettingsRefresh(context.Background()) }, opsService.StopRuntimeSettingsRefresh),
		component("ops-system-log-sink", []runtime.Role{runtime.RoleAPI}, func() { opsSystemLogSink.Start(); logger.SetSink(opsSystemLogSink) }, opsSystemLogSink.Stop),
		component("audit-log", []runtime.Role{runtime.RoleAPI}, auditLog.Start, auditLog.Stop),
		component("email-queue", []runtime.Role{runtime.RoleAPI}, emailQueue.Start, emailQueue.Stop),
		component("billing-cache-writers", []runtime.Role{runtime.RoleAPI}, billingCache.Start, billingCache.Stop),
		component("usage-record-worker-pool", []runtime.Role{runtime.RoleAPI}, usageRecordWorkerPool.Start, usageRecordWorkerPool.Stop),
		component("subscription-runtime", []runtime.Role{runtime.RoleAPI}, func() { subscriptionService.Start(context.Background()) }, subscriptionService.Stop),
		{
			Name:  "content-moderation-runtime",
			Roles: []runtime.Role{runtime.RoleAPI},
			Start: func(context.Context) error { contentModerationService.Start(); return nil },
			Stop:  contentModerationService.StopContext,
		},
		component("auth-cache-invalidation-worker", []runtime.Role{runtime.RoleWorker}, authCacheInvalidationWorker.Start, authCacheInvalidationWorker.Stop),
		component("batch-image-cleanup", []runtime.Role{runtime.RoleWorker}, batchImageCleanup.Start, batchImageCleanup.Stop),
		component("batch-image-worker", []runtime.Role{runtime.RoleWorker}, batchImageWorker.Start, batchImageWorker.Stop),
		{
			Name:  "prompt-audit-config",
			Roles: []runtime.Role{runtime.RoleAPI, runtime.RoleWorker},
			Start: func(ctx context.Context) error {
				if err := promptAudit.StartConfig(ctx); err != nil {
					log.Printf("Prompt audit configuration started in degraded mode: %v", err)
				}
				return nil
			},
			Stop: promptAudit.ShutdownConfig,
		},
		{
			Name:  "prompt-audit-runner",
			Roles: []runtime.Role{runtime.RoleWorker},
			Start: promptAudit.StartRunner,
			Stop:  promptAudit.ShutdownRunner,
		},
		{
			Name:  "pricing-initialization",
			Roles: []runtime.Role{runtime.RoleAPI, runtime.RoleScheduler},
			Start: func(context.Context) error {
				if err := pricingService.Initialize(); err != nil {
					log.Printf("pricing initialization failed: %v", err)
				}
				return nil
			},
			Stop: func(context.Context) error { return nil },
		},
		component("pricing-sync", []runtime.Role{runtime.RoleScheduler}, pricingService.Start, pricingService.Stop),
		component("token-refresh", []runtime.Role{runtime.RoleScheduler}, tokenRefresh.Start, tokenRefresh.Stop),
		component("scheduler-snapshot", []runtime.Role{runtime.RoleScheduler}, schedulerSnapshot.Start, schedulerSnapshot.Stop),
		component("dashboard-aggregation", []runtime.Role{runtime.RoleScheduler}, dashboardAggregation.Start, dashboardAggregation.Stop),
		component("usage-cleanup", []runtime.Role{runtime.RoleScheduler}, usageCleanup.Start, usageCleanup.Stop),
		component("account-expiry", []runtime.Role{runtime.RoleScheduler}, accountExpiry.Start, accountExpiry.Stop),
		component("codex-version-sync", []runtime.Role{runtime.RoleScheduler}, codexVersionSync.Start, codexVersionSync.Stop),
		component("proxy-expiry", []runtime.Role{runtime.RoleScheduler}, proxyExpiry.Start, proxyExpiry.Stop),
		component("subscription-expiry", []runtime.Role{runtime.RoleScheduler}, subscriptionExpiry.Start, subscriptionExpiry.Stop),
		component("ops-metrics", []runtime.Role{runtime.RoleScheduler}, opsMetrics.Start, opsMetrics.Stop),
		component("ops-aggregation", []runtime.Role{runtime.RoleScheduler}, opsAggregation.Start, opsAggregation.Stop),
		component("ops-alerts", []runtime.Role{runtime.RoleScheduler}, opsAlert.Start, opsAlert.Stop),
		component("ops-cleanup", []runtime.Role{runtime.RoleScheduler}, opsCleanup.Start, opsCleanup.Stop),
		component("idempotency-cleanup", []runtime.Role{runtime.RoleWorker}, idempotencyCleanup.Start, idempotencyCleanup.Stop),
		component("scheduled-tests", []runtime.Role{runtime.RoleScheduler}, scheduledTests.Start, scheduledTests.Stop),
		component("ops-reports", []runtime.Role{runtime.RoleScheduler}, opsReports.Start, opsReports.Stop),
		component("backup", []runtime.Role{runtime.RoleScheduler}, backup.Start, backup.Stop),
		component("quota-flusher", []runtime.Role{runtime.RoleWorker}, quotaFlusher.Start, quotaFlusher.Stop),
		component("payment-expiry", []runtime.Role{runtime.RoleScheduler}, paymentExpiry.Start, paymentExpiry.Stop),
		component("channel-monitor", []runtime.Role{runtime.RoleScheduler}, channelMonitor.Start, channelMonitor.Stop),
		component("channel-monitor-v2", []runtime.Role{runtime.RoleScheduler}, channelMonitorV2.Start, channelMonitorV2.Stop),
		component("upstream-billing-probe", []runtime.Role{runtime.RoleScheduler}, upstreamBillingProbe.Start, upstreamBillingProbe.Stop),
		component("ollama-cloud-usage", []runtime.Role{runtime.RoleScheduler}, ollamaCloudUsage.Start, ollamaCloudUsage.Stop),
		serverComponent,
	}
	lifecycle, err := runtime.NewLifecycle(manifest)
	if err != nil {
		return nil, fmt.Errorf("create lifecycle: %w", err)
	}
	return lifecycle, nil
}

func provideAPILifecycle(
	serverComponent runtime.Component,
	timingWheel *service.TimingWheelService,
	deferredService *service.DeferredService,
	opsIngressRejectAggregator *service.OpsIngressRejectAggregator,
	oauthService *service.OAuthService,
	openAIOAuthService *service.OpenAIOAuthService,
	geminiOAuthService *service.GeminiOAuthService,
	antigravityOAuthService *service.AntigravityOAuthService,
	grokOAuthService *service.GrokOAuthService,
	openAIGatewayService *service.OpenAIGatewayService,
	pricingService *service.PricingService,
	concurrencyService *service.ConcurrencyService,
	userMessageQueueService *service.UserMessageQueueService,
	tlsFingerprintProfileService *service.TLSFingerprintProfileService,
	errorPassthroughService *service.ErrorPassthroughService,
	cfg *config.Config,
	apiKeyService *service.APIKeyService,
	opsService *service.OpsService,
	opsSystemLogSink *service.OpsSystemLogSink,
	auditLog *service.AuditLogService,
	emailQueue *service.EmailQueueService,
	billingCache *service.BillingCacheService,
	usageRecordWorkerPool *service.UsageRecordWorkerPool,
	subscriptionService *service.SubscriptionService,
	contentModerationService *service.ContentModerationService,
	promptAudit *securityaudit.PromptService,
) (*runtime.Lifecycle, error) {
	component := func(name string, start func(), stop func()) runtime.Component {
		return runtime.Component{
			Name:  name,
			Roles: []runtime.Role{runtime.RoleAPI},
			Start: func(context.Context) error { start(); return nil },
			Stop:  func(context.Context) error { stop(); return nil },
		}
	}
	manifest := []runtime.Component{
		{
			Name:  "timing-wheel",
			Roles: []runtime.Role{runtime.RoleAPI},
			Start: func(context.Context) error { return timingWheel.Start() },
			Stop:  func(context.Context) error { timingWheel.Stop(); return nil },
		},
		component("deferred-last-used-flush", deferredService.Start, deferredService.Stop),
		component("ops-ingress-reject-aggregation", opsIngressRejectAggregator.Start, opsIngressRejectAggregator.Stop),
		component("oauth-session-workers", func() {
			startOAuthServices(oauthService, openAIOAuthService, geminiOAuthService, antigravityOAuthService, grokOAuthService)
		}, func() {
			stopOAuthServices(oauthService, openAIOAuthService, geminiOAuthService, antigravityOAuthService, grokOAuthService)
		}),
		component("openai-websocket-pool", func() {}, openAIGatewayService.CloseOpenAIWSPool),
		{
			Name:  "concurrency-slot-cleanup",
			Roles: []runtime.Role{runtime.RoleAPI},
			Start: func(ctx context.Context) error {
				if err := concurrencyService.CleanupStaleProcessSlots(ctx); err != nil {
					log.Printf("stale concurrency slot cleanup failed: %v", err)
				}
				if cfg != nil {
					concurrencyService.StartSlotCleanupWorker(cfg.Gateway.Scheduling.SlotCleanupInterval)
				}
				return nil
			},
			Stop: func(context.Context) error {
				concurrencyService.StopSlotCleanupWorker()
				return nil
			},
		},
		component("user-message-lock-cleanup", func() {
			if cfg != nil {
				userMessageQueueService.StartCleanupWorker(time.Duration(cfg.Gateway.UserMessageQueue.CleanupIntervalSeconds) * time.Second)
			}
		}, userMessageQueueService.Stop),
		component("tls-fingerprint-profile-runtime", tlsFingerprintProfileService.Start, tlsFingerprintProfileService.Stop),
		component("error-passthrough-runtime", errorPassthroughService.Start, errorPassthroughService.Stop),
		component("api-key-cache-subscriber", func() { apiKeyService.StartAuthCacheInvalidationSubscriber(context.Background()) }, apiKeyService.StopAuthCacheInvalidationSubscriber),
		component("ops-runtime-settings", func() { opsService.StartRuntimeSettingsRefresh(context.Background()) }, opsService.StopRuntimeSettingsRefresh),
		component("ops-system-log-sink", func() { opsSystemLogSink.Start(); logger.SetSink(opsSystemLogSink) }, opsSystemLogSink.Stop),
		component("audit-log", auditLog.Start, auditLog.Stop),
		component("email-queue", emailQueue.Start, emailQueue.Stop),
		component("billing-cache-writers", billingCache.Start, billingCache.Stop),
		component("usage-record-worker-pool", usageRecordWorkerPool.Start, usageRecordWorkerPool.Stop),
		component("subscription-runtime", func() { subscriptionService.Start(context.Background()) }, subscriptionService.Stop),
		{
			Name:  "content-moderation-runtime",
			Roles: []runtime.Role{runtime.RoleAPI},
			Start: func(context.Context) error { contentModerationService.Start(); return nil },
			Stop:  contentModerationService.StopContext,
		},
		{
			Name:  "prompt-audit-config",
			Roles: []runtime.Role{runtime.RoleAPI},
			Start: func(ctx context.Context) error {
				if err := promptAudit.StartConfig(ctx); err != nil {
					log.Printf("Prompt audit configuration started in degraded mode: %v", err)
				}
				return nil
			},
			Stop: promptAudit.ShutdownConfig,
		},
		{
			Name:  "pricing-initialization",
			Roles: []runtime.Role{runtime.RoleAPI},
			Start: func(context.Context) error {
				if err := pricingService.Initialize(); err != nil {
					log.Printf("pricing initialization failed: %v", err)
				}
				return nil
			},
			Stop: func(context.Context) error { return nil },
		},
		serverComponent,
	}
	return newRoleLifecycle(runtime.RoleAPI, manifest)
}

func provideWorkerLifecycle(
	timingWheel *service.TimingWheelService,
	authCacheInvalidationWorker *service.AuthCacheInvalidationWorker,
	batchImageCleanup *service.BatchImageCleanupService,
	batchImageWorker *service.BatchImageWorkerRuntime,
	promptAudit *securityaudit.PromptService,
	idempotencyCleanup *service.IdempotencyCleanupService,
	quotaFlusher *service.UserPlatformQuotaUsageFlusher,
) (*runtime.Lifecycle, error) {
	component := func(name string, start func(), stop func()) runtime.Component {
		return runtime.Component{
			Name:  name,
			Roles: []runtime.Role{runtime.RoleWorker},
			Start: func(context.Context) error { start(); return nil },
			Stop:  func(context.Context) error { stop(); return nil },
		}
	}
	manifest := []runtime.Component{
		{
			Name:  "timing-wheel",
			Roles: []runtime.Role{runtime.RoleWorker},
			Start: func(context.Context) error { return timingWheel.Start() },
			Stop:  func(context.Context) error { timingWheel.Stop(); return nil },
		},
		component("auth-cache-invalidation-worker", authCacheInvalidationWorker.Start, authCacheInvalidationWorker.Stop),
		component("batch-image-cleanup", batchImageCleanup.Start, batchImageCleanup.Stop),
		component("batch-image-worker", batchImageWorker.Start, batchImageWorker.Stop),
		{
			Name:  "prompt-audit-config",
			Roles: []runtime.Role{runtime.RoleWorker},
			Start: func(ctx context.Context) error {
				if err := promptAudit.StartConfig(ctx); err != nil {
					log.Printf("Prompt audit configuration started in degraded mode: %v", err)
				}
				return nil
			},
			Stop: promptAudit.ShutdownConfig,
		},
		{
			Name:  "prompt-audit-runner",
			Roles: []runtime.Role{runtime.RoleWorker},
			Start: promptAudit.StartRunner,
			Stop:  promptAudit.ShutdownRunner,
		},
		component("idempotency-cleanup", idempotencyCleanup.Start, idempotencyCleanup.Stop),
		component("quota-flusher", quotaFlusher.Start, quotaFlusher.Stop),
	}
	return newRoleLifecycle(runtime.RoleWorker, manifest)
}

func provideSchedulerLifecycle(
	timingWheel *service.TimingWheelService,
	oauthService *service.OAuthService,
	openAIOAuthService *service.OpenAIOAuthService,
	geminiOAuthService *service.GeminiOAuthService,
	antigravityOAuthService *service.AntigravityOAuthService,
	grokOAuthService *service.GrokOAuthService,
	pricingService *service.PricingService,
	tokenRefresh *service.TokenRefreshService,
	schedulerSnapshot *service.SchedulerSnapshotService,
	dashboardAggregation *service.DashboardAggregationService,
	usageCleanup *service.UsageCleanupService,
	accountExpiry *service.AccountExpiryService,
	codexVersionSync *service.OpenAICodexVersionSyncService,
	proxyExpiry *service.ProxyExpiryService,
	subscriptionExpiry *service.SubscriptionExpiryService,
	opsMetrics *service.OpsMetricsCollector,
	opsAggregation *service.OpsAggregationService,
	opsAlert *service.OpsAlertEvaluatorService,
	opsCleanup *service.OpsCleanupService,
	scheduledTests *service.ScheduledTestRunnerService,
	opsReports *service.OpsScheduledReportService,
	backup *service.BackupService,
	paymentExpiry *service.PaymentOrderExpiryService,
	channelMonitor *service.ChannelMonitorRunner,
	channelMonitorV2 *service.ChannelMonitorV2Aggregator,
	upstreamBillingProbe *service.UpstreamBillingProbeService,
	ollamaCloudUsage *service.OllamaCloudUsageService,
) (*runtime.Lifecycle, error) {
	component := func(name string, start func(), stop func()) runtime.Component {
		return runtime.Component{
			Name:  name,
			Roles: []runtime.Role{runtime.RoleScheduler},
			Start: func(context.Context) error { start(); return nil },
			Stop:  func(context.Context) error { stop(); return nil },
		}
	}
	manifest := []runtime.Component{
		{
			Name:  "timing-wheel",
			Roles: []runtime.Role{runtime.RoleScheduler},
			Start: func(context.Context) error { return timingWheel.Start() },
			Stop:  func(context.Context) error { timingWheel.Stop(); return nil },
		},
		component("oauth-session-workers", func() {
			startOAuthServices(oauthService, openAIOAuthService, geminiOAuthService, antigravityOAuthService, grokOAuthService)
		}, func() {
			stopOAuthServices(oauthService, openAIOAuthService, geminiOAuthService, antigravityOAuthService, grokOAuthService)
		}),
		{
			Name:  "pricing-initialization",
			Roles: []runtime.Role{runtime.RoleScheduler},
			Start: func(context.Context) error {
				if err := pricingService.Initialize(); err != nil {
					log.Printf("pricing initialization failed: %v", err)
				}
				return nil
			},
			Stop: func(context.Context) error { return nil },
		},
		component("pricing-sync", pricingService.Start, pricingService.Stop),
		component("token-refresh", tokenRefresh.Start, tokenRefresh.Stop),
		component("scheduler-snapshot", schedulerSnapshot.Start, schedulerSnapshot.Stop),
		component("dashboard-aggregation", dashboardAggregation.Start, dashboardAggregation.Stop),
		component("usage-cleanup", usageCleanup.Start, usageCleanup.Stop),
		component("account-expiry", accountExpiry.Start, accountExpiry.Stop),
		component("codex-version-sync", codexVersionSync.Start, codexVersionSync.Stop),
		component("proxy-expiry", proxyExpiry.Start, proxyExpiry.Stop),
		component("subscription-expiry", subscriptionExpiry.Start, subscriptionExpiry.Stop),
		component("ops-metrics", opsMetrics.Start, opsMetrics.Stop),
		component("ops-aggregation", opsAggregation.Start, opsAggregation.Stop),
		component("ops-alerts", opsAlert.Start, opsAlert.Stop),
		component("ops-cleanup", opsCleanup.Start, opsCleanup.Stop),
		component("scheduled-tests", scheduledTests.Start, scheduledTests.Stop),
		component("ops-reports", opsReports.Start, opsReports.Stop),
		component("backup", backup.Start, backup.Stop),
		component("payment-expiry", paymentExpiry.Start, paymentExpiry.Stop),
		component("channel-monitor", channelMonitor.Start, channelMonitor.Stop),
		component("channel-monitor-v2", channelMonitorV2.Start, channelMonitorV2.Stop),
		component("upstream-billing-probe", upstreamBillingProbe.Start, upstreamBillingProbe.Stop),
		component("ollama-cloud-usage", ollamaCloudUsage.Start, ollamaCloudUsage.Stop),
	}
	return newRoleLifecycle(runtime.RoleScheduler, manifest)
}

func startOAuthServices(
	oauthService *service.OAuthService,
	openAIOAuthService *service.OpenAIOAuthService,
	geminiOAuthService *service.GeminiOAuthService,
	antigravityOAuthService *service.AntigravityOAuthService,
	grokOAuthService *service.GrokOAuthService,
) {
	oauthService.Start()
	openAIOAuthService.Start()
	geminiOAuthService.Start()
	antigravityOAuthService.Start()
	grokOAuthService.Start()
}

func stopOAuthServices(
	oauthService *service.OAuthService,
	openAIOAuthService *service.OpenAIOAuthService,
	geminiOAuthService *service.GeminiOAuthService,
	antigravityOAuthService *service.AntigravityOAuthService,
	grokOAuthService *service.GrokOAuthService,
) {
	oauthService.Stop()
	openAIOAuthService.Stop()
	geminiOAuthService.Stop()
	antigravityOAuthService.Stop()
	grokOAuthService.Stop()
}

func newRoleLifecycle(role runtime.Role, manifest []runtime.Component) (*runtime.Lifecycle, error) {
	lifecycle, err := runtime.NewLifecycle(manifest)
	if err != nil {
		return nil, fmt.Errorf("create %s lifecycle: %w", role, err)
	}
	return lifecycle, nil
}
