// Command hello-world is the canonical smoke-test plugin for the Sub2API
// plugin runtime. It is also the recommended starting template for new
// plugin authors — see plugins/hello-world/README.md for the operator
// view (build / run / deploy) and plugin-sdk/README.md for the full SDK
// reference.
//
// It exercises every transport the SDK provides:
//   - PluginLifecycle handshake + Manifest delivery (host → plugin)
//   - HTTP routes registered via HTTPRegistrar (no auth + admin auth)
//   - SQL proxy round-trip via PluginContext.DB() (plugin → host)
//   - Redis proxy round-trip via PluginContext.Redis() (plugin → host)
//   - SettingsExtension read via PluginContext.Settings().GetTyped (V5/W3)
//   - EventsExtension subscribe via PluginContext.Events().Subscribe (Phase A)
//   - Embedded frontend bundle served through GetFrontendBundle gRPC stream
//
// It is deliberately tiny so any failure points to the SDK or core, not to
// the plugin itself.
package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"path"
	"sync/atomic"
	"time"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
	sdkdriver "github.com/Wei-Shaw/sub2api/plugin-sdk/driver"
)

// frontendAssets 是插件预先编译好的前端 bundle.
// 把整个 frontend/dist 目录嵌进二进制, 核心通过 GetFrontendBundle gRPC 流读取.
//
//go:embed all:frontend/dist
var frontendAssets embed.FS

const (
	pluginVersion = "0.1.0"

	// dbTestTimeout / redisTestTimeout cap how long a single smoke-test
	// request can wait on the proxy before giving up. The values are
	// generous because the proxy hop adds ~1 RTT on top of the underlying
	// driver latency.
	dbTestTimeout    = 5 * time.Second
	redisTestTimeout = 5 * time.Second

	// redisTestKey is the key the /redis-test endpoint writes and reads
	// back. The SDK automatically prefixes it with `plugin:hello-world:`
	// so the actual Redis key is `plugin:hello-world:smoke-test`. Plugins
	// must NOT include the prefix manually any more — doing so would
	// produce double-prefixed keys (plugin:hello-world:plugin:hello-world:…).
	redisTestKey = "smoke-test"
	redisTestTTL = 60 * time.Second
)

// HelloPlugin holds the resources the SDK injects via Init. The pluginCtx
// pointer is stored atomically so HTTP handlers can read it without a mutex
// even when they race with Init (in practice Init always finishes first).
//
// eventsCtx / eventsCancel scope the EventsExtension subscribe loop to the
// plugin lifetime: Shutdown cancels the context so the SDK's subscribe
// goroutine can exit cleanly before the gRPC connection tears down.
type HelloPlugin struct {
	ctx          atomic.Pointer[pluginsdk.PluginContext]
	eventsCtx    context.Context
	eventsCancel context.CancelFunc
}

// pluginRoutePrefix is the path prefix the core gateway uses when proxying
// plugin endpoints to this plugin. The convention (per plugins/channel-management)
// is that the manifest declares full paths including this prefix; the core
// matches them verbatim, no prepending.
const pluginRoutePrefix = "/api/v1/plugin/hello-world"

// helloWorldSettingsSchema is the V5/W3 SettingsExtension demo: a single
// "greeting" string that the admin UI can edit. We embed the schema +
// defaults as raw JSON so the example remains self-contained; production
// plugins can also generate these from struct tags via a build-time tool.
var (
	helloWorldSettingsSchema = []byte(`{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "title": "Hello World Plugin Settings",
  "type": "object",
  "properties": {
    "greeting": {
      "type": "string",
      "title": "Greeting",
      "description": "The string returned by /api/v1/plugin/hello-world/hello.",
      "default": "Hello"
    }
  }
}`)
	helloWorldSettingsDefaults = []byte(`{"greeting":"Hello"}`)
)

