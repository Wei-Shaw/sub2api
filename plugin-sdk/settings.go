// Package pluginsdk — settings extension client.
//
// This file implements the plugin-side helpers for the V5 W3
// SettingsExtension capability. The plugin author interacts with two
// surfaces:
//
//  1. SettingsSchemaDoc inside Manifest — declares which settings the
//     plugin exposes, plus default values. The host renders the schema
//     into an admin form and validates writes against it.
//
//  2. PluginContext.Settings() — a lightweight client that reads the
//     current value of a key, optionally watches it for changes, and
//     unmarshals it into a Go value via GetTyped. The cache + watch
//     loop let the plugin treat settings as a near-zero-cost lookup.
//
// Failure modes are documented inline; in short, a missing key returns
// ErrSettingNotFound (NOT a nil-without-error), and stream interruptions
// are recovered by exponential-backoff reconnects driven from
// runWatchLoop.
package pluginsdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// ErrSettingNotFound is returned by SettingsClient.Get when the plugin
// asks for a key that has not been written yet AND no schema default has
// been seeded by the host. Callers should treat this as "fall back to
// hard-coded default" rather than a hard error.
var ErrSettingNotFound = errors.New("pluginsdk: setting not found")

// ErrSchemaVersionMismatch is the sentinel returned by SettingsClient.GetTyped
// when the stored value was written under a schema_version that differs from
// the schema_version the plugin currently declares. Callers should treat it
// as "the plugin upgraded its schema and the stored value may not unmarshal
// into the current Go type" — typically the right reaction is to fall back
// to the schema default and trigger a one-time migration.
//
// Use errors.Is to detect it; the concrete type SchemaVersionMismatchError
// carries the two version strings if the caller wants to log them.
var ErrSchemaVersionMismatch = errors.New("pluginsdk: settings schema version mismatch")

// SchemaVersionMismatchError is returned wrapped in ErrSchemaVersionMismatch
// when a Get / GetTyped finds a value written under an older schema version.
// Inspect its fields to log the precise drift; use errors.Is to branch.
type SchemaVersionMismatchError struct {
	Key                  string
	StoredSchemaVersion  string
	CurrentSchemaVersion string
	UnderlyingErr        error
}

func (e *SchemaVersionMismatchError) Error() string {
	return fmt.Sprintf(
		"pluginsdk: settings %q stored under schema_version=%q, current schema_version=%q (underlying: %v)",
		e.Key, e.StoredSchemaVersion, e.CurrentSchemaVersion, e.UnderlyingErr,
	)
}

func (e *SchemaVersionMismatchError) Unwrap() error { return e.UnderlyingErr }

// Is implements errors.Is so callers can `if errors.Is(err, ErrSchemaVersionMismatch)`.
func (e *SchemaVersionMismatchError) Is(target error) bool {
	return target == ErrSchemaVersionMismatch
}

// settingsCacheTTL is how long a cached value stays fresh in the absence
// of a Watch event. Watch updates invalidate cache eagerly so this TTL
// matters only when the watch stream is unhealthy.
const settingsCacheTTL = 30 * time.Second

// settingsGetTimeout caps the Get RPC duration so a stuck host cannot
// block plugin code forever.
const settingsGetTimeout = 5 * time.Second

// settingsWatchInitialBackoff / settingsWatchMaxBackoff control the
// exponential backoff for watch-stream reconnects. The numbers are
// deliberately conservative so a flapping host does not produce a
// reconnect storm.
const (
	settingsWatchInitialBackoff = 1 * time.Second
	settingsWatchMaxBackoff     = 30 * time.Second
)

// SettingsChange is the SDK-level event delivered to subscribers. It
// mirrors pb.SettingsChangeEvent but lives in the public SDK package so
// plugins do not need to import the generated proto.
type SettingsChange struct {
	Key      string
	Value    json.RawMessage
	Revision int64
}

