//go:build wireinject
// +build wireinject

package main

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/pluginhost"
	"github.com/Wei-Shaw/sub2api/internal/pluginkit"
	"github.com/Wei-Shaw/sub2api/internal/plugins"
	"github.com/Wei-Shaw/sub2api/internal/plugins/moderation"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/server"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"

	"github.com/google/wire"
	"github.com/redis/go-redis/v9"
)

type Application struct {
	Server *http.Server
	// PluginManager 暴露给 main：在 server 监听前调用 Bootstrap 完成插件装配。
	PluginManager *pluginkit.Manager
	// PluginSupervisor 暴露给 main：在 Bootstrap 之后 Start，
	// 拉起已启用的外部插件子进程（phase-4）。
	PluginSupervisor *pluginhost.Supervisor
	Cleanup          func()
}

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

		// Plugin runtime (内建层插件内核)
		providePluginHostDeps,
		providePluginsConfig,
		// 内容审计句柄：热路径/后台 handler 与 content-moderation 插件共享的
		// 可切换引用（类型随搬家移入 moderation 包，provider 在此装配）。
		moderation.NewContentModerationHandle,
		provideBuiltinFactories,
		providePluginManager,

		// External plugin layer (外部插件层：子进程宿主 + 安装器，phase-4)
		providePluginSupervisor,
		provideExternalPluginLayer,

		// Cleanup function provider
		provideCleanup,

		// Application struct
		wire.Struct(new(Application), "Server", "PluginManager", "PluginSupervisor", "Cleanup"),
	)
	return nil, nil
}

// providePluginHostDeps 装配各插件 Host 所需的宿主侧依赖。
// Logger 取全局 slog（main 在 initializeApplication 之前已完成 logger.Init）。
func providePluginHostDeps(entClient *ent.Client, rdb *redis.Client) pluginkit.HostDeps {
	return pluginkit.HostDeps{
		Logger: slog.Default(),
		DB:     entClient,
		Redis:  rdb,
	}
}

// providePluginsConfig 从主配置的 plugins 原始子树还原点分插件 ID 的私有配置。
func providePluginsConfig(cfg *config.Config) (pluginkit.PluginsConfig, error) {
	return pluginkit.ParsePluginsConfig(cfg.PluginsRaw)
}

// provideBuiltinFactories 汇总全部编译期装配的插件工厂：内建示例 + 功能域
// 插件（内容审计）。作为管理器与外部层保留 ID 集合的单一事实源。
// 拆分口径（用户决策）：插件 = 按功能域整体竖切；基础设施型后台 worker
// （账号/代理过期、幂等清理等）不是插件，由 Wire 直接管理常驻生命周期。
func provideBuiltinFactories(
	settingRepo service.SettingRepository,
	moderationRepo moderation.ContentModerationRepository,
	moderationHashCache moderation.ContentModerationHashCache,
	groupRepo service.GroupRepository,
	userRepo service.UserRepository,
	authCacheInvalidator service.APIKeyAuthCacheInvalidator,
	emailService *service.EmailService,
	moderationHandle *moderation.ContentModerationHandle,
) []pluginkit.Factory {
	return append(plugins.Builtin(), moderation.New(moderation.Deps{
		SettingRepo:          settingRepo,
		Repo:                 moderationRepo,
		HashCache:            moderationHashCache,
		GroupRepo:            groupRepo,
		UserRepo:             userRepo,
		AuthCacheInvalidator: authCacheInvalidator,
		EmailService:         emailService,
		Handle:               moderationHandle,
	}))
}

// providePluginManager 用编译期装配清单实例化插件生命周期驱动器。
// 此处仅实例化（Factory 无副作用），Bootstrap 由 main 在 server 监听前调用。
func providePluginManager(hostDeps pluginkit.HostDeps, states pluginkit.StateStore, pluginsCfg pluginkit.PluginsConfig, factories []pluginkit.Factory) (*pluginkit.Manager, error) {
	return pluginkit.NewManager(hostDeps, states, pluginsCfg, factories)
}

