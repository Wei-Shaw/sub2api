package jshandler

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

type Config struct {
	Enabled bool   `json:"enabled"`
	Timeout string `json:"timeout"`
}

func (c Config) timeoutDuration() time.Duration {
	raw := strings.TrimSpace(c.Timeout)
	if raw == "" {
		return time.Second
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return time.Second
	}
	return d
}

func defaultConfig() Config {
	return Config{
		Enabled: false,
		Timeout: "1s",
	}
}

func parseConfigJSON(raw []byte) (Config, error) {
	cfg := defaultConfig()
	if len(strings.TrimSpace(string(raw))) == 0 {
		return cfg, nil
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, fmt.Errorf("invalid jshandler config: %w", err)
	}
	return cfg, nil
}

func isPathWithinDir(path, dir string) bool {
	cleanPath, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return false
	}
	cleanDir, err := filepath.Abs(filepath.Clean(dir))
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(cleanDir, cleanPath)
	if err != nil {
		return false
	}
	return rel == "." || (rel != "" && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..")
}