// SettingsClient is the read-side API plugins use to access their own
// settings. Writes go through the admin UI; the SDK intentionally does
// NOT expose a Set method to plugins so configuration drift from
// runtime code is impossible.
type SettingsClient interface {
	// Get returns the JSON-encoded current value for key. If the key has
	// no value and no default seeded by the host the call returns
	// ErrSettingNotFound; the SDK never returns (nil, nil).
	Get(ctx context.Context, key string) (json.RawMessage, error)

	// GetTyped is a convenience that runs Get followed by json.Unmarshal
	// into out. out must be a pointer to the target type. Missing keys
	// surface as ErrSettingNotFound so callers can branch on them.
	GetTyped(ctx context.Context, key string, out any) error

	// Watch returns a channel that receives SettingsChange events for
	// the supplied key. An empty key subscribes to every key in the
	// plugin's namespace. The returned cleanup must be called when the
	// caller is done so the SDK can release resources; calling it more
	// than once is safe.
	Watch(ctx context.Context, key string) (<-chan SettingsChange, func(), error)
}

// settingsClient is the concrete implementation backing SettingsClient.
// It is goroutine-safe; the cache uses sync.Map for lock-free reads and
// the watcher table uses a mutex because slice appends are short.
type settingsClient struct {
	grpc       pb.SettingsExtensionClient
	pluginName string
	logger     *slog.Logger

	cache sync.Map // string key -> *cachedSetting

	watchOnce sync.Once
	watchCtx  context.Context
	watchStop context.CancelFunc
	watchDone chan struct{}

	subMu     sync.Mutex
	subs      map[string][]*settingSubscription
	subsNexID uint64
}

type cachedSetting struct {
	value                json.RawMessage
	revision             int64
	exists               bool
	fetchedAt            time.Time
	storedSchemaVersion  string // V5/W6 SETTINGS-V2: schema_version active when written
	currentSchemaVersion string // V5/W6 SETTINGS-V2: schema_version the plugin currently declares
}

type settingSubscription struct {
	id  uint64
	key string
	ch  chan SettingsChange
}

// nilSettingsClient is returned when the host indicates that the plugin
// did not opt into SettingsExtension. Every method returns a clear error
// so debugging is straightforward; we do NOT silently no-op because a
// silent client would mask manifest bugs.
type nilSettingsClient struct{}

func (nilSettingsClient) Get(ctx context.Context, key string) (json.RawMessage, error) {
	return nil, errors.New("pluginsdk: SettingsExtension not enabled (declare a SettingsSchema in your Manifest)")
}
func (n nilSettingsClient) GetTyped(ctx context.Context, key string, out any) error {
	_, err := n.Get(ctx, key)
	return err
}
func (n nilSettingsClient) Watch(ctx context.Context, key string) (<-chan SettingsChange, func(), error) {
	_, err := n.Get(ctx, key)
	return nil, func() {}, err
}

// newSettingsClient wires up a SettingsClient on top of an existing gRPC
// connection. pluginName is captured for logging; the actual identity
// the host trusts is the metadata interceptor configured in runner.go.
func newSettingsClient(grpc pb.SettingsExtensionClient, pluginName string, logger *slog.Logger) *settingsClient {
	c := &settingsClient{
		grpc:       grpc,
		pluginName: pluginName,
		logger:     logger,
		subs:       make(map[string][]*settingSubscription),
	}
	return c
}

// startWatchLoop launches the long-lived stream that the SDK uses to
// receive change notifications from the host. It is idempotent so the
// caller can invoke it from Init without worrying about duplicate
// goroutines.
func (c *settingsClient) startWatchLoop() {
	c.watchOnce.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		c.watchCtx = ctx
		c.watchStop = cancel
		c.watchDone = make(chan struct{})
		go c.runWatchLoop(ctx)
	})
}

// stopWatchLoop is invoked from the SDK shutdown path. It cancels the
// stream and waits up to a second for the goroutine to drain so the
// final cleanup logs are emitted before the process exits.
func (c *settingsClient) stopWatchLoop() {
	if c.watchStop == nil {
		return
	}
	c.watchStop()
	if c.watchDone != nil {
		select {
		case <-c.watchDone:
		case <-time.After(time.Second):
		}
	}
}

