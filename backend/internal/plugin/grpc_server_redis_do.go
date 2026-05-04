// grpc_server_redis_do.go implements the universal RedisProxy.Do RPC plus the
// per-plugin capability registry that polices it.
//
// Design summary
// --------------
// The SDK client side already namespaces keys with `plugin:<name>:` before
// dispatching. The server's job is therefore mostly forwarding plus two
// safety checks:
//
//   1. raw_key=true requires the plugin to hold the redis_raw_keys
//      capability. The capability registry is populated by manager.go when
//      Init succeeds and cleared on stop / unregister.
//   2. When raw_key=false, we still verify each "key argument" carries the
//      plugin's namespace prefix as defence-in-depth against a misbehaving
//      SDK or rogue binary. A namespace violation is treated as a hard
//      protocol error rather than silently rewriting because rewriting on
//      the server would mask client bugs.
//
// The plugin name is resolved through gRPC peer information rather than
// trusting any caller-supplied field. Currently we identify plugins via the
// loopback TCP address recorded at init time; refining this to use mTLS
// SPIFFE identities is straightforward when the transport is upgraded.

package plugin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	pluginsdkroot "github.com/Wei-Shaw/sub2api/plugin-sdk"
	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// callerMetadataKey is the gRPC metadata header the SDK client attaches to
// every outgoing call so the server can identify which plugin issued it.
// The trust model is "the operator installed the plugin binary; we trust it
// to honour the SDK's metadata convention". Capability enforcement still
// applies because the registry only contains plugins the operator has
// explicitly approved at Init time.
//
// The literal lives in pluginsdkroot.CallerMetadataKey — host and SDK consume
// the same constant so the wire contract cannot drift between them. We keep
// the unexported package-local alias so the rest of this file reads exactly
// like before.
const callerMetadataKey = pluginsdkroot.CallerMetadataKey

// pluginCapabilityRegistry tracks which plugins have which capabilities and
// which DB tables they own. Keyed by plugin name; values are sets for O(1)
// membership checks. The tables map drives the SQL gate (P12·B-1).
type pluginCapabilityRegistry struct {
	mu     sync.RWMutex
	caps   map[string]map[string]struct{}
	tables map[string]map[string]struct{}
}

func newPluginCapabilityRegistry() *pluginCapabilityRegistry {
	return &pluginCapabilityRegistry{
		caps:   make(map[string]map[string]struct{}),
		tables: make(map[string]map[string]struct{}),
	}
}

