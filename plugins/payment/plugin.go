// Package main is the payment plugin entry point. plugin.go owns the
// PaymentPlugin struct + Manifest + Init/Shutdown lifecycle hooks; the
// static manifest data (endpoints, capabilities, owned tables, migrations,
// menu items, routes, settings schema) lives in manifest_decls.go.
package main

import (
	"context"
	"embed"
	"io/fs"
	"path"
	"sync/atomic"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/gin-gonic/gin"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
	pluginent "github.com/Wei-Shaw/sub2api/plugins/payment/ent"
	"github.com/Wei-Shaw/sub2api/plugins/payment/handler"
	paymentpkg "github.com/Wei-Shaw/sub2api/plugins/payment/internal/payment"
	"github.com/Wei-Shaw/sub2api/plugins/payment/service"
)

const (
	pluginName        = "payment"
	pluginDisplayName = "Payment"
	pluginVersion     = "0.1.0"
	pluginDescription = "Payment orders, providers, fulfillment, refunds and webhooks"

	// userRoutePrefix and adminRoutePrefix are the prefixes the core
	// gateway uses when proxying plugin endpoints to this plugin. They
	// match the pre-migration host paths so external integrations
	// (Stripe Dashboard webhook URLs, Alipay/WeChat merchant notifyUrl
	// fields) keep working without operator action. The host's
	// per-plugin namespace gate accepts both prefixes for plugin
	// "payment" — see backend/internal/plugin/manager_routes.go
	// pluginOwnNamespacePrefixes.
	userRoutePrefix  = "/api/v1/" + pluginName
	adminRoutePrefix = "/api/v1/admin/" + pluginName

	// orderExpiryJobInterval defines how often the leader replica checks
	// for timed-out PENDING orders and transitions them to EXPIRED. The
	// 60s cadence matches the legacy in-process ticker the plugin
	// replaces (PaymentOrderExpiryService). Concurrency=1 + LeaderOnly
	// guarantees a single replica runs the sweep regardless of fleet size.
	orderExpiryJobInterval = 60 * time.Second

	// orderExpiryJobName is the JobScheduler name. Re-registering with
	// the same name is idempotent on the host side.
	orderExpiryJobName = "payment.expire-orders"
)

// frontendAssets embeds the payment plugin's frontend bundle. CI / local
// build must produce frontend/dist/entry.js + entry.css before this
// embed becomes useful (Phase 2 will replace the stub bundle shipped
// today).
//
//go:embed all:frontend/dist
var frontendAssets embed.FS

// migrationFiles embeds the SQL migration files so the host can fetch them
// via PluginLifecycle.GetMigration and apply them under the
// MigrationRunner advisory-lock + plugin_migrations bookkeeping path.
//
//go:embed migrations/*.sql
var migrationFiles embed.FS

// settingsSchemaJSON / settingsDefaultsJSON embed the payment plugin's
// settings schema + defaults declared via the SettingsExtensionCapability.
//
//go:embed internal/settings/settings_schema.json
var settingsSchemaJSON []byte

//go:embed internal/settings/settings_defaults.json
var settingsDefaultsJSON []byte

// PaymentPlugin is the payment plugin entry point. It owns the plugin's
// service / repository wiring and exposes a Gin engine to the SDK.
type PaymentPlugin struct {
	ctx atomic.Pointer[pluginsdk.PluginContext]

	// entClient is the ent ORM client backed by the SDK's SQL proxy.
	entClient *pluginent.Client

	// engine is the Gin HTTP engine the SDK mounts at "/" so the plugin
	// can register handlers under both userRoutePrefix and adminRoutePrefix.
	// Built once at construction time so that RegisterHTTP (called before
	// Init) can hand it to the SDK and Init can plug services into it.
	engine *gin.Engine

	// Services and handlers are constructed in Init and wired into the
	// engine by registerRoutes. Tests / shutdown can refer to them via
	// these fields without re-running the wiring helpers.
	paymentService *service.PaymentService
	configService  *service.PaymentConfigService

	userHandler    *handler.PaymentHandler
	webhookHandler *handler.PaymentWebhookHandler
	adminHandler   *handler.AdminPaymentHandler
}