func (c *settingsClient) Get(ctx context.Context, key string) (json.RawMessage, error) {
	if v, ok := c.cache.Load(key); ok {
		entry := v.(*cachedSetting)
		if !entry.exists {
			return nil, ErrSettingNotFound
		}
		if time.Since(entry.fetchedAt) < settingsCacheTTL {
			return entry.value, nil
		}
	}

	rpcCtx, cancel := context.WithTimeout(ctx, settingsGetTimeout)
	defer cancel()
	resp, err := c.grpc.Get(rpcCtx, &pb.SettingsGetRequest{Key: key})
	if err != nil {
		return nil, fmt.Errorf("pluginsdk: settings get %q: %w", key, err)
	}
	if !resp.GetExists() {
		// Cache the negative answer briefly so a tight Get loop on a
		// missing key does not hammer the host.
		c.cache.Store(key, &cachedSetting{exists: false, fetchedAt: time.Now()})
		return nil, ErrSettingNotFound
	}
	val := append(json.RawMessage(nil), resp.GetValueJson()...)
	c.cache.Store(key, &cachedSetting{
		value:                val,
		revision:             resp.GetRevision(),
		exists:               true,
		fetchedAt:            time.Now(),
		storedSchemaVersion:  resp.GetStoredSchemaVersion(),
		currentSchemaVersion: resp.GetCurrentSchemaVersion(),
	})
	return val, nil
}

func (c *settingsClient) GetTyped(ctx context.Context, key string, out any) error {
	raw, err := c.Get(ctx, key)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, out); err != nil {
		// V5/W6 SETTINGS-V2 §3.4.4: look up cached schema_version to attach
		// precise drift info. Get already cached the values from the RPC.
		var stored, current string
		if v, ok := c.cache.Load(key); ok {
			entry := v.(*cachedSetting)
			stored = entry.storedSchemaVersion
			current = entry.currentSchemaVersion
		}
		// Normalise empty strings to "0" so callers can compare without
		// special-casing pre-V2 hosts.
		if stored == "" {
			stored = "0"
		}
		if current == "" {
			current = "0"
		}
		underlying := fmt.Errorf("pluginsdk: settings unmarshal %q: %w", key, err)
		if stored != current {
			return &SchemaVersionMismatchError{
				Key:                  key,
				StoredSchemaVersion:  stored,
				CurrentSchemaVersion: current,
				UnderlyingErr:        underlying,
			}
		}
		return underlying
	}
	return nil
}

func (c *settingsClient) Watch(ctx context.Context, key string) (<-chan SettingsChange, func(), error) {
	c.startWatchLoop()
	sub := &settingSubscription{
		key: key,
		ch:  make(chan SettingsChange, 8),
	}
	c.subMu.Lock()
	c.subsNexID++
	sub.id = c.subsNexID
	c.subs[key] = append(c.subs[key], sub)
	c.subMu.Unlock()

	cleanup := func() {
		c.removeSub(key, sub.id)
	}
	// Auto-cleanup when the caller's context expires so callers can fire
	// off a Watch and forget the cleanup func when convenient.
	if ctx != nil && ctx.Done() != nil {
		go func() {
			<-ctx.Done()
			cleanup()
		}()
	}
	return sub.ch, cleanup, nil
}

func (c *settingsClient) removeSub(key string, id uint64) {
	c.subMu.Lock()
	defer c.subMu.Unlock()
	subs := c.subs[key]
	for i, s := range subs {
		if s.id == id {
			close(s.ch)
			c.subs[key] = append(subs[:i], subs[i+1:]...)
			if len(c.subs[key]) == 0 {
				delete(c.subs, key)
			}
			return
		}
	}
}

