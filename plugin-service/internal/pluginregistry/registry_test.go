package pluginregistry

import (
	"errors"
	"net/http"
	"testing"
)

func TestRegistry_RegisterAndList(t *testing.T) {
	registry := New()

	err := registry.Register(StaticPlugin{
		Meta: Metadata{
			Key:              "image-generation",
			Name:             "Image Generation",
			Description:      "Generate images",
			Enabled:          true,
			FrontendMode:     FrontendModeHosted,
			DefaultEntryPath: "/plugins/image-generation/index.html",
		},
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	metadata := registry.List()
	if len(metadata) != 1 {
		t.Fatalf("List() len = %d, want 1", len(metadata))
	}
	if got := metadata[0].Key; got != "image-generation" {
		t.Fatalf("List()[0].Key = %q, want %q", got, "image-generation")
	}
	if got := metadata[0].Name; got != "Image Generation" {
		t.Fatalf("List()[0].Name = %q, want %q", got, "Image Generation")
	}
	if got := metadata[0].FrontendMode; got != FrontendModeHosted {
		t.Fatalf("List()[0].FrontendMode = %q, want %q", got, FrontendModeHosted)
	}
}

func TestRegistry_RejectsDuplicateKeys(t *testing.T) {
	registry := New()
	plugin := StaticPlugin{
		Meta: Metadata{
			Key: "image-generation",
		},
	}

	if err := registry.Register(plugin); err != nil {
		t.Fatalf("Register() first error = %v", err)
	}

	err := registry.Register(plugin)
	if !errors.Is(err, ErrDuplicatePluginKey) {
		t.Fatalf("Register() duplicate error = %v, want %v", err, ErrDuplicatePluginKey)
	}
}

func TestRegistry_ListSortsByKeyAscending(t *testing.T) {
	registry := New()

	for _, key := range []string{"zeta", "alpha"} {
		err := registry.Register(StaticPlugin{
			Meta: Metadata{
				Key: key,
			},
		})
		if err != nil {
			t.Fatalf("Register(%q) error = %v", key, err)
		}
	}

	metadata := registry.List()
	if len(metadata) != 2 {
		t.Fatalf("List() len = %d, want 2", len(metadata))
	}
	if got := metadata[0].Key; got != "alpha" {
		t.Fatalf("List()[0].Key = %q, want %q", got, "alpha")
	}
	if got := metadata[1].Key; got != "zeta" {
		t.Fatalf("List()[1].Key = %q, want %q", got, "zeta")
	}
}

func TestRegistry_GetReturnsRegisteredPlugin(t *testing.T) {
	registry := New()
	plugin := StaticPlugin{
		Meta: Metadata{
			Key: "image-generation",
		},
	}

	if err := registry.Register(plugin); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	got, ok := registry.Get("image-generation")
	if !ok {
		t.Fatal("Get() ok = false, want true")
	}
	if got.Metadata().Key != "image-generation" {
		t.Fatalf("Get().Metadata().Key = %q, want %q", got.Metadata().Key, "image-generation")
	}
}

func TestRegistry_GetReturnsFalseForMissingPlugin(t *testing.T) {
	registry := New()

	got, ok := registry.Get("missing")
	if ok {
		t.Fatal("Get() ok = true, want false")
	}
	if got != nil {
		t.Fatalf("Get() plugin = %#v, want nil", got)
	}
}

type routableTestPlugin struct {
	StaticPlugin
	called bool
}

func (p *routableTestPlugin) RegisterRoutes(mux *http.ServeMux, _ RouteDeps) {
	p.called = true
	mux.HandleFunc("GET /plugins/test-plugin", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
}

func TestRegistry_RegisterRoutesCallsRegisteredPlugins(t *testing.T) {
	registry := New()
	plugin := &routableTestPlugin{
		StaticPlugin: StaticPlugin{
			Meta: Metadata{
				Key:              "test-plugin",
				Name:             "Test Plugin",
				FrontendMode:     FrontendModeHosted,
				DefaultEntryPath: "/plugins/test-plugin",
			},
		},
	}

	if err := registry.Register(plugin); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	mux := http.NewServeMux()
	registry.RegisterRoutes(mux, RouteDeps{})

	if !plugin.called {
		t.Fatal("RegisterRoutes() did not call plugin route registration")
	}
}

func TestRegistry_RejectsNilPlugin(t *testing.T) {
	registry := New()

	var plugin Plugin
	err := registry.Register(plugin)
	if !errors.Is(err, ErrInvalidPlugin) {
		t.Fatalf("Register() nil error = %v, want %v", err, ErrInvalidPlugin)
	}
}

func TestRegistry_RejectsBlankPluginKey(t *testing.T) {
	registry := New()

	for _, key := range []string{"", "   ", "\n\t"} {
		err := registry.Register(StaticPlugin{
			Meta: Metadata{
				Key: key,
			},
		})
		if !errors.Is(err, ErrPluginKeyRequired) {
			t.Fatalf("Register(%q) error = %v, want %v", key, err, ErrPluginKeyRequired)
		}
	}
}