// Manifest declares the plugin's capabilities to the core. Static data
// lives in manifest_decls.go so this body stays a thin assembler — adding
// an endpoint, menu item or migration means editing the matching slice
// there, not the Manifest() function.
func (p *PaymentPlugin) Manifest() *pluginsdk.Manifest {
	return &pluginsdk.Manifest{
		Name:            pluginName,
		DisplayName:     pluginDisplayName,
		Version:         pluginVersion,
		Description:     pluginDescription,
		Author:          "Sub2API",
		IconSVG:         pluginsdk.IconCog,
		PluginEndpoints: endpointDecls,
		Capabilities:    capabilityDecls,
		OwnedTables:     ownedTableDecls,
		Migrations:      migrationDecls,
		PublicFlags:     publicFlagDecls,
		SettingsSchema: &pluginsdk.SettingsSchemaDoc{
			Schema:   settingsSchemaJSON,
			Defaults: settingsDefaultsJSON,
			Version:  "1.0.0",
		},
		// Settings UI lives at the dedicated /admin/payment/settings route
		// (see manifest_decls.go menuItemDecls + routeDecls). The plugin
		// management dialog no longer mounts a custom settings component.
		Frontend: &pluginsdk.FrontendManifest{
			EntryJS:        "dist/entry.js",
			EntryCSS:       "dist/entry.css",
			MenuItems:      menuItemDecls,
			Routes:         routeDecls,
			I18nNamespaces: []string{"payment"},
		},
	}
}

// Init wires the repository, service and handler layers using the SDK's
// DB / Redis / Settings / Host clients. Heavy wiring will live in helper
// methods (see channel-management for the pattern).
//
// TODO(payment-migration): full wiring of repository + 31 service files +
// handlers is staged across follow-up commits. This Init currently sets up
// the ent client, registers the order-expiry job, and stores the context
// pointer so HealthCheck can report ready.
func (p *PaymentPlugin) Init(ctx pluginsdk.PluginContext) error {
	p.ctx.Store(&ctx)

	// Initialise the ent ORM client using the SDK's SQL proxy driver. The
	// SDK driver transparently proxies queries through gRPC to the core's
	// connection pool; entsql.OpenDB wraps it with ent's dialect-specific
	// query builder so payment_* services share a single *pluginent.Client.
	drv := entsql.OpenDB(pluginsdk.Dialect, ctx.DB())
	p.entClient = pluginent.NewClient(pluginent.Driver(drv))
	ctx.Logger().Info("payment: ent client initialised", "dialect", pluginsdk.Dialect)

	p.buildServices(ctx)
	p.buildHandlers(ctx)

	if err := p.registerOrderExpiryJob(ctx); err != nil {
		ctx.Logger().Warn("payment: order-expiry job registration failed; expiry sweep disabled",
			"error", err)
	}

	p.registerRoutes()

	ctx.Logger().Info("payment plugin initialised", "version", pluginVersion)
	return nil
}

// buildServices constructs the PaymentConfigService and PaymentService
// from the SDK-provided clients. HostPayment() returns the payment
// extension surface directly; if the host has not registered the
// extension the nilHostClient still satisfies the interface and
// surfaces a clear ErrHost*Unavailable from each call site.
func (p *PaymentPlugin) buildServices(ctx pluginsdk.PluginContext) {
	hostPayment := ctx.HostPayment()

	p.configService = service.NewPaymentConfigService(
		ctx.Settings(),
		ctx.Secrets(),
		p.entClient,
		ctx.DB(),
		ctx.Logger(),
	)
	// Provider registry stays empty until the first SelectInstance call
	// hydrates it from the DB. The load balancer is the actual gate
	// guarding errPaymentServiceUnavailable, so wiring just these two is
	// enough for the read/write paths to leave the 503 fast-path. The
	// legacy AES-key fallback in decryptConfig is the only consumer of
	// the encryptionKey arg; modern records are plaintext JSON, so a nil
	// key is the right default for new deployments.
	loadBalancer := paymentpkg.NewDefaultLoadBalancer(p.entClient, nil)
	registry := paymentpkg.NewRegistry()

	p.paymentService = service.NewPaymentService(service.PaymentServiceDeps{
		EntClient:    p.entClient,
		Registry:     registry,
		LoadBalancer: loadBalancer,
		Config:       p.configService,
		Host:         hostPayment,
		Events:       ctx.Events(),
		Secrets:      ctx.Secrets(),
		Settings:     ctx.Settings(),
		Logger:       ctx.Logger(),
	})
}

// buildHandlers constructs the three handler facets (user, webhook,
// admin). Each handler is a thin protocol adapter over the services
// already built in buildServices, so this is just plumbing.
//
// The webhook handler additionally needs the SDK Redis client to power
// notification-replay protection (see handler/webhook_dedup.go); we read
// it off the plugin context here so the handler stays unaware of the SDK
// shape.
func (p *PaymentPlugin) buildHandlers(ctx pluginsdk.PluginContext) {
	p.userHandler = handler.NewPaymentHandler(p.paymentService, p.configService)
	p.webhookHandler = handler.NewPaymentWebhookHandler(
		p.paymentService,
		p.paymentService.Registry(),
		ctx.Redis(),
		ctx.Logger(),
	)
	p.adminHandler = handler.NewAdminPaymentHandler(p.paymentService, p.configService)
}

