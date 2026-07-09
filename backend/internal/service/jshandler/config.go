package jshandler

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultScriptsSubdir = "jshandler/scripts"

type Config struct {
	Enabled     bool     `json:"enabled"`
	ScriptPaths []string `json:"script_paths"`
	Timeout     string   `json:"timeout"`
	ScriptsDir  string   `json:"scripts_dir,omitempty"`
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

func resolveScriptPaths(cfg Config, dataDir string) ([]string, error) {
	var paths []string
	base := strings.TrimSpace(cfg.ScriptsDir)
	if base == "" && dataDir != "" {
		base = filepath.Join(dataDir, defaultScriptsSubdir)
	}
	if base != "" {
		builtin, err := builtinScriptPaths(base)
		if err != nil {
			return nil, err
		}
		paths = append(paths, builtin...)
	}
	for _, p := range cfg.ScriptPaths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		resolved, err := resolveOneScriptPath(p, dataDir)
		if err != nil {
			return nil, err
		}
		paths = append(paths, resolved)
	}
	return dedupePaths(paths), nil
}

func resolveOneScriptPath(original string, dataDir string) (string, error) {
	p := original
	relative := !filepath.IsAbs(p)
	if relative {
		if dataDir == "" {
			return "", fmt.Errorf("relative script path %q requires data_dir", original)
		}
		p = filepath.Join(dataDir, p)
		if !isPathWithinDir(p, dataDir) {
			return "", fmt.Errorf("relative script path %q escapes data_dir", original)
		}
	}
	cleanPath, err := filepath.Abs(filepath.Clean(p))
	if err != nil {
		return "", err
	}
	if relative {
		resolved, errEval := filepath.EvalSymlinks(cleanPath)
		if errEval != nil {
			return "", errEval
		}
		if !isResolvedPathWithinDir(resolved, dataDir) {
			return "", fmt.Errorf("relative script path %q escapes data_dir through symlink", original)
		}
		cleanPath = resolved
	} else {
		if dataDir == "" {
			return "", fmt.Errorf("absolute script path %q requires data_dir to validate", original)
		}
		resolved, errEval := filepath.EvalSymlinks(cleanPath)
		if errEval != nil {
			return "", errEval
		}
		if !isResolvedPathWithinDir(resolved, dataDir) {
			return "", fmt.Errorf("absolute script path %q must be under data_dir", original)
		}
		cleanPath = resolved
	}
	return cleanPath, nil
}

func builtinScriptPaths(scriptsDir string) ([]string, error) {
	cleanScriptsDir, err := filepath.Abs(filepath.Clean(scriptsDir))
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(cleanScriptsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var paths []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".js") {
			continue
		}
		candidate := filepath.Join(cleanScriptsDir, name)
		resolved, errEval := filepath.EvalSymlinks(candidate)
		if errEval != nil || !isResolvedPathWithinDir(resolved, cleanScriptsDir) {
			continue
		}
		paths = append(paths, resolved)
	}
	return paths, nil
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

func isResolvedPathWithinDir(path, dir string) bool {
	resolvedDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return false
	}
	return isPathWithinDir(path, resolvedDir)
}

func dedupePaths(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}