package imagegeneration

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	"github.com/Wei-Shaw/sub2api/plugin-service/internal/frontendhost"
)

func RegisterFrontend(mux *http.ServeMux) {
	frontendhost.RegisterHostedPlugin(mux, frontendhost.HostedPluginOptions{
		PluginKey: "image-generation",
		WebRoot:   webRoot(),
	})
}

func webRoot() string {
	for _, candidate := range []string{
		filepath.Join("plugins", "image-generation", "web"),
		filepath.Join("plugin-service", "plugins", "image-generation", "web"),
	} {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		return filepath.Join("plugins", "image-generation", "web")
	}

	pluginRoot := filepath.Clean(filepath.Dir(currentFile))
	return filepath.Join(pluginRoot, "web")
}
