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

// SettingsChange is the SDK-level event delivered to subscribers. It
// mirrors pb.SettingsChangeEvent but lives in the public SDK package so
// plugins do not need to import the generated proto.
type SettingsChange struct {
	Key      string
	Value    json.RawMessage
	Revision int64
}

// SettingsClient is the API plugins use to access their own settings.
// Reads (Get / GetTyped / Watch) and writes (Set) both flow through
// the host SettingsExtension; writes go through the same validation
// pipeline as admin REST writes (schema check + visibility guard).
type SettingsClient interface {
	// Get returns the JSON-encoded current value for key. If the key has
	// no value and no default seeded by the host the call returns
	// ErrSettingNotFound; the SDK never returns (nil, nil).
	Get(ctx context.Context, key string) (json.RawMessage, error)

	// GetTyped is a convenience that runs Get followed by json.Unmarshal
	// into out. out must be a pointer to the target type. Missing keys
	// surface as ErrSettingNotFound so callers can branch on them.
	GetTyped(ctx context.Context, key string, out any) error

	// Set persists a value for key inside the plugin's namespace. The
	// host validates against the schema declared in the manifest. Pass
	// any JSON-serialisable value; the SDK marshals to JSON before
	// sending. Returns the new revision on success.
	Set(ctx context.Context, key string, value any) error

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
//
// cacheWriteMu serializes the write paths of Get / applyEvent: both load
// the current entry, compare revisions, and then decide whether to Store.
// Serializing writes prevents the Get path from overwriting a higher
// revision already stored by the watcher (write-after-write race). Reads
// still go through sync.Map without locking.
type settingsClient struct {
	grpc       pb.SettingsExtensionClient
	pluginName string
	logger     *slog.Logger

	cache        sync.Map // string key -> *cachedSetting
	cacheWriteMu sync.Mutex

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
func (n nilSettingsClient) Set(ctx context.Context, key string, value any) error {
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
		// Cache the negative answer; still go through storeIfNewer so a
		// watch push that landed during our RPC with a higher revision
		// is not clobbered. revision = 0 lets any watch write win.
		c.storeIfNewer(key, &cachedSetting{exists: false, fetchedAt: time.Now()})
		return nil, ErrSettingNotFound
	}
	val := append(json.RawMessage(nil), resp.GetValueJson()...)
	c.storeIfNewer(key, &cachedSetting{
		value:                val,
		revision:             resp.GetRevision(),
		exists:               true,
		fetchedAt:            time.Now(),
		storedSchemaVersion:  resp.GetStoredSchemaVersion(),
		currentSchemaVersion: resp.GetCurrentSchemaVersion(),
	})
	// Return the freshest entry from the cache (a watch push may have
	// already replaced it with a higher revision).
	if v, ok := c.cache.Load(key); ok {
		if entry := v.(*cachedSetting); entry.exists {
			return entry.value, nil
		}
	}
	return val, nil
}

// storeIfNewer writes next into the cache, but only when next.revision is
// >= the current entry's revision. Both write paths (Get RPC completion
// and applyEvent watch push) call it, serialized by cacheWriteMu to avoid
// a stale RPC result overwriting a newer watch push.
//
// revision == 0 is returned by the RPC when the host does not supply a
// version; in that case the behaviour degrades to "latest-write-wins",
// matching the previous semantics.
func (c *settingsClient) storeIfNewer(key string, next *cachedSetting) {
	c.cacheWriteMu.Lock()
	defer c.cacheWriteMu.Unlock()
	if v, ok := c.cache.Load(key); ok {
		cur := v.(*cachedSetting)
		// Only allow overwrite when next.revision >= the existing one.
		// Watch always carries >= the host's currently published
		// revision; the Get path may lag while the RPC is in flight.
		if cur.revision > next.revision {
			return
		}
	}
	c.cache.Store(key, next)
}

// Set implements SettingsClient.Set. Marshals value to JSON and sends
// to the host's SettingsExtension. The cache is updated lazily via the
// Watch stream so callers reading immediately after Set may briefly
// see the old value; this matches host admin write semantics.
func (c *settingsClient) Set(ctx context.Context, key string, value any) error {
	if c == nil || c.grpc == nil {
		return errors.New("pluginsdk: settings client unavailable")
	}
	if key == "" {
		return errors.New("pluginsdk: settings set: empty key")
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("pluginsdk: settings set: marshal value: %w", err)
	}
	if _, err := c.grpc.Set(ctx, &pb.SettingsSetRequest{Key: key, ValueJson: raw}); err != nil {
		return fmt.Errorf("pluginsdk: settings set %q: %w", key, err)
	}
	return nil
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
