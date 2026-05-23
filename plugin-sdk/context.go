package pluginsdk

import (
	"database/sql"
	"log/slog"

	sdkdriver "github.com/Wei-Shaw/sub2api/plugin-sdk/driver"
)

// PluginContext bundles the resources the SDK provides to a Plugin during
// Init. It is implemented by the SDK; plugins should not implement it
// themselves.
//
// All resources are owned by the SDK and freed when the plugin shuts down.
// Plugins must not call Close on the returned *sql.DB or RedisClient — doing
// so will tear down the gRPC channel before Shutdown is invoked.
type PluginContext interface {
	// DB returns a *sql.DB whose driver proxies all queries through the
	// core's gRPC SQLProxy. The returned handle is safe for concurrent use
	// and can be passed straight to ent.NewClient or other ORMs that expect
	// the standard database/sql interface.
	DB() *sql.DB

	// Redis returns a Redis client backed by the core's gRPC RedisProxy.
	Redis() RedisClient

	// Logger returns a slog.Logger pre-tagged with the plugin's name. Plugins
	// should use this rather than slog.Default to keep logs attributable.
	Logger() *slog.Logger

	// Secrets returns the SecretEncryptor backed by the host's
	// SecretEncryption gRPC service. Returns nil when the plugin did not
	// declare CapabilitySecretsEncrypt — the SDK refuses to wire the
	// client in that case so a forgotten manifest entry surfaces as an
	// obvious nil-pointer instead of silent passthrough.
	Secrets() SecretEncryptor
	// Jobs returns the JobScheduler client for declaring scheduled work.
	// Plugins must register every spec from inside Plugin.Init(); the SDK
	// opens the Subscribe stream once Init() returns. Returns nil if the
	// plugin process was started without a JobScheduler endpoint (host
	// versions older than V5).
	Jobs() JobsClient
	// Settings returns the SettingsClient for this plugin's namespace.
	// When the plugin has not declared a SettingsSchema in its manifest
	// the returned client surfaces a clear error from every method
	// instead of silently no-oping. See settings.go for the full surface.
	Settings() SettingsClient
	// Events returns the EventsClient for subscribing to typed host
	// business events (payment.order.created, gateway.model.invoked,
	// auth.user.registered, …). The plugin must declare each event in
	// Manifest.SubscribedEvents; high-frequency events additionally
	// require a matching capability (events.gateway for
	// gateway.model.invoked). See events.go for the full surface.
	Events() EventsClient

	// Host returns the HostClient for narrow reverse-lookups against
	// the host (e.g. global model pricing). Always returns a non-nil
	// client: when the host-side HostService is not wired the client
	// surfaces ErrHostPricingUnavailable instead of panicking, so
	// plugins written against the new surface run unchanged on older
	// hosts. No manifest capability is required — HostService
	// exposes read-only lookups the host already trusts every plugin
	// to perform.
	Host() HostClient

	// HostPayment returns the HostPaymentClient for payment-related
	// host RPCs (balance credit/debit, subscription assignment,
	// affiliate rebate, user lookup). Always returns a non-nil client:
	// when the host has not registered the payment extension the
	// client surfaces typed ErrHost*Unavailable sentinels from each
	// call so plugins degrade gracefully. No manifest capability is
	// required — the same underlying gRPC connection used by Host()
	// carries these RPCs.
	//
	// This accessor eliminates the need for type-asserting Host() to
	// HostPaymentClient; plugin authors call ctx.HostPayment()
	// directly.
	HostPayment() HostPaymentClient
}

// RedisClient is the SDK's go-redis-style Redis client. Internally every
// method routes through the core's RedisProxy gRPC service.
//
// # Implementation
//
// RedisClient is a type alias for *driver.RedisClient. Earlier revisions of
// the SDK declared a parallel interface here plus a 60-method pass-through
// adapter in runner.go; both copies were verbatim mirrors of the driver
// type and drifted whenever a new command landed. The alias collapses the
// three identical surfaces (interface here, concrete struct in driver,
// adapter in runner) into a single source of truth — adding a new Redis
// command now only touches driver/redis_client.go.
//
// # Key namespacing
//
// By default keys are silently prefixed with `plugin:<plugin_name>:` so a
// plugin cannot accidentally clobber another plugin's data. Plugins that
// must read/write shared keys (e.g. the gateway cache contract maintained
// by channel-management) declare the CapabilityRedisRawKeys capability in
// their manifest and call Raw() to obtain a non-namespacing twin.
//
// # Cmder helpers
//
// Read-style methods that returned simple values in v0 (Get, HGet, …) keep
// their original (string, error) shape so existing plugins compile
// unchanged. New methods follow the go-redis cmder pattern: they return a
// typed *XxxCmd and the caller pulls the value out via .Result(), .Val(),
// or .Err().
type RedisClient = *sdkdriver.RedisClient

// RedisMsg is the SDK-level representation of a pub/sub message. Aliased
// to driver.RedisMsg so RedisClient.Subscribe can return its channel
// directly without an intermediate copy goroutine.
type RedisMsg = sdkdriver.RedisMsg
