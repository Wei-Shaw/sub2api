// Command hello-world is the canonical smoke-test plugin for the Sub2API
// plugin runtime. It exercises every transport the SDK provides:
//   - PluginLifecycle handshake + Manifest delivery
//   - HTTP routes registered via HTTPRegistrar (no auth, admin auth)
//   - SQL proxy round-trip via PluginContext.DB()
//   - Redis proxy round-trip via PluginContext.Redis()
//
// It is deliberately tiny so any failure points to the SDK or core, not to
// the plugin itself.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
	sdkdriver "github.com/Wei-Shaw/sub2api/plugin-sdk/driver"
)

const (
	pluginVersion = "0.1.0"

	// dbTestTimeout / redisTestTimeout cap how long a single smoke-test
	// request can wait on the proxy before giving up. The values are
	// generous because the proxy hop adds ~1 RTT on top of the underlying
	// driver latency.
	dbTestTimeout    = 5 * time.Second
	redisTestTimeout = 5 * time.Second

	// redisTestKey is the namespaced key the /redis-test endpoint writes
	// and reads back. Using a stable key (instead of time-based) makes the
	// behaviour idempotent and easy to inspect from the core side.
	redisTestKey = "plugin:hello-world:smoke-test"
	redisTestTTL = 60 * time.Second
)

// HelloPlugin holds the resources the SDK injects via Init. The pluginCtx
// pointer is stored atomically so HTTP handlers can read it without a mutex
// even when they race with Init (in practice Init always finishes first).
type HelloPlugin struct {
	ctx atomic.Pointer[pluginsdk.PluginContext]
}

func (p *HelloPlugin) Manifest() *pluginsdk.Manifest {
	return &pluginsdk.Manifest{
		Name:        "hello-world",
		DisplayName: "Hello World",
		Version:     pluginVersion,
		Description: "A test plugin for the plugin system",
		Author:      "Sub2API",
		PluginEndpoints: []pluginsdk.EndpointDecl{
			{Path: "/hello", Methods: []string{http.MethodGet}, AuthType: pluginsdk.AuthTypeNone},
			{Path: "/db-test", Methods: []string{http.MethodGet}, AuthType: pluginsdk.AuthTypeAdmin},
			{Path: "/redis-test", Methods: []string{http.MethodGet}, AuthType: pluginsdk.AuthTypeAdmin},
		},
		Frontend: &pluginsdk.FrontendManifest{
			MenuItems: []pluginsdk.MenuItemDecl{
				{
					Path:      "/admin/plugins/hello-world",
					LabelKey:  "Hello World",
					Icon:      "puzzle-piece",
					Section:   pluginsdk.SectionAdmin,
					SortOrder: 999,
				},
			},
		},
	}
}

// Init stores the SDK-supplied context for handlers to use. We use an
// atomic.Pointer because Go does not let us assign through a shared interface
// value safely otherwise.
func (p *HelloPlugin) Init(ctx pluginsdk.PluginContext) error {
	p.ctx.Store(&ctx)
	ctx.Logger().Info("hello-world plugin initialised", "version", pluginVersion)
	return nil
}

func (p *HelloPlugin) Shutdown() error {
	if c := p.ctx.Load(); c != nil {
		(*c).Logger().Info("hello-world plugin shutting down")
	}
	return nil
}

// RegisterHTTP wires the three smoke-test routes onto the SDK-managed mux.
// The paths are mounted at the root and MUST match what Manifest declared.
func (p *HelloPlugin) RegisterHTTP(mux pluginsdk.HTTPMux) {
	mux.Handle("/hello", http.HandlerFunc(p.handleHello))
	mux.Handle("/db-test", http.HandlerFunc(p.handleDBTest))
	mux.Handle("/redis-test", http.HandlerFunc(p.handleRedisTest))
}

func main() {
	if err := pluginsdk.Run(&HelloPlugin{}); err != nil {
		log.Fatalf("hello-world plugin exited: %v", err)
	}
}

// ---------------------------------------------------------------------------
// HTTP handlers
// ---------------------------------------------------------------------------

func (p *HelloPlugin) handleHello(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"message": "Hello from plugin!",
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