func (c *settingsClient) runWatchLoop(ctx context.Context) {
	defer close(c.watchDone)
	backoff := settingsWatchInitialBackoff
	for {
		if ctx.Err() != nil {
			return
		}
		stream, err := c.grpc.Watch(ctx, &pb.SettingsWatchRequest{Key: ""})
		if err != nil {
			if c.logger != nil {
				c.logger.Warn("settings watch stream open failed; will retry",
					"plugin", c.pluginName, "error", err, "backoff", backoff)
			}
			if !settingsSleepCtx(ctx, backoff) {
				return
			}
			backoff = settingsNextBackoff(backoff)
			continue
		}
		// Reset backoff after a successful open. The host's sendSnapshot
		// fires a fresh per-key event for every value in the namespace as
		// soon as the stream opens (see SETTINGS-V2-DESIGN §4.5 + INSPECT
		// §2.4), so applyEvent below will re-prime the cache automatically
		// — including the case where the host closed the previous stream
		// because the plugin's schema_version changed.
		backoff = settingsWatchInitialBackoff
		for {
			evt, err := stream.Recv()
			if err != nil {
				if ctx.Err() == nil && c.logger != nil {
					// EOF / Unavailable here may also mean the host closed
					// the stream because schema_version changed (DESIGN
					// §4.5 dropAllSubscribersForPlugin). Either way the
					// outer loop reconnects and the host's snapshot
					// re-primes our cache.
					c.logger.Warn("settings watch stream lost; will reconnect and re-prime cache via host snapshot",
						"plugin", c.pluginName, "error", err)
				}
				break
			}
			c.applyEvent(evt)
		}
	}
}

func (c *settingsClient) applyEvent(evt *pb.SettingsChangeEvent) {
	if evt == nil {
		return
	}
	val := append(json.RawMessage(nil), evt.GetValueJson()...)
	// V5/W6 SETTINGS-V2 §3.5: Watch events do not carry schema_version,
	// only the new value + a requires_reload marker. Preserve any cached
	// schema_version from the previous Get so a subsequent GetTyped on
	// the same key can still detect drift. The next Get RPC refreshes
	// the versions if the schema actually changed.
	prevStored := ""
	prevCurrent := ""
	if v, ok := c.cache.Load(evt.GetKey()); ok {
		entry := v.(*cachedSetting)
		prevStored = entry.storedSchemaVersion
		prevCurrent = entry.currentSchemaVersion
	}
	c.cache.Store(evt.GetKey(), &cachedSetting{
		value:                val,
		revision:             evt.GetRevision(),
		exists:               true,
		fetchedAt:            time.Now(),
		storedSchemaVersion:  prevStored,
		currentSchemaVersion: prevCurrent,
	})
	if evt.GetRequiresReload() && c.logger != nil {
		// V5/W6 SETTINGS-V2 §3.4.3: requires_reload is informational at
		// the SDK layer — the host PluginManager owns the actual reload.
		// Log it so plugin authors can correlate operator-initiated
		// reloads with the change event in their own logs. Never panic
		// or exit the process on this signal.
		c.logger.Info("settings change marked requires_reload; host will reload plugin",
			"plugin", c.pluginName, "key", evt.GetKey(), "revision", evt.GetRevision())
	}
	change := SettingsChange{Key: evt.GetKey(), Value: val, Revision: evt.GetRevision()}

	c.subMu.Lock()
	// Fan-out to exact-key subscribers AND empty-key (namespace) subscribers.
	targets := make([]*settingSubscription, 0)
	targets = append(targets, c.subs[evt.GetKey()]...)
	if evt.GetKey() != "" {
		targets = append(targets, c.subs[""]...)
	}
	c.subMu.Unlock()

	for _, sub := range targets {
		select {
		case sub.ch <- change:
		default:
			// Drop on a full channel rather than blocking the watch loop.
			// Slow consumers are responsible for draining; the cache
			// already holds the freshest value so they can recover via
			// Get.
			if c.logger != nil {
				c.logger.Warn("settings watcher channel full; event dropped",
					"plugin", c.pluginName, "key", evt.GetKey())
			}
		}
	}
}

func settingsSleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func settingsNextBackoff(cur time.Duration) time.Duration {
	next := cur * 3
	if next > settingsWatchMaxBackoff {
		return settingsWatchMaxBackoff
	}
	return next
}
