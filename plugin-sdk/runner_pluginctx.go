package pluginsdk

import (
	"database/sql"
	"log/slog"
)

// pluginCtx is the concrete implementation of PluginContext returned to the
// plugin during Init.
type pluginCtx struct {
	db       *sql.DB
	redis    RedisClient
	logger   *slog.Logger
	secrets  SecretEncryptor
	jobs     JobsClient
	settings SettingsClient
	events   EventsClient
	host     HostClient
}

func (c *pluginCtx) DB() *sql.DB              { return c.db }
func (c *pluginCtx) Redis() RedisClient       { return c.redis }
func (c *pluginCtx) Logger() *slog.Logger     { return c.logger }
func (c *pluginCtx) Secrets() SecretEncryptor { return c.secrets }
func (c *pluginCtx) Settings() SettingsClient {
	if c.settings == nil {
		return nilSettingsClient{}
	}
	return c.settings
}

// Jobs returns the JobScheduler client. May be nil if the SDK runner did
// not wire one (older host or jobs feature not yet enabled).
func (c *pluginCtx) Jobs() JobsClient { return c.jobs }

// Events returns the EventsClient for subscribing to typed host business
// events. Returns a nilEventsClient when the SDK runner has not wired one
// (e.g. an older host without EventsExtension); every method on the nil
// client returns a clear error so misconfiguration is obvious.
func (c *pluginCtx) Events() EventsClient {
	if c.events == nil {
		return nilEventsClient{}
	}
	return c.events
}

// Host returns the HostClient for reverse host-lookups. The SDK always
// wires a real client (no capability gate); the nilHostClient fallback
// is only used by test harnesses that construct a pluginCtx by hand —
// in that mode every Host method returns ErrHostPricingUnavailable so
// misconfiguration is obvious.
func (c *pluginCtx) Host() HostClient {
	if c.host == nil {
		return nilHostClient{}
	}
	return c.host
}

// HostPayment returns the HostPaymentClient for payment-related host
// RPCs. The concrete *hostClient satisfies both HostClient and
// HostPaymentClient, so the cast always succeeds when a real client is
// wired. For test harnesses that construct a pluginCtx with host=nil
// or a custom mock, the nilHostClient fallback also satisfies
// HostPaymentClient (every method returns ErrHost*Unavailable).
func (c *pluginCtx) HostPayment() HostPaymentClient {
	if c.host == nil {
		return nilHostClient{}
	}
	if hp, ok := c.host.(HostPaymentClient); ok {
		return hp
	}
	return nilHostClient{}
}
