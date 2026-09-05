package plugins

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/plugin-service/internal/pluginregistry"
)

func TestRegisterAllIncludesAIRelay(t *testing.T) {
	registry := pluginregistry.New()
	if err := RegisterAll(registry); err != nil {
		t.Fatalf("RegisterAll() error = %v", err)
	}

	plugin, ok := registry.Get("ai-relay")
	if !ok {
		t.Fatal("ai-relay plugin is not registered")
	}
	if got := plugin.Metadata().DefaultEntryPath; got != "/plugins/ai-relay" {
		t.Fatalf("DefaultEntryPath = %q, want %q", got, "/plugins/ai-relay")
	}
}
