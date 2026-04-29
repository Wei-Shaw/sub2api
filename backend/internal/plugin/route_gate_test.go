package plugin

import (
	"strings"
	"testing"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// Tests for the P12·B-1 HTTP route namespace gate. They cover the four
// scenarios called out in the design:
//   1. plugin registers /api/v1/plugin/<name>/* (own namespace) → ok
//   2. plugin registers /v1/* without http.register.gateway → fail
//   3. plugin registers /v1/* with http.register.gateway → ok
//   4. plugin registers /api/v1/admin/* → ALWAYS fail (host-reserved),
//      even with http.register.gateway capability

func mkManifest(gatewayPaths, pluginPaths []string) *pluginsdk.ManifestResponse {
	gws := make([]*pluginsdk.EndpointDeclaration, 0, len(gatewayPaths))
	for _, p := range gatewayPaths {
		gws = append(gws, &pluginsdk.EndpointDeclaration{Path: p})
	}
	pls := make([]*pluginsdk.EndpointDeclaration, 0, len(pluginPaths))
	for _, p := range pluginPaths {
		pls = append(pls, &pluginsdk.EndpointDeclaration{Path: p})
	}
	return &pluginsdk.ManifestResponse{
		GatewayEndpoints: gws,
		PluginEndpoints:  pls,
	}
}

func TestRouteGate_OwnNamespaceAllowed(t *testing.T) {
	// Both canonical forms must be allowed without any capability.
	m := mkManifest(nil, []string{
		"/api/v1/plugin/demo/foo",
		"/api/v1/plugin/demo/bar/:id",
		"/plugins/demo/legacy",
	})
	if err := validatePluginRoutePaths("demo", m, nil); err != nil {
		t.Fatalf("expected own-namespace paths to pass, got %v", err)
	}
}

func TestRouteGate_GatewayPathRequiresCapability(t *testing.T) {
	m := mkManifest([]string{"/v1/anthropic/messages"}, nil)
	err := validatePluginRoutePaths("demo", m, nil)
	if err == nil {
		t.Fatal("expected denial — http.register.gateway not held")
	}
	if !strings.Contains(err.Error(), "http.register.gateway") {
		t.Fatalf("error should mention required capability, got %v", err)
	}
}

func TestRouteGate_GatewayPathWithCapabilityAllowed(t *testing.T) {
	m := mkManifest([]string{"/v1/anthropic/messages"}, nil)
	if err := validatePluginRoutePaths("demo", m, []string{"http.register.gateway"}); err != nil {
		t.Fatalf("expected /v1/ path with capability to pass, got %v", err)
	}
}

func TestRouteGate_AdminPathAlwaysDenied(t *testing.T) {
	// Even with the gateway capability + db.core.write, /api/v1/admin/* is reserved.
	m := mkManifest(nil, []string{"/api/v1/admin/users"})
	err := validatePluginRoutePaths("demo", m,
		[]string{"http.register.gateway", "db.core.write", "redis.raw"})
	if err == nil {
		t.Fatal("expected denial — /api/v1/admin/ is host-reserved")
	}
	if !strings.Contains(err.Error(), "host-reserved") {
		t.Fatalf("error should mention host-reserved, got %v", err)
	}
}

func TestRouteGate_PluginPathOutsideOwnNamespaceRequiresGatewayCap(t *testing.T) {
	// Trying to register under a *different* plugin's namespace falls outside
	// /api/v1/plugin/demo/* and so requires http.register.gateway.
	m := mkManifest(nil, []string{"/api/v1/plugin/foreign-plugin/x"})
	err := validatePluginRoutePaths("demo", m, nil)
	if err == nil {
		t.Fatal("expected denial — path outside /api/v1/plugin/demo/* needs gateway cap")
	}
}

func TestRouteGate_EmptyPathSilentlySkipped(t *testing.T) {
	// An EndpointDeclaration without a path is meaningless — the gate ignores
	// it rather than failing the whole manifest. expandEndpoints drops these
	// entries downstream.
	m := mkManifest([]string{""}, []string{""})
	if err := validatePluginRoutePaths("demo", m, nil); err != nil {
		t.Fatalf("expected empty paths to be skipped, got %v", err)
	}
}