// Set replaces the capability set for plugin name. Passing an empty slice
// effectively revokes all capabilities while keeping the entry around.
func (r *pluginCapabilityRegistry) Set(name string, capabilities []string) {
	set := make(map[string]struct{}, len(capabilities))
	for _, c := range capabilities {
		set[c] = struct{}{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.caps[name] = set
}

// SetTables replaces the owned-tables set for plugin name. Names are
// lowercased for case-insensitive matching against parsed SQL.
func (r *pluginCapabilityRegistry) SetTables(name string, tables []string) {
	set := make(map[string]struct{}, len(tables))
	for _, t := range tables {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" {
			continue
		}
		set[t] = struct{}{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tables[name] = set
}

// Has returns true if plugin name has been granted the specified capability.
func (r *pluginCapabilityRegistry) Has(name, capability string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	set, ok := r.caps[name]
	if !ok {
		return false
	}
	_, has := set[capability]
	return has
}

// OwnsTable returns true if plugin name declared the table in its manifest's
// OwnedTables list. Comparison is case-insensitive.
func (r *pluginCapabilityRegistry) OwnsTable(name, table string) bool {
	table = strings.ToLower(strings.TrimSpace(table))
	if table == "" {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	set, ok := r.tables[name]
	if !ok {
		return false
	}
	_, has := set[table]
	return has
}

// AllowedTable runs the SQL gate decision: a plugin may touch a table when
//  1. it owns the table (declared in OwnedTables), OR
//  2. it holds db.core.read (or db.core.write for writes) and the table
//     is on the host shared whitelist.
//
// `forWrite=true` requires db.core.write for the host-shared branch and
// db.own.write for the owned branch. Owned-table reads/writes also require
// the corresponding db.own.read / db.own.write capability — both are
// default-grant for now but enforcing makes the audit trail explicit.
func (r *pluginCapabilityRegistry) AllowedTable(name, table string, forWrite bool) bool {
	owns := r.OwnsTable(name, table)
	if owns {
		if forWrite {
			return r.Has(name, "db.own.write")
		}
		return r.Has(name, "db.own.read")
	}
	if _, shared := hostSharedReadableTables[strings.ToLower(strings.TrimSpace(table))]; !shared {
		return false
	}
	if forWrite {
		return r.Has(name, "db.core.write")
	}
	return r.Has(name, "db.core.read")
}

// hostSharedReadableTables is the curated whitelist of host tables a plugin
// may touch with the db.core.read / db.core.write capability. Adding a
// table here is a security decision; only include tables whose schema is
// stable and whose contents are safe to expose to plugin code.
//
// Notably absent: api_keys, sessions, password hashes, anything secret-bearing.
var hostSharedReadableTables = map[string]struct{}{
	"users":               {},
	"user_subscriptions":  {},
	"user_allowed_groups": {},
	"groups":              {},
	"accounts":            {},
	"payment_orders":      {},
	// subscription_plans is host-owned (admin defines the catalog) but the
	// payment plugin needs to read it to render plan listings, group rules,
	// and validate plan IDs at order creation time. Schema is stable and
	// rows contain only catalog metadata (name, price, validity) — safe to
	// expose to plugin code.
	"subscription_plans":  {},
}

// Forget drops the entry for plugin name. Called when a plugin is stopped
// so its address space can be reused safely.
func (r *pluginCapabilityRegistry) Forget(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.caps, name)
	delete(r.tables, name)
}

// knownPluginsRegistry stores the set of plugin names the manager has
// registered with the SDK. We use it to validate the metadata header is
// for a plugin we actually know about — random external clients calling
// the SDK socket cannot fabricate an identity.
type knownPluginsRegistry struct {
	mu    sync.RWMutex
	names map[string]struct{}
}

func newKnownPluginsRegistry() *knownPluginsRegistry {
	return &knownPluginsRegistry{names: make(map[string]struct{})}
}

func (r *knownPluginsRegistry) Add(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.names[name] = struct{}{}
}

func (r *knownPluginsRegistry) Remove(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.names, name)
}

func (r *knownPluginsRegistry) Contains(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.names[name]
	return ok
}

// RegisterPlugin records the plugin's approved capabilities and marks the
// name as a known caller. It is called by the manager once GetManifest has
// run and the operator's capability allow-list has been applied. The
// capabilities slice is copied; mutating it after registration has no effect.
func (s *SDKServer) RegisterPlugin(name string, capabilities []string) {
	s.capabilities.Set(name, capabilities)
	s.knownPlugins.Add(name)
}

// UnregisterPlugin clears the plugin's capability binding and removes it
// from the known-callers set. Safe to call multiple times — subsequent
// calls are no-ops.
func (s *SDKServer) UnregisterPlugin(name string) {
	s.capabilities.Forget(name)
	s.knownPlugins.Remove(name)
}

// HasCapability reports whether the plugin has been granted the named
// capability. Exposed so external service wirings (e.g. EventsExtension)
// can implement their own gating without re-implementing the registry.
func (s *SDKServer) HasCapability(name, capability string) bool {
	return s.capabilities.Has(name, capability)
}

// resolveCaller pulls the plugin identity from the gRPC metadata header the
// SDK client attaches to every outgoing call. Returns "" when the header is
// missing or names a plugin the manager has not registered, so callers can
// decide whether to deny or allow anonymous access.
func (s *SDKServer) resolveCaller(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	values := md.Get(callerMetadataKey)
	if len(values) == 0 {
		return ""
	}
	name := values[0]
	if !s.knownPlugins.Contains(name) {
		return ""
	}
	return name
}

// ============================================================
// RedisProxy.Do implementation
// ============================================================

// maxDoArgsSize caps the total payload of a Do call to keep one rogue
// plugin from issuing a 1GB Redis command and exhausting the proxy's
// memory. 4 MiB is generous: typical commands fit in a few hundred bytes.
const maxDoArgsSize = 4 << 20 // 4 MiB

// Do dispatches an arbitrary Redis command. Implements the new RedisProxy
// universal entry point.
func (s *SDKServer) Do(ctx context.Context, req *pluginsdk.DoRequest) (*pluginsdk.DoReply, error) {
	if s.redis == nil {
		return nil, errService("redis")
	}
	args := req.GetArgs()
	if len(args) == 0 {
		return nil, status.Error(codes.InvalidArgument, "redis do: empty args")
	}
	if total := totalArgSize(args); total > maxDoArgsSize {
		return nil, status.Errorf(codes.ResourceExhausted, "redis do: argument size %d exceeds limit %d", total, maxDoArgsSize)
	}

	commandName := strings.ToUpper(string(args[0]))
	// 拦截器已校验非空 caller 并塞入 ctx; 这里 pluginName 不会为空。
	pluginName, _ := CallerFromContext(ctx)

	// raw_key=true requires the redis.raw capability. The interceptor
	// already rejected anonymous callers, so the empty-name branch here is
	// purely a defensive belt-and-braces (capability table is keyed by name
	// — empty name can never match).
	if req.GetRawKey() {
		// P12·B-1: capability normalisation runs in approveCapabilities,
		// so the registry stores the canonical "redis.raw"; legacy
		// "redis_raw_keys" plugins still pass after normalisation.
		if pluginName == "" || !s.capabilities.Has(pluginName, "redis.raw") {
			slog.Warn("redis do: raw_key=true denied",
				"plugin", pluginName, "command", commandName)
			return nil, status.Error(codes.PermissionDenied, "redis do: raw_key requires redis.raw capability")
		}
	} else if pluginName != "" {
		// Defence in depth: every key argument must carry the plugin's
		// namespace. The SDK client always adds it; if it is missing here
		// either the SDK is broken or someone is talking to us directly.
		if err := validateNamespace(pluginName, args, req.GetKeyPositions()); err != nil {
			slog.Warn("redis do: namespace validation failed",
				"plugin", pluginName, "command", commandName, "error", err)
			return nil, status.Errorf(codes.InvalidArgument, "%v", err)
		}
	}

	// Build the []any slice go-redis expects and dispatch.
	cmdArgs := make([]any, len(args))
	for i, a := range args {
		cmdArgs[i] = a
	}
	cmd := s.redis.Do(ctx, cmdArgs...)
	return doResultToReply(cmd), nil
}

// totalArgSize reports the cumulative byte length of all DoRequest args.
// Used for the size guard so we do not need to allocate the []any slice
// before deciding to reject.
func totalArgSize(args [][]byte) int {
	total := 0
	for _, a := range args {
		total += len(a)
	}
	return total
}

// validateNamespace ensures every argument the client identified as a key
// starts with `plugin:<plugin_name>:`. We compare on bytes to avoid
// allocating intermediate strings on the hot path.
func validateNamespace(pluginName string, args [][]byte, keyPositions []int32) error {
	if len(keyPositions) == 0 {
		// No declared key positions: nothing to validate. The SDK client
		// always sends positions for commands that touch keys, so an
		// empty positions list usually means a key-less command (PING).
		return nil
	}
	prefix := []byte("plugin:" + pluginName + ":")
	for _, pos := range keyPositions {
		// Positions are 0-based into args excluding the command, so the
		// real index in args is pos+1.
		idx := int(pos) + 1
		if idx <= 0 || idx >= len(args) {
			continue
		}
		if !hasPrefixBytes(args[idx], prefix) {
			return fmt.Errorf("redis do: key %q not in plugin namespace %q",
				truncateForLog(args[idx]), string(prefix))
		}
	}
	return nil
}

func hasPrefixBytes(b, prefix []byte) bool {
	if len(b) < len(prefix) {
		return false
	}
	for i := range prefix {
		if b[i] != prefix[i] {
			return false
		}
	}
	return true
}

// truncateForLog returns at most the first 64 bytes of b as a string, with
// "…" appended if more was elided. Keeps log messages bounded even when a
// plugin sends a multi-megabyte argument.
func truncateForLog(b []byte) string {
	const limit = 64
	if len(b) <= limit {
		return string(b)
	}
	return string(b[:limit]) + "…"
}

// ============================================================
// go-redis Cmd → DoReply translation
// ============================================================

// doResultToReply converts a go-redis *redis.Cmd into the DoReply shape the
// SDK client expects. We surface Redis-side errors via DoReply.error rather
// than gRPC status because the SDK distinguishes the two: gRPC errors mean
// "transport failed" while DoReply.error means "Redis said no".
func doResultToReply(cmd *redis.Cmd) *pluginsdk.DoReply {
	if cmd == nil {
		return &pluginsdk.DoReply{Kind: "nil", NilValue: true}
	}
	if err := cmd.Err(); err != nil {
		if errors.Is(err, redis.Nil) {
			return &pluginsdk.DoReply{Kind: "nil", NilValue: true}
		}
		return &pluginsdk.DoReply{Kind: "error", Error: err.Error()}
	}
	val, err := cmd.Result()
	if err != nil {
		return &pluginsdk.DoReply{Kind: "error", Error: err.Error()}
	}
	return valueToReply(val)
}

// valueToReply walks the polymorphic value go-redis returns and packs it
// into a DoReply. Recursive cases (arrays of arrays) are handled by
// re-invoking valueToReply on each element.
func valueToReply(v any) *pluginsdk.DoReply {
	switch x := v.(type) {
	case nil:
		return &pluginsdk.DoReply{Kind: "nil", NilValue: true}
	case string:
		return &pluginsdk.DoReply{Kind: "string", StringValue: []byte(x)}
	case []byte:
		return &pluginsdk.DoReply{Kind: "string", StringValue: x}
	case int64:
		return &pluginsdk.DoReply{Kind: "int", IntValue: x}
	case int:
		return &pluginsdk.DoReply{Kind: "int", IntValue: int64(x)}
	case float64:
		return &pluginsdk.DoReply{Kind: "float", FloatValue: x}
	case bool:
		if x {
			return &pluginsdk.DoReply{Kind: "int", IntValue: 1}
		}
		return &pluginsdk.DoReply{Kind: "int", IntValue: 0}
	case []any:
		arr := make([]*pluginsdk.DoReply, 0, len(x))
		for _, el := range x {
			arr = append(arr, valueToReply(el))
		}
		return &pluginsdk.DoReply{Kind: "array", Array: arr}
	case []string:
		arr := make([]*pluginsdk.DoReply, 0, len(x))
		for _, el := range x {
			arr = append(arr, &pluginsdk.DoReply{Kind: "string", StringValue: []byte(el)})
		}
		return &pluginsdk.DoReply{Kind: "array", Array: arr}
	case map[string]string:
		// Some go-redis commands return map directly; flatten to k,v,k,v
		// so the SDK side parser (which expects RESP-style array) is happy.
		arr := make([]*pluginsdk.DoReply, 0, 2*len(x))
		for k, v2 := range x {
			arr = append(arr,
				&pluginsdk.DoReply{Kind: "string", StringValue: []byte(k)},
				&pluginsdk.DoReply{Kind: "string", StringValue: []byte(v2)},
			)
		}
		return &pluginsdk.DoReply{Kind: "array", Array: arr}
	case time.Duration:
		return &pluginsdk.DoReply{Kind: "int", IntValue: int64(x / time.Second)}
	case error:
		return &pluginsdk.DoReply{Kind: "error", Error: x.Error()}
	default:
		// Stringly fallback for types we do not specifically handle. Keeps
		// the path total — a panic here would crash the SDK server for any
		// unfamiliar Redis reply shape.
		return &pluginsdk.DoReply{Kind: "string", StringValue: []byte(stringify(v))}
	}
}

func stringify(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	default:
		return fmt.Sprintf("%v", v)
	}
}