func (p *HelloPlugin) Manifest() *pluginsdk.Manifest {
	return &pluginsdk.Manifest{
		Name:        "hello-world",
		DisplayName: "Hello World",
		Version:     pluginVersion,
		Description: "A test plugin for the plugin system",
		Author:      "Sub2API",
		// IconSVG opts the plugin into the V5/W7 admin-card custom icon path:
		// the plugins management page renders this SVG next to DisplayName
		// instead of the host's generic cube fallback.
		IconSVG: pluginsdk.IconPuzzle,
		PluginEndpoints: []pluginsdk.EndpointDecl{
			{Path: pluginRoutePrefix + "/hello", Methods: []string{http.MethodGet}, AuthType: pluginsdk.AuthTypeNone},
			{Path: pluginRoutePrefix + "/db-test", Methods: []string{http.MethodGet}, AuthType: pluginsdk.AuthTypeAdmin},
			{Path: pluginRoutePrefix + "/redis-test", Methods: []string{http.MethodGet}, AuthType: pluginsdk.AuthTypeAdmin},
		},
		// OwnedTables: hello-world ships no migrations and creates no tables;
		// the explicit empty list is documentation for the P12·B-1 SQL gate.
		// db-test / redis-test endpoints below run only the SDK's own self-test
		// queries, which (db-test included) require either OwnedTables entries
		// or a db.core.read declaration to pass the SQL gate. As written this
		// plugin's db-test only succeeds against the host shared whitelist
		// when the operator adds db.core.read to Capabilities.
		OwnedTables: nil,
		Frontend: &pluginsdk.FrontendManifest{
			// EntryJS 路径相对于插件二进制内的 frontend bundle 根目录;
			// 核心会把它拼接成 /api/v1/plugin-assets/hello-world/dist/entry.js 的 HTTP URL.
			EntryJS: "dist/entry.js",
			MenuItems: []pluginsdk.MenuItemDecl{
				{
					Path:    "/admin/plugins/hello-world",
					IconSVG: pluginsdk.IconPuzzle,
					Labels:  pluginsdk.Labels("Hello World", "Hello World"),
					Section: pluginsdk.SectionAdmin,
					// Placement DSL — hello-world 是测试插件，落在 admin/end 桶
					// (低于业务主菜单和系统类菜单)。Order=100 是桶内相对位置；
					// 与 SortOrder 无关，仅 mergeByPlacement 用。
					Placement: &pluginsdk.Placement{Group: pluginsdk.PlacementAdminEnd, Order: 100},
					SortOrder: 999,
				},
			},
			Routes: []pluginsdk.RouteDecl{
				{
					Path:          "/admin/plugins/hello-world",
					Name:          "PluginHelloWorld",
					ComponentPath: "HelloWorldView.vue",
				},
			},
			I18nNamespaces: []string{"helloWorldPlugin"},
		},
		SettingsSchema: &pluginsdk.SettingsSchemaDoc{
			Schema:   helloWorldSettingsSchema,
			Defaults: helloWorldSettingsDefaults,
		},
		// Phase A EventsExtension demo: hello-world subscribes to
		// auth.user.registered so a fresh registration produces a log
		// line in the host's plugin output. The handler is intentionally
		// trivial — a real plugin would call out to email or Slack via
		// SafeHTTPClient. We deliberately skip gateway.model.invoked here
		// because that event requires the events.gateway capability and
		// firing on every gateway request would drown the demo's logs.
		SubscribedEvents: []string{pluginsdk.EventTypeAuthUserRegistered},
	}
}

// Init stores the SDK-supplied context for handlers to use. We use an
// atomic.Pointer because Go does not let us assign through a shared interface
// value safely otherwise.
func (p *HelloPlugin) Init(ctx pluginsdk.PluginContext) error {
	p.ctx.Store(&ctx)
	ctx.Logger().Info("hello-world plugin initialised", "version", pluginVersion)

	// Phase A EventsExtension demo: subscribe to auth.user.registered.
	// Subscribe spawns its own goroutine and reconnects on failure with
	// exponential backoff (1s → 2s → 4s → 8s → 30s), so we just call
	// it once and let the SDK manage the receive loop.
	p.eventsCtx, p.eventsCancel = context.WithCancel(context.Background())
	if err := ctx.Events().Subscribe(
		p.eventsCtx,
		[]string{pluginsdk.EventTypeAuthUserRegistered},
		p.handleAuthUserRegistered,
	); err != nil {
		// Non-fatal: a misconfigured host (no EventsExtension wired up)
		// should not block the plugin from booting. Log loudly so
		// operators can tell the demo handler is silent on purpose.
		ctx.Logger().Warn("hello-world: event subscribe failed", "error", err)
	}

	return nil
}

func (p *HelloPlugin) Shutdown() error {
	if p.eventsCancel != nil {
		p.eventsCancel()
	}
	if c := p.ctx.Load(); c != nil {
		(*c).Logger().Info("hello-world plugin shutting down")
	}
	return nil
}

// handleAuthUserRegistered is the EventsExtension callback. It is invoked
// once per delivered event on the SDK's subscribe goroutine, so we keep
// the body trivial — anything that takes more than a couple of hundred
// milliseconds should fork a goroutine to avoid back-pressuring the host
// (which times out sends after 2s and closes the stream on overflow).
func (p *HelloPlugin) handleAuthUserRegistered(ctx context.Context, evt *pluginsdk.HostEvent) {
	pctx := p.context()
	if pctx == nil {
		return
	}
	reg := evt.GetAuthUserRegistered()
	if reg == nil {
		// Defensive: SubscribedEvents should narrow the stream to this
		// payload, but the host could still send a fan-out event with
		// an empty oneof during a schema migration.
		return
	}
	pctx.Logger().Info("hello-world: user registered",
		"event_id", evt.GetEventId(),
		"user_id", reg.GetUserId(),
		"email", reg.GetEmail(),
		"source", reg.GetSource(),
		"referrer_id", reg.GetReferrerId(),
	)
}