// providePluginSupervisor 实例化外部插件子进程宿主（无副作用，
// Start 由 main 在 PluginManager.Bootstrap 之后调用）。
// 响应头过滤复用核心网关的白名单配置（phase-4 风险对策 3）。
func providePluginSupervisor(
	installs pluginhost.InstallationStore,
	kv pluginhost.KVStore,
	states pluginkit.StateStore,
	cfg *config.Config,
	buildInfo handler.BuildInfo,
) *pluginhost.Supervisor {
	return pluginhost.NewSupervisor(pluginhost.SupervisorDeps{
		Installs:     installs,
		States:       states,
		KV:           kv,
		Logger:       slog.Default(),
		HeaderFilter: responseheaders.CompileHeaderFilter(cfg.Security.ResponseHeaders),
		HostVersion:  buildInfo.Version,
	})
}

// provideExternalPluginLayer 装配外部插件层：包存储（DATA_DIR/plugins）+
// 安装器（Supervisor 作为 ExternalRuntime 接缝、内建清单为保留 ID 集合）。
func provideExternalPluginLayer(
	supervisor *pluginhost.Supervisor,
	installs pluginhost.InstallationStore,
	kv pluginhost.KVStore,
	states pluginkit.StateStore,
	factories []pluginkit.Factory,
	buildInfo handler.BuildInfo,
) *pluginhost.ExternalLayer {
	installer := pluginhost.NewInstaller(pluginhost.InstallerDeps{
		Store:         pluginhost.NewPackageStore(pluginhost.DefaultStoreRoot()),
		Installations: installs,
		States:        states,
		Runtime:       supervisor,
		KV:            kv,
		Reserved:      pluginhost.ReservedIDs(factories),
		HostVersion:   buildInfo.Version,
		Logger:        slog.Default(),
	})
	return &pluginhost.ExternalLayer{
		Installer:  installer,
		Installs:   installs,
		Supervisor: supervisor,
	}
}

func providePrivacyClientFactory() service.PrivacyClientFactory {
	return repository.CreatePrivacyReqClient
}