// registerOrderExpiryJob installs the periodic order-expiry sweep that
// replaces the legacy in-process PaymentOrderExpiryService ticker. The
// job is leader-only and concurrency-1 so a single replica drives the
// sweep regardless of fleet size.
func (p *PaymentPlugin) registerOrderExpiryJob(ctx pluginsdk.PluginContext) error {
	jobs := ctx.Jobs()
	if jobs == nil {
		ctx.Logger().Warn("payment: JobScheduler unavailable; order-expiry job disabled")
		return nil
	}
	spec := pluginsdk.JobSpec{
		Name:        orderExpiryJobName,
		Trigger:     pluginsdk.JobTrigger{Kind: pluginsdk.TriggerInterval, Interval: orderExpiryJobInterval},
		Concurrency: 1,
		LeaderOnly:  true,
	}
	return jobs.Register(spec, p.handleOrderExpiry)
}

// handleOrderExpiry is invoked once per orderExpiryJobInterval on the
// leader replica. It delegates the actual sweep to the payment service
// and emits a single info log when the sweep actually expired anything,
// so operators see batch counts without flooding the log every minute.
func (p *PaymentPlugin) handleOrderExpiry(ctx context.Context, _ string) error {
	if p.paymentService == nil {
		return nil
	}
	expired, err := p.paymentService.ExpireTimedOutOrders(ctx)
	if err != nil {
		return err
	}
	if expired > 0 {
		if c := p.ctx.Load(); c != nil {
			(*c).Logger().Info("payment: expired orders", "count", expired)
		}
	}
	return nil
}

// Shutdown releases plugin-owned resources. Called once by the SDK when
// the host signals a graceful stop. Closing the ent client releases the
// ent driver wrapper but does NOT close the SDK-managed *sql.DB — the
// SDK runner owns that lifecycle.
func (p *PaymentPlugin) Shutdown() error {
	if c := p.ctx.Load(); c != nil {
		(*c).Logger().Info("payment plugin shutting down")
	}
	if p.entClient != nil {
		return p.entClient.Close()
	}
	return nil
}

// RegisterHTTP hands the Gin engine to the SDK. We mount on "/" so the
// engine sees the full request path — the plugin registers handlers at
// the original /api/v1/payment/* and /api/v1/admin/payment/* prefixes
// directly (see registerRoutes), and the host's reverse proxy forwards
// the inbound URL untouched.
func (p *PaymentPlugin) RegisterHTTP(mux pluginsdk.HTTPMux) {
	if p.engine == nil {
		gin.SetMode(gin.ReleaseMode)
		p.engine = gin.New()
		p.engine.Use(gin.Recovery())
	}
	mux.Handle("/", p.engine)
}

// registerRoutes attaches the payment routes to the Gin engine. Called
// from Init once handlers are constructed. The route layout mirrors the
// pre-migration host paths exactly so external integrations (Stripe
// Dashboard webhooks, Alipay/WeChat notifyUrls) and the existing admin
// frontend keep working without any URL changes.
func (p *PaymentPlugin) registerRoutes() {
	if p.engine == nil {
		// RegisterHTTP was not called (e.g. --no-http); skip routing.
		return
	}
	user := p.engine.Group(userRoutePrefix)
	if p.userHandler != nil {
		p.userHandler.RegisterRoutes(user)
		p.userHandler.RegisterPublicRoutes(user.Group("/public"))
	}
	if p.webhookHandler != nil {
		p.webhookHandler.RegisterRoutes(user.Group("/webhook"))
	}
	if p.adminHandler != nil {
		p.adminHandler.RegisterRoutes(p.engine.Group(adminRoutePrefix))
	}
}

// HealthCheck reports unhealthy until the plugin context is wired so the
// core can distinguish a freshly-spawned process from a fully-initialised
// one.
func (p *PaymentPlugin) HealthCheck() (bool, string) {
	if p.ctx.Load() == nil {
		return false, "plugin not yet initialised"
	}
	return true, "ok"
}

// OpenMigration implements pluginsdk.MigrationProvider so the SDK runner
// can serve migration bodies to the host's MigrationRunner via
// PluginLifecycle.GetMigration.
func (p *PaymentPlugin) OpenMigration(filename string) ([]byte, error) {
	return migrationFiles.ReadFile("migrations/" + filename)
}

// OpenFrontendFile implements pluginsdk.FrontendBundleProvider so the
// core can fetch frontend assets over gRPC.
func (p *PaymentPlugin) OpenFrontendFile(rel string) ([]byte, error) {
	clean := path.Clean("/" + rel)
	if clean == "/" || clean == "/." {
		return nil, fs.ErrInvalid
	}
	clean = clean[1:] // strip leading "/"
	full := "frontend/" + clean
	return frontendAssets.ReadFile(full)
}
