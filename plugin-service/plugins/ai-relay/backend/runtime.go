package backend

import (
	"os"
	"strconv"
	"strings"
)

const defaultPluginServerPort = 8091

func resolvePublicBaseURL() string {
	if configured := strings.TrimRight(strings.TrimSpace(os.Getenv("PLUGIN_AI_RELAY_PUBLIC_BASE_URL")), "/"); configured != "" {
		return configured
	}

	port, err := strconv.Atoi(strings.TrimSpace(os.Getenv("PLUGIN_SERVER_PORT")))
	if err != nil || port < 1 || port > 65535 {
		port = defaultPluginServerPort
	}
	return "http://127.0.0.1:" + strconv.Itoa(port)
}
