package config

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

// IsUnix recognizes both unix:/absolute/path and unix:///absolute/path.
func (r *RedisConfig) IsUnix() bool {
	return strings.HasPrefix(strings.TrimSpace(r.Host), "unix:")
}

// ValidateUnix validates UDS-specific options without changing existing TCP behavior.
func (r *RedisConfig) ValidateUnix() error {
	if !r.IsUnix() {
		return nil
	}
	u, err := url.Parse(strings.TrimSpace(r.Host))
	if err != nil {
		return fmt.Errorf("invalid Redis Unix socket URL: %w", err)
	}
	if u.Host != "" || u.User != nil || u.Opaque != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(r.Host, "#") {
		return fmt.Errorf("redis Unix socket URL must contain only an absolute path; configure credentials and db separately")
	}
	if !filepath.IsAbs(u.Path) || strings.ContainsRune(u.Path, 0) {
		return fmt.Errorf("redis Unix socket URL requires a non-empty absolute path")
	}
	if r.Port != 0 {
		return fmt.Errorf("redis Unix socket connections require port to be omitted, empty, or 0")
	}
	if r.EnableTLS {
		return fmt.Errorf("redis Unix socket connections require enable_tls=false")
	}
	return nil
}

// Address is used after configuration validation by runtime and setup clients.
func (r *RedisConfig) Address() string {
	if r.IsUnix() {
		if u, err := url.Parse(strings.TrimSpace(r.Host)); err == nil {
			return u.Path
		}
	}
	return fmt.Sprintf("%s:%d", r.Host, r.Port)
}