// RegisterHTTP wires the three smoke-test routes onto the SDK-managed mux.
// The paths are mounted at the root and MUST match what Manifest declared.
func (p *HelloPlugin) RegisterHTTP(mux pluginsdk.HTTPMux) {
	// Plugin's HTTP server receives the original path (the core gateway does
	// NOT strip the /api/v1/plugin/<name> prefix), so handlers must register
	// at the full path that matches the manifest declarations above.
	mux.Handle(pluginRoutePrefix+"/hello", http.HandlerFunc(p.handleHello))
	mux.Handle(pluginRoutePrefix+"/db-test", http.HandlerFunc(p.handleDBTest))
	mux.Handle(pluginRoutePrefix+"/redis-test", http.HandlerFunc(p.handleRedisTest))
}

// OpenFrontendFile 实现 pluginsdk.FrontendBundleProvider, 把核心的 GetFrontendBundle
// 请求映射到嵌入式 FS. path 来自 manifest.Frontend.EntryJS / EntryCSS, 调用方已经在
// 核心侧做过路径穿越校验; 这里只做最小化的兜底以防误用.
func (p *HelloPlugin) OpenFrontendFile(rel string) ([]byte, error) {
	clean := path.Clean("/" + rel)
	if clean == "/" || clean == "/." {
		return nil, fs.ErrInvalid
	}
	clean = clean[1:] // strip leading "/"
	full := "frontend/" + clean
	return frontendAssets.ReadFile(full)
}

func main() {
	if err := pluginsdk.Run(&HelloPlugin{}); err != nil {
		log.Fatalf("hello-world plugin exited: %v", err)
	}
}

// ---------------------------------------------------------------------------
// HTTP handlers
// ---------------------------------------------------------------------------

func (p *HelloPlugin) handleHello(w http.ResponseWriter, r *http.Request) {
	greeting := "Hello"
	if pctx := p.context(); pctx != nil {
		// Pull the greeting from V5/W3 SettingsExtension. We fall back to
		// the hard-coded default on any error so the smoke-test endpoint
		// stays operational even when the host's plugin settings table is
		// empty (e.g. immediately after install before defaults seed).
		ctx, cancel := context.WithTimeout(r.Context(), dbTestTimeout)
		defer cancel()
		var got string
		if err := pctx.Settings().GetTyped(ctx, "greeting", &got); err == nil && got != "" {
			greeting = got
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"message": greeting + " from plugin!",
		"version": pluginVersion,
	})
}

// handleDBTest proves the SQL gRPC proxy is reachable by issuing the
// smallest possible query against the core's connection pool. It returns
// the value it read back so callers can confirm the result actually
// round-tripped (not just that the call did not error).
func (p *HelloPlugin) handleDBTest(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), dbTestTimeout)
	defer cancel()

	pctx := p.context()
	if pctx == nil {
		writeError(w, http.StatusServiceUnavailable, "plugin not yet initialised")
		return
	}

	var got int
	if err := pctx.DB().QueryRowContext(ctx, "SELECT 1").Scan(&got); err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("db query failed: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"result": got,
	})
}

// handleRedisTest writes a known value, reads it back, and reports the
// round-trip. We use SetEx so the smoke key never lingers in production
// Redis after the test has run.
func (p *HelloPlugin) handleRedisTest(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), redisTestTimeout)
	defer cancel()

	pctx := p.context()
	if pctx == nil {
		writeError(w, http.StatusServiceUnavailable, "plugin not yet initialised")
		return
	}

	rdb := pctx.Redis()
	expected := time.Now().UTC().Format(time.RFC3339Nano)

	if err := rdb.SetEx(ctx, redisTestKey, expected, redisTestTTL); err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("redis set failed: %v", err))
		return
	}
	got, err := rdb.Get(ctx, redisTestKey)
	if err != nil {
		// Distinguish a missing key (something wiped Redis between Set and
		// Get) from a transport error so the response is actionable.
		if errors.Is(err, sdkdriver.ErrRedisNil) {
			writeError(w, http.StatusBadGateway, "redis returned nil immediately after SetEx")
			return
		}
		writeError(w, http.StatusBadGateway, fmt.Sprintf("redis get failed: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       got == expected,
		"key":      redisTestKey,
		"expected": expected,
		"got":      got,
	})
}

// context returns the live PluginContext or nil if Init has not run yet.
func (p *HelloPlugin) context() pluginsdk.PluginContext {
	c := p.ctx.Load()
	if c == nil {
		return nil
	}
	return *c
}

// ---------------------------------------------------------------------------
// JSON response helpers
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		// Logging is best-effort; the response status is already on the
		// wire so we cannot recover the request.
		log.Printf("hello-world: encode response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
