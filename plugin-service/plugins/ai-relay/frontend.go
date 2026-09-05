package airelay

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	"github.com/Wei-Shaw/sub2api/plugin-service/internal/frontendhost"
)

func RegisterFrontend(mux *http.ServeMux) {
	frontendhost.RegisterHostedPlugin(mux, frontendhost.HostedPluginOptions{
		PluginKey:   "ai-relay",
		WebRoot:     relayWebRoot(),
		AssetPrefix: "/plugins/ai-relay-assets/",
	})
}

func relayWebRoot() string {
	for _, candidate := range []string{
		filepath.Join("plugins", "ai-relay", "web"),
		filepath.Join("plugin-service", "plugins", "ai-relay", "web"),
	} {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		return filepath.Join("plugins", "ai-relay", "web")
	}
	return filepath.Join(filepath.Dir(currentFile), "web")
}