func provideServiceBuildInfo(buildInfo handler.BuildInfo) service.BuildInfo {
	return service.BuildInfo{
		Version:   buildInfo.Version,
		BuildType: buildInfo.BuildType,
	}
}

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
	batchImageCleanup *service.BatchImageCleanupService,
	batchImageWorker *service.BatchImageWorkerRuntime,
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
	pluginManager *pluginkit.Manager,
	pluginStates pluginkit.StateStore,
	pluginSupervisor *pluginhost.Supervisor,
) func() {
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		type cleanupStep struct {
			name string
			fn   func() error
		}

		// 应用层清理步骤可并行执行，基础设施资源（Redis/Ent）最后按顺序关闭。
		parallelSteps := []cleanupStep{
			{"OpsScheduledReportService", func() error {
				if opsScheduledReport != nil {
					opsScheduledReport.Stop()
				}
				return nil
			}},
			{"OpsCleanupService", func() error {
				if opsCleanup != nil {
					opsCleanup.Stop()
				}
				return nil
			}},
			{"OpsSystemLogSink", func() error {
				if opsSystemLogSink != nil {
					opsSystemLogSink.Stop()
				}
				return nil
			}},
			{"OpsAlertEvaluatorService", func() error {
				if opsAlertEvaluator != nil {
					opsAlertEvaluator.Stop()
				}
				return nil
			}},
			{"OpsAggregationService", func() error {
				if opsAggregation != nil {
					opsAggregation.Stop()
				}
				return nil
			}},
			{"OpsMetricsCollector", func() error {
				if opsMetricsCollector != nil {
					opsMetricsCollector.Stop()
				}
				return nil
			}},
			{"SchedulerSnapshotService", func() error {
				if schedulerSnapshot != nil {
					schedulerSnapshot.Stop()
				}
				return nil
			}},
			{"UsageCleanupService", func() error {
				if usageCleanup != nil {
					usageCleanup.Stop()
				}
				return nil
			}},
			{"IdempotencyCleanupService", func() error {
				if idempotencyCleanup != nil {
					idempotencyCleanup.Stop()
				}
				return nil
			}},
			{"BatchImageCleanupService", func() error {
				if batchImageCleanup != nil {
					batchImageCleanup.Stop()
				}
				return nil
			}},
			{"BatchImageWorkerRuntime", func() error {
				if batchImageWorker != nil {
					batchImageWorker.Stop()
				}
				return nil
			}},
			{"TokenRefreshService", func() error {
				tokenRefresh.Stop()
				return nil
			}},
			{"AccountExpiryService", func() error {
				accountExpiry.Stop()
				return nil
			}},
			{"ProxyExpiryService", func() error {
				proxyExpiry.Stop()
				return nil
			}},
			{"SubscriptionExpiryService", func() error {
				subscriptionExpiry.Stop()
				return nil
			}},
			{"SubscriptionService", func() error {
				if subscriptionService != nil {
					subscriptionService.Stop()
				}
				return nil
			}},
			{"PricingService", func() error {
				pricing.Stop()
				return nil
			}},
			{"EmailQueueService", func() error {
				emailQueue.Stop()
				return nil
			}},
			{"BillingCacheService", func() error {
				billingCache.Stop()
				return nil
			}},
			{"UsageRecordWorkerPool", func() error {
				if usageRecordWorkerPool != nil {
					usageRecordWorkerPool.Stop()
				}
				return nil
			}},
			{"OAuthService", func() error {
				oauth.Stop()
				return nil
			}},
			{"OpenAIOAuthService", func() error {
				openaiOAuth.Stop()
				return nil
			}},
			{"GeminiOAuthService", func() error {
				geminiOAuth.Stop()
				return nil
			}},
			{"AntigravityOAuthService", func() error {
				antigravityOAuth.Stop()
				return nil
			}},
			{"GrokOAuthService", func() error {
				if grokOAuth != nil {
					grokOAuth.Stop()
				}
				return nil
			}},
			{"OpenAIWSPool", func() error {
				if openAIGateway != nil {
					openAIGateway.CloseOpenAIWSPool()
				}
				return nil
			}},
			{"ScheduledTestRunnerService", func() error {
				if scheduledTestRunner != nil {
					scheduledTestRunner.Stop()
				}
				return nil
			}},
			{"BackupService", func() error {
				if backupSvc != nil {
					backupSvc.Stop()
				}
				return nil
			}},
			{"PaymentOrderExpiryService", func() error {
				if paymentOrderExpiry != nil {
					paymentOrderExpiry.Stop()
				}
				return nil
			}},
			{"ChannelMonitorRunner", func() error {
				if channelMonitorRunner != nil {
					channelMonitorRunner.Stop()
				}
				return nil
			}},
			{"UserPlatformQuotaUsageFlusher", func() error {
				if quotaFlusher != nil {
					quotaFlusher.Stop()
				}
				return nil
			}},
			{"PluginRuntime", func() error {
				// 先关停启停状态机（订阅与对账循环），再停外部插件子进程与内建
				// 插件：否则 StopAll 期间到达的跨实例 toggle 会把刚停的插件复活。
				// Close 只停后台循环，不影响 StopAll 内部对 SetEnabled 的调用
				//（crash-loop 自禁路径）。
				var errs []error
				if pluginStates != nil {
					errs = append(errs, pluginStates.Close())
				}
				if pluginSupervisor != nil {
					errs = append(errs, pluginSupervisor.StopAll(ctx))
				}
				if pluginManager != nil {
					errs = append(errs, pluginManager.StopAll(ctx))
				}
				return errors.Join(errs...)
			}},
		}

		infraSteps := []cleanupStep{
			{"Redis", func() error {
				if rdb == nil {
					return nil
				}
				return rdb.Close()
			}},
			{"Ent", func() error {
				if entClient == nil {
					return nil
				}
				return entClient.Close()
			}},
		}

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
					}
					log.Printf("[Cleanup] %s succeeded", step.name)
				}()
			}
			wg.Wait()
		}

		runSequential := func(steps []cleanupStep) {
			for i := range steps {
				step := steps[i]
				if err := step.fn(); err != nil {
					log.Printf("[Cleanup] %s failed: %v", step.name, err)
					continue
				}
				log.Printf("[Cleanup] %s succeeded", step.name)
			}
		}

		runParallel(parallelSteps)
		runSequential(infraSteps)

		// Check if context timed out
		select {
		case <-ctx.Done():
			log.Printf("[Cleanup] Warning: cleanup timed out after 10 seconds")
		default:
			log.Printf("[Cleanup] All cleanup steps completed")
		}
	}
}
